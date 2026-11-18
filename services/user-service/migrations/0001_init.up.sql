CREATE TABLE profiles (
    customer_id  TEXT PRIMARY KEY,
    full_name    TEXT        NOT NULL,
    -- CPF encrypted (AES-GCM, data key from kms-service) and a blind index
    -- (HMAC) to find duplicates without decrypting.
    cpf_enc      BYTEA       NOT NULL,
    cpf_index    BYTEA       NOT NULL,
    birth_date   DATE        NOT NULL,
    phone        TEXT        NOT NULL DEFAULT '',
    postal_code  CHAR(8)     NOT NULL,
    street       TEXT        NOT NULL,
    number       TEXT        NOT NULL,
    complement   TEXT        NOT NULL DEFAULT '',
    district     TEXT        NOT NULL DEFAULT '',
    city         TEXT        NOT NULL,
    state        CHAR(2)     NOT NULL,
    kyc_status   TEXT        NOT NULL CHECK (kyc_status IN ('NOT_STARTED', 'PENDING', 'APPROVED', 'REJECTED', 'MANUAL_REVIEW')),
    kyc_reason   TEXT        NOT NULL DEFAULT '',
    decided_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- One person, one customer: a CPF can be in verification or verified once.
CREATE UNIQUE INDEX profiles_cpf_in_use ON profiles (cpf_index) WHERE kyc_status IN ('PENDING', 'MANUAL_REVIEW', 'APPROVED');
CREATE INDEX profiles_pending ON profiles (updated_at) WHERE kyc_status = 'PENDING';

CREATE TABLE outbox (
    id           UUID PRIMARY KEY,
    topic        TEXT        NOT NULL,
    key          TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);
CREATE INDEX outbox_unpublished ON outbox (created_at) WHERE published_at IS NULL;
