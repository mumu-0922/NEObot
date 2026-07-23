package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"neo-chat/mm-chat/backend/internal/runtimeconfig"
)

type ToolCapabilityStatus string

const (
	ToolCapabilitySupported   ToolCapabilityStatus = "supported"
	ToolCapabilityUnsupported ToolCapabilityStatus = "unsupported"
	ToolCapabilityUnknown     ToolCapabilityStatus = "unknown"

	toolCapabilityPolicyAuto        = "auto"
	toolCapabilityPolicyEnabled     = "enabled"
	toolCapabilityPolicyDisabled    = "disabled"
	toolCapabilityProbeTimeout      = 20 * time.Second
	toolCapabilityCacheWriteTimeout = 5 * time.Second
	toolCapabilityProbeToolName     = "neo_chat_capability_probe"
)

type ToolCapabilityCache interface {
	LookupToolCapability(
		context.Context,
		string,
		string,
	) (ToolCapabilityStatus, bool, error)
	StoreToolCapability(
		context.Context,
		string,
		string,
		ToolCapabilityStatus,
		string,
	) error
}

type toolCapabilityProbeGroup struct {
	mu      sync.Mutex
	running map[string]struct{}
}

func newToolCapabilityProbeGroup() *toolCapabilityProbeGroup {
	return &toolCapabilityProbeGroup{running: map[string]struct{}{}}
}

func (h *Handler) resolveToolRoundCapability(
	ctx context.Context,
	provider Provider,
	resolution RuntimeProviderResolution,
	modelRef ModelRef,
) ToolCapabilityStatus {
	toolProvider, adapterCapable := provider.(ToolRoundProvider)
	policy := strings.ToLower(strings.TrimSpace(resolution.ToolCapabilityPolicy))
	if override := strings.ToLower(strings.TrimSpace(
		resolution.ToolCapabilityModelOverrides[strings.TrimSpace(modelRef.ModelID)],
	)); override != "" {
		policy = override
	}
	if policy == "" {
		// In-process providers and legacy tests have no persisted provider
		// identity. Preserve their established adapter capability behavior.
		if adapterCapable {
			return ToolCapabilitySupported
		}
		return ToolCapabilityUnsupported
	}
	if policy == toolCapabilityPolicyDisabled || !adapterCapable {
		return ToolCapabilityUnsupported
	}
	if policy == toolCapabilityPolicyEnabled {
		return ToolCapabilitySupported
	}

	configHash := strings.TrimSpace(resolution.ToolCapabilityConfigHash)
	modelID := strings.TrimSpace(modelRef.ModelID)
	if h.toolCapabilityCache != nil && configHash != "" && modelID != "" {
		status, found, err := h.toolCapabilityCache.LookupToolCapability(
			ctx,
			configHash,
			modelID,
		)
		if err == nil && found {
			switch status {
			case ToolCapabilitySupported, ToolCapabilityUnsupported:
				return status
			case ToolCapabilityUnknown:
				return ToolCapabilityUnknown
			}
		}
	}
	h.startToolCapabilityProbe(toolProvider, configHash, modelRef)
	return ToolCapabilityUnknown
}

// PrewarmToolCapabilities resolves a server-owned provider and starts bounded
// synthetic probes for its default/task models. The caller is expected to run
// this off the provider-save request path.
func (h *Handler) PrewarmToolCapabilities(
	ctx context.Context,
	request runtimeconfig.ToolCapabilityWarmupRequest,
) {
	if h == nil || h.providerResolver == nil || len(request.ModelIDs) == 0 {
		return
	}
	resolveCtx, cancel := context.WithTimeout(ctx, toolCapabilityProbeTimeout)
	defer cancel()
	resolution, err := h.providerResolver.ResolveRuntimeProvider(
		resolveCtx,
		request.Provider,
	)
	if err != nil || resolution.Provider == nil {
		return
	}
	providerID := strings.TrimSpace(request.Provider.ID)
	for _, modelID := range request.ModelIDs {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}
		h.resolveToolRoundCapability(
			resolveCtx,
			resolution.Provider,
			resolution,
			ModelRef{ProviderID: providerID, ModelID: modelID},
		)
	}
}

func (h *Handler) startToolCapabilityProbe(
	provider ToolRoundProvider,
	configHash string,
	modelRef ModelRef,
) {
	if h == nil || h.toolCapabilityProbes == nil || provider == nil ||
		h.toolCapabilityCache == nil || strings.TrimSpace(configHash) == "" ||
		strings.TrimSpace(modelRef.ModelID) == "" {
		return
	}
	key := strings.TrimSpace(configHash) + "\x00" + strings.TrimSpace(modelRef.ModelID)
	group := h.toolCapabilityProbes
	group.mu.Lock()
	if _, running := group.running[key]; running {
		group.mu.Unlock()
		return
	}
	group.running[key] = struct{}{}
	group.mu.Unlock()

	go func() {
		defer func() {
			group.mu.Lock()
			delete(group.running, key)
			group.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), toolCapabilityProbeTimeout)
		status, category := probeToolCapability(ctx, provider, modelRef)
		cancel()
		storeCtx, storeCancel := context.WithTimeout(
			context.Background(),
			toolCapabilityCacheWriteTimeout,
		)
		defer storeCancel()
		_ = h.toolCapabilityCache.StoreToolCapability(
			storeCtx,
			configHash,
			modelRef.ModelID,
			status,
			category,
		)
	}()
}

func probeToolCapability(
	ctx context.Context,
	provider ToolRoundProvider,
	modelRef ModelRef,
) (ToolCapabilityStatus, string) {
	events, err := provider.StreamToolRound(ctx, ProviderRoundRequest{
		ProviderRequest: ProviderRequest{
			Prompt:       "Call the provided fictional capability probe tool exactly once.",
			SystemPrompt: "This is a fixed protocol capability probe. Do not answer with prose.",
			ModelRef:     modelRef,
		},
		Tools: []ToolDefinition{{
			Type: "function",
			Function: ToolFunctionDefinition{
				Name:        toolCapabilityProbeToolName,
				Description: "A fictional no-op used only to verify Tool Call protocol support.",
				Parameters: map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties":           map[string]any{},
				},
			},
		}},
		ToolChoice: ProviderToolChoiceRequired,
	})
	if err != nil {
		if isExplicitToolIncompatibility(err) {
			return ToolCapabilityUnsupported, "explicit_incompatibility"
		}
		return ToolCapabilityUnknown, toolCapabilityTransientCategory(err)
	}
	structuredToolCall := false
	for event := range events {
		if event.Error != nil {
			if isExplicitToolIncompatibility(event.Error) {
				return ToolCapabilityUnsupported, "explicit_incompatibility"
			}
			return ToolCapabilityUnknown, toolCapabilityTransientCategory(event.Error)
		}
		if event.Type == ProviderEventToolCallCompleted &&
			validToolCapabilityProbeCall(event.ToolCall) {
			structuredToolCall = true
		}
	}
	if structuredToolCall {
		return ToolCapabilitySupported, "structured_tool_call"
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) {
		return ToolCapabilityUnknown, "transient_timeout"
	}
	return ToolCapabilityUnknown, "probe_inconclusive"
}

func validToolCapabilityProbeCall(call *ProviderToolCall) bool {
	if call == nil || strings.TrimSpace(call.ID) == "" ||
		call.FailureCategory != "" ||
		normalizedToolName(call.Name) != toolCapabilityProbeToolName {
		return false
	}
	var arguments map[string]any
	return json.Unmarshal([]byte(strings.TrimSpace(call.Arguments)), &arguments) == nil &&
		arguments != nil
}

func isExplicitToolIncompatibility(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	mentionsTool := strings.Contains(message, "tool") ||
		strings.Contains(message, "function call") ||
		strings.Contains(message, "function_call")
	mentionsUnsupported := strings.Contains(message, "unsupported") ||
		strings.Contains(message, "not support") ||
		strings.Contains(message, "unknown field") ||
		strings.Contains(message, "unrecognized field")
	return mentionsTool && mentionsUnsupported
}

func toolCapabilityTransientCategory(err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "transient_timeout"
	}
	message := strings.ToLower(err.Error())
	for _, status := range []string{"status 429", "status 500", "status 502", "status 503", "status 504"} {
		if strings.Contains(message, status) {
			return "transient_provider"
		}
	}
	return "transient_transport"
}
