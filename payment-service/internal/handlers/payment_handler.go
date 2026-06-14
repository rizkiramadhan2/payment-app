package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"payment-service/internal/services"
)

type PaymentHandler struct {
	Service *services.PaymentService
}

func NewPaymentHandler(service *services.PaymentService) *PaymentHandler {
	return &PaymentHandler{
		Service: service,
	}
}

type CreatePaymentRequest struct {
	OrderID     string `json:"order_id"`
	UserID      string `json:"user_id"`
	Provider    string `json:"provider"`
	Amount      int64  `json:"amount"`
	Method      string `json:"method"`
	Currency    string `json:"currency"`
	Description string `json:"description"`
	ReturnURL   string `json:"return_url"`
}

func (h *PaymentHandler) CreatePayment(c *gin.Context) {
	var req CreatePaymentRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request",
		})
		return
	}

	if req.OrderID == "" {
		b := make([]byte, 4)
		rand.Read(b)
		req.OrderID = fmt.Sprintf("INV-%d%s", time.Now().UnixMilli(), hex.EncodeToString(b))
	}

	resp, err := h.Service.CreatePayment(
		c.Request.Context(),
		services.CreatePaymentInput{
			OrderID:     req.OrderID,
			UserID:      req.UserID,
			Provider:    req.Provider,
			Amount:      req.Amount,
			Method:      req.Method,
			Currency:    req.Currency,
			Description: req.Description,
			ReturnURL:   req.ReturnURL,
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *PaymentHandler) GetPayment(c *gin.Context) {
	trxID := c.Param("trx_id")

	payment, err := h.Service.GetPayment(c.Request.Context(), trxID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "payment not found",
		})
		return
	}

	c.JSON(http.StatusOK, payment)
}

func (h *PaymentHandler) Callback(c *gin.Context) {
	provider := c.Param("provider")

	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "failed to read body",
		})
		return
	}

	headers := map[string]string{}

	for key, values := range c.Request.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	// This will internally call:
	// Payment Integration Service
	// POST /v1/integrations/{provider}/callbacks/verify
	err = h.Service.ProcessCallback(
		c.Request.Context(),
		provider,
		headers,
		rawBody,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "callback processed",
	})
}
