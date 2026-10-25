-- +goose Up
CREATE TABLE video_upload (
  video_id    bigint PRIMARY KEY REFERENCES video(id) ON DELETE CASCADE,
  object_key  text NOT NULL,
  upload_id   text NOT NULL,
  size_bytes  bigint NOT NULL CHECK (size_bytes > 0),
  part_size   bigint NOT NULL CHECK (part_size >= 5242880)
);

-- +goose Down
DROP TABLE video_upload;
