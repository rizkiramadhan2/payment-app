CREATE TABLE payment_callbacks (
    id BIGSERIAL PRIMARY KEY,

    provider VARCHAR(50) NOT NULL,
    trx_id VARCHAR(100),
    provider_reference_id VARCHAR(150),

    raw_headers JSONB,
    raw_payload JSONB,

    signature_valid BOOLEAN NOT NULL DEFAULT FALSE,
    processing_status VARCHAR(30) NOT NULL DEFAULT 'RECEIVED',
    error_message TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_payment_callbacks_trx_id ON payment_callbacks(trx_id);
CREATE INDEX idx_payment_callbacks_provider_ref ON payment_callbacks(provider, provider_reference_id);