CREATE TABLE outbox_events (
    id BIGSERIAL PRIMARY KEY,

    event_id UUID NOT NULL UNIQUE,

    aggregate_type VARCHAR(50) NOT NULL,
    aggregate_id VARCHAR(100) NOT NULL,

    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,

    status VARCHAR(30) NOT NULL DEFAULT 'PENDING',

    retry_count INT NOT NULL DEFAULT 0,
    max_retry INT NOT NULL DEFAULT 10,

    next_retry_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    last_error TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_outbox_events_status_next_retry
ON outbox_events(status, next_retry_at);