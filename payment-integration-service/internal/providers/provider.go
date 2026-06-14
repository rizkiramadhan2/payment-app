package providers

import (
	"context"

	entity "github.com/rizkiramadhan2/pow-sdk/payment-integration-service/internal/entity"
)

type PaymentProvider interface {
	Name() string
	CreatePayment(ctx context.Context, req entity.CreatePaymentRequest) (*entity.CreatePaymentResponse, error)
	VerifyAndNormalizeCallback(ctx context.Context, headers map[string]string, rawBody []byte) (*entity.NormalizedCallback, error)
	GetPaymentStatus(ctx context.Context, providerReferenceID string, opts ...interface{}) (*entity.ProviderPaymentStatus, error)
}
