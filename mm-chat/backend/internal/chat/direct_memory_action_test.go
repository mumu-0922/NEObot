package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestDetectDirectMemoryActionIntentIsExplicit(t *testing.T) {
	tests := []struct {
		value string
		want  string
		ok    bool
	}{
		{"请记住我喜欢简短回答", "remember", true},
		{"Please remember that I prefer concise replies", "remember", true},
		{"更正记忆：我喜欢详细回答", "correct", true},
		{"Correct the memory about my reply style", "correct", true},
		{"帮我忘记刚才我说的", "forget", true},
		{"Delete that memory", "forget", true},
		{"那你写进去呀", "remember", true},
		{"把刚才那条记住", "remember", true},
		{"保存刚才我说的", "remember", true},
		{"记住", "remember", true},
		{"记住它", "remember", true},
		{"记下来", "remember", true},
		{"记一下", "remember", true},
		{"save that", "remember", true},
		{"remember", "remember", true},
		{"remember it", "remember", true},
		{"put that in memory", "remember", true},
		{"你记得我喜欢什么吗", "", false},
		{"保存", "", false},
		{"写进去", "", false},
		{"我忘记带钥匙了", "", false},
		{"I forgot my password", "", false},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			got, ok := detectDirectMemoryActionIntent(test.value)
			if got != test.want || ok != test.ok {
				t.Fatalf("intent = %q/%t, want %q/%t", got, ok, test.want, test.ok)
			}
		})
	}
}

func TestDirectMemoryActionIntentDistinguishesReferencedRemember(t *testing.T) {
	for _, command := range []string{
		"那你写进去呀",
		"记住",
		"记住这条",
		"记下来",
		"记一下",
		"remember it",
	} {
		referenced, ok := detectDirectMemoryActionIntentDetail(command)
		if !ok || referenced.action != "remember" ||
			!referenced.referencesPreviousUserMessage {
			t.Fatalf("referenced intent for %q = %#v/%t", command, referenced, ok)
		}
	}
	complete, ok := detectDirectMemoryActionIntentDetail("请记住我喜欢生椰拿铁")
	if !ok || complete.action != "remember" ||
		complete.referencesPreviousUserMessage {
		t.Fatalf("complete intent = %#v/%t", complete, ok)
	}
}

func TestReferencedPreviousUserMessageSelectsNearestCompletedUser(t *testing.T) {
	const conversationID = "20000000-0000-4000-8000-000000000001"
	current := Message{
		ID: "30000000-0000-4000-8000-000000000007", ConversationID: conversationID,
		UserID: "10000000-0000-4000-8000-000000000001",
		Role:   "user", Status: "completed", Content: "那你写进去呀",
	}
	messages := []Message{
		{
			ID: "30000000-0000-4000-8000-000000000001", ConversationID: conversationID,
			UserID: "10000000-0000-4000-8000-000000000001",
			Role:   "user", Status: "completed", Content: "旧事实",
		},
		{
			ID: "30000000-0000-4000-8000-000000000002", ConversationID: conversationID,
			Role: "assistant", Status: "completed", Content: "不要引用助手内容",
		},
		{
			ID: "30000000-0000-4000-8000-000000000003", ConversationID: conversationID,
			UserID: "10000000-0000-4000-8000-000000000001",
			Role:   "user", Status: "completed", Content: "我喜欢喝生椰拿铁",
		},
		{
			ID: "30000000-0000-4000-8000-000000000004", ConversationID: conversationID,
			UserID: "10000000-0000-4000-8000-000000000001",
			Role:   "user", Status: "streaming", Content: "未完成内容",
		},
		{
			ID: "30000000-0000-4000-8000-000000000005", ConversationID: conversationID,
			UserID: "10000000-0000-4000-8000-000000000099",
			Role:   "user", Status: "completed", Content: "其他用户内容",
		},
		{
			ID:             "30000000-0000-4000-8000-000000000006",
			ConversationID: "20000000-0000-4000-8000-000000000099",
			UserID:         "10000000-0000-4000-8000-000000000001",
			Role:           "user", Status: "completed", Content: "其他会话内容",
		},
		current,
	}
	got, ok := referencedPreviousUserMessage(messages, current)
	if !ok || got.Content != "我喜欢喝生椰拿铁" {
		t.Fatalf("referenced message = %#v/%t", got, ok)
	}
}

func TestPlanDirectMemoryActionStrictJSON(t *testing.T) {
	valid := `{"action":"remember",` +
		`"memoryType":"preference","content":"I prefer concise replies",` +
		`"importance":5,"tags":["reply"],"sensitivity":"normal",` +
		`"scopeType":"global","confidence":0.99,"targets":[]}`
	tests := []struct {
		name string
		body string
	}{
		{"missing", strings.Replace(valid, `,"targets":[]`, "", 1)},
		{"unknown", strings.Replace(valid, `"targets":[]`, `"targets":[],"extra":true`, 1)},
		{"model supplied schema", strings.Replace(
			valid,
			`"action":"remember"`,
			`"schemaVersion":"neo-chat.memory-user-action.v1","action":"remember"`,
			1,
		)},
		{"duplicate", strings.Replace(valid, `"action":"remember"`, `"action":"remember","action":"remember"`, 1)},
		{"trailing", valid + `{}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &directActionProvider{body: test.body}
			_, err := planDirectMemoryAction(
				context.Background(), provider, ModelRef{ModelID: "fixture"},
				"remember", "remember this", "", usermemory.DirectActionContext{},
			)
			if err == nil {
				t.Fatal("malformed planner output unexpectedly accepted")
			}
		})
	}
	provider := &directActionProvider{body: valid}
	plan, err := planDirectMemoryAction(
		context.Background(), provider, ModelRef{ModelID: "fixture"},
		"remember", "remember this", "", usermemory.DirectActionContext{},
	)
	if err != nil || plan.Content == nil || *plan.Content != "I prefer concise replies" {
		t.Fatalf("valid plan = %#v/%v", plan, err)
	}
	if plan.SchemaVersion != directMemoryActionSchemaVersion ||
		provider.lastRound.ToolChoice != ProviderToolChoiceRequired ||
		len(provider.lastRound.Tools) != 1 ||
		provider.lastRound.Tools[0].Function.Name != directMemoryActionToolName ||
		!provider.lastRound.Tools[0].Function.Strict ||
		!provider.lastRound.DisableThinking ||
		provider.lastRound.MaxOutputTokens != directMemoryActionOutputTokens ||
		provider.lastRound.Temperature == nil || *provider.lastRound.Temperature != 0 {
		t.Fatalf("versioned Tool request/plan = %#v/%#v", provider.lastRound, plan)
	}
	properties, _ := provider.lastRound.Tools[0].Function.Parameters["properties"].(map[string]any)
	if _, modelControlled := properties["schemaVersion"]; modelControlled {
		t.Fatalf("schema version was delegated to model: %#v", properties)
	}
}

func TestPlanDirectMemoryActionRejectsNonToolPlannerResponses(t *testing.T) {
	validCall := ProviderEvent{
		Type: ProviderEventToolCallCompleted,
		ToolCall: &ProviderToolCall{
			Name: directMemoryActionToolName,
			Arguments: `{"action":"remember","memoryType":"fact",` +
				`"content":"fixture","importance":3,"tags":[],` +
				`"sensitivity":"normal","scopeType":"global",` +
				`"confidence":0.99,"targets":[]}`,
		},
	}
	tests := []struct {
		name   string
		events []ProviderEvent
	}{
		{name: "no call"},
		{name: "ordinary text", events: []ProviderEvent{{
			Type: ProviderEventDelta, Delta: "not a Tool call",
		}}},
		{name: "wrong Tool", events: []ProviderEvent{{
			Type:     ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{Name: "other", Arguments: `{}`},
		}}},
		{name: "multiple Tools", events: []ProviderEvent{validCall, validCall}},
		{name: "failed Tool framing", events: []ProviderEvent{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				Name: directMemoryActionToolName, Arguments: `{}`,
				FailureCategory: "tool_arguments_invalid",
			},
		}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &directActionScriptProvider{events: test.events}
			_, err := planDirectMemoryAction(
				context.Background(), provider, ModelRef{ModelID: "fixture"},
				"remember", "remember fixture", "", usermemory.DirectActionContext{},
			)
			if !errors.Is(err, errDirectMemoryPlannerOutputInvalid) {
				t.Fatalf("error = %v, want planner output invalid", err)
			}
		})
	}
}

func TestPrepareDirectMemoryActionClassifiesToolTransportFailure(t *testing.T) {
	repo := newDirectActionMemoryRepository()
	provider := &directActionScriptProvider{roundErr: errors.New("fixture transport failure")}
	handler := NewHandler(nil, WithUserMemoryService(usermemory.NewService(repo)))
	preparation := handler.prepareDirectMemoryAction(
		context.Background(),
		"20000000-0000-4000-8000-000000000001",
		Message{
			ID:   "30000000-0000-4000-8000-000000000001",
			Role: "user", Status: "completed", Content: "请记住我喜欢生椰拿铁",
		},
		Message{ID: "30000000-0000-4000-8000-000000000002"},
		nil, provider, ModelRef{ModelID: "fixture"},
	)
	if preparation.Result == nil || preparation.Result.Status != "failed" ||
		preparation.Result.ResultCode != "PLANNER_PROVIDER_FAILED" ||
		repo.applied.PreflightResultCode != "PLANNER_PROVIDER_FAILED" {
		t.Fatalf("transport classification = %#v/%#v", preparation, repo.applied)
	}
}

func TestPrepareDirectMemoryActionRejectsSecretWithoutProviderEgress(t *testing.T) {
	repo := newDirectActionMemoryRepository()
	provider := &directActionProvider{body: "provider must not run"}
	handler := NewHandler(
		nil,
		WithUserMemoryService(usermemory.NewService(repo)),
	)
	preparation := handler.prepareDirectMemoryAction(
		context.Background(),
		"20000000-0000-4000-8000-000000000001",
		Message{
			ID:   "30000000-0000-4000-8000-000000000001",
			Role: "user", Status: "completed",
			Content: "请记住 password: super-secret-value",
		},
		Message{ID: "30000000-0000-4000-8000-000000000002"},
		nil,
		provider,
		ModelRef{ModelID: "fixture"},
	)
	if provider.calls != 0 {
		t.Fatalf("secret reached provider %d times", provider.calls)
	}
	if preparation.Result == nil || preparation.Result.Status != "rejected" {
		t.Fatalf("preparation = %#v", preparation)
	}
	if repo.applied.PreflightResultCode != "SECRET_REJECTED" ||
		repo.applied.Content != "" || repo.applied.NormalizedContent != "" {
		t.Fatalf("secret apply input = %#v", repo.applied)
	}
}

func TestPrepareDirectMemoryActionReferencedRememberUsesPreviousUserMessage(t *testing.T) {
	const (
		conversationID = "20000000-0000-4000-8000-000000000001"
		currentID      = "30000000-0000-4000-8000-000000000003"
	)
	previous := Message{
		ID: "30000000-0000-4000-8000-000000000001", ConversationID: conversationID,
		Role: "user", Status: "completed",
		Content: "我的家庭住址：银河大道88号。我喜欢喝生椰拿铁。",
	}
	current := Message{
		ID: currentID, ConversationID: conversationID,
		Role: "user", Status: "completed", Content: "那你写进去呀",
	}
	messages := []Message{
		previous,
		{
			ID: "30000000-0000-4000-8000-000000000002", ConversationID: conversationID,
			Role: "assistant", Status: "completed", Content: "助手猜测用户喜欢美式咖啡",
		},
		current,
	}
	repo := newDirectActionMemoryRepository()
	provider := &directActionProvider{body: `{"action":"remember",` +
		`"memoryType":"preference","content":"我喜欢喝生椰拿铁",` +
		`"importance":5,"tags":["饮品"],"sensitivity":"normal",` +
		`"scopeType":"global","confidence":0.99,"targets":[]}`}
	handler := NewHandler(nil, WithUserMemoryService(usermemory.NewService(repo)))
	preparation := handler.prepareDirectMemoryAction(
		context.Background(), conversationID, current,
		Message{ID: "30000000-0000-4000-8000-000000000004"},
		messages, provider, ModelRef{ModelID: "fixture"},
	)
	if preparation.Result == nil || provider.calls != 1 {
		t.Fatalf("preparation/provider = %#v/%d", preparation, provider.calls)
	}
	if preparation.Result.Action != "remember" ||
		preparation.Result.ResultCode != "DIRECT_CREATED" {
		t.Fatalf("direct result = %#v", preparation.Result)
	}
	if repo.applied.RequestedAction != "remember" ||
		repo.applied.SourceMessageID != currentID ||
		repo.applied.Content != "我喜欢喝生椰拿铁" {
		t.Fatalf("applied = %#v", repo.applied)
	}
	if strings.Contains(provider.last.Prompt, "银河大道88号") ||
		strings.Contains(provider.last.Prompt, "美式咖啡") ||
		!strings.Contains(provider.last.Prompt, "我喜欢喝生椰拿铁") {
		t.Fatalf("planner prompt was not safely referenced: %s", provider.last.Prompt)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(provider.last.Prompt), &payload); err != nil {
		t.Fatalf("decode planner prompt: %v", err)
	}
	if payload["schemaVersion"] != "neo-chat.memory-user-action-input.v2" ||
		payload["currentUserMessage"] != current.Content ||
		payload["referencedPreviousUserMessage"] != "我喜欢喝生椰拿铁。" ||
		provider.last.Metadata["profile"] != "memory-user-action-v2" {
		t.Fatalf("planner payload/metadata = %#v/%#v", payload, provider.last.Metadata)
	}
}

func TestPrepareDirectMemoryActionReferencedRememberRejectsPreviousSecret(t *testing.T) {
	const conversationID = "20000000-0000-4000-8000-000000000001"
	current := Message{
		ID: "30000000-0000-4000-8000-000000000002", ConversationID: conversationID,
		Role: "user", Status: "completed", Content: "那你写进去呀",
	}
	repo := newDirectActionMemoryRepository()
	provider := &directActionProvider{body: "provider must not run"}
	handler := NewHandler(nil, WithUserMemoryService(usermemory.NewService(repo)))
	preparation := handler.prepareDirectMemoryAction(
		context.Background(), conversationID, current,
		Message{ID: "30000000-0000-4000-8000-000000000003"},
		[]Message{
			{
				ID: "30000000-0000-4000-8000-000000000001", ConversationID: conversationID,
				Role: "user", Status: "completed", Content: "我的 password: super-secret-value",
			},
			current,
		},
		provider, ModelRef{ModelID: "fixture"},
	)
	if provider.calls != 0 || preparation.Result == nil ||
		repo.applied.PreflightResultCode != "SECRET_REJECTED" ||
		repo.applied.Content != "" || repo.applied.NormalizedContent != "" {
		t.Fatalf("secret reference escaped: preparation=%#v provider=%d apply=%#v",
			preparation, provider.calls, repo.applied)
	}
}

func TestPrepareDirectMemoryActionReferencedRememberRejectsFullyRedactedPreviousMessage(t *testing.T) {
	const conversationID = "20000000-0000-4000-8000-000000000001"
	current := Message{
		ID: "30000000-0000-4000-8000-000000000002", ConversationID: conversationID,
		Role: "user", Status: "completed", Content: "那你写进去呀",
	}
	repo := newDirectActionMemoryRepository()
	provider := &directActionProvider{body: "provider must not run"}
	handler := NewHandler(nil, WithUserMemoryService(usermemory.NewService(repo)))
	preparation := handler.prepareDirectMemoryAction(
		context.Background(), conversationID, current,
		Message{ID: "30000000-0000-4000-8000-000000000003"},
		[]Message{
			{
				ID: "30000000-0000-4000-8000-000000000001", ConversationID: conversationID,
				Role: "user", Status: "completed", Content: "我的家庭住址：银河大道88号。",
			},
			current,
		},
		provider, ModelRef{ModelID: "fixture"},
	)
	if provider.calls != 0 || preparation.Result == nil ||
		repo.applied.PreflightResultCode != "REFERENCE_REDACTED" ||
		repo.applied.Content != "" || repo.applied.NormalizedContent != "" {
		t.Fatalf("redacted reference escaped: preparation=%#v provider=%d apply=%#v",
			preparation, provider.calls, repo.applied)
	}
}

func TestPrepareDirectMemoryActionReferencedRememberRequiresPreviousUser(t *testing.T) {
	const conversationID = "20000000-0000-4000-8000-000000000001"
	current := Message{
		ID: "30000000-0000-4000-8000-000000000001", ConversationID: conversationID,
		Role: "user", Status: "completed", Content: "那你写进去呀",
	}
	repo := newDirectActionMemoryRepository()
	provider := &directActionProvider{body: "provider must not run"}
	handler := NewHandler(nil, WithUserMemoryService(usermemory.NewService(repo)))
	preparation := handler.prepareDirectMemoryAction(
		context.Background(), conversationID, current,
		Message{ID: "30000000-0000-4000-8000-000000000002"},
		[]Message{current}, provider, ModelRef{ModelID: "fixture"},
	)
	if preparation.Result != nil || preparation.DegradationCode != "" ||
		provider.calls != 0 || repo.applied.ActionID != "" {
		t.Fatalf("missing reference was executed: %#v provider=%d apply=%#v",
			preparation, provider.calls, repo.applied)
	}
}

func TestPrepareDirectMemoryActionCompleteFactDoesNotUsePreviousMessage(t *testing.T) {
	const conversationID = "20000000-0000-4000-8000-000000000001"
	current := Message{
		ID: "30000000-0000-4000-8000-000000000002", ConversationID: conversationID,
		Role: "user", Status: "completed", Content: "请记住我喜欢美式咖啡",
	}
	repo := newDirectActionMemoryRepository()
	provider := &directActionProvider{body: `{"action":"remember",` +
		`"memoryType":"preference","content":"我喜欢美式咖啡",` +
		`"importance":5,"tags":[],"sensitivity":"normal",` +
		`"scopeType":"global","confidence":0.99,"targets":[]}`}
	handler := NewHandler(nil, WithUserMemoryService(usermemory.NewService(repo)))
	preparation := handler.prepareDirectMemoryAction(
		context.Background(), conversationID, current,
		Message{ID: "30000000-0000-4000-8000-000000000003"},
		[]Message{
			{
				ID: "30000000-0000-4000-8000-000000000001", ConversationID: conversationID,
				Role: "user", Status: "completed", Content: "我喜欢喝生椰拿铁",
			},
			current,
		},
		provider, ModelRef{ModelID: "fixture"},
	)
	if preparation.Result == nil || provider.calls != 1 ||
		strings.Contains(provider.last.Prompt, "生椰拿铁") ||
		strings.Contains(provider.last.Prompt, "referencedPreviousUserMessage") ||
		provider.last.Metadata["profile"] != "memory-user-action-v1" {
		t.Fatalf("complete fact mixed history: %#v prompt=%s metadata=%#v",
			preparation, provider.last.Prompt, provider.last.Metadata)
	}
}

func TestDirectMemoryActionReferenceHashBindsBothMessages(t *testing.T) {
	current := "那你写进去呀"
	first := Message{Content: "我喜欢生椰拿铁"}
	second := Message{Content: "我喜欢美式咖啡"}
	firstHash := directMemoryActionRequestHash(current, &first)
	if firstHash == usermemoryHash(current) ||
		firstHash == directMemoryActionRequestHash(current, &second) ||
		firstHash == directMemoryActionRequestHash("那就写进去吧", &first) {
		t.Fatalf("reference hash did not bind inputs: %s", firstHash)
	}
	if got := directMemoryActionRequestHash(current, nil); got != usermemoryHash(current) {
		t.Fatalf("ordinary request hash changed: %s", got)
	}
}

func TestAppendDirectMemoryActionAnswerInstructionUsesServerOutcomeOnly(t *testing.T) {
	tests := []struct {
		name        string
		preparation directMemoryActionPreparation
		want        string
	}{
		{
			name: "remember applied",
			preparation: directMemoryActionPreparation{Result: &usermemory.DirectActionResult{
				Action: "remember", Status: "applied", ResultCode: "DIRECT_CREATED",
				ActionID: "do-not-expose-action-id", MemoryID: "do-not-expose-memory-id",
			}},
			want: "already saved",
		},
		{
			name: "exact noop",
			preparation: directMemoryActionPreparation{Result: &usermemory.DirectActionResult{
				Action: "remember", Status: "noop", ResultCode: "EXACT_NOOP",
			}},
			want: "already present",
		},
		{
			name: "privacy rejection",
			preparation: directMemoryActionPreparation{Result: &usermemory.DirectActionResult{
				Action: "remember", Status: "rejected", ResultCode: "SECRET_REJECTED",
			}},
			want: "privacy or safety",
		},
		{
			name: "review",
			preparation: directMemoryActionPreparation{Result: &usermemory.DirectActionResult{
				Action: "remember", Status: "review_required", ResultCode: "LOW_CONFIDENCE",
			}},
			want: "clarification or review",
		},
		{
			name:        "degraded",
			preparation: directMemoryActionPreparation{DegradationCode: "action_context_failed"},
			want:        "temporary internal failure",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := appendDirectMemoryActionAnswerInstruction("base instruction", test.preparation)
			if !strings.HasPrefix(got, "base instruction\n\n") ||
				!strings.Contains(got, test.want) ||
				!strings.Contains(got, "server result is authoritative") ||
				strings.Contains(got, "DIRECT_CREATED") ||
				strings.Contains(got, "do-not-expose") {
				t.Fatalf("answer instruction = %q", got)
			}
		})
	}
	if got := appendDirectMemoryActionAnswerInstruction(
		"base instruction", directMemoryActionPreparation{},
	); got != "base instruction" {
		t.Fatalf("ordinary prompt changed: %q", got)
	}
}

func TestHandlerReferencedRememberWritesAndInstructsAnswerProvider(t *testing.T) {
	chatRepo := newFakeRepository()
	chatRepo.conversations = append(
		chatRepo.conversations,
		fakeConversation(testConversationID, "referential memory", 3),
	)
	current := fakeMessage(
		testMessageID, testConversationID, 2, "user", "记住",
	)
	chatRepo.messages[testConversationID] = []Message{
		fakeMessage(
			"22222222-2222-4222-8222-222222222221",
			testConversationID,
			0,
			"user",
			"我喜欢喝生椰拿铁",
		),
		fakeMessage(
			"33333333-3333-4333-8333-333333333331",
			testConversationID,
			1,
			"assistant",
			"你可以让我记住这条信息",
		),
		current,
	}
	memoryRepo := newDirectActionMemoryRepository()
	provider := &directActionRoundProvider{plannerBody: `{"action":"remember",` +
		`"memoryType":"preference","content":"我喜欢喝生椰拿铁",` +
		`"importance":5,"tags":["饮品"],"sensitivity":"normal",` +
		`"scopeType":"global","confidence":0.99,"targets":[]}`}
	handler := NewHandler(
		NewService(chatRepo),
		WithProvider(provider),
		WithUserMemoryService(usermemory.NewService(memoryRepo)),
	)

	stream := performRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		`{"userMessageId":"`+testMessageID+`",`+
			`"modelRef":{"providerId":"fixture","modelId":"fixture"},`+
			`"idempotencyKey":"referential-memory-stream"}`,
	)
	assertStreamStatus(t, stream, http.StatusOK)
	if provider.plannerCalls != 1 || provider.answerCalls != 1 ||
		memoryRepo.applied.Content != "我喜欢喝生椰拿铁" {
		t.Fatalf("calls/apply = %d/%d/%#v",
			provider.plannerCalls, provider.answerCalls, memoryRepo.applied)
	}
	if !strings.Contains(provider.answerInput.SystemPrompt, "already saved") ||
		!strings.Contains(provider.answerInput.SystemPrompt, "server result is authoritative") ||
		strings.Contains(provider.answerInput.SystemPrompt, "我喜欢喝生椰拿铁") ||
		strings.Contains(provider.answerInput.SystemPrompt, "DIRECT_CREATED") ||
		strings.Contains(provider.answerInput.SystemPrompt, memoryRepo.applied.ActionID) {
		t.Fatalf("answer system prompt = %q", provider.answerInput.SystemPrompt)
	}
	messages := chatRepo.messages[testConversationID]
	assistant := messages[len(messages)-1]
	metadata, err := json.Marshal(assistant.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	if assistant.Status != "completed" ||
		!strings.Contains(string(metadata), `"memoryActionResults"`) ||
		!strings.Contains(string(metadata), `"resultCode":"DIRECT_CREATED"`) {
		t.Fatalf("assistant = %#v metadata=%s", assistant, metadata)
	}
}

func TestPrepareDirectMemoryActionRequiresCompletedCurrentUserMessage(t *testing.T) {
	tests := []Message{
		{
			ID:   "30000000-0000-4000-8000-000000000001",
			Role: "assistant", Status: "completed", Content: "请记住我喜欢简短回答",
		},
		{
			ID:   "30000000-0000-4000-8000-000000000002",
			Role: "user", Status: "streaming", Content: "请记住我喜欢简短回答",
		},
	}
	for _, message := range tests {
		repo := newDirectActionMemoryRepository()
		provider := &directActionProvider{body: "provider must not run"}
		handler := NewHandler(nil, WithUserMemoryService(usermemory.NewService(repo)))
		preparation := handler.prepareDirectMemoryAction(
			context.Background(),
			"20000000-0000-4000-8000-000000000001",
			message,
			Message{ID: "30000000-0000-4000-8000-000000000003"},
			nil,
			provider,
			ModelRef{ModelID: "fixture"},
		)
		if preparation.Result != nil || preparation.DegradationCode != "" ||
			provider.calls != 0 || repo.applied.ActionID != "" {
			t.Fatalf("non-current-user action = %#v provider=%d apply=%#v",
				preparation, provider.calls, repo.applied)
		}
	}
}

func TestPrepareDirectMemoryActionBindsVisibleTargetAndRejectsSpoof(t *testing.T) {
	const memoryID = "50000000-0000-4000-8000-000000000001"
	tests := []struct {
		name             string
		targetID         string
		expectedRevision int64
		scopeType        string
		wantPreflight    string
		wantTargetCount  int
	}{
		{"visible", memoryID, 7, "global", "", 1},
		{"spoof", "50000000-0000-4000-8000-000000000099", 7, "global", "TARGET_INVALID", 0},
		{"stale", memoryID, 6, "global", "REVISION_STALE", 0},
		{"scope mismatch", memoryID, 7, "project", "TARGET_SCOPE_MISMATCH", 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newDirectActionMemoryRepository()
			repo.context = usermemory.DirectActionContext{
				ProjectID: "60000000-0000-4000-8000-000000000001",
				Memories: []usermemory.DirectActionMemory{{
					ID: memoryID, Revision: 7, Type: "preference",
					Content: "Use detailed replies", ScopeType: "global",
					AuthorityKind: "manual", Sensitivity: "normal",
				}},
			}
			provider := &directActionProvider{body: `{"action":"correct",` +
				`"memoryType":"preference","content":"Use concise replies",` +
				`"importance":5,"tags":["reply"],"sensitivity":"normal",` +
				`"scopeType":"` + test.scopeType + `","confidence":0.99,"targets":[{"memoryId":"` +
				test.targetID + `","expectedRevision":` +
				strconv.FormatInt(test.expectedRevision, 10) + `}]}`}
			handler := NewHandler(nil, WithUserMemoryService(usermemory.NewService(repo)))
			preparation := handler.prepareDirectMemoryAction(
				context.Background(),
				"20000000-0000-4000-8000-000000000001",
				Message{
					ID:   "30000000-0000-4000-8000-000000000001",
					Role: "user", Status: "completed",
					Content: "更正记忆：我喜欢简短回答",
				},
				Message{ID: "30000000-0000-4000-8000-000000000002"},
				nil,
				provider,
				ModelRef{ModelID: "fixture"},
			)
			if preparation.Result == nil || provider.calls != 1 {
				t.Fatalf("preparation/provider = %#v/%d", preparation, provider.calls)
			}
			if repo.applied.PreflightResultCode != test.wantPreflight ||
				len(repo.applied.Targets) != test.wantTargetCount {
				t.Fatalf("apply = %#v", repo.applied)
			}
			if strings.Contains(provider.last.Prompt, repo.context.ProjectID) {
				t.Fatal("provider input exposed bound project id")
			}
		})
	}
}

type directActionProvider struct {
	body      string
	calls     int
	last      ProviderRequest
	lastRound ProviderRoundRequest
}

type directActionScriptProvider struct {
	events   []ProviderEvent
	roundErr error
}

type directActionRoundProvider struct {
	plannerBody  string
	plannerCalls int
	answerCalls  int
	answerInput  ProviderRequest
}

func (p *directActionRoundProvider) StreamChat(
	_ context.Context,
	input ProviderRequest,
) (<-chan ProviderEvent, error) {
	p.answerCalls++
	p.answerInput = input
	events := make(chan ProviderEvent, 1)
	events <- ProviderEvent{Type: ProviderEventDelta, Delta: "已保存"}
	close(events)
	return events, nil
}

func (p *directActionRoundProvider) StreamToolRound(
	_ context.Context,
	input ProviderRoundRequest,
) (<-chan ProviderEvent, error) {
	p.plannerCalls++
	events := make(chan ProviderEvent, 1)
	events <- ProviderEvent{
		Type: ProviderEventToolCallCompleted,
		ToolCall: &ProviderToolCall{
			Name: directMemoryActionToolName, Arguments: p.plannerBody,
		},
	}
	close(events)
	return events, nil
}

func (p *directActionProvider) StreamChat(
	_ context.Context,
	input ProviderRequest,
) (<-chan ProviderEvent, error) {
	p.calls++
	p.last = input
	events := make(chan ProviderEvent, 1)
	events <- ProviderEvent{Type: ProviderEventDelta, Delta: p.body}
	close(events)
	return events, nil
}

func (p *directActionProvider) StreamToolRound(
	_ context.Context,
	input ProviderRoundRequest,
) (<-chan ProviderEvent, error) {
	p.calls++
	p.last = input.ProviderRequest
	p.lastRound = input
	events := make(chan ProviderEvent, 1)
	events <- ProviderEvent{
		Type: ProviderEventToolCallCompleted,
		ToolCall: &ProviderToolCall{
			Name: directMemoryActionToolName, Arguments: p.body,
		},
	}
	close(events)
	return events, nil
}

func (p *directActionScriptProvider) StreamChat(
	context.Context,
	ProviderRequest,
) (<-chan ProviderEvent, error) {
	return nil, errors.New("plain chat must not be used for direct Memory planning")
}

func (p *directActionScriptProvider) StreamToolRound(
	_ context.Context,
	_ ProviderRoundRequest,
) (<-chan ProviderEvent, error) {
	if p.roundErr != nil {
		return nil, p.roundErr
	}
	events := make(chan ProviderEvent, len(p.events))
	for _, event := range p.events {
		events <- event
	}
	close(events)
	return events, nil
}

type directActionMemoryRepository struct {
	*chatMemoryRepository
	context usermemory.DirectActionContext
	applied usermemory.DirectActionApplyInput
}

func newDirectActionMemoryRepository() *directActionMemoryRepository {
	return &directActionMemoryRepository{
		chatMemoryRepository: &chatMemoryRepository{settings: usermemory.DefaultSettings()},
	}
}

func (r *directActionMemoryRepository) HydrateDirectAction(
	_ context.Context,
	_ usermemory.DirectActionHydrationInput,
) (usermemory.DirectActionContext, error) {
	return r.context, nil
}

func (r *directActionMemoryRepository) ApplyDirectAction(
	_ context.Context,
	input usermemory.DirectActionApplyInput,
) (usermemory.DirectActionResult, error) {
	r.applied = input
	return usermemory.DirectActionResult{
		ActionID: input.ActionID,
		Status: func() string {
			if input.PreflightStatus != "" {
				return input.PreflightStatus
			}
			return "applied"
		}(),
		ResultCode: func() string {
			if input.PreflightResultCode != "" {
				return input.PreflightResultCode
			}
			switch input.RequestedAction {
			case "remember":
				return "DIRECT_CREATED"
			case "forget":
				return "DIRECT_FORGOTTEN"
			default:
				return "DIRECT_CORRECTED"
			}
		}(),
		MemoryID:       "50000000-0000-4000-8000-000000000001",
		MemoryRevision: 8,
		ScopeType:      input.ScopeType,
		ActivityID:     input.ActivityID,
	}, nil
}

func (r *directActionMemoryRepository) ListActivities(
	context.Context, string, int,
) ([]usermemory.MemoryActivity, error) {
	return nil, nil
}

func (r *directActionMemoryRepository) ListMessageUsages(
	context.Context, string,
) ([]usermemory.MessageMemoryUsage, error) {
	return nil, nil
}

func (r *directActionMemoryRepository) UndoActivity(
	context.Context, usermemory.UndoActivityInput,
) (usermemory.UndoActivityResult, error) {
	return usermemory.UndoActivityResult{}, nil
}

var _ usermemory.ActionRepository = (*directActionMemoryRepository)(nil)
