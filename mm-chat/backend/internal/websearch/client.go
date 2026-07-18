package websearch

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxErrorBodyBytes = 4096

type providerClient struct {
	id       ProviderID
	endpoint string
	apiKey   string
	doer     HTTPDoer
}

func newProviderClient(
	id ProviderID,
	config Config,
	defaultBaseURL string,
	path string,
	requireAPIKey bool,
) (providerClient, error) {
	apiKey, err := normalizeAPIKey(config.APIKey, requireAPIKey)
	if err != nil {
		return providerClient{}, err
	}
	baseURL, err := normalizeProviderBaseURL(config.BaseURL, defaultBaseURL)
	if err != nil {
		return providerClient{}, err
	}
	endpoint := *baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path

	doer := config.Client
	if doer == nil {
		doer = newSecureHTTPClient()
	}
	return providerClient{
		id: id, endpoint: endpoint.String(), apiKey: apiKey, doer: doer,
	}, nil
}

func (c providerClient) postJSON(
	ctx context.Context,
	body any,
	headers map[string]string,
	output any,
) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return &ProviderError{Provider: c.id, Code: "REQUEST_ENCODE_FAILED"}
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.endpoint,
		strings.NewReader(string(payload)),
	)
	if err != nil {
		return &ProviderError{Provider: c.id, Code: "REQUEST_BUILD_FAILED"}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := c.doer.Do(req)
	if err != nil {
		return &ProviderError{Provider: c.id, Code: "REQUEST_FAILED"}
	}
	if resp == nil || resp.Body == nil {
		return &ProviderError{Provider: c.id, Code: "RESPONSE_INVALID"}
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBodyBytes))
		return &ProviderError{
			Provider: c.id, Code: "UPSTREAM_STATUS", Status: resp.StatusCode,
		}
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if contentType != "" && !strings.Contains(contentType, "json") {
		return &ProviderError{Provider: c.id, Code: "RESPONSE_CONTENT_TYPE_INVALID"}
	}
	contentEncoding := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	if contentEncoding != "" && contentEncoding != "identity" {
		return &ProviderError{Provider: c.id, Code: "RESPONSE_ENCODING_INVALID"}
	}

	limited := io.LimitReader(resp.Body, MaxResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return &ProviderError{Provider: c.id, Code: "RESPONSE_READ_FAILED"}
	}
	if len(data) > MaxResponseBytes {
		return &ProviderError{Provider: c.id, Code: "RESPONSE_TOO_LARGE"}
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if err := decoder.Decode(output); err != nil {
		return &ProviderError{Provider: c.id, Code: "RESPONSE_DECODE_FAILED"}
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return &ProviderError{Provider: c.id, Code: "RESPONSE_DECODE_FAILED"}
	}
	return nil
}

func normalizeProviderBaseURL(value, fallback string) (*url.URL, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	if len(value) > MaxSourceURLBytes {
		return nil, ErrInvalidConfig
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, ErrInvalidConfig
	}
	if isLocalHostname(parsed.Hostname()) {
		return nil, ErrInvalidConfig
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && !isPublicIP(ip) {
		return nil, ErrInvalidConfig
	}
	return parsed, nil
}

func newSecureHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		DialContext:           dialPublicAddress,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("web search redirects are disabled")
		},
	}
}

func dialPublicAddress(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("resolve web search address: %w", err)
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("resolve web search address failed")
	}
	for _, address := range addresses {
		if !isPublicIP(net.IP(address.AsSlice())) {
			return nil, errors.New("web search address is not public")
		}
	}
	dialer := net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].String(), port))
}

func isPublicIP(ip net.IP) bool {
	return ip != nil && !ip.IsUnspecified() && !ip.IsLoopback() &&
		!ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() &&
		!ip.IsMulticast()
}

func isLocalHostname(hostname string) bool {
	hostname = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(hostname), "."))
	return hostname == "localhost" || strings.HasSuffix(hostname, ".localhost")
}
