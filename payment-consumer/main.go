package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"payment-consumer/handlers"

	"github.com/rabbitmq/amqp091-go"
)

const retryDelayMs = 5000

func main() {
	cfg := loadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := amqp091.Dial(cfg.RabbitMQURL)
	if err != nil {
		log.Fatal("failed to connect rabbitmq:", err)
	}
	defer conn.Close()

	consumerConfigs := []handlers.ConsumerConfig{
		{
			QueueName:  "payment.usage.queue",
			BindingKey: "payment.succeeded",
			Handler:    "chat_usage",
			MaxRetries: 3,
		},
		{
			QueueName:  "payment.alert.queue",
			BindingKey: "payment.*",
			Handler:    "alert_telegram",
			MaxRetries: 3,
		},
	}

	for _, consumerCfg := range consumerConfigs {
		go startConsumer(ctx, conn, cfg, consumerCfg)
	}

	waitForShutdown(cancel)
	log.Println("payment-consumer stopped")
}

func startConsumer(
	ctx context.Context,
	conn *amqp091.Connection,
	appCfg handlers.Config,
	consumerCfg handlers.ConsumerConfig,
) {
	ch, err := conn.Channel()
	if err != nil {
		log.Println("failed to open channel:", err)
		return
	}
	defer ch.Close()

	err = ch.ExchangeDeclare(
		appCfg.RabbitMQExchange,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Println("failed to declare exchange:", err)
		return
	}

	dlqName := consumerCfg.QueueName + ".dlq"
	retryName := consumerCfg.QueueName + ".retry"

	queue, err := declareQueue(ch, consumerCfg.QueueName, amqp091.Table{
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": dlqName,
	})
	if err != nil {
		log.Println("failed to declare main queue:", err)
		return
	}

	_, err = declareQueue(ch, dlqName, nil)
	if err != nil {
		log.Println("failed to declare dlq:", err)
		return
	}

	_, err = declareQueue(ch, retryName, amqp091.Table{
		"x-dead-letter-exchange":    appCfg.RabbitMQExchange,
		"x-dead-letter-routing-key": consumerCfg.BindingKey,
		"x-message-ttl":             retryDelayMs,
	})
	if err != nil {
		log.Println("failed to declare retry queue:", err)
		return
	}

	err = ch.QueueBind(
		queue.Name,
		consumerCfg.BindingKey,
		appCfg.RabbitMQExchange,
		false,
		nil,
	)
	if err != nil {
		log.Println("failed to bind queue:", err)
		return
	}

	err = ch.Qos(
		5,
		0,
		false,
	)
	if err != nil {
		log.Println("failed to set qos:", err)
		return
	}

	msgs, err := ch.Consume(
		queue.Name,
		consumerCfg.QueueName+"-consumer",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Println("failed to consume:", err)
		return
	}

	log.Println("consumer started")
	log.Println("queue:", consumerCfg.QueueName)
	log.Println("retry queue:", retryName)
	log.Println("dlq:", dlqName)
	log.Println("binding:", consumerCfg.BindingKey)
	log.Println("handler:", consumerCfg.Handler)
	log.Println("max retries:", consumerCfg.MaxRetries)

	for {
		select {
		case <-ctx.Done():
			log.Println("consumer stopped:", consumerCfg.QueueName)
			return

		case msg, ok := <-msgs:
			if !ok {
				log.Println("message channel closed:", consumerCfg.QueueName)
				return
			}

			err := handlers.HandleMessage(ctx, appCfg, consumerCfg, msg.Body)
			if err != nil {
				log.Println("failed to handle message:", err)

				retryCount := getRetryCount(msg.Headers)
				if retryCount < consumerCfg.MaxRetries {
					log.Printf(
						"retrying message queue=%s trx_id=%s attempt=%d/%d",
						consumerCfg.QueueName,
						getHeaderString(msg.Headers, "trx_id"),
						retryCount+1,
						consumerCfg.MaxRetries,
					)

					if err := publishRetry(ch, appCfg.RabbitMQExchange, retryName, msg.Body, retryCount+1, msg.Headers); err != nil {
						log.Println("failed to publish to retry queue:", err)
						_ = msg.Nack(false, true)
						continue
					}

					_ = msg.Ack(false)
					continue
				}

				log.Printf(
					"max retries exceeded, sending to dlq queue=%s",
					consumerCfg.QueueName,
				)
				_ = msg.Nack(false, false)
				continue
			}

			_ = msg.Ack(false)
		}
	}
}

func declareQueue(ch *amqp091.Channel, name string, args amqp091.Table) (amqp091.Queue, error) {
	queue, err := ch.QueueDeclare(name, true, false, false, false, args)
	if err == nil {
		return queue, nil
	}

	if !isPreconditionFailed(err) {
		return amqp091.Queue{}, err
	}

	log.Printf("queue %s has stale args, redeclaring...", name)

	_, err = ch.QueueDelete(name, false, false, false)
	if err != nil {
		return amqp091.Queue{}, fmt.Errorf("delete stale queue %s: %w", name, err)
	}

	queue, err = ch.QueueDeclare(name, true, false, false, false, args)
	if err != nil {
		return amqp091.Queue{}, fmt.Errorf("redeclare queue %s: %w", name, err)
	}

	return queue, nil
}

func isPreconditionFailed(err error) bool {
	var amqpErr *amqp091.Error
	if errors.As(err, &amqpErr) {
		return amqpErr.Code == 406
	}
	return false
}

func getRetryCount(headers amqp091.Table) int {
	if headers == nil {
		return 0
	}

	raw, ok := headers["x-retry-count"]
	if !ok {
		return 0
	}

	switch v := raw.(type) {
	case int64:
		return int(v)
	case int:
		return v
	case uint64:
		return int(v)
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}

func getHeaderString(headers amqp091.Table, key string) string {
	if headers == nil {
		return ""
	}

	if v, ok := headers[key]; ok {
		return fmt.Sprintf("%v", v)
	}

	return ""
}

func publishRetry(
	ch *amqp091.Channel,
	exchange string,
	retryQueue string,
	body []byte,
	retryCount int,
	originalHeaders amqp091.Table,
) error {
	headers := amqp091.Table{}

	if originalHeaders != nil {
		for k, v := range originalHeaders {
			if k != "x-retry-count" {
				headers[k] = v
			}
		}
	}

	headers["x-retry-count"] = retryCount

	return ch.Publish(
		exchange,
		retryQueue,
		false,
		false,
		amqp091.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp091.Persistent,
			Headers:      headers,
			Body:         body,
		},
	)
}

func loadConfig() handlers.Config {
	return handlers.Config{
		RabbitMQURL:        getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		RabbitMQExchange:   getEnv("RABBITMQ_EXCHANGE", "payment.events"),
		UsageAPIURL:        getEnv("USAGE_API_URL", "http://localhost:9002/v1/usage/payment"),
		AlertAPIURL:        getEnv("ALERT_API_URL", ""),
		AlertEmailAPIURL:   getEnv("ALERT_EMAIL_API_URL", ""),
		TelegramBotToken:   getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:     getEnv("TELEGRAM_CHAT_ID", ""),
		ChatUsageAuthToken: getEnv("CHAT_USAGE_AUTH_TOKEN", ""),
	}
}

func waitForShutdown(cancel context.CancelFunc) {
	quit := make(chan os.Signal, 1)

	signal.Notify(
		quit,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	<-quit
	log.Println("shutting down payment-consumer...")
	cancel()

	time.Sleep(1 * time.Second)
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
