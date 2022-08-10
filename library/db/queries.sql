-- name: AddVideo :one
INSERT INTO videos (
  tid, sd_hash, url, channel, storage, path, size, checksum, manifest
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING *;

-- name: GetAllVideos :many
SELECT * FROM videos;

-- name: GetAllVideosForStorage :many
SELECT * FROM videos
WHERE storage = $1;

-- name: GetAllVideosForStorageLimit :many
SELECT * FROM videos
WHERE storage = $1
LIMIT $2 OFFSET $3;

-- name: GetVideo :one
SELECT * FROM videos
WHERE sd_hash = $1 LIMIT 1;

-- name: RecordVideoAccess :exec
UPDATE videos
SET accessed_at = NOW(), access_count = access_count + 1
WHERE sd_hash = $1;

-- name: DeleteVideo :exec
DELETE from videos
WHERE tid = $1;

-- name: AddChannel :one
INSERT into channels (
    url, claim_id, priority
) VALUES (
    $1, $2, $3
)
RETURNING *;

-- name: GetChannel :one
SELECT * from channels
WHERE claim_id = $1;

-- name: GetAllChannels :many
SELECT * from channels;
