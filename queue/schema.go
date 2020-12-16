package queue

var InitialMigration = `
-- +migrate Up

-- +migrate StatementBegin
CREATE TABLE IF NOT EXISTS tasks  (
    "id" integer NOT NULL PRIMARY KEY AUTOINCREMENT,
    "sd_hash" TEXT UNIQUE NOT NULL,

    "created_at" TEXT NOT NULL,

    "url" TEXT NOT NULL,
    "progress" FLOAT,
    "status" TEXT NOT NULL,
    "started_at" TEXT,
    "type" TEXT NOT NULL
);
-- +migrate StatementEnd

-- +migrate Down

-- +migrate StatementBegin
DROP TABLE tasks;
-- +migrate StatementEnd
`
