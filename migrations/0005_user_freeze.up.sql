-- Freeze a user: account is retained but cannot authenticate. NULL = active.
ALTER TABLE users ADD COLUMN frozen_at TIMESTAMP;
