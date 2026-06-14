package models

import (
	"database/sql"
	"encoding/json"
	"time"
)

type Payment struct {
	ID                  int64          `db:"id" json:"id"`
	TrxID               string         `db:"trx_id" json:"trx_id"`
	OrderID             string         `db:"order_id" json:"order_id"`
	UserID              string         `db:"user_id" json:"user_id"`
	Provider            string         `db:"provider" json:"provider"`
	ProviderReferenceID sql.NullString `db:"provider_reference_id" json:"provider_reference_id"`
	Amount              int64          `db:"amount" json:"amount"`
	Currency            string         `db:"currency" json:"currency"`
	Status              string         `db:"status" json:"status"`
	PaymentURL          sql.NullString `db:"payment_url" json:"payment_url"`
	ExpiresAt           sql.NullTime   `db:"expires_at" json:"expires_at"`
	PaidAt              sql.NullTime   `db:"paid_at" json:"paid_at"`
	FailedAt            sql.NullTime   `db:"failed_at" json:"failed_at"`
	CreatedAt           time.Time      `db:"created_at" json:"created_at"`
	Method              string         `db:"method" json:"method"`
	UpdatedAt           time.Time      `db:"updated_at" json:"updated_at"`
}

type OutboxEvent struct {
	ID            int64           `db:"id"`
	EventID       string          `db:"event_id"`
	AggregateType string          `db:"aggregate_type"`
	AggregateID   string          `db:"aggregate_id"`
	EventType     string          `db:"event_type"`
	Payload       json.RawMessage `db:"payload"`
	Status        string          `db:"status"`
	RetryCount    int             `db:"retry_count"`
	MaxRetry      int             `db:"max_retry"`
	NextRetryAt   time.Time       `db:"next_retry_at"`
	PublishedAt   sql.NullTime    `db:"published_at"`
	LastError     sql.NullString  `db:"last_error"`
	CreatedAt     time.Time       `db:"created_at"`
	UpdatedAt     time.Time       `db:"updated_at"`
}
