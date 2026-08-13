package agentbroker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/safenet"
)

const (
	maxEgressRequestBytes  = int64(1 << 20)
	maxEgressResponseBytes = int64(8 << 20)
	maxEgressHeaderBytes   = 32 << 10
)

type EgressRequest struct {
	Mode       string
	Method     string
	URL        string
	Headers    http.Header
	Body       io.Reader
	BodyBytes  int64
	Credential *CredentialApplication
}

type CredentialApplication struct {
	Header string
	Prefix string
	Value  []byte
}

type EgressResponse struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
}

// EgressBroker enforces the frozen registry Egress policy. It returns no URL
// path/query in diagnostics and applies credentials only inside the Broker.
type EgressBroker struct {
	clientFactory func(safenet.Policy, *url.URL, http.Header, time.Duration) *http.Client
	semaphore     chan struct{}
	timeout       time.Duration
}

func NewEgressBroker(maxConcurrency int, timeout time.Duration) (*EgressBroker, error) {
	if maxConcurrency < 1 || maxConcurrency > 128 || timeout < time.Second || timeout > time.Minute {
		return nil, ErrInvalidInput
	}
	return &EgressBroker{clientFactory: safenet.NewHTTPClient,
		semaphore: make(chan struct{}, maxConcurrency), timeout: timeout}, nil
}

func (broker *EgressBroker) Do(ctx context.Context, policy EgressPolicy, request EgressRequest) (EgressResponse, error) {
	if broker == nil || broker.clientFactory == nil || request.Mode != policy.Mode || policy.Mode == "none" ||
		!member(policy.Mode, "allowlist", "brokered") ||
		!member(request.Method, http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete) ||
		request.BodyBytes < 0 || request.BodyBytes > maxEgressRequestBytes || headerBytes(request.Headers) > maxEgressHeaderBytes {
		return EgressResponse{}, ErrEgressDenied
	}
	if policy.Mode == "allowlist" && request.Credential != nil {
		return EgressResponse{}, ErrEgressDenied
	}
	payload, err := readExactEgressBody(request.Body, request.BodyBytes)
	if err != nil {
		return EgressResponse{}, err
	}
	defer clear(payload)
	endpoint, origins, err := egressEndpoint(request.URL, policy)
	if err != nil {
		return EgressResponse{}, err
	}
	headers := request.Headers.Clone()
	if request.Credential != nil {
		if !validCredentialApplication(*request.Credential) {
			return EgressResponse{}, ErrSecretDenied
		}
		headers.Set(request.Credential.Header, request.Credential.Prefix+string(request.Credential.Value))
	}
	select {
	case broker.semaphore <- struct{}{}:
		defer func() { <-broker.semaphore }()
	case <-ctx.Done():
		return EgressResponse{}, ctx.Err()
	}
	client := broker.clientFactory(safenet.Policy{RequireHTTPS: true, RejectIPLiteral: true,
		AllowedOrigins: origins, MaxRedirects: 3, MaxResponseBytes: maxEgressResponseBytes},
		endpoint, headers, broker.timeout)
	httpRequest, err := http.NewRequestWithContext(ctx, request.Method, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return EgressResponse{}, ErrEgressDenied
	}
	response, err := client.Do(httpRequest)
	if err != nil {
		if errors.Is(err, safenet.ErrURLBlocked) || errors.Is(err, safenet.ErrResponseTooLarge) {
			return EgressResponse{}, ErrEgressDenied
		}
		return EgressResponse{}, ErrExecutorUnavailable
	}
	return EgressResponse{StatusCode: response.StatusCode, Header: response.Header.Clone(), Body: response.Body}, nil
}

func readExactEgressBody(reader io.Reader, declared int64) ([]byte, error) {
	if declared < 0 || declared > maxEgressRequestBytes || reader == nil && declared != 0 {
		return nil, ErrEgressDenied
	}
	if reader == nil {
		return []byte{}, nil
	}
	payload, err := io.ReadAll(io.LimitReader(reader, declared+1))
	if err != nil || int64(len(payload)) != declared {
		clear(payload)
		return nil, ErrEgressDenied
	}
	return payload, nil
}

func egressEndpoint(raw string, policy EgressPolicy) (*url.URL, []string, error) {
	parsed, err := safenet.ParseEndpoint(raw, true)
	if err != nil || parsed.Scheme != "https" {
		return nil, nil, ErrEgressDenied
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	port := 443
	if parsed.Port() != "" {
		port, err = strconv.Atoi(parsed.Port())
		if err != nil {
			return nil, nil, ErrEgressDenied
		}
	}
	allowed := false
	origins := make([]string, 0, len(policy.Rules))
	for _, rule := range policy.Rules {
		if rule.Scheme != "https" {
			continue
		}
		for _, allowedPort := range rule.Ports {
			origin := "https://" + rule.Host + ":" + strconv.Itoa(allowedPort)
			origins = append(origins, origin)
			if host == rule.Host && port == allowedPort {
				allowed = true
			}
		}
	}
	if !allowed {
		return nil, nil, ErrEgressDenied
	}
	return parsed, origins, nil
}

func EgressDestinationFingerprint(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	destination := safenet.Origin(parsed)
	digest := sha256.Sum256([]byte("neo-egress-destination-v1\x00" + destination))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func headerBytes(headers http.Header) int {
	total := 0
	for name, values := range headers {
		total += len(name)
		for _, value := range values {
			total += len(value)
		}
	}
	return total
}

func validCredentialApplication(value CredentialApplication) bool {
	return value.Header != "" && len(value.Header) <= 64 && !strings.ContainsAny(value.Header, "\x00\r\n") &&
		len(value.Prefix) <= 64 && !strings.ContainsAny(value.Prefix, "\x00\r\n") &&
		len(value.Value) > 0 && len(value.Value) <= 64<<10 && !strings.ContainsAny(string(value.Value), "\x00\r\n")
}
