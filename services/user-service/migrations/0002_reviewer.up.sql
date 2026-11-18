-- Who decided a manual review (audit), and when the review was requested.
ALTER TABLE profiles ADD COLUMN reviewed_by TEXT NOT NULL DEFAULT '';
CREATE INDEX profiles_manual_review ON profiles (updated_at) WHERE kyc_status = 'MANUAL_REVIEW';
