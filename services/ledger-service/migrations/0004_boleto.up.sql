-- Boleto payments wait here until the clearing network settles them with the
-- payee's bank.
INSERT INTO ledger_accounts (id, kind, currency, code)
VALUES ('00000000-0000-0000-0000-000000000006', 'INTERNAL', 'BRL', 'boleto-settlement');
INSERT INTO balances (account_id) VALUES ('00000000-0000-0000-0000-000000000006');
