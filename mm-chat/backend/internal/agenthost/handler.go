package agenthost

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"runtime"
	"strings"

	"neo-chat/mm-chat/backend/internal/strictjson"
)

type HandlerConfig struct {
	RunnerID string
	Version  string
	Token    string
	Resolver WorkspaceResolver
}

type Handler struct {
	runnerID     string
	tokenSum     [sha256.Size]byte
	resolver     WorkspaceResolver
	capabilities Capabilities
}

func NewHandler(config HandlerConfig) (*Handler, error) {
	if err := validateRunnerID(config.RunnerID); err != nil {
		return nil, err
	}
	if err := validateToken(config.Token); err != nil {
		return nil, err
	}
	platform := runtime.GOOS
	if runtime.GOOS == "linux" && isWSLRuntime() {
		platform = "linux-wsl"
	}
	features := HostFeatures{PermissionModes: []PermissionMode{}}
	if config.Resolver != nil {
		features.WorkspaceResolve = true
		features.WindowsPathInterop = config.Resolver.WindowsPathInterop()
	}
	return &Handler{
		runnerID: config.RunnerID,
		tokenSum: sha256.Sum256([]byte(config.Token)),
		resolver: config.Resolver,
		capabilities: Capabilities{
			ProtocolVersion: ProtocolVersion,
			RunnerID:        config.RunnerID,
			Version:         normalizeVersion(config.Version),
			Platform:        platform,
			Architecture:    runtime.GOARCH,
			Features:        features,
			Limits: HostLimits{
				MaxRequestBytes:  maxControlRequestBytes,
				MaxResponseBytes: maxControlResponseBytes,
				MaxPathBytes:     maxWorkspacePathBytes,
			},
		},
	}, nil
}

func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if !handler.authorized(request) {
		writer.Header().Set("WWW-Authenticate", `Bearer realm="neo-chat-agent-host"`)
		handler.writeError(writer, http.StatusUnauthorized, "AGENT_HOST_UNAUTHORIZED", "Agent Host authorization failed")
		return
	}
	if request.URL.RawQuery != "" || request.URL.Fragment != "" {
		handler.writeError(writer, http.StatusBadRequest, "AGENT_HOST_REQUEST_INVALID", "Agent Host request is invalid")
		return
	}
	switch request.URL.Path {
	case CapabilitiesPath:
		if request.Method != http.MethodGet {
			handler.methodNotAllowed(writer, http.MethodGet)
			return
		}
		handler.writeJSON(writer, http.StatusOK, handler.capabilities)
	case WorkspaceResolvePath:
		if request.Method != http.MethodPost {
			handler.methodNotAllowed(writer, http.MethodPost)
			return
		}
		handler.resolveWorkspace(writer, request)
	default:
		handler.writeError(writer, http.StatusNotFound, "AGENT_HOST_ROUTE_NOT_FOUND", "Agent Host route not found")
	}
}

func (handler *Handler) authorized(request *http.Request) bool {
	value := strings.TrimSpace(request.Header.Get("Authorization"))
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") ||
		validateToken(parts[1]) != nil {
		return false
	}
	candidate := sha256.Sum256([]byte(parts[1]))
	return subtle.ConstantTimeCompare(candidate[:], handler.tokenSum[:]) == 1
}

func (handler *Handler) resolveWorkspace(writer http.ResponseWriter, request *http.Request) {
	if handler.resolver == nil {
		handler.writeError(
			writer, http.StatusServiceUnavailable,
			"WORKSPACE_RESOLVE_UNAVAILABLE", "Workspace resolution is unavailable",
		)
		return
	}
	var input WorkspaceResolveRequest
	if err := decodeStrictJSON(writer, request, &input); err != nil {
		handler.writeError(
			writer, http.StatusBadRequest,
			"AGENT_HOST_REQUEST_INVALID", "Agent Host request is invalid",
		)
		return
	}
	if input.ProtocolVersion != ProtocolVersion {
		handler.writeError(
			writer, http.StatusConflict,
			"AGENT_HOST_PROTOCOL_UNSUPPORTED", "Agent Host protocol version is unsupported",
		)
		return
	}
	workspace, err := handler.resolver.ResolveWorkspace(request.Context(), input.Path)
	if err != nil {
		switch {
		case errors.Is(err, ErrWindowsInteropUnavailable):
			handler.writeError(
				writer, http.StatusServiceUnavailable,
				"WINDOWS_PATH_INTEROP_UNAVAILABLE", "Windows path interop is unavailable",
			)
		case errors.Is(err, ErrWorkspacePathUnavailable):
			handler.writeError(writer, http.StatusNotFound, "WORKSPACE_PATH_UNAVAILABLE", "Workspace path is unavailable")
		default:
			handler.writeError(writer, http.StatusBadRequest, "WORKSPACE_PATH_INVALID", "Workspace path is invalid")
		}
		return
	}
	handler.writeJSON(writer, http.StatusOK, WorkspaceResolveResponse{
		ProtocolVersion: ProtocolVersion,
		RunnerID:        handler.runnerID,
		Workspace:       workspace,
	})
}

func decodeStrictJSON(writer http.ResponseWriter, request *http.Request, output any) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("invalid content type")
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxControlRequestBytes)
	data, err := io.ReadAll(request.Body)
	if err != nil {
		return err
	}
	return strictjson.Decode(data, int(maxControlRequestBytes), output)
}

func (handler *Handler) methodNotAllowed(writer http.ResponseWriter, allowed string) {
	writer.Header().Set("Allow", allowed)
	handler.writeError(
		writer, http.StatusMethodNotAllowed,
		"AGENT_HOST_METHOD_NOT_ALLOWED", "Agent Host method not allowed",
	)
}

func (handler *Handler) writeError(writer http.ResponseWriter, status int, code, message string) {
	handler.writeJSON(writer, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}

func (handler *Handler) writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func isWSLRuntime() bool {
	data, err := osReadFile("/proc/sys/kernel/osrelease")
	return err == nil && strings.Contains(strings.ToLower(string(data)), "microsoft")
}

var osReadFile = func(name string) ([]byte, error) {
	return os.ReadFile(name)
}
