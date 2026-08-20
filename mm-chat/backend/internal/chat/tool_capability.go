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
	running map[string]*toolCapabilityProbeState
}

type toolCapabilityProbeState struct {
	done   chan struct{}
	status ToolCapabilityStatus
}

func newToolCapabilityProbeGroup() *toolCapabilityProbeGroup {
	return &toolCapabilityProbeGroup{
		running: map[string]*toolCapabilityProbeState{},
	}
}

func (h *Handler) resolveToolRoundCapability(
	ctx context.Context,
	provider Provider,
	resolution RuntimeProviderResolution,
	modelRef ModelRef,
) ToolCapabilityStatus {
	status, _ := h.resolveToolRoundCapabilityWithProbe(
		ctx,
		provider,
		resolution,
		modelRef,
	)
	return status
}

func (h *Handler) resolveToolRoundCapabilityForMode(
	ctx context.Context,
	provider Provider,
	resolution RuntimeProviderResolution,
	modelRef ModelRef,
	requestedMode chatToolMode,
) ToolCapabilityStatus {
	status, probe := h.resolveToolRoundCapabilityWithProbe(
		ctx,
		provider,
		resolution,
		modelRef,
	)
	if requestedMode != chatToolModeAgent || status != ToolCapabilityUnknown {
		return status
	}
	if probe != nil {
		select {
		case <-probe.done:
			status = probe.status
		case <-ctx.Done():
			return ToolCapabilitySupported
		}
	}
	if status == ToolCapabilityUnsupported {
		return ToolCapabilityUnsupported
	}
	// Auto/unknown is not proof that the model lacks Tools. The Provider
	// adapter already implements the native Tool round, so preserve an explicit
	// Agent request and let the existing first-round incompatibility path make a
	// confirmed downgrade. This also covers a cached transient-unknown result
	// without bypassing its five-minute probe backoff.
	return ToolCapabilitySupported
}

func (h *Handler) resolveToolRoundCapabilityWithProbe(
	ctx context.Context,
	provider Provider,
	resolution RuntimeProviderResolution,
	modelRef ModelRef,
) (ToolCapabilityStatus, *toolCapabilityProbeState) {
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
			return ToolCapabilitySupported, nil
		}
		return ToolCapabilityUnsupported, nil
	}
	if policy == toolCapabilityPolicyDisabled || !adapterCapable {
		return ToolCapabilityUnsupported, nil
	}
	if policy == toolCapabilityPolicyEnabled {
		return ToolCapabilitySupported, nil
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
				return status, nil
			case ToolCapabilityUnknown:
				return ToolCapabilityUnknown, nil
			}
		}
	}
	probe := h.startToolCapabilityProbe(toolProvider, configHash, modelRef)
	return ToolCapabilityUnknown, probe
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
) *toolCapabilityProbeState {
	if h == nil || h.toolCapabilityProbes == nil || provider == nil ||
		h.toolCapabilityCache == nil || strings.TrimSpace(configHash) == "" ||
		strings.TrimSpace(modelRef.ModelID) == "" {
		return nil
	}
	key := strings.TrimSpace(configHash) + "\x00" + strings.TrimSpace(modelRef.ModelID)
	group := h.toolCapabilityProbes
	group.mu.Lock()
	if state, running := group.running[key]; running {
		group.mu.Unlock()
		return state
	}
	state := &toolCapabilityProbeState{done: make(chan struct{})}
	group.running[key] = state
	group.mu.Unlock()

	go func() {
		defer func() {
			group.mu.Lock()
			if group.running[key] == state {
				delete(group.running, key)
			}
			group.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), toolCapabilityProbeTimeout)
		status, category := probeToolCapability(ctx, provider, modelRef)
		cancel()
		// Publish the probe result before writing the cache. Agent requests wait
		// for provider capability evidence, not for a best-effort persistence
		// round trip that may take up to toolCapabilityCacheWriteTimeout.
		state.status = status
		close(state.done)
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
	return state
}

func probeToolCapability(
	ctx context.Context,
	provider ToolRoundProvider,
	modelRef ModelRef,
) (ToolCapabilityStatus, string) {
	temperature := 0.0
	events, err := provider.StreamToolRound(ctx, ProviderRoundRequest{
		ProviderRequest: ProviderRequest{
			Prompt:          "Call the provided fictional capability probe tool exactly once.",
			SystemPrompt:    "This is a fixed protocol capability probe. Do not answer with prose.",
			DisableThinking: true,
			MaxOutputTokens: 128,
			Temperature:     &temperature,
			ModelRef:        modelRef,
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
