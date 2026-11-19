-- Interest-free installment purchases: the whole amount is held on the limit
-- and posted to the ledger at capture; one installment goes on each invoice.
ALTER TABLE holds ADD COLUMN installments SMALLINT NOT NULL DEFAULT 1 CHECK (installments BETWEEN 1 AND 12);

CREATE TABLE installments (
    id          UUID PRIMARY KEY,
    line_id     UUID        NOT NULL REFERENCES credit_lines (id),
    hold_id     UUID        NOT NULL REFERENCES holds (id),
    number      SMALLINT    NOT NULL CHECK (number >= 1),
    total       SMALLINT    NOT NULL CHECK (total BETWEEN 2 AND 12 AND number <= total),
    amount      BIGINT      NOT NULL CHECK (amount > 0),
    description TEXT        NOT NULL,
    -- The invoice it went on; NULL while still to come.
    invoice_id  UUID REFERENCES invoices (id),
    created_at  TIMESTAMPTZ NOT NULL,
    UNIQUE (hold_id, number)
);
CREATE INDEX installments_pending ON installments (line_id, hold_id, number) WHERE invoice_id IS NULL;
