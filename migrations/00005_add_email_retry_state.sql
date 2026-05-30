-- +goose Up
ALTER TABLE email_notifications
ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0,
ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

ALTER TABLE email_notifications
DROP CONSTRAINT IF EXISTS email_notifications_status_allowed;

ALTER TABLE email_notifications
ADD CONSTRAINT email_notifications_status_allowed CHECK (status IN ('PENDING', 'SENT', 'FAILED', 'DEAD_LETTER'));

ALTER TABLE email_notifications
ADD CONSTRAINT email_notifications_attempts_non_negative CHECK (attempts >= 0);

UPDATE email_notifications
SET next_retry_at = COALESCE(next_retry_at, created_at, NOW());

CREATE INDEX IF NOT EXISTS idx_email_notifications_status_next_retry_at ON email_notifications(status, next_retry_at);

-- +goose Down
DROP INDEX IF EXISTS idx_email_notifications_status_next_retry_at;

ALTER TABLE email_notifications
DROP CONSTRAINT IF EXISTS email_notifications_attempts_non_negative;

ALTER TABLE email_notifications
DROP CONSTRAINT IF EXISTS email_notifications_status_allowed;

ALTER TABLE email_notifications
ADD CONSTRAINT email_notifications_status_allowed CHECK (status IN ('PENDING', 'SENT', 'FAILED'));

ALTER TABLE email_notifications
DROP COLUMN IF EXISTS next_retry_at,
DROP COLUMN IF EXISTS attempts;
