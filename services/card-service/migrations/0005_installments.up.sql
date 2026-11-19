-- Credit purchases split into interest-free installments (1 = paid at once).
ALTER TABLE authorizations ADD COLUMN installments SMALLINT NOT NULL DEFAULT 1 CHECK (installments BETWEEN 1 AND 12);
