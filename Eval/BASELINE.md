# 当前基线

2026-10-25 的真实模型基线 `baseline-v1` **未通过索引验收**：5 个视频完成真实上传，3 个 ready，29/35 个必需阶段完成。英文故事与中文 vlog 的 ASR 返回可复现的时间戳重叠错误。详细状态、向量覆盖和版本身份见 [可对比报告](runs/baseline-v1-index/report.md) 及 [机器可读报告](runs/baseline-v1-index/report.json)。

[完整运行流程](RUNBOOK.md) 已提供独立索引验收、固定快照检索、逐题评分、两轮比较、快照恢复及恢复后验证。当前被测实现阻止后续阶段；没有把不完整索引冻结成合格快照，也没有伪造检索指标。

缺陷原理、原始模型输出、涉及代码、最小复现和修复建议保存在 [ASR 缺陷报告](defects/asr-timestamps/README.md)，已按用户要求发送给「videohelper-包工头」，其 [只读评估结论](defects/asr-timestamps/assessment.md) 已收录。当前生产代码未修改；修复确认后，以新 run ID 重跑并保留本次失败基线。

评估工具已通过 38 项 Python 单元测试、Go 桥接 race 测试与 vet；4 项 frozen 输出测试确认了缺陷复现。索引验收与质量达标是不同结论，不能将这些工具测试替代真实检索指标。临时 worktree 均在合并后移除；本轮 worker 与独立 Eval Redis 已停止，Redis 持久化数据、数据库及桶保留。
