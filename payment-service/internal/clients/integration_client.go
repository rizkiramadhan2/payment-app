package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type PaymentIntegrationClient struct {
	BaseURL string
	Client  *http.Client
}

func NewPaymentIntegrationClient(baseURL string) *PaymentIntegrationClient {
	return &PaymentIntegrationClient{
		BaseURL: baseURL,
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type CreateProviderPaymentRequest struct {
	TrxID       string `json:"trx_id"`
	OrderID     string `json:"order_id"`
	UserID      string `json:"user_id"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	Method      string `json:"method"`
	Description string `json:"description"`
	CallbackURL string `json:"callback_url"`
	ReturnURL   string `json:"return_url"`
}

type CreateProviderPaymentResponse struct {
	Provider            string `json:"provider"`
	ProviderReferenceID string `json:"provider_reference_id"`
	PaymentURL          string `json:"payment_url"`
	Status              string `json:"status"`
}

type VerifyCallbackRequest struct {
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type NormalizedCallback struct {
	Provider            string `json:"provider"`
	TrxID               string `json:"trx_id"`
	ProviderReferenceID string `json:"provider_reference_id"`
	Status              string `json:"status"`
	Amount              int64  `json:"amount"`
	Currency            string `json:"currency"`
	PaidAt              string `json:"paid_at,omitempty"`
	RawPayload          any    `json:"raw_payload,omitempty"`
	Method              string `json:"method,omitempty"`
}

// CreatePayment calls:
// POST /v1/integrations/{provider}/payments
func (c *PaymentIntegrationClient) CreatePayment(
	provider string,
	req CreateProviderPaymentRequest,
) (*CreateProviderPaymentResponse, error) {
	url := fmt.Sprintf(
		"%s/v1/integrations/%s/payments",
		c.BaseURL,
		provider,
	)

	var resp CreateProviderPaymentResponse

	if err := c.postJSON(url, req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// VerifyCallback calls:
// POST /v1/integrations/{provider}/callbacks/verify
func (c *PaymentIntegrationClient) VerifyCallback(
	provider string,
	headers map[string]string,
	rawBody []byte,
) (*NormalizedCallback, error) {
	url := fmt.Sprintf(
		"%s/v1/integrations/%s/callbacks/verify",
		c.BaseURL,
		provider,
	)

	req := VerifyCallbackRequest{
		Headers: headers,
		Body:    string(rawBody),
	}

	var resp NormalizedCallback

	if err := c.postJSON(url, req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (c *PaymentIntegrationClient) postJSON(
	url string,
	payload any,
	result any,
) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	httpResp, err := c.Client.Post(
		url,
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	respBody, _ := io.ReadAll(httpResp.Body)

	if httpResp.StatusCode >= 300 {
		return fmt.Errorf(
			"integration service error status=%d body=%s",
			httpResp.StatusCode,
			string(respBody),
		)
	}

	if err := json.Unmarshal(respBody, result); err != nil {
		return err
	}

	return nil
}
