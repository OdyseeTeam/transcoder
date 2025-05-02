-- +migrate Up

ALTER TABLE videos
    ADD COLUMN released_at TIMESTAMP;

-- +migrate Down
ALTER TABLE videos
    DROP COLUMN released_at;
