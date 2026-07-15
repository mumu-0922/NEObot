package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/runtimeconfig"
)

const (
	contentTypeJSON        = "application/json; charset=utf-8"
	maxExecutionBodyBytes  = 128 << 10
	maxPluginResponseBytes = 2 << 20
	defaultTimeout         = 30 * time.Second
)

var (
	ErrPluginBaseURLMissing       = errors.New("plugin base URL is missing")
	ErrPluginFunctionMissing      = errors.New("plugin function is missing")
	ErrPluginFunctionMismatch     = errors.New("plugin function is not declared by this plugin")
	ErrPluginPathInvalid          = errors.New("plugin path is invalid")
	ErrPluginMethodUnsupported    = errors.New("plugin method is not supported")
	ErrPluginPathArgsMissing      = errors.New("plugin path parameters are missing")
	ErrPluginURLBlocked           = errors.New("plugin URL is blocked by policy")
	ErrPluginAuthRequired         = errors.New("plugin authentication is required")
	ErrPluginAuthUnsupported      = errors.New("plugin authentication type is not supported")
	ErrPlaintextPluginAuth        = errors.New("plaintext plugin auth is not accepted")
	ErrPluginRequestFailed        = errors.New("plugin request failed")
	ErrPluginResponseTooLarge     = errors.New("plugin response is too large")
	ErrPluginExecutionPayload     = errors.New("plugin execution payload is invalid")
	ErrPluginExecutionUnsupported = errors.New("plugin execution requires a plugin manifest payload until the registry is implemented")
)

type SecretDecrypter func(*runtimeconfig.EncryptedSecretEnvelope, string) (string, error)

type ServiceOption func(*Service)

type Service struct {
	httpClient          *http.Client
	secretDecrypter     SecretDecrypter
	allowPrivateNetwork bool
	maxResponseBytes    int64
}

type Handler struct {
	service *Service
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ListResponse struct {
	Plugins     []PluginSummary `json:"plugins"`
	Unavailable bool            `json:"unavailable,omitempty"`
}

type PluginSummary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	LogoURL     string `json:"logoUrl,omitempty"`
	ManifestURL string `json:"manifestUrl,omitempty"`
}

type ExecuteResponse struct {
	Result any `json:"result"`
}

type ExecuteRequest struct {
	Plugin       *Plugin           `json:"plugin"`
	FunctionDef  *PluginFunction   `json:"functionDef"`
	Args         map[string]any    `json:"args"`
	AuthConfig   *PluginAuthConfig `json:"authConfig,omitempty"`
	PluginID     string            `json:"pluginId,omitempty"`
	FunctionName string            `json:"functionName,omitempty"`
	CallID       string            `json:"callId,omitempty"`
}

type Plugin struct {
	ID              string           `json:"id"`
	Title           string           `json:"title"`
	Description     string           `json:"description"`
	LogoURL         string           `json:"logoUrl"`
	ManifestURL     string           `json:"manifestUrl"`
	ExternalDocsURL string           `json:"externalDocsUrl,omitempty"`
	BaseURL         string           `json:"baseUrl"`
	Functions       []PluginFunction `json:"functions"`
	Category        string           `json:"category,omitempty"`
	Categories      []string         `json:"categories,omitempty"`
	Added           string           `json:"added,omitempty"`
	BuiltIn         bool             `json:"builtIn,omitempty"`
	Auth            *PluginAuth      `json:"auth,omitempty"`
}

type PluginFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
	Path        string         `json:"path"`
	Method      string         `json:"method"`
	Risk        string         `json:"risk,omitempty"`
}

type PluginAuth struct {
	Type     string `json:"type"`
	Name     string `json:"name,omitempty"`
	In       string `json:"in,omitempty"`
	Required *bool  `json:"required,omitempty"`
}

type PluginAuthConfig struct {
	Type        string                                 `json:"type,omitempty"`
	Value       string                                 `json:"value,omitempty"`
	ValueSecret *runtimeconfig.EncryptedSecretEnvelope `json:"valueSecret,omitempty"`
	Key         string                                 `json:"key,omitempty"`
	AddTo       string                                 `json:"addTo,omitempty"`
}

func NewService(cfg config.Config, opts ...ServiceOption) *Service {
	runtimeConfigService := runtimeconfig.NewService(cfg)
	service := &Service{
		httpClient:       &http.Client{Timeout: defaultTimeout},
		secretDecrypter:  runtimeConfigService.DecryptOptionalSecret,
		maxResponseBytes: maxPluginResponseBytes,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

func WithHTTPClient(client *http.Client) ServiceOption {
	return func(service *Service) {
		if client != nil {
			service.httpClient = client
		}
	}
}

func WithSecretDecrypter(decrypter SecretDecrypter) ServiceOption {
	return func(service *Service) {
		if decrypter != nil {
			service.secretDecrypter = decrypter
		}
	}
}

func WithAllowPrivateNetwork(allow bool) ServiceOption {
	return func(service *Service) {
		service.allowPrivateNetwork = allow
	}
}

func WithMaxResponseBytes(maxBytes int64) ServiceOption {
	return func(service *Service) {
		if maxBytes > 0 {
			service.maxResponseBytes = maxBytes
		}
	}
}

func NewHandler(service *Service) *Handler {
	if service == nil {
		service = NewService(config.Config{})
	}
	return &Handler{service: service}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/plugins":
		h.handlePlugins(w, r)
	case "/v1/plugins/install":
		h.requireMethod(w, r, http.MethodPost, h.installPlugin)
	case "/v1/plugins/execute":
		h.requireMethod(w, r, http.MethodPost, h.executePlugin)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (h *Handler) handlePlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, ListResponse{
		Plugins:     []PluginSummary{},
		Unavailable: true,
	})
}

func (h *Handler) installPlugin(w http.ResponseWriter, _ *http.Request) {
	writeError(
		w,
		http.StatusNotImplemented,
		"PLUGIN_INSTALL_UNAVAILABLE",
		"plugin install is not available in the Go backend yet",
	)
}

func (h *Handler) executePlugin(w http.ResponseWriter, r *http.Request) {
	var request ExecuteRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PLUGIN_EXECUTION_REQUEST", "plugin execution request body is invalid")
		return
	}
	result, err := h.service.Execute(r.Context(), request)
	if err != nil {
		writeExecutionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ExecuteResponse{Result: result})
}

func (s *Service) Execute(ctx context.Context, request ExecuteRequest) (any, error) {
	plugin := request.Plugin
	functionDef := request.FunctionDef
	if plugin == nil || functionDef == nil {
		return nil, ErrPluginExecutionUnsupported
	}
	if request.Args == nil {
		return nil, ErrPluginExecutionPayload
	}
	if strings.TrimSpace(plugin.BaseURL) == "" {
		return nil, ErrPluginBaseURLMissing
	}
	if strings.TrimSpace(functionDef.Name) == "" {
		return nil, ErrPluginFunctionMissing
	}
	if !declaresFunction(plugin, functionDef) {
		return nil, ErrPluginFunctionMismatch
	}
	method := strings.ToUpper(strings.TrimSpace(functionDef.Method))
	if !isSupportedMethod(method) {
		return nil, ErrPluginMethodUnsupported
	}
	path := strings.TrimSpace(functionDef.Path)
	if path == "" || strings.ContainsAny(path, "\r\n") {
		return nil, ErrPluginPathInvalid
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	outboundArgs := copyArgs(request.Args)
	consumed := map[string]struct{}{}
	for key, value := range outboundArgs {
		replacement := url.PathEscape(fmt.Sprint(value))
		for _, placeholder := range []string{"{" + key + "}", "{" + strings.ReplaceAll(key, "_", "-") + "}"} {
			if strings.Contains(path, placeholder) {
				path = strings.ReplaceAll(path, placeholder, replacement)
				consumed[key] = struct{}{}
			}
		}
	}
	if strings.Contains(path, "{") || strings.Contains(path, "}") {
		return nil, ErrPluginPathArgsMissing
	}

	pluginURL, err := url.Parse(strings.TrimRight(plugin.BaseURL, "/") + path)
	if err != nil || pluginURL == nil {
		return nil, ErrPluginPathInvalid
	}
	if method == http.MethodGet {
		query := pluginURL.Query()
		for key, value := range outboundArgs {
			if _, ok := consumed[key]; ok {
				continue
			}
			query.Add(key, fmt.Sprint(value))
		}
		pluginURL.RawQuery = query.Encode()
	}
	if err := s.validateOutboundURL(ctx, pluginURL); err != nil {
		return nil, err
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	if plugin.ID == "jina-web-reader" {
		headers.Set("Accept", "application/json")
	}
	authValue, err := s.resolveAuthValue(plugin, request.AuthConfig)
	if err != nil {
		return nil, err
	}
	if requiresAuth(plugin) && plugin.ID != "unsplash" && authValue == "" {
		return nil, ErrPluginAuthRequired
	}
	if authValue != "" {
		if !isSupportedAuthType(authType(plugin, request.AuthConfig)) {
			return nil, ErrPluginAuthUnsupported
		}
		applyAuth(pluginURL, headers, outboundArgs, plugin, request.AuthConfig, authValue, method)
	}

	var body io.Reader
	if method != http.MethodGet {
		encoded, err := json.Marshal(outboundArgs)
		if err != nil {
			return nil, ErrPluginExecutionPayload
		}
		body = bytes.NewReader(encoded)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, method, pluginURL.String(), body)
	if err != nil {
		return nil, ErrPluginPathInvalid
	}
	httpRequest.Header = headers

	response, err := s.do(httpRequest)
	if err != nil {
		if errors.Is(err, ErrPluginURLBlocked) {
			return nil, ErrPluginURLBlocked
		}
		return nil, fmt.Errorf("%w: %v", ErrPluginRequestFailed, err)
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, s.maxResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPluginRequestFailed, err)
	}
	if int64(len(data)) > s.maxResponseBytes {
		return nil, ErrPluginResponseTooLarge
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: status %d", ErrPluginRequestFailed, response.StatusCode)
	}
	var parsed any
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return string(data), nil
	}
	return parsed, nil
}

func (s *Service) do(request *http.Request) (*http.Response, error) {
	client := *s.httpClient
	originalRedirectPolicy := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if originalRedirectPolicy != nil {
			if err := originalRedirectPolicy(req, via); err != nil {
				return err
			}
		}
		return s.validateOutboundURL(req.Context(), req.URL)
	}
	return client.Do(request)
}

func (s *Service) resolveAuthValue(plugin *Plugin, authConfig *PluginAuthConfig) (string, error) {
	if authConfig == nil {
		return "", nil
	}
	if strings.TrimSpace(authConfig.Value) != "" {
		return "", ErrPlaintextPluginAuth
	}
	if authConfig.ValueSecret == nil {
		return "", nil
	}
	value, err := s.secretDecrypter(authConfig.ValueSecret, pluginAuthContext(plugin.ID))
	if err != nil {
		return "", ErrPluginAuthRequired
	}
	return strings.TrimSpace(value), nil
}

func (s *Service) validateOutboundURL(ctx context.Context, parsed *url.URL) error {
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ErrPluginURLBlocked
	}
	host := parsed.Hostname()
	if host == "" || strings.ContainsAny(host, "\r\n") {
		return ErrPluginURLBlocked
	}
	if s.allowPrivateNetwork {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return ErrPluginURLBlocked
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return ErrPluginURLBlocked
	}
	for _, ip := range ips {
		if isBlockedIP(ip.IP) {
			return ErrPluginURLBlocked
		}
	}
	return nil
}

func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

func applyAuth(
	pluginURL *url.URL,
	headers http.Header,
	outboundArgs map[string]any,
	plugin *Plugin,
	authConfig *PluginAuthConfig,
	authValue string,
	method string,
) {
	authType := strings.TrimSpace(authType(plugin, authConfig))
	authName := authName(plugin, authConfig, authType)
	authIn := authLocation(plugin, authConfig)
	switch authType {
	case "bearer", "oauth2":
		headers.Set("Authorization", "Bearer "+authValue)
	case "apiKey":
		switch authIn {
		case "query":
			query := pluginURL.Query()
			query.Set(authName, authValue)
			pluginURL.RawQuery = query.Encode()
		case "header", "":
			headers.Set(authName, authValue)
		default:
			if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
				outboundArgs[authName] = authValue
			} else {
				headers.Set(authName, authValue)
			}
		}
	}
}

func authType(plugin *Plugin, authConfig *PluginAuthConfig) string {
	if plugin.Auth != nil && plugin.Auth.Type != "" && plugin.Auth.Type != "none" {
		return plugin.Auth.Type
	}
	if authConfig != nil {
		return authConfig.Type
	}
	return ""
}

func authName(plugin *Plugin, authConfig *PluginAuthConfig, authType string) string {
	if authConfig != nil && strings.TrimSpace(authConfig.Key) != "" {
		return strings.TrimSpace(authConfig.Key)
	}
	if plugin.Auth != nil && strings.TrimSpace(plugin.Auth.Name) != "" {
		return strings.TrimSpace(plugin.Auth.Name)
	}
	if authType == "apiKey" {
		return "X-API-Key"
	}
	return "Authorization"
}

func authLocation(plugin *Plugin, authConfig *PluginAuthConfig) string {
	if authConfig != nil && strings.TrimSpace(authConfig.AddTo) != "" {
		return authConfig.AddTo
	}
	if plugin.Auth != nil {
		return plugin.Auth.In
	}
	return ""
}

func requiresAuth(plugin *Plugin) bool {
	if plugin.Auth == nil || plugin.Auth.Type == "none" {
		return false
	}
	return plugin.Auth.Required == nil || *plugin.Auth.Required
}

func declaresFunction(plugin *Plugin, functionDef *PluginFunction) bool {
	for _, declared := range plugin.Functions {
		if declared.Name == functionDef.Name && declared.Path == functionDef.Path && strings.EqualFold(declared.Method, functionDef.Method) {
			return true
		}
	}
	return false
}

func isSupportedMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func isSupportedAuthType(value string) bool {
	switch strings.TrimSpace(value) {
	case "", "none", "bearer", "oauth2", "apiKey":
		return true
	default:
		return false
	}
}

func copyArgs(args map[string]any) map[string]any {
	out := make(map[string]any, len(args))
	for key, value := range args {
		out[key] = value
	}
	return out
}

func pluginAuthContext(pluginID string) string {
	return "plugin:" + pluginID + ":auth"
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxExecutionBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func (h *Handler) requireMethod(
	w http.ResponseWriter,
	r *http.Request,
	method string,
	next func(http.ResponseWriter, *http.Request),
) {
	if r.Method != method {
		w.Header().Set("Allow", method)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	next(w, r)
}

func writeExecutionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrPluginExecutionUnsupported):
		writeError(w, http.StatusNotImplemented, "PLUGIN_REGISTRY_REQUIRED", "plugin registry-backed execution is not available for id-only payloads yet")
	case errors.Is(err, ErrPluginBaseURLMissing),
		errors.Is(err, ErrPluginFunctionMissing),
		errors.Is(err, ErrPluginFunctionMismatch),
		errors.Is(err, ErrPluginPathInvalid),
		errors.Is(err, ErrPluginPathArgsMissing),
		errors.Is(err, ErrPluginMethodUnsupported),
		errors.Is(err, ErrPluginExecutionPayload):
		writeError(w, http.StatusBadRequest, "INVALID_PLUGIN_EXECUTION_REQUEST", err.Error())
	case errors.Is(err, ErrPlaintextPluginAuth):
		writeError(w, http.StatusBadRequest, "PLAINTEXT_PLUGIN_AUTH_REJECTED", "plaintext plugin auth is not accepted")
	case errors.Is(err, ErrPluginAuthRequired):
		writeError(w, http.StatusBadRequest, "PLUGIN_AUTH_REQUIRED", "plugin authentication is required")
	case errors.Is(err, ErrPluginAuthUnsupported):
		writeError(w, http.StatusBadRequest, "PLUGIN_AUTH_UNSUPPORTED", "plugin authentication type is not supported")
	case errors.Is(err, ErrPluginURLBlocked):
		writeError(w, http.StatusForbidden, "PLUGIN_URL_BLOCKED", "plugin URL is blocked by policy")
	case errors.Is(err, ErrPluginResponseTooLarge):
		writeError(w, http.StatusBadGateway, "PLUGIN_RESPONSE_TOO_LARGE", "plugin response is too large")
	case errors.Is(err, ErrPluginRequestFailed):
		writeError(w, http.StatusBadGateway, "PLUGIN_REQUEST_FAILED", "plugin request failed")
	default:
		writeError(w, http.StatusInternalServerError, "PLUGIN_EXECUTION_FAILED", "plugin execution failed")
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}
