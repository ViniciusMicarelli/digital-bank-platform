-- Purchases can be charged to the credit line (credit-service); then
-- transaction_id holds the credit hold id.
ALTER TABLE authorizations ADD COLUMN function TEXT NOT NULL DEFAULT 'DEBIT' CHECK (function IN ('DEBIT', 'CREDIT'));
