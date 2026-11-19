-- Refunds of cleared purchases (merchant refunds and won disputes) and
-- customer disputes (chargebacks).
ALTER TABLE authorizations ADD COLUMN refunded_amount BIGINT NOT NULL DEFAULT 0
    CHECK (refunded_amount >= 0 AND refunded_amount <= cleared_amount);

CREATE TABLE refunds (
    id               UUID PRIMARY KEY,
    authorization_id UUID        NOT NULL REFERENCES authorizations (id),
    account_id       UUID        NOT NULL,
    amount           BIGINT      NOT NULL CHECK (amount > 0),
    source           TEXT        NOT NULL CHECK (source IN ('MERCHANT', 'DISPUTE')),
    -- The network's refund id, or the dispute id: one refund each.
    reference        TEXT        NOT NULL,
    status           TEXT        NOT NULL CHECK (status IN ('PENDING', 'COMPLETED', 'FAILED')),
    failure_reason   TEXT        NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (authorization_id, reference)
);
CREATE INDEX refunds_pending ON refunds (updated_at) WHERE status = 'PENDING';

CREATE TABLE disputes (
    id               UUID PRIMARY KEY,
    authorization_id UUID        NOT NULL REFERENCES authorizations (id),
    card_id          UUID        NOT NULL REFERENCES cards (id),
    account_id       UUID        NOT NULL,
    reason           TEXT        NOT NULL CHECK (reason IN ('NOT_RECOGNIZED', 'NOT_RECEIVED', 'DUPLICATE', 'WRONG_AMOUNT', 'CANCELLED')),
    amount           BIGINT      NOT NULL CHECK (amount > 0),
    description      TEXT        NOT NULL DEFAULT '',
    status           TEXT        NOT NULL CHECK (status IN ('OPEN', 'WON', 'LOST')),
    decided_by       TEXT        NOT NULL DEFAULT '',
    decision_reason  TEXT        NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL,
    decided_at       TIMESTAMPTZ
);
-- One open dispute per purchase.
CREATE UNIQUE INDEX disputes_one_open ON disputes (authorization_id) WHERE status = 'OPEN';
CREATE INDEX disputes_account ON disputes (account_id, created_at DESC);
CREATE INDEX disputes_status ON disputes (status, created_at);
