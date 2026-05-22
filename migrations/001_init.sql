CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS customers (
    id VARCHAR(50) PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT customers_id_length CHECK (char_length(id) <= 50),
    CONSTRAINT customers_email_length CHECK (char_length(email) <= 255),
    CONSTRAINT customers_email_basic CHECK (email LIKE '%@%'),
    CONSTRAINT customers_name_length CHECK (char_length(name) <= 100)
);

CREATE TABLE IF NOT EXISTS sales_events (
    id UUID PRIMARY KEY,
    name VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT sales_events_name_length CHECK (char_length(name) <= 50),
    CONSTRAINT sales_events_status_allowed CHECK (status IN ('DRAFT', 'PUBLISHED', 'CANCELLED'))
);

CREATE TABLE IF NOT EXISTS tickets (
    id UUID PRIMARY KEY,
    sales_event_id UUID NOT NULL REFERENCES sales_events(id) ON DELETE CASCADE,
    name VARCHAR(50) NOT NULL,
    price INTEGER NOT NULL,
    available_quantity INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT tickets_name_length CHECK (char_length(name) <= 50),
    CONSTRAINT tickets_price_positive CHECK (price > 0),
    CONSTRAINT tickets_price_max CHECK (price <= 1000000000),
    CONSTRAINT tickets_available_quantity_non_negative CHECK (available_quantity >= 0)
);

CREATE TABLE IF NOT EXISTS sales (
    id UUID PRIMARY KEY,
    sales_event_id UUID REFERENCES sales_events(id),
    customer_id VARCHAR(50) NOT NULL REFERENCES customers(id),
    status VARCHAR(50) NOT NULL,
    total_amount INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT sales_customer_id_length CHECK (char_length(customer_id) <= 50),
    CONSTRAINT sales_status_allowed CHECK (status IN ('COMPLETED', 'FAILED')),
    CONSTRAINT sales_total_amount_non_negative CHECK (total_amount >= 0),
    CONSTRAINT sales_total_amount_max CHECK (total_amount <= 1000000000)
);

CREATE TABLE IF NOT EXISTS sale_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    sale_id UUID NOT NULL REFERENCES sales(id) ON DELETE CASCADE,
    ticket_id UUID NOT NULL REFERENCES tickets(id),
    quantity INTEGER NOT NULL,
    unit_price INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT sale_items_quantity_positive CHECK (quantity > 0),
    CONSTRAINT sale_items_unit_price_positive CHECK (unit_price > 0),
    CONSTRAINT sale_items_unit_price_max CHECK (unit_price <= 1000000000),
    UNIQUE (sale_id, ticket_id)
);

CREATE TABLE IF NOT EXISTS issued_tickets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    sale_id UUID NOT NULL REFERENCES sales(id) ON DELETE CASCADE,
    ticket_id UUID NOT NULL REFERENCES tickets(id),
    customer_id VARCHAR(50) NOT NULL REFERENCES customers(id),
    qr_code_payload VARCHAR(255) NOT NULL UNIQUE,
    emailed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    sale_id UUID NOT NULL UNIQUE REFERENCES sales(id) ON DELETE CASCADE,
    status VARCHAR(50) NOT NULL,
    amount INTEGER NOT NULL,
    provider VARCHAR(50) NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT payments_status_allowed CHECK (status IN ('APPROVED', 'FAILED')),
    CONSTRAINT payments_amount_non_negative CHECK (amount >= 0),
    CONSTRAINT payments_amount_max CHECK (amount <= 1000000000),
    CONSTRAINT payments_provider_length CHECK (char_length(provider) <= 50)
);

CREATE TABLE IF NOT EXISTS outbox_events (
    event_id UUID PRIMARY KEY,
    event_type VARCHAR(50) NOT NULL,
    aggregate_id UUID NOT NULL,
    payload JSONB NOT NULL,
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT outbox_events_event_type_length CHECK (char_length(event_type) <= 50),
    CONSTRAINT outbox_events_event_type_allowed CHECK (event_type IN ('SALE_COMPLETED', 'SALE_FAILED'))
);

CREATE TABLE IF NOT EXISTS email_notifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    sale_id UUID NOT NULL UNIQUE REFERENCES sales(id) ON DELETE CASCADE,
    recipient_email VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'PENDING',
    error_message TEXT,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT email_notifications_status_allowed CHECK (status IN ('PENDING', 'SENT', 'FAILED')),
    CONSTRAINT email_notifications_recipient_email_length CHECK (char_length(recipient_email) <= 255)
);

CREATE INDEX IF NOT EXISTS idx_sales_status ON sales(status);
CREATE INDEX IF NOT EXISTS idx_tickets_sales_event_id ON tickets(sales_event_id);
CREATE INDEX IF NOT EXISTS idx_issued_tickets_sale_id ON issued_tickets(sale_id);
CREATE INDEX IF NOT EXISTS idx_email_notifications_status ON email_notifications(status);
CREATE INDEX IF NOT EXISTS idx_outbox_events_published_at ON outbox_events(published_at);

INSERT INTO sales_events (id, name, status, starts_at)
VALUES ('11111111-1111-1111-1111-111111111111', 'Backend Moderno Conference', 'PUBLISHED', NOW() + INTERVAL '30 days')
ON CONFLICT (id) DO NOTHING;

INSERT INTO tickets (id, sales_event_id, name, price, available_quantity)
VALUES
    ('22222222-2222-2222-2222-222222222222', '11111111-1111-1111-1111-111111111111', 'General Admission', 10000, 100),
    ('33333333-3333-3333-3333-333333333333', '11111111-1111-1111-1111-111111111111', 'VIP', 25000, 25)
ON CONFLICT (id) DO NOTHING;
