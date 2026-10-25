# 运行范围更正

本次调用未传 `--state`，因此实际读取原始 `baseline-v2` 数据库与桶，是原库的额外重复检索。目录名不能作为恢复验证证据。原始结果保持不变。

快照恢复本身已成功，但恢复后检索应以另一次显式传入 `Eval/.runtime/baseline-v2-restored/state.json` 的 `baseline-v2-restored-verified` 及其 metadata.runtime 为准。
