-- Holders of customer accounts, projected from bank.account.events.
CREATE TABLE accounts_view (
    account_id   UUID PRIMARY KEY,
    holder_name  TEXT        NOT NULL,
    branch       TEXT        NOT NULL,
    number       TEXT        NOT NULL,
    status       TEXT        NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One row per side of a completed transaction that a customer sees.
CREATE TABLE feed_items (
    id                       UUID PRIMARY KEY,
    account_id               UUID        NOT NULL,
    transaction_id           UUID        NOT NULL,
    type                     TEXT        NOT NULL,
    direction                TEXT        NOT NULL CHECK (direction IN ('DEBIT', 'CREDIT')),
    amount                   BIGINT      NOT NULL CHECK (amount > 0),
    currency                 CHAR(3)     NOT NULL,
    -- Name from the event (Pix), else resolved from accounts_view at read time.
    counterparty_name        TEXT,
    counterparty_account_id  UUID,
    description              TEXT        NOT NULL DEFAULT '',
    end_to_end_id            TEXT        NOT NULL DEFAULT '',
    occurred_at              TIMESTAMPTZ NOT NULL,
    UNIQUE (account_id, transaction_id)
);
-- The feed query: one index range scan, newest first.
CREATE INDEX feed_items_account_time ON feed_items (account_id, occurred_at DESC, id DESC);

CREATE TABLE inbox (
    consumer     TEXT        NOT NULL,
    event_id     TEXT        NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);
