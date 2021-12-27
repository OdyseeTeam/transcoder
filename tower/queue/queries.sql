-- name: CreateTask :one
INSERT INTO tasks (
  status, uuid, worker, url, sd_hash
) VALUES (
  'new', $1, $2, $3, $4
)
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks
WHERE uuid = $1 LIMIT 1;

-- name: GetActiveTasks :many
SELECT * FROM tasks
WHERE status IN ('new', 'processing', 'retrying');

-- name: GetRetriableTasks :many
SELECT * FROM tasks
WHERE status = 'errored' AND (fatal IS FALSE OR retries < 10);

-- name: SetStageProgress :one
UPDATE tasks
SET stage = $2, stage_progress = $3, status = 'processing', updated_at = NOW() WHERE uuid = $1
RETURNING *;

-- name: SetStatus :one
UPDATE tasks
SET status = $2 WHERE uuid = $1
RETURNING *;

-- name: SetError :one
UPDATE tasks
SET status = 'errored', error = $2, fatal = $3, updated_at = NOW() WHERE uuid = $1
RETURNING *;

-- name: MarkRetrying :one
UPDATE tasks
SET status = 'retrying', retries = retries + 1, updated_at = NOW() WHERE uuid = $1
RETURNING *;

-- name: MarkDone :one
UPDATE tasks
SET status = 'done', stage = 'done', result = $2, updated_at = NOW() WHERE uuid = $1
RETURNING *;
