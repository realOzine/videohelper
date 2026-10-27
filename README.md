# VideoHelper Desktop

个人使用的视频检索桌面 App：导入本地视频或链接，用文字找到画面、转录和声音，再跳转播放。使用 MyGo、Go、React、SQLite + sqlite-vec，优先支持 Apple Silicon Mac。

打开即用，没有账号、租户、JWT、套餐、PostgreSQL、Redis、S3 或 Docker。项目只是本地视频分组。界面采用 `Design/Icon/` 的墨黑、米纸色与红色播放头，图标和字体随程序携带。

## 构建与运行

开发需要 Go 1.27.3、Node.js 24+ 与目标系统 C 编译工具链。macOS 安装 Xcode Command Line Tools 后执行：

```sh
npm --prefix web ci
make desktop
./bin/VideoHelper
```

`make build` 同样构建桌面应用。macOS 产物 `bin/VideoHelper` 是可单独复制运行的主二进制；`.local/desktop-build/` 下还保留 MyGo 的系统应用包装。构建内嵌前端、字体、模型服务源码与固定依赖清单。当前 Mac 构建为本机架构、最低 macOS 13，使用本地签名，尚未做 Developer ID 公证。

Mac 单文件首次启动会在数据目录的 `Launcher/VideoHelper.app` 恢复应用包装，并交给系统启动，以获得正常的 Dock、窗口和文件选择器身份。终端中的启动器随后退出，桌面窗口继续运行；关闭窗口或按 `⌘Q` 退出应用。更新包装时原子替换，失败保留旧版本，不要求用户安装签名工具。

首次运行会在后台下载私有 Python、FFmpeg/ffprobe、yt-dlp、Deno、模型依赖与固定版本权重。用户无需手工安装这些组件。窗口显示准备阶段、实际推理设备和失败重试入口；资料库与已有视频播放不依赖模型准备完成。后续运行复用缓存。

主程序单文件交付不等于运行时不创建文件：数据库、原片、模型和释放的运行环境均在应用数据目录中。

## 使用

1. 在资料库选择本地文件，或粘贴视频链接。导入复制原片，不改动用户源文件；模型准备期间先持久排队。
2. 后台完成探测、播放副本、缩略图、转录及三通道向量索引。任务可以取消、重试；退出后可恢复未完成任务。
3. 在全部视频、项目或单个视频中输入描述，点击命中片段定位播放。
4. 删除视频先进入“已删除”，可恢复。界面没有自动清空原片的后台策略。

默认搜索权重为画面 `1`、转录 `1`、声音 `0`；声音已经建索引，需在通道设置中主动开启召回。查询文字原样传入模型，不自动翻译。

WeMM 根据实际 Torch 运算自动选择 MPS → CUDA → CPU；ASR 与 CLAP 当前使用 CPU。不同后端的索引身份隔离，切换后不能混用旧向量。需要重建时从视频详情发起重新索引，原片保留。

## 数据与运行边界

默认使用系统用户配置目录下的 `VideoHelper`：macOS 为 `~/Library/Application Support/VideoHelper`；Windows 为 `%APPDATA%/VideoHelper`；Linux 为 `$XDG_CONFIG_HOME/VideoHelper` 或 `~/.config/VideoHelper`。开发和验证可用 `VIDEOHELPER_DATA_DIR` 指向独立目录。

- `library.sqlite`：视频、项目、任务、转录和向量。
- `media/`：受管原片、播放副本、缩略图与原始转录结果。
- `models/`：固定 revision 的模型权重缓存。
- `runtime/`：版本隔离的私有工具、Python 和模型依赖环境。
- `Launcher/`：macOS 单文件启动时恢复的应用包装。
- `desktop.log`：本地应用日志。

备份数据库前先退出应用；原片与数据库一起保存，模型和工具缓存可重新准备。运行环境包含绝对路径，不直接搬运旧虚拟环境到新目录。

跨平台实现与实际验收分开：Apple Silicon macOS 是当前验证目标；Windows x64、Linux x64 已提供运行时配置，但仍需目标机 GUI、音视频和真实推理验收。Linux 需要系统 GTK/WebKitGTK，Windows 需要 WebView2。Intel Mac 的当前 Torch / Transformers 组合没有兼容发行，应用会明确报错，不会假称已回退到可用 CPU。

## 验证

```sh
make test-go
make test-web
make vet
```

Apple Silicon Mac 已使用真实模型完成六秒视频的原生文件导入、yt-dlp 本地 HTTP 链接导入、三通道索引、中文搜索、视频播放、项目分组、删除恢复与重启保留验证。模型权重复用了缓存；这个短样本不代表冷安装、长视频性能或检索质量验收。 GUI 自动化中出现过视频区域黑屏、缩放窗口后恢复的现象，尚未区分窗口合成与捕获问题。

真实工具与模型验证为显式 opt-in；命令和证据边界见 [桌面运行时](docs/desktop-runtime.md)。假模型测试只验证接线、事务、取消和恢复。旧 PostgreSQL 后端的检索指标不能作为 SQLite 后端质量证明；冻结语料与历史证据保留在 [Eval](Eval/README.md)。所有构建和测试排除 `model-test/`。

产品边界见 [设计](docs/design.md)，模块与接口见 [实现说明](docs/spec.md)。此前服务端实现可在 Git 历史 `cc1fb38` 查看；桌面分支不再维护其部署入口。
