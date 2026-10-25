import copy
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import score


def query(qid="q1", answerable=True):
    return {"id": qid, "category": "visual" if answerable else "negative",
            "answerable": answerable, "targets": [{"media_id": "media1", "start_ms": 100,
            "end_ms": 200, "acceptable_jump_ms": [90, 110]}] if answerable else []}


def moment(**updates):
    value = {"media_id": "media1", "video_id": "db-video", "start_ms": 100,
             "end_ms": 200, "jump_ms": 100, "score": 0.02}
    return value | updates


def responses(queries, moments=None):
    return [{"query_id": item["id"], "mode": mode, "latency_ms": 5.0,
             "moments": copy.deepcopy([moment()] if moments is None else moments),
             "error": "", "skipped": []} for mode in score.MODES for item in queries]


def metadata(**updates):
    return {"dataset_id": "test", "dataset_sha256": "corpus-hash", "labels_sha256": "labels-hash",
            "snapshot_id": "s1", "index_sha256": "index-hash", "code_revision": "abc",
            "model_mode": "fake", "channels": ["visual", "speech", "audio"],
            "search_config": {"limit": 10}, "run_id": "run1"} | updates


def metrics(report, mode="visual"):
    return report["by_mode"][mode]["all"]["positive_metrics"]


class ScoreTests(unittest.TestCase):
    def test_overlap_does_not_reward_huge_interval_as_tiou(self):
        items = [query()]
        report = score.evaluate(items, responses(items, [moment(start_ms=0, end_ms=10000)]), metadata())
        values = metrics(report)
        self.assertEqual(values["video_recall@1"]["value"], 1)
        self.assertEqual(values["overlap_recall@1"]["value"], 1)
        self.assertEqual(values["tiou_0.3_recall@1"]["value"], 0)
        self.assertEqual(values["best_tiou"]["value"], 0.01)

    def test_wrong_video_and_half_open_boundaries_and_rank(self):
        items = [query()]
        found = [moment(media_id="other"), moment(start_ms=200, end_ms=300, jump_ms=200),
                 moment(jump_ms=110), moment(jump_ms=109)]
        report = score.evaluate(items, responses(items, found), metadata())
        values = metrics(report)
        self.assertEqual(values["video_recall@1"]["value"], 0)
        self.assertEqual(values["video_recall@5"]["value"], 1)
        self.assertEqual(values["overlap_mrr"]["value"], 1 / 3)
        self.assertEqual(values["jump_hit@1"]["value"], 0)
        self.assertEqual(values["jump_hit@5"]["value"], 1)
        self.assertIsNone(report["per_query"][0]["localization"])
        for found in ([moment(start_ms=200, end_ms=300, jump_ms=200)], [moment(jump_ms=110)]):
            values = metrics(score.evaluate(items, responses(items, found), metadata()))
            if found[0]["start_ms"] == 200:
                self.assertEqual(values["overlap_recall@10"]["value"], 0)
            self.assertEqual(values["jump_hit@10"]["value"], 0)

    def test_tiou_threshold_and_localization_choose_same_target(self):
        items = [query()]
        items[0]["targets"].append({"media_id": "media1", "start_ms": 300,
                                    "end_ms": 400, "acceptable_jump_ms": [300, 310]})
        report = score.evaluate(items, responses(items, [moment(start_ms=250, end_ms=450, jump_ms=300)]), metadata())
        self.assertEqual(metrics(report)["tiou_0.5_recall@1"]["value"], 1)
        self.assertEqual(report["per_query"][0]["localization"], {
            "start_absolute_error_ms": 50, "end_absolute_error_ms": 50, "tiou": 0.5})
        group = report["by_mode"]["visual"]["all"]
        self.assertEqual(group["top1_correct_video_localization"]["tiou"]["n"], 1)

    def test_missing_duplicate_error_and_degraded_keep_denominator(self):
        items = [query("q1"), query("q2"), query("q3"), query("q4"), query("q5")]
        found = responses(items)
        found = [row for row in found if (row["query_id"], row["mode"]) != ("q1", "visual")]
        found.append(copy.deepcopy(next(row for row in found if row["query_id"] == "q2")))
        next(row for row in found if row["query_id"] == "q3")["error"] = "service unavailable"
        next(row for row in found if row["query_id"] == "q4")["skipped"] = ["audio"]
        report = score.evaluate(items, found, metadata())
        self.assertFalse(report["passed"])
        self.assertEqual(metrics(report)["overlap_recall@1"], {"numerator": 1, "denominator": 5, "value": 0.2})
        self.assertEqual(report["by_mode"]["visual"]["all"]["status_counts"], {"failed": 4, "ok": 1})
        self.assertEqual(metrics(report, "speech")["overlap_recall@1"]["value"], 1)
        self.assertTrue(all(row["issues"] for row in report["per_query"][:4]))

    def test_invalid_moments_fail_instead_of_scoring_partial_results(self):
        items = [query()]
        for bad in (moment(start_ms=-1), moment(start_ms=200), moment(end_ms=100),
                    moment(start_ms=1.5), moment(jump_ms=200), moment(jump_ms=True),
                    moment(media_id=""), moment(score=float("nan")), moment(score=float("inf")),
                    moment(score=10 ** 1000),
                    moment(score="1"), {"start_ms": None}, "bad"):
            with self.subTest(bad=bad):
                report = score.evaluate(items, responses(items, [moment(), bad]), metadata())
                self.assertFalse(report["passed"])
                self.assertEqual(metrics(report)["overlap_recall@1"]["value"], 0)
                json.dumps(report, allow_nan=False)

    def test_negative_failures_are_never_counted_as_empty_success(self):
        items = [query("negative1", False), query("negative2", False), query("negative3", False)]
        found = responses(items)
        found[1]["moments"] = None
        found[2]["error"] = "failed"
        report = score.evaluate(items, found, metadata())
        group = report["by_mode"]["visual"]["all"]
        self.assertEqual(group["negative_count"], 3)
        self.assertEqual(group["negative"]["nonempty_rate"], {"numerator": 1, "denominator": 2, "value": 0.5})
        self.assertEqual(group["negative"]["failed_count"], 1)
        self.assertEqual(group["negative"]["top1_scores"]["mean"], 0.02)
        self.assertIsNone(group["positive_metrics"]["overlap_mrr"]["value"])

    def test_validation_rejects_bad_labels_metadata_and_unknown_response(self):
        for items in ([], [query(), query()], [query() | {"targets": []}],
                      [query() | {"answerable": "true"}]):
            with self.subTest(items=items), self.assertRaises(ValueError):
                score.evaluate(items, [], metadata())
        with self.assertRaisesRegex(ValueError, "snapshot_id"):
            score.evaluate([query()], [], metadata(snapshot_id=None))
        with self.assertRaisesRegex(ValueError, "unknown"):
            score.evaluate([query()], [{"query_id": "unknown", "mode": "visual"}], metadata())
        with self.assertRaisesRegex(ValueError, "unknown"):
            score.evaluate([query()], [{"query_id": "q1", "mode": "audio"}], metadata())

    def test_comparison_compatibility_regressions_and_improvements(self):
        items = [query()]
        before = score.evaluate(items, responses(items), metadata())
        after = score.evaluate(items, responses(items, []), metadata(run_id="run2", snapshot_id="s2"))
        delta = score.compare_reports(before, after)
        self.assertFalse(delta["fixed_index_regression"])
        self.assertEqual(delta["positive_metric_delta"]["visual"]["overlap_mrr"], -1)
        self.assertIn("overlap_mrr", delta["per_query_changes"][0]["regressions"])
        self.assertIn("overlap_mrr", score.compare_reports(after, before)["per_query_changes"][0]["improvements"])
        for key in ("dataset_id", "dataset_sha256", "labels_sha256"):
            changed = copy.deepcopy(before)
            changed["metadata"][key] = "different"
            with self.subTest(key=key), self.assertRaisesRegex(ValueError, key):
                score.compare_reports(before, changed)
        changed = copy.deepcopy(before)
        changed["scoring"]["version"] = "different"
        with self.assertRaisesRegex(ValueError, "scoring"):
            score.compare_reports(before, changed)
        changed = copy.deepcopy(before)
        changed["per_query"].pop()
        with self.assertRaisesRegex(ValueError, "query/mode"):
            score.compare_reports(before, changed)

    def test_report_artifacts_and_cli(self):
        items = [query()]
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "queries.jsonl").write_text(json.dumps(items[0]) + "\n")
            (root / "results.jsonl").write_text("".join(json.dumps(row) + "\n" for row in responses(items)))
            (root / "metadata.json").write_text(json.dumps(metadata()))
            command = [sys.executable, str(Path(score.__file__)), "evaluate", "--queries", str(root / "queries.jsonl"),
                       "--results", str(root / "results.jsonl"), "--metadata", str(root / "metadata.json"), "--output", str(root)]
            self.assertEqual(subprocess.run(command, capture_output=True).returncode, 0)
            report = json.loads((root / "report.json").read_text())
            rows = [json.loads(line) for line in (root / "per_query.jsonl").read_text().splitlines()]
            self.assertEqual(rows, report["per_query"])
            self.assertIn("not probabilities", (root / "report.md").read_text())
            command = [sys.executable, str(Path(score.__file__)), "compare", "--before", str(root / "report.json"),
                       "--after", str(root / "report.json"), "--output", str(root)]
            self.assertEqual(subprocess.run(command, capture_output=True).returncode, 0)
            self.assertEqual(json.loads((root / "delta.json").read_text())["per_query_changes"], [])
            self.assertIn("Fixed-index", (root / "delta.md").read_text())

    def test_model_changes_and_localization_are_reported(self):
        items = [query()]
        before = score.evaluate(items, responses(items, [moment(start_ms=95, end_ms=205)]),
                                metadata(model_fingerprints={"asr": "old"}))
        after = score.evaluate(items, responses(items), metadata(model_fingerprints={"asr": "new"}))
        delta = score.compare_reports(before, after)
        self.assertFalse(delta["fixed_index_regression"])
        self.assertIn("model_fingerprints", delta["metadata_changes"])
        row = delta["per_query_changes"][0]
        self.assertEqual(row["localization_delta"]["start_absolute_error_ms"], -5)
        self.assertIn("top1_start_absolute_error_ms", row["improvements"])

    def test_cli_writes_failure_report_and_returns_nonzero(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "queries.jsonl").write_text(json.dumps(query()) + "\n")
            (root / "results.jsonl").write_text("")
            (root / "metadata.json").write_text(json.dumps(metadata()))
            result = subprocess.run([
                sys.executable, str(Path(score.__file__)), "evaluate", "--queries", str(root / "queries.jsonl"),
                "--results", str(root / "results.jsonl"), "--metadata", str(root / "metadata.json"), "--output", str(root),
            ], capture_output=True)
            self.assertEqual(result.returncode, 1, result.stderr)
            report = json.loads((root / "report.json").read_text())
            self.assertFalse(report["passed"])
            self.assertEqual(len(report["per_query"]), 3)


if __name__ == "__main__":
    unittest.main()
