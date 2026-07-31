package ragproviders

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestProviderRetryAdviceAcceptsOnlyTransientFailures(t *testing.T) {
	for _, value := range []string{"", "5"} {
		delay, ok := ProviderRetryDelay(newProviderRetryError(value))
		if !ok {
			t.Fatalf("Retry-After %q was rejected", value)
		}
		want := 5 * time.Second
		if delay != want {
			t.Fatalf("Retry-After %q delay = %s, want %s", value, delay, want)
		}
	}
	if _, ok := ProviderRetryDelay(ErrProviderGatewayUpstream); ok {
		t.Fatal("unclassified upstream failure became retryable")
	}
	if !providerGatewayRetryableStatus(http.StatusRequestTimeout) ||
		!providerGatewayRetryableStatus(http.StatusTooManyRequests) ||
		!providerGatewayRetryableStatus(http.StatusBadGateway) ||
		providerGatewayRetryableStatus(http.StatusUnauthorized) ||
		providerGatewayRetryableStatus(http.StatusUnprocessableEntity) {
		t.Fatal("retryable HTTP status policy drifted")
	}
	if !errors.Is(newProviderRetryError(""), ErrProviderGatewayUpstream) {
		t.Fatal("retry failure did not preserve upstream classification")
	}
}

func TestAccuracyFirstDevelopmentGatewayHTTPClientHasNoElapsedTimeout(t *testing.T) {
	gateway := NewProviderGateway(
		nil,
		WithProviderGatewayAccuracyFirstDevelopmentNoTimeouts(),
	)
	client, ok := gateway.httpClient.(*http.Client)
	if !ok || client.Timeout != 0 || client.CheckRedirect == nil {
		t.Fatalf("accuracy-first HTTP client = %#v", gateway.httpClient)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil || transport.TLSClientConfig == nil ||
		transport.TLSClientConfig.MinVersion == 0 ||
		transport.TLSHandshakeTimeout != 0 ||
		transport.ResponseHeaderTimeout != 0 {
		t.Fatalf("accuracy-first HTTP transport = %#v", client.Transport)
	}
}
