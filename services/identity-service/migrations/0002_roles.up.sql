-- Staff accounts (back office) share the credential store; the role goes into
-- the access token and the gateway checks it on back-office routes.
ALTER TABLE credentials ADD COLUMN role TEXT NOT NULL DEFAULT 'CUSTOMER' CHECK (role IN ('CUSTOMER', 'ANALYST'));
