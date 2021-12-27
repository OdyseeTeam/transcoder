-- +migrate Up
CREATE TYPE status AS ENUM (
  'new',
  'processing',
  'retrying',
  'errored',
  'done'
);

CREATE TABLE tasks (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at timestamp NOT NULL DEFAULT NOW(),
    updated_at timestamp,

    uuid text NOT NULL CHECK (uuid <> ''),
    status status NOT NULL,
    retries integer DEFAULT 0,
    stage text,
    stage_progress integer,
    error text,
    fatal boolean,
    worker text NOT NULL,

    url text NOT NULL,
    sd_hash text NOT NULL,
    result text,

    UNIQUE ("uuid")
);

-- +migrate Down
DROP TABLE tasks;
