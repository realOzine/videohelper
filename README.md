# VideoHelper

`cloud` 分支保留旧服务器与 AWS Demo 版本；当前桌面应用位于 [`main`](https://github.com/realOzine/videohelper/tree/main)。以下说明对应 `cloud`。

本地视频处理与文字搜索：保留原片音轨，通过 WeMM 编码画面，通过 FunASR 转录后编码文字，通过 CLAP 编码声音，保留 `visual`、`speech`、`audio` 三路索引。当前默认检索采用 K=5、RRF=60、gap=1，查询权重为 `visual:speech:audio=1:1:0`；变更依据、局限和回滚方法见 [Evaluation.md](docs/Evaluation.md)。图片文档编码已提供 ML 接口；搜索输入为文字。设计与契约见 [docs/design.md](docs/design.md) 和 [docs/spec.md](docs/spec.md)。

需要 Go、Python 3.12、uv、FFmpeg/ffprobe 和 Docker Compose。当前真实模型清单使用 Apple Silicon MPS 运行 WeMM，CPU 运行 ASR、CLAP。以下命令在仓库根目录执行；每个终端先载入 `.env`。

```sh
cp -n .env.example .env
set -a
source .env
set +a
make deps-up
make build
uv --directory ml sync --frozen --extra wemm --extra asr --extra clap
```

先准备固定版本的权重，再离线启动真实模型服务，保持此终端运行。下面保留单进程组合方式；独立插件环境的命令见后文。

```sh
uv --directory ml run --frozen --extra wemm --extra asr --extra clap videohelper-ml prepare --models models.local.yaml
ML_FAKE=0 HF_HUB_OFFLINE=1 uv --directory ml run --frozen --extra wemm --extra asr --extra clap videohelper-ml serve --models models.local.yaml --port 18080
```

在另一个已载入 `.env` 的终端初始化数据库、对象桶及向量索引，再启动 worker：

```sh
bin/videohelper migrate
bin/videohelper local-init
bin/videohelper space add visual wemm-2b@r1
bin/videohelper space add speech wemm-2b@r1
bin/videohelper space add audio clap-htsat-unfused@r1
bin/videohelper worker
```

在第三个已载入 `.env` 的终端上传、查看阶段进度并搜索。将示例路径和 ID 换成自己的值：

```sh
bin/videohelper local-upload /path/to/video.mp4
bin/videohelper local-video 1
bin/videohelper local-search '如何搜索视频内容'
```

声音通道采用 edit-mind 同款 CLAP HTSAT unfused 的 Python 原版权重。查询文字原样传入，英文转换后续再做。启用新通道后重启 api/worker；已有视频在 worker 启动及定期对账时补建缺失的声音索引，保留原有 ASR/画面索引和 `ready` 状态；切片版本变更的重建另行处理。默认查询关闭声音召回，但继续生成声音索引。HTTP 搜索可传 `channel_weights.audio=1` 单次启用声音查询；全局默认则修改 `search.channel_weights.audio` 并重启 API，`local-search` 每次启动读取配置。`channels.yaml` 的基础权重保持为 1，ASR 和原始音轨不受查询默认值影响。

ASR 允许起点有序的词、句时间重叠，合并范围取最早起点和最晚终点，保留原文。真实清单的 `adapter_version: 2` 会改变 ASR fingerprint；部署时同步更新并重启 Go 与 ML 服务。已有已完成索引仍可查询，旧计划和断点不会自动迁移到新 fingerprint，普通 retry 也不会改写它们；验证新行为请使用新上传或新的隔离 Eval run，保留旧基线。

`local-*` 命令使用数据库管理凭据和固定本地身份，直接调用业务服务。上传按预签名分片地址流式发送；失败时保留上传会话，错误包含视频 ID。`local-video` 只依赖数据库。搜索输出是诊断 JSON，字段为 Go 的 `Moments`、`Skipped` 等名称，视频 ID 为整数；对外 HTTP 接口使用独立的 snake_case、字符串 ID 契约。

HTTP 服务通过 `bin/videohelper -config /path/to/config.yaml api` 启动，配置必须包含有效的 `auth.issuer`、`auth.audience`、`auth.jwks_url`。`/v1/*` 始终校验 JWT；`/healthz`、`/readyz` 无需登录。`local-*` 的固定身份不会用于 HTTP 请求。全局 `-config` 参数放在子命令之前。

本地 Web 前端在 `web/`（React + TypeScript + Tailwind，Vite 构建），需要 Node 22+，首次先运行 `npm --prefix web ci`。`config/local.yaml` 的 `auth` 指向本地开发签发器，`cors_origins` 已允许 `http://127.0.0.1:5173`。在三个已载入 `.env` 的终端分别运行下列命令，再打开 <http://127.0.0.1:5173>，登录页填 API 地址 `http://127.0.0.1:18000` 和签发器打印的令牌：

```sh
node web/dev/devauth.mjs                          # 打印 7 天有效的令牌；重启后旧令牌失效
bin/videohelper api
npm --prefix web run dev                          # 改动即时刷新；npm --prefix web run build 输出 web/dist，可交给任意静态主机
```

浏览器上传直传对象存储，存储的 CORS 需要暴露 `ETag` 响应头（本地 MinIO 默认满足）。

用 Ctrl-C 停止 Go/ML 进程，`make deps-down` 停止基础服务并保留数据卷。`make test-go`、`make vet` 检查 Go；Python 常规检查为 `uv --directory ml run pytest` 和 `uv --directory ml run ruff check .`。真实模型与 MinIO 网络测试需要显式 `VH_TEST_REAL_MODELS=1`、`VH_TEST_S3_ENDPOINT`，运行方式见 [测试文件](ml/tests/test_real_models.py)。CLAP 独立真实测试使用 `VH_TEST_REAL_CLAP=1`，见 [CLAP 测试](ml/tests/test_clap_real.py)。

## 按检索通道选择模型（后端 API）

已登录的调用方可先查询同一搜索范围的模型目录，再传入各路选择；未提供的模型 / 权重仍沿用默认值。Web 搜索页的「模型」按钮提供同样的逐路选择；选择保存在当前浏览器，之后的搜索沿用，直到恢复默认。

```sh
# API_URL、TOKEN 使用当前 API 地址和有效 JWT；所有 ID 都是字符串。
curl --fail-with-body "$API_URL/v1/search/models" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"project_id":"7"}'

curl --fail-with-body "$API_URL/v1/search" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"query":"海边有人弹吉他","project_id":"7","space_overrides":{"visual":"wemm-2b@r1","speech":"wemm-2b@r1","audio":"clap-htsat-unfused@r1"},"channel_weights":{"audio":1}}'
```

目录按 `visual`、`speech`、`audio` 返回能力兼容的已发现空间、active/shadow 候选及状态。选择 `status=ready` 的空间；安装模型还不够，需要先 `space add` 建索引、加入 shadow、重启 api/worker 并完成回填。显式选择未完成索引的模型返回 409 `not_ready`；不兼容或未知选择返回 400，模型不可用返回 503，不会换成另一个模型。

就绪检查按范围内 ready 视频的完整编码阶段计算，不以已有部分向量判断；没有适用视频时计数为 0/0。`speech` 选的是 ASR 文字的编码模型，不切换 ASR。当前 WeMM 不能用于 audio，CLAP 可用于 audio；同一多模态空间支持多路时可分别选择。请求不改变后台索引配置或后续请求的默认值。完整字段、范围与错误语义见 [Spec 接口定义](docs/spec.md#44-接口定义)。

## 模型插件与独立环境

通用服务 `videohelper-ml` 与三个模型包 `videohelper-ml-wemm`、`videohelper-ml-funasr`、`videohelper-ml-clap` 分开安装。插件以 wheel 交付，通过 `videohelper.ml.adapters.v1` entry point 注册；接口与新增插件约定见 [spec 第 10 章](docs/spec.md#10-python-模型服务)。缺包或配置错误会拒绝启动，只有 `ML_FAKE=1` 才使用假模型。

每个运行配置有独立依赖锁文件和环境：

| Profile | 平台 | 模型 |
|---|---|---|
| `wemm-mps` | Mac arm64 原生 | WeMM，MPS float16 |
| `wemm-cpu` | Mac arm64 / Linux amd64 | WeMM，CPU float32，独立向量空间 |
| `speech-audio-cpu` | Mac arm64 / Linux amd64 | FunASR、CLAP，CPU float32 |
| `wemm-cuda` | Linux amd64 + NVIDIA GPU | WeMM CUDA 候选，独立向量空间 |

在 Mac 上分别安装、准备；不会复用 `ml/.venv`：

```sh
make ml-install ML_PROFILE=wemm-mps
make ml-install ML_PROFILE=speech-audio-cpu
make ml-prepare ML_PROFILE=wemm-mps
make ml-prepare ML_PROFILE=speech-audio-cpu
```

停止前面的组合模型服务，将 `config/local.yaml` 的 `ml.endpoints` 改为 `["http://127.0.0.1:18080", "http://127.0.0.1:18081"]`，保留 `ml.transcriber: paraformer-zh@r1`。在两个已载入 `.env` 的终端分别运行，随后重启 Go API/worker：

```sh
ML_FAKE=0 HF_HUB_OFFLINE=1 make ml-serve ML_PROFILE=wemm-mps ML_PORT=18080
ML_FAKE=0 HF_HUB_OFFLINE=1 make ml-serve ML_PROFILE=speech-audio-cpu ML_PORT=18081
```

WeMM 也可用纯 CPU：将安装、准备和启动命令中的 `ML_PROFILE=wemm-mps` 换成 `ML_PROFILE=wemm-cpu`。它沿用相同插件、固定权重和预处理，使用 `device: cpu`、`dtype: float32`；Linux 安装官方 CPU 版 Torch / Torchvision，无需 CUDA。空间 ID 为 `wemm-2b-cpu@r1`：新部署须为 visual、speech 执行 `space add` 并配置这个空间；已有 MPS 索引按下文 shadow 回填流程迁移，不能直接复用旧空间身份。CPU 推理性能尚未验收。

`make ml-status ML_PORT=18080` 检查就绪；`videohelper-ml describe --models <清单>` 只展示校验后的配置。实际加载信息通过带 `X-Internal-Token`、`X-Request-ID` 的 `GET /v1/runtime` 查询，包含服务/插件/依赖版本、设备和 fingerprint。用 Ctrl-C 停止前台服务；停止不会删除缓存与索引。

Linux 容器使用仓库根目录作为构建上下文，不把权重打进镜像。先设置 `ML_INTERNAL_TOKEN`、S3 地址与凭据；容器内 S3 地址须能访问对象存储，不能直接沿用宿主机的 `127.0.0.1`。CPU 示例：

```sh
docker compose -f deploy/ml-compose.yml build speech-audio
HF_HUB_OFFLINE=0 docker compose -f deploy/ml-compose.yml run --rm speech-audio videohelper-ml prepare --models models.yaml
docker compose -f deploy/ml-compose.yml up -d speech-audio
docker compose -f deploy/ml-compose.yml stop speech-audio
```

CPU 默认监听宿主机 8001；CUDA 用 `--profile cuda` 和服务名 `wemm-cuda`，监听 8002，需 NVIDIA Container Toolkit。当前 CUDA 锁文件使用 13.0，驱动至少满足 [NVIDIA 的 580 系列下限](https://docs.nvidia.com/deploy/cuda-compatibility/minor-version-compatibility.html)，具体 GPU 与内核仍需实机验证。将实际服务地址加入 Go `ml.endpoints`。CUDA 配置是待验收候选，使用 `wemm-2b-cuda-experimental@r1`，不能直接与 MPS 旧向量混用。

换模型时，先安装新插件与运行配置、prepare、启动、登记 shadow 空间并回填，验证后再切换 active；已有 ASR 任务仍依赖冻结的旧模型和指纹，保留旧服务直到任务与回滚窗口结束。同 ID 同时由多个服务提供时，在 `ml.model_endpoints` 显式绑定地址；绑定服务离线时不会自动换到其他服务。升级插件后重新锁定 profile 并重装，避免安装环境与锁文件不一致。只改打包方式保持现有空间指纹；改变输出行为必须使用新指纹和空间。

`make test-ml-lifecycle` 在临时环境安装 core wheel，验证启动、鉴权、假模型请求和停止，并清理进程；不下载模型，也不证明真实检索质量。

## AWS 单用户 Demo

部署、资源计费和启停操作见 [部署手册](deploy/aws/README.md)，架构见 [设计文档](docs/design.md#102-aws-单用户-demo)。CPU Demo 已通过短样本和启停验证；GitHub 仓库已配置 CodeConnections / CodeBuild 发布，本分支配置目标为 `cloud`，既有云端分支关联须另行同步，自动发布运行验收仍等待 AWS 并发配额。停止状态不自动开机。AWS Toolkit 技能仅安装于本仓库 `.agents/skills/`，未安装到用户全局技能目录。
