-- Which account each card belongs to (card.issued), to place card events
-- that do not name the account.
CREATE TABLE cards_view (
    card_id    UUID PRIMARY KEY,
    account_id UUID NOT NULL
);

-- Money spent, one row per purchase, Pix / transfer sent or bill paid;
-- refunds are negative rows in the purchase's category.
CREATE TABLE spending (
    id          UUID PRIMARY KEY,
    account_id  UUID        NOT NULL,
    ref         TEXT        NOT NULL,
    category    TEXT        NOT NULL,
    merchant    TEXT        NOT NULL DEFAULT '',
    amount      BIGINT      NOT NULL CHECK (amount <> 0),
    occurred_at TIMESTAMPTZ NOT NULL,
    UNIQUE (account_id, ref)
);
CREATE INDEX spending_account_time ON spending (account_id, occurred_at);
CREATE INDEX spending_ref ON spending (ref);
