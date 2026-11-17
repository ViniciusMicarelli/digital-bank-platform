-- Ledger schema. Money is always BIGINT minor units; postings are signed
-- (positive = credit to the account, negative = debit).

CREATE TABLE ledger_accounts (
    id                    UUID PRIMARY KEY,
    kind                  TEXT        NOT NULL CHECK (kind IN ('CUSTOMER', 'INTERNAL')),
    currency              CHAR(3)     NOT NULL,
    overdraft_limit       BIGINT      NOT NULL DEFAULT 0 CHECK (overdraft_limit >= 0),
    code                  TEXT UNIQUE,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Balance snapshot, updated in the same transaction as the postings.
-- posted  = sum of postings; pending = sum of active holds.
CREATE TABLE balances (
    account_id  UUID PRIMARY KEY REFERENCES ledger_accounts (id),
    posted      BIGINT      NOT NULL DEFAULT 0,
    pending     BIGINT      NOT NULL DEFAULT 0 CHECK (pending >= 0),
    version     BIGINT      NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE journal_entries (
    id                UUID PRIMARY KEY,
    idempotency_key   TEXT        NOT NULL UNIQUE,
    -- SHA-256 of the request, to detect a key reused with a different payload.
    fingerprint       BYTEA       NOT NULL,
    description       TEXT        NOT NULL DEFAULT '',
    metadata          JSONB       NOT NULL DEFAULT '{}',
    -- An entry can be reversed at most once.
    reverses_entry_id UUID UNIQUE REFERENCES journal_entries (id),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE postings (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    entry_id    UUID        NOT NULL REFERENCES journal_entries (id),
    account_id  UUID        NOT NULL REFERENCES ledger_accounts (id),
    amount      BIGINT      NOT NULL CHECK (amount <> 0),
    currency    CHAR(3)     NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (entry_id, account_id)
);
CREATE INDEX postings_account_id ON postings (account_id, id DESC);
CREATE INDEX postings_account_time ON postings (account_id, created_at);

CREATE TABLE holds (
    id                  UUID PRIMARY KEY,
    idempotency_key     TEXT        NOT NULL UNIQUE,
    fingerprint         BYTEA       NOT NULL,
    account_id          UUID        NOT NULL REFERENCES ledger_accounts (id),
    counter_account_id  UUID        NOT NULL REFERENCES ledger_accounts (id),
    amount              BIGINT      NOT NULL CHECK (amount > 0),
    currency            CHAR(3)     NOT NULL,
    status              TEXT        NOT NULL CHECK (status IN ('ACTIVE', 'CAPTURED', 'RELEASED')),
    capture_entry_id    UUID REFERENCES journal_entries (id),
    description         TEXT        NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX holds_active_account ON holds (account_id) WHERE status = 'ACTIVE';

CREATE TABLE outbox (
    id           UUID PRIMARY KEY,
    topic        TEXT        NOT NULL,
    key          TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);
CREATE INDEX outbox_unpublished ON outbox (created_at) WHERE published_at IS NULL;

-- Defense in depth: the application validates entries, and the database
-- refuses to commit an entry whose postings do not sum to zero or mix currencies.
CREATE FUNCTION check_entry_balanced() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    total      NUMERIC;
    currencies INT;
BEGIN
    SELECT COALESCE(SUM(amount), 0), COUNT(DISTINCT currency)
      INTO total, currencies
      FROM postings WHERE entry_id = NEW.entry_id;
    IF total <> 0 THEN
        RAISE EXCEPTION 'journal entry % is unbalanced (sum = %)', NEW.entry_id, total
            USING ERRCODE = 'check_violation';
    END IF;
    IF currencies > 1 THEN
        RAISE EXCEPTION 'journal entry % mixes currencies', NEW.entry_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER postings_balanced
    AFTER INSERT ON postings
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION check_entry_balanced();

-- The journal is append-only: corrections are reversal entries.
CREATE FUNCTION forbid_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME USING ERRCODE = 'insufficient_privilege';
END;
$$;

CREATE TRIGGER journal_entries_append_only
    BEFORE UPDATE OR DELETE ON journal_entries
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TRIGGER postings_append_only
    BEFORE UPDATE OR DELETE ON postings
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- System accounts with well-known IDs, used by other services.
INSERT INTO ledger_accounts (id, kind, currency, code) VALUES
    ('00000000-0000-0000-0000-000000000001', 'INTERNAL', 'BRL', 'bank-funding'),
    ('00000000-0000-0000-0000-000000000002', 'INTERNAL', 'BRL', 'pix-settlement');
INSERT INTO balances (account_id) VALUES
    ('00000000-0000-0000-0000-000000000001'),
    ('00000000-0000-0000-0000-000000000002');
