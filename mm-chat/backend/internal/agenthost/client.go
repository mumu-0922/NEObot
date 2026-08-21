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

	"neo-chat/mm-chat/backend/internal/localskills"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

var (
	ErrHostUnavailable = errors.New("agent Host is unavailable")
	ErrHostProtocol    = errors.New("agent Host protocol is invalid")
)

const nativePickerRequestTimeout = 270 * time.Second
const executionRequestTimeout = 330 * time.Second

type ClientConfig struct {
	SocketPath       string
	Token            string
	ExpectedRunnerID string
	Timeout          time.Duration
}

type Client struct {
	token               string
	expectedRunnerID    string
	httpClient          *http.Client
	transport           *http.Transport
	pickerHTTPClient    *http.Client
	pickerTransport     *http.Transport
	executionHTTPClient *http.Client
	executionTransport  *http.Transport
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
	newTransport := func(responseHeaderTimeout time.Duration) *http.Transport {
		dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
		return &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, "unix", socketPath)
			},
			ForceAttemptHTTP2:      false,
			MaxIdleConns:           4,
			MaxIdleConnsPerHost:    4,
			IdleConnTimeout:        30 * time.Second,
			ResponseHeaderTimeout:  responseHeaderTimeout,
			MaxResponseHeaderBytes: 16 << 10,
		}
	}
	transport := newTransport(timeout)
	pickerTransport := newTransport(nativePickerRequestTimeout)
	executionTransport := newTransport(executionRequestTimeout)
	return &Client{
		token:               config.Token,
		expectedRunnerID:    config.ExpectedRunnerID,
		httpClient:          &http.Client{Transport: transport, Timeout: timeout},
		transport:           transport,
		pickerHTTPClient:    &http.Client{Transport: pickerTransport, Timeout: nativePickerRequestTimeout},
		pickerTransport:     pickerTransport,
		executionHTTPClient: &http.Client{Transport: executionTransport, Timeout: executionRequestTimeout},
		executionTransport:  executionTransport,
	}, nil
}

func (client *Client) Close() {
	if client != nil && client.transport != nil {
		client.transport.CloseIdleConnections()
	}
	if client != nil && client.pickerTransport != nil {
		client.pickerTransport.CloseIdleConnections()
	}
	if client != nil && client.executionTransport != nil {
		client.executionTransport.CloseIdleConnections()
	}
}

func (client *Client) ExecuteTool(
	ctx context.Context,
	request ToolExecuteRequest,
	output any,
) error {
	if output == nil || !validWorkspacePathInput(request.Workspace.CanonicalPath) ||
		!validExecutionFingerprint(request.Workspace.DirectoryFingerprint) || request.Tool == "" {
		return ErrHostProtocol
	}
	request.ProtocolVersion = ProtocolVersion
	var response ToolExecuteResponse
	err := client.doWithHTTPClientLimits(
		ctx, client.executionHTTPClient, http.MethodPost, ToolExecutePath, request, &response,
		maxExecutionRequestBytes, maxExecutionResponseBytes,
	)
	if err != nil {
		return mapToolExecutionError(err)
	}
	if response.ProtocolVersion != ProtocolVersion || response.RunnerID != client.expectedRunnerID ||
		len(response.Result) == 0 {
		return ErrHostProtocol
	}
	if err := strictjson.Decode(response.Result, int(maxExecutionResponseBytes), output); err != nil {
		return ErrHostProtocol
	}
	return nil
}

func mapToolExecutionError(err error) error {
	var remote RemoteError
	if !errors.As(err, &remote) {
		return err
	}
	switch remote.Code {
	case "APPROVAL_REQUIRED":
		return localskills.ErrApprovalRequired
	case "COMMAND_BLOCKED":
		return localskills.ErrCommandBlocked
	case "ARGUMENTS_INVALID":
		return localskills.ErrInvalidCommand
	case "RUNTIME_BUSY":
		return localskills.ErrRuntimeBusy
	case "JOB_NOT_FOUND":
		return localskills.ErrJobNotFound
	case "JOB_SCOPE_INVALID":
		return localskills.ErrJobScopeInvalid
	case "FILE_NOT_FOUND":
		return localskills.ErrWorkspaceFileNotFound
	case "FILE_TOO_LARGE":
		return localskills.ErrWorkspaceFileTooLarge
	case "INVALID_UTF8":
		return localskills.ErrWorkspaceInvalidUTF8
	case "VERSION_CONFLICT":
		return localskills.ErrWorkspaceVersionConflict
	case "EDIT_CONFLICT":
		return localskills.ErrWorkspaceEditConflict
	case "PATH_INVALID", "WORKSPACE_AUTHORITY_INVALID":
		return localskills.ErrWorkspaceInvalidPath
	case "HOST_EXECUTION_UNAVAILABLE":
		return ErrHostUnavailable
	case "TOOL_NOT_AVAILABLE", "EXECUTION_FAILED", "EXECUTION_RESULT_INVALID":
		return localskills.ErrRuntimeFailed
	default:
		return ErrHostProtocol
	}
}

func (client *Client) RunnerID() string {
	if client == nil {
		return ""
	}
	return client.expectedRunnerID
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

func (client *Client) BrowseDirectories(
	ctx context.Context,
	path string,
) (DirectoryBrowseResponse, error) {
	var response DirectoryBrowseResponse
	if err := client.do(ctx, http.MethodPost, DirectoryBrowsePath, DirectoryBrowseRequest{
		ProtocolVersion: ProtocolVersion,
		Path:            path,
	}, &response); err != nil {
		return DirectoryBrowseResponse{}, err
	}
	if !validDirectoryBrowseResponse(client.expectedRunnerID, response) {
		return DirectoryBrowseResponse{}, ErrHostProtocol
	}
	if response.Entries == nil {
		response.Entries = []DirectoryEntry{}
	}
	return response, nil
}

func (client *Client) PickNativeDirectory(
	ctx context.Context,
) (NativeDirectoryPickResponse, error) {
	var response NativeDirectoryPickResponse
	if err := client.doWithHTTPClient(ctx, client.pickerHTTPClient, http.MethodPost, NativeDirectoryPickPath, NativeDirectoryPickRequest{
		ProtocolVersion: ProtocolVersion,
	}, &response); err != nil {
		return NativeDirectoryPickResponse{}, err
	}
	if response.ProtocolVersion != ProtocolVersion ||
		response.RunnerID != client.expectedRunnerID ||
		(response.Cancelled && response.Workspace != nil) ||
		(!response.Cancelled && (response.Workspace == nil ||
			!validWorkspaceDescriptor(client.expectedRunnerID, *response.Workspace))) {
		return NativeDirectoryPickResponse{}, ErrHostProtocol
	}
	return response, nil
}

func validDirectoryBrowseResponse(runnerID string, value DirectoryBrowseResponse) bool {
	if value.ProtocolVersion != ProtocolVersion || value.RunnerID != runnerID ||
		!filepath.IsAbs(value.Path) || !validWorkspacePathInput(value.Path) ||
		!validWorkspacePathInput(value.DisplayPath) ||
		(value.PathKind != "wsl" && value.PathKind != "windows-mounted") ||
		(value.ParentPath != "" && (!filepath.IsAbs(value.ParentPath) ||
			!validWorkspacePathInput(value.ParentPath))) || len(value.Entries) > 256 {
		return false
	}
	for _, entry := range value.Entries {
		if !validDirectoryName(entry.Name) || !filepath.IsAbs(entry.Path) ||
			!validWorkspacePathInput(entry.Path) || !validWorkspacePathInput(entry.DisplayPath) ||
			(entry.PathKind != "wsl" && entry.PathKind != "windows-mounted") {
			return false
		}
	}
	return true
}

func (client *Client) do(
	ctx context.Context,
	method string,
	path string,
	input any,
	output any,
) error {
	if client == nil {
		return ErrHostUnavailable
	}
	return client.doWithHTTPClient(ctx, client.httpClient, method, path, input, output)
}

func (client *Client) doWithHTTPClient(
	ctx context.Context,
	httpClient *http.Client,
	method string,
	path string,
	input any,
	output any,
) error {
	return client.doWithHTTPClientLimits(
		ctx, httpClient, method, path, input, output,
		maxControlRequestBytes, maxControlResponseBytes,
	)
}

func (client *Client) doWithHTTPClientLimits(
	ctx context.Context,
	httpClient *http.Client,
	method string,
	path string,
	input any,
	output any,
	requestLimit int64,
	responseLimit int64,
) error {
	if client == nil || httpClient == nil {
		return ErrHostUnavailable
	}
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil || int64(len(encoded)) > requestLimit {
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
	response, err := httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrHostUnavailable
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, responseLimit+1))
	if err != nil || int64(len(data)) > responseLimit {
		return ErrHostProtocol
	}
	mediaType, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaErr != nil || mediaType != "application/json" {
		return ErrHostProtocol
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure ErrorResponse
		if strictjson.Decode(data, int(responseLimit), &failure) != nil ||
			!validRemoteErrorCode(failure.Error.Code) {
			return ErrHostProtocol
		}
		return RemoteError{
			StatusCode: response.StatusCode,
			Code:       failure.Error.Code,
			Message:    "Agent Host request failed",
		}
	}
	if strictjson.Decode(data, int(responseLimit), output) != nil {
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
		"DIRECTORY_BROWSE_UNAVAILABLE",
		"NATIVE_DIRECTORY_PICKER_UNAVAILABLE",
		"HOST_EXECUTION_UNAVAILABLE",
		"WORKSPACE_AUTHORITY_INVALID",
		"TOOL_NOT_AVAILABLE", "EXECUTION_FAILED", "EXECUTION_RESULT_INVALID",
		"APPROVAL_REQUIRED", "COMMAND_BLOCKED", "ARGUMENTS_INVALID", "RUNTIME_BUSY",
		"JOB_NOT_FOUND", "JOB_SCOPE_INVALID", "FILE_NOT_FOUND", "FILE_TOO_LARGE",
		"INVALID_UTF8", "VERSION_CONFLICT", "EDIT_CONFLICT", "PATH_INVALID",
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
