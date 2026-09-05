-- +goose Up
-- Prevent duplicate imported transactions at the database level.
-- Application-level duplicate detection (HasExternalReference) still
-- provides user-friendly preview messages; this constraint is the
-- safety net against race conditions.

-- Remove any pre-existing duplicates (keep the first row by id).
-- In practice this should never fire, but it makes the migration
-- safe to run on databases that might have been manually seeded.
DELETE FROM transactions
WHERE external_system IS NOT NULL
  AND external_reference IS NOT NULL
  AND id NOT IN (
      SELECT MIN(id) FROM transactions
      WHERE external_system IS NOT NULL
        AND external_reference IS NOT NULL
      GROUP BY external_system, external_reference
  );

-- Allow multiple NULL/NULL rows (manually entered transactions) but
-- enforce uniqueness for non-NULL external references.
CREATE UNIQUE INDEX idx_transactions_external_ref
    ON transactions(external_system, external_reference)
    WHERE external_system IS NOT NULL AND external_reference IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_transactions_external_ref;
