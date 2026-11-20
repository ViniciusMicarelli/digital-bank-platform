-- Credit limit changes: decreases apply at once; increases go through an
-- automatic analysis and, above what it may grant, an analyst.
CREATE TABLE limit_requests (
    id              UUID PRIMARY KEY,
    idempotency_key TEXT        NOT NULL UNIQUE,
    line_id         UUID        NOT NULL REFERENCES credit_lines (id),
    account_id      UUID        NOT NULL,
    current_limit   BIGINT      NOT NULL,
    requested       BIGINT      NOT NULL CHECK (requested > 0),
    status          TEXT        NOT NULL CHECK (status IN ('APPROVED', 'REJECTED', 'IN_REVIEW')),
    granted         BIGINT      NOT NULL DEFAULT 0,
    reason          TEXT        NOT NULL DEFAULT '',
    decided_by      TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL,
    decided_at      TIMESTAMPTZ
);
-- One request in review per line.
CREATE UNIQUE INDEX limit_requests_one_open ON limit_requests (line_id) WHERE status = 'IN_REVIEW';
CREATE INDEX limit_requests_status ON limit_requests (status, created_at);
CREATE INDEX limit_requests_line ON limit_requests (line_id, created_at DESC);
