package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"payment-service/internal/clients"
	"payment-service/internal/models"
)

const (
	PaymentStatusPending  = "pending"
	PaymentStatusPaid     = "completed"
	PaymentStatusFailed   = "failed"
	PaymentStatusRefunded = "refunded"
)

type PaymentService struct {
	DB                *sqlx.DB
	IntegrationClient *clients.PaymentIntegrationClient
	PublicBaseURL     string
}

func NewPaymentService(
	db *sqlx.DB,
	integrationClient *clients.PaymentIntegrationClient,
	publicBaseURL string,
) *PaymentService {
	return &PaymentService{
		DB:                db,
		IntegrationClient: integrationClient,
		PublicBaseURL:     publicBaseURL,
	}
}

type CreatePaymentInput struct {
	OrderID     string
	UserID      string
	Provider    string
	Amount      int64
	Currency    string
	Method      string
	Description string
	ReturnURL   string
}

type CreatePaymentOutput struct {
	TrxID               string `json:"trx_id"`
	OrderID             string `json:"order_id"`
	UserID              string `json:"user_id"`
	Provider            string `json:"provider"`
	ProviderReferenceID string `json:"provider_reference_id"`
	Amount              int64  `json:"amount"`
	Currency            string `json:"currency"`
	Status              string `json:"status"`
	Method              string `json:"method"`
	PaymentURL          string `json:"payment_url"`
}

func (s *PaymentService) CreatePayment(
	ctx context.Context,
	input CreatePaymentInput,
) (*CreatePaymentOutput, error) {
	if input.OrderID == "" {
		return nil, errors.New("order_id is required")
	}

	if input.UserID == "" {
		return nil, errors.New("user_id is required")
	}

	if input.Provider == "" {
		return nil, errors.New("provider is required")
	}

	if input.Amount <= 0 {
		return nil, errors.New("amount must be greater than zero")
	}

	if input.Method == "" {
		return nil, errors.New("method is required")
	}

	if input.Currency == "" {
		input.Currency = "IDR"
	}

	trxID := "TRX-" + uuid.New().String()
	expiresAt := time.Now().Add(30 * time.Minute)

	// 1. Create local payment first as source of truth.
	_, err := s.DB.ExecContext(
		ctx,
		`
        INSERT INTO payments (
            trx_id,
            order_id,
            user_id,
            provider,
            amount,
            currency,
            status,
            expires_at,
			method
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        `,
		trxID,
		input.OrderID,
		input.UserID,
		input.Provider,
		input.Amount,
		input.Currency,
		PaymentStatusPending,
		expiresAt,
		input.Method,
	)
	if err != nil {
		return nil, err
	}

	callbackURL := s.PublicBaseURL + "/v1/payments/callbacks/" + input.Provider

	// 2. Call Payment Integration Service create payment endpoint:
	// POST /v1/integrations/{provider}/payments
	providerResp, err := s.IntegrationClient.CreatePayment(
		input.Provider,
		clients.CreateProviderPaymentRequest{
			TrxID:       trxID,
			OrderID:     input.OrderID,
			UserID:      input.UserID,
			Amount:      input.Amount,
			Currency:    input.Currency,
			Description: input.Description,
			CallbackURL: callbackURL,
			ReturnURL:   input.ReturnURL,
			Method:      input.Method,
		},
	)
	if err != nil {
		_, _ = s.DB.ExecContext(
			ctx,
			`
            UPDATE payments
            SET status = $1,
                failed_at = NOW(),
                updated_at = NOW()
            WHERE trx_id = $2
            `,
			PaymentStatusFailed,
			trxID,
		)

		return nil, err
	}

	if providerResp.PaymentURL == "" {
		_, _ = s.DB.ExecContext(
			ctx,
			`
			UPDATE payments
			SET status = $1,
				failed_at = NOW(),
				updated_at = NOW()
			WHERE trx_id = $2
			`,
			PaymentStatusFailed,
			trxID,
		)

		return nil, errors.New("invalid provider response: missing payment_url")
	}

	// 3. Store provider response.
	_, err = s.DB.ExecContext(
		ctx,
		`
        UPDATE payments
        SET provider_reference_id = $1,
            payment_url = $2,
            updated_at = NOW()
        WHERE trx_id = $3
        `,
		providerResp.ProviderReferenceID,
		providerResp.PaymentURL,
		trxID,
	)
	if err != nil {
		return nil, err
	}

	return &CreatePaymentOutput{
		TrxID:               trxID,
		OrderID:             input.OrderID,
		UserID:              input.UserID,
		Provider:            input.Provider,
		ProviderReferenceID: providerResp.ProviderReferenceID,
		Amount:              input.Amount,
		Currency:            input.Currency,
		Status:              PaymentStatusPending,
		Method:              input.Method,
		PaymentURL:          providerResp.PaymentURL,
	}, nil
}

func (s *PaymentService) GetPayment(
	ctx context.Context,
	trxID string,
) (*models.Payment, error) {
	var payment models.Payment

	err := s.DB.GetContext(
		ctx,
		&payment,
		`
        SELECT *
        FROM payments
        WHERE trx_id = $1
        `,
		trxID,
	)
	if err != nil {
		return nil, err
	}

	return &payment, nil
}

func (s *PaymentService) ProcessCallback(
	ctx context.Context,
	provider string,
	headers map[string]string,
	rawBody []byte,
) error {
	rawHeadersJSON, _ := json.Marshal(headers)

	// 1. Call Payment Integration Service callback verification endpoint:
	// POST /v1/integrations/{provider}/callbacks/verify
	normalized, err := s.IntegrationClient.VerifyCallback(
		provider,
		headers,
		rawBody,
	)
	if err != nil {
		errMsg := err.Error()

		_, _ = s.DB.ExecContext(
			ctx,
			`
            INSERT INTO payment_callbacks (
                provider,
                raw_headers,
                raw_payload,
                signature_valid,
                processing_status,
                error_message
            )
            VALUES ($1, $2, $3, false, 'FAILED', $4)
            `,
			provider,
			rawHeadersJSON,
			rawBody,
			errMsg,
		)

		return err
	}

	rawPayloadJSON, _ := json.Marshal(normalized.RawPayload)

	tx, err := s.DB.BeginTxx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var callbackID int64

	// 2. Store verified callback log.
	err = tx.QueryRowxContext(
		ctx,
		`
        INSERT INTO payment_callbacks (
            provider,
            trx_id,
            provider_reference_id,
            raw_headers,
            raw_payload,
            signature_valid,
            processing_status
        )
        VALUES ($1, $2, $3, $4, $5, true, 'VERIFIED')
        RETURNING id
        `,
		provider,
		normalized.TrxID,
		normalized.ProviderReferenceID,
		rawHeadersJSON,
		rawPayloadJSON,
	).Scan(&callbackID)
	if err != nil {
		return err
	}

	// 3. Lock payment row for idempotency and safe update.
	var payment models.Payment

	err = tx.GetContext(
		ctx,
		&payment,
		`
        SELECT *
        FROM payments
        WHERE (order_id = $1 OR trx_id = $1) AND amount = $2
        FOR UPDATE
        `,
		normalized.TrxID,
		normalized.Amount,
	)
	fmt.Println("order_id:", normalized.TrxID, "trx_id:", normalized.TrxID, "amount:", normalized.Amount)
	if err != nil {
		_, _ = tx.ExecContext(
			ctx,
			`
            UPDATE payment_callbacks
            SET processing_status = 'FAILED',
                error_message = $1
            WHERE id = $2
            `,
			err.Error(),
			callbackID,
		)

		return err
	}

	if payment.Provider != provider {
		errMsg := "provider mismatch"

		_, _ = tx.ExecContext(
			ctx,
			`
            UPDATE payment_callbacks
            SET processing_status = 'FAILED',
                error_message = $1
            WHERE id = $2
            `,
			errMsg,
			callbackID,
		)

		return errors.New(errMsg)
	}

	if payment.Amount != normalized.Amount {
		errMsg := "amount mismatch"

		_, _ = tx.ExecContext(
			ctx,
			`
            UPDATE payment_callbacks
            SET processing_status = 'FAILED',
                error_message = $1
            WHERE id = $2
            `,
			errMsg,
			callbackID,
		)

		return errors.New(errMsg)
	}

	if payment.Currency != normalized.Currency {
		errMsg := "currency mismatch"

		_, _ = tx.ExecContext(
			ctx,
			`
            UPDATE payment_callbacks
            SET processing_status = 'FAILED',
                error_message = $1
            WHERE id = $2
            `,
			errMsg,
			callbackID,
		)

		return errors.New(errMsg)
	}

	newStatus := normalized.Status

	// 4. Idempotency.
	if payment.Status == newStatus {
		_, _ = tx.ExecContext(
			ctx,
			`
            UPDATE payment_callbacks
            SET processing_status = 'DUPLICATE'
            WHERE id = $1
            `,
			callbackID,
		)

		return tx.Commit()
	}

	if isFinalStatus(payment.Status) {
		_, _ = tx.ExecContext(
			ctx,
			`
            UPDATE payment_callbacks
            SET processing_status = 'IGNORED_FINAL_STATUS'
            WHERE id = $1
            `,
			callbackID,
		)

		return tx.Commit()
	}

	// 5. Update payment status.
	err = updatePaymentStatus(
		ctx,
		tx,
		payment.ID,
		newStatus,
		normalized,
	)
	if err != nil {
		return err
	}

	// 6. Insert outbox event only when status actually changes.
	eventType := mapStatusToEventType(newStatus)

	if eventType != "" {
		payload := map[string]any{
			"trx_id":                payment.TrxID,
			"order_id":              payment.OrderID,
			"user_id":               payment.UserID,
			"provider":              payment.Provider,
			"provider_reference_id": normalized.ProviderReferenceID,
			"amount":                payment.Amount,
			"currency":              payment.Currency,
			"status":                newStatus,
		}

		payloadJSON, _ := json.Marshal(payload)

		_, err = tx.ExecContext(
			ctx,
			`
            INSERT INTO outbox_events (
                event_id,
                aggregate_type,
                aggregate_id,
                event_type,
                payload,
                status,
                retry_count,
                max_retry,
                next_retry_at
            )
            VALUES ($1, 'payment', $2, $3, $4, 'PENDING', 0, 10, NOW())
            `,
			uuid.New().String(),
			payment.TrxID,
			eventType,
			payloadJSON,
		)
		if err != nil {
			return err
		}
	}

	// 7. Mark callback processed.
	_, err = tx.ExecContext(
		ctx,
		`
        UPDATE payment_callbacks
        SET processing_status = 'PROCESSED'
        WHERE id = $1
        `,
		callbackID,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func updatePaymentStatus(
	ctx context.Context,
	tx *sqlx.Tx,
	paymentID int64,
	newStatus string,
	normalized *clients.NormalizedCallback,
) error {
	switch newStatus {
	case PaymentStatusPaid:
		paidAt := time.Now()

		if normalized.PaidAt != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, normalized.PaidAt); err == nil {
				paidAt = parsed
			}
		}

		_, err := tx.ExecContext(
			ctx,
			`
            UPDATE payments
            SET status = $1,
                provider_reference_id = $2,
                paid_at = $3,
                updated_at = NOW()
            WHERE id = $4
            `,
			newStatus,
			normalized.ProviderReferenceID,
			paidAt,
			paymentID,
		)

		return err

	case PaymentStatusFailed:
		_, err := tx.ExecContext(
			ctx,
			`
            UPDATE payments
            SET status = $1,
                provider_reference_id = $2,
                failed_at = NOW(),
                updated_at = NOW()
            WHERE id = $3
            `,
			newStatus,
			normalized.ProviderReferenceID,
			paymentID,
		)

		return err

	default:
		_, err := tx.ExecContext(
			ctx,
			`
            UPDATE payments
            SET status = $1,
                provider_reference_id = $2,
                updated_at = NOW()
            WHERE id = $3
            `,
			newStatus,
			normalized.ProviderReferenceID,
			paymentID,
		)

		return err
	}
}

func isFinalStatus(status string) bool {
	return status == PaymentStatusPaid ||
		status == PaymentStatusFailed ||
		status == PaymentStatusRefunded
}

func mapStatusToEventType(status string) string {
	switch status {
	case PaymentStatusPaid:
		return "PAYMENT_SUCCEEDED"
	case PaymentStatusFailed:
		return "PAYMENT_FAILED"
	case PaymentStatusRefunded:
		return "PAYMENT_REFUNDED"
	default:
		return ""
	}
}
