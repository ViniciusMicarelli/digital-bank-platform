CREATE TABLE transactions (
    id               UUID PRIMARY KEY,
    idempotency_key  TEXT        NOT NULL UNIQUE,
    fingerprint      BYTEA       NOT NULL,
    type             TEXT        NOT NULL CHECK (type IN ('INTERNAL_TRANSFER', 'PIX_OUT', 'PIX_IN')),
    status           TEXT        NOT NULL CHECK (status IN ('PENDING', 'RESERVED', 'COMPLETED', 'COMPENSATING', 'FAILED')),
    from_account_id  UUID        NOT NULL,
    to_account_id    UUID        NOT NULL,
    amount           BIGINT      NOT NULL CHECK (amount > 0),
    currency         CHAR(3)     NOT NULL,
    description      TEXT        NOT NULL DEFAULT '',
    metadata         JSONB       NOT NULL DEFAULT '{}',
    limit_consumed   BOOLEAN     NOT NULL DEFAULT false,
    hold_id          UUID,
    entry_id         UUID,
    failure_reason   TEXT        NOT NULL DEFAULT '',
    -- Lease: while in the future, one worker owns the saga.
    locked_until     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Recovery worker scans unfinished sagas.
CREATE INDEX transactions_unfinished ON transactions (updated_at)
    WHERE status IN ('PENDING', 'RESERVED', 'COMPENSATING');

-- Audit log of every saga step.
CREATE TABLE saga_steps (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    transaction_id  UUID        NOT NULL REFERENCES transactions (id),
    step            TEXT        NOT NULL,
    outcome         TEXT        NOT NULL CHECK (outcome IN ('OK', 'REJECTED', 'ERROR')),
    detail          TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX saga_steps_transaction ON saga_steps (transaction_id, id);

CREATE TABLE outbox (
    id           UUID PRIMARY KEY,
    topic        TEXT        NOT NULL,
    key          TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);
CREATE INDEX outbox_unpublished ON outbox (created_at) WHERE published_at IS NULL;
