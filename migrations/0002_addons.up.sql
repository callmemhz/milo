CREATE TABLE addons (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT    NOT NULL,
    engine          TEXT    NOT NULL CHECK (engine IN ('postgres','redis')),
    version         TEXT    NOT NULL,
    cpu_limit       REAL    NOT NULL DEFAULT 0.5,
    memory_limit_mb INTEGER NOT NULL DEFAULT 512,
    password        TEXT    NOT NULL,
    status          TEXT    NOT NULL DEFAULT 'provisioning',
    container_name  TEXT,
    created_at      TIMESTAMP NOT NULL,
    deleted_at      TIMESTAMP
);
CREATE UNIQUE INDEX addons_name_active ON addons(name) WHERE deleted_at IS NULL;

CREATE TABLE addon_owners (
    addon_id INTEGER NOT NULL REFERENCES addons(id) ON DELETE CASCADE,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (addon_id, user_id)
);

CREATE TABLE links (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id     INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    addon_id INTEGER NOT NULL REFERENCES addons(id) ON DELETE CASCADE,
    alias      TEXT    NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL,
    UNIQUE (app_id, addon_id)
);
