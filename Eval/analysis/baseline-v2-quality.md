# baseline-v2-a 检索质量分析

当前结果说明系统常能找到正确视频，但片段边界与跳转仍弱；加入全部通道没有改善这批查询，反而降低 Top1 视频召回并扩大片段。这里的证据尚不足以把问题归为生产实现 bug。

## 证据与口径

历史分析数据来自 `runs/baseline-v2-a/{report.json,per_query.jsonl,results.jsonl}`、`annotations/queries.jsonl` 和 `manifest.json`；路径均相对 `Eval/`。当前仅保留原样[基线汇总](../baselines/baseline-v2.json)，逐题输出与旧快照已清理。原始证据可从提交 `11a6668` 查看，例如 `git show 11a6668:Eval/runs/baseline-v2-a/results.jsonl`；下文例子与归因均针对该历史运行。真实模型运行版本为 `bdbf87636e787e9c7f184b59eb4682b4cb8b18dc`，snapshot 为 `baseline-v2`，报告记录索引未改变、bridge 正常退出。5 个合成视频共 498 秒，30 个正例（speech / visual / mixed 各 10 个）和 5 个无答案查询；标签源于脚本和渲染时间线，未经独立人工审核。

每路召回 `K=50`，`rrf_k=60`，`merge_gap_cells=1`，各通道权重为 1。visual / speech 分别只启用对应通道；fused 启用 visual、speech、audio（不是仅前两者融合）。runner 每次请求最多返回 10 个 moment。WeMM 用于 visual 和 speech，Paraformer 用于 ASR，CLAP 用于 audio。

| 指标 | visual | speech | fused |
|---|---:|---:|---:|
| 成功响应 | 35/35 | 35/35 | 35/35 |
| 正例 video Recall@1 | 29/30 | 28/30 | 26/30 |
| 正例 video Recall@5 | 29/30 | 30/30 | 30/30 |
| 正例 overlap Recall@1 | 27/30 | 26/30 | 24/30 |
| 正例 tIoU≥0.5 Recall@1 | 6/30 | 5/30 | 0/30 |
| 正例 tIoU≥0.5 Recall@10 | 7/30 | 7/30 | 0/30 |
| 正例 jump_hit@1 | 9/30 | 8/30 | 5/30 |
| 负例返回非空 | 5/5 | 5/5 | 5/5 |

video Recall 只要求同一视频，不要求时间重叠；overlap 只要求正长度交集；tIoU 衡量交集/并集；jump_hit 要求跳转落入标注的可接受窗口。K 按返回 moment 排名截断，没有先按视频去重。这些指标不能互相替代。`report.passed=true` 仅代表所有响应通过结构/执行有效性检查，不是质量达标（`tools/score.py:205–232`）。

## 四个可复核例子

以下区间均为半开毫秒区间；tIoU 为该结果与标注的时间交并比。

1. **q004「读完代码之后，怎样验证自己是否真正理解了它？」：正确局部被扩成长段。** gold 为 `study_zh [43000,51029)`。visual Top1 是 `[42000,56000)`，tIoU=0.5735；speech 是 `[39000,52000)`，tIoU≈0.6176；fused 是 `[39000,83000)`，tIoU≈0.1825。三者都找到正确视频，但 fused 边界显著变宽。fused 的 `jump_ms=39910` 早于标注可接受窗口 `[42000,45000)`，snippet 为「嗯嗯，」；该现象需要核对 ASR 句子与音频，不能仅凭返回值认定原声没有目标内容。
2. **q017「A fox and a rabbit sit outside a cottage with a round glowing window at night.」：更多通道可改变正确视频排序。** gold 为 `story_en [72000,90000)`；visual、speech 单通道 Top1 均为 `story_en`。fused 把 `study_zh [61000,84000)` 排到第一，分数 0.03991191192935528；正确视频 `[0,108000)` 排第二，分数 0.03962812590436307。错误段记录的最好名次为 audio=1、speech=11、visual=45，正确段为 audio=33、speech=1、visual=1。这与 audio 参与 RRF 后排序受影响相符；尚未运行 visual+speech 双通道消融，不能把全部损失唯一归因于 CLAP。`Channels` 是整段各通道最好名次，可能来自不同 cell，不能直接用这些汇总名次重算 moment 分数。
3. **q030「A green aurora above a dark rocky coast … everyday worries feeling more distant.」：视觉场景与语音标签时长存在设计冲突。** gold 为 `landscape_en [76000,82854)`，是本场景讲解的 6854ms。speech Top1 `[75000,85000)` 的 tIoU=0.6854；visual 与 fused 均为整个场景 `[75000,90000)`，tIoU≈0.4569，fused 的 `jump_ms=75980` 却命中可接受窗口 `[75000,78000)`。因此「找到正确场景并可跳转」与「tIoU≥0.5」可得不同判断；mixed 标签只覆盖讲解区间，使整场景结果无法过 0.5，不宜把这一例全算作模型失效，也不能因此事后改冻结标签抬高分数。
4. **q031「宇航员在月球表面插上一面旗帜。」：排序不等于无答案识别。** gold 无目标；visual、speech Top1 分数均为 `1/(60+1)=0.01639344262295082`，fused 返回 `story_en [45000,100000)`，分数 0.04201792359687097。其余 4 个负例也全部非空。单通道所有负例的 Top1 分数恰好相同，是 rank-only RRF 的直接结果；不能把该分数当相似度或置信概率，也不能据此选出通用拒答阈值。

## 机制与归因

生产链路为 `search.Search → recall → index.SearchVectors → fuse → mergeCells`。对应代码与设计是：

- **召回没有语义相关性门槛。** `internal/search/search.go:190` 每路取 `cfg.K`；`internal/index/vector.go:183–200` 按距离取 TopK，没有距离阈值；`internal/search/fuse.go:94` 用名次加权，未使用距离幅度。因此非空索引通常仍为无答案查询返回候选。这是缺少校准/拒答机制，不是响应失败。
- **弱命中也可把局部连成大段。** `fuse.go:78–94` 将所有命中投到 cell 并赋分；`:110–115` 不区分高低分、也不检测场景边界，按相邻或最多空 1 cell 连通合并；`:135–143` 返回整个连通段并取其中最大 cell 分数。增加通道增加了候选覆盖和连接机会。调 `merge_gap_cells=0` 仍会合并连续命中，并不保证定位改善。cell 宽度由 `internal/timeline/grid.go:5–9` 决定：视频不足 90 秒用 1 秒，否则 2.5 秒；映射还会引入格子量化。
- **跳转不一定指向 fused 分数最高处。** `fuse.go:149–162` 优先选择整个合并段中 speech 名次最好句子的起点，即使原查询主要描述画面；没有 speech 才用片段起点。ASR 识别/时间戳误差和长段内的非目标高排名句子都可能拉低 jump_hit。snippet 根据跳转所覆盖句子附加（`:167–181`），不是模型新生成的回答。

上述合并、最高 cell 分数、speech 优先跳转、段内最好通道名次均与 `docs/spec.md:2061–2083` 一致，其中明确暂不限制片段长度；`internal/search/fuse_test.go:37–42` 也固定了跨空 cell 合并和最大分数行为。因此本次未发现这些现象对应的契约违背、半开区间错误或漏执行代码分支。真实模型局部排序、ASR 质量、合成标签边界和融合设计仍须分开验证。结果文件没有逐路原始距离/命中 cell 轨迹，不能精确还原每一条低排名命中的连接贡献。

## 下一轮最小实验

1. 旧索引与快照已清理，先用冻结 corpus、labels 和选定模型重新索引；与旧基线只能作跨索引比较。随后固定新 index 和模型，先补逐路 TopK 的距离、rank、span 与合并前 cell 分数诊断，再做 **visual+speech 与当前三通道** 对照；以 q017/q018/q024/q025 的视频排序变化及全部 30 正例指标判断 audio 的净贡献。audio 没有独立声音 gold，本轮不构成 CLAP 声音检索验收。
2. 固定通道，分别对照 `K=10/20/50` 与 `merge_gap_cells=0/1`，一次只变一个因素；同时记录召回、tIoU、jump_hit 和返回段长。若连续弱命中仍贯通长段，再评估保留局部高分峰/分段的设计，不能预先宣称减 K 或 gap 必定有效。
3. 人工听看 q004/q017/q030，分别检查 ASR 原文与起点、目标证据、mixed 应取讲解还是场景窗口。审核形成独立记录；语义标签需要改变时发布新 dataset 版本，不覆盖当前标签。无答案能力需另留校准/验证查询，并保留原始距离等证据；5 个负例不足以制定可靠阈值。

本分析仅复核现有 JSON 与对应版本源码，没有调参运行、重建索引、播放媒体或独立人工标注；没有修改生产代码、冻结媒体或标签。单次顺序、服务预加载的合成小语料结果不能外推到自然视频、大库、多用户并发、冷启动或 AWS 验收。
