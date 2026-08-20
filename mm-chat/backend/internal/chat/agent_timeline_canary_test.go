package chat

import (
	"context"
	"testing"

	"neo-chat/mm-chat/backend/internal/auth"
)

func TestAgentTimelineCanaryAdmissionIsExactAndFailClosed(t *testing.T) {
	const canaryID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	handler := NewHandler(NewService(newFakeRepository()),
		WithAgentTimelineCanary(true, []string{canaryID}),
	)
	if !handler.agentTimelineEnabledFor(canaryID) {
		t.Fatal("exact canary was denied")
	}
	if handler.agentTimelineEnabledFor("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb") {
		t.Fatal("non-canary was admitted")
	}
	WithAgentTimelineCanary(false, []string{canaryID})(handler)
	if handler.agentTimelineEnabledFor(canaryID) {
		t.Fatal("disabled global gate admitted canary")
	}
	WithAgentTimelineCanary(true, nil)(handler)
	if handler.agentTimelineEnabledFor(canaryID) {
		t.Fatal("empty canary set admitted user")
	}
}

func TestAgentTimelineGateKeepsDurableAuthorityAndReturnsLegacyProjection(t *testing.T) {
	const canaryID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	presentation := &ProcessStepPresentation{
		Version: 1, Card: "terminal", Command: "printf ok",
	}
	message := Message{
		ID: testMessageID, ConversationID: testConversationID, Role: "assistant",
		Status: "completed", Metadata: map[string]any{
			"fixture": "kept",
			processTraceMetadataKey: []ProcessStep{{
				ID: testMessageID + ":tool:1", Kind: ProcessStepKindTool,
				Status: ProcessStepStatusCompleted, LabelKey: "process.tool",
				Detail: map[string]any{
					"toolName": localTerminalToolName, "mode": "local_direct",
				},
				Presentation: presentation,
			}},
		},
		AgentEvents: []ChatAgentEvent{{EventID: "event-1"}},
	}
	handler := NewHandler(NewService(newFakeRepository()),
		WithAgentTimelineCanary(true, []string{canaryID}),
	)
	nonCanaryDTO := handler.newMessageDTO(context.Background(), message)
	if len(nonCanaryDTO.AgentEvents) != 0 || nonCanaryDTO.Metadata["fixture"] != "kept" {
		t.Fatalf("non-canary DTO=%#v", nonCanaryDTO)
	}
	steps, ok := nonCanaryDTO.Metadata[processTraceMetadataKey].([]map[string]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("legacy process trace=%#v", nonCanaryDTO.Metadata[processTraceMetadataKey])
	}
	if _, exposed := steps[0]["presentation"]; exposed {
		t.Fatalf("non-canary presentation escaped=%#v", steps[0])
	}
	if message.Metadata[processTraceMetadataKey].([]ProcessStep)[0].Presentation == nil ||
		len(message.AgentEvents) != 1 {
		t.Fatal("display gate mutated durable message authority")
	}

	ctx := auth.WithUser(context.Background(), auth.User{ID: canaryID})
	canaryDTO := handler.newMessageDTO(ctx, message)
	if len(canaryDTO.AgentEvents) != 1 {
		t.Fatalf("canary events=%#v", canaryDTO.AgentEvents)
	}
	canarySteps := canaryDTO.Metadata[processTraceMetadataKey].([]ProcessStep)
	if canarySteps[0].Presentation == nil {
		t.Fatal("canary presentation was stripped")
	}
}
