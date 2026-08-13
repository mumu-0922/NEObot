package mcpclient

import (
	"context"
	"errors"
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
