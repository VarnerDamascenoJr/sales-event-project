-- +goose Up
-- Persisting trace context keeps asynchronous outbox publication in the original trace.
ALTER TABLE outbox_events
ADD COLUMN IF NOT EXISTS trace_context TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE outbox_events
DROP COLUMN IF EXISTS trace_context;
