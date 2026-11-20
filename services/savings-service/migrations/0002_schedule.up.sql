-- Monthly scheduled deposits ("depósito programado").
ALTER TABLE boxes
    ADD COLUMN schedule_amount       BIGINT NOT NULL DEFAULT 0 CHECK (schedule_amount >= 0),
    ADD COLUMN schedule_day          INT    NOT NULL DEFAULT 0 CHECK (schedule_day BETWEEN 0 AND 28),
    -- The next run (Brasília day); NULL without a schedule.
    ADD COLUMN schedule_next         DATE,
    ADD COLUMN schedule_last_failure TEXT   NOT NULL DEFAULT '',
    ADD CONSTRAINT boxes_schedule CHECK ((schedule_amount = 0) = (schedule_next IS NULL)
        AND (schedule_amount = 0) = (schedule_day = 0));
CREATE INDEX boxes_schedule_due ON boxes (schedule_next) WHERE status = 'ACTIVE' AND schedule_next IS NOT NULL;
