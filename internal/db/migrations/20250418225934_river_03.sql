-- +goose Up
-- +goose StatementBegin
SELECT 'up SQL query';

-- River main migration 003 [up]
ALTER TABLE river_job ALTER COLUMN tags SET DEFAULT '{}';
UPDATE river_job SET tags = '{}' WHERE tags IS NULL;
ALTER TABLE river_job ALTER COLUMN tags SET NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 'down SQL query';

-- River main migration 003 [down]
ALTER TABLE river_job ALTER COLUMN tags DROP NOT NULL,
ALTER COLUMN tags DROP DEFAULT;

-- +goose StatementEnd
