# 当前基线：baseline-v2

2026-10-25，真实模型的完整链路已跑通：**上传 → 完整索引 → 索引检查 → 冻结快照 → visual / speech / fused 检索 → 逐题评分 → 重复运行比较 → 恢复快照再比较**。本次建立的是初始基线，尚未设定检索质量达标阈值。下文记录历史验收结果；旧数据库、对象桶、运行输出及快照已清理，当前不能恢复该索引。

完整操作命令见 [RUNBOOK](RUNBOOK.md)。固定数据为 5 个合成视频、498 秒、30 条正例和 5 条负例；媒体、标注及哈希保持原样，`human_reviewed=false`。

## 索引验收

| 检查 | 旧 baseline-v1 | 新 baseline-v2 |
| --- | ---: | ---: |
| 真实上传 | 5/5 | 5/5 |
| 视频 ready | 3/5 | 5/5 |
| 必需阶段完成 | 29/35 | 35/35 |
| visual / speech / audio 向量 | 251 / 45 / 251 | 251 / 83 / 251 |
| 完整索引验收 | 失败 | 通过 |
| 冻结快照（历史） | 未创建 | 当时已保存，现已清理 |

新轮次耗时 337.48 秒，包含环境准备、上传、索引检查及快照归档；模型已提前加载。五个视频的 ASR 都只尝试一次。原失败基线与原始诊断报告可从 Git 历史追溯；ASR 修复 `main 30efaef` 经 `a98cd20` 合入 `eval` 后，新建数据库和桶重跑。

索引身份：`3a8440de6c054e4413613c3d35b018015f8516bb2a846f9d27cfcbd1bbd73f7c`。WeMM / CLAP 指纹未变；ASR 适配器升为 version 2，新指纹、模型清单、代码版本及二进制哈希均记录在历史报告中。

历史原始报告保存在提交 `11a6668`，可在仓库根目录查看（不恢复运行资源）：

```sh
git show 11a6668:Eval/runs/baseline-v2-index/report.md
git show 11a6668:Eval/runs/baseline-v2-index/inspection.json
git show 11a6668:Eval/snapshots/baseline-v2/snapshot.json
```

[历史 ASR 缺陷及保留测试证据](defects/asr-timestamps/README.md)继续保留。

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

保留的[机器可读基线汇总](baselines/baseline-v2.json)是原 `baseline-v2-a/report.json` 的原样副本，供后续评分比较；不包含原始返回片段。完整报告、逐题评分与原始响应可从 Git 历史读取：

```sh
git show 11a6668:Eval/runs/baseline-v2-a/report.md
git show 11a6668:Eval/runs/baseline-v2-a/per_query.jsonl
git show 11a6668:Eval/runs/baseline-v2-a/raw-responses.jsonl
```

## 重复性与恢复

| 验证 | 结果 |
| --- | --- |
| 原索引 A → B | 105/105 响应内容相同，耗时除外；逐题指标变化 0 |
| 快照恢复至新数据库/桶 | 索引 SHA256 与原快照一致 |
| 恢复对象完整性 | 20/20 个对象，145,149,694 字节；对象集合、大小与 SHA256 全部匹配 |
| 原索引 A → 恢复库检索 | 105/105 响应内容相同，耗时除外；逐题指标变化 0 |

两份正式比较均为 `compatible=true`、`fixed_index_regression=true`、`identical_retrieval_conditions=true`。恢复检索的 `metadata.runtime` 明确记录 `vh_eval_baseline_v2_restored` / `vh-eval-baseline-v2-restored`。新增的运行来源字段只影响报告，不改变搜索算法或二进制。

重复与恢复验证的原始报告也在 `11a6668`：

```sh
git show 11a6668:Eval/runs/baseline-v2-a-vs-b/delta.md
git show 11a6668:Eval/runs/baseline-v2-a-vs-restore-verified/delta.md
git show 11a6668:Eval/runs/baseline-v2-restore/verification.json
git show 11a6668:Eval/runs/baseline-v2-restore/repeatability.json
```

另有一次名为 `baseline-v2-restored-check` 的运行，调用时漏传 `--state`，实际仍查询原库；它不计入恢复验收。范围更正见 `git show 11a6668:Eval/runs/baseline-v2-restored-check/RUN-NOTE.md`。随后补充了报告中的实际资源来源和恢复分支测试，并以显式 `--state` 完成上述正式验证。这些是历史结果，不代表当前仍有可用恢复资源。

## 使用与边界

后续开发继续合入 `eval`；每个并行单元使用独立 worktree、`gpt-6.1-sol` / high，验证后小步提交、合并、移除 worktree。工具已通过 39 项 Python 单元检查、12 项冻结 ASR 输出检查，以及 Go 桥接 race / vet。

完整流程见 [RUNBOOK](RUNBOOK.md)：选择全新的 `SNAPSHOT` 与 `RUN`，先重新索引，再将新报告与 `baselines/baseline-v2.json` 比较。重新索引后与旧基线的比较不能称作固定索引回归；同一新快照内的重复检索、调参与恢复比较仍可按固定索引规则验收。

保留内容限于结论、上述机器可读基线汇总、评估工具与测试、冻结数据集和必需 ASR frozen fixtures。旧运行日志、逐题输出、数据库、对象桶、dump、对象快照、缓存、构建产物和临时 worktree 已清理；Git 历史中的报告不能替代已删除的数据库和对象归档。

未覆盖独立人工金标、真实拍摄视频、大库规模、冷启动/并发压测、HTTP JWT 鉴权入口、前端或 AWS 验收。串行热模型查询耗时不能作为线上 SLA；五视频上的重复一致不能证明所有输入上的确定性。
