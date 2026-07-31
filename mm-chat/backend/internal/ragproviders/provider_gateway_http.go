package ragproviders

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	providerGatewayTimeout          = 40 * time.Second
	providerGatewayMaxResponseBytes = 16 << 20
)

func newProviderGatewayHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 nil,
			ForceAttemptHTTP2:     true,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       30 * time.Second,
		},
		Timeout: providerGatewayTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func newAccuracyFirstDevelopmentProviderGatewayHTTPClient() *http.Client {
	return NewAccuracyFirstDevelopmentHTTPClient()
}

// NewAccuracyFirstDevelopmentHTTPClient returns the schema-v12 transport
// shared by fixed BGE and Luna. It has no elapsed-time deadline, rejects
// redirects and environment proxies, and leaves cancellation to the caller.
func NewAccuracyFirstDevelopmentHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:               nil,
			ForceAttemptHTTP2:   true,
			TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
			DialContext:         (&net.Dialer{KeepAlive: 30 * time.Second}).DialContext,
			IdleConnTimeout:     30 * time.Second,
			TLSHandshakeTimeout: 0,
		},
		Timeout: 0,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (gateway *ProviderGateway) providerJSON(
	ctx context.Context,
	method string,
	endpoint string,
	credential string,
	body []byte,
	maxResponseBytes int64,
) ([]byte, error) {
	if gateway == nil || gateway.httpClient == nil ||
		maxResponseBytes < 1 || maxResponseBytes > providerGatewayMaxResponseBytes {
		return nil, ErrProviderGatewayUnavailable
	}
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, ErrProviderGatewayUpstream
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("Authorization", "Bearer "+credential)
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := gateway.httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ErrProviderGatewayUpstream
		}
		return nil, newProviderRetryError("")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if providerGatewayRetryableStatus(response.StatusCode) {
			return nil, newProviderRetryError(response.Header.Get("Retry-After"))
		}
		return nil, ErrProviderGatewayUpstream
	}
	if !providerGatewayJSONContentType(response.Header.Get("Content-Type")) {
		return nil, ErrProviderGatewayUpstream
	}
	contentEncoding := strings.TrimSpace(response.Header.Get("Content-Encoding"))
	if contentEncoding != "" && !strings.EqualFold(contentEncoding, "identity") {
		return nil, ErrProviderGatewayUpstream
	}
	if response.ContentLength > maxResponseBytes {
		return nil, ErrProviderGatewayUpstream
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ErrProviderGatewayUpstream
		}
		return nil, newProviderRetryError("")
	}
	if len(raw) == 0 || int64(len(raw)) > maxResponseBytes {
		return nil, ErrProviderGatewayUpstream
	}
	return raw, nil
}

func decodeProviderJSON(raw []byte, target any, strict bool) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(target); err != nil {
		return ErrProviderGatewayUpstream
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return ErrProviderGatewayUpstream
	}
	return nil
}

func providerGatewayJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && mediaType == "application/json"
}
