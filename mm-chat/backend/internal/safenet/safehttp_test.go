package safenet

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"testing"
)

type staticResolver map[string][]netip.Addr

func (resolver staticResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	addresses, ok := resolver[host]
	if !ok {
		return nil, errors.New("not found")
	}
	return addresses, nil
}

func TestPolicyRejectsLiteralPrivateMixedAndUnlistedOrigins(t *testing.T) {
	resolver := staticResolver{
		"api.example":   {netip.MustParseAddr("93.184.216.34")},
		"mixed.example": {netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("127.0.0.1")},
	}
	policy := Policy{RequireHTTPS: true, RejectIPLiteral: true,
		AllowedOrigins: []string{"https://api.example:443"}}
	for _, raw := range []string{
		"https://127.0.0.1/v1", "https://[::ffff:127.0.0.1]/v1",
		"https://mixed.example/v1", "https://evil.example/v1",
		"https://api.example:8443/v1", "https://user@api.example/v1",
		"https://api.example/v1#fragment", "http://api.example/v1",
	} {
		if _, err := ValidateEndpointWithResolver(context.Background(), raw, policy, resolver); !errors.Is(err, ErrURLBlocked) {
			t.Fatalf("%q error = %v", raw, err)
		}
	}
	if _, err := ValidateEndpointWithResolver(context.Background(), "https://api.example/v1", policy, resolver); err != nil {
		t.Fatal(err)
	}
}

func TestRedirectRevalidatesDNSAndStripsCrossOriginCredentials(t *testing.T) {
	resolver := staticResolver{
		"api.example":   {netip.MustParseAddr("93.184.216.34")},
		"other.example": {netip.MustParseAddr("93.184.216.35")},
	}
	policy := Policy{RequireHTTPS: true, RejectIPLiteral: true,
		AllowedOrigins: []string{"https://api.example:443", "https://other.example:443"}}
	client := NewHTTPClientWithResolver(policy, mustURL(t, "https://api.example/v1"),
		http.Header{"Authorization": {"Bearer canary"}, "X-Api-Key": {"canary"}}, 0, resolver)
	previous, _ := http.NewRequest(http.MethodGet, "https://api.example/v1", nil)
	request, _ := http.NewRequest(http.MethodGet, "https://other.example/v2", nil)
	request.Header.Set("Authorization", "Bearer canary")
	request.Header.Set("X-Api-Key", "canary")
	request.Header.Set("Cookie", "canary")
	if err := client.CheckRedirect(request, []*http.Request{previous}); err != nil {
		t.Fatal(err)
	}
	if request.Header.Get("Authorization") != "" || request.Header.Get("X-Api-Key") != "" || request.Header.Get("Cookie") != "" {
		t.Fatalf("cross-origin credentials survived: %#v", request.Header)
	}
	resolver["other.example"] = []netip.Addr{netip.MustParseAddr("169.254.169.254")}
	if err := client.CheckRedirect(request, []*http.Request{previous}); !errors.Is(err, ErrURLBlocked) {
		t.Fatalf("redirect DNS rebinding error = %v", err)
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	value, err := ParseEndpoint(raw, true)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
