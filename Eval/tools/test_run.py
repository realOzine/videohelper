import json
import subprocess
import sys
import tempfile
import unittest
from contextlib import ExitStack, contextmanager
from pathlib import Path
from unittest.mock import Mock, patch

import run


class RunnerBoundaries(unittest.TestCase):
    def test_retrieve_cli_passes_fused_channel_multipliers(self):
        with patch.object(sys, "argv", ["run.py", "retrieve", "--snapshot", "s", "--run-id", "r",
                                      "--speech-weight", "1.5", "--audio-weight", "0"]), \
                patch.object(run, "retrieve_run", return_value=True) as retrieve:
            self.assertEqual(run.main(), 0)
        self.assertEqual(retrieve.call_args.args[4], {"visual": 1, "speech": 1.5, "audio": 0})

    def test_binary_provenance_requires_matching_build_hash(self):
        with tempfile.TemporaryDirectory() as folder, patch.object(run, "ROOT", Path(folder)):
            binary = Path(folder) / "bin/evalbridge"
            binary.parent.mkdir()
            binary.write_bytes(b"old build")
            self.assertEqual(run.binary_provenance("evalbridge"), {
                "sha256": run.digest(binary), "build_revision": None})
            run.write_json(binary.parent / "build.json", {"code_revision": "built-revision",
                           "binaries": {"evalbridge": run.digest(binary)}})
            self.assertEqual(run.binary_provenance("evalbridge")["build_revision"], "built-revision")
            binary.write_bytes(b"replaced")
            self.assertIsNone(run.binary_provenance("evalbridge")["build_revision"])

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
                      "s3": {"bucket": "vh-eval-sample", "endpoint": "http://127.0.0.1:59000"},
                      "ml": {"token_env": "VH_EVAL_ML_TOKEN", "endpoints": ["http://127.0.0.1:18080"]}}
            path = runtime / "config.yaml"
            path.write_text(json.dumps(config))
            state = {"config_path": str(path), "bucket": "vh-eval-sample", "s3_endpoint": "http://127.0.0.1:59000",
                     "ml_endpoint": "http://127.0.0.1:18080"}
            with patch.object(run, "ROOT", root), self.assertRaises(ValueError):
                run.config_for(state)
            config["database"]["url_env"] = "VH_EVAL_DB_URL"
            config["s3"]["endpoint"] = "http://unexpected.invalid"
            path.write_text(json.dumps(config))
            with patch.object(run, "ROOT", root), self.assertRaises(ValueError):
                run.config_for(state)

    def test_index_restore_operations_cannot_overlap(self):
        with (tempfile.TemporaryDirectory() as folder, patch.object(run, "ROOT", Path(folder)),
              run.operation_lock(), self.assertRaises(RuntimeError), run.operation_lock()):
            self.fail("second operation acquired lock")

    def test_index_cancellation_records_failure_and_stops_worker(self):
        with tempfile.TemporaryDirectory() as folder, ExitStack() as stack:
            root = Path(folder)
            channels = root / "channels.json"
            channels.write_text('{"channels": {}}')
            manifest = {"videos": [{"id": "media", "path": "source.mp4"}]}
            worker = Mock()
            worker.poll.return_value = None
            for name, value in (("ROOT", root), ("corpus", lambda: (manifest, [])),
                                ("revision", lambda: "checkout"),
                                ("config_for", lambda state: {"channels_file": str(channels)}),
                                ("app_command", lambda *args: ["unused"]),
                                ("binary_provenance", lambda name: {})):
                stack.enter_context(patch.object(run, name, value))
            stack.enter_context(patch.object(run.environment, "prepare", return_value={}))
            stack.enter_context(patch.object(run.environment, "runtime_env", return_value={}))
            stack.enter_context(patch.object(run.subprocess, "Popen", return_value=worker))
            stack.enter_context(patch.object(run, "app_json", side_effect=[None, {"user_id": "1", "workspace_id": "2"},
                                                                          {"id": "3"}, KeyboardInterrupt]))
            with self.assertRaises(KeyboardInterrupt):
                run.index_run("cancelled", 1)
            summary = run.read_json(root / "runs/cancelled-index/index-run.json")
            self.assertEqual(summary["status"], "cancelled")
            self.assertIn("KeyboardInterrupt", summary["error"])
            worker.send_signal.assert_called_once_with(run.signal.SIGINT)
            worker.wait.assert_called_once_with(timeout=30)


class RetrievalOrchestration(unittest.TestCase):
    @contextmanager
    def fixture(self, script, exit_code=0, after=None):
        # Only a local Python pipe replaces the bridge; no DB, ML, Redis or S3 is contacted.
        with tempfile.TemporaryDirectory() as folder, ExitStack() as stack:
            root = Path(folder)
            channels = root / "channels.json"
            channels.write_text(json.dumps({"channels": {"visual": {"weight": 1}, "speech": {"weight": 2}, "audio": {"weight": 1}}}))
            query = {"id": "q1", "query": "查询 unchanged", "answerable": True, "category": "visual",
                     "targets": [{"media_id": "media", "start_ms": 0, "end_ms": 10,
                                  "acceptable_jump_ms": [0, 10]}]}
            manifest = {"videos": [{"id": "media", "language": "zh"}]}
            state = {"ml_endpoint": "http://not-used.invalid", "models": {}, "config_path": "unused",
                     "run_id": "original", "db_name": "vh_eval_original", "bucket": "vh-eval-original"}
            snapshot = dict.fromkeys(("dataset_id", "dataset_sha256", "labels_sha256", "snapshot_id",
                                      "index_sha256", "models_config_sha256"), "frozen")
            snapshot.update(state=state, principal={"workspace_id": 1, "user_id": 2},
                            mapping={"media": 3}, model_fingerprints={})
            config = {"channels_file": str(channels), "search": {"k": 50, "rrf_k": 60, "merge_gap_cells": 1}}
            inspection = {"passed": True, "index_sha256": "frozen"}
            for name, value in (("ROOT", root), ("corpus", lambda: (manifest, [query])),
                                ("load_snapshot", lambda name: (root, snapshot)),
                                ("config_for", lambda state: config), ("revision", lambda: "checkout"),
                                ("binary_provenance", lambda name: {"sha256": "binary", "build_revision": "built"})):
                stack.enter_context(patch.object(run, name, value))
            stack.enter_context(patch.object(run.environment, "runtime_env", return_value={"VH_EVAL_ML_TOKEN": "test"}))
            stack.enter_context(patch.object(run.environment, "verify_models", return_value={}))
            stack.enter_context(patch.object(run, "inspect", side_effect=[inspection, after or inspection]))
            popen = subprocess.Popen
            def launch(*args, **kwargs):
                kwargs.pop("cwd")
                kwargs.pop("env")
                return popen([sys.executable, "-c", script + f"\nraise SystemExit({exit_code})"], **kwargs)
            launched = stack.enter_context(patch.object(run.subprocess, "Popen", side_effect=launch))
            yield root, launched

    def test_real_pipe_preserves_scope_text_overrides_and_provenance(self):
        script = """import json, sys
for line in sys.stdin:
    request = json.loads(line)
    assert request['video_ids'] == [3] and request['text'] == '查询 unchanged'
    print(json.dumps(dict(query_id=request['query_id'], mode=request['mode'], latency_ms=1,
                          moments=[dict(video_id=3,start_ms=0,end_ms=10,jump_ms=0,score=1)])), flush=True)
"""
        with self.fixture(script) as (root, launched):
            self.assertTrue(run.retrieve_run("snapshot", "test", overrides={"k": 4, "rrf-k": 20.5, "merge-gap": 0}))
            metadata = run.read_json(root / "runs/test/metadata.json")
            self.assertEqual(metadata["search_config"], {"k": 4, "rrf_k": 20.5, "merge_gap_cells": 0})
            self.assertEqual(metadata["channels"]["fused"], ["audio", "speech", "visual"])
            self.assertEqual(metadata["fused_channel_weights"], {
                "multipliers": {"visual": 1, "speech": 1, "audio": 1},
                "effective": {"visual": 1, "speech": 2, "audio": 1}})
            self.assertEqual(metadata["bridge_build_provenance"]["build_revision"], "built")
            self.assertEqual(metadata["runtime"], {
                "run_id": "original", "db_name": "vh_eval_original", "bucket": "vh-eval-original"})
            self.assertEqual(launched.call_args.args[0][-7:], ["-k", "4", "-rrf-k", "20.5", "-merge-gap", "0", "search"])

    def test_weights_only_change_fused_requests_and_record_effective_routes(self):
        script = """import json, sys
for line in sys.stdin:
    request = json.loads(line)
    if request['mode'] == 'fused':
        assert request['channel_weights'] == dict(visual=1.0, speech=1.5, audio=0)
    else:
        assert 'channel_weights' not in request
    print(json.dumps(dict(query_id=request['query_id'], mode=request['mode'], latency_ms=1,
                          moments=[dict(video_id=3,start_ms=0,end_ms=10,jump_ms=0,score=1)])), flush=True)
"""
        with self.fixture(script) as (root, _):
            self.assertTrue(run.retrieve_run("snapshot", "weights", fused_weights={"speech": 1.5, "audio": 0}))
            metadata = run.read_json(root / "runs/weights/metadata.json")
            self.assertEqual(metadata["fused_channel_weights"], {
                "multipliers": {"visual": 1, "speech": 1.5, "audio": 0},
                "effective": {"visual": 1, "speech": 3, "audio": 0}})
            self.assertEqual(metadata["channels"]["fused"], ["speech", "visual"])
            self.assertEqual(metadata["channel_config"]["speech"]["weight"], 2)
            self.assertEqual(metadata["channels"]["visual"], ["visual"])
            self.assertEqual(metadata["channels"]["speech"], ["speech"])

    def test_invalid_fused_weights_rejected_before_resource_access(self):
        for weights in ([1], {"unknown": 1}, {"audio": True}, {"speech": "1"},
                        {"visual": -1}, {"audio": float("nan")}, {"speech": float("inf")},
                        {"visual": 0, "speech": 0, "audio": 0}):
            with self.subTest(weights=weights), patch.object(run, "corpus") as corpus:
                with self.assertRaises(ValueError):
                    run.retrieve_run("snapshot", "invalid", fused_weights=weights)
                corpus.assert_not_called()
        with self.fixture("") as (root, launched):
            with self.assertRaises(ValueError):
                run.retrieve_run("snapshot", "overflow", fused_weights={"speech": 1e308})
            run.environment.verify_models.assert_not_called()
            run.inspect.assert_not_called()
            launched.assert_not_called()
            self.assertFalse((root / "runs/overflow").exists())
        with self.fixture("") as (root, launched):
            (root / "channels.json").write_text(json.dumps({"channels": {"visual": {"weight": 1}}}))
            with self.assertRaises(ValueError):
                run.retrieve_run("snapshot", "disabled", fused_weights={"visual": 0})
            run.environment.verify_models.assert_not_called()
            run.inspect.assert_not_called()
            launched.assert_not_called()

    def test_restored_runtime_provenance_matches_selected_state(self):
        script = """import json, sys
for line in sys.stdin:
    request = json.loads(line)
    print(json.dumps(dict(query_id=request['query_id'], mode=request['mode'], latency_ms=1,
                          moments=[dict(video_id=3,start_ms=0,end_ms=10,jump_ms=0,score=1)])), flush=True)
"""
        with self.fixture(script) as (root, launched):
            state = {"run_id": "restored", "db_name": "vh_eval_restored", "bucket": "vh-eval-restored",
                     "ml_endpoint": "http://restored.invalid", "models": {}, "config_path": "restored.yaml",
                     "secret": "must not enter metadata"}
            state_path = root / "restored-state.json"
            run.write_json(state_path, state)
            self.assertTrue(run.retrieve_run("snapshot", "restore-check", restored_state=state_path))
            metadata = run.read_json(root / "runs/restore-check/metadata.json")
            self.assertEqual(metadata["runtime"], {
                "run_id": "restored", "db_name": "vh_eval_restored", "bucket": "vh-eval-restored"})
            self.assertEqual(launched.call_args.args[0][1:3], ["-config", "restored.yaml"])
            self.assertTrue(all(call.args[0] == state for call in run.inspect.call_args_list))

    def test_bad_responses_and_nonzero_exit_leave_complete_invalid_report(self):
        lines = ["malformed", json.dumps({"query_id": "unknown", "mode": "visual"}),
                 json.dumps({"query_id": "q1", "mode": "visual", "latency_ms": 1,
                             "moments": [{"video_id": 999, "media_id": "forged", "score": 1,
                                          "start_ms": 0, "end_ms": 10, "jump_ms": 0}]}),
                 json.dumps({"query_id": "q1", "mode": "speech", "latency_ms": 1,
                             "moments": [None]})]
        script = "import sys\nsys.stdout.write(" + repr("\n".join(lines) + "\n") + ")"
        with self.fixture(script, exit_code=2, after=OSError("inspection offline")) as (root, _):
            self.assertFalse(run.retrieve_run("snapshot", "invalid"))
            output = root / "runs/invalid"
            self.assertEqual((output / "raw-responses.jsonl").read_text(), "\n".join(lines) + "\n")
            report = run.read_json(output / "report.json")
            self.assertEqual(len(report["per_query"]), 3)
            self.assertTrue(all(row["status"] == "failed" for row in report["per_query"]))
            self.assertEqual(report["metadata"]["bridge_exit_code"], 2)
            self.assertEqual(len(report["metadata"]["bridge_errors"]), 2)
            self.assertFalse(report["metadata"]["index_unchanged"])

    def test_partial_line_timeout_is_cancellable_and_raw_bytes_survive(self):
        script = "import sys,time\nsys.stdout.write('{partial');sys.stdout.flush();time.sleep(30)"
        with self.fixture(script) as (root, _), patch.object(run.time, "monotonic", side_effect=[0, 0, 361]):
            self.assertFalse(run.retrieve_run("snapshot", "timeout"))
            output = root / "runs/timeout"
            self.assertEqual((output / "raw-responses.jsonl").read_bytes(), b"{partial")
            metadata = run.read_json(output / "metadata.json")
            self.assertIn("TimeoutError", metadata["bridge_errors"][0])

    def test_bridge_start_failure_still_writes_invalid_report(self):
        with self.fixture("") as (root, launched):
            launched.side_effect = OSError("executable unavailable")
            self.assertFalse(run.retrieve_run("snapshot", "unavailable"))
            report = run.read_json(root / "runs/unavailable/report.json")
            self.assertFalse(report["passed"])
            self.assertIsNone(report["metadata"]["bridge_exit_code"])

    def test_keyboard_interrupt_stops_bridge_and_records_cancelled_run(self):
        with self.fixture("import time; time.sleep(30)") as (root, _):
            with patch.object(run.selectors.DefaultSelector, "select", side_effect=KeyboardInterrupt):
                self.assertFalse(run.retrieve_run("snapshot", "cancelled"))
            metadata = run.read_json(root / "runs/cancelled/metadata.json")
            self.assertIn("KeyboardInterrupt", metadata["bridge_errors"][0])
            self.assertIsNotNone(metadata["bridge_exit_code"])

    def test_noop_and_invalid_overrides_are_rejected_before_service_access(self):
        for overrides in ({"k": 0}, {"rrf-k": 0}, {"rrf-k": float("nan")},
                          {"merge-gap": -1}, {"merge-gap": 1.5}, {"unknown": 1}):
            with self.subTest(overrides=overrides), patch.object(run, "corpus") as corpus:
                with self.assertRaises(ValueError):
                    run.retrieve_run("snapshot", "invalid", overrides=overrides)
                corpus.assert_not_called()


if __name__ == "__main__":
    unittest.main()
