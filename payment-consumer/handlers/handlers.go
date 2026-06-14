package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"payment-consumer/handlers/chatusage"
)

type Config struct {
	RabbitMQURL         string
	RabbitMQExchange    string
	UsageAPIURL         string
	AlertAPIURL         string
	AlertEmailAPIURL    string
	AlertTelegramAPIURL string
	TelegramBotToken    string
	TelegramChatID      string
	ChatUsageAuthToken  string
}

type ConsumerConfig struct {
	QueueName  string
	BindingKey string
	Handler    string
	MaxRetries int
}

type PaymentEvent struct {
	TrxID               string `json:"trx_id"`
	OrderID             string `json:"order_id"`
	UserID              string `json:"user_id"`
	Provider            string `json:"provider"`
	ProviderReferenceID string `json:"provider_reference_id"`
	Amount              int64  `json:"amount"`
	Currency            string `json:"currency"`
	Status              string `json:"status"`
}

func HandleMessage(
	ctx context.Context,
	appCfg Config,
	consumerCfg ConsumerConfig,
	body []byte,
) error {
	var event PaymentEvent

	if err := json.Unmarshal(body, &event); err != nil {
		log.Println("invalid json:", string(body))
		return nil
	}

	log.Printf(
		"received event queue=%s trx_id=%s status=%s amount=%d",
		consumerCfg.QueueName,
		event.TrxID,
		event.Status,
		event.Amount,
	)

	switch consumerCfg.Handler {
	case "alert":
		return HandleAlert(ctx, appCfg, event)

	case "alert_email":
		return HandleAlertEmail(ctx, appCfg, event)

	case "alert_telegram":
		return HandleAlertTelegram(ctx, appCfg, event)

	case "chat_usage":
		return chatusage.HandleUpdateUsage(ctx, appCfg.ChatUsageAuthToken, chatusage.Event{
			TrxID:   event.TrxID,
			OrderID: event.OrderID,
			UserID:  event.UserID,
			Amount:  event.Amount,
			Status:  event.Status,
		})

	default:
		return errors.New("unknown handler: " + consumerCfg.Handler)
	}
}

func HandleUsage(ctx context.Context, cfg Config, event PaymentEvent) error {
	if event.Status != "completed" {
		log.Println("usage ignored non-paid event:", event.Status)
		return nil
	}

	usageCredit := CalculateUsageCredit(event.Amount)

	payload := map[string]any{
		"user_id":      event.UserID,
		"trx_id":       event.TrxID,
		"order_id":     event.OrderID,
		"amount":       event.Amount,
		"currency":     event.Currency,
		"usage_credit": usageCredit,
		"source":       "payment",
	}

	log.Println("calling usage api for trx:", event.TrxID)

	return PostJSON(ctx, cfg.UsageAPIURL, payload)
}

func HandleAlert(ctx context.Context, cfg Config, event PaymentEvent) error {
	payload := map[string]any{
		"type":                  "PAYMENT_" + event.Status,
		"user_id":               event.UserID,
		"trx_id":                event.TrxID,
		"order_id":              event.OrderID,
		"provider":              event.Provider,
		"provider_reference_id": event.ProviderReferenceID,
		"amount":                event.Amount,
		"currency":              event.Currency,
		"status":                event.Status,
	}

	if cfg.AlertAPIURL == "" {
		log.Printf(
			"[ALERT MOCK] payment status=%s trx_id=%s user_id=%s amount=%d",
			event.Status,
			event.TrxID,
			event.UserID,
			event.Amount,
		)
		return nil
	}

	log.Println("calling alert api for trx:", event.TrxID)

	return PostJSON(ctx, cfg.AlertAPIURL, payload)
}

func HandleAlertEmail(ctx context.Context, cfg Config, event PaymentEvent) error {
	payload := map[string]any{
		"type":                  "PAYMENT_" + event.Status,
		"user_id":               event.UserID,
		"trx_id":                event.TrxID,
		"order_id":              event.OrderID,
		"provider":              event.Provider,
		"provider_reference_id": event.ProviderReferenceID,
		"amount":                event.Amount,
		"currency":              event.Currency,
		"status":                event.Status,
	}

	if cfg.AlertEmailAPIURL == "" {
		log.Printf(
			"[ALERT EMAIL MOCK] payment status=%s trx_id=%s user_id=%s amount=%d",
			event.Status,
			event.TrxID,
			event.UserID,
			event.Amount,
		)
		return nil
	}

	log.Println("calling alert email api for trx:", event.TrxID)

	return PostJSON(ctx, cfg.AlertEmailAPIURL, payload)
}

func HandleAlertTelegram(ctx context.Context, cfg Config, event PaymentEvent) error {
	if cfg.TelegramBotToken == "" || cfg.TelegramChatID == "" {
		log.Printf(
			"[ALERT TELEGRAM MOCK] payment status=%s trx_id=%s user_id=%s amount=%d",
			event.Status,
			event.TrxID,
			event.UserID,
			event.Amount,
		)
		return nil
	}

	log.Println("Telegram chat id", cfg.TelegramChatID)

	text := formatTelegramMessage(event)

	apiURL := "https://api.telegram.org/bot" + cfg.TelegramBotToken + "/sendMessage"

	payload := map[string]string{
		"chat_id":    cfg.TelegramChatID,
		"text":       text,
		"parse_mode": "HTML",
	}

	log.Println("sending alert telegram for trx:", event.TrxID)
	err := PostJSON(ctx, apiURL, payload)
	if err != nil {
		log.Println("failed to send alert telegram:", err)
	}

	return err
}

func formatTelegramMessage(event PaymentEvent) string {
	statusEmoji := "⏳"
	switch event.Status {
	case "completed":
		statusEmoji = "✅"
	case "failed":
		statusEmoji = "❌"
	case "pending":
		statusEmoji = "⏳"
	case "refunded":
		statusEmoji = "🔄"
	}

	msg := statusEmoji + " <b>Payment " + event.Status + "</b>\n\n"
	msg += "📋 <b>Trx ID:</b> " + event.TrxID + "\n"
	msg += "📦 <b>Order ID:</b> " + event.OrderID + "\n"
	msg += "👤 <b>User ID:</b> " + event.UserID + "\n"
	msg += "🏦 <b>Provider:</b> " + event.Provider + "\n"

	if event.ProviderReferenceID != "" {
		msg += "🔗 <b>Ref ID:</b> " + event.ProviderReferenceID + "\n"
	}

	msg += "💰 <b>Amount:</b> " + event.Currency + " " + formatAmount(event.Amount) + "\n"
	msg += "📊 <b>Status:</b> " + event.Status + "\n"

	return msg
}

func formatAmount(amount int64) string {
	negative := amount < 0
	if negative {
		amount = -amount
	}

	result := fmt.Sprintf("%d", amount)

	return result
}

func CalculateUsageCredit(amount int64) int64 {
	return amount / 1000
}

func PostJSON(ctx context.Context, url string, payload any) error {
	if url == "" {
		return errors.New("target url is empty")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(
		reqCtx,
		http.MethodPost,
		url,
		bytes.NewBuffer(body),
	)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return errors.New("unexpected response status: " + resp.Status)
	}

	return nil
}
