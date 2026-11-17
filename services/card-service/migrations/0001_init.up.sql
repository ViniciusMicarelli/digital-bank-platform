CREATE TABLE cards (
    id               UUID PRIMARY KEY,
    idempotency_key  TEXT        NOT NULL UNIQUE,
    account_id       UUID        NOT NULL,
    kind             TEXT        NOT NULL CHECK (kind IN ('PHYSICAL', 'VIRTUAL')),
    status           TEXT        NOT NULL CHECK (status IN ('ACTIVE', 'BLOCKED', 'CANCELLED')),
    status_reason    TEXT        NOT NULL DEFAULT '',
    last4            CHAR(4)     NOT NULL,
    expiry_month     INT         NOT NULL CHECK (expiry_month BETWEEN 1 AND 12),
    expiry_year      INT         NOT NULL,
    holder_name      TEXT        NOT NULL,
    online           BOOLEAN     NOT NULL DEFAULT true,
    international    BOOLEAN     NOT NULL DEFAULT false,
    contactless      BOOLEAN     NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX cards_account ON cards (account_id);

-- The vault: the only place the card number exists, encrypted (AES-256-GCM
-- bound to the card id) with a keyed blind index for lookups by PAN.
-- The CVV is never stored: it is derived with the issuer's key.
-- In production this is a separate service, database and key (PCI DSS scope).
CREATE TABLE card_vault (
    card_id    UUID PRIMARY KEY REFERENCES cards (id),
    pan_enc    BYTEA NOT NULL,
    pan_index  BYTEA NOT NULL UNIQUE
);

CREATE TABLE authorizations (
    id                 UUID PRIMARY KEY,
    network_reference  TEXT        NOT NULL UNIQUE,
    card_id            UUID REFERENCES cards (id),
    status             TEXT        NOT NULL CHECK (status IN ('APPROVED', 'DECLINED', 'CLEARED', 'REVERSED', 'PENDING_REVERSAL')),
    response_code      CHAR(2)     NOT NULL,
    reason             TEXT        NOT NULL DEFAULT '',
    amount             BIGINT      NOT NULL CHECK (amount > 0),
    currency           CHAR(3)     NOT NULL,
    cleared_amount     BIGINT      NOT NULL DEFAULT 0,
    merchant_name      TEXT        NOT NULL,
    mcc                CHAR(4)     NOT NULL,
    country            CHAR(2)     NOT NULL,
    channel            TEXT        NOT NULL,
    transaction_id     UUID,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX authorizations_card ON authorizations (card_id, created_at DESC);
CREATE INDEX authorizations_pending_reversal ON authorizations (updated_at) WHERE status = 'PENDING_REVERSAL';

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
