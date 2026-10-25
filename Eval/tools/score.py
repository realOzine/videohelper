"""Standard-library scoring. Run `python3 Eval/tools/score.py --help` for CLI use."""

import argparse
import json
import math
from collections import Counter, defaultdict
from pathlib import Path

MODES = ("visual", "speech", "fused")
KS = (1, 5, 10)
METADATA_KEYS = (
    "dataset_id", "dataset_sha256", "labels_sha256", "snapshot_id",
    "index_sha256", "code_revision", "model_mode", "channels", "search_config", "run_id",
)
SCORING = {
    "version": "videohelper-eval-v1",
    "modes": list(MODES),
    "k": list(KS),
    "intervals": "half-open integer milliseconds",
    "positive_denominator": "all answerable queries, including failures",
    "failure_policy": "missing, duplicate, invalid, error or skipped => zero positive credit",
    "ranking": "input order; no score sorting or deduplication",
    "matching": "same media_id; any annotated target",
    "tiou_thresholds": [0.3, 0.5],
    "mrr": "reciprocal first same-media overlapping rank across all returned moments",
    "localization": "rank-1 only when correct media; target maximizing tIoU, then minimum boundary error",
    "best_tiou": "maximum across all returned same-media moments, failures and misses zero",
    "negative_denominator": "valid responses only; failures separately counted, never empty successes",
    "percentiles": "nearest rank: sorted values[ceil(p*n)-1]",
}
LIMITATIONS = [
    "Labels are synthetic script/timeline derived and have not been independently human reviewed.",
    "Fused uses active channels including audio; this corpus has no independent sound-channel gold labels.",
    "Negative nonempty rates describe returned candidates, not calibrated refusal quality. RRF scores are not probabilities.",
    "Fake model runs establish process behavior only, not real-model retrieval quality.",
]


def _interval(start, end):
    return type(start) is int and type(end) is int and 0 <= start < end


def _finite(value):
    try:
        return type(value) in (int, float) and math.isfinite(value)
    except OverflowError:
        return False


def _ratio(numerator, denominator):
    return {"numerator": numerator, "denominator": denominator,
            "value": numerator / denominator if denominator else None}


def _stats(values):
    ordered = sorted(values)
    return {"n": len(values), "mean": sum(values) / len(values) if values else None,
            "min": min(values) if values else None, "max": max(values) if values else None,
            "p50": ordered[math.ceil(0.5 * len(values)) - 1] if values else None,
            "p95": ordered[math.ceil(0.95 * len(values)) - 1] if values else None}


def _validate_queries(queries):
    seen = set()
    for query in queries:
        qid = query.get("id")
        if not isinstance(qid, str) or not qid or qid in seen:
            raise ValueError(f"missing or duplicate query id: {qid!r}")
        seen.add(qid)
        if type(query.get("answerable")) is not bool or not query.get("category"):
            raise ValueError(f"{qid}: answerable/category required")
        targets = query.get("targets")
        if not isinstance(targets, list) or bool(targets) != query["answerable"]:
            raise ValueError(f"{qid}: positive requires targets; negative requires none")
        for target in targets:
            jump = target.get("acceptable_jump_ms")
            if (not isinstance(target.get("media_id"), str) or not target["media_id"]
                    or not _interval(target.get("start_ms"), target.get("end_ms"))
                    or not isinstance(jump, list) or len(jump) != 2
                    or not _interval(*jump)):
                raise ValueError(f"{qid}: invalid target interval/media/jump window")
    if not queries:
        raise ValueError("queries must not be empty")


def _response_issues(response):
    issues = []
    if response.get("error"):
        issues.append(f"error: {response['error']}")
    if response.get("skipped"):
        issues.append(f"degraded/skipped: {response['skipped']}")
    if not _finite(response.get("latency_ms")) or response["latency_ms"] < 0:
        issues.append("invalid latency_ms")
    moments = response.get("moments")
    if moments is None:
        # Go encodes a nil successful result slice as null.
        moments = []
    if "moments" not in response or not isinstance(moments, list):
        return issues + ["missing or invalid moments"]
    for index, moment in enumerate(moments):
        if not isinstance(moment, dict):
            issues.append(f"moment {index}: expected object")
            continue
        video_id = moment.get("video_id")
        valid_video_id = ((type(video_id) is int and video_id > 0)
                          or (isinstance(video_id, str) and video_id.isascii()
                              and video_id.isdecimal() and int(video_id) > 0))
        if (not isinstance(moment.get("media_id"), str) or not moment["media_id"]
                or not valid_video_id):
            issues.append(f"moment {index}: media_id and positive video_id required")
        if (not _interval(moment.get("start_ms"), moment.get("end_ms"))
                or type(moment.get("jump_ms")) is not int
                or not moment.get("start_ms", 0) <= moment.get("jump_ms", -1) < moment.get("end_ms", 0)):
            issues.append(f"moment {index}: invalid interval/jump")
        if not _finite(moment.get("score")):
            issues.append(f"moment {index}: nonfinite or invalid score")
    return issues


def _tiou(moment, target):
    intersection = max(0, min(moment["end_ms"], target["end_ms"])
                       - max(moment["start_ms"], target["start_ms"]))
    union = max(moment["end_ms"], target["end_ms"]) - min(moment["start_ms"], target["start_ms"])
    return intersection / union


def _positive_metrics(targets, moments):
    matches = []
    for moment in moments:
        same = [target for target in targets if target["media_id"] == moment["media_id"]]
        matches.append({
            "video": bool(same),
            "overlap": any(_tiou(moment, target) > 0 for target in same),
            "tiou_0.3": any(_tiou(moment, target) >= 0.3 for target in same),
            "tiou_0.5": any(_tiou(moment, target) >= 0.5 for target in same),
            "jump": any(target["acceptable_jump_ms"][0] <= moment["jump_ms"]
                        < target["acceptable_jump_ms"][1] for target in same),
        })
    metrics = {f"{name}_recall@{k}": int(any(match[name] for match in matches[:k]))
               for name in ("video", "overlap", "tiou_0.3", "tiou_0.5") for k in KS}
    metrics.update({f"jump_hit@{k}": int(any(match["jump"] for match in matches[:k])) for k in KS})
    metrics["overlap_mrr"] = next((1 / rank for rank, match in enumerate(matches, 1)
                                   if match["overlap"]), 0)
    metrics["best_tiou"] = max((_tiou(moment, target) for moment in moments
                                for target in targets if moment["media_id"] == target["media_id"]), default=0)
    localization = None
    if moments and matches[0]["video"]:
        moment = moments[0]
        target = max((target for target in targets if target["media_id"] == moment["media_id"]),
                     key=lambda target: (_tiou(moment, target),
                                         -abs(moment["start_ms"] - target["start_ms"])
                                         - abs(moment["end_ms"] - target["end_ms"])))
        localization = {"start_absolute_error_ms": abs(moment["start_ms"] - target["start_ms"]),
                        "end_absolute_error_ms": abs(moment["end_ms"] - target["end_ms"]),
                        "tiou": _tiou(moment, target)}
    return metrics, localization


def _aggregate(rows):
    positives = [row for row in rows if row["answerable"]]
    negatives = [row for row in rows if not row["answerable"]]
    valid_negatives = [row for row in negatives if row["status"] == "ok"]
    names = positives[0]["metrics"] if positives else _positive_metrics([], [])[0]
    return {
        "query_count": len(rows), "positive_count": len(positives), "negative_count": len(negatives),
        "status_counts": dict(sorted(Counter(row["status"] for row in rows).items())),
        "positive_metrics": {name: _ratio(sum(row["metrics"][name] for row in positives), len(positives))
                             for name in names},
        "top1_correct_video_localization": {
            name: _stats([row["localization"][name] for row in positives if row["localization"] is not None])
            for name in ("start_absolute_error_ms", "end_absolute_error_ms", "tiou")
        },
        "negative": {
            "failed_count": len(negatives) - len(valid_negatives),
            "nonempty_rate": _ratio(sum(row["result_count"] > 0 for row in valid_negatives), len(valid_negatives)),
            "top1_scores": _stats([row["top1_score"] for row in valid_negatives if row["top1_score"] is not None]),
        },
        "latency_ms": _stats([row["latency_ms"] for row in rows if row["latency_ms"] is not None]),
    }


def evaluate(queries, results, metadata):
    """Score every query in all three modes; bad responses receive no positive credit."""
    queries, results = list(queries), list(results)
    _validate_queries(queries)
    for key in METADATA_KEYS:
        if key not in metadata or metadata[key] is None:
            raise ValueError(f"metadata requires {key}")
    # Also reject NaN/Infinity anywhere in metadata before writing JSON artifacts.
    json.dumps(metadata, allow_nan=False)
    languages = metadata.get("languages", {})
    if not isinstance(languages, dict) or any(not isinstance(value, str) or not value for value in languages.values()):
        raise ValueError("metadata languages must map query IDs to nonempty language strings")
    query_ids = {query["id"] for query in queries}
    indexed = defaultdict(list)
    for response in results:
        if not isinstance(response, dict):
            raise TypeError("result must be an object")
        key = response.get("query_id"), response.get("mode")
        if key[0] not in query_ids or key[1] not in MODES:
            raise ValueError(f"unknown result query/mode: {key}")
        indexed[key].append(response)
    rows = []
    for mode in MODES:
        for query in queries:
            responses = indexed[query["id"], mode]
            issues = (["missing result"] if not responses else
                      [f"duplicate results: {len(responses)}"] if len(responses) > 1 else
                      _response_issues(responses[0]))
            response = responses[0] if len(responses) == 1 else {}
            moments = (response.get("moments") or []) if not issues else []
            metrics, localization = _positive_metrics(query["targets"], moments)
            rows.append({"query_id": query["id"], "mode": mode, "category": query["category"],
                         "language": languages.get(query["id"], "unknown"),
                         "answerable": query["answerable"], "status": "failed" if issues else "ok",
                         "issues": issues, "result_count": len(moments) if not issues else None,
                         "top1_score": moments[0]["score"] if moments else None,
                         "latency_ms": response.get("latency_ms") if not issues else None,
                         "metrics": metrics if query["answerable"] else {},
                         "localization": localization})
    by_mode = {}
    for mode in MODES:
        selected = [row for row in rows if row["mode"] == mode]
        by_mode[mode] = {"all": _aggregate(selected), "by_category": {
            category: _aggregate([row for row in selected if row["category"] == category])
            for category in sorted({row["category"] for row in selected})}}
        if languages:
            by_mode[mode]["by_language"] = {
                language: _aggregate([row for row in selected if row["language"] == language])
                for language in sorted({row["language"] for row in selected})}
    return {"scoring": SCORING, "metadata": dict(metadata), "limitations": LIMITATIONS,
            "passed": all(row["status"] == "ok" for row in rows), "by_mode": by_mode,
            "per_query": rows}


def _json_write(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2, allow_nan=False) + "\n", encoding="utf-8")


def _number(value):
    return "n/a" if value is None else f"{value:.6g}"


def _report_markdown(report):
    lines = ["# Retrieval evaluation", "", f"Run: `{report['metadata']['run_id']}`", "",
             f"All query-mode responses valid: **{report['passed']}**", "", "## Metadata", "",
             "```json", json.dumps(report["metadata"], ensure_ascii=False, indent=2), "```", ""]
    for mode, groups in report["by_mode"].items():
        sections = [("all", groups["all"]), *groups["by_category"].items()]
        sections.extend((f"language: {language}", group) for language, group in groups.get("by_language", {}).items())
        for category, group in sections:
            lines.extend([f"## {mode} / {category}", "",
                          (f"Queries: {group['query_count']}; positives: {group['positive_count']}; "
                          f"negatives: {group['negative_count']}; status: {group['status_counts']}"), "",
                          "| Metric | Sum / denominator | Value |", "|---|---:|---:|"])
            for name, metric in group["positive_metrics"].items():
                lines.append(f"| {name} | {_number(metric['numerator'])} / {metric['denominator']} | {_number(metric['value'])} |")
            negative = group["negative"]
            metric = negative["nonempty_rate"]
            lines.extend([f"| negative_nonempty_rate | {metric['numerator']} / {metric['denominator']} valid negatives | {_number(metric['value'])} |",
                          "", f"Negative failures: {negative['failed_count']}; top-1 scores: `{negative['top1_scores']}`.", "",
                          f"Correct-video rank-1 localization (conditional sample counts): `{group['top1_correct_video_localization']}`.", "",
                          f"Latency ms: `{group['latency_ms']}`.", ""])
    failures = [row for row in report["per_query"] if row["status"] != "ok"]
    if failures:
        lines.extend(["## Failed responses", ""])
        lines.extend(f"- {row['query_id']} / {row['mode']}: {row['issues']}" for row in failures)
        lines.append("")
    lines.extend(["## Scoring definition", "", "```json",
                  json.dumps(report["scoring"], ensure_ascii=False, indent=2), "```", "",
                  "## Limits", "", *[f"- {limit}" for limit in report["limitations"]]])
    return "\n".join(lines) + "\n"


def write_report(report, output_dir):
    """Write portable report artifacts. report.json includes rows for standalone comparison."""
    output = Path(output_dir)
    output.mkdir(parents=True, exist_ok=True)
    _json_write(output / "report.json", report)
    (output / "report.md").write_text(_report_markdown(report), encoding="utf-8")
    (output / "per_query.jsonl").write_text("".join(
        json.dumps(row, ensure_ascii=False, allow_nan=False) + "\n" for row in report["per_query"]), encoding="utf-8")


def compare_reports(before, after):
    """Require fixed corpus/labels/scoring; annotate changed retrieval conditions."""
    for key in ("dataset_id", "dataset_sha256", "labels_sha256"):
        if before["metadata"][key] != after["metadata"][key]:
            raise ValueError(f"incompatible comparison: {key}")
    if before["scoring"] != after["scoring"]:
        raise ValueError("incompatible comparison: scoring definition")
    old = {(row["query_id"], row["mode"]): row for row in before["per_query"]}
    new = {(row["query_id"], row["mode"]): row for row in after["per_query"]}
    if (len(old) != len(before["per_query"]) or len(new) != len(after["per_query"])
            or old.keys() != new.keys()):
        raise ValueError("incompatible comparison: query/mode set")
    changes = {key: {"before": before["metadata"].get(key), "after": after["metadata"].get(key)}
               for key in sorted(before["metadata"].keys() | after["metadata"].keys())
               if before["metadata"].get(key) != after["metadata"].get(key)}
    rows = []
    for key, previous in old.items():
        current = new[key]
        if (previous["answerable"], previous["category"]) != (current["answerable"], current["category"]):
            raise ValueError("incompatible comparison: query labels")
        delta = {name: current["metrics"][name] - value for name, value in previous["metrics"].items()
                 if current["metrics"][name] != value}
        improvements = [name for name, value in delta.items() if value > 0]
        regressions = [name for name, value in delta.items() if value < 0]
        localization_delta = {}
        if previous["localization"] is not None and current["localization"] is not None:
            for name, value in previous["localization"].items():
                difference = current["localization"][name] - value
                if difference:
                    localization_delta[name] = difference
                    improved = difference > 0 if name == "tiou" else difference < 0
                    (improvements if improved else regressions).append(f"top1_{name}")
        if previous["status"] != current["status"]:
            (improvements if current["status"] == "ok" else regressions).append("response_validity")
        if delta or previous["status"] != current["status"] or previous["top1_score"] != current["top1_score"] or previous["result_count"] != current["result_count"] or previous["localization"] != current["localization"]:
            rows.append({"query_id": key[0], "mode": key[1], "answerable": current["answerable"],
                         "metric_delta": delta, "improvements": improvements, "regressions": regressions,
                         "localization_delta": localization_delta,
                         "before": previous, "after": current})
    metric_delta = {}
    for mode in MODES:
        metric_delta[mode] = {}
        for name, metric in before["by_mode"][mode]["all"]["positive_metrics"].items():
            value = after["by_mode"][mode]["all"]["positive_metrics"][name]["value"]
            metric_delta[mode][name] = value - metric["value"] if value is not None and metric["value"] is not None else None
    fixed = not any(key in changes for key in (
        "snapshot_id", "index_sha256", "model_mode", "model_fingerprints",
        "models_config_sha256",
    ))
    identical_conditions = fixed and not any(key in changes for key in ("channels", "search_config"))
    return {"before_run_id": before["metadata"]["run_id"], "after_run_id": after["metadata"]["run_id"],
            "compatible": True, "fixed_index_regression": fixed, "metadata_changes": changes,
            "identical_retrieval_conditions": identical_conditions,
            "positive_metric_delta": metric_delta, "per_query_changes": rows,
            "summary_before": before["by_mode"], "summary_after": after["by_mode"],
            "limitations": LIMITATIONS}


def write_comparison(delta, output_dir):
    output = Path(output_dir)
    output.mkdir(parents=True, exist_ok=True)
    _json_write(output / "delta.json", delta)
    lines = ["# Evaluation comparison", "", f"`{delta['before_run_id']}` → `{delta['after_run_id']}`", "",
             (f"Compatible corpus/labels/scoring: **{delta['compatible']}**. "
             f"Fixed-index regression: **{delta['fixed_index_regression']}**. "
             f"Identical retrieval conditions: **{delta['identical_retrieval_conditions']}**."), "",
             "## Metadata changes", "", "```json", json.dumps(delta["metadata_changes"], ensure_ascii=False, indent=2), "```", "",
             "## Positive metric deltas", "", "| Mode | Metric | After − before |", "|---|---|---:|"]
    for mode, metrics in delta["positive_metric_delta"].items():
        lines.extend(f"| {mode} | {name} | {_number(value)} |" for name, value in metrics.items())
    lines.extend(["", "## Negative and runtime observations", ""])
    for mode in MODES:
        previous = delta["summary_before"][mode]["all"]
        current = delta["summary_after"][mode]["all"]
        lines.append(f"- {mode}: negative nonempty {previous['negative']['nonempty_rate']} → "
                     f"{current['negative']['nonempty_rate']}; negative failures "
                     f"{previous['negative']['failed_count']} → {current['negative']['failed_count']}; "
                     f"negative top-1 scores {previous['negative']['top1_scores']} → {current['negative']['top1_scores']}; "
                     f"latency ms {previous['latency_ms']} → {current['latency_ms']}.")
    lines.extend(["", "## Per-query changes", ""])
    for row in delta["per_query_changes"]:
        lines.append(f"- {row['query_id']} / {row['mode']}: improvements {row['improvements']}; "
                     f"regressions {row['regressions']}; metrics {row['metric_delta']}; "
                     f"localization delta {row['localization_delta']}; "
                     f"status {row['before']['status']} → {row['after']['status']}; "
                     f"result count {row['before']['result_count']} → {row['after']['result_count']}; "
                     f"top-1 score {row['before']['top1_score']} → {row['after']['top1_score']}.")
    lines.extend(["", *[f"- {limit}" for limit in delta["limitations"]]])
    (output / "delta.md").write_text("\n".join(lines) + "\n", encoding="utf-8")


def _load_json(path):
    return json.loads(Path(path).read_text(encoding="utf-8"))


def _load_jsonl(path):
    return [json.loads(line) for line in Path(path).read_text(encoding="utf-8").splitlines() if line.strip()]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    scoring = commands.add_parser("evaluate", help="score raw bridge JSONL with stable media_id")
    for name in ("queries", "results", "metadata", "output"):
        scoring.add_argument(f"--{name}", required=True)
    comparison = commands.add_parser("compare", help="compare standalone report.json files")
    for name in ("before", "after", "output"):
        comparison.add_argument(f"--{name}", required=True)
    args = parser.parse_args()
    try:
        if args.command == "evaluate":
            report = evaluate(_load_jsonl(args.queries), _load_jsonl(args.results), _load_json(args.metadata))
            write_report(report, args.output)
            return 0 if report["passed"] else 1
        write_comparison(compare_reports(_load_json(args.before), _load_json(args.after)), args.output)
        return 0
    except (ValueError, OSError, KeyError, TypeError) as error:
        parser.exit(2, f"{error}\n")


if __name__ == "__main__":
    raise SystemExit(main())
