package config

import "os"

type Config struct {
	Port                  string
	DatabaseURL           string
	PublicBaseURL         string
	PaymentIntegrationURL string
	RabbitMQURL           string
	RabbitMQExchange      string
}

func Load() Config {
	return Config{
		Port:                  getEnv("PORT", "8080"),
		DatabaseURL:           getEnv("DATABASE_URL", ""),
		PublicBaseURL:         getEnv("PUBLIC_BASE_URL", "http://localhost:8080"),
		PaymentIntegrationURL: getEnv("PAYMENT_INTEGRATION_URL", "http://localhost:8081"),
		RabbitMQURL:           getEnv("RABBITMQ_URL", ""),
		RabbitMQExchange:      getEnv("RABBITMQ_EXCHANGE", ""),
	}
}

func getEnv(key string, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}
