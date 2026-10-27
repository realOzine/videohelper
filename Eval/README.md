# VideoHelper Eval

历史服务器真实运行结果见 [基线报告](BASELINE.md)：`baseline-v1` 索引验收失败、ASR 修复后 `baseline-v2` 的真实索引验收、重复检索与恢复验证均有历史结论。保留[基线汇总](baselines/baseline-v2.json)供比较；原始报告可从提交 `11a6668` 查看，旧运行资源与快照已清理。

这个目录集中保存评估工具、视频、素材、标注、复核页面和必要结论。后续的 Eval / Evaluation 系统也放在这里；已有应用单元测试保持原位。

固定的小型合成视频库 `videohelper-synthetic-v1` 包含 **5 个视频，30 条有答案查询，5 条无答案查询，总长 8 分 18 秒**。旧服务器版本实现过真实索引验收、快照保存/恢复和固定索引检索；当前保留独立评分工具。当前命令、结果判读与数据保留规则见 [基线运行说明](RUNBOOK.md)，接口与验收边界见 [评估契约](CONTRACT.md)。工具已实现不代表真实模型质量基线已经通过；以具体运行报告为准。

固定模型参数消融已形成独立的[对比报告与复跑流程](experiments/ablation-v1/REPORT.md)：41 组参数及两次重复运行，候选改善定位，全量跳转仍未改善。机器可读对照保存在 `baselines/ablation-v1-default.json` 和 `baselines/ablation-v1-selected.json`。实验当时未修改生产默认参数；当前已采用 K=5、RRF=60、merge gap=1、融合乘数 visual/speech/audio=1/1/0，采用依据与边界见[评估说明](../docs/Evaluation.md)。历史报告和分数保留原样。

## 先看成片

打开 [视频与标注复核页](review/index.html)，点击每条描述可跳到对应位置。推荐在仓库根目录启动支持视频分段读取的本地预览：

```sh
ml/.venv/bin/python Eval/tools/serve.py
```

浏览器访问 `http://127.0.0.1:18766/review/index.html`，结束后按 Ctrl-C。服务只绑定本机、只提供 `Eval/` 下的文件。不要用普通 `python -m http.server` 验证跳转，它不支持这里所需的 HTTP Range。也可以自行用浏览器打开 HTML 或播放器打开 MP4。

| 视频 | 内容与语言 | 时长 |
| --- | --- | --- |
| [把学习变成六个小步骤](videos/study_zh.mp4) | 中文学习讲解；桌面、计时器、代码练习 | 1:24 |
| [理解区块链：从副本到信任边界](videos/blockchain_mix.mp4) | 中英文混合教程；账本、哈希、共识、私钥 | 2:00 |
| [Milo and the Little Lantern](videos/story_en.mp4) | 英文绘本故事；狐狸、兔子与提灯 | 1:48 |
| [海边小城的一天](videos/vlog_zh.mp4) | 中文旅行 vlog；早餐、单车、地图、海港 | 1:36 |
| [Six Quiet Landscapes](videos/landscape_en.mp4) | 英文旁白与配乐；雪山、瀑布、沙丘、竹林、花田、极光 | 1:30 |

每个视频有 6 个场景。统一编码为 1280×720、24 fps、H.264 / yuv420p、AAC 48 kHz 单声道。视频文件合计约 72.9 MiB；保留的源音频和原图会额外占用空间。时长覆盖 90 秒以下、等于 90 秒和超过 90 秒，便于后续观察切片边界。

## 标注是什么

- [queries.jsonl](annotations/queries.jsonl)：评估程序应读取的 35 条标注。
- [queries.md](annotations/queries.md)：便于阅读的查询、时间范围和讲解依据。
- [manifest.json](manifest.json)：稳定的视频 ID、文件 SHA-256、场景与实际合成时间线。
- [blueprint.json](annotations/blueprint.json)：原始脚本、分镜、声音配置及随机种子。
- `annotations/*.srt`：脚本对应的外部字幕。原片没有烧录讲解字幕，也没有内嵌字幕流。

30 条正例包含 **10 条画面、10 条讲解、10 条画面与讲解组合**，每条指向一个明确视频与时间范围。另有 5 条整个视频库中不存在的内容，`answerable=false`、`targets=[]`。所有查询都应搜索完整的 5 个视频；不要提前用目标视频过滤搜索范围。

时间均为相对视频起点的整数毫秒，证据区间采用 `[start_ms, end_ms)`。画面查询覆盖整个场景；讲解与组合查询按实际生成的语音文件长度定位，而非仅凭文字估算。中英文分段语音间留有短暂停顿。

`acceptable_jump_ms` 是初始跳转窗口，取证据起点前 1 秒到后 2 秒并限制在场景内；它是启发式草案，人工复核后再确定是否适合作为评分规则。SRT 和讲解文本来自合成脚本，不是独立 ASR 结果，也不是逐词强制对齐。

当前全部 `human_reviewed=false`。助手已检查 30 个成片场景代表帧；独立人工观看、听辨和标注仍待后续处理。不能把脚本、助手帧复核或结构校验说成人工金标。

## 校验与复用

在仓库根目录执行：

```sh
ml/.venv/bin/python Eval/tools/validate.py
ml/.venv/bin/python -m unittest discover -s Eval/tools -p 'test_*.py' -v
cd Eval
shasum -a 256 -c checksums.sha256
```

`validate.py` 会核对视频哈希、完整解码、流与编码信息、帧数、时长、音频信号，以及查询与时间范围的结构约束。报告在 [review/validation.json](review/validation.json)。音频信号通过不等于逐句听辨正确。

`checksums.sha256` 只冻结素材、标注和成片等语料文件；评估工具和报告可以独立演进，不属于该校验清单。

本次复用了项目已有 Python 环境，未安装新依赖。若需要隔离环境，将虚拟环境建在 `Eval/.venv/`，使用 [requirements.txt](requirements.txt)；之后将命令中的解释器替换为 `Eval/.venv/bin/python`。视频验证还需要 PATH 中的 `ffmpeg` / `ffprobe`。

`tools/build.py` 保存了完整合成流程。**成片已经生成，无须再次运行**；检测到 `manifest.json` 时会拒绝覆盖。重建还需要 macOS 的 `say`、Tingting / Samantha 语音和系统 Arial 字体。原图、语音和配乐源文件均已保存，使用现有视频不依赖生成服务。系统语音、编码器或依赖版本改变时，重建不保证字节一致；回归测试应固定当前文件与哈希。

## 桌面版与历史评估

当前产品是单用户桌面 App，使用 SQLite、sqlite-vec、受管本地文件与进程内任务。原依赖 PostgreSQL、Redis、S3 的 `evalbridge`、索引 runner、快照恢复 runner 已移除；不为桌面版保留服务器测试环境。

现有 [基线](BASELINE.md)、`baselines/`、`analysis/`、`experiments/` 和 `defects/` 是旧服务器实现的历史证据。**桌面版尚未完成同语料的真实模型质量评估，不能继承旧服务器分数或索引验收结论。** 复现旧实现时，应在独立工作树检出相应报告记录的历史代码版本，并使用该版本的 RUNBOOK；不要在当前桌面数据目录恢复旧数据库或对象快照。

固定视频、35 条查询、标签、字幕、素材哈希和复核证据保持原样。`tools/score.py` 与 `tools/ablate.py` 只读取保存的结果进行评分和比较；`tools/validate.py` 检查固定媒体与标注结构，`tools/serve.py` 提供本地复核预览。当前可执行命令见 [RUNBOOK.md](RUNBOOK.md)。它们均不证明桌面版的真实模型检索质量。

## 目录与边界

```text
Eval/
  assets/          原始分镜、语音、配乐、提示词与来源说明
  videos/          固定 MP4 成片
  annotations/     脚本、查询、时间标注、外部字幕
  tools/           独立的合成、校验、预览、评分及历史结果消融工具
  review/          复核页面、成片代表帧、校验报告
  baselines/       保留的机器可读基线汇总
  analysis/        检索质量结论与后续实验建议
  experiments/     冻结实验方案、参数消融和比较结论
  defects/         缺陷结论、回归测试及所需 frozen ASR fixtures
```

历史 `runs/`、`snapshots/`、`.runtime/`、`bin/` 和 `.work/` 的旧内容已清理；当前分支不再提供生成或恢复服务器快照的入口。旧 Eval 数据库和对象桶也不再保留；历史报告可用 `git show 11a6668:Eval/runs/baseline-v2-a/report.md` 查看。`defects/asr-timestamps/` 中最小复现与两个视频的 raw/stages JSON 是测试输入，不能按日志清理；其用途见[缺陷说明](defects/asr-timestamps/README.md)。

素材由内置 ImageGen、代码绘图、macOS 系统 TTS 和原创程序合成配乐构成，详见 [素材来源](assets/SOURCES.md) 与 [图像生成提示词](assets/image-prompts.json)。未下载外部图库或歌曲。

这是生成静态分镜后用镜头缩放、语音和配乐合成的视频，不是视频模型生成的真实连续动作。它能帮助启动导入、索引、检索与定位回归，但不能代表真实 vlog、动作识别、口音/噪声、多说话人、自然环境音、长视频、并发压力或系统稳定性的完整验收。配乐也不是瀑布、风声等实录音效。以后增加真实样本和评估工具时，继续放在 `Eval/`，并保留本版数据。
