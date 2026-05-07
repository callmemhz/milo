PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;

CREATE TABLE users (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    username    TEXT    NOT NULL,
    is_admin    BOOLEAN NOT NULL DEFAULT 0,
    created_at  TIMESTAMP NOT NULL,
    deleted_at  TIMESTAMP
);
CREATE UNIQUE INDEX users_username_active ON users(username) WHERE deleted_at IS NULL;

CREATE TABLE apps (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    name               TEXT    NOT NULL,
    port               INTEGER NOT NULL DEFAULT 8080,
    health_path        TEXT    NOT NULL DEFAULT '/',
    health_timeout_sec INTEGER NOT NULL DEFAULT 30,
    cpu_limit          REAL    NOT NULL DEFAULT 0.5,
    memory_limit_mb    INTEGER NOT NULL DEFAULT 512,
    current_deploy_id  INTEGER,
    created_at         TIMESTAMP NOT NULL,
    deleted_at         TIMESTAMP
);
CREATE UNIQUE INDEX apps_name_active ON apps(name) WHERE deleted_at IS NULL;

CREATE TABLE app_owners (
    app_id  INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (app_id, user_id)
);

CREATE TABLE app_env (
    app_id INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    key    TEXT    NOT NULL,
    value  TEXT    NOT NULL,
    PRIMARY KEY (app_id, key)
);

CREATE TABLE tokens (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    token_hash   TEXT    NOT NULL UNIQUE,
    kind         TEXT    NOT NULL CHECK (kind IN ('user','deploy')),
    user_id      INTEGER REFERENCES users(id),
    app_id       INTEGER REFERENCES apps(id),
    name         TEXT,
    created_at   TIMESTAMP NOT NULL,
    last_used_at TIMESTAMP,
    revoked_at   TIMESTAMP,
    CHECK (
        (kind = 'user'   AND user_id IS NOT NULL AND app_id IS NULL) OR
        (kind = 'deploy' AND app_id  IS NOT NULL AND user_id IS NULL)
    )
);
CREATE INDEX tokens_app ON tokens(app_id) WHERE app_id IS NOT NULL;
CREATE INDEX tokens_user ON tokens(user_id) WHERE user_id IS NOT NULL;

CREATE TABLE deployments (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id          INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    image_digest    TEXT    NOT NULL,
    image_ref       TEXT    NOT NULL,
    commit_sha      TEXT,
    ref             TEXT,
    status          TEXT    NOT NULL,
    failure_reason  TEXT,
    triggered_by    INTEGER NOT NULL REFERENCES tokens(id),
    container_name  TEXT,
    created_at      TIMESTAMP NOT NULL,
    finished_at     TIMESTAMP
);
