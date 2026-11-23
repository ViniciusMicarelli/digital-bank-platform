-- Round-up ("arredondamento"): each debit card purchase is rounded up to
-- the next real and the difference goes into one box of the account.
ALTER TABLE boxes ADD COLUMN round_up BOOLEAN NOT NULL DEFAULT false;
CREATE UNIQUE INDEX boxes_one_round_up ON boxes (account_id) WHERE round_up AND status = 'ACTIVE';
