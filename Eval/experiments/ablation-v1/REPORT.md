# 固定模型参数消融结果

本报告保存实验当时的设置与结论；后续已将候选作为 `main` 的默认参数，采用与回滚记录见 [Evaluation.md](../../../docs/Evaluation.md)。原始运行结果与快照现位于主仓库 `Eval/runs/`、`Eval/snapshots/`；迁移后的复用方式见 [RUNBOOK](../../RUNBOOK.md)。下方 v1 复跑命令显式声明三路乘数 1，避免受后来默认值影响。

本轮在独立 `eval-ablation` worktree 中完成 **41 组不同参数、2 次重复运行，共 4515 条有效响应**。模型、语料、标签、查询原文和索引均固定。开发集选出的候选为 **K=5、RRF K=60、merge gap=1、visual:speech:audio=1:1:0**。它通过内部验证集的视频召回约束，显著改善片段定位；**全量跳转命中没有改善**，也没有解决无答案查询的拒答问题。

这是本批数据的参数候选，生产默认配置和模型没有修改。权重是请求级乘数；`audio=0` 通过 `channel_weights` 关闭声音路线，不能直接写成 `config/channels.yaml` 中的零配置权重。当前合成库缺少独立自然声音金标，不能据此建议所有真实场景关闭 audio。

## 默认与候选的可比结果

以下均为 fused，分母包含所有正例。Video Recall@5 表示前五个 moment 中包含正确视频，并非五个不同视频。

| 指标 | 开发集默认 → 候选（15 题） | 内部验证默认 → 候选（15 题） | 全部默认 → 候选（30 题） |
|---|---:|---:|---:|
| 视频 Recall@1 | 14 → 14 | 12 → 15 | **26 → 29** |
| 视频 Recall@5 | 15 → 15 | 15 → 15 | **30 → 30** |
| 定位 tIoU≥0.3 Recall@1 | 2 → 9 | 2 → 12 | **4 → 21** |
| 定位 tIoU≥0.5 Recall@1 | 0 → 6 | 0 → 7 | **0 → 13** |
| 跳转 Hit@1 | 4 → 3 | 1 → 2 | **5 → 5** |

全量视频 Recall@1 提高 10 个百分点，定位 tIoU≥0.5 提高 43.3 个百分点。Top-1 片段长度中位数从 **72.5 秒降至 17.5 秒**，均值从 74.2 秒降至 20.9 秒；该统计包含预测到错误视频的正例，完整口径见 [span-lengths.json](span-lengths.json)。边界缩短并不保证 jump 正确。

5 条负例在默认和候选下均返回非空结果（5/5）。本次没有新增拒答阈值；RRF 分数不能直接当作置信概率。fused 延迟中位数在本轮顺序执行中为 153.8 → 137.3 ms，仅是运行记录，未控制预热、顺序和系统负载，不作为性能提升证明。

全局 K 同时影响单通道控制组：

| 模式（全部 30 正例） | 视频 R@1 | 视频 R@5 | tIoU≥0.5 R@1 | Jump H@1 |
|---|---:|---:|---:|---:|
| visual：默认 → 候选 | 29 → 29 | 29 → 29 | 6 → 13 | 9 → 13 |
| speech：默认 → 候选 | 28 → 28 | 30 → 30 | 5 → 11 | 8 → 8 |
| fused：默认 → 候选 | 26 → 29 | 30 → 30 | 0 → 13 | 5 → 5 |

总体指标会掩盖题型回退：例如 fused 的讲解类跳转从 2/10 降到 0/10。逐题证据、具体机制和控制组题型变化见 [FAILURES.md](FAILURES.md)。因此候选适合下一轮定位回归，不应被解释为所有场景均优于默认。

## 实验边界与选择过程

- 索引验收独立完成：5/5 视频 ready、35/35 阶段完成；visual 251、speech 83、audio 251 条向量全部覆盖。冻结 20 个对象，模型模式为 real，索引指纹 `3e82f81d34918567385f2ede34560dd0cd3dc27895310b4ae4d79a473f8007c2`。详见 [index-acceptance.json](index-acceptance.json)。
- 按视频与题型分层，将每组两条正例的一条用于开发、一条用于内部验证：各 15 条正例；负例按 3/2 分割。全部查询仍执行，但参数排名只使用开发集。
- 视频 Recall@1、@5 必须不低于开发集默认值；在合格配置中最大化 `(Jump H@1 + tIoU≥0.5 R@1) / 2`，依次用 tIoU≥0.3、best tIoU、更小 K、run ID 解同分。此目标允许定位与跳转间的取舍，并没有要求每项或每题型分别不退步。
- 第一阶段 23 组：召回 K、RRF 常数、合并间隔、权重、关闭通道及单通道消融。按预定规则选择 K=5/50、RRF=60 后，与 gap=0/1 和五组权重组合，去重后再运行 18 组。完整参数与开发集比较见 [matrix.md](matrix.md) 和 [plan-stage2.json](plan-stage2.json)。
- 候选在查看验证集前提交冻结（`cd7846f`）；随后修复同分浮点误差并再次冻结（`e2ef784`），候选及第二阶段方案均不变，见 [selection-correction.json](selection-correction.json)。之后才核验该候选的内部验证集与全量指标，没有根据验证结果改选。冻结配置见 [selection-summary/selected_config.json](selection-summary/selected_config.json)，最终核验见 [final-summary/summary.json](final-summary/summary.json)。
- 默认与候选各复跑一次，分别 **105/105 原始响应在排除 latency_ms 后完全一致**；所有仅改权重的可比控制组共有 2100 条 visual/speech 响应完全一致。索引和模型身份不变，bridge 二进制全程未重建，详见 [repeatability.json](repeatability.json)。

首轮索引 `ablation-v1` 在宿主机反复睡眠期间发生状态查询/Redis 超时，未进入参数实验。保留失败证据后，新建隔离索引 `ablation-v2` 完整验收成功；有效结果全部使用这一新索引。它与历史 `baseline-v2` 属于不同索引，旧分数只作历史参考，不作为本次参数 A/B 对照。

本轮还修复了两处 Eval 工具问题：缺失/空/不完整权重元数据不能证明检索条件相同；相同整数命中总数必须同分，避免浮点求和绕过 tie-break。两者都有回归测试，并已将原理、复现、代码与修复建议发送至“videohelper-包工头”会话评估。未确认新的生产实现违约；当前长片段合并与 speech 优先跳转符合现有代码和设计，属于效果边界。

## 复跑与回归流程

所有命令在本 worktree 根目录执行；沿用 [RUNBOOK](../../RUNBOOK.md) 启动本地依赖与真实模型服务。worktree 没有独立虚拟环境，本次复用主仓库解释器。旧 run ID 不可复用；下面的 ID 如已存在，应一起更换。

```sh
EVAL_PY=/Users/ozine/Project/Go/videohelper/ml/.venv/bin/python
SNAPSHOT=abl-next-01
RUN=abl-check-01

# 新一轮先验收完整索引；本次现有 ablation-v2 快照可在资源仍保留时复用。
"$EVAL_PY" Eval/tools/run.py build
"$EVAL_PY" Eval/tools/run.py index --run-id "$SNAPSHOT"

# 同一个索引上的默认、候选及重复运行；两次 build 之间不要混用二进制。
"$EVAL_PY" Eval/tools/run.py retrieve --snapshot "$SNAPSHOT" --run-id "${RUN}-default" --k 50 --rrf-k 60 --merge-gap 1 --visual-weight 1 --speech-weight 1 --audio-weight 1
"$EVAL_PY" Eval/tools/run.py retrieve --snapshot "$SNAPSHOT" --run-id "${RUN}-candidate" --k 5 --rrf-k 60 --merge-gap 1 --visual-weight 1 --speech-weight 1 --audio-weight 0
"$EVAL_PY" Eval/tools/run.py retrieve --snapshot "$SNAPSHOT" --run-id "${RUN}-default-repeat" --k 50 --rrf-k 60 --merge-gap 1 --visual-weight 1 --speech-weight 1 --audio-weight 1
"$EVAL_PY" Eval/tools/run.py retrieve --snapshot "$SNAPSHOT" --run-id "${RUN}-repeat" --k 5 --rrf-k 60 --merge-gap 1 --visual-weight 1 --speech-weight 1 --audio-weight 0
"$EVAL_PY" Eval/tools/score.py compare --before "Eval/runs/${RUN}-default/report.json" --after "Eval/runs/${RUN}-candidate/report.json" --output "Eval/runs/${RUN}-comparison"
"$EVAL_PY" Eval/tools/score.py compare --before "Eval/runs/${RUN}-default/report.json" --after "Eval/runs/${RUN}-default-repeat/report.json" --output "Eval/runs/${RUN}-default-repeat-check"
"$EVAL_PY" Eval/tools/score.py compare --before "Eval/runs/${RUN}-candidate/report.json" --after "Eval/runs/${RUN}-repeat/report.json" --output "Eval/runs/${RUN}-repeat-check"
```

上面参数对比应有 `compatible=true`、`fixed_index_regression=true`、`identical_retrieval_conditions=false`；重复运行应三个都为 true，且逐题无变化。需要按本轮全部方案重跑时，逐条读取 `plan-stage2.json` 的 cases，调用 `run.retrieve_run(snapshot_id, run_id, overrides=case["overrides"], fused_weights=case["fused_weights"])`；使用新的 snapshot/run IDs，保持既定分割和选参规则。用 `ablate.py summarize` 生成开发集排名，提交候选后才加 `--verify`。

现有结论无需模型服务即可比较：

```sh
"$EVAL_PY" Eval/tools/score.py compare --before Eval/baselines/ablation-v1-default.json --after Eval/baselines/ablation-v1-selected.json --output Eval/.work/ablation-report-check
```

完整逐题比较已保存为 [comparison/delta.md](comparison/delta.md) 与 [delta.json](comparison/delta.json)，未来回归可直接读取两份 `Eval/baselines/ablation-v1-*.json`。原始响应、索引快照和运行资源位于本 worktree 的 ignored 目录；没有将模型、缓存、大量中间运行文件加入 Git。

本轮 ML 服务、专用 Eval Redis 和随 ML 结束的防休眠进程均已停止；子 Agent worktree 和临时 Go cache 已清理。主 worktree 的实验响应、快照、隔离数据库与对象桶仍保留，供复查；开发用 Postgres、MinIO 和 Redis 未停止。复跑检索前需先重新启动真实模型服务。

本次真实验证覆盖索引、固定索引检索和重复性；工具检查通过 **48 项 Python 单元测试、12 项 ASR frozen fixture 回归、Go evalbridge race/vet 和 Ruff**，另核对了报告链接与 shell 语法。没有重跑快照恢复（历史基线已验收）、生产 HTTP/JWT、并发压力、长时间稳定性或新真实视频盲测。35 条合成标注未经独立人工复核，内部验证集也曾用于旧基线；需新增人工金标和自然声音样本后，再决定是否推广候选。
