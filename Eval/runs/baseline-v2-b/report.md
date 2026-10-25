# Retrieval evaluation

Run: `baseline-v2-b`

All query-mode responses valid: **True**

## Metadata

```json
{
  "dataset_id": "videohelper-synthetic-v1",
  "dataset_sha256": "a554a93ad073c19d48f6b2d6024d2b30726b7f06b715c218e3b1f61da92f9956",
  "labels_sha256": "ae5ea40899a795679e0eb848fff3286a333243c346dd94155811f4ed3496caa5",
  "snapshot_id": "baseline-v2",
  "index_sha256": "3a8440de6c054e4413613c3d35b018015f8516bb2a846f9d27cfcbd1bbd73f7c",
  "model_fingerprints": {
    "wemm-2b@r1": "sha256:10930e4bb624c52978574c4e88d998cd663b7a67455ed33dd34cef2e5f6731c0",
    "clap-htsat-unfused@r1": "sha256:ed71d672a7feac1f1ded9e5934ae787f8af1716bc101015d5a2b5566c5f35da3",
    "paraformer-zh@r1": "sha256:8b5293fa83545a547f846c5878a83546b103de2601f844b026bf23d325a95e8f"
  },
  "models_config_sha256": "6039b22c73f754d1a2e2fa571c0ba6a851dc6e7daa36594960d446467aee4225",
  "run_id": "baseline-v2-b",
  "code_revision": "bdbf87636e787e9c7f184b59eb4682b4cb8b18dc",
  "model_mode": "real",
  "search_config": {
    "k": 50,
    "rrf_k": 60,
    "merge_gap_cells": 1,
    "max_limit": 50
  },
  "channels": {
    "visual": [
      "visual"
    ],
    "speech": [
      "speech"
    ],
    "fused": [
      "audio",
      "speech",
      "visual"
    ]
  },
  "channel_config": {
    "visual": {
      "doc": "video",
      "seg": "grid@v1",
      "weight": 1.0,
      "active": [
        "wemm-2b@r1"
      ],
      "shadow": []
    },
    "speech": {
      "doc": "text",
      "seg": "sentence@v1",
      "weight": 1.0,
      "active": [
        "wemm-2b@r1"
      ],
      "shadow": []
    },
    "audio": {
      "doc": "audio",
      "seg": "grid@v1",
      "weight": 1.0,
      "active": [
        "clap-htsat-unfused@r1"
      ],
      "shadow": []
    }
  },
  "index_unchanged": true,
  "bridge_exit_code": 0,
  "bridge_errors": [],
  "bridge_build_provenance": {
    "sha256": "4c22be8f39255bbf4f6745d39b972e61295b9158935281e47620a8c2b53598bb",
    "build_revision": "bdbf87636e787e9c7f184b59eb4682b4cb8b18dc"
  },
  "languages": {
    "q001": "zh",
    "q002": "zh",
    "q003": "zh",
    "q004": "zh",
    "q005": "zh",
    "q006": "zh",
    "q007": "zh",
    "q008": "zh",
    "q009": "zh",
    "q010": "en",
    "q011": "zh",
    "q012": "zh",
    "q013": "en",
    "q014": "en",
    "q015": "en",
    "q016": "en",
    "q017": "en",
    "q018": "en",
    "q019": "zh",
    "q020": "zh",
    "q021": "zh",
    "q022": "zh",
    "q023": "zh",
    "q024": "zh",
    "q025": "en",
    "q026": "en",
    "q027": "en",
    "q028": "en",
    "q029": "zh",
    "q030": "en",
    "q031": "zh",
    "q032": "en",
    "q033": "zh",
    "q034": "en",
    "q035": "zh"
  },
  "timing": "sequential queries, model service preloaded; not a cold-start or load test",
  "media_languages": {
    "study_zh": "zh",
    "blockchain_mix": "zh-en",
    "story_en": "en",
    "vlog_zh": "zh",
    "landscape_en": "en"
  }
}
```

## visual / all

Queries: 35; positives: 30; negatives: 5; status: {'ok': 35}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 29 / 30 | 0.966667 |
| video_recall@5 | 29 / 30 | 0.966667 |
| video_recall@10 | 30 / 30 | 1 |
| overlap_recall@1 | 27 / 30 | 0.9 |
| overlap_recall@5 | 29 / 30 | 0.966667 |
| overlap_recall@10 | 30 / 30 | 1 |
| tiou_0.3_recall@1 | 10 / 30 | 0.333333 |
| tiou_0.3_recall@5 | 12 / 30 | 0.4 |
| tiou_0.3_recall@10 | 13 / 30 | 0.433333 |
| tiou_0.5_recall@1 | 6 / 30 | 0.2 |
| tiou_0.5_recall@5 | 7 / 30 | 0.233333 |
| tiou_0.5_recall@10 | 7 / 30 | 0.233333 |
| jump_hit@1 | 9 / 30 | 0.3 |
| jump_hit@5 | 10 / 30 | 0.333333 |
| jump_hit@10 | 11 / 30 | 0.366667 |
| overlap_mrr | 28 / 30 | 0.933333 |
| best_tiou | 9.28808 / 30 | 0.309603 |
| negative_nonempty_rate | 5 / 5 valid negatives | 1 |

Negative failures: 0; top-1 scores: `{'n': 5, 'mean': 0.01639344262295082, 'min': 0.01639344262295082, 'max': 0.01639344262295082, 'p50': 0.01639344262295082, 'p95': 0.01639344262295082}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 29, 'mean': 26706.896551724138, 'min': 1000, 'max': 101000, 'p50': 16000, 'p95': 91000}, 'end_absolute_error_ms': {'n': 29, 'mean': 28280.689655172413, 'min': 0, 'max': 85195, 'p50': 18000, 'p95': 80000}, 'tiou': {'n': 29, 'mean': 0.2715870439828945, 'min': 0.0, 'max': 0.9333333333333333, 'p50': 0.1850952380952381, 'p95': 0.5735}}`.

Latency ms: `{'n': 35, 'mean': 151.64611428571428, 'min': 126.57, 'max': 684.038, 'p50': 135.161, 'p95': 149.298}`.

## visual / mixed

Queries: 10; positives: 10; negatives: 0; status: {'ok': 10}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 10 / 10 | 1 |
| video_recall@5 | 10 / 10 | 1 |
| video_recall@10 | 10 / 10 | 1 |
| overlap_recall@1 | 10 / 10 | 1 |
| overlap_recall@5 | 10 / 10 | 1 |
| overlap_recall@10 | 10 / 10 | 1 |
| tiou_0.3_recall@1 | 3 / 10 | 0.3 |
| tiou_0.3_recall@5 | 3 / 10 | 0.3 |
| tiou_0.3_recall@10 | 3 / 10 | 0.3 |
| tiou_0.5_recall@1 | 1 / 10 | 0.1 |
| tiou_0.5_recall@5 | 1 / 10 | 0.1 |
| tiou_0.5_recall@10 | 1 / 10 | 0.1 |
| jump_hit@1 | 4 / 10 | 0.4 |
| jump_hit@5 | 4 / 10 | 0.4 |
| jump_hit@10 | 4 / 10 | 0.4 |
| overlap_mrr | 10 / 10 | 1 |
| best_tiou | 2.36199 / 10 | 0.236199 |
| negative_nonempty_rate | 0 / 0 valid negatives | n/a |

Negative failures: 0; top-1 scores: `{'n': 0, 'mean': None, 'min': None, 'max': None, 'p50': None, 'p95': None}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 10, 'mean': 33050.0, 'min': 1000, 'max': 101000, 'p50': 23000, 'p95': 101000}, 'end_absolute_error_ms': {'n': 10, 'mean': 23340.7, 'min': 5021, 'max': 67872, 'p50': 8082, 'p95': 67872}, 'tiou': {'n': 10, 'mean': 0.23619937135220642, 'min': 0.06497222222222222, 'max': 0.5699285714285715, 'p50': 0.12456666666666667, 'p95': 0.5699285714285715}}`.

Latency ms: `{'n': 10, 'mean': 137.1239, 'min': 133.813, 'max': 140.634, 'p50': 135.205, 'p95': 140.634}`.

## visual / negative

Queries: 5; positives: 0; negatives: 5; status: {'ok': 5}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 0 / 0 | n/a |
| video_recall@5 | 0 / 0 | n/a |
| video_recall@10 | 0 / 0 | n/a |
| overlap_recall@1 | 0 / 0 | n/a |
| overlap_recall@5 | 0 / 0 | n/a |
| overlap_recall@10 | 0 / 0 | n/a |
| tiou_0.3_recall@1 | 0 / 0 | n/a |
| tiou_0.3_recall@5 | 0 / 0 | n/a |
| tiou_0.3_recall@10 | 0 / 0 | n/a |
| tiou_0.5_recall@1 | 0 / 0 | n/a |
| tiou_0.5_recall@5 | 0 / 0 | n/a |
| tiou_0.5_recall@10 | 0 / 0 | n/a |
| jump_hit@1 | 0 / 0 | n/a |
| jump_hit@5 | 0 / 0 | n/a |
| jump_hit@10 | 0 / 0 | n/a |
| overlap_mrr | 0 / 0 | n/a |
| best_tiou | 0 / 0 | n/a |
| negative_nonempty_rate | 5 / 5 valid negatives | 1 |

Negative failures: 0; top-1 scores: `{'n': 5, 'mean': 0.01639344262295082, 'min': 0.01639344262295082, 'max': 0.01639344262295082, 'p50': 0.01639344262295082, 'p95': 0.01639344262295082}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 0, 'mean': None, 'min': None, 'max': None, 'p50': None, 'p95': None}, 'end_absolute_error_ms': {'n': 0, 'mean': None, 'min': None, 'max': None, 'p50': None, 'p95': None}, 'tiou': {'n': 0, 'mean': None, 'min': None, 'max': None, 'p50': None, 'p95': None}}`.

Latency ms: `{'n': 5, 'mean': 133.554, 'min': 126.57, 'max': 138.136, 'p50': 134.68, 'p95': 138.136}`.

## visual / speech

Queries: 10; positives: 10; negatives: 0; status: {'ok': 10}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 9 / 10 | 0.9 |
| video_recall@5 | 9 / 10 | 0.9 |
| video_recall@10 | 10 / 10 | 1 |
| overlap_recall@1 | 7 / 10 | 0.7 |
| overlap_recall@5 | 9 / 10 | 0.9 |
| overlap_recall@10 | 10 / 10 | 1 |
| tiou_0.3_recall@1 | 1 / 10 | 0.1 |
| tiou_0.3_recall@5 | 3 / 10 | 0.3 |
| tiou_0.3_recall@10 | 4 / 10 | 0.4 |
| tiou_0.5_recall@1 | 1 / 10 | 0.1 |
| tiou_0.5_recall@5 | 2 / 10 | 0.2 |
| tiou_0.5_recall@10 | 2 / 10 | 0.2 |
| jump_hit@1 | 5 / 10 | 0.5 |
| jump_hit@5 | 6 / 10 | 0.6 |
| jump_hit@10 | 7 / 10 | 0.7 |
| overlap_mrr | 8 / 10 | 0.8 |
| best_tiou | 2.91272 / 10 | 0.291272 |
| negative_nonempty_rate | 0 / 0 valid negatives | n/a |

Negative failures: 0; top-1 scores: `{'n': 0, 'mean': None, 'min': None, 'max': None, 'p50': None, 'p95': None}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 9, 'mean': 22388.88888888889, 'min': 1000, 'max': 61000, 'p50': 1000, 'p95': 61000}, 'end_absolute_error_ms': {'n': 9, 'mean': 42248.11111111111, 'min': 4971, 'max': 85195, 'p50': 39760, 'p95': 85195}, 'tiou': {'n': 9, 'mean': 0.16674113756613756, 'min': 0.0, 'max': 0.5735, 'p50': 0.1174, 'p95': 0.5735}}`.

Latency ms: `{'n': 10, 'mean': 190.6057, 'min': 130.433, 'max': 684.038, 'p50': 132.843, 'p95': 684.038}`.

## visual / visual

Queries: 10; positives: 10; negatives: 0; status: {'ok': 10}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 10 / 10 | 1 |
| video_recall@5 | 10 / 10 | 1 |
| video_recall@10 | 10 / 10 | 1 |
| overlap_recall@1 | 10 / 10 | 1 |
| overlap_recall@5 | 10 / 10 | 1 |
| overlap_recall@10 | 10 / 10 | 1 |
| tiou_0.3_recall@1 | 6 / 10 | 0.6 |
| tiou_0.3_recall@5 | 6 / 10 | 0.6 |
| tiou_0.3_recall@10 | 6 / 10 | 0.6 |
| tiou_0.5_recall@1 | 4 / 10 | 0.4 |
| tiou_0.5_recall@5 | 4 / 10 | 0.4 |
| tiou_0.5_recall@10 | 4 / 10 | 0.4 |
| jump_hit@1 | 0 / 10 | 0 |
| jump_hit@5 | 0 / 10 | 0 |
| jump_hit@10 | 0 / 10 | 0 |
| overlap_mrr | 10 / 10 | 1 |
| best_tiou | 4.01336 / 10 | 0.401336 |
| negative_nonempty_rate | 0 / 0 valid negatives | n/a |

Negative failures: 0; top-1 scores: `{'n': 0, 'mean': None, 'min': None, 'max': None, 'p50': None, 'p95': None}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 10, 'mean': 24250.0, 'min': 1000, 'max': 72000, 'p50': 15000, 'p95': 72000}, 'end_absolute_error_ms': {'n': 10, 'mean': 20650.0, 'min': 0, 'max': 80000, 'p50': 500, 'p95': 80000}, 'tiou': {'n': 10, 'mean': 0.401336032388664, 'min': 0.16666666666666666, 'max': 0.9333333333333333, 'p50': 0.42105263157894735, 'p95': 0.9333333333333333}}`.

Latency ms: `{'n': 10, 'mean': 136.2548, 'min': 130.834, 'max': 142.632, 'p50': 135.818, 'p95': 142.632}`.

## visual / language: en

Queries: 14; positives: 12; negatives: 2; status: {'ok': 14}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 11 / 12 | 0.916667 |
| video_recall@5 | 11 / 12 | 0.916667 |
| video_recall@10 | 12 / 12 | 1 |
| overlap_recall@1 | 11 / 12 | 0.916667 |
| overlap_recall@5 | 11 / 12 | 0.916667 |
| overlap_recall@10 | 12 / 12 | 1 |
| tiou_0.3_recall@1 | 2 / 12 | 0.166667 |
| tiou_0.3_recall@5 | 2 / 12 | 0.166667 |
| tiou_0.3_recall@10 | 3 / 12 | 0.25 |
| tiou_0.5_recall@1 | 1 / 12 | 0.0833333 |
| tiou_0.5_recall@5 | 1 / 12 | 0.0833333 |
| tiou_0.5_recall@10 | 1 / 12 | 0.0833333 |
| jump_hit@1 | 4 / 12 | 0.333333 |
| jump_hit@5 | 4 / 12 | 0.333333 |
| jump_hit@10 | 5 / 12 | 0.416667 |
| overlap_mrr | 11.1667 / 12 | 0.930556 |
| best_tiou | 2.55298 / 12 | 0.212748 |
| negative_nonempty_rate | 2 / 2 valid negatives | 1 |

Negative failures: 0; top-1 scores: `{'n': 2, 'mean': 0.01639344262295082, 'min': 0.01639344262295082, 'max': 0.01639344262295082, 'p50': 0.01639344262295082, 'p95': 0.01639344262295082}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 11, 'mean': 32090.909090909092, 'min': 1000, 'max': 91000, 'p50': 18000, 'p95': 91000}, 'end_absolute_error_ms': {'n': 11, 'mean': 32807.181818181816, 'min': 0, 'max': 72000, 'p50': 22026, 'p95': 72000}, 'tiou': {'n': 11, 'mean': 0.19756473063973062, 'min': 0.06275, 'max': 0.5, 'p50': 0.16666666666666666, 'p95': 0.5}}`.

Latency ms: `{'n': 14, 'mean': 135.34814285714285, 'min': 131.755, 'max': 140.564, 'p50': 134.68, 'p95': 140.564}`.

## visual / language: zh

Queries: 21; positives: 18; negatives: 3; status: {'ok': 21}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 18 / 18 | 1 |
| video_recall@5 | 18 / 18 | 1 |
| video_recall@10 | 18 / 18 | 1 |
| overlap_recall@1 | 16 / 18 | 0.888889 |
| overlap_recall@5 | 18 / 18 | 1 |
| overlap_recall@10 | 18 / 18 | 1 |
| tiou_0.3_recall@1 | 8 / 18 | 0.444444 |
| tiou_0.3_recall@5 | 10 / 18 | 0.555556 |
| tiou_0.3_recall@10 | 10 / 18 | 0.555556 |
| tiou_0.5_recall@1 | 5 / 18 | 0.277778 |
| tiou_0.5_recall@5 | 6 / 18 | 0.333333 |
| tiou_0.5_recall@10 | 6 / 18 | 0.333333 |
| jump_hit@1 | 5 / 18 | 0.277778 |
| jump_hit@5 | 6 / 18 | 0.333333 |
| jump_hit@10 | 6 / 18 | 0.333333 |
| overlap_mrr | 16.8333 / 18 | 0.935185 |
| best_tiou | 6.7351 / 18 | 0.374172 |
| negative_nonempty_rate | 3 / 3 valid negatives | 1 |

Negative failures: 0; top-1 scores: `{'n': 3, 'mean': 0.01639344262295082, 'min': 0.01639344262295082, 'max': 0.01639344262295082, 'p50': 0.01639344262295082, 'p95': 0.01639344262295082}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 18, 'mean': 23416.666666666668, 'min': 1000, 'max': 101000, 'p50': 15000, 'p95': 101000}, 'end_absolute_error_ms': {'n': 18, 'mean': 25514.5, 'min': 0, 'max': 85195, 'p50': 8082, 'p95': 85195}, 'tiou': {'n': 18, 'mean': 0.3168229021370503, 'min': 0.0, 'max': 0.9333333333333333, 'p50': 0.19158762886597938, 'p95': 0.9333333333333333}}`.

Latency ms: `{'n': 21, 'mean': 162.51142857142855, 'min': 126.57, 'max': 684.038, 'p50': 138.136, 'p95': 149.298}`.

## speech / all

Queries: 35; positives: 30; negatives: 5; status: {'ok': 35}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 28 / 30 | 0.933333 |
| video_recall@5 | 30 / 30 | 1 |
| video_recall@10 | 30 / 30 | 1 |
| overlap_recall@1 | 26 / 30 | 0.866667 |
| overlap_recall@5 | 29 / 30 | 0.966667 |
| overlap_recall@10 | 30 / 30 | 1 |
| tiou_0.3_recall@1 | 14 / 30 | 0.466667 |
| tiou_0.3_recall@5 | 16 / 30 | 0.533333 |
| tiou_0.3_recall@10 | 17 / 30 | 0.566667 |
| tiou_0.5_recall@1 | 5 / 30 | 0.166667 |
| tiou_0.5_recall@5 | 6 / 30 | 0.2 |
| tiou_0.5_recall@10 | 7 / 30 | 0.233333 |
| jump_hit@1 | 8 / 30 | 0.266667 |
| jump_hit@5 | 8 / 30 | 0.266667 |
| jump_hit@10 | 8 / 30 | 0.266667 |
| overlap_mrr | 27.6667 / 30 | 0.922222 |
| best_tiou | 10.5077 / 30 | 0.350256 |
| negative_nonempty_rate | 5 / 5 valid negatives | 1 |

Negative failures: 0; top-1 scores: `{'n': 5, 'mean': 0.01639344262295082, 'min': 0.01639344262295082, 'max': 0.01639344262295082, 'p50': 0.01639344262295082, 'p95': 0.01639344262295082}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 28, 'mean': 21892.85714285714, 'min': 0, 'max': 91000, 'p50': 15000, 'p95': 72000}, 'end_absolute_error_ms': {'n': 28, 'mean': 21876.321428571428, 'min': 240, 'max': 99189, 'p50': 15000, 'p95': 64000}, 'tiou': {'n': 28, 'mean': 0.304355488400114, 'min': 0.0, 'max': 0.84528, 'p50': 0.21863529411764707, 'p95': 0.845}}`.

Latency ms: `{'n': 35, 'mean': 133.60505714285713, 'min': 119.235, 'max': 157.496, 'p50': 133.478, 'p95': 141.792}`.

## speech / mixed

Queries: 10; positives: 10; negatives: 0; status: {'ok': 10}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 10 / 10 | 1 |
| video_recall@5 | 10 / 10 | 1 |
| video_recall@10 | 10 / 10 | 1 |
| overlap_recall@1 | 10 / 10 | 1 |
| overlap_recall@5 | 10 / 10 | 1 |
| overlap_recall@10 | 10 / 10 | 1 |
| tiou_0.3_recall@1 | 6 / 10 | 0.6 |
| tiou_0.3_recall@5 | 6 / 10 | 0.6 |
| tiou_0.3_recall@10 | 6 / 10 | 0.6 |
| tiou_0.5_recall@1 | 2 / 10 | 0.2 |
| tiou_0.5_recall@5 | 2 / 10 | 0.2 |
| tiou_0.5_recall@10 | 2 / 10 | 0.2 |
| jump_hit@1 | 5 / 10 | 0.5 |
| jump_hit@5 | 5 / 10 | 0.5 |
| jump_hit@10 | 5 / 10 | 0.5 |
| overlap_mrr | 10 / 10 | 1 |
| best_tiou | 3.29443 / 10 | 0.329443 |
| negative_nonempty_rate | 0 / 0 valid negatives | n/a |

Negative failures: 0; top-1 scores: `{'n': 0, 'mean': None, 'min': None, 'max': None, 'p50': None, 'p95': None}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 10, 'mean': 24350.0, 'min': 1000, 'max': 91000, 'p50': 18000, 'p95': 91000}, 'end_absolute_error_ms': {'n': 10, 'mean': 10690.7, 'min': 240, 'max': 38483, 'p50': 3082, 'p95': 38483}, 'tiou': {'n': 10, 'mean': 0.32944278315801845, 'min': 0.07423, 'max': 0.845, 'p50': 0.30688461538461537, 'p95': 0.845}}`.

Latency ms: `{'n': 10, 'mean': 137.0284, 'min': 130.232, 'max': 157.496, 'p50': 133.882, 'p95': 157.496}`.

## speech / negative

Queries: 5; positives: 0; negatives: 5; status: {'ok': 5}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 0 / 0 | n/a |
| video_recall@5 | 0 / 0 | n/a |
| video_recall@10 | 0 / 0 | n/a |
| overlap_recall@1 | 0 / 0 | n/a |
| overlap_recall@5 | 0 / 0 | n/a |
| overlap_recall@10 | 0 / 0 | n/a |
| tiou_0.3_recall@1 | 0 / 0 | n/a |
| tiou_0.3_recall@5 | 0 / 0 | n/a |
| tiou_0.3_recall@10 | 0 / 0 | n/a |
| tiou_0.5_recall@1 | 0 / 0 | n/a |
| tiou_0.5_recall@5 | 0 / 0 | n/a |
| tiou_0.5_recall@10 | 0 / 0 | n/a |
| jump_hit@1 | 0 / 0 | n/a |
| jump_hit@5 | 0 / 0 | n/a |
| jump_hit@10 | 0 / 0 | n/a |
| overlap_mrr | 0 / 0 | n/a |
| best_tiou | 0 / 0 | n/a |
| negative_nonempty_rate | 5 / 5 valid negatives | 1 |

Negative failures: 0; top-1 scores: `{'n': 5, 'mean': 0.01639344262295082, 'min': 0.01639344262295082, 'max': 0.01639344262295082, 'p50': 0.01639344262295082, 'p95': 0.01639344262295082}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 0, 'mean': None, 'min': None, 'max': None, 'p50': None, 'p95': None}, 'end_absolute_error_ms': {'n': 0, 'mean': None, 'min': None, 'max': None, 'p50': None, 'p95': None}, 'tiou': {'n': 0, 'mean': None, 'min': None, 'max': None, 'p50': None, 'p95': None}}`.

Latency ms: `{'n': 5, 'mean': 126.92360000000001, 'min': 119.235, 'max': 131.716, 'p50': 128.222, 'p95': 131.716}`.

## speech / speech

Queries: 10; positives: 10; negatives: 0; status: {'ok': 10}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 10 / 10 | 1 |
| video_recall@5 | 10 / 10 | 1 |
| video_recall@10 | 10 / 10 | 1 |
| overlap_recall@1 | 9 / 10 | 0.9 |
| overlap_recall@5 | 9 / 10 | 0.9 |
| overlap_recall@10 | 10 / 10 | 1 |
| tiou_0.3_recall@1 | 5 / 10 | 0.5 |
| tiou_0.3_recall@5 | 5 / 10 | 0.5 |
| tiou_0.3_recall@10 | 6 / 10 | 0.6 |
| tiou_0.5_recall@1 | 2 / 10 | 0.2 |
| tiou_0.5_recall@5 | 2 / 10 | 0.2 |
| tiou_0.5_recall@10 | 3 / 10 | 0.3 |
| jump_hit@1 | 3 / 10 | 0.3 |
| jump_hit@5 | 3 / 10 | 0.3 |
| jump_hit@10 | 3 / 10 | 0.3 |
| overlap_mrr | 9.16667 / 10 | 0.916667 |
| best_tiou | 3.75112 / 10 | 0.375112 |
| negative_nonempty_rate | 0 / 0 valid negatives | n/a |

Negative failures: 0; top-1 scores: `{'n': 0, 'mean': None, 'min': None, 'max': None, 'p50': None, 'p95': None}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 10, 'mean': 15200.0, 'min': 0, 'max': 55000, 'p50': 4000, 'p95': 55000}, 'end_absolute_error_ms': {'n': 10, 'mean': 28363.0, 'min': 934, 'max': 99189, 'p50': 16695, 'p95': 99189}, 'tiou': {'n': 10, 'mean': 0.30200225392254804, 'min': 0.0, 'max': 0.84528, 'p50': 0.1652, 'p95': 0.84528}}`.

Latency ms: `{'n': 10, 'mean': 132.4567, 'min': 123.016, 'max': 141.792, 'p50': 132.711, 'p95': 141.792}`.

## speech / visual

Queries: 10; positives: 10; negatives: 0; status: {'ok': 10}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 8 / 10 | 0.8 |
| video_recall@5 | 10 / 10 | 1 |
| video_recall@10 | 10 / 10 | 1 |
| overlap_recall@1 | 7 / 10 | 0.7 |
| overlap_recall@5 | 10 / 10 | 1 |
| overlap_recall@10 | 10 / 10 | 1 |
| tiou_0.3_recall@1 | 3 / 10 | 0.3 |
| tiou_0.3_recall@5 | 5 / 10 | 0.5 |
| tiou_0.3_recall@10 | 5 / 10 | 0.5 |
| tiou_0.5_recall@1 | 1 / 10 | 0.1 |
| tiou_0.5_recall@5 | 2 / 10 | 0.2 |
| tiou_0.5_recall@10 | 2 / 10 | 0.2 |
| jump_hit@1 | 0 / 10 | 0 |
| jump_hit@5 | 0 / 10 | 0 |
| jump_hit@10 | 0 / 10 | 0 |
| overlap_mrr | 8.5 / 10 | 0.85 |
| best_tiou | 3.46213 / 10 | 0.346213 |
| negative_nonempty_rate | 0 / 0 valid negatives | n/a |

Negative failures: 0; top-1 scores: `{'n': 0, 'mean': None, 'min': None, 'max': None, 'p50': None, 'p95': None}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 8, 'mean': 27187.5, 'min': 0, 'max': 72000, 'p50': 15000, 'p95': 72000}, 'end_absolute_error_ms': {'n': 8, 'mean': 27750.0, 'min': 5000, 'max': 64000, 'p50': 15000, 'p95': 64000}, 'tiou': {'n': 8, 'mean': 0.275937913049691, 'min': 0.0, 'max': 0.5714285714285714, 'p50': 0.18, 'p95': 0.5714285714285714}}`.

Latency ms: `{'n': 10, 'mean': 134.6708, 'min': 131.946, 'max': 138.739, 'p50': 133.889, 'p95': 138.739}`.

## speech / language: en

Queries: 14; positives: 12; negatives: 2; status: {'ok': 14}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 12 / 12 | 1 |
| video_recall@5 | 12 / 12 | 1 |
| video_recall@10 | 12 / 12 | 1 |
| overlap_recall@1 | 11 / 12 | 0.916667 |
| overlap_recall@5 | 11 / 12 | 0.916667 |
| overlap_recall@10 | 12 / 12 | 1 |
| tiou_0.3_recall@1 | 2 / 12 | 0.166667 |
| tiou_0.3_recall@5 | 2 / 12 | 0.166667 |
| tiou_0.3_recall@10 | 3 / 12 | 0.25 |
| tiou_0.5_recall@1 | 1 / 12 | 0.0833333 |
| tiou_0.5_recall@5 | 1 / 12 | 0.0833333 |
| tiou_0.5_recall@10 | 2 / 12 | 0.166667 |
| jump_hit@1 | 3 / 12 | 0.25 |
| jump_hit@5 | 3 / 12 | 0.25 |
| jump_hit@10 | 3 / 12 | 0.25 |
| overlap_mrr | 11.1667 / 12 | 0.930556 |
| best_tiou | 2.82237 / 12 | 0.235198 |
| negative_nonempty_rate | 2 / 2 valid negatives | 1 |

Negative failures: 0; top-1 scores: `{'n': 2, 'mean': 0.01639344262295082, 'min': 0.01639344262295082, 'max': 0.01639344262295082, 'p50': 0.01639344262295082, 'p95': 0.01639344262295082}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 12, 'mean': 33916.666666666664, 'min': 1000, 'max': 91000, 'p50': 21000, 'p95': 91000}, 'end_absolute_error_ms': {'n': 12, 'mean': 34441.333333333336, 'min': 1209, 'max': 99189, 'p50': 31526, 'p95': 99189}, 'tiou': {'n': 12, 'mean': 0.1742725942661989, 'min': 0.0, 'max': 0.6854, 'p50': 0.09962857142857143, 'p95': 0.6854}}`.

Latency ms: `{'n': 14, 'mean': 131.83371428571428, 'min': 123.016, 'max': 136.454, 'p50': 131.878, 'p95': 136.454}`.

## speech / language: zh

Queries: 21; positives: 18; negatives: 3; status: {'ok': 21}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 16 / 18 | 0.888889 |
| video_recall@5 | 18 / 18 | 1 |
| video_recall@10 | 18 / 18 | 1 |
| overlap_recall@1 | 15 / 18 | 0.833333 |
| overlap_recall@5 | 18 / 18 | 1 |
| overlap_recall@10 | 18 / 18 | 1 |
| tiou_0.3_recall@1 | 12 / 18 | 0.666667 |
| tiou_0.3_recall@5 | 14 / 18 | 0.777778 |
| tiou_0.3_recall@10 | 14 / 18 | 0.777778 |
| tiou_0.5_recall@1 | 4 / 18 | 0.222222 |
| tiou_0.5_recall@5 | 5 / 18 | 0.277778 |
| tiou_0.5_recall@10 | 5 / 18 | 0.277778 |
| jump_hit@1 | 5 / 18 | 0.277778 |
| jump_hit@5 | 5 / 18 | 0.277778 |
| jump_hit@10 | 5 / 18 | 0.277778 |
| overlap_mrr | 16.5 / 18 | 0.916667 |
| best_tiou | 7.68531 / 18 | 0.426962 |
| negative_nonempty_rate | 3 / 3 valid negatives | 1 |

Negative failures: 0; top-1 scores: `{'n': 3, 'mean': 0.01639344262295082, 'min': 0.01639344262295082, 'max': 0.01639344262295082, 'p50': 0.01639344262295082, 'p95': 0.01639344262295082}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 16, 'mean': 12875.0, 'min': 0, 'max': 60000, 'p50': 4000, 'p95': 60000}, 'end_absolute_error_ms': {'n': 16, 'mean': 12452.5625, 'min': 240, 'max': 55000, 'p50': 5000, 'p95': 55000}, 'tiou': {'n': 16, 'mean': 0.4019176590005504, 'min': 0.0, 'max': 0.84528, 'p50': 0.3424, 'p95': 0.84528}}`.

Latency ms: `{'n': 21, 'mean': 134.78595238095238, 'min': 119.235, 'max': 157.496, 'p50': 135.093, 'p95': 141.792}`.

## fused / all

Queries: 35; positives: 30; negatives: 5; status: {'ok': 35}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 26 / 30 | 0.866667 |
| video_recall@5 | 30 / 30 | 1 |
| video_recall@10 | 30 / 30 | 1 |
| overlap_recall@1 | 24 / 30 | 0.8 |
| overlap_recall@5 | 30 / 30 | 1 |
| overlap_recall@10 | 30 / 30 | 1 |
| tiou_0.3_recall@1 | 4 / 30 | 0.133333 |
| tiou_0.3_recall@5 | 5 / 30 | 0.166667 |
| tiou_0.3_recall@10 | 5 / 30 | 0.166667 |
| tiou_0.5_recall@1 | 0 / 30 | 0 |
| tiou_0.5_recall@5 | 0 / 30 | 0 |
| tiou_0.5_recall@10 | 0 / 30 | 0 |
| jump_hit@1 | 5 / 30 | 0.166667 |
| jump_hit@5 | 8 / 30 | 0.266667 |
| jump_hit@10 | 8 / 30 | 0.266667 |
| overlap_mrr | 26.8333 / 30 | 0.894444 |
| best_tiou | 5.37453 / 30 | 0.179151 |
| negative_nonempty_rate | 5 / 5 valid negatives | 1 |

Negative failures: 0; top-1 scores: `{'n': 5, 'mean': 0.04215216731403337, 'min': 0.03662057044079516, 'max': 0.04507165184914545, 'p50': 0.042285619688776074, 'p95': 0.04507165184914545}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 26, 'mean': 29846.153846153848, 'min': 1000, 'max': 101000, 'p50': 20000, 'p95': 80000}, 'end_absolute_error_ms': {'n': 26, 'mean': 39321.346153846156, 'min': 5021, 'max': 108434, 'p50': 32026, 'p95': 99689}, 'tiou': {'n': 26, 'mean': 0.16200289879149932, 'min': 0.0, 'max': 0.45693333333333336, 'p50': 0.1521391304347826, 'p95': 0.35654545454545455}}`.

Latency ms: `{'n': 35, 'mean': 150.08345714285716, 'min': 133.746, 'max': 177.514, 'p50': 147.993, 'p95': 167.669}`.

## fused / mixed

Queries: 10; positives: 10; negatives: 0; status: {'ok': 10}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 8 / 10 | 0.8 |
| video_recall@5 | 10 / 10 | 1 |
| video_recall@10 | 10 / 10 | 1 |
| overlap_recall@1 | 7 / 10 | 0.7 |
| overlap_recall@5 | 10 / 10 | 1 |
| overlap_recall@10 | 10 / 10 | 1 |
| tiou_0.3_recall@1 | 1 / 10 | 0.1 |
| tiou_0.3_recall@5 | 1 / 10 | 0.1 |
| tiou_0.3_recall@10 | 1 / 10 | 0.1 |
| tiou_0.5_recall@1 | 0 / 10 | 0 |
| tiou_0.5_recall@5 | 0 / 10 | 0 |
| tiou_0.5_recall@10 | 0 / 10 | 0 |
| jump_hit@1 | 3 / 10 | 0.3 |
| jump_hit@5 | 5 / 10 | 0.5 |
| jump_hit@10 | 5 / 10 | 0.5 |
| overlap_mrr | 8.33333 / 10 | 0.833333 |
| best_tiou | 1.73807 / 10 | 0.173807 |
| negative_nonempty_rate | 0 / 0 valid negatives | n/a |

Negative failures: 0; top-1 scores: `{'n': 0, 'mean': None, 'min': None, 'max': None, 'p50': None, 'p95': None}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 8, 'mean': 35875.0, 'min': 1000, 'max': 101000, 'p50': 31000, 'p95': 101000}, 'end_absolute_error_ms': {'n': 8, 'mean': 29392.75, 'min': 5021, 'max': 67872, 'p50': 15752, 'p95': 67872}, 'tiou': {'n': 8, 'mean': 0.154540539452496, 'min': 0.0, 'max': 0.45693333333333336, 'p50': 0.09098333333333333, 'p95': 0.45693333333333336}}`.

Latency ms: `{'n': 10, 'mean': 155.0302, 'min': 135.3, 'max': 177.514, 'p50': 151.692, 'p95': 177.514}`.

## fused / negative

Queries: 5; positives: 0; negatives: 5; status: {'ok': 5}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 0 / 0 | n/a |
| video_recall@5 | 0 / 0 | n/a |
| video_recall@10 | 0 / 0 | n/a |
| overlap_recall@1 | 0 / 0 | n/a |
| overlap_recall@5 | 0 / 0 | n/a |
| overlap_recall@10 | 0 / 0 | n/a |
| tiou_0.3_recall@1 | 0 / 0 | n/a |
| tiou_0.3_recall@5 | 0 / 0 | n/a |
| tiou_0.3_recall@10 | 0 / 0 | n/a |
| tiou_0.5_recall@1 | 0 / 0 | n/a |
| tiou_0.5_recall@5 | 0 / 0 | n/a |
| tiou_0.5_recall@10 | 0 / 0 | n/a |
| jump_hit@1 | 0 / 0 | n/a |
| jump_hit@5 | 0 / 0 | n/a |
| jump_hit@10 | 0 / 0 | n/a |
| overlap_mrr | 0 / 0 | n/a |
| best_tiou | 0 / 0 | n/a |
| negative_nonempty_rate | 5 / 5 valid negatives | 1 |

Negative failures: 0; top-1 scores: `{'n': 5, 'mean': 0.04215216731403337, 'min': 0.03662057044079516, 'max': 0.04507165184914545, 'p50': 0.042285619688776074, 'p95': 0.04507165184914545}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 0, 'mean': None, 'min': None, 'max': None, 'p50': None, 'p95': None}, 'end_absolute_error_ms': {'n': 0, 'mean': None, 'min': None, 'max': None, 'p50': None, 'p95': None}, 'tiou': {'n': 0, 'mean': None, 'min': None, 'max': None, 'p50': None, 'p95': None}}`.

Latency ms: `{'n': 5, 'mean': 135.906, 'min': 134.147, 'max': 138.782, 'p50': 135.503, 'p95': 138.782}`.

## fused / speech

Queries: 10; positives: 10; negatives: 0; status: {'ok': 10}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 9 / 10 | 0.9 |
| video_recall@5 | 10 / 10 | 1 |
| video_recall@10 | 10 / 10 | 1 |
| overlap_recall@1 | 9 / 10 | 0.9 |
| overlap_recall@5 | 10 / 10 | 1 |
| overlap_recall@10 | 10 / 10 | 1 |
| tiou_0.3_recall@1 | 1 / 10 | 0.1 |
| tiou_0.3_recall@5 | 1 / 10 | 0.1 |
| tiou_0.3_recall@10 | 1 / 10 | 0.1 |
| tiou_0.5_recall@1 | 0 / 10 | 0 |
| tiou_0.5_recall@5 | 0 / 10 | 0 |
| tiou_0.5_recall@10 | 0 / 10 | 0 |
| jump_hit@1 | 2 / 10 | 0.2 |
| jump_hit@5 | 3 / 10 | 0.3 |
| jump_hit@10 | 3 / 10 | 0.3 |
| overlap_mrr | 9.5 / 10 | 0.95 |
| best_tiou | 1.33407 / 10 | 0.133407 |
| negative_nonempty_rate | 0 / 0 valid negatives | n/a |

Negative failures: 0; top-1 scores: `{'n': 0, 'mean': None, 'min': None, 'max': None, 'p50': None, 'p95': None}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 9, 'mean': 19333.333333333332, 'min': 1000, 'max': 61000, 'p50': 4000, 'p95': 61000}, 'end_absolute_error_ms': {'n': 9, 'mean': 51634.77777777778, 'min': 16695, 'max': 108434, 'p50': 43226, 'p95': 108434}, 'tiou': {'n': 9, 'mean': 0.13778028487489272, 'min': 0.06275, 'max': 0.35654545454545455, 'p50': 0.08805, 'p95': 0.35654545454545455}}`.

Latency ms: `{'n': 10, 'mean': 150.1992, 'min': 133.746, 'max': 166.459, 'p50': 145.74, 'p95': 166.459}`.

## fused / visual

Queries: 10; positives: 10; negatives: 0; status: {'ok': 10}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 9 / 10 | 0.9 |
| video_recall@5 | 10 / 10 | 1 |
| video_recall@10 | 10 / 10 | 1 |
| overlap_recall@1 | 8 / 10 | 0.8 |
| overlap_recall@5 | 10 / 10 | 1 |
| overlap_recall@10 | 10 / 10 | 1 |
| tiou_0.3_recall@1 | 2 / 10 | 0.2 |
| tiou_0.3_recall@5 | 3 / 10 | 0.3 |
| tiou_0.3_recall@10 | 3 / 10 | 0.3 |
| tiou_0.5_recall@1 | 0 / 10 | 0 |
| tiou_0.5_recall@5 | 0 / 10 | 0 |
| tiou_0.5_recall@10 | 0 / 10 | 0 |
| jump_hit@1 | 0 / 10 | 0 |
| jump_hit@5 | 0 / 10 | 0 |
| jump_hit@10 | 0 / 10 | 0 |
| overlap_mrr | 9 / 10 | 0.9 |
| best_tiou | 2.3024 / 10 | 0.23024 |
| negative_nonempty_rate | 0 / 0 valid negatives | n/a |

Negative failures: 0; top-1 scores: `{'n': 0, 'mean': None, 'min': None, 'max': None, 'p50': None, 'p95': None}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 9, 'mean': 35000.0, 'min': 14000, 'max': 80000, 'p50': 20000, 'p95': 80000}, 'end_absolute_error_ms': {'n': 9, 'mean': 35833.333333333336, 'min': 10000, 'max': 80000, 'p50': 20000, 'p95': 80000}, 'tiou': {'n': 9, 'mean': 0.19285872100944215, 'min': 0.0, 'max': 0.34146341463414637, 'p50': 0.16666666666666666, 'p95': 0.34146341463414637}}`.

Latency ms: `{'n': 10, 'mean': 152.1097, 'min': 144.34, 'max': 163.385, 'p50': 147.993, 'p95': 163.385}`.

## fused / language: en

Queries: 14; positives: 12; negatives: 2; status: {'ok': 14}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 9 / 12 | 0.75 |
| video_recall@5 | 12 / 12 | 1 |
| video_recall@10 | 12 / 12 | 1 |
| overlap_recall@1 | 9 / 12 | 0.75 |
| overlap_recall@5 | 12 / 12 | 1 |
| overlap_recall@10 | 12 / 12 | 1 |
| tiou_0.3_recall@1 | 1 / 12 | 0.0833333 |
| tiou_0.3_recall@5 | 1 / 12 | 0.0833333 |
| tiou_0.3_recall@10 | 1 / 12 | 0.0833333 |
| tiou_0.5_recall@1 | 0 / 12 | 0 |
| tiou_0.5_recall@5 | 0 / 12 | 0 |
| tiou_0.5_recall@10 | 0 / 12 | 0 |
| jump_hit@1 | 1 / 12 | 0.0833333 |
| jump_hit@5 | 3 / 12 | 0.25 |
| jump_hit@10 | 3 / 12 | 0.25 |
| overlap_mrr | 10.5 / 12 | 0.875 |
| best_tiou | 1.61359 / 12 | 0.134466 |
| negative_nonempty_rate | 2 / 2 valid negatives | 1 |

Negative failures: 0; top-1 scores: `{'n': 2, 'mean': 0.043678635768960763, 'min': 0.042285619688776074, 'max': 0.04507165184914545, 'p50': 0.042285619688776074, 'p95': 0.04507165184914545}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 9, 'mean': 29444.444444444445, 'min': 1000, 'max': 61000, 'p50': 31000, 'p95': 61000}, 'end_absolute_error_ms': {'n': 9, 'mean': 51255.77777777778, 'min': 7146, 'max': 99689, 'p50': 48709, 'p95': 99689}, 'tiou': {'n': 9, 'mean': 0.14268357972818757, 'min': 0.06275, 'max': 0.45693333333333336, 'p50': 0.08304444444444445, 'p95': 0.45693333333333336}}`.

Latency ms: `{'n': 14, 'mean': 143.50307142857145, 'min': 133.746, 'max': 165.648, 'p50': 142.704, 'p95': 165.648}`.

## fused / language: zh

Queries: 21; positives: 18; negatives: 3; status: {'ok': 21}

| Metric | Sum / denominator | Value |
|---|---:|---:|
| video_recall@1 | 17 / 18 | 0.944444 |
| video_recall@5 | 18 / 18 | 1 |
| video_recall@10 | 18 / 18 | 1 |
| overlap_recall@1 | 15 / 18 | 0.833333 |
| overlap_recall@5 | 18 / 18 | 1 |
| overlap_recall@10 | 18 / 18 | 1 |
| tiou_0.3_recall@1 | 3 / 18 | 0.166667 |
| tiou_0.3_recall@5 | 4 / 18 | 0.222222 |
| tiou_0.3_recall@10 | 4 / 18 | 0.222222 |
| tiou_0.5_recall@1 | 0 / 18 | 0 |
| tiou_0.5_recall@5 | 0 / 18 | 0 |
| tiou_0.5_recall@10 | 0 / 18 | 0 |
| jump_hit@1 | 4 / 18 | 0.222222 |
| jump_hit@5 | 5 / 18 | 0.277778 |
| jump_hit@10 | 5 / 18 | 0.277778 |
| overlap_mrr | 16.3333 / 18 | 0.907407 |
| best_tiou | 3.76094 / 18 | 0.208941 |
| negative_nonempty_rate | 3 / 3 valid negatives | 1 |

Negative failures: 0; top-1 scores: `{'n': 3, 'mean': 0.04113452167741511, 'min': 0.03662057044079516, 'max': 0.0447650709945792, 'p50': 0.04201792359687097, 'p95': 0.0447650709945792}`.

Correct-video rank-1 localization (conditional sample counts): `{'start_absolute_error_ms': {'n': 17, 'mean': 30058.823529411766, 'min': 1000, 'max': 101000, 'p50': 20000, 'p95': 101000}, 'end_absolute_error_ms': {'n': 17, 'mean': 33003.117647058825, 'min': 5021, 'max': 108434, 'p50': 16695, 'p95': 108434}, 'tiou': {'n': 17, 'mean': 0.17223077358972314, 'min': 0.0, 'max': 0.35654545454545455, 'p50': 0.16666666666666666, 'p95': 0.35654545454545455}}`.

Latency ms: `{'n': 21, 'mean': 154.47038095238096, 'min': 134.147, 'max': 177.514, 'p50': 154.979, 'p95': 167.669}`.

## Scoring definition

```json
{
  "version": "videohelper-eval-v1",
  "modes": [
    "visual",
    "speech",
    "fused"
  ],
  "k": [
    1,
    5,
    10
  ],
  "intervals": "half-open integer milliseconds",
  "positive_denominator": "all answerable queries, including failures",
  "failure_policy": "missing, duplicate, invalid, error or skipped => zero positive credit",
  "ranking": "input order; no score sorting or deduplication",
  "matching": "same media_id; any annotated target",
  "tiou_thresholds": [
    0.3,
    0.5
  ],
  "mrr": "reciprocal first same-media overlapping rank across all returned moments",
  "localization": "rank-1 only when correct media; target maximizing tIoU, then minimum boundary error",
  "best_tiou": "maximum across all returned same-media moments, failures and misses zero",
  "negative_denominator": "valid responses only; failures separately counted, never empty successes",
  "percentiles": "nearest rank: sorted values[ceil(p*n)-1]"
}
```

## Limits

- Labels are synthetic script/timeline derived and have not been independently human reviewed.
- Fused uses active channels including audio; this corpus has no independent sound-channel gold labels.
- Negative nonempty rates describe returned candidates, not calibrated refusal quality. RRF scores are not probabilities.
- Fake model runs establish process behavior only, not real-model retrieval quality.
