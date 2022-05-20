-- +migrate Up

ALTER TABLE videos
    ADD COLUMN manifest jsonb;

-- +migrate Down
ALTER TABLE videos
    DROP COLUMN manifest;
