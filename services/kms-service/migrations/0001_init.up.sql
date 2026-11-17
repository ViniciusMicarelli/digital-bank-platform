-- Master keys, one row per (name, version). The key material is encrypted
-- under the KMS root key (an HSM in production). Old versions are kept so
-- data keys wrapped by them can still be unwrapped.
CREATE TABLE master_keys (
    name        TEXT        NOT NULL,
    version     INT         NOT NULL CHECK (version > 0),
    key_enc     BYTEA       NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (name, version)
);
