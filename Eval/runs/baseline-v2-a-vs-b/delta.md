# Evaluation comparison

`baseline-v2-a` → `baseline-v2-b`

Compatible corpus/labels/scoring: **True**. Fixed-index regression: **True**. Identical retrieval conditions: **True**.

## Metadata changes

```json
{
  "run_id": {
    "before": "baseline-v2-a",
    "after": "baseline-v2-b"
  }
}
```

## Positive metric deltas

| Mode | Metric | After − before |
|---|---|---:|
| visual | video_recall@1 | 0 |
| visual | video_recall@5 | 0 |
| visual | video_recall@10 | 0 |
| visual | overlap_recall@1 | 0 |
| visual | overlap_recall@5 | 0 |
| visual | overlap_recall@10 | 0 |
| visual | tiou_0.3_recall@1 | 0 |
| visual | tiou_0.3_recall@5 | 0 |
| visual | tiou_0.3_recall@10 | 0 |
| visual | tiou_0.5_recall@1 | 0 |
| visual | tiou_0.5_recall@5 | 0 |
| visual | tiou_0.5_recall@10 | 0 |
| visual | jump_hit@1 | 0 |
| visual | jump_hit@5 | 0 |
| visual | jump_hit@10 | 0 |
| visual | overlap_mrr | 0 |
| visual | best_tiou | 0 |
| speech | video_recall@1 | 0 |
| speech | video_recall@5 | 0 |
| speech | video_recall@10 | 0 |
| speech | overlap_recall@1 | 0 |
| speech | overlap_recall@5 | 0 |
| speech | overlap_recall@10 | 0 |
| speech | tiou_0.3_recall@1 | 0 |
| speech | tiou_0.3_recall@5 | 0 |
| speech | tiou_0.3_recall@10 | 0 |
| speech | tiou_0.5_recall@1 | 0 |
| speech | tiou_0.5_recall@5 | 0 |
| speech | tiou_0.5_recall@10 | 0 |
| speech | jump_hit@1 | 0 |
| speech | jump_hit@5 | 0 |
| speech | jump_hit@10 | 0 |
| speech | overlap_mrr | 0 |
| speech | best_tiou | 0 |
| fused | video_recall@1 | 0 |
| fused | video_recall@5 | 0 |
| fused | video_recall@10 | 0 |
| fused | overlap_recall@1 | 0 |
| fused | overlap_recall@5 | 0 |
| fused | overlap_recall@10 | 0 |
| fused | tiou_0.3_recall@1 | 0 |
| fused | tiou_0.3_recall@5 | 0 |
| fused | tiou_0.3_recall@10 | 0 |
| fused | tiou_0.5_recall@1 | 0 |
| fused | tiou_0.5_recall@5 | 0 |
| fused | tiou_0.5_recall@10 | 0 |
| fused | jump_hit@1 | 0 |
| fused | jump_hit@5 | 0 |
| fused | jump_hit@10 | 0 |
| fused | overlap_mrr | 0 |
| fused | best_tiou | 0 |

## Negative and runtime observations

- visual: negative nonempty {'numerator': 5, 'denominator': 5, 'value': 1.0} → {'numerator': 5, 'denominator': 5, 'value': 1.0}; negative failures 0 → 0; negative top-1 scores {'n': 5, 'mean': 0.01639344262295082, 'min': 0.01639344262295082, 'max': 0.01639344262295082, 'p50': 0.01639344262295082, 'p95': 0.01639344262295082} → {'n': 5, 'mean': 0.01639344262295082, 'min': 0.01639344262295082, 'max': 0.01639344262295082, 'p50': 0.01639344262295082, 'p95': 0.01639344262295082}; latency ms {'n': 35, 'mean': 187.85237142857142, 'min': 122.892, 'max': 959.696, 'p50': 139.351, 'p95': 914.315} → {'n': 35, 'mean': 151.64611428571428, 'min': 126.57, 'max': 684.038, 'p50': 135.161, 'p95': 149.298}.
- speech: negative nonempty {'numerator': 5, 'denominator': 5, 'value': 1.0} → {'numerator': 5, 'denominator': 5, 'value': 1.0}; negative failures 0 → 0; negative top-1 scores {'n': 5, 'mean': 0.01639344262295082, 'min': 0.01639344262295082, 'max': 0.01639344262295082, 'p50': 0.01639344262295082, 'p95': 0.01639344262295082} → {'n': 5, 'mean': 0.01639344262295082, 'min': 0.01639344262295082, 'max': 0.01639344262295082, 'p50': 0.01639344262295082, 'p95': 0.01639344262295082}; latency ms {'n': 35, 'mean': 133.09725714285716, 'min': 123.617, 'max': 142.706, 'p50': 133.431, 'p95': 140.983} → {'n': 35, 'mean': 133.60505714285713, 'min': 119.235, 'max': 157.496, 'p50': 133.478, 'p95': 141.792}.
- fused: negative nonempty {'numerator': 5, 'denominator': 5, 'value': 1.0} → {'numerator': 5, 'denominator': 5, 'value': 1.0}; negative failures 0 → 0; negative top-1 scores {'n': 5, 'mean': 0.04215216731403337, 'min': 0.03662057044079516, 'max': 0.04507165184914545, 'p50': 0.042285619688776074, 'p95': 0.04507165184914545} → {'n': 5, 'mean': 0.04215216731403337, 'min': 0.03662057044079516, 'max': 0.04507165184914545, 'p50': 0.042285619688776074, 'p95': 0.04507165184914545}; latency ms {'n': 35, 'mean': 160.82217142857144, 'min': 133.798, 'max': 590.239, 'p50': 146.434, 'p95': 164.558} → {'n': 35, 'mean': 150.08345714285716, 'min': 133.746, 'max': 177.514, 'p50': 147.993, 'p95': 167.669}.

## Per-query changes


- Labels are synthetic script/timeline derived and have not been independently human reviewed.
- Fused uses active channels including audio; this corpus has no independent sound-channel gold labels.
- Negative nonempty rates describe returned candidates, not calibrated refusal quality. RRF scores are not probabilities.
- Fake model runs establish process behavior only, not real-model retrieval quality.
