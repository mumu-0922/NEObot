package mcpclient

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type NetworkPolicy struct {
	RequireHTTPS bool
	AllowPrivate bool
}

const maxMCPHTTPResponseBytes = int64(64 << 20)

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type netResolver struct{ resolver *net.Resolver }

func (r netResolver) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	return r.resolver.LookupNetIP(ctx, network, host)
}

func ValidateEndpoint(ctx context.Context, raw string, policy NetworkPolicy) (*url.URL, error) {
	return validateEndpointWithResolver(ctx, raw, policy, netResolver{resolver: net.DefaultResolver})
}

func validateEndpointWithResolver(
	ctx context.Context,
	raw string,
	policy NetworkPolicy,
	resolver Resolver,
) (*url.URL, error) {
	parsed, err := parseEndpoint(raw, policy.RequireHTTPS)
	if err != nil {
		return nil, err
	}
	if policy.AllowPrivate {
		return parsed, nil
	}
	addresses, err := resolveHost(ctx, resolver, parsed.Hostname())
	if err != nil || len(addresses) == 0 {
		return nil, ErrURLBlocked
	}
	for _, address := range addresses {
		if blockedAddress(address) {
			return nil, ErrURLBlocked
		}
	}
	return parsed, nil
}

func parseEndpoint(raw string, requireHTTPS bool) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, ErrURLBlocked
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, ErrURLBlocked
	}
	if requireHTTPS && parsed.Scheme != "https" {
		return nil, ErrURLBlocked
	}
	if strings.ContainsAny(parsed.Host, "\r\n") || parsed.Port() != "" && !validPort(parsed.Port()) {
		return nil, ErrURLBlocked
	}
	return parsed, nil
}

func NewSafeHTTPClient(
	policy NetworkPolicy,
	baseURL *url.URL,
	headers http.Header,
	timeout time.Duration,
) *http.Client {
	resolver := netResolver{resolver: net.DefaultResolver}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                  nil,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           32,
		MaxIdleConnsPerHost:    8,
		IdleConnTimeout:        90 * time.Second,
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  30 * time.Second,
		ExpectContinueTimeout:  time.Second,
		MaxResponseHeaderBytes: 64 << 10,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12},
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, ErrURLBlocked
		}
		addresses, err := resolveHost(ctx, resolver, host)
		if err != nil || len(addresses) == 0 {
			return nil, ErrURLBlocked
		}
		var dialErrors []error
		for _, candidate := range addresses {
			if !policy.AllowPrivate && blockedAddress(candidate) {
				return nil, ErrURLBlocked
			}
			connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.String(), port))
			if dialErr == nil {
				return connection, nil
			}
			dialErrors = append(dialErrors, dialErr)
		}
		return nil, errors.Join(dialErrors...)
	}
	baseOrigin := origin(baseURL)
	client := &http.Client{
		Transport: sameOriginHeaderTransport{
			base: responseLimitTransport{
				base: transport,
				max:  maxMCPHTTPResponseBytes,
			},
			baseOrigin: baseOrigin,
			headers:    headers.Clone(),
		},
		Timeout: timeout,
	}
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return ErrURLBlocked
		}
		if _, err := validateEndpointWithResolver(request.Context(), request.URL.String(), policy, resolver); err != nil {
			return err
		}
		if len(via) > 0 && origin(via[0].URL) != origin(request.URL) {
			request.Header.Del("Authorization")
			request.Header.Del("Cookie")
			for name := range headers {
				request.Header.Del(name)
			}
		}
		return nil
	}
	return client
}

type responseLimitTransport struct {
	base http.RoundTripper
	max  int64
}

func (t responseLimitTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if response.ContentLength > t.max {
		_ = response.Body.Close()
		return nil, ErrResponseTooLarge
	}
	response.Body = &limitedResponseBody{body: response.Body, remaining: t.max}
	return response, nil
}

type limitedResponseBody struct {
	body      io.ReadCloser
	remaining int64
}

func (b *limitedResponseBody) Read(buffer []byte) (int, error) {
	if b.remaining <= 0 {
		var probe [1]byte
		n, err := b.body.Read(probe[:])
		if n > 0 || err == nil {
			return 0, ErrResponseTooLarge
		}
		return 0, err
	}
	if int64(len(buffer)) > b.remaining {
		buffer = buffer[:b.remaining]
	}
	n, err := b.body.Read(buffer)
	b.remaining -= int64(n)
	return n, err
}

func (b *limitedResponseBody) Close() error {
	return b.body.Close()
}

type sameOriginHeaderTransport struct {
	base       http.RoundTripper
	baseOrigin string
	headers    http.Header
}

func (t sameOriginHeaderTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	if origin(clone.URL) == t.baseOrigin {
		for name, values := range t.headers {
			clone.Header.Del(name)
			for _, value := range values {
				clone.Header.Add(name, value)
			}
		}
	}
	return t.base.RoundTrip(clone)
}

func origin(value *url.URL) string {
	if value == nil {
		return ""
	}
	port := value.Port()
	if port == "" {
		if value.Scheme == "https" {
			port = "443"
		} else if value.Scheme == "http" {
			port = "80"
		}
	}
	return strings.ToLower(value.Scheme) + "://" + strings.ToLower(value.Hostname()) + ":" + port
}

func resolveHost(ctx context.Context, resolver Resolver, host string) ([]netip.Addr, error) {
	host = strings.TrimSpace(strings.TrimSuffix(host, "."))
	if parsed, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{parsed.Unmap()}, nil
	}
	addresses, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	result := make([]netip.Addr, 0, len(addresses))
	seen := map[netip.Addr]struct{}{}
	for _, address := range addresses {
		address = address.Unmap()
		if !address.IsValid() {
			continue
		}
		if _, exists := seen[address]; exists {
			continue
		}
		seen[address] = struct{}{}
		result = append(result, address)
	}
	return result, nil
}

func blockedAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || address.IsUnspecified() || address.IsLoopback() ||
		address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsMulticast() ||
		!address.IsGlobalUnicast() {
		return true
	}
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

var blockedPrefixes = mustPrefixes(
	"0.0.0.0/8",
	"100.64.0.0/10",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"240.0.0.0/4",
	"2001:db8::/32",
	"2001:10::/28",
)

func mustPrefixes(values ...string) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefixes = append(prefixes, netip.MustParsePrefix(value))
	}
	return prefixes
}

func validPort(value string) bool {
	if value == "" {
		return true
	}
	port, err := strconv.Atoi(value)
	if err != nil {
		return false
	}
	return port >= 1 && port <= 65535
}
