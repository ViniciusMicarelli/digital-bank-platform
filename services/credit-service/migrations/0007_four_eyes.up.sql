-- Large limit increases need a second analyst (four-eyes): who approved it.
ALTER TABLE limit_requests ADD COLUMN second_approver TEXT NOT NULL DEFAULT '';
ALTER TABLE limit_requests ADD CONSTRAINT limit_requests_four_eyes CHECK (second_approver = '' OR second_approver <> decided_by);
