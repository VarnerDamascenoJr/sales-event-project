-- +goose Up
CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL,
    key_hash VARCHAR(64) NOT NULL UNIQUE,
    role VARCHAR(50) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    CONSTRAINT api_keys_name_length CHECK (char_length(name) <= 100),
    CONSTRAINT api_keys_hash_length CHECK (char_length(key_hash) = 64),
    CONSTRAINT api_keys_role_allowed CHECK (role IN ('ADMIN', 'SUPPORT', 'CHECK_IN', 'PAYMENT_PROVIDER'))
);

CREATE INDEX IF NOT EXISTS idx_api_keys_active_role ON api_keys(active, role);

INSERT INTO api_keys (name, key_hash, role)
VALUES
    ('Local Admin', 'df76ff796f70d2c9cb055ea6280553caa27eda26b70e01082c160de75a05a4a9', 'ADMIN'),
    ('Local Support', '252ace35257f828398f1958affaf656903393196cb1d77a79e9989d22263eb58', 'SUPPORT'),
    ('Local Check-in', '61d567eed3acec974859c7232abf997eee0a553029c3e97233e500c3034bbd98', 'CHECK_IN'),
    ('Local Payment Provider', '47fe98372bfa0aeb90b5d52328864c40212e03eeb43e24345dc17ab92e1a36c6', 'PAYMENT_PROVIDER')
ON CONFLICT (key_hash) DO NOTHING;

-- +goose Down
DELETE FROM api_keys
WHERE key_hash IN (
    'df76ff796f70d2c9cb055ea6280553caa27eda26b70e01082c160de75a05a4a9',
    '252ace35257f828398f1958affaf656903393196cb1d77a79e9989d22263eb58',
    '61d567eed3acec974859c7232abf997eee0a553029c3e97233e500c3034bbd98',
    '47fe98372bfa0aeb90b5d52328864c40212e03eeb43e24345dc17ab92e1a36c6'
);

DROP TABLE IF EXISTS api_keys;
