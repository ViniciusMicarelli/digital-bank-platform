-- Audit trail of staff actions: append-only and hash-chained, so a changed
-- or removed event breaks the chain (VerifyAudit finds where).
CREATE TABLE audit_events (
    seq         BIGINT PRIMARY KEY,
    actor_id    TEXT NOT NULL,
    action      TEXT NOT NULL,
    target_type TEXT NOT NULL DEFAULT '',
    target_id   TEXT NOT NULL DEFAULT '',
    details     JSONB NOT NULL DEFAULT '{}',
    at          TIMESTAMPTZ NOT NULL,
    prev_hash   TEXT NOT NULL,
    hash        TEXT NOT NULL UNIQUE
);
CREATE INDEX audit_events_target ON audit_events (target_id, seq DESC);
CREATE INDEX audit_events_actor ON audit_events (actor_id, seq DESC);

CREATE FUNCTION audit_events_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER audit_events_append_only BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION audit_events_append_only();
CREATE TRIGGER audit_events_no_truncate BEFORE TRUNCATE ON audit_events
    FOR EACH STATEMENT EXECUTE FUNCTION audit_events_append_only();

-- Four-eyes approvals: requested by one analyst, decided by another.
CREATE TABLE approvals (
    id              UUID PRIMARY KEY,
    kind            TEXT NOT NULL CHECK (kind IN ('UNBLOCK_ACCOUNT')),
    target_id       TEXT NOT NULL,
    reason          TEXT NOT NULL,
    requested_by    TEXT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('PENDING', 'APPROVED', 'EXECUTED', 'REJECTED', 'FAILED', 'EXPIRED')),
    decided_by      TEXT NOT NULL DEFAULT '',
    decision_reason TEXT NOT NULL DEFAULT '',
    failure_reason  TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL,
    decided_at      TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (decided_by = '' OR decided_by <> requested_by)
);
-- One open request per action and target.
CREATE UNIQUE INDEX approvals_one_open ON approvals (kind, target_id) WHERE status IN ('PENDING', 'APPROVED');
CREATE INDEX approvals_status ON approvals (status, created_at);
