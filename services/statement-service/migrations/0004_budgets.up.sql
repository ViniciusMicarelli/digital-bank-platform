-- Monthly budgets per spending category (or TOTAL), set by the customer.
-- Unlike the feed, this is owned data: replaying events does not rebuild it.
CREATE TABLE budgets (
    account_id UUID        NOT NULL,
    category   TEXT        NOT NULL,
    amount     BIGINT      NOT NULL CHECK (amount > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, category)
);

-- Each alert (80% / 100% of a budget) is sent once per month.
CREATE TABLE budget_alerts (
    account_id UUID        NOT NULL,
    category   TEXT        NOT NULL,
    month      TEXT        NOT NULL, -- YYYY-MM, Brasília
    threshold  INT         NOT NULL CHECK (threshold IN (80, 100)),
    spent      BIGINT      NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, category, month, threshold)
);

-- Budget alerts go out through the transactional outbox.
CREATE TABLE outbox (
    id           UUID PRIMARY KEY,
    topic        TEXT        NOT NULL,
    key          TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);
CREATE INDEX outbox_unpublished ON outbox (created_at) WHERE published_at IS NULL;
