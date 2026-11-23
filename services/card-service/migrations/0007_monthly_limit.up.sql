-- A spending limit per card and calendar month (Brasília), set by the
-- customer; 0 means none (only the account's balance or credit limit).
ALTER TABLE cards ADD COLUMN monthly_limit BIGINT NOT NULL DEFAULT 0 CHECK (monthly_limit >= 0);
CREATE INDEX authorizations_card_month ON authorizations (card_id, created_at)
    WHERE status IN ('APPROVED', 'CLEARED', 'PENDING_REVERSAL');
