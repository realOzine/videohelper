-- +goose Up
CREATE EXTENSION IF NOT EXISTS vector;  -- pgvector：向量类型与 HNSW 索引

-- 工作区：业务数据的归属与访问隔离边界
CREATE TABLE workspace (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name        text        NOT NULL,
  plan        text        NOT NULL DEFAULT 'free',   -- 套餐名；额度在配置里
  created_at  timestamptz NOT NULL DEFAULT now()
);

-- 登录的人。不叫 user，因为 user 是 Postgres 保留字。登录本身由认证服务负责。
CREATE TABLE app_user (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  auth_sub    text        NOT NULL UNIQUE,           -- 认证服务里的用户 id（令牌的 sub）
  email       text        NOT NULL,
  name        text        NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now()
);

-- 谁属于哪个工作区、什么角色
CREATE TABLE membership (
  workspace_id bigint      NOT NULL REFERENCES workspace(id)   ON DELETE CASCADE,
  user_id     bigint      NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
  role        text        NOT NULL CHECK (role IN ('owner', 'member')),
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, user_id)
);
CREATE INDEX membership_user_idx ON membership (user_id);

-- 项目：一组视频，是搜索与对话的范围
CREATE TABLE project (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  workspace_id bigint      NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  name        text        NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX project_workspace_idx ON project (workspace_id);

-- 视频元数据；文件本身在 S3
CREATE TABLE video (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  workspace_id bigint      NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  title        text        NOT NULL,
  source_kind  text        NOT NULL CHECK (source_kind IN ('upload', 'youtube', 'bilibili', 'direct')),  -- 来源：上传 / 平台导入 / 直链
  source_url   text,                                 -- 导入链接时的原始地址
  source_key   text,                                 -- S3 原片路径；上传完成或下载完成后填写
  normalized_video_key text,                                 -- S3 标准化视频路径；prepare_media 阶段完成后填写
  duration_ms  int CHECK (duration_ms > 0),                                  -- probe 填写；格子边界由它算出
  has_video    boolean,                              -- probe 填写；false 时不建画面阶段
  has_audio    boolean,                              -- probe 填写；false 时不建 asr 和音频阶段
  thumbs       jsonb,                                -- 雪碧图布局 {"tile_w":320,"tile_h":180,"cols":10,"rows":10,"count":1440}；prepare_media 填写
  status       text        NOT NULL CHECK (status IN ('uploading', 'importing', 'indexing', 'ready', 'failed')),
  error        text,                                 -- failed 时给用户看的原因
  deleted_at   timestamptz,                          -- 软删除时间；非空表示已删除，列表、检索、Agent 都看不到
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (id, workspace_id)
);
CREATE INDEX video_workspace_created_idx ON video (workspace_id, created_at DESC, id DESC)
  WHERE deleted_at IS NULL;                          -- 列表分页，只含未删除的视频
CREATE INDEX video_deleted_idx ON video (deleted_at)
  WHERE deleted_at IS NOT NULL;                      -- 清除任务查找到期的软删除视频

-- 项目与视频：多对多
CREATE TABLE project_video (
  project_id  bigint      NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  video_id    bigint      NOT NULL REFERENCES video(id)   ON DELETE CASCADE,
  added_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (project_id, video_id)
);
CREATE INDEX project_video_video_idx ON project_video (video_id);

-- 流水线阶段状态：流水线状态的唯一来源（Asynq 只负责投递）
CREATE TABLE stage_run (
  video_id    bigint      NOT NULL REFERENCES video(id) ON DELETE CASCADE,
  stage       text        NOT NULL,                  -- fetch / probe / prepare_media / asr / embed:visual:wemm-2b@r1 / finalize
  version     text        NOT NULL,                  -- 阶段实现版本；有不兼容变化时升级
  status      text        NOT NULL CHECK (status IN ('pending', 'queued', 'running', 'done', 'failed')),
  required    boolean     NOT NULL DEFAULT true,     -- finalize 是否要等它；生成计划时写死，shadow 的 embed 阶段为 false
  attempt     int         NOT NULL DEFAULT 0 CHECK (attempt >= 0),        -- 已执行次数
  error       text,                                  -- 最近一次错误
  output      jsonb       NOT NULL DEFAULT '{}',     -- 阶段产出与断点进度
  updated_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (video_id, stage, version)
);
CREATE INDEX stage_run_open_idx ON stage_run (status, updated_at)
  WHERE status IN ('pending', 'queued', 'running');  -- reconciler 扫描用

-- 文本片段：首版只有转录句子（speech）；OCR 以后用同一张表，kind 加 'ocr'
CREATE TABLE text_span (
  video_id    bigint      NOT NULL,
  workspace_id bigint      NOT NULL,                  -- 冗余，便于按工作区过滤
  kind        text        NOT NULL CHECK (kind IN ('speech')),
  start_ms    int         NOT NULL CHECK (start_ms >= 0),
  end_ms      int         NOT NULL,
  text        text        NOT NULL,
  words       jsonb,                                 -- 词级时间戳 [{"w":"接下来","s":12300,"e":12900}]，仅 speech
  model       text        NOT NULL,                  -- 产出它的模型，如 paraformer-zh@r1
  PRIMARY KEY (video_id, kind, start_ms),
  CHECK (end_ms > start_ms),
  FOREIGN KEY (video_id, workspace_id) REFERENCES video(id, workspace_id) ON DELETE CASCADE
);
CREATE INDEX text_span_workspace_idx    ON text_span (workspace_id, kind);

-- 向量空间登记：一个模型的一个版本
CREATE TABLE embedding_space (
  id             text        PRIMARY KEY,            -- 如 wemm-2b@r1
  dim            int         NOT NULL CHECK (dim BETWEEN 1 AND 4000),  -- halfvec 的 HNSW 上限是 4000 维
  metric         text        NOT NULL CHECK (metric IN ('cosine', 'ip', 'l2')),
  fingerprint    text        NOT NULL,               -- 权重 + 预处理配置的 hash
  doc_inputs     text[]      NOT NULL,               -- 能编码的文档类型：video / audio / text
  query_inputs   text[]      NOT NULL,               -- 能编码的查询类型：text
  max_span_ms    int CHECK (max_span_ms > 0),                                -- 视频、音频输入的单段最长时长
  disabled       boolean     NOT NULL DEFAULT false, -- fingerprint 或维度不一致时停用
  registered_at  timestamptz NOT NULL DEFAULT now()
);

-- 向量：一行一条；只放永远不变的定位字段，会变的属性（项目、标题）不放这里
CREATE TABLE embedding (
  video_id   bigint      NOT NULL,
  workspace_id bigint      NOT NULL,                   -- 工作区隔离；所有查询必带
  channel    text        NOT NULL CHECK (channel IN ('visual', 'speech', 'audio')),
  space_id   text        NOT NULL REFERENCES embedding_space(id),
  seg        text        NOT NULL,                   -- 切片版本：grid@v1 / sentence@v1
  start_ms   int         NOT NULL CHECK (start_ms >= 0),
  end_ms     int         NOT NULL,
  vec        halfvec     NOT NULL,                   -- 不写维度，各 space 共用一列；维度由写入方按登记表校验
  PRIMARY KEY (video_id, channel, space_id, seg, start_ms),
  CHECK (end_ms > start_ms),
  FOREIGN KEY (video_id, workspace_id) REFERENCES video(id, workspace_id) ON DELETE CASCADE
);
CREATE INDEX embedding_scan_idx ON embedding (workspace_id, channel, space_id, seg);  -- 小范围精确扫描、统计
-- 每个 (channel, space) 组合一个部分 HNSW 索引，由 videohelper space add 调用 index.EnsureVectorIndex 创建（提议），
-- 不放在迁移文件里，因为它取决于接入了哪些空间：
--   CREATE INDEX CONCURRENTLY IF NOT EXISTS emb_hnsw_visual_wemm_2b_r1 ON embedding
--     USING hnsw ((vec::halfvec(1024)) halfvec_cosine_ops)
--     WHERE channel = 'visual' AND space_id = 'wemm-2b@r1';

-- 计量：配额检查与以后计费
CREATE TABLE usage_event (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  workspace_id bigint      NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  kind       text        NOT NULL CHECK (kind IN ('gpu_seconds', 'llm_tokens', 'indexed_ms')),
  amount     bigint      NOT NULL CHECK (amount >= 0),
  ref        text,                                   -- 关联对象，如 video:42、session:s_9f2
  at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX usage_event_workspace_idx ON usage_event (workspace_id, kind, at);

-- +goose Down
DROP TABLE usage_event, embedding, embedding_space, text_span, stage_run,
           project_video, video, project, membership, app_user, workspace;
