-- Rolls back the mobile device-binding columns added in 000020.
-- SQLite >= 3.35 supports ALTER TABLE DROP COLUMN.
-- Wrapped in a tx so a mid-rollback failure leaves no half-applied schema.
BEGIN;
ALTER TABLE sessions DROP COLUMN client_type;
ALTER TABLE sessions DROP COLUMN device_id;
ALTER TABLE sessions DROP COLUMN device_info;
COMMIT;
