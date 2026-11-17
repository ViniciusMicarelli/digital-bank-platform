-- Contract step of the CPF encryption (expand was 0002). It refuses to run
-- while any row still has a plaintext CPF and no ciphertext: deploy the
-- release that encrypts legacy rows on startup first.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM accounts WHERE document IS NOT NULL OR document_enc IS NULL) THEN
        RAISE EXCEPTION 'accounts still hold plaintext or unencrypted documents; run the backfill release first';
    END IF;
END $$;

ALTER TABLE accounts
    DROP COLUMN document,
    ALTER COLUMN document_enc SET NOT NULL,
    ALTER COLUMN document_index SET NOT NULL;
