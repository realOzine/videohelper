#!/usr/bin/env python3
"""Operate only the dedicated demo instance. AWS credentials stay in the AWS CLI."""
import argparse
import io
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import tarfile
import tempfile
import time

HERE = Path(__file__).resolve().parent
RELEASE_FILES = ('compose.yml', 'config.template.yaml', 'channels.yaml', 'demo-auth.mjs', 'host.sh')


def aws(*args):
    result = subprocess.run(['aws', *args, '--no-cli-pager', '--output', 'json'], text=True, capture_output=True)
    if result.returncode:
        raise RuntimeError(result.stderr.strip())
    return json.loads(result.stdout) if result.stdout.strip() else {}


def release_sha(value):
    if not re.fullmatch(r'[0-9a-f]{40}', value):
        raise ValueError('Release must be a full lowercase Git commit SHA')
    return value


def resources():
    keys = {'InstanceId': 'VH_INSTANCE_ID', 'ArtifactsBucket': 'VH_ARTIFACTS_BUCKET', 'ImageRepository': 'VH_IMAGE_REPOSITORY'}
    if all(os.environ.get(key) for key in keys.values()):
        return {key: os.environ[env] for key, env in keys.items()}
    stack = aws('cloudformation', 'describe-stacks', '--stack-name', 'videohelper-demo')['Stacks'][0]
    return {item['OutputKey']: item['OutputValue'] for item in stack['Outputs']}


def state(instance):
    result = aws('ec2', 'describe-instances', '--instance-ids', instance)
    return result['Reservations'][0]['Instances'][0]['State']['Name']


def remote(instance, command, timeout=5400):
    deadline = time.monotonic() + 600
    while time.monotonic() < deadline:
        info = aws('ssm', 'describe-instance-information', '--filters', json.dumps([{'Key': 'InstanceIds', 'Values': [instance]}]))
        if any(item['PingStatus'] == 'Online' for item in info['InstanceInformationList']):
            break
        time.sleep(10)
    else:
        raise RuntimeError('SSM is not online; inspect EC2/cloud-init before retrying')
    result = aws('ssm', 'send-command', '--instance-ids', instance, '--document-name', 'AWS-RunShellScript',
                 '--parameters', json.dumps({'commands': [command], 'executionTimeout': [str(timeout)]}))
    command_id = result['Command']['CommandId']
    print(f'SSM command: {command_id}', flush=True)
    deadline = time.monotonic() + timeout + 60
    while time.monotonic() < deadline:
        try:
            result = aws('ssm', 'get-command-invocation', '--command-id', command_id, '--instance-id', instance)
        except RuntimeError as error:
            if 'InvocationDoesNotExist' not in str(error):
                raise
            time.sleep(3)
            continue
        if result['Status'] not in ('Pending', 'InProgress', 'Delayed'):
            print(result.get('StandardOutputContent', ''), end='')
            if result['Status'] != 'Success':
                raise RuntimeError(f"SSM {result['Status']}: {result.get('StandardErrorContent', '')}")
            return
        time.sleep(10)
    raise RuntimeError('SSM command wait timed out; the host auto-stop timer remains armed')


def deploy_command(bucket, sha):
    release_sha(sha)
    url = shlex.quote(f's3://{bucket}/releases/{sha}/host.sh')
    return f'set -eu; timeout 900 cloud-init status --wait; test -f /opt/videohelper/bootstrap-ready; aws s3 cp {url} /opt/videohelper/deploy.sh --only-show-errors; chmod 700 /opt/videohelper/deploy.sh; /opt/videohelper/deploy.sh deploy {sha}'


def release_archive(sha, archive):
    release_sha(sha)
    with tarfile.open(archive, 'w:gz') as output:
        for name in RELEASE_FILES:
            content = subprocess.run(['git', 'show', f'{sha}:deploy/aws/{name}'], cwd=HERE,
                                     check=True, capture_output=True).stdout
            info = tarfile.TarInfo(name)
            info.size = len(content)
            info.mode = 0o700 if name == 'host.sh' else 0o600
            output.addfile(info, io.BytesIO(content))


def publish(values, sha):
    release_sha(sha)
    repository = values['ImageRepository'].split('/', 1)[1]
    # Never move the desired release until every image exists.
    for image in ('go', 'web', 'ml-wemm', 'ml-cpu'):
        aws('ecr', 'describe-images', '--repository-name', repository, '--image-ids', f'imageTag={image}-{sha}')
    with tempfile.TemporaryDirectory() as directory:
        archive = Path(directory) / 'release.tar.gz'
        release_archive(sha, archive)
        host = Path(directory) / 'host.sh'
        with tarfile.open(archive) as source:
            host.write_bytes(source.extractfile('host.sh').read())
        bucket = values['ArtifactsBucket']
        aws('s3api', 'put-object', '--bucket', bucket, '--key', f'releases/{sha}.tar.gz', '--body', str(archive))
        aws('s3api', 'put-object', '--bucket', bucket, '--key', f'releases/{sha}/host.sh', '--body', str(host))
        latest = Path(directory) / 'latest.json'
        latest.write_text(json.dumps({'sha': sha}))
        aws('s3api', 'put-object', '--bucket', bucket, '--key', 'releases/latest.json', '--body', str(latest))


def latest(values):
    with tempfile.TemporaryDirectory() as directory:
        path = Path(directory) / 'latest.json'
        aws('s3api', 'get-object', '--bucket', values['ArtifactsBucket'], '--key', 'releases/latest.json', str(path))
        return release_sha(json.loads(path.read_text())['sha'])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['status', 'publish', 'deploy', 'start', 'stop', 'connect', 'shell'])
    parser.add_argument('--sha', type=release_sha)
    args = parser.parse_args()
    os.environ.setdefault('AWS_REGION', 'us-east-2')
    values = resources()
    instance = values['InstanceId']
    if args.action == 'publish':
        if not args.sha:
            parser.error('publish requires --sha')
        publish(values, args.sha)
        return
    current = state(instance)
    print(f'{instance}: {current}', flush=True)
    if args.action == 'status':
        return
    if args.action == 'stop':
        if current == 'stopped':
            return
        if current == 'running':
            try:
                remote(instance, 'if test -x /opt/videohelper/current/host.sh; then /opt/videohelper/current/host.sh stop; else shutdown -h +1; fi', 300)
            except RuntimeError as error:
                print(f'Backup/graceful-stop warning: {error}', flush=True)
        aws('ec2', 'stop-instances', '--instance-ids', instance)
        aws('ec2', 'wait', 'instance-stopped', '--instance-ids', instance)
        print('Stopped; retained storage still incurs charges.')
        return
    if args.action in ('deploy', 'start'):
        sha = args.sha or latest(values)
        if current == 'stopped' and args.action == 'deploy':
            print(f'Release {sha} is published. Start explicitly to deploy; no instance started.')
            return
        if current == 'stopped':
            aws('ec2', 'start-instances', '--instance-ids', instance)
        elif current not in ('running', 'pending'):
            raise RuntimeError(f'Wait for the instance to finish {current} before retrying')
        aws('ec2', 'wait', 'instance-running', '--instance-ids', instance)
        remote(instance, deploy_command(values['ArtifactsBucket'], sha))
        return
    if current != 'running':
        raise RuntimeError('Start the demo before connecting')
    command = ['aws', 'ssm', 'start-session', '--target', instance]
    if args.action == 'connect':
        command += ['--document-name', 'AWS-StartPortForwardingSession', '--parameters', json.dumps({'portNumber': ['8080'], 'localPortNumber': ['8080']})]
    subprocess.run(command, check=True)


if __name__ == '__main__':
    main()
