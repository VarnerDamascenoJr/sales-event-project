CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS sales_events (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    status TEXT NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tickets (
    id UUID PRIMARY KEY,
    sales_event_id UUID NOT NULL REFERENCES sales_events(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    price BIGINT NOT NULL CHECK (price > 0),
    available_quantity INTEGER NOT NULL CHECK (available_quantity >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sales (
    id UUID PRIMARY KEY,
    sales_event_id UUID REFERENCES sales_events(id),
    customer_id TEXT NOT NULL,
    status TEXT NOT NULL,
    total_amount BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sale_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    sale_id UUID NOT NULL REFERENCES sales(id) ON DELETE CASCADE,
    ticket_id UUID NOT NULL REFERENCES tickets(id),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_price BIGINT NOT NULL CHECK (unit_price > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (sale_id, ticket_id)
);

CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    sale_id UUID NOT NULL UNIQUE REFERENCES sales(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    amount BIGINT NOT NULL,
    provider TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS outbox_events (
    event_id UUID PRIMARY KEY,
    event_type TEXT NOT NULL,
    aggregate_id UUID NOT NULL,
    payload JSONB NOT NULL,
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sales_status ON sales(status);
CREATE INDEX IF NOT EXISTS idx_tickets_sales_event_id ON tickets(sales_event_id);
CREATE INDEX IF NOT EXISTS idx_outbox_events_published_at ON outbox_events(published_at);

INSERT INTO sales_events (id, name, status, starts_at)
VALUES ('11111111-1111-1111-1111-111111111111', 'Backend Moderno Conference', 'PUBLISHED', NOW() + INTERVAL '30 days')
ON CONFLICT (id) DO NOTHING;

INSERT INTO tickets (id, sales_event_id, name, price, available_quantity)
VALUES
    ('22222222-2222-2222-2222-222222222222', '11111111-1111-1111-1111-111111111111', 'General Admission', 10000, 100),
    ('33333333-3333-3333-3333-333333333333', '11111111-1111-1111-1111-111111111111', 'VIP', 25000, 25)
ON CONFLICT (id) DO NOTHING;
