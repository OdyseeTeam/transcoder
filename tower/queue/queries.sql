-- name: CreateTask :one
INSERT INTO tasks (
  status, ref, worker, url, sd_hash
) VALUES (
  'new', $1, $2, $3, $4
)
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks
WHERE id = $1 AND STATUS IN ('new', 'active') LIMIT 1;

-- name: SetStageProgress :one
UPDATE tasks
SET stage = $2, stage_progress = $3 WHERE ref = $1
RETURNING *;

-- name: SetStatus :one
UPDATE tasks
SET status = $2 WHERE ref = $1
RETURNING *;

-- name: SetError :one
UPDATE tasks
SET status = 'error', error = $2 WHERE ref = $1
RETURNING *;

-- name: IncAttempts :one
UPDATE tasks
SET attempts = attempts + 1 WHERE ref = $1
RETURNING *;

-- name: MarkDone :one
UPDATE tasks
SET status = 'done', stage = 'done', result = $2 WHERE ref = $1
RETURNING *;
