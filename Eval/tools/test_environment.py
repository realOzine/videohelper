import copy
import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import MagicMock, patch

import environment
import yaml


class EnvironmentTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        (self.root / "config").mkdir()
        self.config = yaml.safe_load(
            (environment.ROOT / "config/local.yaml").read_text()
        )
        (self.root / "config/local.yaml").write_text(yaml.safe_dump(self.config))
        (self.root / ".env.example").write_text(
            (environment.ROOT / ".env.example").read_text()
        )
        self.patches = [
            patch.object(environment, "ROOT", self.root),
            patch.object(environment, "EVAL", self.root / "Eval"),
        ]
        for item in self.patches:
            item.start()
            self.addCleanup(item.stop)

    def test_names_reject_sql_paths_and_bucket_invalid_ids(self):
        for invalid in ("", "../run", "a_b", "x'", "A", "a-", "a" * 41, None):
            with self.subTest(invalid=invalid), self.assertRaises(ValueError):
                environment.names(invalid)
        self.assertEqual(
            environment.names("run-42"), ("vh_eval_run_42", "vh-eval-run-42")
        )

    def test_prepare_isolates_config_and_environment(self):
        client = MagicMock()
        client.list_buckets.return_value = {"Buckets": []}
        with (
            patch.object(environment, "verify_models", return_value={"spaces": []}),
            patch.object(environment, "_psql", return_value="") as psql,
            patch.object(environment, "s3_client", return_value=client),
            patch.object(environment, "start_redis") as redis,
            patch.dict(
                os.environ,
                {
                    "VH_DB_URL": "postgres://production/real",
                    "AWS_ACCESS_KEY_ID": "real-key",
                    "AWS_PROFILE": "production",
                    "AWS_SESSION_TOKEN": "secret",
                },
            ),
        ):
            state = environment.prepare("run-42")
            env = environment.runtime_env(state)
        config = yaml.safe_load(Path(state["config_path"]).read_text())
        self.assertEqual(config["database"]["url_env"], "VH_EVAL_DB_URL")
        self.assertEqual(config["redis"]["addr_env"], "VH_EVAL_REDIS_ADDR")
        self.assertEqual(config["s3"]["bucket"], "vh-eval-run-42")
        self.assertEqual(config["ml"]["token_env"], "VH_EVAL_ML_TOKEN")
        self.assertEqual(
            config["channels_file"], str(self.root / "config/channels.yaml")
        )
        self.assertTrue(Path(config["media"]["tmp_dir"]).is_dir())
        self.assertIn("/vh_eval_run_42?", env["VH_EVAL_DB_URL"])
        self.assertEqual(env["VH_EVAL_REDIS_ADDR"], "127.0.0.1:56380")
        self.assertEqual(env["AWS_ACCESS_KEY_ID"], "videohelper-local")
        self.assertNotIn("AWS_PROFILE", env)
        self.assertNotIn("AWS_SESSION_TOKEN", env)
        self.assertNotIn("videohelper-local-secret", json.dumps(state))
        self.assertNotIn("postgres://", json.dumps(state))
        psql.assert_any_call('CREATE DATABASE "vh_eval_run_42"')
        client.create_bucket.assert_called_once_with(Bucket="vh-eval-run-42")
        redis.assert_called_once()
        self.assertEqual(
            yaml.safe_load((self.root / "config/local.yaml").read_text()), self.config
        )

    def test_existing_runtime_is_never_overwritten(self):
        directory = self.root / "Eval/.runtime/run-1"
        directory.mkdir(parents=True)
        config = directory / "config.yaml"
        config.write_text("preserve me")
        with (
            patch.object(environment, "verify_models") as discover,
            self.assertRaises(FileExistsError),
        ):
            environment.prepare("run-1")
        self.assertEqual(config.read_text(), "preserve me")
        discover.assert_not_called()

    def test_existing_database_or_bucket_is_never_reused(self):
        for database_exists in (True, False):
            with self.subTest(database_exists=database_exists):
                client = MagicMock()
                client.list_buckets.return_value = {
                    "Buckets": [{"Name": "vh-eval-run-1"}]
                }
                with (
                    patch.object(environment, "verify_models", return_value={}),
                    patch.object(
                        environment,
                        "_psql",
                        return_value="1" if database_exists else "",
                    ) as psql,
                    patch.object(environment, "s3_client", return_value=client),
                    patch.object(environment, "start_redis") as redis,
                    self.assertRaises(FileExistsError),
                ):
                    environment.prepare("run-1")
                self.assertEqual(psql.call_count, 1)
                client.create_bucket.assert_not_called()
                redis.assert_not_called()
                self.assertFalse((self.root / "Eval/.runtime/run-1").exists())

    def test_tampered_state_cannot_select_other_resources(self):
        with self.assertRaises(ValueError):
            environment.runtime_env(
                {"run_id": "run-1", "db_name": "videohelper", "bucket": "videohelper"}
            )

    def test_discovery_rejects_fake_stale_and_missing_model(self):
        expected = {
            "spaces": [{"id": "visual", "fingerprint": "real"}],
            "transcribers": [],
        }
        for actual in (
            {"spaces": [], "transcribers": []},
            {"spaces": [{"id": "visual", "fingerprint": "fake"}], "transcribers": []},
            {
                "spaces": [{"id": "visual", "fingerprint": "real"}],
                "transcribers": [],
                "extra": True,
            },
        ):
            response = MagicMock()
            response.__enter__.return_value.read.return_value = json.dumps(
                actual
            ).encode()
            opener = MagicMock()
            opener.open.return_value = response
            with (
                patch.object(environment, "build_opener", return_value=opener),
                patch.object(
                    environment, "expected_models", return_value=copy.deepcopy(expected)
                ),
                self.assertRaises(RuntimeError),
            ):
                environment.verify_models("http://localhost:18080", "secret")
        response.__enter__.return_value.read.return_value = json.dumps(
            expected
        ).encode()
        with (
            patch.object(environment, "build_opener", return_value=opener),
            patch.object(environment, "expected_models", return_value=expected),
        ):
            self.assertEqual(
                environment.verify_models("http://localhost:18080", "secret"), expected
            )
        headers = opener.open.call_args.args[0].headers
        self.assertEqual(headers["X-internal-token"], "secret")
        self.assertIn("X-request-id", headers)

    def test_nonempty_redis_is_not_flushed(self):
        result = MagicMock(stdout="12\n")
        with (
            patch.object(environment.subprocess, "run", return_value=result) as run,
            self.assertRaises(RuntimeError),
        ):
            environment.start_redis()
        self.assertEqual(run.call_count, 2)
        self.assertFalse(any("FLUSH" in str(call) for call in run.call_args_list))


if __name__ == "__main__":
    unittest.main()
