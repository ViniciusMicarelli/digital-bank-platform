-- Four-eyes manual credits (goodwill, corrections) to a customer account.
ALTER TABLE approvals DROP CONSTRAINT approvals_kind_check;
ALTER TABLE approvals ADD CONSTRAINT approvals_kind_check CHECK (kind IN ('UNBLOCK_ACCOUNT', 'LIMIT_INCREASE', 'MANUAL_CREDIT'));
