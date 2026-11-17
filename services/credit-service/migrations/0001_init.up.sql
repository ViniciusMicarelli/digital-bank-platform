CREATE TABLE credit_lines (
    id               UUID PRIMARY KEY,
    account_id       UUID        NOT NULL UNIQUE,
    idempotency_key  TEXT        NOT NULL UNIQUE,
    status           TEXT        NOT NULL CHECK (status IN ('ACTIVE', 'BLOCKED')),
    credit_limit     BIGINT      NOT NULL CHECK (credit_limit > 0),
    -- Held purchases plus everything owed on unpaid invoices.
    used             BIGINT      NOT NULL DEFAULT 0 CHECK (used >= 0),
    closing_day      SMALLINT    NOT NULL CHECK (closing_day BETWEEN 1 AND 28),
    interest_rate_bp INT         NOT NULL CHECK (interest_rate_bp >= 0),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE invoices (
    id               UUID PRIMARY KEY,
    line_id          UUID        NOT NULL REFERENCES credit_lines (id),
    status           TEXT        NOT NULL CHECK (status IN ('OPEN', 'CLOSED', 'PAID', 'OVERDUE')),
    closing_date     DATE        NOT NULL,
    due_date         DATE        NOT NULL,
    total            BIGINT      NOT NULL DEFAULT 0,
    paid             BIGINT      NOT NULL DEFAULT 0,
    -- Payments sent to the ledger and not settled yet.
    pending          BIGINT      NOT NULL DEFAULT 0 CHECK (pending >= 0),
    -- Fine + interest when the invoice went overdue, and whether the ledger
    -- posting of them was made (posted after the commit, retried by the worker).
    charges          BIGINT      NOT NULL DEFAULT 0,
    charges_posted   BOOLEAN     NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (paid + pending <= total OR status = 'OPEN')
);
CREATE UNIQUE INDEX invoices_one_open ON invoices (line_id) WHERE status = 'OPEN';
CREATE INDEX invoices_line ON invoices (line_id, closing_date DESC);
CREATE INDEX invoices_due ON invoices (closing_date) WHERE status = 'OPEN';
CREATE INDEX invoices_closed ON invoices (due_date) WHERE status = 'CLOSED';
CREATE INDEX invoices_unposted ON invoices (line_id) WHERE NOT charges_posted;

CREATE TABLE invoice_items (
    id           UUID PRIMARY KEY,
    invoice_id   UUID        NOT NULL REFERENCES invoices (id),
    kind         TEXT        NOT NULL CHECK (kind IN ('PURCHASE', 'PAYMENT', 'BALANCE_CARRIED', 'FINE', 'INTEREST')),
    amount       BIGINT      NOT NULL CHECK (amount > 0),
    description  TEXT        NOT NULL DEFAULT '',
    reference    TEXT        NOT NULL,
    occurred_at  TIMESTAMPTZ NOT NULL,
    UNIQUE (invoice_id, kind, reference)
);

CREATE TABLE holds (
    id               UUID PRIMARY KEY,
    idempotency_key  TEXT        NOT NULL UNIQUE,
    line_id          UUID REFERENCES credit_lines (id),
    account_id       UUID        NOT NULL,
    status           TEXT        NOT NULL CHECK (status IN ('RESERVED', 'CAPTURED', 'RELEASED', 'FAILED')),
    failure_reason   TEXT        NOT NULL DEFAULT '',
    amount           BIGINT      NOT NULL CHECK (amount > 0),
    captured         BIGINT      NOT NULL DEFAULT 0,
    description      TEXT        NOT NULL DEFAULT '',
    metadata         JSONB       NOT NULL DEFAULT '{}',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX holds_reserved ON holds (created_at) WHERE status = 'RESERVED';

CREATE TABLE payments (
    id               UUID PRIMARY KEY,
    idempotency_key  TEXT        NOT NULL UNIQUE,
    invoice_id       UUID        NOT NULL REFERENCES invoices (id),
    account_id       UUID        NOT NULL,
    status           TEXT        NOT NULL CHECK (status IN ('PENDING', 'COMPLETED', 'FAILED')),
    failure_reason   TEXT        NOT NULL DEFAULT '',
    amount           BIGINT      NOT NULL CHECK (amount > 0),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX payments_pending ON payments (created_at) WHERE status = 'PENDING';

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
