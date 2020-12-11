-- name: GetTask :one
select * from tasks order by created_at desc limit 1;

-- name: PutTask :one
BEGIN TRANSACTION; 
INSERT INTO tasks (
  url, sd_hash, type, created_at
) VALUES (
  $1, $2, $3, datetime('now')
);
SELECT * FROM tasks where id = last_insert_rowid();
COMMIT; 
