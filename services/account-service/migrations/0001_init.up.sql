CREATE SEQUENCE account_number_seq START 1001;

CREATE TABLE accounts (
    id               UUID PRIMARY KEY,
    idempotency_key  TEXT        NOT NULL UNIQUE,
    customer_id      TEXT        NOT NULL,
    holder_name      TEXT        NOT NULL,
    document         CHAR(11)    NOT NULL,
    branch           CHAR(4)     NOT NULL,
    number           TEXT        NOT NULL UNIQUE,
    currency         CHAR(3)     NOT NULL,
    status           TEXT        NOT NULL CHECK (status IN ('ACTIVE', 'BLOCKED', 'CLOSED')),
    status_reason    TEXT        NOT NULL DEFAULT '',
    opened_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at        TIMESTAMPTZ,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX accounts_customer ON accounts (customer_id);

CREATE TABLE account_limits (
    account_id       UUID PRIMARY KEY REFERENCES accounts (id),
    per_transaction  BIGINT NOT NULL CHECK (per_transaction > 0),
    daily            BIGINT NOT NULL CHECK (daily > 0),
    nightly          BIGINT NOT NULL CHECK (nightly > 0),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One row per outgoing transaction counted against the limits.
CREATE TABLE limit_usage (
    account_id      UUID        NOT NULL REFERENCES accounts (id),
    transaction_id  TEXT        NOT NULL,
    amount          BIGINT      NOT NULL CHECK (amount > 0),
    local_date      DATE        NOT NULL,
    -- Start date of the night window (20:00-06:00) the usage fell in, if any.
    night_of        DATE,
    restored        BOOLEAN     NOT NULL DEFAULT false,
    used_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, transaction_id)
);
CREATE INDEX limit_usage_day ON limit_usage (account_id, local_date) WHERE NOT restored;
CREATE INDEX limit_usage_night ON limit_usage (account_id, night_of) WHERE NOT restored AND night_of IS NOT NULL;

CREATE TABLE outbox (
    id           UUID PRIMARY KEY,
    topic        TEXT        NOT NULL,
    key          TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);
CREATE INDEX outbox_unpublished ON outbox (created_at) WHERE published_at IS NULL;
