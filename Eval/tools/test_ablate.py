import copy
import json
import tempfile
import unittest
from pathlib import Path

import ablate
import score


def plan():
    return {"baseline_run_id": "default", "development_query_ids": ["q1", "q2", "n1"],
            "verification_query_ids": ["q3", "n2"],
            "cases": [{"run_id": "default"}, {"run_id": "a", "overrides": {"k": 10}},
                      {"run_id": "b", "overrides": {"k": 15}}]}


def report(run_id="default", good=(), wrong=(), delayed=()):
    queries = [{"id": qid, "category": "visual" if qid.startswith("q") else "negative",
                "answerable": qid.startswith("q"),
                "targets": [{"media_id": "media", "start_ms": 100, "end_ms": 200,
                             "acceptable_jump_ms": [90, 110]}] if qid.startswith("q") else []}
               for qid in ("q1", "q2", "q3", "n1", "n2")]
    results = []
    for mode in score.MODES:
        for query in queries:
            qid = query["id"]
            moment = {"media_id": "other" if qid in wrong else "media", "video_id": 1,
                      "start_ms": 100 if qid in good else 0,
                      "end_ms": 200 if qid in good else 1000,
                      "jump_ms": 100 if qid in good else 0, "score": 0.02}
            moments = [moment | {"media_id": "other"}, moment] if qid in delayed else [moment]
            results.append({"query_id": qid, "mode": mode, "moments": moments, "latency_ms": 1})
    multipliers = {name: 1.0 for name in ("visual", "speech", "audio")}
    metadata = {"dataset_id": "synthetic", "dataset_sha256": "corpus", "labels_sha256": "labels",
                "snapshot_id": "snapshot", "index_sha256": "index", "model_mode": "real",
                "model_fingerprints": {"wemm": "modelhash"}, "models_config_sha256": "config",
                "code_revision": "revision", "run_id": run_id,
                "channel_config": {name: {"weight": 1.0} for name in multipliers},
                "channels": {"visual": ["visual"], "speech": ["speech"], "fused": sorted(multipliers)},
                "fused_channel_weights": {"multipliers": multipliers, "effective": multipliers.copy()},
                "bridge_build_provenance": {"sha256": "binary", "build_revision": "revision"},
                "index_unchanged": True, "bridge_exit_code": 0, "bridge_errors": [],
                "search_config": {"k": {"default": 20, "a": 10, "b": 15}[run_id],
                                  "rrf_k": 60, "merge_gap_cells": 1}}
    return score.evaluate(queries, results, metadata)


class AblationSelection(unittest.TestCase):
    def test_verification_cannot_improve_or_reselect_development_winner(self):
        reports = [report(), report("a", good=["q1"], wrong=["q3"]), report("b", good=["q3"])]
        summary, selected = ablate.summarize(plan(), reports)
        self.assertEqual(summary["selected_run_id"], "a")
        self.assertEqual(summary["recommendation"], "pending_final_verification")
        self.assertNotIn("final_verification", summary)
        self.assertTrue(all("full" not in entry for entry in summary["cases"]))
        self.assertEqual(summary["cases"][1]["development"]["fused"]["positive_count"], 2)
        self.assertEqual(summary["cases"][1]["development"]["fused"]["negative_count"], 1)
        self.assertEqual(selected["selection_basis"]["query_ids"], ["n1", "q1", "q2"])
        verified, selected = ablate.summarize(plan(), reports, verify=True)
        self.assertEqual(verified["selected_run_id"], "a")
        self.assertEqual(verified["recommendation"], "blocked_verification_regression")
        self.assertFalse(verified["final_verification"]["verification"]["passed"])
        self.assertEqual(selected["run_id"], "a")
        self.assertEqual(selected["recommendation"], "blocked_verification_regression")
        reports[2] = report("b")
        self.assertEqual(ablate.summarize(plan(), reports)[0]["selected_run_id"], "a")

    def test_dev_video_guardrail_and_invalid_reports_are_excluded(self):
        for bad in (report("a", good=["q1"], wrong=["q2"]), report("a", good=["q1", "q2"])):
            if not any(row["query_id"] == "q2" and row["metrics"].get("video_recall@1") == 0
                       for row in bad["per_query"]):
                bad["per_query"][0]["status"] = "failed"
            summary, _ = ablate.summarize(plan(), [report(), bad])
            self.assertEqual(summary["selected_run_id"], "default")
            self.assertFalse(summary["cases"][1]["eligible"])
        drifted = report("a", good=["q1", "q2"])
        drifted["metadata"]["index_unchanged"] = False
        summary, _ = ablate.summarize(plan(), [report(), drifted])
        self.assertFalse(summary["cases"][1]["eligible"])
        invalid = report()
        invalid["passed"] = False
        with self.assertRaisesRegex(ValueError, "baseline report invalid"):
            ablate.summarize(plan(), [invalid])
        summary, _ = ablate.summarize(plan(), [report(delayed=["q2"]),
                                               report("a", good=["q1"], wrong=["q2"])])
        gate = summary["cases"][1]["development_guardrail"]
        self.assertTrue(gate["video_recall@1"]["passed"])
        self.assertFalse(gate["video_recall@5"]["passed"])
        self.assertFalse(summary["cases"][1]["eligible"])

    def test_identity_scoring_provenance_and_plan_mismatch_rejected(self):
        for key in ablate.IDENTITY:
            changed = report("a")
            changed["metadata"][key] = "changed"
            with self.subTest(key=key), self.assertRaisesRegex(ValueError, key):
                ablate.summarize(plan(), [report(), changed])
        changed = report("a")
        changed["scoring"] = changed["scoring"] | {"version": "other"}
        with self.assertRaisesRegex(ValueError, "scoring"):
            ablate.summarize(plan(), [report(), changed])
        changed = report("a")
        changed["metadata"]["search_config"]["rrf_k"] = 100
        with self.assertRaisesRegex(ValueError, "declared case"):
            ablate.summarize(plan(), [report(), changed])
        changed = report()
        changed["metadata"]["bridge_build_provenance"]["build_revision"] = None
        with self.assertRaisesRegex(ValueError, "provenance"):
            ablate.summarize(plan(), [changed])
        settings = plan() | {"snapshot_id": "another-snapshot"}
        with self.assertRaisesRegex(ValueError, "declared plan"):
            ablate.summarize(settings, [report()])

    def test_split_ties_weights_and_artifacts(self):
        settings = plan()
        settings["cases"][1]["fused_weights"] = {"audio": 0.0}
        changed = report("a")
        changed["metadata"]["fused_channel_weights"]["multipliers"]["audio"] = 0.0
        changed["metadata"]["fused_channel_weights"]["effective"]["audio"] = 0.0
        changed["metadata"]["channels"]["fused"].remove("audio")
        summary, selected = ablate.summarize(settings, [report(), changed, report("b")], verify=True)
        self.assertEqual(summary["selected_run_id"], "a")  # Equal metrics prefer lower K.
        self.assertEqual(summary["recommendation"], "verified_candidate")
        self.assertEqual(selected["fused_weights"]["audio"], 0)
        with tempfile.TemporaryDirectory() as folder:
            ablate.write_summary(summary, selected, folder)
            self.assertEqual(json.loads((Path(folder) / "selected_config.json").read_text()), selected)
            self.assertIn("Final verification", (Path(folder) / "summary.md").read_text())
        settings = copy.deepcopy(settings)
        settings["verification_query_ids"].append("q1")
        with self.assertRaisesRegex(ValueError, "overlap"):
            ablate.summarize(settings, [report()])
        changed["per_query"].pop()
        with self.assertRaisesRegex(ValueError, "query/mode"):
            ablate.summarize(plan(), [report(), changed])


if __name__ == "__main__":
    unittest.main()
