DROP INDEX IF EXISTS idx_audit_logs_type;
ALTER TABLE audit_logs DROP COLUMN type;
