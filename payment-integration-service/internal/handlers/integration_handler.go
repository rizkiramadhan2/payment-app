package handlers

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rizkiramadhan2/pow-sdk/payment-integration-service/internal/entity"
	"github.com/rizkiramadhan2/pow-sdk/payment-integration-service/internal/providers"
)

type IntegrationHandler struct {
	Registry *providers.Registry
}

func NewIntegrationHandler(registry *providers.Registry) *IntegrationHandler {
	return &IntegrationHandler{
		Registry: registry,
	}
}

func (h *IntegrationHandler) CreatePayment(c *gin.Context) {
	providerName := c.Param("provider")

	provider, err := h.Registry.Get(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "provider not supported",
		})
		return
	}

	var req entity.CreatePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request",
		})
		return
	}

	resp, err := provider.CreatePayment(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to create provider payment",
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *IntegrationHandler) VerifyCallback(c *gin.Context) {
	providerName := c.Param("provider")

	provider, err := h.Registry.Get(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "provider not supported",
		})
		return
	}

	var req entity.VerifyCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request",
		})
		return
	}

	normalized, err := provider.VerifyAndNormalizeCallback(
		c.Request.Context(),
		req.Headers,
		[]byte(req.Body),
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, normalized)
}

func (h *IntegrationHandler) VerifyCallbackRaw(c *gin.Context) {
	providerName := c.Param("provider")

	provider, err := h.Registry.Get(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "provider not supported",
		})
		return
	}

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

	normalized, err := provider.VerifyAndNormalizeCallback(
		c.Request.Context(),
		headers,
		rawBody,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, normalized)
}

func (h *IntegrationHandler) GetPaymentStatus(c *gin.Context) {
	providerName := c.Param("provider")
	providerReferenceID := c.Param("provider_reference_id")
	amount := c.Query("amount")
	amountInt64 := int64(0)
	err := error(nil)
	if amount != "" {
		amountInt64, err = strconv.ParseInt(amount, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid amount",
			})
			return
		}
	}

	provider, err := h.Registry.Get(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "provider not supported",
		})
		return
	}

	resp, err := provider.GetPaymentStatus(
		c.Request.Context(),
		providerReferenceID,
		amountInt64,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get provider payment status",
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}
