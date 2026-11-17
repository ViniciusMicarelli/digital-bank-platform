-- Which customer owns each account (from account.opened events, or asked to
-- account-service when an event arrives first).
CREATE TABLE account_owners (
    account_id   UUID PRIMARY KEY,
    customer_id  TEXT NOT NULL
);

CREATE TABLE preferences (
    customer_id  TEXT PRIMARY KEY,
    push         BOOLEAN     NOT NULL,
    email        BOOLEAN     NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE notifications (
    id           UUID PRIMARY KEY,
    customer_id  TEXT        NOT NULL,
    account_id   UUID        NOT NULL,
    event_id     TEXT        NOT NULL,
    category     TEXT        NOT NULL,
    title        TEXT        NOT NULL,
    body         TEXT        NOT NULL,
    important    BOOLEAN     NOT NULL,
    channels     TEXT[]      NOT NULL,
    occurred_at  TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at      TIMESTAMPTZ,
    UNIQUE (event_id, account_id)
);
CREATE INDEX notifications_customer ON notifications (customer_id, id DESC);
CREATE INDEX notifications_unread ON notifications (customer_id) WHERE read_at IS NULL;

-- Push and e-mail deliveries, sent after the commit by the dispatcher.
CREATE TABLE deliveries (
    notification_id UUID        NOT NULL REFERENCES notifications (id),
    channel         TEXT        NOT NULL CHECK (channel IN ('PUSH', 'EMAIL')),
    status          TEXT        NOT NULL CHECK (status IN ('PENDING', 'SENT', 'FAILED')),
    attempts        INT         NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (notification_id, channel)
);
CREATE INDEX deliveries_pending ON deliveries (updated_at) WHERE status = 'PENDING';

CREATE TABLE inbox (
    consumer     TEXT        NOT NULL,
    event_id     TEXT        NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);
