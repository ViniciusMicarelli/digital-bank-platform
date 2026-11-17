CREATE TABLE boleto_payments (
    id               UUID PRIMARY KEY,
    idempotency_key  TEXT        NOT NULL UNIQUE,
    fingerprint      TEXT        NOT NULL,
    account_id       UUID        NOT NULL,
    status           TEXT        NOT NULL CHECK (status IN ('SCHEDULED', 'CREATED', 'FUNDS_RESERVED', 'SENT_TO_CLEARING',
                                                           'COMPLETED', 'REJECTED', 'FAILED', 'CANCELLED')),
    failure_reason   TEXT        NOT NULL DEFAULT '',
    barcode          CHAR(44)    NOT NULL,
    line             CHAR(47)    NOT NULL,
    payee_name       TEXT        NOT NULL DEFAULT '',
    payee_document   TEXT        NOT NULL DEFAULT '',
    payee_bank       TEXT        NOT NULL DEFAULT '',
    due_date         DATE,
    nominal          BIGINT      NOT NULL CHECK (nominal >= 0),
    fine             BIGINT      NOT NULL DEFAULT 0,
    interest         BIGINT      NOT NULL DEFAULT 0,
    scheduled_for    DATE        NOT NULL,
    transaction_id   UUID,
    lease_until      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX boleto_payments_account ON boleto_payments (account_id, created_at DESC);
CREATE INDEX boleto_payments_unfinished ON boleto_payments (updated_at)
    WHERE status IN ('CREATED', 'FUNDS_RESERVED', 'SENT_TO_CLEARING');
CREATE INDEX boleto_payments_scheduled ON boleto_payments (scheduled_for) WHERE status = 'SCHEDULED';
-- A boleto is paid once: no two live payments of the same barcode.
CREATE UNIQUE INDEX boleto_payments_live ON boleto_payments (barcode)
    WHERE status IN ('SCHEDULED', 'CREATED', 'FUNDS_RESERVED', 'SENT_TO_CLEARING', 'COMPLETED');

-- Sandbox: boletos issued for demos and tests, with their payee (the
-- simulated registry reads them).
CREATE TABLE sandbox_boletos (
    barcode         CHAR(44) PRIMARY KEY,
    payee_name      TEXT        NOT NULL,
    payee_document  TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

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
