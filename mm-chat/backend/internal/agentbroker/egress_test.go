package agentbroker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/safenet"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestEgressNoneAndExactAllowlistFailBeforeNetwork(t *testing.T) {
	broker, err := NewEgressBroker(1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	broker.clientFactory = func(safenet.Policy, *url.URL, http.Header, time.Duration) *http.Client {
		calls++
		return &http.Client{}
	}
	for _, test := range []struct {
		policy  EgressPolicy
		request EgressRequest
	}{
		{policy: EgressPolicy{Mode: "none"}, request: EgressRequest{Mode: "none", Method: http.MethodGet, URL: "https://api.example/v1"}},
		{policy: EgressPolicy{Mode: "allowlist", Rules: []EgressRule{{Scheme: "https", Host: "api.example", Ports: []int{443}}}},
			request: EgressRequest{Mode: "allowlist", Method: http.MethodGet, URL: "https://evil.example/v1"}},
		{policy: EgressPolicy{Mode: "allowlist", Rules: []EgressRule{{Scheme: "https", Host: "api.example", Ports: []int{443}}}},
			request: EgressRequest{Mode: "allowlist", Method: http.MethodGet, URL: "https://api.example:8443/v1"}},
		{policy: EgressPolicy{Mode: "allowlist", Rules: []EgressRule{{Scheme: "https", Host: "api.example", Ports: []int{443}}}},
			request: EgressRequest{Mode: "allowlist", Method: http.MethodGet, URL: "https://api.example/v1",
				Credential: &CredentialApplication{Header: "Authorization", Value: []byte("canary")}}},
	} {
		if _, err := broker.Do(context.Background(), test.policy, test.request); !errors.Is(err, ErrEgressDenied) {
			t.Fatalf("Egress error = %v", err)
		}
	}
	if calls != 0 {
		t.Fatalf("denied Egress constructed %d clients", calls)
	}
}

func TestEgressDiagnosticsFingerprintOmitsPathAndQuery(t *testing.T) {
	first := EgressDestinationFingerprint("https://api.example/private?token=canary")
	second := EgressDestinationFingerprint("https://api.example/other")
	if first == "" || first != second || strings.Contains(first, "private") || strings.Contains(first, "canary") {
		t.Fatalf("destination fingerprint = %q / %q", first, second)
	}
}

func TestEgressBindsDeclaredBodyBeforeNetworkAndHandlesNilBody(t *testing.T) {
	broker, _ := NewEgressBroker(1, time.Second)
	calls := 0
	broker.clientFactory = func(safenet.Policy, *url.URL, http.Header, time.Duration) *http.Client {
		calls++
		return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(request.Body)
			if err != nil || len(body) != 0 {
				t.Fatalf("request body = %q, %v", body, err)
			}
			return &http.Response{StatusCode: http.StatusNoContent, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(nil))}, nil
		})}
	}
	policy := EgressPolicy{Mode: "allowlist", Rules: []EgressRule{{Scheme: "https", Host: "api.example", Ports: []int{443}}}}
	if _, err := broker.Do(context.Background(), policy, EgressRequest{Mode: "allowlist",
		Method: http.MethodPost, URL: "https://api.example/v1", Body: strings.NewReader("oversized"), BodyBytes: 1}); !errors.Is(err, ErrEgressDenied) || calls != 0 {
		t.Fatalf("body mismatch = %v, network calls=%d", err, calls)
	}
	response, err := broker.Do(context.Background(), policy, EgressRequest{Mode: "allowlist",
		Method: http.MethodGet, URL: "https://api.example/v1"})
	if err != nil || response.StatusCode != http.StatusNoContent || calls != 1 {
		t.Fatalf("nil body request = %#v, %v, calls=%d", response, err, calls)
	}
	_ = response.Body.Close()
}
