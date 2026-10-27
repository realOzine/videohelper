CREATE TABLE IF NOT EXISTS video (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 title TEXT NOT NULL,
 source_kind TEXT NOT NULL CHECK(source_kind IN ('upload','youtube','bilibili','direct')),
 source_url TEXT NOT NULL DEFAULT '',
 source_filename TEXT NOT NULL DEFAULT '',
 source_key TEXT NOT NULL DEFAULT '',
 normalized_video_key TEXT NOT NULL DEFAULT '',
 duration_ms INTEGER CHECK(duration_ms > 0),
 has_video INTEGER,
 has_audio INTEGER,
 thumbs TEXT,
 status TEXT NOT NULL CHECK(status IN ('uploading','importing','indexing','ready','failed')),
 error TEXT NOT NULL DEFAULT '',
 deleted_at TEXT,
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS project (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 name TEXT NOT NULL CHECK(length(trim(name)) > 0),
 created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS project_video (
 project_id INTEGER NOT NULL REFERENCES project(id) ON DELETE CASCADE,
 video_id INTEGER NOT NULL REFERENCES video(id) ON DELETE CASCADE,
 PRIMARY KEY(project_id,video_id)
);
CREATE TABLE IF NOT EXISTS job (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 video_id INTEGER NOT NULL REFERENCES video(id) ON DELETE CASCADE,
 status TEXT NOT NULL CHECK(status IN ('queued','running','done','failed','cancelled')),
 stage TEXT NOT NULL DEFAULT 'probe',
 attempt INTEGER NOT NULL DEFAULT 0 CHECK(attempt >= 0),
 progress TEXT NOT NULL DEFAULT '{}',
 error TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS job_active_video ON job(video_id) WHERE status IN ('queued','running');
CREATE INDEX IF NOT EXISTS job_queued ON job(status,id);
CREATE TABLE IF NOT EXISTS sentence (
 video_id INTEGER NOT NULL REFERENCES video(id) ON DELETE CASCADE,
 start_ms INTEGER NOT NULL CHECK(start_ms >= 0),
 end_ms INTEGER NOT NULL CHECK(end_ms > start_ms),
 text TEXT NOT NULL,
 words TEXT,
 model TEXT NOT NULL,
 PRIMARY KEY(video_id,start_ms)
);
CREATE TABLE IF NOT EXISTS embedding_space (
 id TEXT PRIMARY KEY,
 fingerprint TEXT NOT NULL,
 dim INTEGER NOT NULL CHECK(dim BETWEEN 1 AND 4000),
 metric TEXT NOT NULL CHECK(metric IN ('cosine','l2')),
 disabled INTEGER NOT NULL DEFAULT 0,
 doc_inputs TEXT NOT NULL,
 query_inputs TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS embedding (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 video_id INTEGER NOT NULL REFERENCES video(id) ON DELETE CASCADE,
 space_id TEXT NOT NULL REFERENCES embedding_space(id),
 channel TEXT NOT NULL CHECK(channel IN ('visual','speech','audio')),
 seg TEXT NOT NULL,
 start_ms INTEGER NOT NULL CHECK(start_ms >= 0),
 end_ms INTEGER NOT NULL CHECK(end_ms > start_ms),
 UNIQUE(video_id,space_id,channel,seg,start_ms)
);
CREATE INDEX IF NOT EXISTS embedding_scope ON embedding(space_id,channel,seg,video_id);
