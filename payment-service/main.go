package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"payment-service/internal/clients"
	"payment-service/internal/config"
	database "payment-service/internal/db"
	"payment-service/internal/handlers"
	"payment-service/internal/queue"
	"payment-service/internal/services"
	"payment-service/internal/workers"
)

func main() {
	cfg := config.Load()

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("db connection failed:", err)
	}
	defer db.Close()

	integrationClient := clients.NewPaymentIntegrationClient(cfg.PaymentIntegrationURL)

	paymentService := services.NewPaymentService(
		db,
		integrationClient,
		cfg.PublicBaseURL,
	)

	paymentHandler := handlers.NewPaymentHandler(paymentService)

	publisher, err := queue.NewPublisher(cfg.RabbitMQURL, cfg.RabbitMQExchange)
	if err != nil {
		log.Fatal("rabbitmq connection failed:", err)
	}
	defer publisher.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	outboxWorker := workers.NewOutboxWorker(db, publisher)
	go outboxWorker.Start(ctx)

	r := gin.Default()
	r.Use(cors.Default())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"service": "payment-service",
		})
	})

	v1 := r.Group("/v1")
	{
		v1.POST("/payments", paymentHandler.CreatePayment)
		v1.GET("/payments/:trx_id", paymentHandler.GetPayment)
		v1.POST("/payments/callbacks/:provider", paymentHandler.Callback)
	}

	go func() {
		log.Println("payment-service running on :" + cfg.Port)
		if err := r.Run(":" + cfg.Port); err != nil {
			log.Fatal(err)
		}
	}()

	waitForShutdown(cancel)
}

func waitForShutdown(cancel context.CancelFunc) {
	quit := make(chan os.Signal, 1)

	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit

	log.Println("shutting down...")
	cancel()
}
