-- Card refunds on captured credit purchases (merchant refunds, won disputes).
CREATE TABLE refunds (
    id              UUID PRIMARY KEY,
    idempotency_key TEXT        NOT NULL UNIQUE,
    line_id         UUID        NOT NULL REFERENCES credit_lines (id),
    account_id      UUID        NOT NULL,
    invoice_id      UUID        NOT NULL REFERENCES invoices (id),
    amount          BIGINT      NOT NULL CHECK (amount > 0),
    description     TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Credits on an invoice: refunds, and a credit larger than the invoice
-- moved to the next one when it closes.
ALTER TABLE invoice_items DROP CONSTRAINT invoice_items_kind_check;
ALTER TABLE invoice_items ADD CONSTRAINT invoice_items_kind_check
    CHECK (kind IN ('PURCHASE', 'PAYMENT', 'BALANCE_CARRIED', 'FINE', 'INTEREST',
                    'REFUND', 'CREDIT_CARRIED_IN', 'CREDIT_CARRIED_OUT'));
