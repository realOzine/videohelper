# 桌面私有运行时

桌面入口由一个 Go 二进制提供，首次联网下载组件、依赖与模型，并在用户数据目录中释放文件和启动私有子进程。它不是“永不释放文件的单进程”，也不是“首次启动即可完全离线”。前端资源和 ML 公共服务、插件、profile 清单与锁文件嵌入交付物；用户无需安装 Python、uv、FFmpeg、yt-dlp 或 Deno。

`internal/desktop/runtime.Manager` 接收数据目录与打包的 `fs.FS`。UI 可先显示 `State()`；后台依次调用 `Ensure(ctx, progress)`、`Start(ctx)`，完成后使用 `Services` 的内部端点、随机令牌与实际空间 ID。准备失败、进程退出或未完成加载时不得显示模型已就绪；`Close(ctx)` 只终止本次创建的进程树。客户端终止 HTTP 请求不能中断已执行的 Torch 推理线程；桌面退出时通过终止自己托管的 ML 进程结束推理。

## 首次启动与重复启动

1. 根据打包资源内容生成隔离目录 `runtime/ml-<digest>`，保留 `ml/`、`plugins/`、`profiles/` 的相对关系。
2. 按 `internal/desktop/runtime/components.json` 下载固定组件。下载写私有 staging，限制体积，核验发行文件 SHA256，提取固定目标文件后才发布。损坏的已安装文件不会直接执行；安装锁使用 OS 文件锁，失败或取消只清理本次 staging。
3. 固定 uv 0.13.6 使用随包 `python-downloads.json` 安装 CPython 3.12.15、build 20261021。该目录来自同版 uv 的上游元数据，含各平台准确 URL 与 SHA256；不修改用户 PATH、Python 可执行入口或 Windows 注册表。
4. profile 使用自己的私有 `.venv` 与 `uv sync --frozen --no-dev --no-default-groups --no-editable`，不依赖用户的 Python 环境。各依赖版本、下载地址与哈希来自该 profile 的 `uv.lock`。没有 NVIDIA 候选的 Linux/Windows 使用 CPU wheel；CUDA 候选才安装 cu130 wheel。
5. 首先尝试用缓存离线 `prepare`，缺失时才联网准备固定 revision 的权重。`HF_HOME` 固定为数据目录下的 `models/`；生产明确 `ML_FAKE=0`。WeMM 的固定修订与远程代码哈希检查继续生效。
6. 两个 ML 服务仅绑定 loopback 随机端口，使用本次随机内部令牌。以 `HF_HUB_OFFLINE=1` 加载，等待 `/readyz` 并核对 `/v1/spaces` 后才向处理器提供端点。

运行时与模型缓存独立于媒体与数据库。版本升级保留旧运行时；当前没有自动清理旧版本策略。首次安装需网络、足够磁盘空间和依赖源可达；下载失败可以重新启动后重试。

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
| FFmpeg / ffprobe | [eugeneware/ffmpeg-static b6.1.1](https://github.com/eugeneware/ffmpeg-static/releases/tag/b6.1.1) | 第三方构建，不是 FFmpeg 官方二进制；各平台实际 FFmpeg 版本和配置可能不同。按平台固定文件与哈希，同时下载该发行的 LICENSE、README |

FFmpeg 不能统一标作 LGPL：此功能需要 `libx264`，对应启用 GPL 的构建；Windows 构建 README 明示 GPLv3，其他平台应以本体 `-version`、`-L` 与上游对应构建源码核对。若发行最终 App 包含这些组件，发行流程必须同时整理准确许可、第三方 notices 与适用的对应源码/构建材料；仅附项目主页或一条“开源”说明不足以完成分发义务。当前启动器直接下载上述上游独立组件，没有自行重新链接它们。

yt-dlp 官方独立包已带 EJS，仍需 JS runtime。处理器须传私有 Deno 与 FFmpeg 的绝对路径，关闭用户配置与插件，不把 URL 拼接成 shell 命令；受控任务目录先保留完整下载原片，ASR/CLAP 读取保留原音频的输入。下载站点发生变化时要更新固定发行与校验和，再验证实际导入；运行时不执行隐式自更新。
