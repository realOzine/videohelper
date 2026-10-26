# VideoHelper

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

启动真实模型服务，保持此终端运行。首次启动下载清单固定版本的权重；已缓存时可额外设置 `HF_HUB_OFFLINE=1`。

```sh
ML_FAKE=0 ML_MODELS_FILE=models.local.yaml uv --directory ml run --extra wemm --extra asr --extra clap uvicorn app.main:app --host 127.0.0.1 --port 18080
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

声音通道采用 edit-mind 同款 CLAP HTSAT unfused 的 Python 原版权重。查询文字原样传入，英文转换后续再做。启用新通道后重启 api/worker；已有视频在 worker 启动及定期对账时补建缺失的声音索引，保留原有 ASR/画面索引和 `ready` 状态；切片版本变更的重建另行处理。默认查询关闭声音召回，但继续生成声音索引。需要搜索环境音时，将 `search.channel_weights.audio` 设为 1 并重启 API；`local-search` 每次启动读取配置。内部调用也可用显式 `Query.ChannelWeights["audio"]=1` 覆盖默认值。`channels.yaml` 的基础权重保持为 1，ASR 和原始音轨不受查询默认值影响。

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
