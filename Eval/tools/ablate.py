"""Select fixed-index ablations using development queries only.

Usage: python Eval/tools/ablate.py summarize --plan plan.json \
    --reports Eval/runs/*/report.json --output Eval/experiments/ablation-v1/summary
Add --verify only for final selected/default verification; it never changes selection.
Plan keys: baseline_run_id, development_query_ids, verification_query_ids,
cases: [{run_id, overrides: {k, rrf-k, merge-gap}, fused_weights: {visual,speech,audio}}].
Omitted case overrides/weights inherit the baseline settings. Run retrieval separately
with run.retrieve_run; this tool reads saved evidence and never executes model calls.
Python API: summarize(plan, reports, verify=False) returns (summary, selected_config).
summary.cases keeps plan order: eligible, objective, tie_break, development[mode]
aggregates; summary.selection_basis.ranking orders eligible run IDs best first.
"""

import argparse
import json
from pathlib import Path

import score

IDENTITY = (
    "dataset_id", "dataset_sha256", "labels_sha256", "snapshot_id", "index_sha256",
    "model_mode", "model_fingerprints", "models_config_sha256", "channel_config",
    "bridge_build_provenance",
)
SEARCH_KEYS = {"k": "k", "rrf-k": "rrf_k", "merge-gap": "merge_gap_cells"}
GUARDRAILS = ("video_recall@1", "video_recall@5")
LIMITATION = ("Verification queries are an internal check, not an untouched blind test: "
              "the historical 35-query set has been used before. Labels remain synthetic.")


def _read(path):
    return json.loads(Path(path).read_text(encoding="utf-8"))


def _write(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2, allow_nan=False) + "\n",
                    encoding="utf-8")


def _split(plan):
    groups = [plan[key] for key in ("development_query_ids", "verification_query_ids")]
    for group in groups:
        if (not isinstance(group, list) or not group
                or any(not isinstance(qid, str) or not qid for qid in group)
                or len(set(group)) != len(group)):
            raise ValueError("each query split requires distinct nonempty query IDs")
    if set(groups[0]) & set(groups[1]):
        raise ValueError("development and verification query IDs overlap")
    return [set(group) for group in groups]


def _rows(report, query_ids):
    return {mode: score._aggregate([row for row in report["per_query"]
                                   if row["mode"] == mode and row["query_id"] in query_ids])
            for mode in score.MODES}


def _metrics(groups):
    return {name: value["value"] for name, value in groups["fused"]["positive_metrics"].items()}


def _gate(groups, baseline):
    current, reference = _metrics(groups), _metrics(baseline)
    return {name: {"baseline": reference[name], "candidate": current[name],
                   "passed": current[name] is not None and reference[name] is not None
                   and current[name] >= reference[name]} for name in GUARDRAILS}


def _passed(gate):
    return all(item["passed"] for item in gate.values())


def _invalid(report):
    metadata = report["metadata"]
    reasons = []
    if report.get("passed") is not True or any(row["status"] != "ok" for row in report["per_query"]):
        reasons.append("invalid query response or failed report")
    if metadata.get("index_unchanged") is not True:
        reasons.append("index drift or missing stability evidence")
    if metadata.get("bridge_exit_code") != 0 or metadata.get("bridge_errors") != []:
        reasons.append("bridge failure or missing process evidence")
    return reasons


def _compatible(baseline, report, query_ids):
    before, after = baseline["metadata"], report["metadata"]
    for key in IDENTITY:
        if before.get(key) is None or after.get(key) != before[key]:
            raise ValueError(f"incompatible comparison: {key}")
    provenance = before["bridge_build_provenance"]
    if not provenance.get("sha256") or not provenance.get("build_revision"):
        raise ValueError("missing bridge binary/build provenance")
    if baseline["scoring"] != report["scoring"]:
        raise ValueError("incompatible comparison: scoring definition")
    expected = {(qid, mode) for qid in query_ids for mode in score.MODES}
    old = {(row["query_id"], row["mode"]): row for row in baseline["per_query"]}
    new = {(row["query_id"], row["mode"]): row for row in report["per_query"]}
    if (len(old) != len(baseline["per_query"]) or len(new) != len(report["per_query"])
            or old.keys() != expected or new.keys() != expected):
        raise ValueError("incompatible comparison: complete planned query/mode set required")
    if any((old[key]["answerable"], old[key]["category"]) !=
           (new[key]["answerable"], new[key]["category"]) for key in old):
        raise ValueError("incompatible comparison: query labels")
    if any(before["channels"][mode] != after["channels"][mode] for mode in ("visual", "speech")):
        raise ValueError("incompatible comparison: control channels")


def _declared(case, baseline, report):
    before, after = baseline["metadata"], report["metadata"]
    expected = dict(before["search_config"])
    for flag, value in case.get("overrides", {}).items():
        if flag not in SEARCH_KEYS:
            raise ValueError(f"unknown planned override: {flag}")
        expected[SEARCH_KEYS[flag]] = value
    if expected != after["search_config"]:
        raise ValueError(f"{case['run_id']}: search_config differs from declared case")
    weights = dict(before["fused_channel_weights"]["multipliers"])
    weights.update(case.get("fused_weights", {}))
    if (set(weights) != {"visual", "speech", "audio"}
            or weights != after["fused_channel_weights"]["multipliers"]):
        raise ValueError(f"{case['run_id']}: weights differ from declared case")
    effective = {name: settings["weight"] * weights[name]
                 for name, settings in after["channel_config"].items()}
    if (effective != after["fused_channel_weights"]["effective"]
            or sorted(name for name, weight in effective.items() if weight > 0) != after["channels"]["fused"]):
        raise ValueError(f"{case['run_id']}: effective fused channels/weights inconsistent")


def summarize(plan, reports, verify=False):
    """Return development-only ranking and optional checks of the already selected case."""
    development, verification = _split(plan)
    cases = {case["run_id"]: case for case in plan["cases"]}
    if len(cases) != len(plan["cases"]) or plan["baseline_run_id"] not in cases:
        raise ValueError("case run IDs must be distinct and include the baseline")
    indexed = {report["metadata"]["run_id"]: report for report in reports}
    if len(indexed) != len(reports) or not indexed.keys() <= cases.keys():
        raise ValueError("reports must have distinct declared run IDs")
    baseline_id = plan["baseline_run_id"]
    if baseline_id not in indexed:
        raise ValueError("baseline report required")
    baseline = indexed[baseline_id]
    for key in IDENTITY:
        if key in plan and plan[key] != baseline["metadata"].get(key):
            raise ValueError(f"baseline differs from declared plan: {key}")
    for run_id, report in indexed.items():
        _compatible(baseline, report, development | verification)
        _declared(cases[run_id], baseline, report)
    if _invalid(baseline):
        raise ValueError("baseline report invalid: " + "; ".join(_invalid(baseline)))
    reference = _rows(baseline, development)
    if reference["fused"]["positive_count"] == 0:
        raise ValueError("development split requires positive queries")
    entries = []
    for run_id in cases:
        if run_id not in indexed:
            entries.append({"run_id": run_id, "eligible": False, "exclusion_reasons": ["not run"]})
            continue
        report = indexed[run_id]
        groups = _rows(report, development)
        metrics = _metrics(groups)
        gate = _gate(groups, reference)
        reasons = _invalid(report)
        if not _passed(gate):
            reasons.append("development video recall guardrail failed")
        entries.append({"run_id": run_id, "eligible": not reasons, "exclusion_reasons": reasons,
                        "development": groups, "development_guardrail": gate,
                        "objective": (metrics["jump_hit@1"] + metrics["tiou_0.5_recall@1"]) / 2,
                        "tie_break": [metrics["tiou_0.3_recall@1"], metrics["best_tiou"],
                                      report["metadata"]["search_config"]["k"]]})
    eligible = [entry for entry in entries if entry["eligible"]]
    ranked = sorted(eligible, key=lambda entry: (-entry["objective"], -entry["tie_break"][0],
                    -entry["tie_break"][1], entry["tie_break"][2], entry["run_id"]))
    selected_id = ranked[0]["run_id"]
    selected = indexed[selected_id]
    basis = {"query_ids": sorted(development), "mode": "fused",
             "objective": "mean(jump_hit@1, tiou_0.5_recall@1)",
             "guardrail": "video_recall@1 and @5 >= baseline on development",
             "tie_break": "tiou_0.3_recall@1 desc, best_tiou desc, k asc, run_id asc",
             "ranking": [entry["run_id"] for entry in ranked]}
    result = {"baseline_run_id": baseline_id, "selected_run_id": selected_id,
              "selection_basis": basis, "cases": entries, "verification_performed": verify,
              "recommendation": "pending_final_verification", "limitations": [LIMITATION]}
    if verify:
        checks = {}
        for name, query_ids in (("verification", verification), ("full", development | verification)):
            groups, default = _rows(selected, query_ids), _rows(baseline, query_ids)
            gate = _gate(groups, default)
            checks[name] = {"selected": groups, "baseline": default, "guardrail": gate,
                            "passed": _passed(gate)}
        result["final_verification"] = checks
        result["recommendation"] = ("verified_candidate" if selected_id != baseline_id else "retain_baseline") \
            if all(check["passed"] for check in checks.values()) else "blocked_verification_regression"
    configuration = {"run_id": selected_id, "baseline_run_id": baseline_id,
                     "overrides": cases[selected_id].get("overrides", {}),
                     "fused_weights": selected["metadata"]["fused_channel_weights"]["multipliers"],
                     "search_config": selected["metadata"]["search_config"],
                     "fused_channel_weights": selected["metadata"]["fused_channel_weights"],
                     "selection_basis": basis, "recommendation": result["recommendation"],
                     "verification_performed": verify}
    return result, configuration


def write_summary(summary, configuration, output):
    output = Path(output)
    output.mkdir(parents=True, exist_ok=True)
    _write(output / "summary.json", summary)
    _write(output / "selected_config.json", configuration)
    lines = ["# Fixed-index parameter ablation", "",
             (f"Selected on development only: `{summary['selected_run_id']}`; "
              f"baseline: `{summary['baseline_run_id']}`."), "",
             f"Recommendation: **{summary['recommendation']}**.", "",
             ("Objective: mean(fused jump_hit@1, fused tIoU≥0.5 recall@1). "
              "Fused video recall@1 and @5 must not decrease."), "",
             "| Run | Eligible | Dev objective | Exclusion |", "|---|---|---:|---|"]
    for entry in summary["cases"]:
        lines.append(f"| {entry['run_id']} | {entry['eligible']} | "
                     f"{score._number(entry.get('objective'))} | {'; '.join(entry['exclusion_reasons'])} |")
    if summary["verification_performed"]:
        lines.extend(["", "## Final verification", ""])
        for name, check in summary["final_verification"].items():
            lines.append(f"- {name} video recall guardrails passed: **{check['passed']}**.")
        lines.extend(["", "A failed final check blocks recommendation; verification never reselects a case."])
    lines.extend(["", "All filtered metrics and counts are in summary.json.", "", *summary["limitations"]])
    (output / "summary.md").write_text("\n".join(lines) + "\n", encoding="utf-8")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    command = commands.add_parser("summarize")
    command.add_argument("--plan", required=True)
    command.add_argument("--reports", nargs="+", required=True)
    command.add_argument("--output", required=True)
    command.add_argument("--verify", action="store_true")
    args = parser.parse_args()
    try:
        summary, configuration = summarize(_read(args.plan), [_read(path) for path in args.reports], args.verify)
        write_summary(summary, configuration, args.output)
    except (ValueError, OSError, KeyError, TypeError) as error:
        parser.exit(2, f"{error}\n")
    return 1 if summary["recommendation"] == "blocked_verification_regression" else 0


if __name__ == "__main__":
    raise SystemExit(main())
