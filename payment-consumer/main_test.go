package main

import "testing"

func TestRetryPublishTargetUsesDefaultExchange(t *testing.T) {
	exchange, routingKey := retryPublishTarget("payment.usage.queue.retry")
	if exchange != "" {
		t.Fatalf("exchange = %q, want default exchange", exchange)
	}
	if routingKey != "payment.usage.queue.retry" {
		t.Fatalf("routing key = %q", routingKey)
	}
}
