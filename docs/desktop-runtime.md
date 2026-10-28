# 桌面私有运行时

桌面入口由一个 Go 二进制提供，首次联网下载组件、依赖与模型，并在用户数据目录中释放文件和启动私有子进程。它不是“永不释放文件的单进程”，也不是“首次启动即可完全离线”。前端资源和 ML 公共服务、插件、profile 清单与锁文件嵌入交付物；用户无需安装 Python、uv、FFmpeg、yt-dlp 或 Deno。

macOS 单文件会先恢复数据目录内的 `Launcher/VideoHelper.app` 并交给系统启动，保留原构建签名，不在用户机器执行 codesign。终端启动器随后退出，窗口关闭、`⌘Q` 或向实际 GUI 进程发送 SIGTERM 才触发模型回收。本地签名完整性检查不替代 Developer ID 公证。

`internal/desktop/runtime.Manager` 接收数据目录与打包的 `fs.FS`。UI 可先显示 `State()`；后台依次调用 `Ensure(ctx, progress)`、`Start(ctx)`，完成后使用 `Services` 的内部端点、随机令牌与实际空间 ID。准备失败、进程退出或未完成加载时不得显示模型已就绪；`Close(ctx)` 只终止本次创建的进程树。客户端终止 HTTP 请求不能中断已执行的 Torch 推理线程；桌面退出时通过终止自己托管的 ML 进程结束推理。

## 首次启动与重复启动

1. 根据打包资源内容生成隔离目录 `runtime/ml-<digest>`，保留 `ml/`、`plugins/`、`profiles/` 的相对关系。
2. 按 `internal/desktop/runtime/components.json` 校验私有工具缓存；缺失或损坏时才下载固定组件。下载写私有 staging，限制体积，核验发行文件 SHA256，提取固定目标文件后才发布。损坏的已安装文件不会直接执行；安装锁使用 OS 文件锁，失败或取消只清理本次 staging。校验后的 FFmpeg 和 ffprobe 以本体哈希组合发布到同一个私有 `media-<digest>/` 目录，优先硬链接，跨文件系统时复制；yt-dlp 的 `--ffmpeg-location` 指向这个目录，不依赖系统 PATH 发现 ffprobe。
3. 首先以固定 uv 0.13.6 执行 `python find --managed-python --no-python-downloads` 检查 CPython 3.12.15；已存在则直接复用，未找到时才使用随包 `python-downloads.json` 安装 build 20261021。该目录来自同版 uv 的上游元数据，含各平台准确 URL 与 SHA256；不修改用户 PATH、Python 可执行入口或 Windows 注册表。
4. profile 使用自己的私有 `.venv`。首先按原有同步参数执行 `uv sync --frozen --no-dev --no-default-groups --no-editable --check --offline`，命中则复用现有环境；检查不通过时才去掉 `--check --offline` 安装或修复，不依赖用户的 Python 环境。各依赖版本、下载地址与哈希来自该 profile 的 `uv.lock`。没有 NVIDIA 候选的 Linux/Windows 使用 CPU wheel；CUDA 候选才安装 cu130 wheel。
5. 首先离线 `prepare` 检查固定模型缓存；失败且未取消时记录原始原因，再联网尝试补齐固定 revision 的权重。若联网准备也失败，返回的诊断同时保留离线与在线失败。取消检查不会触发安装或联网回退。`HF_HOME` 固定为数据目录下的 `models/`；生产明确 `ML_FAKE=0`。WeMM 的固定修订与远程代码哈希检查继续生效。
6. 两个 ML 服务仅绑定 loopback 随机端口，使用本次随机内部令牌。以 `HF_HUB_OFFLINE=1` 加载，等待 `/readyz` 并核对 `/v1/spaces` 后才向处理器提供端点。

`checking` / `checking_models` 表示检查缓存，`python` / `packages` 只在实际进入安装分支时发出，`models` 只在联网补齐分支发出；完整暖缓存启动不经过这三类安装/下载阶段。之后的 `loading` 仍会把本地权重加载到内存和设备，不代表重新下载。此改动不改变资源版本目录、依赖锁、模型 fingerprint 或私有缓存位置，也不自动搜索、导入其他 Hugging Face 缓存。

运行时与模型缓存独立于媒体与数据库。Python 仅读取 `ML_LOCAL_MEDIA_ROOT` 中的受管本地文件，S3 downloader、Boto3 与远端媒体缓存已移除；路径解析不复制文件，Go 在所有推理退出前保持输入存活。版本升级保留旧运行时；当前没有自动清理旧版本策略。首次安装需网络、足够磁盘空间和依赖源可达；下载失败可以重新启动后重试。

## 后端与索引身份

`ml/app/hardware.py --cuda-candidate` 仅作为 Linux/Windows 安装候选提示，调用 CUDA Driver API 检查初始化、设备数和 CUDA 13 驱动支持，不依据 `nvidia-smi` 或 OS 判定最终后端。安装后执行真正 Torch 张量分配、矩阵运算、回读与有限值校验：MPS 可运行则选择 MPS，否则尝试 CUDA，再回退 CPU。MPS 宣告可用但实际操作报错时仍回退；CUDA 安装或 Torch 启动失败也尝试 CPU profile，并在运行状态显示原因。

| 后端 | 配置清单 | 空间 ID |
| --- | --- | --- |
| MPS | `wemm-mps/models.yaml` | `wemm-2b@r1` |
| CPU | `wemm-cpu/models.yaml` | `wemm-2b-cpu@r1` |
| CUDA | `wemm-cuda/models.yaml` | `wemm-2b-cuda-experimental@r1` |

同维度不能证明空间兼容。切换后端使用该清单的原始 fingerprint 与独立索引，不能沿用另一个后端的向量。设备小张量检测不证明完整 WeMM 模型可装入显存，也不证明长视频速度或检索质量。ASR/CLAP 继续使用各自 CPU 清单，查询文本不翻译。

配置与组件覆盖 macOS arm64、Linux x64、Windows x64；Windows/Linux 的模型运行仍需各自实机验收。Intel Mac 的固定 Torch 2.14.1 没有相应 wheel；PyPI 上的 Torch 2.2.2 虽有 Intel Mac CPython 3.12 wheel，但 Transformers 5.2 要求 Torch >=2.4，故不能通过简单降级声称兼容。当前 Intel Mac 明确报告此限制，后续需独立验证编译版 Torch 或其他模型后端，不更改已有空间身份。

## 组件来源与许可

| 组件 | 固定来源 | 分发说明 |
| --- | --- | --- |
| uv | [astral-sh/uv 0.13.6](https://github.com/astral-sh/uv/releases/tag/0.13.6) | Apache-2.0 / MIT；CPython 构建另含 Python 与其第三方依赖许可 |
| Python | [python-build-standalone 20261021](https://github.com/astral-sh/python-build-standalone/releases/tag/20261021) | 使用固定构建及校验和；保留发行包内的许可文件 |
| yt-dlp | [yt-dlp 2026.10.26](https://github.com/yt-dlp/yt-dlp/releases/tag/2026.10.26) | 源码 Unlicense；此处选官方 PyInstaller 独立包，其整体含 GPLv3+ 代码，须遵守其第三方许可，不能只标 Unlicense |
| Deno | [denoland/deno v2.9.8](https://github.com/denoland/deno/releases/tag/v2.9.8) | MIT 与其第三方 notices；作为 YouTube EJS 的私有 JS runtime |
| FFmpeg / ffprobe（macOS） | [Martin Riedl 9.0.2 arm64 release](https://ffmpeg.martin-riedl.de/download/macos/arm64/1789931890_9.0.2/versions.txt)；amd64 同版固定构建 | 第三方静态构建、GPLv3+，固定 ZIP 和 SHA256；保存 GPLv3 文本及各库版本/配置，不使用 latest 地址 |
| FFmpeg / ffprobe（Linux / Windows） | [eugeneware/ffmpeg-static b6.1.1](https://github.com/eugeneware/ffmpeg-static/releases/tag/b6.1.1) | 第三方 GPL 构建，实际 FFmpeg 版本可能因平台不同；保存该平台发行的 LICENSE、README |

FFmpeg 不能统一标作 LGPL：此功能需要 `libx264`，对应启用 GPL 的构建；macOS 选定构建配置为 `--enable-gpl --enable-version3`，无 `--enable-nonfree`；本机验证本体 `-L` 为 GPLv3+。Windows 构建 README 明示 GPLv3；所有平台在启动前执行 `-L`，拒绝自报不可再分发的二进制。若发行最终 App 包含这些组件，发行流程必须同时整理准确许可、第三方 notices 与适用的对应源码/构建材料；仅附项目主页或一条“开源”说明不足以完成分发义务。当前启动器直接下载上述上游独立组件，没有自行重新链接它们。

yt-dlp 官方独立包已带 EJS，仍需 JS runtime。处理器须传私有 Deno 与 FFmpeg 的绝对路径，关闭用户配置与插件，不把 URL 拼接成 shell 命令；受控任务目录先保留完整下载原片，ASR/CLAP 读取保留原音频的输入。下载站点发生变化时要更新固定发行与校验和，再验证实际导入；运行时不执行隐式自更新。


## 显式运行验证

常规 `go test ./internal/desktop/runtime` 不下载组件和模型。显式提供独立数据目录后可执行：

```sh
VH_TEST_DESKTOP_TOOLS_DIR=/absolute/disposable/data go test ./internal/desktop/runtime -run '^TestPinnedToolsIntegration$' -v
VH_TEST_DESKTOP_TOOLS_DIR=/absolute/disposable/data go test ./internal/desktop/runtime -run '^TestYTDLPPrivateMediaToolsIntegration$' -v
VH_TEST_DESKTOP_DIR=/absolute/disposable/data go test ./internal/desktop/runtime -run '^TestManagerIntegration$' -v -timeout 35m
```

工具测试校验实际发行文件与版本/许可；yt-dlp 测试在清空 PATH 后从本地 HTTP DASH fixture 下载分离音视频、合并并提取 WAV，验证其确实找到私有 ffprobe；它不证明某个线上站点当前可下载。Manager 测试实际安装私有依赖、加载模型、验证 `fake=false` 并编码原文中文查询，保存无令牌诊断；已有权重缓存时属于 warm-cache 验证，不能当作全新设备冷下载证明。该测试结束真实查询后停止自建模型进程，以新 Manager 在同目录再次 Ensure，并断言暖启动没有 `python`、`packages`、`models` 阶段。
