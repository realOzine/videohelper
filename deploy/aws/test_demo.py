import contextlib
import io
import subprocess
import tarfile
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import demo


class DeploymentTests(unittest.TestCase):
    def test_release_sha_rejects_command_injection(self):
        for value in ('main', '../release', 'a' * 40 + ';id', 'A' * 40, ''):
            with self.assertRaises(ValueError):
                demo.release_sha(value)
        self.assertEqual(demo.release_sha('a' * 40), 'a' * 40)

    def test_archive_uses_committed_files_not_dirty_checkout(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / 'deploy/aws'
            source.mkdir(parents=True)
            def git(*args):
                return subprocess.run(['git', *args], cwd=root, check=True, capture_output=True, text=True).stdout.strip()
            git('init', '-q')
            for name in demo.RELEASE_FILES:
                (source / name).write_text('committed')
            git('add', 'deploy')
            git('-c', 'user.name=Test', '-c', 'user.email=test@example.invalid', 'commit', '-qm', 'fixture')
            sha = git('rev-parse', 'HEAD')
            (source / 'host.sh').write_text('uncommitted')
            archive = root / 'release.tar.gz'
            with patch.object(demo, 'HERE', source):
                demo.release_archive(sha, archive)
            with tarfile.open(archive) as result:
                self.assertEqual(set(result.getnames()), set(demo.RELEASE_FILES))
                self.assertEqual(result.extractfile('host.sh').read(), b'committed')

    @patch.object(demo, 'resources', return_value={'InstanceId': 'i-test', 'ArtifactsBucket': 'test'})
    @patch.object(demo, 'state', return_value='stopped')
    @patch.object(demo, 'aws')
    @patch.object(demo, 'remote')
    def test_deploy_does_not_start_a_stopped_instance(self, remote, aws, state, resources):
        with patch('sys.argv', ['demo.py', 'deploy', '--sha', 'a' * 40]), contextlib.redirect_stdout(io.StringIO()):
            demo.main()
        aws.assert_not_called()
        remote.assert_not_called()

    @patch.object(demo, 'resources', return_value={'InstanceId': 'i-test'})
    @patch.object(demo, 'state', return_value='running')
    @patch.object(demo, 'aws')
    @patch.object(demo, 'remote', side_effect=RuntimeError('Backup failed'))
    def test_stop_still_stops_when_backup_fails(self, remote, aws, state, resources):
        with patch('sys.argv', ['demo.py', 'stop']), contextlib.redirect_stdout(io.StringIO()):
            demo.main()
        self.assertEqual(aws.call_args_list[0].args, ('ec2', 'stop-instances', '--instance-ids', 'i-test'))
        self.assertEqual(aws.call_args_list[1].args, ('ec2', 'wait', 'instance-stopped', '--instance-ids', 'i-test'))


if __name__ == '__main__':
    unittest.main()
