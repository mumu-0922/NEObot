package mcpclient

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"testing"
)

type staticResolver map[string][]netip.Addr

func (r staticResolver) LookupNetIP(_ context.Context, _ string, host string) ([]netip.Addr, error) {
	addresses, ok := r[host]
	if !ok {
		return nil, errors.New("not found")
	}
	return addresses, nil
}

func TestValidateEndpointBlocksPrivateAndMalformedTargets(t *testing.T) {
	t.Parallel()
	resolver := staticResolver{
		"public.example": {netip.MustParseAddr("93.184.216.34")},
		"mixed.example": {
			netip.MustParseAddr("93.184.216.34"),
			netip.MustParseAddr("127.0.0.1"),
		},
	}
	tests := []struct {
		name string
		raw  string
	}{
		{name: "http", raw: "http://public.example/mcp"},
		{name: "loopback literal", raw: "https://127.0.0.1/mcp"},
		{name: "metadata", raw: "https://169.254.169.254/latest/meta-data"},
		{name: "mixed dns", raw: "https://mixed.example/mcp"},
		{name: "userinfo", raw: "https://token@public.example/mcp"},
		{name: "fragment", raw: "https://public.example/mcp#secret"},
		{name: "invalid port suffix", raw: "https://public.example:443x/mcp"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := validateEndpointWithResolver(
				context.Background(),
				test.raw,
				NetworkPolicy{RequireHTTPS: true},
				resolver,
			)
			if !errors.Is(err, ErrURLBlocked) {
				t.Fatalf("validate endpoint error = %v, want ErrURLBlocked", err)
			}
		})
	}
	endpoint, err := validateEndpointWithResolver(
		context.Background(),
		"https://public.example:8443/mcp?tenant=one",
		NetworkPolicy{RequireHTTPS: true},
		resolver,
	)
	if err != nil || endpoint.Port() != "8443" {
		t.Fatalf("public endpoint = %v, %v", endpoint, err)
	}
}

func TestSameOriginHeaderTransportOnlyAddsSecretsToBaseOrigin(t *testing.T) {
	t.Parallel()
	base, err := parseEndpoint("https://mcp.example/mcp", true)
	if err != nil {
		t.Fatal(err)
	}
	capture := &capturingRoundTripper{}
	transport := sameOriginHeaderTransport{
		base:       capture,
		baseOrigin: origin(base),
		headers:    http.Header{"Authorization": {"Bearer secret"}, "X-Api-Key": {"secret"}},
	}
	for _, raw := range []string{"https://mcp.example/next", "https://evil.example/next"} {
		request, err := http.NewRequest(http.MethodGet, raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := transport.RoundTrip(request); err != nil {
			t.Fatal(err)
		}
	}
	if got := capture.headers[0].Get("Authorization"); got != "Bearer secret" {
		t.Fatalf("same-origin authorization = %q", got)
	}
	if got := capture.headers[1].Get("Authorization"); got != "" {
		t.Fatalf("cross-origin authorization = %q", got)
	}
}

type capturingRoundTripper struct {
	headers []http.Header
}

func (r *capturingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	r.headers = append(r.headers, request.Header.Clone())
	return &http.Response{
		StatusCode: http.StatusNoContent,
		Header:     make(http.Header),
		Body:       http.NoBody,
		Request:    request,
	}, nil
}
