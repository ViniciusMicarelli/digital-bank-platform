-- Four-eyes on large credit limit increases; an approval now carries the
-- parameters of its action (e.g. the amount to grant).
ALTER TABLE approvals DROP CONSTRAINT approvals_kind_check;
ALTER TABLE approvals ADD CONSTRAINT approvals_kind_check CHECK (kind IN ('UNBLOCK_ACCOUNT', 'LIMIT_INCREASE'));
ALTER TABLE approvals ADD COLUMN params JSONB NOT NULL DEFAULT '{}';
