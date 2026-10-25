# baseline-v1 索引验收报告

**结论：验收未通过。** 5 个视频已真实上传；3 个 ready，29/35 个必需阶段完成。两个视频的 ASR 时间戳兼容性缺陷阻止完整索引，因此没有生成合格索引快照，也没有执行检索质量评分或恢复验收。

本报告保留失败基线，后续修复后应以新 run ID 对比；缺失的 Recall/MRR 等指标是未运行，不是 0，也不能填写估计值。

| 视频 | 语言 | 状态 | 完成必需阶段 | visual 向量 | speech 向量 | audio 向量 | ASR |
| --- | --- | --- | ---: | ---: | ---: | ---: | --- |
| study_zh | zh | ready | 7/7 | 84 | 15 | 84 | done，最近轮询累计 1 次 |
| blockchain_mix | zh-en | ready | 7/7 | 48 | 18 | 48 | done，最近轮询累计 1 次 |
| story_en | en | indexing | 4/7 | 44 | 0 | 44 | queued，最近轮询累计 6 次 |
| vlog_zh | zh | indexing | 4/7 | 39 | 0 | 39 | queued，最近轮询累计 6 次 |
| landscape_en | en | ready | 7/7 | 36 | 12 | 36 | done，最近轮询累计 1 次 |

画面/声音两路各 251 条向量的实际区间与预期格子完全相符，维度、数值及 HNSW 检查通过。已有 ASR 的三个视频共 45 条 speech 向量；缺失 ASR 的两项显示 expected=actual=0，**不能作为 speech 覆盖成功**，检查器同时明确报 empty ASR、阶段未完成和视频未 ready。

## 缺陷与复现

独立 HTTP 转录重放均返回 `500 internal: ASR word timestamps are invalid`。进一步捕获 pinned FunASR 原始输出，确认相邻 VAD 段的帧级时间戳越界后直接拼接，产生 60ms / 140ms 词级重叠；adapter 与路由要求单调时间轴，整次转录被拒绝。Go 将该 500 映射为可重试服务错误，因此相同输入反复推理。

完整原理、产生过程、代码位置、最小 fixture 和修复建议见 [缺陷交接报告](../../defects/asr-timestamps/README.md)。按用户要求已发送给「videohelper-包工头」评估，当前没有修改生产实现。

## 可对比的运行身份与证据

- 数据集：`videohelper-synthetic-v1`；代码：`9d10a37411881b862152d2c00183c3e386edf3bd`。
- 数据集清单 SHA-256：`a554a93ad073c19d48f6b2d6024d2b30726b7f06b715c218e3b1f61da92f9956`。
- 标签 SHA-256：`ae5ea40899a795679e0eb848fff3286a333243c346dd94155811f4ed3496caa5`。
- 从启动到明确停止的墙钟时间：927.070 秒，包含等待、重试和诊断，不是正常索引性能。
- 真实模型：WeMM-Embedding-2B、paraformer-zh、CLAP HTSAT unfused；完整指纹在 [report.json](report.json)，环境与二进制哈希在 [software.json](software.json)。
- 原始阶段状态：[stages.json](stages.json)；只读实际索引检查：[inspection.json](inspection.json)；独立重放错误：[asr-diagnostic.json](asr-diagnostic.json)。
- 数据库 `vh_eval_baseline_v1`、桶 `vh-eval-baseline-v1` 和 Eval 队列证据均保留；本轮 worker 已停止。

## 修复后的复跑

按 [完整流程](../../RUNBOOK.md) 用新 ID（例如 `baseline-v2`）运行索引验收，避免覆盖本报告。通过后再分别执行 visual、speech、fused 的 105 次查询、第二次重复运行、数据库/对象快照恢复和恢复后回归。对比机器字段为 ready_videos、required_stages_done/total、vectors、模型/代码身份，以及通过后的 report.json/delta.json。

当前合成标签尚待独立人工复核；尚未验证实际检索质量、无答案拒绝能力、并发/长时间稳定性或 HTTP JWT/前端流程。
