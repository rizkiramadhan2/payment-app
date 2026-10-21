package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	entity "github.com/rizkiramadhan2/pow-sdk/payment-integration-service/internal/entity"
)

type ProviderPakasir struct {
	ServerKey  string
	BaseURL    string
	ProjectKey string
}

const (
	StatusPending   = "pending"
	StatusPaid      = "completed"
	StatusFailed    = "failed"
	StatusExpired   = "expired"
	StatusCancelled = "cancelled"
)

func NewProviderPakasir(serverKey, baseURL, projectKey string) *ProviderPakasir {
	return &ProviderPakasir{
		ServerKey:  serverKey,
		BaseURL:    baseURL,
		ProjectKey: projectKey,
	}
}

func (p *ProviderPakasir) Name() string {
	return "pakasir"
}

func (p *ProviderPakasir) CreatePayment(ctx context.Context, req entity.CreatePaymentRequest) (*entity.CreatePaymentResponse, error) {

	if req.Method == "" {
		err := fmt.Errorf("payment method is required")
		log.Printf("pakasir create payment failed: trx_id=%s error=%v", req.TrxID, err)
		return nil, err
	}

	if req.Amount <= 0 {
		err := fmt.Errorf("amount must be greater than 0")
		log.Printf("pakasir create payment failed: trx_id=%s error=%v", req.TrxID, err)
		return nil, err
	}

	if req.TrxID == "" {
		err := fmt.Errorf("trx_id is required")
		log.Printf("pakasir create payment failed: error=%v", err)
		return nil, err
	}

	apiURL := p.BaseURL + "/create-transaction/" + p.ProjectKey + "/" + req.TrxID
	body := map[string]any{
		"method": req.Method,
		"amount": req.Amount,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		log.Printf("pakasir create payment failed to marshal request body: trx_id=%s method=%s error=%v", req.TrxID, req.Method, err)
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		log.Printf("pakasir create payment failed to create request: trx_id=%s method=%s error=%v", req.TrxID, req.Method, err)
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Api-Key", p.ServerKey)

	httpClient := &http.Client{}
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		log.Printf("pakasir create payment request failed: trx_id=%s method=%s error=%v", req.TrxID, req.Method, err)
		return nil, err
	}
	defer httpResp.Body.Close()
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		log.Printf("pakasir create payment failed to read response body: trx_id=%s method=%s error=%v", req.TrxID, req.Method, err)
		return nil, err
	}

	if httpResp.StatusCode != http.StatusOK {
		log.Printf("pakasir create payment returned unexpected status: trx_id=%s method=%s status_code=%d", req.TrxID, req.Method, httpResp.StatusCode)
		return nil, err
	}

	pakasirRespStruct := struct {
		Project       string `json:"project"`
		OrderID       string `json:"order_id"`
		TrxID         string `json:"txn_id"`
		Amount        int64  `json:"amount"`
		Fee           int64  `json:"fee"`
		TotalPayment  int64  `json:"total_payment"`
		PaymentMethod string `json:"payment_method"`
		QRString      string `json:"qr_string"`
		VANumber      string `json:"va_number"`
		ExpiredAt     string `json:"expired_at"`
	}{}

	err = json.Unmarshal(respBody, &pakasirRespStruct)
	if err != nil {
		log.Printf("pakasir create payment failed to decode response: trx_id=%s method=%s error=%v", req.TrxID, req.Method, err)
		return nil, err
	}

	paymentURL := ""
	switch req.Method {
	case "qris":
		paymentURL = pakasirRespStruct.QRString
	case "va":
		paymentURL = pakasirRespStruct.VANumber
	default:
		err := fmt.Errorf("unsupported payment method: %s", req.Method)
		log.Printf("pakasir create payment failed: trx_id=%s method=%s error=%v", req.TrxID, req.Method, err)
		return nil, err
	}

	resp := &entity.CreatePaymentResponse{
		Provider:            p.Name(),
		Method:              req.Method,
		ProviderReferenceID: pakasirRespStruct.TrxID,
		ExpiredAt:           pakasirRespStruct.ExpiredAt,
		PaymentURL:          paymentURL,
		TotalAmount:         pakasirRespStruct.TotalPayment,
		RawResponse:         pakasirRespStruct,
		Status:              StatusPending,
	}

	log.Printf("pakasir create payment success: trx_id=%s method=%s provider_reference_id=%s", req.TrxID, req.Method, resp.ProviderReferenceID)
	return resp, nil
}

func (p *ProviderPakasir) VerifyAndNormalizeCallback(ctx context.Context, headers map[string]string, rawBody []byte) (*entity.NormalizedCallback, error) {

	bodyStruct := struct {
		OrderID string `json:"order_id"`
		Amount  int64  `json:"amount"`
		Method  string `json:"method"`
	}{}
	err := json.Unmarshal(rawBody, &bodyStruct)
	if err != nil {
		return nil, err
	}

	// get payment status from pakasir API to verify the callback
	paymentResp, err := p.GetPaymentStatus(ctx, bodyStruct.OrderID, bodyStruct.Amount)
	if err != nil {
		return nil, err
	}
	if paymentResp == nil {
		return nil, fmt.Errorf("payment not found for order_id: %s", bodyStruct.OrderID)
	}

	if paymentResp.Status != StatusPaid {
		return nil, fmt.Errorf("payment status is not paid for order_id: %s, got: %s", bodyStruct.OrderID, paymentResp.Status)
	}

	return &entity.NormalizedCallback{
		Provider:            p.Name(),
		Method:              bodyStruct.Method,
		ProviderReferenceID: bodyStruct.OrderID,
		TrxID:               bodyStruct.OrderID,
		Amount:              bodyStruct.Amount,
		Status:              StatusPaid,
		RawPayload:          bodyStruct,
		Currency:            "IDR",
	}, nil
}

func (p *ProviderPakasir) GetPaymentStatus(ctx context.Context, providerReferenceID string, opts ...interface{}) (*entity.ProviderPaymentStatus, error) {

	if len(opts) != 1 {
		err := fmt.Errorf("invalid number of options provided")
		log.Printf("pakasir get payment status failed: provider_reference_id=%s error=%v", providerReferenceID, err)
		return nil, err
	}

	amount := opts[0].(int64)
	apiURL := p.BaseURL + "/transaction-status/" + p.ProjectKey + "/" + providerReferenceID
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		log.Printf("pakasir get payment status failed to create request: provider_reference_id=%s amount=%d error=%v", providerReferenceID, amount, err)
		return nil, err
	}

	httpReq.Header.Set("X-Api-Key", p.ServerKey)

	httpClient := &http.Client{}
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		log.Printf("pakasir get payment status request failed: provider_reference_id=%s amount=%d error=%v", providerReferenceID, amount, err)
		return nil, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		err := fmt.Errorf("unexpected status code: %d", httpResp.StatusCode)
		log.Printf("pakasir get payment status returned unexpected status: provider_reference_id=%s amount=%d status_code=%d", providerReferenceID, amount, httpResp.StatusCode)
		return nil, err
	}

	responseStruct := struct {
		Amount      int64  `json:"amount"`
		OrderID     string `json:"order_id"`
		TrxID       string `json:"txn_id"`
		Project     string `json:"project"`
		Status      string `json:"status"`
		CompletedAt string `json:"completed_at"`
	}{}

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		log.Printf("pakasir get payment status failed to decode response: provider_reference_id=%s amount=%d error=%v", providerReferenceID, amount, err)
		return nil, err
	}
	err = json.Unmarshal(respBody, &responseStruct)
	if err != nil {
		log.Printf("pakasir get payment status failed to decode response: provider_reference_id=%s amount=%d error=%v", providerReferenceID, amount, err)
		return nil, err
	}

	resp := &entity.ProviderPaymentStatus{
		Provider:            p.Name(),
		TrxID:               responseStruct.TrxID,
		Status:              responseStruct.Status,
		Amount:              responseStruct.Amount,
		Currency:            "IDR",
		ProviderReferenceID: responseStruct.TrxID,
		RawResponse:         responseStruct,
	}

	log.Printf("pakasir get payment status success: provider_reference_id=%s amount=%d status=%s transaction_order_id=%s", providerReferenceID, amount, resp.Status, resp.ProviderReferenceID)
	return resp, nil
}
