-- +migrate Up

CREATE TYPE channel_priority AS ENUM (
  'high',
  'normal',
  'low',
  'disabled'
);

CREATE TABLE videos (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP,
    accessed_at TIMESTAMP,
    access_count integer DEFAULT 0,

    tid text NOT NULL UNIQUE CHECK (tid <> ''),

    url text NOT NULL CHECK (url <> ''),
    sd_hash text NOT NULL UNIQUE CHECK (sd_hash <> ''),
    channel text NOT NULL CHECK (channel <> ''),

    storage text NOT NULL CHECK (storage <> ''),
    path text NOT NULL CHECK (path <> ''),
    size bigint NOT NULL CHECK (size > 0),

    checksum text
);

CREATE TABLE channels (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMP NOT NULL DEFAULT NOW(),

    url text NOT NULL UNIQUE CHECK (url <> ''),
    claim_id text NOT NULL UNIQUE CHECK (claim_id <> ''),
    priority channel_priority NOT NULL
);

-- +migrate Down
DROP TABLE videos;
DROP TABLE channels;
DROP TYPE channel_priority;
