package agenthost

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/strictjson"
)

var (
	ErrHostUnavailable = errors.New("agent Host is unavailable")
	ErrHostProtocol    = errors.New("agent Host protocol is invalid")
)

type ClientConfig struct {
	SocketPath       string
	Token            string
	ExpectedRunnerID string
	Timeout          time.Duration
}

type Client struct {
	token            string
	expectedRunnerID string
	httpClient       *http.Client
	transport        *http.Transport
}

func NewClient(config ClientConfig) (*Client, error) {
	socketPath := filepath.Clean(strings.TrimSpace(config.SocketPath))
	if !filepath.IsAbs(socketPath) || containsControl(socketPath) ||
		len(socketPath) > maxUnixSocketPathBytes {
		return nil, ErrHostUnavailable
	}
	if err := validateToken(config.Token); err != nil {
		return nil, err
	}
	if err := validateRunnerID(config.ExpectedRunnerID); err != nil {
		return nil, err
	}
	timeout := config.Timeout
	if timeout <= 0 || timeout > time.Minute {
		timeout = 15 * time.Second
	}
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socketPath)
		},
		ForceAttemptHTTP2:      false,
		MaxIdleConns:           4,
		MaxIdleConnsPerHost:    4,
		IdleConnTimeout:        30 * time.Second,
		ResponseHeaderTimeout:  timeout,
		MaxResponseHeaderBytes: 16 << 10,
	}
	return &Client{
		token:            config.Token,
		expectedRunnerID: config.ExpectedRunnerID,
		httpClient:       &http.Client{Transport: transport, Timeout: timeout},
		transport:        transport,
	}, nil
}

func (client *Client) Close() {
	if client != nil && client.transport != nil {
		client.transport.CloseIdleConnections()
	}
}

func (client *Client) Capabilities(ctx context.Context) (Capabilities, error) {
	var response Capabilities
	if err := client.do(ctx, http.MethodGet, CapabilitiesPath, nil, &response); err != nil {
		return Capabilities{}, err
	}
	if response.ProtocolVersion != ProtocolVersion ||
		response.RunnerID != client.expectedRunnerID ||
		validateRunnerID(response.RunnerID) != nil ||
		!validCapabilities(response) {
		return Capabilities{}, ErrHostProtocol
	}
	if response.Features.PermissionModes == nil {
		response.Features.PermissionModes = []PermissionMode{}
	}
	return response, nil
}

func validCapabilities(value Capabilities) bool {
	if value.Version == "" || len(value.Version) > maxVersionBytes || containsControl(value.Version) ||
		!validCapabilityLabel(value.Platform) || !validCapabilityLabel(value.Architecture) ||
		value.Limits.MaxRequestBytes <= 0 || value.Limits.MaxRequestBytes > maxControlRequestBytes ||
		value.Limits.MaxResponseBytes <= 0 || value.Limits.MaxResponseBytes > maxControlResponseBytes ||
		value.Limits.MaxPathBytes <= 0 || value.Limits.MaxPathBytes > maxWorkspacePathBytes {
		return false
	}
	seenModes := make(map[PermissionMode]struct{}, len(value.Features.PermissionModes))
	for _, mode := range value.Features.PermissionModes {
		if mode != PermissionReadOnly && mode != PermissionWorkspaceWrite && mode != PermissionFullAccess {
			return false
		}
		if _, duplicate := seenModes[mode]; duplicate {
			return false
		}
		seenModes[mode] = struct{}{}
	}
	return true
}

func validCapabilityLabel(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' ||
			char == '.' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func (client *Client) ResolveWorkspace(
	ctx context.Context,
	path string,
) (WorkspaceDescriptor, error) {
	var response WorkspaceResolveResponse
	if err := client.do(ctx, http.MethodPost, WorkspaceResolvePath, WorkspaceResolveRequest{
		ProtocolVersion: ProtocolVersion,
		Path:            path,
	}, &response); err != nil {
		return WorkspaceDescriptor{}, err
	}
	if response.ProtocolVersion != ProtocolVersion ||
		response.RunnerID != client.expectedRunnerID ||
		!validWorkspaceDescriptor(client.expectedRunnerID, response.Workspace) {
		return WorkspaceDescriptor{}, ErrHostProtocol
	}
	return response.Workspace, nil
}

func (client *Client) do(
	ctx context.Context,
	method string,
	path string,
	input any,
	output any,
) error {
	if client == nil || client.httpClient == nil {
		return ErrHostUnavailable
	}
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil || int64(len(encoded)) > maxControlRequestBytes {
			return ErrHostProtocol
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(
		ctx, method, "http://agent-host.internal"+path, body,
	)
	if err != nil {
		return ErrHostProtocol
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Accept", "application/json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrHostUnavailable
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxControlResponseBytes+1))
	if err != nil || int64(len(data)) > maxControlResponseBytes {
		return ErrHostProtocol
	}
	mediaType, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaErr != nil || mediaType != "application/json" {
		return ErrHostProtocol
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure ErrorResponse
		if decodeOneJSON(data, &failure) != nil || !validRemoteErrorCode(failure.Error.Code) {
			return ErrHostProtocol
		}
		return RemoteError{
			StatusCode: response.StatusCode,
			Code:       failure.Error.Code,
			Message:    "Agent Host request failed",
		}
	}
	if decodeOneJSON(data, output) != nil {
		return ErrHostProtocol
	}
	return nil
}

func validRemoteErrorCode(value string) bool {
	switch value {
	case "AGENT_HOST_UNAUTHORIZED",
		"AGENT_HOST_REQUEST_INVALID",
		"AGENT_HOST_PROTOCOL_UNSUPPORTED",
		"AGENT_HOST_METHOD_NOT_ALLOWED",
		"AGENT_HOST_ROUTE_NOT_FOUND",
		"WORKSPACE_RESOLVE_UNAVAILABLE",
		"WINDOWS_PATH_INTEROP_UNAVAILABLE",
		"WORKSPACE_PATH_INVALID",
		"WORKSPACE_PATH_UNAVAILABLE":
		return true
	default:
		return false
	}
}

func decodeOneJSON(data []byte, output any) error {
	return strictjson.Decode(data, int(maxControlResponseBytes), output)
}

func validWorkspaceDescriptor(runnerID string, value WorkspaceDescriptor) bool {
	fingerprint := strings.TrimPrefix(value.DirectoryFingerprint, "sha256:")
	decodedFingerprint, err := hex.DecodeString(fingerprint)
	expectedFingerprint := sha256.Sum256([]byte(runnerID + "\x00" + value.CanonicalPath))
	return err == nil && len(decodedFingerprint) == sha256.Size &&
		fingerprint == strings.ToLower(fingerprint) &&
		fingerprint == hex.EncodeToString(expectedFingerprint[:]) &&
		validWorkspacePathInput(value.CanonicalPath) && filepath.IsAbs(value.CanonicalPath) &&
		validWorkspacePathInput(value.DisplayPath) &&
		(value.PathKind == "wsl" || value.PathKind == "windows-mounted") &&
		strings.HasPrefix(value.DirectoryFingerprint, "sha256:")
}
