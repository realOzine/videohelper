# 素材来源与生成记录

本库在 2026-10-25 为 VideoHelper 的合成评估任务制作。没有下载外部图库、公开视频、商业歌曲或真人配音；未调用专用视频生成模型。

| 文件 | 来源 | 用法 |
| --- | --- | --- |
| `images/study-sheet.png` | 内置 ImageGen，新图生成模式，提示词 key=`study` | 学习讲解的 6 格分镜 |
| `images/story-sheet.png` | 内置 ImageGen，新图生成模式，提示词 key=`story` | 英文绘本的 6 格分镜 |
| `images/vlog-sheet.png` | 内置 ImageGen，新图生成模式，提示词 key=`vlog` | 旅行 vlog 的 6 格分镜 |
| `images/landscape-sheet.png` | 内置 ImageGen，新图生成模式，提示词 key=`landscape` | 风景的 6 格分镜 |
| `images/blockchain-1.png` 至 `blockchain-6.png` | `tools/build.py` 使用 Pillow 绘制 | 账本、链接、记录修改、共识、私钥、来源真实性示意图 |
| `voice/<media_id>/sN_P.txt` | 本库原创脚本 | 每个语音片段的输入文字 |
| `voice/<media_id>/sN_P.aiff` | macOS `say` | 系统 TTS 原始输出 |
| `voice/<media_id>/sN_P.wav` | FFmpeg 转码 | 48 kHz 单声道 PCM，用于测量语音长度 |
| `voice/<media_id>.wav` | 合成脚本 | 按时间线拼接并调整音量的完整纯讲解轨 |
| `music/<media_id>.wav` | NumPy 正弦波叠加与包络生成 | 原创固定音符琶音，无歌曲采样 |

完整图像提示词保存在 [image-prompts.json](image-prompts.json)，可审查每个画面最初要求。实际生成图与要求可能存在差异，应以已保存原图和成片为准。每张图由 2 列 × 3 行构成；合成按阅读顺序裁切，加轻微镜头缩放。区块链图中的英文标题属于教学画面；没有烧录旁白全文。

中文使用 `Tingting`，语速参数 190；英文使用 `Samantha`，语速参数 165。中英混合视频按语言拆分发声。配乐随机种子为 `20261025 + 视频序号`（从 0 开始），固定音阶与音量，生成算法见 `tools/build.py`。字体引用 macOS 系统 Arial，字体文件没有复制进数据集。

成片与原始素材的固定哈希在 `../checksums.sha256`；平台和生成脚本哈希在 `../manifest.json`。素材在本任务中生成不等于这里为生成服务、系统语音或字体授予新的授权；本文件不作 CC0、公共领域或第三方素材授权声明。
