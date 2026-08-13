package mcpclient

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"neo-chat/mm-chat/backend/internal/safenet"
)

type NetworkPolicy = safenet.Policy
type Resolver = safenet.Resolver

func ValidateEndpoint(ctx context.Context, raw string, policy NetworkPolicy) (*url.URL, error) {
	return safenet.ValidateEndpoint(ctx, raw, policy)
}

func validateEndpointWithResolver(ctx context.Context, raw string, policy NetworkPolicy, resolver Resolver) (*url.URL, error) {
	return safenet.ValidateEndpointWithResolver(ctx, raw, policy, resolver)
}

func parseEndpoint(raw string, requireHTTPS bool) (*url.URL, error) {
	return safenet.ParseEndpoint(raw, requireHTTPS)
}

func NewSafeHTTPClient(policy NetworkPolicy, baseURL *url.URL, headers http.Header, timeout time.Duration) *http.Client {
	return safenet.NewHTTPClient(policy, baseURL, headers, timeout)
}
