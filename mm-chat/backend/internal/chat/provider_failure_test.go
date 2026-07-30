package chat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProviderHTTPFailureCategoriesAreBounded(t *testing.T) {
	tests := map[int]ProviderFailureCategory{
		http.StatusUnauthorized:        ProviderFailureAuthentication,
		http.StatusForbidden:           ProviderFailureAuthentication,
		http.StatusPaymentRequired:     ProviderFailureQuotaExhausted,
		http.StatusRequestTimeout:      ProviderFailureRequestTimeout,
		http.StatusTooManyRequests:     ProviderFailureRateLimited,
		http.StatusUnprocessableEntity: ProviderFailureRequestRejected,
		http.StatusInternalServerError: ProviderFailureUpstreamFailed,
		http.StatusServiceUnavailable:  ProviderFailureUpstreamFailed,
	}
	for status, want := range tests {
		if got := providerHTTPFailureCategory(status); got != want {
			t.Errorf("status %d category = %q, want %q", status, got, want)
		}
	}
}

func TestOpenAICompatibleProviderReturnsTypedHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error":{"message":"private balance detail"}}`))
	}))
	defer server.Close()
	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
		BaseURL: server.URL,
		APIKey:  "fixture-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.StreamToolRound(context.Background(), ProviderRoundRequest{
		ProviderRequest: ProviderRequest{
			Prompt:   "fixture",
			ModelRef: ModelRef{ProviderID: OpenAICompatibleProviderID, ModelID: "fixture-model"},
		},
	})
	category, ok := ProviderFailureCategoryOf(err)
	if !ok || category != ProviderFailureQuotaExhausted {
		t.Fatalf("HTTP failure category = %q/%t (%v)", category, ok, err)
	}
}

func TestProviderFailureCategoryOfDoesNotRequireErrorText(t *testing.T) {
	err := newProviderFailure(ProviderFailureRateLimited, "private upstream text")
	category, ok := ProviderFailureCategoryOf(err)
	if !ok || category != ProviderFailureRateLimited {
		t.Fatalf("category = %q/%t", category, ok)
	}
	category, ok = ProviderFailureCategoryOf(context.DeadlineExceeded)
	if !ok || category != ProviderFailureContextDeadline {
		t.Fatalf("deadline category = %q/%t", category, ok)
	}
}
