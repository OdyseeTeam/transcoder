-- name: CreateTask :one
INSERT INTO tasks (
  status, ulid, worker, url, sd_hash
) VALUES (
  'new', $1, $2, $3, $4
)
RETURNING *;

-- name: GetAllTasks :many
SELECT * FROM tasks;

-- name: GetTask :one
SELECT * FROM tasks
WHERE ulid = $1 LIMIT 1;

-- name: GetTaskBySDHash :one
SELECT * FROM tasks
WHERE sd_hash = $1 LIMIT 1;

-- name: GetRunnableTaskByPayload :one
SELECT * FROM tasks
WHERE status NOT IN ('done', 'failed')
AND url = $1 AND sd_hash = $2 LIMIT 1;

-- name: GetActiveTasks :many
SELECT * FROM tasks
WHERE status IN ('new', 'processing', 'retrying', 'errored');

-- name: GetActiveTasksForWorker :many
SELECT * FROM tasks
WHERE status IN ('new', 'processing', 'retrying', 'errored') AND worker = $1;

-- name: GetRetriableTasks :many
SELECT * FROM tasks
WHERE status = 'errored' AND retries < 10;

-- name: SetStageProgress :one
UPDATE tasks
SET stage = $2, stage_progress = $3, status = 'processing', updated_at = NOW()
WHERE ulid = $1 AND status != ('errored', 'failed')
RETURNING *;

-- name: SetStatus :one
UPDATE tasks
SET status = $2 WHERE ulid = $1
RETURNING *;

-- name: SetError :one
UPDATE tasks
SET status = 'errored', error = $2, updated_at = NOW() WHERE ulid = $1
RETURNING *;

-- name: MarkRetrying :one
UPDATE tasks
SET status = 'retrying', retries = retries + 1, updated_at = NOW() WHERE ulid = $1
RETURNING *;

-- name: MarkFailed :one
UPDATE tasks
SET status = 'failed', error = $2, updated_at = NOW() WHERE ulid = $1
RETURNING *;

-- name: MarkDone :one
UPDATE tasks
SET status = 'done', stage = 'done', result = $2, updated_at = NOW() WHERE ulid = $1
RETURNING *;
