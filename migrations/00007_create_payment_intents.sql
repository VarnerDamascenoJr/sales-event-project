-- +goose Up
CREATE TABLE IF NOT EXISTS payment_intents (
    id UUID PRIMARY KEY,
    sale_id UUID NOT NULL UNIQUE REFERENCES sales(id) ON DELETE CASCADE,
    provider VARCHAR(50) NOT NULL,
    amount INTEGER NOT NULL,
    status VARCHAR(50) NOT NULL,
    provider_reference VARCHAR(100) NOT NULL,
    client_secret VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT payment_intents_provider_length CHECK (char_length(provider) <= 50),
    CONSTRAINT payment_intents_amount_non_negative CHECK (amount >= 0),
    CONSTRAINT payment_intents_amount_max CHECK (amount <= 1000000000),
    CONSTRAINT payment_intents_status_allowed CHECK (status IN ('PENDING', 'SUCCEEDED', 'FAILED')),
    CONSTRAINT payment_intents_provider_reference_length CHECK (char_length(provider_reference) <= 100),
    CONSTRAINT payment_intents_client_secret_length CHECK (char_length(client_secret) <= 100)
);

CREATE INDEX IF NOT EXISTS idx_payment_intents_status ON payment_intents(status);

-- +goose Down
DROP TABLE IF EXISTS payment_intents;
