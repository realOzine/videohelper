#!/bin/bash
set -euo pipefail
umask 077
base=/opt/videohelper
data=/var/lib/videohelper

if [[ ${1:-} == stop ]]; then
    # Even a failed lock, backup, or Docker command must not leave billing running.
    trap 'shutdown -h +1' EXIT
    timeout --kill-after=5s 230s bash "$0" drain
    exit
fi
if [[ ${1:-} != backup && ${1:-} != drain && ( ${1:-} != deploy || ! ${2:-} =~ ^[0-9a-f]{40}$ ) ]]; then
    echo 'Usage: host.sh deploy COMMIT_SHA | backup | stop' >&2
    exit 2
fi
exec 9>"$base/deploy.lock"
flock -n 9 || { echo 'Another deployment or backup is running' >&2; exit 1; }
source "$base/aws.env"
export AWS_REGION

compose() {
    docker compose -p videohelper-demo --env-file "$base/runtime.env" -f "$base/current/compose.yml" "$@"
}

backup() {
    test -f "$base/runtime.env" || return 0
    test -L "$base/current" || return 0
    # Stop writes before the database snapshot; leave all persistent paths intact.
    compose stop worker api web || return
    local postgres
    postgres=$(compose ps -q --status running postgres) || return
    if test -n "$postgres"; then
        mkdir -p "$data/backups" || return
        local partial dump
        partial=$(mktemp --suffix=.dump.partial "$data/backups/$(date -u +%Y%m%dT%H%M%SZ).XXXXXX") || return
        dump=${partial%.partial}
        compose exec -T postgres pg_dump -U videohelper -d videohelper -Fc >"$partial" || return
        mv "$partial" "$dump" || return
        aws s3 cp "$dump" "s3://$ARTIFACTS_BUCKET/backups/$(basename "$dump")" --only-show-errors || return
    fi
}

if [[ ${1:-} == backup ]]; then
    backup
    exit
fi
if [[ ${1:-} == drain ]]; then
    backup_status=0
    backup || backup_status=$?
    if test -L "$base/current"; then
        compose stop || backup_status=$?
    fi
    exit "$backup_status"
fi
test -f "$base/bootstrap-ready"
sha=$2
release="$base/releases/$sha"
mkdir -p "$base/releases"
staging=$(mktemp -d "$base/releases/.$sha.XXXXXX")
trap 'rm -rf -- "$staging"' EXIT
aws s3 cp "s3://$ARTIFACTS_BUCKET/releases/$sha.tar.gz" "$staging/release.tar.gz" --only-show-errors
python3 - "$base" "$staging" "$sha" <<'PY'
import pathlib, secrets, shutil, sys, tarfile
base, staging = map(pathlib.Path, sys.argv[1:3])
release = base / 'releases' / sys.argv[3]
allowed = {'compose.yml', 'config.template.yaml', 'channels.yaml', 'demo-auth.mjs', 'host.sh'}
with tarfile.open(staging / 'release.tar.gz') as archive:
    members = archive.getmembers()
    for member in members:
        if member.name not in allowed or not member.isfile():
            raise ValueError('Unexpected release member')
    if len(members) != len(allowed) or {m.name for m in members} != allowed:
        raise ValueError('Incomplete or duplicate release members')
    for member in members:
        with archive.extractfile(member) as source, (staging / member.name).open('xb') as destination:
            shutil.copyfileobj(source, destination)
(staging / 'host.sh').chmod(0o700)
config = (staging / 'config.template.yaml').read_text()
values = dict(line.split('=', 1) for line in (base / 'aws.env').read_text().splitlines())
for key in ['MEDIA_BUCKET', 'AWS_REGION']:
    config = config.replace(f'__{key}__', values[key])
(staging / 'config.yaml').write_text(config)
if release.is_symlink():
    raise ValueError('Release path must not be a symlink')
if release.exists():
    for name in allowed | {'config.yaml'}:
        path = release / name
        if path.is_symlink() or not path.is_file() or path.read_bytes() != (staging / name).read_bytes():
            raise ValueError('Release already exists with different contents; publish a new commit')
else:
    staging.rename(release)
runtime = base / 'runtime.env'
if not runtime.exists() and pathlib.Path('/var/lib/videohelper/postgres/PG_VERSION').exists():
    raise ValueError('Existing PostgreSQL data requires its original runtime.env')
env = dict(line.split('=', 1) for line in runtime.read_text().splitlines()) if runtime.exists() else {}
env.setdefault('VH_DB_PASSWORD', secrets.token_hex(32))
env.setdefault('VH_ML_TOKEN', secrets.token_hex(32))
env['ML_INTERNAL_TOKEN'] = env['VH_ML_TOKEN']
env['VH_DB_URL'] = f"postgres://videohelper:{env['VH_DB_PASSWORD']}@postgres:5432/videohelper?sslmode=disable"
env['VH_REDIS_ADDR'] = 'redis:6379'
env['AWS_REGION'] = values['AWS_REGION']
for key, tag in [('GO_IMAGE', 'go'), ('WEB_IMAGE', 'web'), ('WEMM_IMAGE', 'ml-wemm'), ('ML_CPU_IMAGE', 'ml-cpu')]:
    env[key] = f"{values['IMAGE_REPOSITORY']}:{tag}-{sys.argv[3]}"
(base / 'runtime.next.env').write_text(''.join(f'{k}={v}\n' for k, v in env.items()))
PY
aws ecr get-login-password | docker login --username AWS --password-stdin "${IMAGE_REPOSITORY%%/*}"
docker compose -p videohelper-demo --env-file "$base/runtime.next.env" -f "$release/compose.yml" pull
# Preserve the previous release and environment for operator rollback; migrations need separate review.
if test -L "$base/current"; then
    backup
    if [[ $(readlink -f "$base/current") != "$release" ]]; then
        cp "$base/runtime.env" "$base/runtime.previous.env"
        readlink -f "$base/current" >"$base/previous-release"
    fi
    compose stop
fi
mv "$base/runtime.next.env" "$base/runtime.env"
ln -sfn "$release" "$base/current.next"
mv -Tf "$base/current.next" "$base/current"
compose run --rm --no-deps -e HF_HUB_OFFLINE=0 wemm videohelper-ml prepare --models models.yaml
compose run --rm --no-deps -e HF_HUB_OFFLINE=0 speech-audio videohelper-ml prepare --models models.yaml
compose up -d --wait postgres redis auth wemm speech-audio
compose run --rm --no-deps api -config /etc/videohelper/config.yaml migrate
compose run --rm --no-deps api -config /etc/videohelper/config.yaml space add visual wemm-2b-cpu@r1
compose run --rm --no-deps api -config /etc/videohelper/config.yaml space add speech wemm-2b-cpu@r1
compose run --rm --no-deps api -config /etc/videohelper/config.yaml space add audio clap-htsat-unfused@r1
compose up -d --no-deps --force-recreate --wait --wait-timeout 900 api worker web
curl -fsS http://127.0.0.1:8080/healthz
python3 - "$base" "$IMAGE_REPOSITORY" <<'PY'
import pathlib, re, shutil, subprocess, sys
base = pathlib.Path(sys.argv[1])
root = base / 'releases'
current = (base / 'current').resolve()
previous_file = base / 'previous-release'
previous = pathlib.Path(previous_file.read_text().strip()).resolve() if previous_file.exists() else current
keep = set()
for path in (current, previous):
    if path.parent != root or not re.fullmatch('[0-9a-f]{40}', path.name):
        raise ValueError('Refusing cleanup with an unexpected retained release path')
    keep.add(path.name)
images = subprocess.check_output(['docker', 'image', 'ls', '--format', '{{.Repository}}:{{.Tag}}', sys.argv[2]], text=True)
pattern = re.compile(re.escape(sys.argv[2]) + r':(?:go|web|ml-wemm|ml-cpu|ml-cuda)-([0-9a-f]{40})')
for image in images.splitlines():
    match = pattern.fullmatch(image)
    if match and match[1] not in keep:
        # No --force: Docker refuses to delete images still used by containers.
        subprocess.run(['docker', 'image', 'rm', image], check=True)
allowed = {'compose.yml', 'config.template.yaml', 'channels.yaml', 'demo-auth.mjs', 'host.sh', 'config.yaml', 'release.tar.gz'}
for path in root.iterdir():
    if path.is_symlink() or not path.is_dir() or path.name in keep:
        continue
    if not re.fullmatch(r'[0-9a-f]{40}|\.[0-9a-f]{40}\.[A-Za-z0-9]{6}', path.name):
        continue
    if any(item.name not in allowed or not item.is_file() or item.is_symlink() for item in path.iterdir()):
        print(f'Skipping release with unexpected contents: {path}', file=sys.stderr)
        continue
    shutil.rmtree(path)
PY
echo "Deployed $sha; auto-stop remains armed for two hours after boot."
