# 修复会话的只读评估

2026-10-25，按用户要求向 Codex 会话「videohelper-包工头」（`01a16e2e-d6ff-73e3-857d-9fd87eede2d1`）发送本目录的原理、复现过程、涉及代码与修复建议。该会话返回以下只读评估；这是建议与其报告的实验结果，尚不是已实施修复或本评估主流程的重跑结论。

## 时间戳处理

对方报告：4 项 frozen 复现测试通过，在内存中按 VAD 段裁剪完整 raw 后，146 / 240 个 token 和全部句文本保留，offset=0 / 4000 两种情形通过现有时间轴校验。统一裁剪会改变 18 / 10 个词边界，并非只处理两个冲突词。

建议在单段 ASR 返回后、FunASR 添加 VAD offset 和合句前，按实际切片时长收敛时间戳。现有推理入口可以取得 `speech_j`，应优先复用而不是重写 VAD / 标点流程。保留全部 token 和严格校验；零时长、倒序和不可解释越界仍应失败，不能仅凭两个样本设置任意通用毫秒容差。

## 错误分类与版本

建议为确定的模型输出契约失败使用 HTTP 500 和明确代码 `model_output_invalid`，由 Go 白名单映射为 Internal、`Retryable=false`，让已有 pipeline 终止自动重试；普通 `500 internal` 和临时网络错误保留原策略。不用消息文本猜错误类别，不将模型契约问题标成媒体损坏的 422。

实施时需同步 Python schema / routes / adapter、Go `responseError`、契约文档和测试；`preprocess.adapter_version` 应升级。现有 ASR stage 固定旧 fingerprint，Retry 保留 plan/progress，升级后重试旧任务会触发 `asr_changed`。应保留 `baseline-v1`，在新隔离 Eval run 建立新计划验收，不覆盖旧指纹或混用旧断点。

## 当前状态

本会话和对方均未实施生产修复、切换根分支或重启现有模型服务。当前失败证据、数据库和对象桶保留；完整 5 视频检索评分与恢复验收等待实现缺陷处理后进行。后续不能把此只读评估当成已通过的基线。
