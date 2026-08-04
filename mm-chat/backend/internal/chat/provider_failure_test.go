package chat

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProviderHTTPFailureCategoriesAreBounded(t *testing.T) {
	tests := map[int]ProviderFailureCategory{
		http.StatusFound:               ProviderFailureRequestRejected,
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

func TestProviderRetryDelayAcceptsOnlyTransientCategories(t *testing.T) {
	retryable := []ProviderFailureCategory{
		ProviderFailureRequestTimeout,
		ProviderFailureRateLimited,
		ProviderFailureUpstreamFailed,
		ProviderFailureTransportFailed,
		ProviderFailureStreamReadFailed,
		ProviderFailureStreamIncomplete,
	}
	for _, category := range retryable {
		delay, ok := ProviderRetryDelay(newProviderFailure(category, "bounded"))
		if !ok || delay != 5*time.Second {
			t.Fatalf("category %q retry = %s/%t", category, delay, ok)
		}
	}
	for _, category := range []ProviderFailureCategory{
		ProviderFailureAuthentication,
		ProviderFailureQuotaExhausted,
		ProviderFailureRequestRejected,
		ProviderFailureResponseInvalid,
		ProviderFailureStreamParseFailed,
		ProviderFailureStreamRemoteError,
		ProviderFailureContextCanceled,
	} {
		if _, ok := ProviderRetryDelay(newProviderFailure(category, "bounded")); ok {
			t.Fatalf("category %q became retryable", category)
		}
	}
	err := errors.Join(
		errors.New("adapter boundary"),
		newProviderHTTPFailure(ProviderFailureRateLimited, "bounded", "7"),
	)
	if delay, ok := ProviderRetryDelay(err); !ok || delay != 7*time.Second {
		t.Fatalf("Retry-After chain = %s/%t", delay, ok)
	}
	if delay, ok := ProviderExplicitRetryDelay(err); !ok || delay != 7*time.Second {
		t.Fatalf("explicit Retry-After chain = %s/%t", delay, ok)
	}
	if _, ok := ProviderExplicitRetryDelay(
		newProviderFailure(ProviderFailureTransportFailed, "bounded"),
	); ok {
		t.Fatal("fallback-only transport failure exposed an explicit delay")
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

func TestProviderFailureCategoryCatalogueIsCompleteAndSorted(t *testing.T) {
	categories := ProviderFailureCategories()
	if len(categories) != 15 {
		t.Fatalf("category count=%d categories=%v", len(categories), categories)
	}
	for index, category := range categories {
		if !ValidProviderFailureCategory(category) {
			t.Fatalf("invalid category %q", category)
		}
		if index > 0 && categories[index-1] >= category {
			t.Fatalf("categories not strictly sorted: %v", categories)
		}
	}
}
