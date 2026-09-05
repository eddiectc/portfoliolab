-- +goose Up
-- +goose StatementBegin
ALTER TABLE transactions ADD COLUMN description TEXT DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE transactions DROP COLUMN description;
-- +goose StatementEnd
