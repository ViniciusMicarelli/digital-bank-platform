-- Card events are keyed per purchase or per refund, so a refund can arrive
-- before its purchase (another partition): it waits here until the
-- purchase is projected.
CREATE TABLE pending_refunds (
    refund_id        TEXT PRIMARY KEY,
    authorization_id TEXT        NOT NULL,
    amount           BIGINT      NOT NULL CHECK (amount > 0),
    occurred_at      TIMESTAMPTZ NOT NULL
);
CREATE INDEX pending_refunds_purchase ON pending_refunds (authorization_id);
