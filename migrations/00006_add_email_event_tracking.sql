-- +goose Up
ALTER TABLE email_notifications
ADD COLUMN IF NOT EXISTS delivered_at TIMESTAMPTZ,
ADD COLUMN IF NOT EXISTS opened_at TIMESTAMPTZ,
ADD COLUMN IF NOT EXISTS clicked_at TIMESTAMPTZ,
ADD COLUMN IF NOT EXISTS bounced_at TIMESTAMPTZ,
ADD COLUMN IF NOT EXISTS provider_event_id VARCHAR(100);

ALTER TABLE email_notifications
DROP CONSTRAINT IF EXISTS email_notifications_status_allowed;

ALTER TABLE email_notifications
ADD CONSTRAINT email_notifications_status_allowed CHECK (
    status IN ('PENDING', 'SENT', 'DELIVERED', 'OPENED', 'CLICKED', 'BOUNCED', 'FAILED', 'DEAD_LETTER')
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_email_notifications_provider_event_id
ON email_notifications(provider_event_id)
WHERE provider_event_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_email_notifications_provider_event_id;

ALTER TABLE email_notifications
DROP CONSTRAINT IF EXISTS email_notifications_status_allowed;

ALTER TABLE email_notifications
ADD CONSTRAINT email_notifications_status_allowed CHECK (status IN ('PENDING', 'SENT', 'FAILED', 'DEAD_LETTER'));

ALTER TABLE email_notifications
DROP COLUMN IF EXISTS provider_event_id,
DROP COLUMN IF EXISTS bounced_at,
DROP COLUMN IF EXISTS clicked_at,
DROP COLUMN IF EXISTS opened_at,
DROP COLUMN IF EXISTS delivered_at;
