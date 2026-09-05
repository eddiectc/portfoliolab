-- +goose Up
-- +goose StatementBegin
-- Enforce globally unique account names at the database level.
-- Application-level uniqueness check (GetByName) still provides
-- user-friendly error messages; this constraint is the safety net
-- against race conditions and direct DB writes.
CREATE UNIQUE INDEX idx_accounts_name_unique ON accounts(name);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_accounts_name_unique;
-- +goose StatementEnd
