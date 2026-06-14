package providers

import (
	"errors"
)

var ErrProviderNotFound = errors.New("provider not found")

type Registry struct {
	providers map[string]PaymentProvider
}

func NewRegistry() *Registry {
	return &Registry{
		providers: map[string]PaymentProvider{},
	}
}

func (r *Registry) Register(provider PaymentProvider) {
	r.providers[provider.Name()] = provider
}

func (r *Registry) Get(name string) (PaymentProvider, error) {
	provider, ok := r.providers[name]
	if !ok {
		return nil, ErrProviderNotFound
	}

	return provider, nil
}
