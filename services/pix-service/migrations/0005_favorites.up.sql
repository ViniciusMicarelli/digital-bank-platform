-- Favorite Pix contacts. Recent ones come from the transfers themselves.
CREATE TABLE pix_favorites (
    account_id UUID        NOT NULL,
    key        TEXT        NOT NULL,
    name       TEXT        NOT NULL DEFAULT '',
    bank_name  TEXT        NOT NULL DEFAULT '',
    nickname   TEXT        NOT NULL DEFAULT '' CHECK (length(nickname) <= 30),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, key)
);
CREATE INDEX pix_transfers_contacts ON pix_transfers (account_id, key, created_at DESC)
    WHERE direction = 'OUTGOING' AND status = 'COMPLETED';
