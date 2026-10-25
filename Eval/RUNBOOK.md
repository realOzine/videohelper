# 真实模型基线运行说明

本流程将**索引验收**与**固定索引检索回归**分开：上传、worker 和索引推理只在 `index` 执行；`retrieve` 校验已有索引，再调用真实查询编码和业务搜索。命令在仓库根目录执行。本文提供操作步骤，不宣称已有基线通过或给出未运行的质量分数。

`baseline-v1` 是保留的失败运行，不能当作可用快照。ASR 修复已从 `main` 的 `30efaef` 合入 `eval`（`a98cd20`），`baseline-v2` 正在重新索引验收；只有其 [index-run.json](runs/baseline-v2-index/index-run.json) 为 `status: passed` 且完整快照已保存后，才能执行下文检索与恢复命令。已使用的 ID（成功或失败）均不能重跑；后续索引用新的 ID，例如 `baseline-v3`。

## 前置条件与隔离资源

使用 Go、Python 3.12、FFmpeg/ffprobe、Docker Compose，以及已安装 `wemm`、`asr`、`clap` 三组 extras 的 `ml/.venv/`。这里直接复用现有解释器及依赖，不执行 `uv sync` 或重建环境；仅安装语料用的 `Eval/requirements.txt` 不足以运行 runner。新环境安装方法见[项目 README](../README.md)。

本地 Postgres/pgvector 与 MinIO 应已启动（`make deps-up`），真实模型服务应已按以下命令启动并完成加载：

```sh
(
  set -a
  . ./.env.example
  set +a
  export ML_INTERNAL_TOKEN="${VH_EVAL_ML_TOKEN:-$VH_ML_TOKEN}"
  export AWS_ACCESS_KEY_ID="${VH_EVAL_AWS_ACCESS_KEY_ID:-$AWS_ACCESS_KEY_ID}"
  export AWS_SECRET_ACCESS_KEY="${VH_EVAL_AWS_SECRET_ACCESS_KEY:-$AWS_SECRET_ACCESS_KEY}"
  export AWS_REGION="${VH_EVAL_AWS_REGION:-$AWS_REGION}"
  unset AWS_SESSION_TOKEN AWS_PROFILE AWS_DEFAULT_PROFILE AWS_ENDPOINT_URL AWS_ENDPOINT_URL_S3
  export ML_FAKE=0 ML_MODELS_FILE="$PWD/ml/models.local.yaml"
  export ML_CACHE_DIR="$PWD/Eval/.runtime/ml-cache"
  ml/.venv/bin/python -m uvicorn app.main:app --app-dir ml --host 127.0.0.1 --port 18080
)
```

`.env.example` 是仓库公开的本地测试凭据，不是实际 `.env`；上述子 shell 为 ML 服务提供令牌、MinIO endpoint 和 S3 凭据，将媒体缓存留在 `Eval/.runtime/ml-cache/`。runner 也默认读取这个公开文件。如本地凭据不同，在启动服务和 runner 的终端都设置 `VH_EVAL_ML_TOKEN`、`VH_EVAL_AWS_ACCESS_KEY_ID`、`VH_EVAL_AWS_SECRET_ACCESS_KEY`、`VH_EVAL_AWS_REGION`；数据库凭据用 `VH_EVAL_DB_USER` / `VH_EVAL_DB_PASSWORD`。不要把密钥写进报告。ML 地址可用 `VH_EVAL_ML_ENDPOINT` 指定，MinIO endpoint 须与 `config/local.yaml` 及服务的 `ML_S3_ENDPOINT` 一致；runner 用 `/v1/spaces` 核对真实模型清单及指纹，拒绝 fake 或过期配置。

资源选择以 [environment.py](tools/environment.py) 为准：

| 资源 | `baseline-v2` | 约束 |
| --- | --- | --- |
| 数据库 | `vh_eval_baseline_v2` | 固定本机端口 `55432`；必须不存在，不复用开发库 |
| MinIO 桶 | `vh-eval-baseline-v2` | 使用 `config/local.yaml` 的本地 endpoint；必须不存在 |
| Redis | `127.0.0.1:56380` | [独立 Compose](deploy/redis-compose.yml)，不使用开发队列 |
| 本地状态 | `Eval/.runtime/baseline-v2/` | 必须不存在；配置与临时文件留在 Eval 内 |

`index` 自动启动 Eval Redis，发现非空即拒绝，不自动清队列。`run_id` 只接受 1–40 位小写字母、数字和中划线，首尾必须是字母或数字。重复运行应换新 ID；不要删除失败证据来复用旧 ID。`index` / `restore` 有互斥锁；整个基线流程期间不要并行启动会改写这些资源的 worker。

## 1. 构建、校验语料与单元检查

```sh
ml/.venv/bin/python Eval/tools/run.py build
ml/.venv/bin/python Eval/tools/validate.py
ml/.venv/bin/python -m unittest discover -s Eval/tools -p 'test_*.py' -v
ml/.venv/bin/python -m pytest Eval/defects/asr-timestamps/test_repro.py -q
go test -race ./Eval/cmd/evalbridge
go vet ./Eval/cmd/evalbridge
```

`build` 编译 `Eval/bin/videohelper` 和 `Eval/bin/evalbridge`；不会导入视频。`validate.py` 校验固定媒体及时间标注并更新 `review/validation.json`，不证明检索质量。runner 还会检查 `checksums.sha256`，该文件只冻结语料素材、标注和成片，不冻结工具或报告。保留当前语料字节、35 条查询与标签；改媒体或语义标签应另建数据集版本。

## 2. 单独完成索引验收并保存快照

在后续命令使用的同一终端选定一次快照名和检索 ID 前缀：

```sh
SNAPSHOT=baseline-v3
RUN=check-01
```

新索引使用尚未用过的 `SNAPSHOT`；当前 `baseline-v2` 已启动，不要重跑或并行启动新索引。若要复用已有快照，只有确认 `baseline-v2` 验收通过且归档完整后，才可改设 `SNAPSHOT=baseline-v2` 并**跳过下一条 `index`**。`RUN` 每次流程必须换新值；它会派生 `-a`、`-b`、`-tuned`、`-restored`、`-restored-check`，这些 ID 和输出目录均不可复用。新建索引的 `SNAPSHOT` 最多 34 字符（运行目录附加 `-index`），`RUN` 最多 25 字符，以便最长后缀仍满足 40 字符限制；二者均遵守前述 ID 字符规则。

```sh
ml/.venv/bin/python Eval/tools/run.py index --run-id "$SNAPSHOT"
```

默认索引等待上限为 7200 秒，可显式传 `--timeout-seconds 10800`。runner 创建独立数据库/桶，执行迁移和空间初始化，仅上传 5 个固定 MP4，通过真实分片上传及 worker 完成全流程。标注、字幕和参考讲解不导入应用。

检查 `Eval/runs/${SNAPSHOT}-index/` 下的 `index-run.json`、`stages.json`、`inspection.json` 和 `worker.log`。当前 v2 的实际状态见 [index-run.json](runs/baseline-v2-index/index-run.json)。索引检查包含视频 ready、必要阶段、归属、非空 ASR、切片覆盖、向量和 HNSW/模型空间身份。worker 停止后保存数据库、对象及稳定的 `index_sha256`；上传源文件还会核对冻结视频哈希，归一化视频核对音视频流与时长。

只有索引报告为 `status: passed` 后，才使用 `Eval/snapshots/${SNAPSHOT}/` 的完整快照继续。失败则保留阶段、日志、已上传数据和资源，先查明原因；不能将部分成功或 fake 运行记为真实基线。

## 3. 在同一固定索引运行两次检索并比较

```sh
ml/.venv/bin/python Eval/tools/run.py retrieve --snapshot "$SNAPSHOT" --run-id "${RUN}-a"
ml/.venv/bin/python Eval/tools/run.py retrieve --snapshot "$SNAPSHOT" --run-id "${RUN}-b"
ml/.venv/bin/python Eval/tools/score.py compare --before "Eval/runs/${RUN}-a/report.json" --after "Eval/runs/${RUN}-b/report.json" --output "Eval/runs/${RUN}-a-vs-b"
```

每次针对全库 5 个视频执行 35 条查询 × `visual` / `speech` / `fused`，共 105 个响应，每题最多取 10 个 moment。检索前后检查索引指纹与模型身份；不重新上传或启动 worker。查询文字原样传给模型，当前没有英文转换。`fused` 使用当前 active 的全部通道，包含 `audio`；没有独立声音金标，不能据此单独验收 CLAP。

`report.json` / `report.md` 为汇总，`per_query.jsonl` 为逐题评分，`raw-responses.jsonl` 保存桥接原始输出，`results.jsonl` 补充稳定 `media_id`，`metadata.json` 记录代码、模型、索引、通道与检索参数。`requests.jsonl`、前后 `inspection-*.json` 与 `search.log` 用于追查失败。`passed=true` 仅表示响应有效且索引稳定，**没有设定质量达标阈值**。

比较输出 `delta.json` / `delta.md`；核对 `compatible`、`fixed_index_regression`、`identical_retrieval_conditions`，再看逐题变化。语料、标签、评分定义不一致会拒绝比较；快照或模型变化会标明，不能称作固定索引回归。同条件重复运行也要保留实际差异，不预先宣称结果一致。

固定索引上试验召回数、RRF 常数和合并间隙时，使用新 run ID，参数进入报告，不改快照：

```sh
ml/.venv/bin/python Eval/tools/run.py retrieve --snapshot "$SNAPSHOT" --run-id "${RUN}-tuned" --k 100 --rrf-k 30 --merge-gap 0
ml/.venv/bin/python Eval/tools/score.py compare --before "Eval/runs/${RUN}-a/report.json" --after "Eval/runs/${RUN}-tuned/report.json" --output "Eval/runs/${RUN}-a-vs-tuned"
```

`--k` 是每路召回数，不是评分的 Recall@K，也不改变每题输出上限 10；`--rrf-k` 对应 `search.rrf_k`，`--merge-gap` 对应 `search.merge_gap_cells`。参数比较可以仍是固定索引，但 `identical_retrieval_conditions` 应为 false。不要用同批调参样本宣称最终泛化验收。

## 4. 恢复快照，再验证固定索引检索

```sh
ml/.venv/bin/python Eval/tools/run.py restore --snapshot "$SNAPSHOT" --run-id "${RUN}-restored"
ml/.venv/bin/python Eval/tools/run.py retrieve --snapshot "$SNAPSHOT" --run-id "${RUN}-restored-check" --state "Eval/.runtime/${RUN}-restored/state.json"
ml/.venv/bin/python Eval/tools/score.py compare --before "Eval/runs/${RUN}-a/report.json" --after "Eval/runs/${RUN}-restored-check/report.json" --output "Eval/runs/${RUN}-a-vs-restored"
```

恢复使用 `${RUN}-restored` 派生的新数据库和桶；例如 `RUN=check-01` 时为 `vh_eval_check_01_restored` 和 `vh-eval-check-01-restored`。先验证归档哈希，再还原数据库、将已归档对象复制到新桶，检查恢复后的 `index_sha256` 与原快照一致；结果在新 runtime 的 `restored-from.json`。对象复制是快照恢复，不执行 `local-upload`、worker、视频重处理或索引模型推理。随后 `retrieve` 仍需真实 ML 服务做查询编码。恢复前必须保留完整 dump 和对象归档；只有 Git 元数据不能恢复数据。

## 如何判读质量与失败

- 正例按模式、类别及查询语言分组；每组所有有答案查询都是分母，缺失、重复、非法、报错或 skipped 的响应计失败并得零分，不把它们排除或当作有效空结果。
- video Recall@1/5/10 只要求视频正确；overlap Recall 要同视频且时间交集大于零。tIoU 是交集时长 / 并集时长，分别报告 ≥0.3、≥0.5 的 Recall，避免超长片段虚增定位命中。MRR 为首个同视频且 overlap 结果的倒数排名。
- jump Hit@1/5/10 要 `jump_ms` 落在标注的半开窗口内；所有区间均为整数毫秒 `[start_ms,end_ms)`。跳转窗口是脚本导出的初始启发式，仍待人工复核。Top-1 起终点绝对误差及 tIoU 仅在首名视频正确时统计，须同时看条件样本数；best tIoU 对未命中/失败为零。
- 负例非空率仅以有效负例响应为分母，失败数另列，同时看 Top-1 分数。返回候选不等于已校准拒答；RRF 分数不是概率，5 条负例也不足以标定拒答阈值。
- 当前标签均为合成脚本/时间线推导，`human_reviewed=false`，助手场景帧检查不等于独立人工金标。当前 `paraformer-zh@r1` 声明 `languages: [zh]`；英文及混合语音的转录和 speech 检索存在已知能力边界，实际质量失败应保留。串行、模型已加载的耗时不是冷启动或并发压测。

## 保留、停止与提交

保留真实质量失败及原因。报告、逐题结果、元数据、快照清单和检查结果可以版本化；`database.dump`、`snapshots/*/objects/`、`.runtime/`、`bin/` 及日志按 [Eval/.gitignore](.gitignore) 忽略。dump/对象可能很大，须另行备份；不要用 `git add -f` 填进仓库。源图、提示词、音频、冻结成片和标注保持原样。

`index` 在成功或异常退出时都会尝试停止其 worker。结束后可停止 Eval Redis并保留持久化数据：

```sh
docker compose -f Eval/deploy/redis-compose.yml down
```

不自动删除数据库、桶、快照或运行目录。下一轮索引若因 Eval 队列非空被拒绝，先确认该轮 Eval worker 已停并保存证据，再执行上述 `compose down`。独立 Redis 的持久化目录是 `Eval/.runtime/redis/`；将它整体归档到旧轮次目录，保留原数据，再建空目录。以下仅用于 **baseline-v2 已结束之后**，不得在当前索引运行时执行：

```sh
(
  set -e
  test -d Eval/.runtime/baseline-v2
  test ! -e Eval/.runtime/baseline-v2/redis-archive
  mv Eval/.runtime/redis Eval/.runtime/baseline-v2/redis-archive
  mkdir Eval/.runtime/redis
)
```

`baseline-v1` 的旧队列已保存在 `Eval/.runtime/baseline-v1/redis-archive/`，不要覆盖。归档名若已存在，应换一个未使用的路径。下一轮 `index` 会自行启动独立 Redis 并检查空队列；归档操作不删除数据库、桶或失败证据。

开发按 [AGENTS.md](AGENTS.md) 的小单元进行：并行任务使用独立 `Eval/.worktrees/`、`gpt-6.1-sol` / high effort；各单元运行相应检查后独立 commit，协调者评审并合并到 `eval`，再删除已合并 worktree，不直接集成到 `main`。提交时检查 diff 和忽略规则，只提交对应单元及真实证据；本说明不代替实际执行记录或运行报告。
