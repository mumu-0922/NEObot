package agentbrokerrelay

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

type Client struct {
	endpoint *url.URL
	client   *http.Client
}

func NewClient(endpoint string, files agentrunner.ClientTLSFiles, timeout time.Duration) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	port, portErr := 0, error(nil)
	if parsed != nil {
		port, portErr = strconv.Atoi(parsed.Port())
	}
	ip := net.IP(nil)
	if parsed != nil {
		ip = net.ParseIP(parsed.Hostname())
	}
	if err != nil || parsed == nil || parsed.Scheme != "https" || parsed.Path != Path ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil || ip == nil ||
		(!ip.IsLoopback() && !ip.IsPrivate()) || portErr != nil || port < 1 || port > 65535 ||
		timeout < time.Second || timeout > time.Minute {
		return nil, agentrunner.ErrInvalidInput
	}
	tlsConfig, err := agentrunner.LoadClientTLS(files)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{TLSClientConfig: tlsConfig, DisableCompression: true,
		DisableKeepAlives: true, ForceAttemptHTTP2: true, MaxResponseHeaderBytes: 16 << 10,
		DialContext: (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: -1}).DialContext}
	return &Client{endpoint: parsed, client: &http.Client{Transport: transport, Timeout: timeout}}, nil
}

func (client *Client) Prepare(ctx context.Context, input agentrunner.PrepareRequest) (agentrunner.PrepareResult, error) {
	var decoded response
	if err := client.call(ctx, agentrunner.MethodPrepare, input, &decoded); err != nil {
		return agentrunner.PrepareResult{}, err
	}
	if decoded.Prepare == nil || decoded.Commit != nil {
		return agentrunner.PrepareResult{}, agentrunner.ErrRuntimeUnavailable
	}
	return *decoded.Prepare, nil
}

func (client *Client) Commit(ctx context.Context, input agentrunner.CommitRequest) (agentrunner.CommitResult, error) {
	var decoded response
	if err := client.call(ctx, agentrunner.MethodCommit, input, &decoded); err != nil {
		return agentrunner.CommitResult{}, err
	}
	if decoded.Commit == nil || decoded.Prepare != nil {
		return agentrunner.CommitResult{}, agentrunner.ErrRuntimeUnavailable
	}
	return *decoded.Commit, nil
}

func (client *Client) call(ctx context.Context, method string, body any, decoded *response) error {
	if client == nil || client.endpoint == nil || client.client == nil || decoded == nil {
		return agentrunner.ErrRuntimeUnavailable
	}
	payload, err := json.Marshal(struct {
		SchemaVersion string `json:"schemaVersion"`
		Method        string `json:"method"`
		Body          any    `json:"body"`
	}{ProtocolVersion, method, body})
	if err != nil || len(payload) > maxRelayBytes {
		return agentrunner.ErrInvalidInput
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return agentrunner.ErrInvalidInput
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	httpResponse, err := client.client.Do(request)
	if err != nil {
		return agentrunner.ErrRuntimeUnavailable
	}
	defer httpResponse.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxRelayBytes+1))
	if err != nil || len(raw) > maxRelayBytes || strictjson.Decode(raw, maxRelayBytes, decoded) != nil ||
		decoded.SchemaVersion != ProtocolVersion {
		return agentrunner.ErrRuntimeUnavailable
	}
	if decoded.Error != nil {
		return relayCodeError(decoded.Error.Code)
	}
	if httpResponse.StatusCode != http.StatusOK || decoded.Method != method {
		return agentrunner.ErrRuntimeUnavailable
	}
	return nil
}

func relayCodeError(code string) error {
	for _, mapping := range []struct {
		code string
		err  error
	}{
		{agentrunner.ErrorAuthFailed, agentrunner.ErrAuthFailed},
		{agentrunner.ErrorReplayDetected, agentrunner.ErrReplayDetected},
		{agentrunner.ErrorVersionUnsupported, agentrunner.ErrVersionUnsupported},
		{agentrunner.ErrorRuntimeUnavailable, agentrunner.ErrRuntimeUnavailable},
		{agentrunner.ErrorSnapshotMismatch, agentrunner.ErrSnapshotMismatch},
		{agentrunner.ErrorGrantDenied, agentrunner.ErrGrantDenied},
		{agentrunner.ErrorLeaseStale, agentrunner.ErrLeaseStale},
		{agentrunner.ErrorKillSwitchActive, agentrunner.ErrKillSwitchActive},
		{agentrunner.ErrorBudgetExhausted, agentrunner.ErrBudgetExhausted},
		{agentrunner.ErrorApprovalRequired, agentrunner.ErrApprovalRequired},
		{agentrunner.ErrorApprovalDenied, agentrunner.ErrApprovalDenied},
		{agentrunner.ErrorIntentExpired, agentrunner.ErrIntentExpired},
		{agentrunner.ErrorArtifactDenied, agentrunner.ErrArtifactDenied},
		{agentrunner.ErrorProjectMutationDenied, agentrunner.ErrProjectMutationDenied},
		{agentrunner.ErrorExecutorUnavailable, agentrunner.ErrExecutorUnavailable},
		{agentrunner.ErrorInvalidTransition, agentrunner.ErrInvalidTransition},
		{agentrunner.ErrorOutcomeUnknown, agentrunner.ErrOutcomeUnknown},
	} {
		if code == mapping.code {
			return mapping.err
		}
	}
	return agentrunner.ErrRuntimeUnavailable
}
