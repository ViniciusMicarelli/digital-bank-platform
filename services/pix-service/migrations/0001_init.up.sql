CREATE TABLE pix_keys (
    id          UUID PRIMARY KEY,
    account_id  UUID        NOT NULL,
    type        TEXT        NOT NULL CHECK (type IN ('CPF', 'EMAIL', 'PHONE', 'EVP')),
    value       TEXT        NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX pix_keys_account ON pix_keys (account_id);

CREATE TABLE pix_transfers (
    id                 UUID PRIMARY KEY,
    idempotency_key    TEXT        NOT NULL UNIQUE,
    fingerprint        BYTEA       NOT NULL,
    end_to_end_id      CHAR(32)    NOT NULL UNIQUE,
    direction          TEXT        NOT NULL CHECK (direction IN ('OUTGOING', 'INCOMING')),
    status             TEXT        NOT NULL CHECK (status IN ('CREATED', 'FUNDS_RESERVED', 'SENT_TO_SPI', 'COMPLETED', 'FAILED', 'REJECTED')),
    account_id         UUID        NOT NULL,
    key                TEXT        NOT NULL,
    -- Receiver of an outgoing transfer, payer of an incoming one.
    counterparty_name  TEXT        NOT NULL DEFAULT '',
    counterparty_doc   TEXT        NOT NULL DEFAULT '',
    counterparty_ispb  CHAR(8)     NOT NULL,
    counterparty_bank  TEXT        NOT NULL DEFAULT '',
    -- Set when the counterparty is one of our own accounts.
    internal_account_id UUID,
    amount             BIGINT      NOT NULL CHECK (amount > 0),
    currency           CHAR(3)     NOT NULL,
    description        TEXT        NOT NULL DEFAULT '',
    transaction_id     UUID,
    failure_reason     TEXT        NOT NULL DEFAULT '',
    locked_until       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX pix_transfers_unfinished ON pix_transfers (updated_at)
    WHERE status IN ('CREATED', 'FUNDS_RESERVED', 'SENT_TO_SPI');
CREATE INDEX pix_transfers_account ON pix_transfers (account_id, created_at DESC);

CREATE TABLE outbox (
    id           UUID PRIMARY KEY,
    topic        TEXT        NOT NULL,
    key          TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);
CREATE INDEX outbox_unpublished ON outbox (created_at) WHERE published_at IS NULL;

CREATE TABLE inbox (
    consumer     TEXT        NOT NULL,
    event_id     TEXT        NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);
