-- Credits made by the back office (goodwill, corrections), each approved by
-- two analysts: the bank's cost of them.
INSERT INTO ledger_accounts (id, kind, currency, code)
VALUES ('00000000-0000-0000-0000-000000000008', 'INTERNAL', 'BRL', 'manual-adjustments');
INSERT INTO balances (account_id) VALUES ('00000000-0000-0000-0000-000000000008');
