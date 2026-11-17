-- Internal account that receives card purchases until they are settled with
-- the card network.
INSERT INTO ledger_accounts (id, kind, currency, code)
VALUES ('00000000-0000-0000-0000-000000000003', 'INTERNAL', 'BRL', 'card-clearing');
INSERT INTO balances (account_id) VALUES ('00000000-0000-0000-0000-000000000003');
