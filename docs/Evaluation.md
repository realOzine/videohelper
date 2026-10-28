# 检索参数版本与评估

本页记录现 `cloud` 分支中服务端版本的历史评估；下文的 `main` 指评估时的旧主分支，不是当前桌面主分支。

本页按参数版本保存设置、实验结论与取舍。**v2 将固定模型消融选出的配置用作默认检索参数：K=5，visual:speech:audio 默认乘数为 1:1:0。** 现有真实模型证据显示视频 Top1 与片段定位改善，但全量跳转命中没有改善，讲解类跳转还出现回退。它是有边界的参数选择，不能代表真实视频质量已全面达标。

这里的参数版本 v1/v2 与实验索引名 `baseline-v2`、`ablation-v2` 不同。模型、语料或标签发生变化时，必须另记身份，不能仅凭参数版本声称固定索引回归。

## 指标与共同边界

当前证据使用 `videohelper-synthetic-v1`：5 个合成视频、498 秒、30 条正例（讲解、画面、组合各 10 条）和 5 条负例。标签来自脚本与渲染时间线，未经独立人工审核。内部验证的 15 条正例曾用于历史基线，属于内部检查，不是新盲测。

- Video Recall@N：前 N 个 moment 中含正确视频，未按视频去重。
- tIoU≥0.5 Recall@1：第一条结果与目标的时间交并比至少 0.5；找到视频并不等于定位成功。
- Jump Hit@1：第一条结果的跳转点落入标注的可接受窗口；片段缩短不保证跳转正确。
- 负例非空：无答案查询仍返回候选；RRF 分数是名次融合分数，不能直接当作置信概率。

查询原文不翻译；visual 与 speech 使用 WeMM-Embedding-2B，speech 由 ASR 提供文本，audio 使用 CLAP HTSAT unfused。参数变化不替换模型、不改变索引内容，继续保留供 ASR 与 CLAP 使用的源音频。配置规则见 [设计](design.md)与[规格](spec.md)。

## v1：初始三通道基线

| 参数 | 设置 |
|---|---|
| 每路召回 `search.k` | 50 |
| `search.rrf_k` | 60 |
| `search.merge_gap_cells` | 1 |
| visual / speech / audio 乘数 | 1 / 1 / 1 |
| `search.max_limit` | 50；评估请求返回最多 10 个 moment |
| 通道基础权重 | `config/channels.yaml` 中三路均为 1 |

### 实验与证据

历史 `baseline-v2` 在真实模型下完成上传、完整索引、三种模式检索、重复运行与快照恢复检查：5/5 视频 ready、35/35 必需阶段完成，visual/speech/audio 向量为 251/83/251。首次检索 105/105 响应有效；同索引重复与恢复后检索均为 105/105 响应一致（耗时除外）。这证明历史运行链路及该批输入的重复性，没有设定质量达标阈值。

| 全部 30 正例 | visual | speech | fused |
|---|---:|---:|---:|
| Video Recall@1 | 29/30 | 28/30 | 26/30 |
| Video Recall@5 | 29/30 | 30/30 | 30/30 |
| tIoU≥0.5 Recall@1 | 6/30 | 5/30 | 0/30 |
| Jump Hit@1 | 9/30 | 8/30 | 5/30 |

三种模式的 5 条负例均返回非空。完整口径、失败案例和归档入口分别见 [历史基线](../Eval/BASELINE.md)、[质量分析](../Eval/analysis/baseline-v2-quality.md)与[机器可读基线](../Eval/baselines/baseline-v2.json)。旧运行资源与快照已清理；原始报告保留于 Git 提交 `11a6668`，读取方法见历史基线。

**优势：** 三路均参与检索，保留声音证据的机会；正确视频常能进入前五个 moment，适合作为完整链路与后续参数对照的起点。

**不足与机制：** K50 保留更多尾部命中，可能将局部候选连成长段。相邻命中间空格数不超过 gap 就继续合并，这种连接可传递；gap1 不限制整段长度，gap0 也仍会合并连续命中。融合取同一 cell 的加权 RRF 和，moment 取其中最高 cell 分数，不累加整段得分，也不惩罚跨度。因此更多通道或更多命中并不保证更好的视频排序或边界。跳转优先使用整段内 raw rank 最好的 speech 句子起点；没有 speech 才取片段起点，可能与目标画面或最高分 cell 错开。源码见 [search.go](../internal/search/search.go)与[fuse.go](../internal/search/fuse.go)。

## v2：缩小召回并默认关闭 audio 检索

| 参数 | 设置；相对 v1 的变化 |
|---|---|
| 每路召回 `search.k` | **5**；由 50 降低 |
| `search.rrf_k` | 60；不变 |
| `search.merge_gap_cells` | 1；不变 |
| `search.channel_weights` | **`{visual: 1, speech: 1, audio: 0}`**；新增默认乘数配置 |
| `search.max_limit` | 50；不变 |
| 通道基础权重、模型与索引 | 不变；三路基础权重仍为 1，继续索引 audio |

默认乘数由 `SearchConfig.ChannelWeights` 提供，缺省通道按 1 处理；内部 `search.Query.ChannelWeights` 按通道覆盖默认乘数，再乘 `config/channels.yaml` 的基础权重。因而内部调用显式传 `audio: 1` 可恢复该路，未传的通道保留默认；Eval 桥接对应字段为 `channel_weights`，这次没有扩展生产 HTTP 请求字段。`audio: 0` 关闭本次检索的声音路线，不是将通道基础权重改成零；基础权重必须保持正值。配置入口为 [config.example.yaml](../config/config.example.yaml)与[local.yaml](../config/local.yaml)。

默认参数落地提交为 `63721d2`，已合入 `main`；Eval runner 随之继承配置默认，并为 visual/speech 控制组显式保留乘数 1。旧配置省略 `search.channel_weights` 时保持三路乘数 1，不会自动改写已有部署的自定义配置。API 进程需重新加载配置后生效；本次没有重启运行中的服务。

### 实验：固定模型参数消融 ablation-v1

本轮在新索引 `ablation-v2` 上完成 **41 组不同参数与 2 次重复，共 4515 条有效响应**。模型、冻结媒体、标签、查询原文与索引固定；bridge 二进制在 sweep 期间未重建。索引单独验收通过，向量数仍为 251/83/251，索引指纹为 `3e82f81d34918567385f2ede34560dd0cd3dc27895310b4ae4d79a473f8007c2`。它不同于历史 `baseline-v2`，因此参数收益采用同一新索引上的 `abl01-default` 与 `abl02-combo06` 对照。

开发与内部验证各含 15 正例，负例按 3/2 分割。全部查询都执行，但选参只使用开发集：先要求视频 R@1/@5 不低于 v1，再最大化 `(Jump H@1 + tIoU≥0.5 R@1) / 2`；同分依次比较 tIoU≥0.3、best tIoU、更小 K、run ID。第一阶段 23 组，第二阶段去重后新增 18 组。候选在查看内部验证结果前冻结；同分浮点修正后的冻结提交为 **`e2ef784`**，候选及第二阶段方案未改变，之后未按验证结果改选。

| fused 指标 | 开发集 v1 → v2（15 正例） | 内部验证 v1 → v2（15 正例） | 全量 v1 → v2（30 正例） |
|---|---:|---:|---:|
| Video Recall@1 | 14 → 14 | 12 → 15 | **26 → 29** |
| Video Recall@5 | 15 → 15 | 15 → 15 | **30 → 30** |
| tIoU≥0.3 Recall@1 | 2 → 9 | 2 → 12 | **4 → 21** |
| tIoU≥0.5 Recall@1 | 0 → 6 | 0 → 7 | **0 → 13** |
| Jump Hit@1 | 4 → 3 | 1 → 2 | **5 → 5** |

Top1 片段长度中位数从 **72.5 秒降至 17.5 秒**，该统计包含预测到错误视频的正例。5 条负例在两版下均非空。默认与候选各复跑一次，分别 105/105 原始响应在排除 `latency_ms` 后完全一致；仅改权重的可比控制组中，2100 条 visual/speech 响应一致。顺序执行的耗时记录未控制预热与系统负载，不作为性能收益证明。

可追溯证据已随报告提交 **`b4703a5`** 保存：

- [实验报告](../Eval/experiments/ablation-v1/REPORT.md)、[选参计划](../Eval/experiments/ablation-v1/plan.json)、[第二阶段计划](../Eval/experiments/ablation-v1/plan-stage2.json)与[完整开发集参数表](../Eval/experiments/ablation-v1/matrix.md)。
- [冻结候选](../Eval/experiments/ablation-v1/selection-summary/selected_config.json)、[同分修正记录](../Eval/experiments/ablation-v1/selection-correction.json)与[最终核验](../Eval/experiments/ablation-v1/final-summary/summary.json)。冻结文件中的 pending 状态是当时的选择记录，最终结论以最终核验为准。
- [v1 同索引汇总](../Eval/baselines/ablation-v1-default.json)、[v2 同索引汇总](../Eval/baselines/ablation-v1-selected.json)、[逐题差异](../Eval/experiments/ablation-v1/comparison/delta.md)、[重复性及响应哈希](../Eval/experiments/ablation-v1/repeatability.json)与[片段长度统计](../Eval/experiments/ablation-v1/span-lengths.json)。

### 优势、不足与作用机制

**优势：** 在本批合成查询上，全量 Top1 视频召回由 26/30 升至 29/30，Top5 保持 30/30，片段明显缩短，tIoU≥0.5 由 0/30 升至 13/30。K5 减少尾部候选覆盖与传递合并的机会；保留相同模型与索引，使参数调整和回滚都无需重新索引。

**不足：** 全量 Jump 仍为 5/30；开发集 4→3，讲解类更从 2/10 降至 0/10。选参目标允许 tIoU 与跳转之间取舍，没有要求每题型分别不退步。K 更小也可能丢掉有用候选，当前 Top5 未退步不能外推到大库。audio 默认关闭会放弃声音证据；当前库没有独立自然声音金标，不能据此建议所有真实场景关闭 CLAP。

**归因边界：** v2 同时改变 K 与 audio 乘数，不能将组合收益全部归因于关闭 audio。开发集单独 K5、三路仍为 1 时，tIoU≥0.5 已由 0/15 升至 5/15；单独 audio0、K 仍为 50 时该指标仍为 0/15。K 是每路共用配置，visual-only、speech-only 也随之变化，不能把它们当作召回条件完全固定的对照。具体案例见[剩余错误分析](../Eval/experiments/ablation-v1/FAILURES.md)。

v2 没有改变同 cell RRF、传递合并、moment 最高分或 raw-rank speech 跳转规则。缩短片段可能拆开竞争 moment，局部双路共现仍可能压过正确句子；`Channels` 记录整段各路最好名次，不能据此推定它们在同一 cell 同时支持。保持片段成员与胜出 moment 不变时，调整正权重不会改变其最好 raw-rank speech 句子；权重改变排序或归零移除通道时仍可能间接改变跳转。q027 等例仍有跨场景长段，现有响应未记录融合前逐格轨迹，无法指定某条命中就是连接长段的桥梁。

### 验证范围与回滚

上述质量结论来自已归档的真实模型索引、检索与重复性实验。本次合入 `main` 后，Go 的 `cmd/`、`internal/`、`migrations/` 和 Eval bridge 范围 race/vet 及主程序编译通过；51 项 Eval 工具单测、12 项 frozen ASR 回归通过。新增检查覆盖默认乘数、逐项覆盖、显式启用 audio、请求/配置 map 不变性、非法默认值、旧配置兼容和单通道控制组；另逐项核对两份发布配置的 K/RRF/gap 与最终有效权重等于冻结候选。报告链接、复跑 shell 语法及迁移后的快照/语料哈希也已校验。

这些检查不能替代真实模型质量验收。本次未设置 `VH_TEST_*`，需要显式数据库/S3 的集成测试未执行；没有重新运行真实模型，也没有验证新配置入口的生产 HTTP/JWT 请求、新真实视频盲测、大库、并发压力、长时间稳定性或 AWS。已构建主程序，但未启动或重启服务，参数落地不能扩大原实验结论的范围。

回滚 v1 时，同时将 `search.k` 恢复为 50，并将 `search.channel_weights` 恢复为 `{visual: 1, speech: 1, audio: 1}`；RRF60、gap1 与通道基础权重保持不变。无需更换模型或重建索引。仅把 audio 改回 1 并不等于回滚 v1；显式请求乘数也会继续覆盖相应默认值。

本轮原始响应及快照已从实验 worktree 迁至主仓库 `Eval/runs/`、`Eval/snapshots/`，迁移核对 441 个运行文件、25 个快照文件字节不变，并校验全部 20 个对象与冻结语料哈希。历史快照中的原始绝对路径保留为证据；复用现有数据库须显式指定迁移后的 `Eval/.runtime/ablation-v2/state.json`，或恢复到新隔离资源，方法见 [RUNBOOK](../Eval/RUNBOOK.md)。已归档的机器报告与指标没有改写；本次没有重新运行真实模型质量评估。

## 后续版本如何记录

每次采纳新参数，在本页追加一个版本，紧接其下记录实验、收益、回退与机制解释。保持旧版参数和证据不变；若修正结论，注明对应证据，避免覆盖历史。

采纳前，先提交不可变的选参计划与候选冻结记录，再检查留出的验证结果；保存旧版/候选的完整参数、模型与数据/标签/索引身份、二进制身份、逐题结果、重复响应哈希及可读取的原始证据或归档位置。仅保存在 ignored 运行目录、仅有口头结论或 `passed=true` 不足以证明质量改进。按已有 [RUNBOOK](../Eval/RUNBOOK.md)复跑与评分，无需另建评估框架。

后续优先补充独立人工金标、自然声音样本和新真实视频；选参前明确是否要求跳转及各题型不退步，同时检查视频召回、tIoU、jump、片段长度与负例。比较参数时固定同一索引与模型；改变媒体或语义标签就发布新 dataset 版本，跨索引结果另行说明。保留请求覆盖与回滚检查，避免只复现评估 runner 而遗漏生产入口。
