import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import run


class RunnerBoundaries(unittest.TestCase):
    def test_existing_run_is_never_overwritten(self):
        with tempfile.TemporaryDirectory() as folder:
            parent = Path(folder)
            directory = run.fresh(parent, "example")
            (directory / "evidence").write_text("preserve")
            with self.assertRaises(FileExistsError):
                run.fresh(parent, "example")
            self.assertEqual((directory / "evidence").read_text(), "preserve")

    def test_scope_contains_whole_corpus(self):
        snapshot = {"principal": {"user_id": 4, "workspace_id": 7}, "mapping": {"a": 9, "b": 3}}
        self.assertEqual(run.scope(snapshot), {"user_id": 4, "workspace_id": 7, "video_ids": [3, 9]})

    def test_archive_tamper_and_path_escape_rejected(self):
        with tempfile.TemporaryDirectory() as folder:
            directory = Path(folder)
            (directory / "objects").mkdir()
            payload = directory / "objects" / "sample.mp4"
            payload.write_bytes(b"original")
            inventory = [{"key": "sample.mp4", "sha256": run.digest(payload)}]
            (directory / "objects.json").write_text(json.dumps(inventory))
            run.verify_objects(directory)
            payload.write_bytes(b"changed")
            with self.assertRaises(ValueError):
                run.verify_objects(directory)
            inventory[0]["key"] = "../../outside"
            (directory / "objects.json").write_text(json.dumps(inventory))
            with self.assertRaises(ValueError):
                run.verify_objects(directory)

    def test_runtime_config_rejects_development_resources(self):
        with tempfile.TemporaryDirectory() as folder, patch.object(run.environment, "runtime_env"):
            root = Path(folder)
            runtime = root / ".runtime" / "sample"
            runtime.mkdir(parents=True)
            config = {"database": {"url_env": "VH_DB_URL"}, "redis": {"addr_env": "VH_EVAL_REDIS_ADDR"},
                      "s3": {"bucket": "vh-eval-sample"}, "ml": {"token_env": "VH_EVAL_ML_TOKEN", "endpoints": ["http://127.0.0.1:18080"]}}
            path = runtime / "config.yaml"
            path.write_text(json.dumps(config))
            state = {"config_path": str(path), "bucket": "vh-eval-sample", "ml_endpoint": "http://127.0.0.1:18080"}
            with patch.object(run, "ROOT", root), self.assertRaises(ValueError):
                run.config_for(state)

    def test_index_restore_operations_cannot_overlap(self):
        with (tempfile.TemporaryDirectory() as folder, patch.object(run, "ROOT", Path(folder)),
              run.operation_lock(), self.assertRaises(RuntimeError), run.operation_lock()):
            self.fail("second operation acquired lock")


if __name__ == "__main__":
    unittest.main()
