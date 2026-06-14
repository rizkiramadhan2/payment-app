package main

import (
	"log"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/rizkiramadhan2/pow-sdk/payment-integration-service/internal/handlers"
	"github.com/rizkiramadhan2/pow-sdk/payment-integration-service/internal/providers"
)

func main() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("failed to load .env file: %v", err)
	}

	r := gin.Default()
	r.Use(cors.Default())

	registry := providers.NewRegistry()

	pakasirServerKey := getEnv("PAKASIR_SERVER_KEY", "dummy-pakasir-server-key")
	pakasirBaseURL := getEnv("PAKASIR_BASE_URL", "https://app.pakasir.com/api")
	pakasirProjectKey := getEnv("PAKASIR_PROJECT_KEY", "")

	registry.Register(
		providers.NewProviderPakasir(
			pakasirServerKey,
			pakasirBaseURL,
			pakasirProjectKey,
		),
	)

	handler := handlers.NewIntegrationHandler(registry)

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"service": "payment-integration-service",
		})
	})

	v1 := r.Group("/v1")

	v1.POST("/integrations/:provider/payments", handler.CreatePayment)

	// Recommended internal endpoint used by Payment Service.
	// Payment Service sends headers/body as JSON.
	v1.POST("/integrations/:provider/callbacks/verify", handler.VerifyCallback)

	// Optional raw endpoint.
	// Can be useful if callback is sent directly here.
	// v1.POST("/integrations/:provider/callbacks/raw", handler.VerifyCallbackRaw)

	v1.GET(
		"/integrations/:provider/payments/:provider_reference_id",
		handler.GetPaymentStatus,
	)

	port := getEnv("PORT", "8081")

	log.Println("payment integration service running on :" + port)
	r.Run(":" + port)
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
