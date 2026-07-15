package plugins

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"neo-chat/mm-chat/backend/internal/auth"
)

const (
	AuditActionInstall = "plugin.install"
	AuditActionExecute = "plugin.execute"

	AuditStatusAdmitted = "admitted"

	auditSourcePluginPayload     = "plugin_payload"
	auditSourceCustomOpenAPIJSON = "custom_openapi_json"
	auditSourceCustomManifestURL = "custom_manifest_url"
	auditSourceManifestURL       = "manifest_url"
	auditSourceRegistryID        = "registry_id"
	auditSourceFullPayloadCompat = "full_payload_compat"
)

var ErrPluginAuditUnavailable = errors.New("plugin audit recorder is unavailable")

type AuditEvent struct {
	Action        string `json:"action"`
	Status        string `json:"status"`
	UserID        string `json:"userId,omitempty"`
	PluginID      string `json:"pluginId,omitempty"`
	FunctionName  string `json:"functionName,omitempty"`
	FunctionCount int    `json:"functionCount,omitempty"`
	Source        string `json:"source,omitempty"`
	BuiltIn       bool   `json:"builtIn,omitempty"`
	HasAuth       bool   `json:"hasAuth,omitempty"`
	CallID        string `json:"callId,omitempty"`
	ArgumentCount int    `json:"argumentCount,omitempty"`
	BaseHost      string `json:"baseHost,omitempty"`
	ManifestHost  string `json:"manifestHost,omitempty"`
	RequestID     string `json:"requestId,omitempty"`
	UserAgent     string `json:"userAgent,omitempty"`
	IPAddress     string `json:"ipAddress,omitempty"`
}

type AuditRecorder interface {
	RecordPluginEvent(context.Context, AuditEvent) error
}

type AuditRecorderFunc func(context.Context, AuditEvent) error

func (fn AuditRecorderFunc) RecordPluginEvent(ctx context.Context, event AuditEvent) error {
	if fn == nil {
		return nil
	}
	return fn(ctx, event)
}

type requestMetadataContextKey struct{}

type requestMetadata struct {
	RequestID string
	UserAgent string
	IPAddress string
}

func withRequestMetadata(ctx context.Context, metadata requestMetadata) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, requestMetadataContextKey{}, metadata)
}

func requestMetadataFromContext(ctx context.Context) requestMetadata {
	if ctx == nil {
		return requestMetadata{}
	}
	metadata, _ := ctx.Value(requestMetadataContextKey{}).(requestMetadata)
	return metadata
}

func requestMetadataFromHTTP(r *http.Request) requestMetadata {
	if r == nil {
		return requestMetadata{}
	}
	return requestMetadata{
		RequestID: cleanAuditString(r.Header.Get("X-Request-Id"), 128),
		UserAgent: cleanAuditString(r.UserAgent(), 512),
		IPAddress: cleanAuditIP(r.RemoteAddr),
	}
}

func RecordPluginAudit(ctx context.Context, recorder AuditRecorder, event AuditEvent) error {
	if recorder == nil {
		return nil
	}
	event = NormalizeAuditEvent(ctx, event)
	if err := recorder.RecordPluginEvent(ctx, event); err != nil {
		return fmt.Errorf("%w: %v", ErrPluginAuditUnavailable, err)
	}
	return nil
}

func NormalizeAuditEvent(ctx context.Context, event AuditEvent) AuditEvent {
	user := auth.UserOrDevelopment(ctx)
	metadata := requestMetadataFromContext(ctx)
	if event.UserID == "" {
		event.UserID = user.ID
	}
	if event.RequestID == "" {
		event.RequestID = metadata.RequestID
	}
	if event.UserAgent == "" {
		event.UserAgent = metadata.UserAgent
	}
	if event.IPAddress == "" {
		event.IPAddress = metadata.IPAddress
	}
	event.Action = cleanAuditString(event.Action, 64)
	event.Status = cleanAuditString(event.Status, 32)
	event.UserID = cleanAuditString(event.UserID, 64)
	event.PluginID = cleanAuditString(event.PluginID, 160)
	event.FunctionName = cleanAuditString(event.FunctionName, 160)
	event.Source = cleanAuditString(event.Source, 64)
	event.CallID = cleanAuditString(event.CallID, 160)
	event.BaseHost = cleanAuditHost(event.BaseHost)
	event.ManifestHost = cleanAuditHost(event.ManifestHost)
	event.RequestID = cleanAuditString(event.RequestID, 128)
	event.UserAgent = cleanAuditString(event.UserAgent, 512)
	event.IPAddress = cleanAuditIP(event.IPAddress)
	if event.Status == "" {
		event.Status = AuditStatusAdmitted
	}
	if event.FunctionCount < 0 {
		event.FunctionCount = 0
	}
	if event.ArgumentCount < 0 {
		event.ArgumentCount = 0
	}
	return event
}

func auditInstallEvent(plugin Plugin, source string) AuditEvent {
	return AuditEvent{
		Action:        AuditActionInstall,
		Status:        AuditStatusAdmitted,
		PluginID:      plugin.ID,
		FunctionCount: len(plugin.Functions),
		Source:        source,
		BuiltIn:       plugin.BuiltIn,
		HasAuth:       plugin.Auth != nil && plugin.Auth.Type != "" && plugin.Auth.Type != "none",
		BaseHost:      hostOnly(plugin.BaseURL),
		ManifestHost:  hostOnly(plugin.ManifestURL),
	}
}

func auditExecuteEvent(plugin *Plugin, functionDef *PluginFunction, request ExecuteRequest, source string) AuditEvent {
	event := AuditEvent{
		Action:        AuditActionExecute,
		Status:        AuditStatusAdmitted,
		Source:        source,
		CallID:        request.CallID,
		ArgumentCount: len(request.Args),
	}
	if plugin != nil {
		event.PluginID = plugin.ID
		event.FunctionCount = len(plugin.Functions)
		event.BuiltIn = plugin.BuiltIn
		event.HasAuth = (plugin.Auth != nil && plugin.Auth.Type != "" && plugin.Auth.Type != "none") ||
			(request.AuthConfig != nil && (request.AuthConfig.Type != "" || request.AuthConfig.ValueSecret != nil))
		event.BaseHost = hostOnly(plugin.BaseURL)
		event.ManifestHost = hostOnly(plugin.ManifestURL)
	}
	if functionDef != nil {
		event.FunctionName = functionDef.Name
	}
	return event
}

func executionAuditSource(request ExecuteRequest) string {
	if request.Plugin == nil || request.FunctionDef == nil {
		return auditSourceRegistryID
	}
	return auditSourceFullPayloadCompat
}

func installAuditSourceFromCustomInput(input string) string {
	if isManifestURL(input) {
		return auditSourceCustomManifestURL
	}
	return auditSourceCustomOpenAPIJSON
}

func hostOnly(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil {
		return ""
	}
	return parsed.Hostname()
}

func cleanAuditString(value string, maxLen int) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		switch {
		case r == '\r' || r == '\n' || r == '\t':
			return ' '
		case r < 0x20 || r == 0x7f:
			return -1
		default:
			return r
		}
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	if maxLen > 0 && len(value) > maxLen {
		return value[:maxLen]
	}
	return value
}

func cleanAuditHost(value string) string {
	value = strings.ToLower(cleanAuditString(value, 255))
	if strings.ContainsAny(value, "/?#@:") {
		return ""
	}
	return value
}

func cleanAuditIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	ip := net.ParseIP(value)
	if ip == nil {
		return ""
	}
	return ip.String()
}
