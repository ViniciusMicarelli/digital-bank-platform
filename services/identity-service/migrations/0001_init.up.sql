-- Customers' credentials. Secrets are never stored in clear text:
-- passwords as argon2id hashes, TOTP seeds encrypted (AES-256-GCM).
CREATE TABLE credentials (
    customer_id       UUID PRIMARY KEY,
    email             TEXT        NOT NULL UNIQUE,
    full_name         TEXT        NOT NULL,
    password_hash     TEXT        NOT NULL,
    totp_secret_enc   BYTEA,
    totp_enabled      BOOLEAN     NOT NULL DEFAULT false,
    -- Last accepted TOTP time step, so a code cannot be replayed.
    totp_last_step    BIGINT      NOT NULL DEFAULT 0,
    failed_attempts   INT         NOT NULL DEFAULT 0,
    locked_until      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Refresh tokens are stored as SHA-256 hashes. All tokens of one login
-- share a family; reusing a rotated token revokes the family.
CREATE TABLE refresh_tokens (
    id           UUID PRIMARY KEY,
    family_id    UUID        NOT NULL,
    customer_id  UUID        NOT NULL REFERENCES credentials (customer_id),
    token_hash   BYTEA       NOT NULL UNIQUE,
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ,
    replaced_by  UUID,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX refresh_tokens_family ON refresh_tokens (family_id);

-- Ed25519 signing keys. The private key is encrypted under the KEK.
-- retired_at: no longer signs. expires_at: no longer published in the JWKS.
CREATE TABLE signing_keys (
    kid               TEXT PRIMARY KEY,
    private_key_enc   BYTEA       NOT NULL,
    public_key        BYTEA       NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    retired_at        TIMESTAMPTZ,
    expires_at        TIMESTAMPTZ
);
-- At most one key signs at a time.
CREATE UNIQUE INDEX signing_keys_one_active ON signing_keys ((true)) WHERE retired_at IS NULL;

CREATE TABLE outbox (
    id           UUID PRIMARY KEY,
    topic        TEXT        NOT NULL,
    key          TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);
CREATE INDEX outbox_unpublished ON outbox (created_at) WHERE published_at IS NULL;
