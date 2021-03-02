package video

const dbFile = "videos.db"

var InitialMigration = `
-- +migrate Up

-- +migrate StatementBegin
CREATE TABLE IF NOT EXISTS videos (
    "sd_hash" TEXT PRIMARY KEY,

    "created_at" TIMESTAMP NOT NULL,

    "url" TEXT NOT NULL,
    "path" TEXT NOT NULL,
    "remote_path" TEXT NOT NULL DEFAULT "",

	"type" TEXT NOT NULL,
	"channel" TEXT NOT NULL,

	"last_accessed" TIMESTAMP,
	"access_count" INTEGER NOT NULL DEFAULT 0,

	"size" INTEGER NOT NULL DEFAULT 0,
	"checksum" TEXT NOT NULL DEFAULT ""
);
-- +migrate StatementEnd

-- +migrate Down

-- +migrate StatementBegin
DROP TABLE videos;
-- +migrate StatementEnd
`
