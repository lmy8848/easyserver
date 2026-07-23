-- Rolls back FIM tables added in 000022.
BEGIN;
DROP TABLE IF EXISTS fim_changes;
DROP TABLE IF EXISTS fim_baseline;
COMMIT;
