# Fixed-index parameter ablation

Selected on development only: `abl02-combo06`; baseline: `abl01-default`.

Recommendation: **verified_candidate**.

Objective: mean(fused jump_hit@1, fused tIoU≥0.5 recall@1). Fused video recall@1 and @5 must not decrease.

| Run | Eligible | Dev objective | Exclusion |
|---|---|---:|---|
| abl01-default | True | 0.133333 |  |
| abl01-k3 | False | 0.3 | development video recall guardrail failed |
| abl01-k5 | True | 0.3 |  |
| abl01-k10 | False | 0.2 | development video recall guardrail failed |
| abl01-k20 | False | 0.1 | development video recall guardrail failed |
| abl01-k100 | False | 0.166667 | development video recall guardrail failed |
| abl01-gap0 | True | 0.133333 |  |
| abl01-rrf10 | True | 0.1 |  |
| abl01-rrf30 | True | 0.133333 |  |
| abl01-rrf100 | True | 0.133333 |  |
| abl01-audio0 | True | 0.133333 |  |
| abl01-audio0p25 | True | 0.133333 |  |
| abl01-audio0p5 | True | 0.133333 |  |
| abl01-audio2 | False | 0.133333 | development video recall guardrail failed |
| abl01-speech0p5 | True | 0.1 |  |
| abl01-speech2 | True | 0.133333 |  |
| abl01-visual0p5 | False | 0.133333 | development video recall guardrail failed |
| abl01-visual2 | True | 0.1 |  |
| abl01-only-visual | False | 0.233333 | development video recall guardrail failed |
| abl01-only-speech | True | 0.266667 |  |
| abl01-only-audio | False | 0 | development video recall guardrail failed |
| abl01-no-visual | False | 0.0666667 | development video recall guardrail failed |
| abl01-no-speech | True | 0.166667 |  |
| abl02-combo01 | True | 0.233333 |  |
| abl02-combo02 | True | 0.233333 |  |
| abl02-combo03 | True | 0.3 |  |
| abl02-combo04 | True | 0.166667 |  |
| abl02-combo05 | True | 0.166667 |  |
| abl02-combo06 | True | 0.3 |  |
| abl02-combo07 | True | 0.233333 |  |
| abl02-combo08 | True | 0.3 |  |
| abl02-combo09 | True | 0.3 |  |
| abl02-combo10 | True | 0.266667 |  |
| abl02-combo11 | True | 0.1 |  |
| abl02-combo12 | True | 0.133333 |  |
| abl02-combo13 | True | 0.1 |  |
| abl02-combo14 | True | 0.1 |  |
| abl02-combo15 | True | 0.1 |  |
| abl02-combo16 | True | 0.133333 |  |
| abl02-combo17 | True | 0.133333 |  |
| abl02-combo18 | True | 0.133333 |  |

## Final verification

- verification video recall guardrails passed: **True**.
- full video recall guardrails passed: **True**.

A failed final check blocks recommendation; verification never reselects a case.

All filtered metrics and counts are in summary.json.

Verification queries are an internal check, not an untouched blind test: the historical 35-query set has been used before. Labels remain synthetic.
