# VideoHelper 桌面版实现说明

本说明对应个人桌面应用，配套 [设计](design.md) 与 [运行时安装说明](desktop-runtime.md)。接口只用于本机界面和托管模型，不是公开多用户 API。

## 模块

| 位置 | 职责 |
| --- | --- |
| `cmd/videohelper-desktop/` | 数据目录、单实例、窗口、后台准备、任务生命周期与退出回收 |
| `desktop/` | MyGo 窗口、系统文件选择器、内存 Bootstrap、macOS 单文件启动包装 |
| `web/src/` | 搜索、资料库、视频详情；共享主题和离线字体 |
| `Design/Icon/`、`Design/Kite/` | 应用图标、网页小图标与转曲品牌字标 |
| `assets.go` | 白名单内嵌模型服务、三个插件和四个固定 profile |
| `internal/desktop/server/` | 本机 UI DTO、文件/链接导入、媒体 Range |
| `internal/desktop/store/` | SQLite migrations、项目与视频、本地文件、任务与 sqlite-vec |
| `internal/desktop/processor/` | 单 owner 队列、转码/转录/编码、搜索与覆盖率 |
| `internal/desktop/runtime/` | 工具校验、私有 Python、硬件检测、托管模型进程 |
| `internal/desktop/filelock/` | 同目录只允许一个 owner；退出由 OS 释放锁 |
| `internal/media/` | FFmpeg/ffprobe 调用与媒体处理；使用私有工具绝对路径 |
| `internal/mlclient/` | ML 契约客户端、取消与响应身份/时间戳校验 |
| `internal/retrieval/`、`internal/timeline/` | 无数据库依赖的 RRF、片段合并、跳转点与时间轴规则 |
| `ml/app/`、`ml/plugins/`、`ml/profiles/` | Python 公共服务、WeMM/FunASR/CLAP 插件、固定环境与模型身份 |

旧用户、工作区、JWT、公开服务限流、计费、PostgreSQL/Redis/S3 和云部署代码不属于桌面路径，已移除。保留的 `internal/config.MLConfig` 仅提供本地模型端点与调用超时。

## 通用约定

JSON 字段使用 snake_case，ID 在传输时为字符串；时间戳使用 UTC RFC3339，时间区间为左闭右开的整数毫秒。文本查询和转录原文不经自动翻译或改写。

前端只通过 `Desktop.Bootstrap` 获取本进程的 loopback 地址与短期连接凭据，凭据不写 localStorage。`Desktop.PickFiles` 使用系统选择器。没有 `/me`、登录、注册或工作区接口。

资料库的状态标签、项目筛选与「查看处理中」在当前页面更新 URL 和列表，不重新加载 WebView；返回/前进同步恢复筛选，保留链接草稿、导入进度与运行环境状态。切换筛选或离开页面时取消旧列表、分页读取及轮询，迟到的响应不能覆盖新筛选；已提交的导入与处理任务不受列表读取取消影响。

macOS 裸二进制在打开数据库和启动模型前，将自身、内嵌图标及签名元数据恢复到 `Launcher/VideoHelper.app`，校验已有内容并原子更新，通过系统 `open` 启动后退出启动器。已有图标缺失或损坏时也重建包装。应用包装、开发模式与生成绑定不重复走此流程；实际 GUI 进程负责模型生命周期和退出清理。

`mygo.json` 的 `icon` 指向 `Design/Icon/png/videohelper-app-icon-1024.png`，由 MyGo 生成平台图标。macOS 单文件同时内嵌 `desktop/AppIcon.icns`、`Info.plist` 和 `CodeResources`；更换图标时必须从 `make desktop-app` 的产物同步这三个文件，再执行 `make desktop` 的逐字节一致性检查。使用产出的二进制运行 `VH_TEST_DESKTOP_EXECUTABLE=... go test ./desktop -run TestBuiltStandaloneRestoresValidSignature`，验证恢复后的资源签名。网页 favicon 和界面 SVG 由 Vite 直接打包，运行时不访问设计目录或外网。

HTTP 绑定 `127.0.0.1:0`，校验实际 Host、已知 MyGo Origin 和内存中的 `X-Desktop-Token`。这用于阻止其他网页驱动本地文件接口，不形成用户身份体系。媒体 URL 使用独立 capability，资源必须落在对应视频的受管目录中；`http.ServeContent` 提供 Range 和定位播放。

搜索播放器保留原生媒体控件，时间、命中播放头与播放状态由 `timeupdate`、`seeked`、`pause`、`ended` 等原生事件同步；命中跳转等待对应媒体的 metadata，不依赖递归 `requestAnimationFrame`。

全局搜索表单携带当前 `project`。搜索详情的 `m` 与当前命中同步，通过 `replaceState` 保留来源与滚动位置，不为 J/K 连续切换添加返回记录。短片段在十分之一秒精度无法区分起止时显示毫秒，避免非空区间呈现为零长度。

## 本地界面接口

| 方法与路径 | 行为 |
| --- | --- |
| `GET /v1/desktop/runtime` | 真实准备阶段、设备、回退原因与错误 |
| `POST /v1/desktop/runtime/retry` | 关闭失败的运行时，重新准备并复用缓存 |
| `GET/POST /v1/projects` | 本地视频分组列表、创建 |
| `DELETE /v1/projects/{id}` | 删除分组，不删除视频 |
| `PUT/DELETE /v1/projects/{id}/videos/{video_id}` | 加入/移出分组 |
| `GET /v1/videos`、`GET /v1/videos/{id}` | 列表、分页、详情和任务进度 |
| `POST /v1/imports/file` | 复制系统选择器选中的文件，并持久入队 |
| `POST /v1/imports/url` | 持久化 URL，由 yt-dlp 下载后进入同一处理链 |
| `POST /v1/videos/{id}/cancel` | 先撤销当前任务写入资格，再取消正在执行的调用 |
| `POST /v1/videos/{id}/retry` | 从已提交断点重试；模型身份变更时建立新索引 |
| `DELETE /v1/videos/{id}`、`POST /v1/videos/{id}/restore` | 软删除、恢复 |
| `GET /v1/videos/{id}/playback` | 返回当前进程有效的播放 URL |
| `POST /v1/search`、`POST /v1/search/models` | 片段搜索、当前模型与范围覆盖率 |

模型准备期间仍可导入，任务持久排队；搜索与主动重试在处理器未就绪时返回可重试错误。资料库和已生成媒体不依赖模型就绪。接口拒绝未知 JSON 字段与非法 ID。没有伪造成功响应或固定示例数据。

## SQLite 与本地文件

`store.Open` 取得非阻塞 OS 文件锁，开启 SQLite 外键、WAL、完整同步与短事务。`schema.sql` 内嵌迁移，`user_version` 拒绝由旧程序打开更新的数据库。当前只允许一个连接和一个任务调度 owner，没有 Redis 或分布式 lease。

本地文件复制完成后，受管源与任务在同一有界短事务提交，不因界面请求取消留下已接收但未入队的源文件；复制失败只清理本次尚未提交的新记录与临时文件。URL 导入同样在一个事务创建视频与任务。用户原始文件始终不变。

业务表是 `video`、`project`、`project_video`、`job`、`sentence`、`embedding_space`、`embedding`，没有用户、租户或计费字段。每个向量空间对应静态链接 sqlite-vec 的 `vec0` 虚表；元数据删除触发器同步删除向量行。

媒体 key 采用 `videos/<video_id>/...`，仅允许受控字符与相对路径。原片复制、派生文件和原始转录结果先写临时文件，再发布；读取拒绝越界、符号链接与非普通文件。任务输出使用 job/attempt 隔离目录。应用永远不删除用户导入前的源文件。

向量写入校验 channel、切片、有限值、维度与 fingerprint。召回在 KNN 前应用 ready、删除状态、视频、项目和时间范围过滤，排序保持确定性。后端/模型/转录身份变化形成独立空间，保留旧索引，不把同维度当作兼容。

## 任务与检索

单个 Go 调度器从 SQLite claim 任务，内存 channel 只负责唤醒。每次 claim 增加 attempt；输出、阶段与批次进度在一次事务中提交。取消、删除和旧 attempt 均不能提交迟到结果。启动时恢复遗留 running 任务，并补齐导入与入队之间的崩溃间隙；用户主动取消的任务不自动重新创建。

视频持久状态为 `failed` 且最新任务为 `cancelled` 时，对界面派生 `cancelled`；详情、列表、筛选和计数使用同一表达式。保留现有数据库约束，不重建视频表；`job(video_id,id DESC)` 索引支持最新任务查询。运行环境未就绪时也可取消已排队任务。

处理顺序为下载（链接输入）→探测→播放副本/缩略图→ASR→画面/转录/声音编码→完成。ASR 与 CLAP 读原片音频，画面编码读播放副本。已完成 ASR 的任务在编码失败后可以复用原始转录断点。

转录窗口按句子起点归属，原始响应持续保存。每个响应校验模型身份和词序，合法重叠窗口不额外要求跨窗口所有词起点单调；这不保证长视频边界转录质量，仍需真实样本验证。

搜索重用同一模型/文字的查询向量，sqlite-vec 每通道 K=5，RRF=60、相邻间隔一格合并；画面/转录/声音默认权重为 1/1/0。返回片段、跳转点、通道与原文摘要。显式指定模型即使遇到空范围或关闭通道也必须验证，不能悄悄忽略错误配置。

前端校验本地模型偏好的结构，并按当前目录的默认权重与可用模型解析覆盖项；全零通道或失效模型恢复默认，仍有效的单声音通道选择保留。模型弹窗禁止保存全零通道，正文独立滚动，操作区保持可见。语音通道需转录编码完成才显示可搜索，仅 ASR 完成不代表可搜索；无音轨视频不显示语音或声音可用。

## 模型运行时与发布

模型权重固定 revision，依赖固定 uv.lock，工具固定版本与 SHA256。模型媒体契约仅接受 `local://media/<key>`，解析到绝对 `ML_LOCAL_MEDIA_ROOT` 下的非空普通文件；不接受 S3、任意 file URL 或公网媒体 URL。

MPS/CUDA/CPU 的最终选择依赖真实 Torch 运算。组件的 OS/驱动检查仅决定安装候选。ASR/CLAP 当前使用 CPU；模型插件继续通过 entry point 发现。详细环境、设备身份、平台限制与许可来源见 [desktop-runtime.md](desktop-runtime.md)。

退出时取消 UI 调用、任务和 bootstrap，等待本次创建的进程树退出后关闭数据库。模型进程意外退出时，主程序停止当前处理并允许重建运行时。单个 HTTP 请求取消不会硬中断已经进入 Torch 的线程；退出应用会结束托管模型进程。

`make desktop` 内嵌 Web 资源与模型源码，导出一个可执行文件；不携带预装模型权重。构建成功、假模型接线测试、真实工具测试、真实模型短样本与 GUI 验收分别报告。旧服务端离线评估工具已退出本分支，冻结语料、历史结果和纯评分/校验工具保留，不能将旧后端基线当作新后端性能或质量证明。
