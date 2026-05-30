-- +goose Up
ALTER TABLE outbox_events
ADD COLUMN IF NOT EXISTS status VARCHAR(50) NOT NULL DEFAULT 'PENDING',
ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0,
ADD COLUMN IF NOT EXISTS last_error TEXT,
ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

ALTER TABLE outbox_events
ADD CONSTRAINT outbox_events_status_allowed CHECK (status IN ('PENDING', 'FAILED', 'PUBLISHED', 'DEAD_LETTER'));

ALTER TABLE outbox_events
ADD CONSTRAINT outbox_events_attempts_non_negative CHECK (attempts >= 0);

UPDATE outbox_events
SET status = CASE
        WHEN published_at IS NULL THEN 'PENDING'
        ELSE 'PUBLISHED'
    END,
    next_attempt_at = COALESCE(next_attempt_at, created_at, NOW());

CREATE INDEX IF NOT EXISTS idx_outbox_events_status_next_attempt_at ON outbox_events(status, next_attempt_at);

-- +goose Down
DROP INDEX IF EXISTS idx_outbox_events_status_next_attempt_at;

ALTER TABLE outbox_events
DROP CONSTRAINT IF EXISTS outbox_events_attempts_non_negative,
DROP CONSTRAINT IF EXISTS outbox_events_status_allowed,
DROP COLUMN IF EXISTS next_attempt_at,
DROP COLUMN IF EXISTS last_error,
DROP COLUMN IF EXISTS attempts,
DROP COLUMN IF EXISTS status;
