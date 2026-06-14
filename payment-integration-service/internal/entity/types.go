package entity

type CreatePaymentRequest struct {
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

type CreatePaymentResponse struct {
	Provider            string `json:"provider"`
	ProviderReferenceID string `json:"provider_reference_id"`
	Method              string `json:"method"`
	PaymentURL          string `json:"payment_url"`
	Status              string `json:"status"`
	TotalAmount         int64  `json:"total_amount,omitempty"`
	ExpiredAt           string `json:"expired_at,omitempty"`
	RawResponse         any    `json:"raw_response,omitempty"`
}

type VerifyCallbackRequest struct {
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type NormalizedCallback struct {
	Provider            string `json:"provider"`
	TrxID               string `json:"trx_id"`
	ProviderReferenceID string `json:"provider_reference_id"`
	Method              string `json:"method"`
	Status              string `json:"status"`
	Amount              int64  `json:"amount"`
	Currency            string `json:"currency"`
	ExpiredAt           string `json:"expired_at,omitempty"`
	PaidAt              string `json:"paid_at,omitempty"`
	RawPayload          any    `json:"raw_payload,omitempty"`
}

type ProviderPaymentStatus struct {
	Provider            string `json:"provider"`
	ProviderReferenceID string `json:"provider_reference_id"`
	Method              string `json:"method"`
	TrxID               string `json:"trx_id"`
	Status              string `json:"status"`
	Amount              int64  `json:"amount"`
	Currency            string `json:"currency"`
	RawResponse         any    `json:"raw_response,omitempty"`
}
