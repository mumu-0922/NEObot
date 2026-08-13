package agentrunner

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/strictjson"
)

type ClientTLSFiles struct {
	CertificateFile string
	KeyFile         string
	ServerCAFile    string
	ServerName      string
}

type RPCClient struct {
	endpoint *url.URL
	client   *http.Client
}

func NewRPCClient(endpoint string, files ClientTLSFiles, timeout time.Duration) (*RPCClient, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Scheme != "https" || parsed.Path != rpcPath || parsed.RawQuery != "" ||
		parsed.Fragment != "" || parsed.User != nil || parsed.Hostname() == "" ||
		(!net.ParseIP(parsed.Hostname()).IsLoopback() && !isPrivateIP(net.ParseIP(parsed.Hostname()))) ||
		timeout < time.Second || timeout > time.Minute {
		return nil, ErrInvalidInput
	}
	tlsConfig, err := LoadClientTLS(files)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{TLSClientConfig: tlsConfig, DisableCompression: true,
		DisableKeepAlives: true, ForceAttemptHTTP2: true, MaxResponseHeaderBytes: 16 << 10,
		DialContext: (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: -1}).DialContext}
	return &RPCClient{endpoint: parsed, client: &http.Client{Transport: transport, Timeout: timeout}}, nil
}

func LoadClientTLS(files ClientTLSFiles) (*tls.Config, error) {
	for _, path := range []string{files.CertificateFile, files.KeyFile, files.ServerCAFile} {
		if !strings.HasPrefix(strings.TrimSpace(path), "/") {
			return nil, errors.New("Runner client TLS path is invalid")
		}
	}
	if err := securePrivateFile(files.KeyFile); err != nil {
		return nil, err
	}
	certificate, err := tls.LoadX509KeyPair(files.CertificateFile, files.KeyFile)
	if err != nil {
		return nil, errors.New("Runner client TLS identity is unavailable")
	}
	caBytes, err := os.ReadFile(files.ServerCAFile)
	if err != nil || len(caBytes) > 1<<20 {
		return nil, errors.New("Runner server CA is unavailable")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caBytes) || !identityPattern.MatchString(files.ServerName) {
		return nil, errors.New("Runner server trust is invalid")
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13,
		Certificates: []tls.Certificate{certificate}, RootCAs: pool, ServerName: files.ServerName,
		SessionTicketsDisabled: true}, nil
}

func (client *RPCClient) Call(ctx context.Context, request Request) (Response, error) {
	if client == nil || client.client == nil || client.endpoint == nil || len(request.Canonical) == 0 {
		return Response{}, ErrInvalidInput
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint.String(), bytes.NewReader(request.Canonical))
	if err != nil {
		return Response{}, ErrInvalidInput
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	response, err := client.client.Do(httpRequest)
	if err != nil {
		return Response{}, ErrRuntimeUnavailable
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxRPCBytes+1))
	if err != nil || len(body) > maxRPCBytes {
		return Response{}, ErrRuntimeUnavailable
	}
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Error RPCError `json:"error"`
		}
		if strictjson.Decode(body, maxRPCBytes, &failure) != nil || failure.Error.Code == "" {
			return Response{}, ErrRuntimeUnavailable
		}
		return Response{}, rpcCodeError(failure.Error.Code)
	}
	decoded, err := decodeReplayResponse(request.Method, body)
	if err != nil || decoded.RequestID != request.RequestID || decoded.Nonce != request.Nonce ||
		decoded.Method != request.Method+".result" {
		return Response{}, ErrRuntimeUnavailable
	}
	return decoded, nil
}

func rpcCodeError(code string) error {
	switch code {
	case ErrorAuthFailed:
		return ErrAuthFailed
	case ErrorReplayDetected:
		return ErrReplayDetected
	case ErrorVersionUnsupported:
		return ErrVersionUnsupported
	case ErrorRuntimeUnavailable:
		return ErrRuntimeUnavailable
	case ErrorIsolationUnavailable:
		return ErrIsolationUnavailable
	case ErrorSnapshotMismatch:
		return ErrSnapshotMismatch
	case ErrorLeaseStale:
		return ErrLeaseStale
	case ErrorKillSwitchActive:
		return ErrKillSwitchActive
	case ErrorInvalidTransition:
		return ErrInvalidTransition
	default:
		return ErrRuntimeUnavailable
	}
}

func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsPrivate()
}
