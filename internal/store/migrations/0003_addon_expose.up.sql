-- Add-on external exposure: when exposed, the add-on container publishes its
-- port on a stable host port so it can be reached from outside the host (e.g.
-- a developer laptop or another cluster) via <addon>.<root_domain>:<host_port>.
ALTER TABLE addons ADD COLUMN exposed BOOLEAN NOT NULL DEFAULT 0;
-- host_port is the published host port. 0 means "not yet assigned"; once Docker
-- picks one on first expose we persist it here and reuse it across restarts so
-- the connection string stays stable. Kept even while unexposed so re-exposing
-- reuses the same port.
ALTER TABLE addons ADD COLUMN host_port INTEGER NOT NULL DEFAULT 0;
