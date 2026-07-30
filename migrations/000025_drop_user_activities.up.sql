-- Login events are now recorded in audit_logs via LoginEventLogger;
-- the separate user_activities table is redundant.
DROP INDEX IF EXISTS idx_user_activities_user;
DROP INDEX IF EXISTS idx_user_actions_action;
DROP TABLE IF EXISTS user_activities;
