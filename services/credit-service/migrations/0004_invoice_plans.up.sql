-- A closed invoice split into installments with interest ("parcelamento da
-- fatura"): the invoice is settled by the plan, whose installments go on the
-- next invoices like those of an installment purchase.
CREATE TABLE invoice_plans (
    id                 UUID PRIMARY KEY,
    idempotency_key    TEXT        NOT NULL UNIQUE,
    invoice_id         UUID        NOT NULL UNIQUE REFERENCES invoices (id),
    line_id            UUID        NOT NULL REFERENCES credit_lines (id),
    principal          BIGINT      NOT NULL CHECK (principal > 0),
    installments       SMALLINT    NOT NULL CHECK (installments BETWEEN 2 AND 12),
    installment_amount BIGINT      NOT NULL CHECK (installment_amount > 0),
    monthly_rate_bp    INT         NOT NULL CHECK (monthly_rate_bp >= 0),
    -- Whether the interest was booked in the ledger (after the commit, retried).
    interest_posted    BOOLEAN     NOT NULL DEFAULT false,
    created_at         TIMESTAMPTZ NOT NULL
);
CREATE INDEX invoice_plans_unposted ON invoice_plans (created_at) WHERE NOT interest_posted;

ALTER TABLE installments ALTER COLUMN hold_id DROP NOT NULL;
ALTER TABLE installments ADD COLUMN plan_id UUID REFERENCES invoice_plans (id);
ALTER TABLE installments ADD CONSTRAINT installments_one_source CHECK ((hold_id IS NULL) <> (plan_id IS NULL));
ALTER TABLE installments DROP CONSTRAINT installments_hold_id_number_key;
CREATE UNIQUE INDEX installments_source_number ON installments (COALESCE(hold_id, plan_id), number);

ALTER TABLE invoice_items DROP CONSTRAINT invoice_items_kind_check;
ALTER TABLE invoice_items ADD CONSTRAINT invoice_items_kind_check
    CHECK (kind IN ('PURCHASE', 'PAYMENT', 'BALANCE_CARRIED', 'FINE', 'INTEREST',
                    'REFUND', 'CREDIT_CARRIED_IN', 'CREDIT_CARRIED_OUT', 'INVOICE_PLAN'));
