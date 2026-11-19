-- The bank's cost of the yield paid on savings boxes (caixinhas).
INSERT INTO ledger_accounts (id, kind, currency, code)
VALUES ('00000000-0000-0000-0000-000000000007', 'INTERNAL', 'BRL', 'savings-yield');
INSERT INTO balances (account_id) VALUES ('00000000-0000-0000-0000-000000000007');
