package workers

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"payment-service/internal/models"
	"payment-service/internal/queue"
)

type OutboxWorker struct {
	DB        *sqlx.DB
	Publisher *queue.Publisher
	Interval  time.Duration
	BatchSize int
}

func NewOutboxWorker(db *sqlx.DB, publisher *queue.Publisher) *OutboxWorker {
	return &OutboxWorker{
		DB:        db,
		Publisher: publisher,
		Interval:  3 * time.Second,
		BatchSize: 20,
	}
}

func (w *OutboxWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("outbox worker stopped")
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

func (w *OutboxWorker) processBatch(ctx context.Context) {
	var events []models.OutboxEvent

	err := w.DB.SelectContext(
		ctx,
		&events,
		`
        SELECT *
        FROM outbox_events
        WHERE status = 'PENDING'
          AND next_retry_at <= NOW()
        ORDER BY id ASC
        LIMIT $1
        `,
		w.BatchSize,
	)
	if err != nil {
		log.Println("fetch outbox failed:", err)
		return
	}

	for _, event := range events {
		w.processEvent(ctx, event.ID)
	}
}

func (w *OutboxWorker) processEvent(ctx context.Context, eventID int64) {
	tx, err := w.DB.BeginTxx(ctx, &sql.TxOptions{})
	if err != nil {
		log.Println("begin tx failed:", err)
		return
	}
	defer tx.Rollback()

	var event models.OutboxEvent

	err = tx.GetContext(
		ctx,
		&event,
		`
        SELECT *
        FROM outbox_events
        WHERE id = $1
          AND status = 'PENDING'
        FOR UPDATE
        `,
		eventID,
	)
	if err != nil {
		if !strings.Contains(err.Error(), "no rows") {
			log.Println("lock outbox failed:", err)
		}
		return
	}

	routingKey := eventTypeToRoutingKey(event.EventType)

	publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err = w.Publisher.Publish(publishCtx, routingKey, event.Payload)
	if err != nil {
		retryCount := event.RetryCount + 1
		nextRetryAt := time.Now().Add(time.Duration(retryCount*retryCount) * time.Second)

		status := "PENDING"
		if retryCount >= event.MaxRetry {
			status = "FAILED"
		}

		_, updateErr := tx.ExecContext(
			ctx,
			`
            UPDATE outbox_events
            SET retry_count = $1,
                next_retry_at = $2,
                status = $3,
                last_error = $4,
                updated_at = NOW()
            WHERE id = $5
            `,
			retryCount,
			nextRetryAt,
			status,
			err.Error(),
			event.ID,
		)
		if updateErr != nil {
			log.Println("update outbox retry failed:", updateErr)
			return
		}

		_ = tx.Commit()
		return
	}

	_, err = tx.ExecContext(
		ctx,
		`
        UPDATE outbox_events
        SET status = 'PUBLISHED',
            published_at = NOW(),
            updated_at = NOW(),
            last_error = NULL
        WHERE id = $1
        `,
		event.ID,
	)
	if err != nil {
		log.Println("mark outbox published failed:", err)
		return
	}

	if err := tx.Commit(); err != nil {
		log.Println("commit outbox tx failed:", err)
		return
	}
}

func eventTypeToRoutingKey(eventType string) string {
	switch eventType {
	case "PAYMENT_SUCCEEDED":
		return "payment.succeeded"
	case "PAYMENT_FAILED":
		return "payment.failed"
	case "PAYMENT_REFUNDED":
		return "payment.refunded"
	default:
		return "payment.unknown"
	}
}
