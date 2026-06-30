-- Add build_command and start_command columns to websites
ALTER TABLE websites ADD COLUMN build_command TEXT DEFAULT '';
ALTER TABLE websites ADD COLUMN start_command TEXT DEFAULT '';
