-- +goose Up
ALTER TABLE sales
ADD COLUMN IF NOT EXISTS request_id VARCHAR(255) NOT NULL DEFAULT '',
ADD COLUMN IF NOT EXISTS correlation_id VARCHAR(255) NOT NULL DEFAULT '',
ADD COLUMN IF NOT EXISTS transaction_id VARCHAR(255) NOT NULL DEFAULT '';

ALTER TABLE outbox_events
ADD COLUMN IF NOT EXISTS request_id VARCHAR(255) NOT NULL DEFAULT '',
ADD COLUMN IF NOT EXISTS correlation_id VARCHAR(255) NOT NULL DEFAULT '',
ADD COLUMN IF NOT EXISTS transaction_id VARCHAR(255) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_sales_correlation_id
ON sales(correlation_id)
WHERE correlation_id <> '';

CREATE INDEX IF NOT EXISTS idx_outbox_events_correlation_id
ON outbox_events(correlation_id)
WHERE correlation_id <> '';

-- +goose Down
DROP INDEX IF EXISTS idx_outbox_events_correlation_id;
DROP INDEX IF EXISTS idx_sales_correlation_id;

ALTER TABLE outbox_events
DROP COLUMN IF EXISTS transaction_id,
DROP COLUMN IF EXISTS correlation_id,
DROP COLUMN IF EXISTS request_id;

ALTER TABLE sales
DROP COLUMN IF EXISTS transaction_id,
DROP COLUMN IF EXISTS correlation_id,
DROP COLUMN IF EXISTS request_id;
