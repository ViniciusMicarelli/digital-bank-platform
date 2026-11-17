-- QR codes issued by our customers ("cobranças"). A one-time code is paid at
-- most once: paid_transfer_id is claimed before any money moves.
CREATE TABLE qr_codes (
    txid              TEXT PRIMARY KEY CHECK (txid ~ '^[A-Za-z0-9]{1,25}$'),
    account_id        UUID        NOT NULL,
    key               TEXT        NOT NULL,
    amount            BIGINT      CHECK (amount > 0),
    description       TEXT        NOT NULL DEFAULT '',
    one_time          BOOLEAN     NOT NULL,
    expires_at        TIMESTAMPTZ,
    paid_transfer_id  UUID,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX qr_codes_account ON qr_codes (account_id, created_at DESC);

ALTER TABLE pix_transfers
    -- Charge being paid, when the transfer came from a QR code.
    ADD COLUMN qr_txid          TEXT,
    -- Set on a refund: the incoming transfer it gives money back for.
    ADD COLUMN refund_of        UUID REFERENCES pix_transfers (id),
    -- On an incoming transfer: total reserved by refunds in flight or done.
    -- The database itself refuses refunding more than was received.
    ADD COLUMN refunded_amount  BIGINT NOT NULL DEFAULT 0,
    ADD CONSTRAINT refunded_within_amount CHECK (refunded_amount >= 0 AND refunded_amount <= amount);
