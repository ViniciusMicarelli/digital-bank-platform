-- Wallet / card-on-file tokens: a different card number per device or
-- merchant, mapped to the card in the vault, suspendable on its own.
CREATE TABLE card_tokens (
    id            UUID PRIMARY KEY,
    card_id       UUID        NOT NULL REFERENCES cards (id),
    wallet        TEXT        NOT NULL CHECK (wallet IN ('APPLE_PAY', 'GOOGLE_PAY', 'SAMSUNG_PAY', 'MERCHANT')),
    device_name   TEXT        NOT NULL,
    status        TEXT        NOT NULL CHECK (status IN ('ACTIVE', 'SUSPENDED', 'DELETED')),
    last4         CHAR(4)     NOT NULL,
    expiry_month  INT         NOT NULL,
    expiry_year   INT         NOT NULL,
    pan_enc       BYTEA       NOT NULL,
    pan_index     BYTEA       NOT NULL UNIQUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX card_tokens_card ON card_tokens (card_id);

ALTER TABLE authorizations ADD COLUMN token_id UUID REFERENCES card_tokens (id);
