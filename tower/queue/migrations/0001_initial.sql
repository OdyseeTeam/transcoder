-- +migrate Up
CREATE TABLE tasks (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at timestamp NOT NULL DEFAULT now(),
    updated_at timestamp NOT NULL DEFAULT now(),

    ref text NOT NULL,
    status text NOT NULL,
    attempts integer DEFAULT 1,
    stage text,
    stage_progress integer,
    error text,
    worker text NOT NULL,

    url text NOT NULL,
    sd_hash text NOT NULL,
    result text,

    UNIQUE ("ref")
);

-- +migrate Down
DROP TABLE tasks;
