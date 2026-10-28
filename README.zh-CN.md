<p align="center">
  <img src="Design/Icon/png/videohelper-app-icon-256.png" width="96" height="96" alt="VideoHelper 应用图标">
</p>

<h1 align="center">VideoHelper</h1>

<p align="center"><strong>用一句话，找到视频里的那一刻。</strong></p>

<p align="center">
  <a href="#快速开始">快速开始</a> ·
  <a href="#功能">功能</a> ·
  <a href="#工作方式">工作方式</a> ·
  <a href="#文档">文档</a> ·
  <a href="README.md">English</a> · 简体中文
</p>

VideoHelper 是个人使用的视频检索桌面应用，可以搜索画面、台词和声音。导入本地文件或视频链接，用文字描述记得的内容，点击命中片段即可定位播放。

资料库、媒体和模型推理都保留在你的电脑上，无需账号或自行部署后端服务。

## 为什么使用 VideoHelper？

- 从素材中寻找一个画面，省去逐个文件拖动进度条。
- 查找中文采访或演讲视频中的一句话，连同上下文一起播放。
- 在一个项目的素材中寻找掌声、海浪等声音。

## 快速开始

**目前已验收的平台是 macOS 13 及以上的 Apple Silicon Mac。** 暂不支持 Intel Mac；Windows 和 Linux 实现尚未完成实机 GUI 与模型验收。

当前还没有发布可下载的二进制版本。源码构建需要 **Go 1.27.3**、**Node.js 24+** 和 C 编译工具链；macOS 使用 Xcode Command Line Tools。

```sh
git clone https://github.com/realOzine/videohelper.git
cd videohelper
npm --prefix web ci
make desktop
./bin/VideoHelper
```

`bin/VideoHelper` 是可复制到兼容 Mac 上运行的单个主程序。构建内嵌界面、字体和模型服务源码，**不包含已安装的模型及其运行依赖**。Mac 构建使用本地签名，尚未完成 Developer ID 公证。

### 首次启动

VideoHelper 会将私有 Python、媒体工具、模型依赖和权重下载到应用数据目录。**首次准备需要联网、依赖与模型下载源可达，并预留足够磁盘空间。** 不需要自己安装 Python、FFmpeg 或 yt-dlp；之后启动会复用缓存。

窗口会显示准备进度和实际设备。模型加载期间仍可浏览资料库、播放已准备好的视频；新导入的任务会持久排队等待。

macOS 上，单文件会先在数据目录恢复一个小型 `.app` 包装，再交给系统启动。终端中的启动器随后退出，应用窗口继续运行；关闭窗口或按 `⌘Q` 退出。

1. 打开**资料库**，选择本地视频或粘贴视频链接。
2. 等待处理完成，在搜索框描述画面、台词或声音。
3. 打开命中片段播放；也可以通过项目或单视频搜索缩小范围。

## 功能

- **三路检索。** 默认启用画面和转录；声音也会建索引，需要按声音查找时在**模型**中开启。查询文字原样传入模型，不自动翻译。
- **本地文件与视频链接。** 使用系统文件选择器，或通过 yt-dlp 导入链接。本地导入会复制原片到资料库，不改动源文件；链接下载能力取决于来源站点。
- **可直接播放的命中。** 结果包含时间戳、缩略图，以及可用的转录摘要；可以跳转命中、切换片段或拖动时间轴。
- **便于整理的资料库。** 按项目分组，在全库、项目或单个视频中搜索，删除后可以恢复。
- **可恢复的任务。** 查看处理阶段、取消或重试任务，重启应用后继续未完成的工作。
- **中英文界面。** 两种界面语言共用同一资料库；当前语音转写模型支持中文。

## 工作方式

```text
本地视频 / 视频链接
        ↓
受管原片 + 播放副本 + 缩略图
        ↓
画面 · 中文转录 · 声音索引
        ↓
文字查询 → 命中片段 → 定位播放
```

桌面界面使用 [MyGo](https://github.com/egoist/mygo)、Go 和 React。SQLite 保存资料库与任务状态，sqlite-vec 检索向量。WeMM-Embedding-2B 编码视频和转录文本，FunASR 转写中文语音，CLAP HTSAT unfused 编码声音；转写与声音分析都保留原始音频。

WeMM 根据实际运行检测按 MPS → CUDA → CPU 选择设备；ASR 和 CLAP 当前使用 CPU。自动检测不代表未验收平台已经可用，也不代表性能保证。设备选择、下载与平台依赖详见[运行时说明](docs/desktop-runtime.md)。

macOS 默认数据目录为 `~/Library/Application Support/VideoHelper`，可通过 `VIDEOHELPER_DATA_DIR` 指定其他目录。单文件交付仍会创建数据库、受管媒体、模型缓存和私有运行时文件。备份时先退出应用，将数据库和媒体一起保存。

## 文档

详细文档目前以中文提供：

- [产品设计](docs/design.md)：产品范围、桌面体验与数据边界。
- [实现说明](docs/spec.md)：模块、本地接口、任务与检索。
- [运行时安装](docs/desktop-runtime.md)：下载、平台、组件许可与显式模型测试。
- [检索评估](Eval/README.md)：冻结语料与历史证据；旧服务端结果不能证明桌面版的检索质量。
- [Cloud / 服务端分支](https://github.com/realOzine/videohelper/tree/cloud)：此前的服务端实现。

开发检查可运行 `make test-web`、`make test-go` 和 `make vet`。Apple Silicon Mac 已验收真实模型导入、索引、搜索、播放与持久化；这些验证不能证明冷安装可靠性、长视频性能或检索质量。
