"""Exercise host.sh on Linux with temporary paths and mocked external services."""
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import time
import unittest

HERE = Path(__file__).resolve().parent
REPOSITORY = '111111111111.dkr.ecr.us-east-2.amazonaws.com/demo'
FILES = ('compose.yml', 'config.template.yaml', 'channels.yaml', 'demo-auth.mjs', 'host.sh')
MOCK = r'''#!/usr/bin/env python3
import json, os, pathlib, shutil, sys, time
tool, args = pathlib.Path(sys.argv[0]).name, sys.argv[1:]
with open(os.environ['MOCK_LOG'], 'a') as log:
    log.write(json.dumps([tool, *args]) + '\n')
if tool == 'docker':
    if args[0] == 'compose':
        if 'exec' in args:
            print('database dump bytes', flush=True)
            if os.environ.get('MOCK_HANG_DUMP'):
                time.sleep(60)
            sys.exit(int(os.environ.get('MOCK_DUMP_STATUS', '0')))
        if 'ps' in args:
            print('postgres-container')
            sys.exit(int(os.environ.get('MOCK_PS_STATUS', '0')))
        if 'pull' in args:
            sys.exit(int(os.environ.get('MOCK_PULL_STATUS', '0')))
    if args[:2] == ['image', 'ls']:
        print(os.environ.get('MOCK_IMAGES', ''))
    if args[0] == 'login':
        sys.stdin.read()
elif tool == 'aws':
    if args[:2] == ['ecr', 'get-login-password']:
        print('mock-login-password')
    elif args[:2] == ['s3', 'cp']:
        if args[2].startswith('s3://'):
            shutil.copyfile(os.environ['MOCK_ARCHIVE'], args[3])
        else:
            sys.exit(int(os.environ.get('MOCK_UPLOAD_STATUS', '0')))
elif tool == 'timeout':
    assert args[:2] == ['--kill-after=5s', '230s'], args
    duration = '0.3s' if os.environ.get('MOCK_HANG_DUMP') else '5s'
    os.execv('/usr/bin/timeout', ['timeout', '--kill-after=0.1s', duration, *args[2:]])
elif tool == 'fallocate':
    with open(args[-1], 'wb') as destination:
        destination.truncate(4294967296)
elif tool == 'blkid':
    print(os.environ.get('MOCK_SWAP_TYPE', 'swap'))
elif tool == 'swapon':
    marker = pathlib.Path(os.environ['MOCK_LOG'] + '.swap')
    if args[0] == '--show=NAME':
        print(marker.read_text() if marker.exists() else '')
    else:
        marker.write_text(args[0])
'''


@unittest.skipUnless(sys.platform == 'linux', 'Run this file directly to test the Linux host in Docker')
class HostTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='videohelper-host-test-')
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.base, self.data = self.root / 'opt', self.root / 'data'
        (self.base / 'releases').mkdir(parents=True)
        self.data.mkdir()
        self.calls = self.root / 'calls.jsonl'
        self.archive = self.root / 'release.tar.gz'
        self.env = {**os.environ, 'MOCK_LOG': str(self.calls), 'MOCK_ARCHIVE': str(self.archive)}
        commands = self.root / 'bin'
        commands.mkdir()
        for name in ('aws', 'docker', 'shutdown', 'curl', 'timeout', 'fallocate', 'mkswap', 'blkid', 'swapon'):
            path = commands / name
            path.write_text(MOCK)
            path.chmod(0o700)
        self.env['PATH'] = f'{commands}:{os.environ["PATH"]}'
        # Test only the path constants; production exposes no test-directory option.
        self.script = self.root / 'host.sh'
        source = (HERE / 'host.sh').read_text().replace('/opt/videohelper', str(self.base))
        self.fstab = self.root / 'fstab'
        self.fstab.write_text('# existing mounts\n')
        source = source.replace('/etc/fstab', str(self.fstab))
        self.script.write_text(source.replace('/var/lib/videohelper', str(self.data)))
        self.script.chmod(0o700)
        (self.base / 'aws.env').write_text(
            'AWS_REGION=us-east-2\nMEDIA_BUCKET=demo-media\nARTIFACTS_BUCKET=demo-artifacts\n'
            f'IMAGE_REPOSITORY={REPOSITORY}\n'
        )
        (self.base / 'bootstrap-ready').touch()
        self.contents = {name: (HERE / name).read_bytes() for name in FILES}

    def release(self, sha):
        directory = self.base / 'releases' / sha
        directory.mkdir()
        for name, content in self.contents.items():
            (directory / name).write_bytes(content)
        template = self.contents['config.template.yaml'].decode()
        config = template.replace('__MEDIA_BUCKET__', 'demo-media')
        (directory / 'config.yaml').write_text(config.replace('__AWS_REGION__', 'us-east-2'))
        return directory

    def current(self, sha='a' * 40):
        directory = self.release(sha)
        (self.base / 'current').symlink_to(directory)
        (self.base / 'runtime.env').write_text(
            'VH_DB_PASSWORD=original-password\nVH_ML_TOKEN=original-token\n'
            f'GO_IMAGE={REPOSITORY}:go-{sha}\n'
        )
        return directory

    def archive_release(
        self, *, extra=None, omit=None, duplicate=False, symlink=False, changed=False
    ):
        with tarfile.open(self.archive, 'w:gz') as archive:
            for name, content in self.contents.items():
                if name == omit:
                    continue
                if changed and name == 'host.sh':
                    content += b'\n# changed release\n'
                member = tarfile.TarInfo(name)
                member.size = len(content)
                if symlink and name == 'host.sh':
                    member.type, member.linkname = tarfile.SYMTYPE, '/etc/passwd'
                    member.size = 0
                archive.addfile(member, io.BytesIO(content))
            if extra:
                archive.addfile(tarfile.TarInfo(extra), io.BytesIO())
            if duplicate:
                archive.addfile(tarfile.TarInfo('host.sh'), io.BytesIO())

    def run_host(self, *args, **environment):
        return subprocess.run(
            ['bash', str(self.script), *args], env={**self.env, **environment},
            capture_output=True, text=True, timeout=15,
        )

    def log(self):
        lines = self.calls.read_text().splitlines() if self.calls.exists() else []
        return [json.loads(line) for line in lines]

    def test_failed_dump_and_upload_keep_backups_and_stop_still_schedules_shutdown(self):
        current = self.current()
        original = (current / 'config.yaml').read_bytes()
        result = self.run_host('backup', MOCK_DUMP_STATUS='7')
        self.assertEqual(result.returncode, 7, result.stderr)
        partials = list((self.data / 'backups').glob('*.partial'))
        self.assertEqual(len(partials), 1)
        self.assertEqual(partials[0].read_text(), 'database dump bytes\n')
        result = self.run_host('stop', MOCK_UPLOAD_STATUS='8')
        self.assertEqual(result.returncode, 8, result.stderr)
        self.assertEqual(partials[0].read_text(), 'database dump bytes\n')
        self.assertEqual(len(list((self.data / 'backups').glob('*.dump'))), 1)
        self.assertIn(['shutdown', '-h', '+1'], self.log())
        self.assertTrue(any(call[0] == 'docker' and call[-1] == 'stop' for call in self.log()))
        self.assertEqual((current / 'config.yaml').read_bytes(), original)

    def test_all_actions_share_lock_and_busy_stop_still_schedules_shutdown(self):
        import fcntl
        self.current()
        with (self.base / 'deploy.lock').open('w') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            for args in [('backup',), ('deploy', 'b' * 40), ('stop',)]:
                result = self.run_host(*args)
                self.assertNotEqual(result.returncode, 0, args)
        self.assertEqual([call for call in self.log() if call[0] in ('docker', 'aws')], [])
        self.assertIn(['shutdown', '-h', '+1'], self.log())

    def test_stop_survives_missing_environment_and_hung_backup(self):
        self.current()
        original = (self.base / 'aws.env').read_text()
        (self.base / 'aws.env').unlink()
        result = self.run_host('stop')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(['shutdown', '-h', '+1'], self.log())
        (self.base / 'aws.env').write_text(original)
        started = time.monotonic()
        result = self.run_host('stop', MOCK_HANG_DUMP='1')
        self.assertEqual(result.returncode, 124, result.stderr)
        self.assertLess(time.monotonic() - started, 5)
        self.assertEqual(self.log().count(['shutdown', '-h', '+1']), 2)
        self.assertTrue(list((self.data / 'backups').glob('*.partial')))

    def test_invalid_archives_do_not_change_live_release(self):
        current = self.current()
        before = {path.name: path.read_bytes() for path in current.iterdir()}
        for options in (
            {'extra': '../escape'}, {'extra': 'unexpected'}, {'omit': 'host.sh'},
            {'duplicate': True}, {'symlink': True},
        ):
            with self.subTest(options=options):
                self.archive_release(**options)
                result = self.run_host('deploy', 'b' * 40)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual((self.base / 'current').resolve(), current)
                self.assertEqual(
                    {path.name: path.read_bytes() for path in current.iterdir()}, before
                )
                self.assertFalse(list((self.base / 'releases').glob('.*')))
        self.assertFalse(any(call[0] == 'docker' for call in self.log()))

    def test_same_sha_is_immutable_and_cleanup_preserves_current_previous_and_unrelated(self):
        old = self.current()
        oldest, stale = 'c' * 40, 'd' * 40
        self.release(oldest)
        self.release(stale)
        unexpected = self.release('e' * 40)
        (unexpected / 'operator-notes.txt').write_text('keep me')
        unrelated = self.base / 'releases' / 'unrelated'
        unrelated.mkdir()
        (unrelated / 'file').write_text('keep me')
        (self.base / 'previous-release').write_text(str(self.base / 'releases' / oldest) + '\n')
        target = 'b' * 40
        self.archive_release()
        images = '\n'.join([
            f'{REPOSITORY}:go-{target}', f'{REPOSITORY}:ml-wemm-{old.name}',
            f'{REPOSITORY}:go-{oldest}', f'{REPOSITORY}:ml-cpu-{stale}',
            f'unrelated/image:go-{stale}', f'{REPOSITORY}:operator-image',
        ])
        result = self.run_host('deploy', target, MOCK_IMAGES=images)
        self.assertEqual(result.returncode, 0, result.stderr)
        current = self.base / 'releases' / target
        self.assertEqual((self.base / 'current').resolve(), current)
        self.assertEqual((self.base / 'previous-release').read_text().strip(), str(old))
        previous_environment = (self.base / 'runtime.previous.env').read_bytes()
        self.assertFalse((self.base / 'releases' / oldest).exists())
        self.assertFalse((self.base / 'releases' / stale).exists())
        self.assertTrue((unexpected / 'operator-notes.txt').is_file())
        self.assertTrue((unrelated / 'file').is_file())
        removed = [call[-1] for call in self.log() if call[:3] == ['docker', 'image', 'rm']]
        self.assertEqual(
            set(removed), {f'{REPOSITORY}:go-{oldest}', f'{REPOSITORY}:ml-cpu-{stale}'}
        )
        snapshots = {
            path.name: (path.stat().st_ino, path.stat().st_mtime_ns, path.read_bytes())
            for path in current.iterdir()
        }
        result = self.run_host('deploy', target)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual({
            path.name: (path.stat().st_ino, path.stat().st_mtime_ns, path.read_bytes())
            for path in current.iterdir()
        }, snapshots)
        self.assertEqual((self.base / 'previous-release').read_text().strip(), str(old))
        self.assertEqual((self.base / 'runtime.previous.env').read_bytes(), previous_environment)
        self.assertIn('VH_DB_PASSWORD=original-password', (self.base / 'runtime.env').read_text())
        final_up = [call for call in self.log() if '--force-recreate' in call]
        self.assertTrue(final_up)
        self.assertTrue(all(
            '--no-deps' in call and call[-3:] == ['api', 'worker', 'web'] for call in final_up
        ))

    def test_changed_same_sha_and_pull_failure_do_not_drain_live_app(self):
        current = self.current()
        before = {path.name: path.read_bytes() for path in current.iterdir()}
        self.archive_release(changed=True)
        result = self.run_host('deploy', current.name)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Release already exists with different contents', result.stderr)
        self.archive_release()
        result = self.run_host('deploy', 'b' * 40, MOCK_PULL_STATUS='9')
        self.assertEqual(result.returncode, 9, result.stderr)
        self.assertEqual((self.base / 'current').resolve(), current)
        self.assertEqual({path.name: path.read_bytes() for path in current.iterdir()}, before)
        self.assertFalse(any('stop' in call for call in self.log()))
        self.assertFalse((self.data / 'backups').exists())

    def test_existing_postgres_data_requires_original_secrets(self):
        self.archive_release()
        postgres = self.data / 'postgres'
        postgres.mkdir()
        (postgres / 'PG_VERSION').write_text('16\n')
        result = self.run_host('deploy', 'b' * 40)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Existing PostgreSQL data requires its original runtime.env', result.stderr)
        self.assertFalse((self.base / 'runtime.next.env').exists())
        self.assertFalse((self.base / 'current').exists())
        self.assertEqual((postgres / 'PG_VERSION').read_text(), '16\n')

    def test_swap_is_private_persistent_and_idempotent_without_overwriting_existing_data(self):
        self.archive_release()
        swap = self.data / 'swapfile'
        swap.write_text('operator data')
        result = self.run_host('deploy', 'b' * 40)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(swap.read_text(), 'operator data')
        self.assertFalse(any(call[0] == 'mkswap' for call in self.log()))
        swap.unlink()
        for _ in range(2):
            result = self.run_host('deploy', 'b' * 40)
            self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(swap.stat().st_mode & 0o777, 0o600)
        self.assertEqual(self.fstab.read_text(), f'# existing mounts\n{swap} none swap sw 0 0\n')
        self.assertEqual(sum(call[0] == 'mkswap' for call in self.log()), 1)
        self.assertEqual(sum(call[0] == 'swapon' and call[-1] == str(swap) for call in self.log()), 1)
        result = self.run_host('deploy', 'b' * 40, MOCK_SWAP_TYPE='ext4')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Invalid existing swap file', result.stderr)
        swap.unlink()
        unrelated = self.root / 'unrelated'
        unrelated.write_text('keep')
        swap.symlink_to(unrelated)
        result = self.run_host('deploy', 'b' * 40)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(unrelated.read_text(), 'keep')


if __name__ == '__main__':
    if sys.platform != 'linux':
        subprocess.run([
            'docker', 'run', '--rm', '--network', 'none', '-v', f'{HERE}:/tests:ro',
            'python:3.12-slim', 'python', '/tests/test_host.py',
        ], check=True)
    else:
        unittest.main()
