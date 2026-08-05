package chat

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProbeToolCapabilityClassifiesOnlyStructuredCallsAsSupported(t *testing.T) {
	tests := []struct {
		name         string
		provider     *capabilityProbeProvider
		cancel       bool
		wantStatus   ToolCapabilityStatus
		wantCategory string
	}{
		{
			name: "structured call",
			provider: &capabilityProbeProvider{events: []ProviderEvent{{
				Type: ProviderEventToolCallCompleted,
				ToolCall: &ProviderToolCall{
					ID: "probe-1", Name: toolCapabilityProbeToolName, Arguments: `{}`,
				},
			}}},
			wantStatus: ToolCapabilitySupported, wantCategory: "structured_tool_call",
		},
		{
			name: "wrong tool",
			provider: &capabilityProbeProvider{events: []ProviderEvent{{
				Type:     ProviderEventToolCallCompleted,
				ToolCall: &ProviderToolCall{ID: "other", Name: "other_tool", Arguments: `{}`},
			}}},
			wantStatus: ToolCapabilityUnknown, wantCategory: "probe_inconclusive",
		},
		{
			name: "malformed structured arguments",
			provider: &capabilityProbeProvider{events: []ProviderEvent{{
				Type: ProviderEventToolCallCompleted,
				ToolCall: &ProviderToolCall{
					ID: "probe-1", Name: toolCapabilityProbeToolName, Arguments: `{`,
				},
			}}},
			wantStatus: ToolCapabilityUnknown, wantCategory: "probe_inconclusive",
		},
		{
			name: "missing structured call id",
			provider: &capabilityProbeProvider{events: []ProviderEvent{{
				Type: ProviderEventToolCallCompleted,
				ToolCall: &ProviderToolCall{
					Name: toolCapabilityProbeToolName, Arguments: `{}`,
				},
			}}},
			wantStatus: ToolCapabilityUnknown, wantCategory: "probe_inconclusive",
		},
		{
			name: "explicit incompatibility",
			provider: &capabilityProbeProvider{
				startErr: errors.New("tools are not supported by this model"),
			},
			wantStatus: ToolCapabilityUnsupported, wantCategory: "explicit_incompatibility",
		},
		{
			name: "ordinary bad request",
			provider: &capabilityProbeProvider{
				startErr: errors.New("provider status 400"),
			},
			wantStatus: ToolCapabilityUnknown, wantCategory: "transient_transport",
		},
		{
			name: "rate limit",
			provider: &capabilityProbeProvider{
				startErr: errors.New("provider status 429"),
			},
			wantStatus: ToolCapabilityUnknown, wantCategory: "transient_provider",
		},
		{
			name:     "cancelled",
			provider: &capabilityProbeProvider{}, cancel: true,
			wantStatus: ToolCapabilityUnknown, wantCategory: "transient_timeout",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			if test.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			status, category := probeToolCapability(
				ctx,
				test.provider,
				ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
			)
			if status != test.wantStatus || category != test.wantCategory {
				t.Fatalf("probe = %q/%q, want %q/%q", status, category, test.wantStatus, test.wantCategory)
			}
			if len(test.provider.requests) != 1 {
				t.Fatalf("probe requests = %d, want 1", len(test.provider.requests))
			}
			request := test.provider.requests[0]
			if request.ToolChoice != ProviderToolChoiceRequired || len(request.Tools) != 1 ||
				request.Tools[0].Function.Name != toolCapabilityProbeToolName ||
				!request.DisableThinking || request.MaxOutputTokens != 128 ||
				request.Temperature == nil || *request.Temperature != 0 {
				t.Fatalf("probe request = %#v", request)
			}
			serialized := request.Prompt + request.SystemPrompt
			for _, forbidden := range []string{"user question", "knowledge_catalog", "conversation"} {
				if strings.Contains(strings.ToLower(serialized), forbidden) {
					t.Fatalf("probe leaked request data marker %q: %q", forbidden, serialized)
				}
			}
		})
	}
}

func TestResolveToolRoundCapabilityHonorsOverridePrecedence(t *testing.T) {
	provider := &capabilityProbeProvider{}
	cache := &capabilityMemoryCache{lookupStatus: ToolCapabilityUnsupported, lookupFound: true}
	handler := NewHandler(nil, WithToolCapabilityCache(cache))
	model := ModelRef{ProviderID: "fixture", ModelID: "model-a"}
	base := RuntimeProviderResolution{
		ToolCapabilityPolicy:         toolCapabilityPolicyDisabled,
		ToolCapabilityModelOverrides: map[string]string{"model-a": toolCapabilityPolicyEnabled},
		ToolCapabilityConfigHash:     strings.Repeat("a", 64),
	}
	if got := handler.resolveToolRoundCapability(context.Background(), provider, base, model); got != ToolCapabilitySupported {
		t.Fatalf("model override status = %q, want supported", got)
	}
	base.ToolCapabilityModelOverrides = nil
	if got := handler.resolveToolRoundCapability(context.Background(), provider, base, model); got != ToolCapabilityUnsupported {
		t.Fatalf("provider override status = %q, want unsupported", got)
	}
	base.ToolCapabilityPolicy = toolCapabilityPolicyAuto
	if got := handler.resolveToolRoundCapability(context.Background(), provider, base, model); got != ToolCapabilityUnsupported {
		t.Fatalf("cache status = %q, want unsupported", got)
	}
	if cache.lookupCalls != 1 {
		t.Fatalf("cache lookups = %d, want 1", cache.lookupCalls)
	}
}

func TestToolCapabilityUnknownUsesNonBlockingSingleflightProbe(t *testing.T) {
	release := make(chan struct{})
	provider := &capabilityProbeProvider{
		release: release,
		events: []ProviderEvent{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID: "probe", Name: toolCapabilityProbeToolName, Arguments: `{}`,
			},
		}},
	}
	cache := &capabilityMemoryCache{stored: make(chan capabilityStoredValue, 1)}
	handler := NewHandler(nil, WithToolCapabilityCache(cache))
	resolution := RuntimeProviderResolution{
		ToolCapabilityPolicy:     toolCapabilityPolicyAuto,
		ToolCapabilityConfigHash: strings.Repeat("b", 64),
	}
	model := ModelRef{ProviderID: "fixture", ModelID: "model-a"}

	started := time.Now()
	for range 2 {
		if got := handler.resolveToolRoundCapability(context.Background(), provider, resolution, model); got != ToolCapabilityUnknown {
			t.Fatalf("unknown status = %q", got)
		}
	}
	if time.Since(started) > 100*time.Millisecond {
		t.Fatal("unknown capability blocked the current chat request")
	}
	deadline := time.Now().Add(time.Second)
	for provider.callCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if provider.callCount() != 1 {
		t.Fatalf("probe calls = %d, want singleflight 1", provider.callCount())
	}
	close(release)
	select {
	case stored := <-cache.stored:
		if stored.status != ToolCapabilitySupported || stored.category != "structured_tool_call" {
			t.Fatalf("stored probe = %#v", stored)
		}
	case <-time.After(time.Second):
		t.Fatal("probe result was not stored")
	}
}

type capabilityProbeProvider struct {
	mu       sync.Mutex
	requests []ProviderRoundRequest
	startErr error
	events   []ProviderEvent
	release  <-chan struct{}
}

func (p *capabilityProbeProvider) StreamChat(
	context.Context,
	ProviderRequest,
) (<-chan ProviderEvent, error) {
	events := make(chan ProviderEvent)
	close(events)
	return events, nil
}

func (p *capabilityProbeProvider) StreamToolRound(
	ctx context.Context,
	request ProviderRoundRequest,
) (<-chan ProviderEvent, error) {
	p.mu.Lock()
	p.requests = append(p.requests, request)
	p.mu.Unlock()
	if p.startErr != nil {
		return nil, p.startErr
	}
	events := make(chan ProviderEvent)
	go func() {
		defer close(events)
		if p.release != nil {
			select {
			case <-ctx.Done():
				return
			case <-p.release:
			}
		}
		for _, event := range p.events {
			select {
			case <-ctx.Done():
				return
			case events <- event:
			}
		}
	}()
	return events, nil
}

func (p *capabilityProbeProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.requests)
}

type capabilityStoredValue struct {
	status   ToolCapabilityStatus
	category string
}

type capabilityMemoryCache struct {
	mu           sync.Mutex
	lookupStatus ToolCapabilityStatus
	lookupFound  bool
	lookupErr    error
	lookupCalls  int
	stored       chan capabilityStoredValue
}

func (c *capabilityMemoryCache) LookupToolCapability(
	context.Context,
	string,
	string,
) (ToolCapabilityStatus, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lookupCalls++
	return c.lookupStatus, c.lookupFound, c.lookupErr
}

func (c *capabilityMemoryCache) StoreToolCapability(
	_ context.Context,
	_ string,
	_ string,
	status ToolCapabilityStatus,
	category string,
) error {
	if c.stored != nil {
		c.stored <- capabilityStoredValue{status: status, category: category}
	}
	return nil
}
