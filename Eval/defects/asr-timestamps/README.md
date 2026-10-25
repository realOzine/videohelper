# FunASR 相邻 VAD 段时间戳重叠导致整条转写 500

> 历史诊断报告：下文的失败位置、修复建议及未修复状态记录修复前的契约与现场，保留供追溯。生产修复已合入 `main` 的 `30efaef`：词起点保序、正时长且位于窗口内时允许重叠，句子边界使用词时间包络。`test_repro.py` 保留旧非重叠契约的诊断证据，并在新 adapter 上重放两个 token 与完整 frozen output，覆盖零及非零 offset、文本/token/时间戳保留和允许重叠；不代表真实重新索引或检索验收。

这是诊断交付，没有修改生产代码、重启服务、重试索引或改写基线数据库/对象桶。2026-10-25 在基线 `baseline-v1` 的两个失败视频上，用当前 adapter 和已缓存的 pinned CPU FunASR 1.3.14 重放，均复现 `500 internal: ASR word timestamps are invalid`。这是模型输出与 adapter 时间轴契约之间的兼容性缺陷；不是已建立的检索质量结论。

## 观察到的原始数据

位置以完整 `raw_text.split()` 的零基 token index 表示，时间单位均为毫秒。

| 视频 | 前一个 token | 当前 token | 重叠 | 位置 |
| --- | --- | --- | --- | --- |
| 3 `story_en.mp4`，108000ms | 105 `warmly` `[76830,77460]` | 106 `and` `[77400,77460]` | 60ms | 跨 sentence 13→14 |
| 4 `vlog_zh.mp4`，96000ms | 224 `的` `[86070,86460]` | 225 `是` `[86320,86620]` | 140ms | 同一 sentence 25 内 |

两个完整结果分别有 146、240 个 token，各只有这一个非法相邻重叠。token 数与 timestamp 数一致，sentence timestamp 拼接等于完整 timestamp，没有发现这两个结果中的零时长或全片边界裁剪问题。两次独立 CPU 重放复现了同样的冲突。

## 已证实的产生过程

`video-*-stages.json` 在 FunASR `AutoModel.inference()` 返回后、合并修改相对时间戳前，深拷贝 VAD 与每段 ASR 结果。将 VAD 按长度排序与 ASR 调用顺序配对，恢复原顺序后加 VAD offset，逐项重建的时间戳与 `video-*-raw.json` 完全相同；`test_repro.py` 验证这个过程。

* 英文相邻 VAD 段为 `[75420,77370]`、`[77370,80360]`。前段长度 1950，但 `warmly` 相对时间戳为 `[1410,2040]`，超过本段尾 90。下一段 `and` 相对 `[30,90]`。拼接后发生 60ms 重叠。
* 中文相邻 VAD 段为 `[80700,86410]`、`[86410,90680]`。前段长度 5710，但 `的` 相对 `[5370,5760]`，超过本段尾 50。下一段 `是` 相对 `[-90,210]`，早于本段头 90。拼接后发生 140ms 重叠。

这里 VAD 分段本身相邻且没有重叠。重叠来自各段 ASR 时间戳延伸到本段之外，随后被直接合并。`segment-trace.json` 是上述证据的精简索引。

安装依赖中 `funasr/auto/auto_model.py:752-768` 逐段给 timestamp 加 VAD 起点并 `extend`，没有按每段边界裁剪。`funasr/utils/timestamp_tools.py:37-55,62-64,97-105` 使用默认 `force_time_shift=-1.5`，在 Paraformer 调用的 `upsample_rate=1` 下帧步长是 60ms，并按帧数扩展末端；这解释了负起点和帧边界外延为何可出现。确切的两个外延数值来自上述真实 capture，未把静态分析当作单独的运行证明。

## 应用中的失败位置

* [`FunASRTranscriber.transcribe()`](../../../ml/app/adapters/funasr.py:139) 加载全片音频并调用 `generate()`；没有保留每段 VAD 边界用于归一化。
* [`transcript_from_result()`](../../../ml/app/adapters/funasr.py:35) 仅按全片窗口裁剪。第 39-40 行要求 `previous <= start < end`，因此真实重叠导致整次请求失败，即使前面的转写有效。`previous` 跨 sentence 保留，所以英文与中文两类冲突都会失败。
* [`transcribe()` 路由](../../../ml/app/api/routes.py:110) 第 112、116 行也验证句与词的时间单调性。只删 adapter 校验仍会在路由失败。
* [`现有测试`](../../../ml/tests/test_funasr.py:89) 覆盖了人工构造的 overlap 拒绝，没有覆盖真实相邻 VAD 段的帧级边界外延。

## 交给修复会话评估的建议

优先评估在 adapter 的统一时间轴转换处，对可解释的 VAD/帧边界外延做保守归一化，同时保留原始结果、所有 token、sentence 文本与既有路由校验。如果选择在合并前按各 VAD 段窗口裁剪，这两个观测案例将分别得到 `warmly.e=77370`、`的.e=86410`、`是.s=86410`；必须验证其他段的 token 不会因此变为零时长。若选择在合并后处理相邻重叠，应让边界在两词之间收敛，并同步跨句的句边界；对本报告最小 fixture，缩短前词末端到当前词起点即可保留两个正时长 token。如何限定允许修正的范围，需要修复会话结合模型帧机制评估，不宜仅为两个样本设一个任意容差。

不能只做 `start=max(start, previous_end)`：英文 `and` 的起止会同时变为 77460，仍然非法。不要丢弃 token、凭空补 1ms、删除验证或改变已冻结语料来绕过问题。对于倒序、明显异常、大重叠和无法形成正时长的情况，应继续明确报错。

验证应覆盖完整真实 frozen output、两个 token 最小 fixture、跨句和句内重叠、完全包含、负相对时间、非零请求 offset、整片末尾、真正倒序及丢 token；确认输出文本/token 保留且通过路由契约，再在保留旧基线证据后明确进行真实重索引。预处理语义改变时也应评估 `adapter_version` / fingerprint 的版本影响。

## 复现及证据范围

模型加载只构造 `FunASRTranscriber`，设置 `HF_HUB_OFFLINE=1`，没有构造 Registry 或加载另一份 WeMM。媒体仅从 scoped Eval bucket 下载到本工作树 `Eval/.work/asr-diagnostic/`，不提交媒体副本；摘要记录 normalized object key、媒体 SHA256、模型配置/fingerprint、生产 adapter SHA256 和代码版本。

```sh
ml/.venv/bin/python Eval/tools/diagnose_asr.py \
  --state Eval/runs/baseline-v1-index/state.json \
  --output Eval/defects/asr-timestamps-new
ml/.venv/bin/python -m pytest Eval/defects/asr-timestamps/test_repro.py -q
```

在独立 worktree 可使用主检出的 `ml/.venv/bin/python` 和 state 绝对路径。输出目录必须不存在。该工具固定只读视频 3、4，不启动服务、不写数据库或 bucket。最小 fixture 只保留相邻两个真实 token，并缩减 sentence 文本用于重现；它不是独立人工标注或完整识别质量判断。

未实施修复、未重新索引、未验证真实检索质量；当前 500 会继续阻止两个视频完成索引，需由修复会话评估并处理。
