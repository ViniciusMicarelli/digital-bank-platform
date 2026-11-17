-- Expand step of encrypting the CPF at rest (ADR-0015).
-- document_enc: AES-256-GCM ciphertext; document_index: HMAC blind index for lookups.
-- On startup the service encrypts legacy rows and clears the plaintext column;
-- a later release drops it (contract step) once every replica runs this code.
ALTER TABLE accounts
    ADD COLUMN document_enc   BYTEA,
    ADD COLUMN document_index BYTEA,
    ALTER COLUMN document DROP NOT NULL;

CREATE INDEX accounts_document_index ON accounts (document_index);
