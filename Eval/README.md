# VideoHelper Eval

当前真实运行结果见 [基线报告](BASELINE.md)：首轮索引验收发现 ASR 时间戳兼容性缺陷，已保留失败证据并交接评估，检索质量基线尚未形成。

这个目录集中保存新增的评估工具、视频、素材、标注、复核页面和评估结果。后续的 Eval / Evaluation 系统也放在这里；已有应用单元测试保持原位。

固定的小型合成视频库 `videohelper-synthetic-v1` 包含 **5 个视频，30 条有答案查询，5 条无答案查询，总长 8 分 18 秒**。真实索引验收、快照保存/恢复、固定索引检索和评分工具已实现；执行方法、结果判读与数据保留规则见 [基线运行说明](RUNBOOK.md)，接口与验收边界见 [评估契约](CONTRACT.md)。工具已实现不代表真实模型质量基线已经通过；以具体运行报告为准。

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

每个视频有 6 个场景。统一编码为 1280×720、24 fps、H.264 / yuv420p、AAC 48 kHz 单声道。视频文件合计约 72.9 MiB；源音频、原图和中间文件会额外占用空间。时长覆盖 90 秒以下、等于 90 秒和超过 90 秒，便于后续观察切片边界。

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

## 运行评估

[RUNBOOK.md](RUNBOOK.md) 给出可直接执行的命令：先用 `tools/run.py index` 单独验收真实上传和索引并冻结快照，再用 `retrieve` 对同一索引执行两次完整检索，通过 `tools/score.py compare` 比较。`restore` 可将数据库和对象恢复到新的独立资源，再验证索引指纹。

每轮都搜索全部 5 个视频，执行 35 条查询的 `visual`、`speech`、`fused` 三种模式；融合包含当前 active 的 `audio`，本库没有独立声音金标。查询、标注、字幕和参考讲解不作为视频内容导入。真实失败保留在报告中；不改本版标签或排除失败题来提高分数。

## 目录与边界

```text
Eval/
  assets/          原始分镜、语音、配乐、提示词与来源说明
  videos/          固定 MP4 成片
  annotations/     脚本、查询、时间标注、外部字幕
  cmd/evalbridge/  复用业务搜索与索引检查的 Go JSONL 桥接
  tools/           合成、校验、预览、运行、评分工具及测试
  deploy/          独立 Eval Redis 配置
  review/          复核页面、成片代表帧、校验报告
  runs/            每轮索引证据、检索原始响应与评分报告
  snapshots/       冻结索引元数据、数据库备份与对象归档
  .runtime/        已忽略的隔离运行配置、状态与临时文件
  bin/            已忽略的评估二进制
  .work/           已忽略的合成中间文件
```

素材由内置 ImageGen、代码绘图、macOS 系统 TTS 和原创程序合成配乐构成，详见 [素材来源](assets/SOURCES.md) 与 [图像生成提示词](assets/image-prompts.json)。未下载外部图库或歌曲。

这是生成静态分镜后用镜头缩放、语音和配乐合成的视频，不是视频模型生成的真实连续动作。它能帮助启动导入、索引、检索与定位回归，但不能代表真实 vlog、动作识别、口音/噪声、多说话人、自然环境音、长视频、并发压力或系统稳定性的完整验收。配乐也不是瀑布、风声等实录音效。以后增加真实样本和评估工具时，继续放在 `Eval/`，并保留本版数据。
