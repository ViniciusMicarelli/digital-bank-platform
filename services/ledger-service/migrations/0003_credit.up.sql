-- Credit card accounts. credit-receivables goes negative by what customers
-- owe on their invoices; credit-revenue receives fines and interest.
INSERT INTO ledger_accounts (id, kind, currency, code) VALUES
    ('00000000-0000-0000-0000-000000000004', 'INTERNAL', 'BRL', 'credit-receivables'),
    ('00000000-0000-0000-0000-000000000005', 'INTERNAL', 'BRL', 'credit-revenue');
INSERT INTO balances (account_id) VALUES
    ('00000000-0000-0000-0000-000000000004'),
    ('00000000-0000-0000-0000-000000000005');
