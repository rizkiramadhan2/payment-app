CREATE TABLE payments (
    id BIGSERIAL PRIMARY KEY,
    trx_id VARCHAR(100) NOT NULL UNIQUE,
    order_id VARCHAR(100) NOT NULL,
    user_id VARCHAR(100) NOT NULL,

    provider VARCHAR(50) NOT NULL,
    provider_reference_id VARCHAR(150),

    amount BIGINT NOT NULL,
    currency VARCHAR(10) NOT NULL DEFAULT 'IDR',

    status VARCHAR(30) NOT NULL DEFAULT 'PENDING',

    payment_url TEXT,

    expires_at TIMESTAMPTZ,
    paid_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_payments_order_id ON payments(order_id);
CREATE INDEX idx_payments_user_id ON payments(user_id);
CREATE INDEX idx_payments_status ON payments(status);
CREATE INDEX idx_payments_provider_ref ON payments(provider, provider_reference_id);
