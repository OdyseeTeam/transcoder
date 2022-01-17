-- +migrate Up
CREATE TYPE status AS ENUM (
  'new',
  'processing',
  'retrying',
  'errored',
  'failed',
  'done'
);

CREATE TABLE tasks (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at timestamp NOT NULL DEFAULT NOW(),
    updated_at timestamp,

    ulid text NOT NULL CHECK (ulid <> ''),
    status status NOT NULL,
    retries integer DEFAULT 0,
    stage text,
    stage_progress integer,
    error text,
    worker text NOT NULL,

    url text NOT NULL,
    sd_hash text NOT NULL,
    result text,

    UNIQUE ("ulid")
);

-- +migrate Down
DROP TABLE tasks;
