# Evaluation v1 契约

新增评估代码、配置、运行结果、快照均在 `Eval/`。集成分支为 `main`。语料身份由 `manifest.json`、`annotations/queries.jsonl` 与固定素材哈希决定；不改动现有 35 条标签。

## 两种运行

1. `index`：校验语料 → 使用现有 local-upload 真实分片上传 → worker 全流程 → 索引检查 → 停止 worker → 保存数据库快照、对象和稳定内容指纹。
2. `retrieve`：验证快照身份/索引未变 → 对全库 35 条查询运行 visual、speech、fused → 保存逐题原始结果、指标与报告。查询失败、跳过通道不能计作有效空结果。fused 使用运行配置与显式覆盖后有效权重大于 0 的 active 通道；当前默认关闭 audio 查询。消融可通过非负权重调整通道，报告记录实际通道。本库没有独立声音金标。

本地入口复用真实业务服务及对象存储；不宣称验收了 JWT HTTP API、前端或云部署。使用独立 Eval 数据库、Redis 队列和对象桶，不写开发数据。任何 fake 运行都必须标为流程测试，不能生成真实模型质量基线。

## Go 桥接入口

代码在 `Eval/cmd/evalbridge/`，编译到 `Eval/bin/evalbridge`，复用 `internal/search` 和 `internal/timeline`。命令为 `evalbridge -config <path> inspect|search`，每行 stdin 为一个 JSON 请求，每行 stdout 为一个 JSON 响应；日志只写 stderr。

共同输入：`workspace_id`、`user_id`、`video_ids`（非空，整个语料库的 ID）。

- inspect 输入只有共同字段。输出 `passed`、`violations`、`index_sha256`、`videos`、`spaces`、`text_spans`。检查 ready、必需阶段完成、归属、非空 ASR、切片覆盖、向量维度/有限数值/非零、有效 HNSW 和模型空间身份。视觉/声音使用 `timeline.Cells`；speech 使用实际 ASR 经 `MergeSentences` 的范围。哈希应含实际向量、文本与稳定媒体/空间元数据，以稳定顺序计算；读取用一致性事务，不包括检查时间。
- search 另有 `query_id`、`mode`（visual/speech/fused）、`text`、`limit`。输出 `query_id`、`mode`、`latency_ms`、`moments`、`skipped`、`error`；moment 字段为 `video_id,start_ms,end_ms,jump_ms,score,channels,snippet`。搜索实际调用 `search.Service.Search`；对所有指定视频搜索。每题失败保留 error，继续后面的查询；输入/初始化错误必须返回非零退出码。

仅 fused 请求可带 `channel_weights`，键为已配置通道，值为有限非负乘数；每个未显式指定的通道继承 `search.channel_weights`，配置中缺少的通道为 1。请求值逐通道覆盖默认乘数，最终有效权重为 `channels.yaml` 中基础权重 × 解析后的乘数；0 关闭该通道，不允许全部有效权重为 0。桥接直接传给已有 `search.Query.ChannelWeights`，不复制或修改排序实现。runner 的 `--visual-weight`、`--speech-weight`、`--audio-weight` 只影响 fused，省略时继承所选快照或恢复状态的运行配置；旧配置没有默认乘数字段时仍为 1/1/1。visual/speech 单通道对照由桥接显式设置对应通道乘数为 1，不继承融合默认乘数，也不接受用户传入权重。

## Python 评分接口

`Eval/tools/score.py` 仅用标准库，提供 `evaluate(queries, results, metadata)` 和 `write_report(report, output_dir)`，另提供对已有两个报告的比较 CLI。`results` 是上述逐题响应，runner 给每个 moment 补充稳定 `media_id`。`metadata` 包含 dataset_id、dataset_sha256、labels_sha256、snapshot_id、index_sha256、code_revision、model_mode、channels、search_config、run_id。缺少/重复题目与模式、非法时间/分数、错误或降级必须明确报告，不可静默排除分母。

正例按模式及类别分别统计：video Recall@1/5/10；半开区间 overlap Recall@1/5/10；tIoU>=0.3/0.5 Recall@1/5/10；MRR（overlap）；jump-window Hit@1/5/10；正确视频 Top-1 的起点/终点误差和最佳 tIoU。分母、样本数必须展示，tIoU 避免用超长结果获取虚高命中率。负例单独报告非空结果率与分数，不把 RRF 分数当概率，也不宣称已校准拒答阈值。报告明确标签尚未独立人工复核。

报告输出 `report.json`、`report.md`、`per_query.jsonl`。比较时强制匹配语料/标签/评分定义；快照或模型不同要标出，不能混成固定索引回归。所有时间区间是 `[start_ms,end_ms)`；jump window 也按半开区间使用。

`metadata.search_config.channel_weights` 保留运行配置的默认乘数（旧配置无此字段）；`metadata.fused_channel_weights` 保存应用显式覆盖后解析出的完整乘数，以及与基础权重相乘后的有效值。只改权重仍可属于固定索引比较，但不是相同检索条件；缺少此字段的历史报告保守标为条件不同。`ablate.py summarize` 只用预先声明的 development 查询选择参数，检查模型、索引及 bridge 二进制身份一致；`--verify` 仅核验已选配置，失败不重新选配置。内部核验集曾用于旧基线，不能称作新盲测。

## 小单元与验收

- U0：冻结语料、建立 eval 分支和工作约定。
- U1：纯评分器、边界测试、机器可读/可对比报告。
- U2：Go 检索桥接及真实索引检查、单元测试。
- U3：独立运行配置、上传/索引编排、快照保存与恢复。
- U4：整合固定快照回归、失败处理和完整流程测试。
- U5：真实模型完整索引基线、两次检索回归与比较、运行说明。

每个实现单元验证后独立 commit；并行单元在独立 worktree 开发，协调者评审合并进 main 后删除 worktree。保留失败结果及原因，不修改冻结标签或覆盖基线；参数消融另存计划、各配置与结果，明确区分调参和核验。
