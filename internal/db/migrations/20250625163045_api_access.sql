-- +goose Up
-- +goose StatementBegin
SELECT 'up SQL query';

CREATE TABLE oracle_example.access
(
    wallet   VARCHAR(43) CONSTRAINT access_pk PRIMARY KEY
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 'down SQL query';

DROP TABLE oracle_example.access;
-- +goose StatementEnd
