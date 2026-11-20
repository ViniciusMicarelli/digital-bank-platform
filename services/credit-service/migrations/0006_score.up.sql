-- The behaviour score at the time of each limit request (0 before scores).
ALTER TABLE limit_requests ADD COLUMN score INT NOT NULL DEFAULT 0 CHECK (score BETWEEN 0 AND 1000);
