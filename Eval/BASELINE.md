# 当前基线：baseline-v2

2026-10-25，真实模型的完整链路已跑通：**上传 → 完整索引 → 索引检查 → 冻结快照 → visual / speech / fused 检索 → 逐题评分 → 重复运行比较 → 恢复快照再比较**。本次建立的是可复现的初始基线，尚未设定检索质量达标阈值。

完整操作命令见 [RUNBOOK](RUNBOOK.md)。固定数据为 5 个合成视频、498 秒、30 条正例和 5 条负例；媒体、标注及哈希保持原样，`human_reviewed=false`。

## 索引验收

| 检查 | 旧 baseline-v1 | 新 baseline-v2 |
| --- | ---: | ---: |
| 真实上传 | 5/5 | 5/5 |
| 视频 ready | 3/5 | 5/5 |
| 必需阶段完成 | 29/35 | 35/35 |
| visual / speech / audio 向量 | 251 / 45 / 251 | 251 / 83 / 251 |
| 完整索引验收 | 失败 | 通过 |
| 冻结快照 | 未创建 | 已保存 |

新轮次耗时 337.48 秒，包含环境准备、上传、索引检查及快照归档；模型已提前加载。五个视频的 ASR 都只尝试一次。原失败基线与原始诊断输出保留，ASR 修复 `main 30efaef` 经 `a98cd20` 合入 `eval` 后，新建数据库和桶重跑。

索引身份：`3a8440de6c054e4413613c3d35b018015f8516bb2a846f9d27cfcbd1bbd73f7c`。WeMM / CLAP 指纹未变；ASR 适配器升为 version 2，新指纹、模型清单、代码版本及二进制哈希均保存在报告中。

- [索引报告与旧轮次对比](runs/baseline-v2-index/report.md)、[机器可读报告](runs/baseline-v2-index/report.json)
- [逐项索引检查](runs/baseline-v2-index/inspection.json)、[快照清单](snapshots/baseline-v2/snapshot.json)
- [历史 ASR 缺陷及交接记录](defects/asr-timestamps/README.md)

## 检索效果

首轮 `baseline-v2-a` 执行全库 35 条查询 × 3 种模式，共 **105/105 有效响应**，无错误、缺失或通道降级，检索前后索引不变。下表每列都以全部 30 条正例为分母；按画面、讲解、组合描述及查询语言分组的结果见完整报告。`fused` 包含 audio、speech、visual 三路。

| 指标 | visual | speech | fused |
| --- | ---: | ---: | ---: |
| 视频 Recall@1 | 29/30（96.7%） | 28/30（93.3%） | 26/30（86.7%） |
| 时间重叠 Recall@1 | 27/30（90.0%） | 26/30（86.7%） | 24/30（80.0%） |
| tIoU ≥ 0.3 Recall@1 | 10/30（33.3%） | 14/30（46.7%） | 4/30（13.3%） |
| tIoU ≥ 0.5 Recall@1 | 6/30（20.0%） | 5/30（16.7%） | 0/30（0%） |
| 跳转窗口 Hit@1 | 9/30（30.0%） | 8/30（26.7%） | 5/30（16.7%） |
| 负例非空候选率 | 5/5（100%） | 5/5（100%） | 5/5（100%） |
| 查询耗时 p50 / p95，ms | 139 / 914 | 133 / 141 | 146 / 165 |

**找到对应视频与准确定位片段是两个结论。** 当前主要质量问题是片段合并偏长，融合也不总优于单通道。负例均返回候选说明尚未建立拒答判定；RRF 分数不能直接作为概率阈值。具体查询、代码机制与后续实验建议见 [失败案例分析](analysis/baseline-v2-quality.md)。本次不修改金标或调参来抬高基线。

- [完整检索报告](runs/baseline-v2-a/report.md)、[机器可读报告](runs/baseline-v2-a/report.json)
- [105 条逐题评分](runs/baseline-v2-a/per_query.jsonl)、[原始响应](runs/baseline-v2-a/raw-responses.jsonl)

## 重复性与恢复

| 验证 | 结果 |
| --- | --- |
| 原索引 A → B | 105/105 响应内容相同，耗时除外；逐题指标变化 0 |
| 快照恢复至新数据库/桶 | 索引 SHA256 与原快照一致 |
| 恢复对象完整性 | 20/20 个对象，145,149,694 字节；对象集合、大小与 SHA256 全部匹配 |
| 原索引 A → 恢复库检索 | 105/105 响应内容相同，耗时除外；逐题指标变化 0 |

两份正式比较均为 `compatible=true`、`fixed_index_regression=true`、`identical_retrieval_conditions=true`。恢复检索的 `metadata.runtime` 明确记录 `vh_eval_baseline_v2_restored` / `vh-eval-baseline-v2-restored`。新增的运行来源字段只影响报告，不改变搜索算法或二进制。

- [同索引重复对比](runs/baseline-v2-a-vs-b/delta.md)
- [恢复后正式对比](runs/baseline-v2-a-vs-restore-verified/delta.md)、[恢复检索报告](runs/baseline-v2-restored-verified/report.md)
- [恢复对象核验](runs/baseline-v2-restore/verification.json)、[响应内容一致性核验](runs/baseline-v2-restore/repeatability.json)

另有一次名为 `baseline-v2-restored-check` 的运行，调用时漏传 `--state`，实际仍查询原库；它及对应比较已加 [范围更正说明](runs/baseline-v2-restored-check/RUN-NOTE.md)，不计入恢复验收。随后补充了报告中的实际资源来源和恢复分支测试，并以显式 `--state` 完成上述正式验证。历史输出没有覆盖。

## 使用与边界

后续开发继续合入 `eval`；每个并行单元使用独立 worktree、`gpt-6.1-sol` / high，验证后小步提交、合并、移除 worktree。工具已通过 39 项 Python 单元检查、12 项冻结 ASR 输出检查，以及 Go 桥接 race / vet。

完整流程见 [RUNBOOK](RUNBOOK.md)：选择一次 `SNAPSHOT` 与全新的 `RUN`，将新报告与 `baseline-v2-a` 比较。只改检索参数时复用快照；改索引、模型身份、素材或标签时建立新索引基线，不能称作固定索引回归。

本轮 worker、Eval Redis 与本轮启动的 ML 服务在验证完成后停止，数据库、桶、快照和日志保留。Git 保存报告和清单；`database.dump`、对象归档及运行缓存按忽略规则留在本地，跨机器复现还需备份完整快照目录。

未覆盖独立人工金标、真实拍摄视频、大库规模、冷启动/并发压测、HTTP JWT 鉴权入口、前端或 AWS 验收。串行热模型查询耗时不能作为线上 SLA；五视频上的重复一致不能证明所有输入上的确定性。
