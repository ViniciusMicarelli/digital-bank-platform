-- Pix Automático: a payer's authorization for a receiver to charge them.
CREATE TABLE pix_mandates (
    id                   UUID PRIMARY KEY,
    receiver_account_id  UUID        NOT NULL,
    receiver_key         TEXT        NOT NULL,
    receiver_name        TEXT        NOT NULL,
    payer_account_id     UUID        NOT NULL,
    max_amount           BIGINT      NOT NULL CHECK (max_amount > 0),
    frequency            TEXT        NOT NULL CHECK (frequency IN ('WEEKLY', 'MONTHLY')),
    description          TEXT        NOT NULL,
    status               TEXT        NOT NULL CHECK (status IN ('PENDING', 'ACTIVE', 'REJECTED', 'REVOKED')),
    last_charge_period   TEXT        NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX pix_mandates_payer ON pix_mandates (payer_account_id);
CREATE INDEX pix_mandates_receiver ON pix_mandates (receiver_account_id);

-- Scheduled and recurring Pix (also used for Pix Automático charges).
CREATE TABLE pix_schedules (
    id               UUID PRIMARY KEY,
    idempotency_key  TEXT        NOT NULL UNIQUE,
    account_id       UUID        NOT NULL,
    key              TEXT        NOT NULL,
    amount           BIGINT      NOT NULL CHECK (amount > 0),
    currency         CHAR(3)     NOT NULL,
    description      TEXT        NOT NULL DEFAULT '',
    start_date       DATE        NOT NULL,
    frequency        TEXT        NOT NULL CHECK (frequency IN ('ONCE', 'WEEKLY', 'MONTHLY')),
    occurrences      INT         NOT NULL DEFAULT 0,
    executed         INT         NOT NULL DEFAULT 0,
    next_date        DATE        NOT NULL,
    status           TEXT        NOT NULL CHECK (status IN ('ACTIVE', 'CANCELLED', 'COMPLETED')),
    mandate_id       UUID REFERENCES pix_mandates (id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX pix_schedules_due ON pix_schedules (next_date) WHERE status = 'ACTIVE';
CREATE INDEX pix_schedules_account ON pix_schedules (account_id, created_at DESC);

-- One row per execution: what happened on each occurrence.
CREATE TABLE pix_schedule_runs (
    schedule_id  UUID        NOT NULL REFERENCES pix_schedules (id),
    occurrence   INT         NOT NULL,
    run_date     DATE        NOT NULL,
    transfer_id  UUID,
    status       TEXT        NOT NULL,
    reason       TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (schedule_id, occurrence)
);
