ALTER TABLE transactions DROP CONSTRAINT transactions_type_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_type_check
    CHECK (type IN ('INTERNAL_TRANSFER', 'PIX_OUT', 'PIX_IN', 'DEBIT_CARD_PURCHASE'));
