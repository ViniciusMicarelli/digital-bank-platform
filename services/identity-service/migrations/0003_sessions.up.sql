-- Sessions: one per login (the family of its refresh tokens), with the
-- device it came from, so customers can see where they are signed in and
-- end a session. Logins from before this table have no row (not listed).
CREATE TABLE sessions (
    id           UUID PRIMARY KEY, -- refresh_tokens.family_id
    customer_id  UUID        NOT NULL REFERENCES credentials (customer_id),
    device       TEXT        NOT NULL,
    user_agent   TEXT        NOT NULL DEFAULT '',
    ip           TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ
);
CREATE INDEX sessions_customer ON sessions (customer_id, last_used_at DESC) WHERE revoked_at IS NULL;
CREATE INDEX sessions_customer_device ON sessions (customer_id, user_agent);
