-- +migrate Up
-- +migrate StatementBegin
ALTER TABLE videos ALTER COLUMN accessed_at SET NOT NULL;
ALTER TABLE videos ALTER COLUMN accessed_at SET DEFAULT NOW();
-- +migrate StatementEnd

-- +migrate Down
-- +migrate StatementBegin
ALTER TABLE videos ALTER COLUMN accessed_at DROP NOT NULL;
ALTER TABLE videos ALTER COLUMN accessed_at DROP DEFAULT;
-- +migrate StatementEnd
