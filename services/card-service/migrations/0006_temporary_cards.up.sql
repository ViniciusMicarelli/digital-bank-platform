-- Temporary (single-use) virtual cards: cancelled by their first approved
-- purchase, unusable after valid_until.
ALTER TABLE cards DROP CONSTRAINT cards_kind_check;
ALTER TABLE cards ADD CONSTRAINT cards_kind_check CHECK (kind IN ('PHYSICAL', 'VIRTUAL', 'TEMPORARY'));
ALTER TABLE cards ADD COLUMN valid_until TIMESTAMPTZ;
