DROP INDEX IF EXISTS sessions_user;
DROP TABLE IF EXISTS sessions;
ALTER TABLE users DROP COLUMN password_set_at;
ALTER TABLE users DROP COLUMN password_hash;
