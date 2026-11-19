-- Savings boxes. The money lives in the ledger: each box is a ledger
-- customer account with the box's id, so it can never go negative.
CREATE TABLE boxes (
    id                UUID PRIMARY KEY,
    idempotency_key   TEXT        NOT NULL UNIQUE,
    account_id        UUID        NOT NULL,
    name              TEXT        NOT NULL CHECK (length(name) BETWEEN 1 AND 40),
    goal              BIGINT      NOT NULL DEFAULT 0 CHECK (goal >= 0),
    status            TEXT        NOT NULL CHECK (status IN ('ACTIVE', 'CLOSED')),
    yielded           BIGINT      NOT NULL DEFAULT 0,
    -- The fraction of a centavo of yield not paid yet, in millionths.
    yield_carry_micro BIGINT      NOT NULL DEFAULT 0 CHECK (yield_carry_micro BETWEEN 0 AND 999999),
    -- The last day whose yield was paid (Brasília time).
    last_accrued      DATE        NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX boxes_account ON boxes (account_id, created_at);
CREATE INDEX boxes_due ON boxes (last_accrued) WHERE status = 'ACTIVE';

CREATE TABLE movements (
    id              UUID PRIMARY KEY,
    box_id          UUID        NOT NULL REFERENCES boxes (id),
    idempotency_key TEXT        NOT NULL,
    kind            TEXT        NOT NULL CHECK (kind IN ('DEPOSIT', 'WITHDRAWAL', 'YIELD')),
    amount          BIGINT      NOT NULL CHECK (amount > 0),
    status          TEXT        NOT NULL CHECK (status IN ('PENDING', 'COMPLETED', 'FAILED')),
    failure_reason  TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (box_id, idempotency_key)
);
CREATE INDEX movements_box ON movements (box_id, created_at DESC);
CREATE INDEX movements_pending ON movements (created_at) WHERE status = 'PENDING';
