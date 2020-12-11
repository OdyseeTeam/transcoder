-- +migrate Up

-- +migrate StatementBegin
CREATE TABLE IF NOT EXISTS video (
    "sd_hash" TEXT PRIMARY KEY,

    "created_at" TEXT NOT NULL,

    "url" TEXT NOT NULL,
    "path" TEXT NOT NULL,
    "type" TEXT NOT NULL
);
-- +migrate StatementEnd

-- +migrate Down

-- +migrate StatementBegin
DROP TABLE video;
-- +migrate StatementEnd
