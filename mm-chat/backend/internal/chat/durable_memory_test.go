package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/runtimeconfig"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestStreamAssistantInjectsRelevantDurableMemoryAndPersistsIDsOnly(t *testing.T) {
	chatRepo := newFakeRepository()
	memoryRepo := &chatMemoryRepository{
		settings: usermemory.Settings{Enabled: true, SearchEnabled: true},
		memories: []usermemory.Memory{
			chatMemoryFixture(
				"11111111-1111-4111-8111-111111111112",
				"preference",
				"Keep answers concise",
				5,
			),
		},
		shadow: usermemory.LexicalShadowSummary{
			ProfileID: usermemory.LexicalShadowProfileID,
			Status:    "completed", ResultCode: "OK",
			BaselineCount: 1, ExactCount: 1, BM25Count: 1,
			LexicalCount: 1, OverlapCount: 1, DurationMillis: 2,
		},
	}
	provider := &capturingProvider{}
	wake := &capturingMemoryWakePublisher{}
	handler := NewHandler(
		NewService(chatRepo),
		WithProvider(provider),
		WithUserMemoryService(usermemory.NewService(memoryRepo)),
		WithMemoryLexicalShadowEnabled(true),
		WithMemoryWakePublisher(wake),
	)
	createdConversation := performRequest(
		handler,
		http.MethodPost,
		conversationsPath,
		`{"title":"memory stream"}`,
	)
	assertStatus(t, createdConversation, http.StatusCreated)
	createdMessage := performRequest(
		handler,
		http.MethodPost,
		conversationPathBase+testConversationID+"/messages",
		`{"content":"Please keep this concise"}`,
	)
	assertStatus(t, createdMessage, http.StatusCreated)
	var userMessage ChatMessageDTO
	decodeBody(t, createdMessage, &userMessage)

	stream := performRequest(
		handler,
		http.MethodPost,
		conversationPathBase+testConversationID+"/stream",
		`{"userMessageId":"`+userMessage.ID+`",`+
			`"modelRef":{"providerId":"fixture","modelId":"fixture"},`+
			`"idempotencyKey":"memory-stream-run"}`,
	)
	if stream.Code != http.StatusOK {
		t.Fatalf("stream status = %d; body=%s", stream.Code, stream.Body.String())
	}
	if !strings.Contains(provider.input.SystemPrompt, "Keep answers concise") {
		t.Fatalf("provider system prompt = %q", provider.input.SystemPrompt)
	}
	messages := chatRepo.messages[testConversationID]
	assistant := messages[len(messages)-1]
	metadataJSON, err := json.Marshal(assistant.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(metadataJSON), memoryRepo.memories[0].ID) ||
		strings.Contains(string(metadataJSON), memoryRepo.memories[0].Content) ||
		!strings.Contains(string(metadataJSON), `"lexicalShadow"`) {
		t.Fatalf("assistant metadata = %s", metadataJSON)
	}
	if memoryRepo.shadowCalls != 1 ||
		memoryRepo.shadowInput.ConversationID != testConversationID ||
		memoryRepo.shadowInput.AssistantMessageID != assistant.ID ||
		memoryRepo.shadowInput.QueryText != userMessage.Content ||
		len(memoryRepo.shadowInput.Baseline) != 1 {
		t.Fatalf("stream lexical shadow input = calls:%d input:%#v",
			memoryRepo.shadowCalls, memoryRepo.shadowInput)
	}
	if wake.eventID == "" || !isUUID(wake.eventID) {
		t.Fatalf("memory wake event id = %q", wake.eventID)
	}
}

func TestNewDurableMemoryCapturePinsServerStoredProfileWithoutSecret(t *testing.T) {
	capture, err := newDurableMemoryCapture(
		"44444444-4444-4444-8444-444444444444",
		ModelRef{ProviderID: "request-provider", ModelID: "fixture-model"},
		&runtimeconfig.ProviderRuntimeConfig{
			ID: "stored-provider", Source: "server-stored", APIKey: "must-not-copy",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if capture.ProviderSource != "server-stored" ||
		capture.ProviderID != "stored-provider" ||
		capture.ModelID != "fixture-model" ||
		capture.EventSchemaMajor != MemoryCaptureEventSchemaMajor ||
		!isUUID(capture.EventID) || !isUUID(capture.JobID) {
		t.Fatalf("capture = %#v", capture)
	}
}

type capturingMemoryWakePublisher struct {
	eventID string
	calls   int
}

func (p *capturingMemoryWakePublisher) PublishMemoryWake(
	_ context.Context,
	eventID string,
) error {
	p.eventID = eventID
	p.calls++
	return nil
}

func TestPublishDurableMemoryWakeSkipsMissingEvent(t *testing.T) {
	publisher := &capturingMemoryWakePublisher{}
	handler := &Handler{memoryWakePublisher: publisher}
	handler.publishDurableMemoryWake("")
	if publisher.calls != 0 {
		t.Fatalf("wake calls = %d, want 0", publisher.calls)
	}
}

func TestPrepareDurableMemoryInjectsOnlyRelevantEntries(t *testing.T) {
	repo := &chatMemoryRepository{
		settings: usermemory.Settings{Enabled: true, SearchEnabled: true},
		memories: []usermemory.Memory{
			chatMemoryFixture("11111111-1111-4111-8111-111111111111", "preference", "Keep answers concise", 5),
			chatMemoryFixture("22222222-2222-4222-8222-222222222222", "fact", "The user owns a blue bicycle", 5),
		},
	}
	handler := NewHandler(
		NewService(&fakeRepository{}),
		WithUserMemoryService(usermemory.NewService(repo)),
	)
	systemPrompt, preparation := handler.prepareDurableMemory(
		context.Background(),
		"Please keep this concise",
		"",
		"",
		"Base instruction",
	)
	if len(preparation.Items) != 1 || preparation.Items[0].ID != repo.memories[0].ID {
		t.Fatalf("preparation = %#v", preparation)
	}
	if !strings.Contains(systemPrompt, "Keep answers concise") ||
		strings.Contains(systemPrompt, "blue bicycle") ||
		!strings.Contains(systemPrompt, "lower-priority, untrusted") {
		t.Fatalf("system prompt = %q", systemPrompt)
	}
	metadata := withDurableMemoryMetadata(map[string]any{"runId": "run-1"}, preparation)
	encoded := metadata["memory"].(map[string]any)
	payload, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if encoded["retrievedCount"] != 1 || strings.Contains(string(payload), "Keep answers concise") {
		t.Fatalf("memory metadata leaked content: %#v", metadata)
	}
	if repo.shadowCalls != 0 || repo.hybridCalls != 0 {
		t.Fatalf("default-off shadow calls = lexical:%d hybrid:%d",
			repo.shadowCalls, repo.hybridCalls)
	}
}

func TestPrepareDurableMemoryShadowFailureDoesNotChangePromptOrV1Usage(t *testing.T) {
	repo := &chatMemoryRepository{
		settings: usermemory.Settings{Enabled: true, SearchEnabled: true},
		memories: []usermemory.Memory{
			chatMemoryFixture("11111111-1111-4111-8111-111111111111", "preference", "Keep answers concise", 5),
		},
		shadowErr: errors.New("raw query and database details must stay private"),
	}
	handler := NewHandler(
		NewService(&fakeRepository{}),
		WithUserMemoryService(usermemory.NewService(repo)),
		WithMemoryLexicalShadowEnabled(true),
	)
	systemPrompt, preparation := handler.prepareDurableMemory(
		context.Background(),
		"Please keep this concise",
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333",
		"Base instruction",
	)
	if repo.shadowCalls != 1 || len(preparation.Items) != 1 ||
		!strings.Contains(systemPrompt, "Keep answers concise") {
		t.Fatalf("shadow failure changed v1 preparation = calls:%d prompt:%q prep:%#v",
			repo.shadowCalls, systemPrompt, preparation)
	}
	usages := durableMemoryUsageInputs(preparation)
	if len(usages) != 1 || usages[0].MemoryID != repo.memories[0].ID ||
		usages[0].Revision != repo.memories[0].Revision {
		t.Fatalf("shadow failure changed Usage = %#v", usages)
	}
	metadata := withDurableMemoryMetadata(nil, preparation)
	payload, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	if !strings.Contains(encoded, `"resultCode":"COMPARE_FAILED"`) ||
		strings.Contains(encoded, "raw query") || strings.Contains(encoded, "database details") ||
		strings.Contains(encoded, "Keep answers concise") {
		t.Fatalf("shadow metadata is not bounded = %s", encoded)
	}
}

func TestPrepareDurableMemoryHybridShadowNeverChangesPromptUsageAndWinsFlagPrecedence(t *testing.T) {
	repo := &chatMemoryRepository{
		settings: usermemory.Settings{Enabled: true, SearchEnabled: true},
		memories: []usermemory.Memory{
			chatMemoryFixture(
				"11111111-1111-4111-8111-111111111111",
				"preference",
				"Keep answers concise",
				5,
			),
		},
		hybridPreparation: usermemory.HybridShadowPreparation{
			Summary: usermemory.HybridShadowSummary{
				ProfileID: usermemory.HybridShadowProfileID,
				Status:    "pending", ResultCode: "CANDIDATES_READY",
				FallbackCode: "PROVIDER_UNAVAILABLE", RRFCount: 1,
			},
			Candidates: []usermemory.HybridShadowCandidate{{
				MemoryID: "22222222-2222-4222-8222-222222222222",
				Revision: 1, ScopeType: "global",
				Content: "Hybrid-only private candidate",
			}},
		},
		hybridSummary: usermemory.HybridShadowSummary{
			ProfileID: usermemory.HybridShadowProfileID,
			Status:    "completed", ResultCode: "OK",
			FallbackCode: "PROVIDER_UNAVAILABLE", RRFCount: 1, FinalCount: 1,
		},
	}
	handler := NewHandler(
		NewService(&fakeRepository{}),
		WithUserMemoryService(usermemory.NewService(repo)),
		WithMemoryLexicalShadowEnabled(true),
		WithMemoryHybridShadowEnabled(true),
	)
	query := "Please keep this concise"
	systemPrompt, preparation := handler.prepareDurableMemory(
		context.Background(), query,
		"33333333-3333-4333-8333-333333333333",
		"44444444-4444-4444-8444-444444444444",
		"Base instruction",
	)
	if repo.hybridCalls != 1 || repo.shadowCalls != 0 ||
		!strings.Contains(systemPrompt, "Keep answers concise") ||
		strings.Contains(systemPrompt, "Hybrid-only private candidate") {
		t.Fatalf("hybrid changed prompt/precedence = prompt:%q repo:%#v",
			systemPrompt, repo)
	}
	usages := durableMemoryUsageInputs(preparation)
	if len(usages) != 1 || usages[0].MemoryID != repo.memories[0].ID {
		t.Fatalf("hybrid changed Usage = %#v", usages)
	}
	metadata := withDurableMemoryMetadata(nil, preparation)
	payload, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	if !strings.Contains(encoded, `"hybridShadow"`) ||
		strings.Contains(encoded, query) ||
		strings.Contains(encoded, "Hybrid-only private candidate") ||
		strings.Contains(encoded, `"lexicalShadow"`) {
		t.Fatalf("hybrid metadata leaked private payload = %s", encoded)
	}
}

func TestPrepareDurableMemoryHybridFlagOffMakesZeroShadowCalls(t *testing.T) {
	repo := &chatMemoryRepository{
		settings: usermemory.Settings{Enabled: true, SearchEnabled: true},
		memories: []usermemory.Memory{chatMemoryFixture(
			"11111111-1111-4111-8111-111111111111",
			"preference",
			"Keep answers concise",
			5,
		)},
	}
	handler := NewHandler(
		NewService(&fakeRepository{}),
		WithUserMemoryService(usermemory.NewService(repo)),
	)
	_, preparation := handler.prepareDurableMemory(
		context.Background(),
		"Please keep this concise",
		"33333333-3333-4333-8333-333333333333",
		"44444444-4444-4444-8444-444444444444",
		"Base instruction",
	)
	if repo.hybridCalls != 0 || repo.shadowCalls != 0 || len(preparation.Items) != 1 {
		t.Fatalf("flag-off shadow calls = hybrid:%d lexical:%d items:%d",
			repo.hybridCalls, repo.shadowCalls, len(preparation.Items))
	}
}

func TestPrepareDurableMemoryL2ShadowNeverChangesPromptOrUsage(t *testing.T) {
	repo := &chatMemoryRepository{
		settings: usermemory.Settings{Enabled: true, SearchEnabled: true},
		memories: []usermemory.Memory{chatMemoryFixture(
			"11111111-1111-4111-8111-111111111111",
			"preference",
			"Keep answers concise",
			5,
		)},
		l2Preparation: usermemory.L2ScenePreparation{
			Summary: usermemory.L2SceneSearchSummary{
				ProfileID: usermemory.L2SceneProfileID, Mode: "shadow",
				Status: "pending", ResultCode: "CANDIDATES_READY",
				FallbackCode: "PROVIDER_UNAVAILABLE", RRFCount: 1,
			},
			Candidates: []usermemory.L2SceneCandidate{{
				SceneID:  "22222222-2222-4222-8222-222222222222",
				Revision: 1, ScopeType: "global", Content: "Shadow-only Scene content",
			}},
		},
		l2Result: usermemory.L2SceneSearchResult{
			Summary: usermemory.L2SceneSearchSummary{
				ProfileID: usermemory.L2SceneProfileID, Mode: "shadow",
				Status: "completed", ResultCode: "COMPLETED",
				FallbackCode: "PROVIDER_UNAVAILABLE", FinalCount: 1,
			},
			Scenes: []usermemory.L2SceneCandidate{{
				SceneID:  "22222222-2222-4222-8222-222222222222",
				Revision: 1, ScopeType: "global", Content: "Shadow-only Scene content",
			}},
		},
	}
	handler := NewHandler(
		NewService(&fakeRepository{}),
		WithUserMemoryService(usermemory.NewService(repo)),
		WithMemoryL2SceneShadowEnabled(true),
	)
	systemPrompt, preparation := handler.prepareDurableMemory(
		context.Background(), "Please keep this concise",
		"33333333-3333-4333-8333-333333333333",
		"44444444-4444-4444-8444-444444444444", "Base instruction",
	)
	if repo.l2PrepareCalls != 1 || repo.l2Prepare.ActiveRequested ||
		!strings.Contains(systemPrompt, "Keep answers concise") ||
		strings.Contains(systemPrompt, "Shadow-only Scene content") ||
		strings.Contains(systemPrompt, "22222222-2222-4222-8222-222222222222") {
		t.Fatalf("L2 shadow changed prompt = %q repo=%#v", systemPrompt, repo)
	}
	usages := durableMemoryUsageInputs(preparation)
	if len(usages) != 1 || usages[0].MemoryID != repo.memories[0].ID {
		t.Fatalf("L2 shadow changed Usage = %#v", usages)
	}
	payload, err := json.Marshal(withDurableMemoryMetadata(nil, preparation))
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	if !strings.Contains(encoded, `"l2Scene"`) ||
		strings.Contains(encoded, "Shadow-only Scene content") ||
		strings.Contains(encoded, "22222222-2222-4222-8222-222222222222") {
		t.Fatalf("L2 shadow metadata leaked payload = %s", encoded)
	}
}

func TestPrepareDurableMemoryL2ActiveUsesSeparatePromptBlockAndL1Usage(t *testing.T) {
	repo := &chatMemoryRepository{
		settings: usermemory.Settings{Enabled: true, SearchEnabled: true},
		memories: []usermemory.Memory{chatMemoryFixture(
			"11111111-1111-4111-8111-111111111111",
			"preference", "Keep answers concise", 5,
		)},
		l2Preparation: usermemory.L2ScenePreparation{
			Summary: usermemory.L2SceneSearchSummary{
				ProfileID: usermemory.L2SceneProfileID, Mode: "active",
				Status: "pending", ResultCode: "CANDIDATES_READY",
				FallbackCode: "PROVIDER_UNAVAILABLE", RRFCount: 1,
			},
			Candidates: []usermemory.L2SceneCandidate{{
				SceneID:  "22222222-2222-4222-8222-222222222222",
				Revision: 1, ScopeType: "project", Content: "The current project uses Go.",
			}},
		},
		l2Result: usermemory.L2SceneSearchResult{
			Summary: usermemory.L2SceneSearchSummary{
				ProfileID: usermemory.L2SceneProfileID, Mode: "active",
				Status: "completed", ResultCode: "COMPLETED",
				FallbackCode: "PROVIDER_UNAVAILABLE", FinalCount: 1,
				InjectedCount: 1,
			},
			Scenes: []usermemory.L2SceneCandidate{{
				SceneID:  "22222222-2222-4222-8222-222222222222",
				Revision: 1, ScopeType: "project", Content: "The current project uses Go.",
			}},
		},
	}
	handler := NewHandler(
		NewService(&fakeRepository{}),
		WithUserMemoryService(usermemory.NewService(repo)),
		WithMemoryL2SceneShadowEnabled(true),
		WithMemoryL2SceneReaderEnabled(true),
	)
	systemPrompt, preparation := handler.prepareDurableMemory(
		context.Background(), "Please keep this concise for the current project",
		"33333333-3333-4333-8333-333333333333",
		"44444444-4444-4444-8444-444444444444", "Base instruction",
	)
	if !repo.l2Prepare.ActiveRequested ||
		!strings.Contains(systemPrompt, "<relevant-user-memory>") ||
		!strings.Contains(systemPrompt, "<relevant-user-scenes>") ||
		!strings.Contains(systemPrompt, "The current project uses Go.") ||
		strings.Contains(systemPrompt, "22222222-2222-4222-8222-222222222222") {
		t.Fatalf("L2 active prompt = %q prepare=%#v", systemPrompt, repo.l2Prepare)
	}
	if len(preparation.ActiveScenes) != 1 {
		t.Fatalf("L2 active preparation = %#v", preparation)
	}
	usages := durableMemoryUsageInputs(preparation)
	if len(usages) != 1 || usages[0].MemoryID != repo.memories[0].ID {
		t.Fatalf("L2 active changed L1 Usage = %#v", usages)
	}
}

func TestPrepareDurableMemoryL2FailureFallsBackToL1(t *testing.T) {
	repo := &chatMemoryRepository{
		settings: usermemory.Settings{Enabled: true, SearchEnabled: true},
		memories: []usermemory.Memory{chatMemoryFixture(
			"11111111-1111-4111-8111-111111111111",
			"preference", "Keep answers concise", 5,
		)},
		l2PrepareErr: errors.New("private Scene database error"),
	}
	handler := NewHandler(
		NewService(&fakeRepository{}),
		WithUserMemoryService(usermemory.NewService(repo)),
		WithMemoryL2SceneShadowEnabled(true),
		WithMemoryL2SceneReaderEnabled(true),
	)
	systemPrompt, preparation := handler.prepareDurableMemory(
		context.Background(), "Please keep this concise",
		"33333333-3333-4333-8333-333333333333",
		"44444444-4444-4444-8444-444444444444", "Base instruction",
	)
	if !strings.Contains(systemPrompt, "Keep answers concise") ||
		strings.Contains(systemPrompt, "private Scene database error") ||
		len(preparation.Items) != 1 || len(preparation.ActiveScenes) != 0 ||
		preparation.L2Scene == nil || preparation.L2Scene.ResultCode != "PREPARE_FAILED" {
		t.Fatalf("L2 failure changed L1 = prompt:%q prep:%#v", systemPrompt, preparation)
	}
}

func TestPrepareDurableMemoryL2ReaderFlagAloneMakesZeroSceneCalls(t *testing.T) {
	repo := &chatMemoryRepository{
		settings: usermemory.Settings{Enabled: true, SearchEnabled: true},
		memories: []usermemory.Memory{chatMemoryFixture(
			"11111111-1111-4111-8111-111111111111",
			"preference", "Keep answers concise", 5,
		)},
	}
	handler := NewHandler(
		NewService(&fakeRepository{}),
		WithUserMemoryService(usermemory.NewService(repo)),
		WithMemoryL2SceneReaderEnabled(true),
	)
	_, preparation := handler.prepareDurableMemory(
		context.Background(), "Please keep this concise",
		"33333333-3333-4333-8333-333333333333",
		"44444444-4444-4444-8444-444444444444", "Base instruction",
	)
	if repo.l2PrepareCalls != 0 || preparation.L2Scene != nil {
		t.Fatalf("reader-only flag reached L2 = calls:%d prep:%#v",
			repo.l2PrepareCalls, preparation)
	}
}

func TestPrepareDurableMemoryDisabledDoesNotReadRows(t *testing.T) {
	repo := &chatMemoryRepository{settings: usermemory.DefaultSettings()}
	handler := NewHandler(
		NewService(&fakeRepository{}),
		WithUserMemoryService(usermemory.NewService(repo)),
	)
	systemPrompt, preparation := handler.prepareDurableMemory(
		context.Background(), "concise", "", "", "Base",
	)
	if systemPrompt != "Base" || len(preparation.Items) != 0 || repo.listCalls != 0 {
		t.Fatalf("disabled preparation = %q/%#v listCalls=%d", systemPrompt, preparation, repo.listCalls)
	}
}

func TestPrepareDurableMemoryConversationUseOffBlocksV1PromptAndUsage(t *testing.T) {
	base := &chatMemoryRepository{
		settings: usermemory.Settings{Enabled: true, SearchEnabled: true},
		memories: []usermemory.Memory{chatMemoryFixture(
			"11111111-1111-4111-8111-111111111111",
			"preference", "Keep answers concise", 5,
		)},
	}
	repo := &chatGovernanceMemoryRepository{
		chatMemoryRepository: base,
		policy: usermemory.ConversationMemoryPolicy{
			ConversationID: "33333333-3333-4333-8333-333333333333",
			UseMode:        "off", LearnMode: "inherit", EffectiveUse: false,
		},
	}
	handler := NewHandler(
		NewService(&fakeRepository{}),
		WithUserMemoryService(usermemory.NewService(repo)),
		WithMemoryHybridShadowEnabled(true),
	)
	systemPrompt, preparation := handler.prepareDurableMemory(
		context.Background(), "Please keep this concise",
		repo.policy.ConversationID,
		"44444444-4444-4444-8444-444444444444", "Base",
	)
	if systemPrompt != "Base" || len(preparation.Items) != 0 ||
		base.listCalls != 0 || base.shadowCalls != 0 || base.hybridCalls != 0 {
		t.Fatalf("Use=off preparation = prompt:%q prep:%#v repo:%#v",
			systemPrompt, preparation, base)
	}
}

func TestConversationMemoryPolicyRoutesAndStaleConflict(t *testing.T) {
	repo := &chatGovernanceMemoryRepository{
		chatMemoryRepository: &chatMemoryRepository{},
		policy: usermemory.ConversationMemoryPolicy{
			ConversationID: "33333333-3333-4333-8333-333333333333",
			UseMode:        "inherit", LearnMode: "inherit", ScopeGeneration: 1,
		},
	}
	handler := NewHandler(
		NewService(&fakeRepository{}),
		WithUserMemoryService(usermemory.NewService(repo)),
	)
	path := "/v1/chat/conversations/" + repo.policy.ConversationID + "/memory-policy"

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"useMode":"inherit"`) {
		t.Fatalf("GET policy = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPatch, path, strings.NewReader(
		`{"expectedScopeGeneration":1,"projectId":"","useMode":"off","learnMode":"on"}`,
	)))
	if response.Code != http.StatusOK || repo.updatedPolicy.UseMode != "off" || repo.updatedPolicy.LearnMode != "on" {
		t.Fatalf("PATCH policy = %d %s input=%#v", response.Code, response.Body.String(), repo.updatedPolicy)
	}

	repo.updatePolicyErr = usermemory.ValidationError{
		Code:    "MEMORY_GOVERNANCE_SCOPE_STALE",
		Message: "memory governance state changed; reload and retry",
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPatch, path, strings.NewReader(
		`{"expectedScopeGeneration":1,"projectId":"","useMode":"off","learnMode":"on"}`,
	)))
	if response.Code != http.StatusConflict ||
		!strings.Contains(response.Body.String(), "MEMORY_GOVERNANCE_SCOPE_STALE") {
		t.Fatalf("stale PATCH policy = %d %s", response.Code, response.Body.String())
	}
}

type chatMemoryRepository struct {
	settings          usermemory.Settings
	memories          []usermemory.Memory
	listCalls         int
	createCalls       int
	shadowCalls       int
	shadowInput       usermemory.LexicalShadowInput
	shadow            usermemory.LexicalShadowSummary
	shadowErr         error
	hybridCalls       int
	hybridPrepare     usermemory.HybridShadowPrepareInput
	hybridRecord      usermemory.HybridShadowRecordInput
	hybridPreparation usermemory.HybridShadowPreparation
	hybridSummary     usermemory.HybridShadowSummary
	hybridPrepareErr  error
	hybridRecordErr   error
	l2PrepareCalls    int
	l2RecordCalls     int
	l2Prepare         usermemory.L2ScenePrepareInput
	l2Record          usermemory.L2SceneRecordInput
	l2Preparation     usermemory.L2ScenePreparation
	l2Result          usermemory.L2SceneSearchResult
	l2PrepareErr      error
	l2RecordErr       error
}

func (r *chatMemoryRepository) GetSettings(context.Context) (usermemory.Settings, bool, error) {
	return r.settings, true, nil
}

func (r *chatMemoryRepository) UpsertSettings(
	_ context.Context,
	input usermemory.Settings,
) (usermemory.Settings, error) {
	r.settings = input
	return input, nil
}

func (r *chatMemoryRepository) List(context.Context) ([]usermemory.Memory, error) {
	r.listCalls++
	return append([]usermemory.Memory(nil), r.memories...), nil
}

func (r *chatMemoryRepository) Create(_ context.Context, input usermemory.CreateInput) (usermemory.Memory, error) {
	r.createCalls++
	return usermemory.Memory{ID: input.ID}, nil
}

func (r *chatMemoryRepository) Update(context.Context, string, usermemory.UpdateInput) (usermemory.Memory, error) {
	return usermemory.Memory{}, usermemory.ErrMemoryNotFound
}

func (r *chatMemoryRepository) Delete(context.Context, string) error {
	return usermemory.ErrMemoryNotFound
}

func (r *chatMemoryRepository) MarkUsed(context.Context, []string, time.Time) error {
	return nil
}

func (r *chatMemoryRepository) CompareLexicalShadow(
	_ context.Context,
	input usermemory.LexicalShadowInput,
) (usermemory.LexicalShadowSummary, error) {
	r.shadowCalls++
	r.shadowInput = input
	return r.shadow, r.shadowErr
}

func (r *chatMemoryRepository) PrepareHybridShadow(
	_ context.Context,
	input usermemory.HybridShadowPrepareInput,
) (usermemory.HybridShadowPreparation, error) {
	r.hybridCalls++
	r.hybridPrepare = input
	return r.hybridPreparation, r.hybridPrepareErr
}

func (r *chatMemoryRepository) RecordHybridShadow(
	_ context.Context,
	input usermemory.HybridShadowRecordInput,
) (usermemory.HybridShadowSummary, error) {
	r.hybridRecord = input
	return r.hybridSummary, r.hybridRecordErr
}

func (r *chatMemoryRepository) PrepareL2SceneSearch(
	_ context.Context,
	input usermemory.L2ScenePrepareInput,
) (usermemory.L2ScenePreparation, error) {
	r.l2PrepareCalls++
	r.l2Prepare = input
	return r.l2Preparation, r.l2PrepareErr
}

func (r *chatMemoryRepository) RecordL2SceneSearch(
	_ context.Context,
	input usermemory.L2SceneRecordInput,
) (usermemory.L2SceneSearchResult, error) {
	r.l2RecordCalls++
	r.l2Record = input
	return r.l2Result, r.l2RecordErr
}

func chatMemoryFixture(id string, memoryType string, content string, importance int) usermemory.Memory {
	now := time.Now().UTC()
	return usermemory.Memory{
		ID: id, Type: memoryType, Content: content, Importance: importance,
		Tags: []string{}, Source: "manual", Enabled: true,
		CreatedAt: now, UpdatedAt: now, Revision: 1, ScopeType: "global",
	}
}

var _ usermemory.LexicalShadowRepository = (*chatMemoryRepository)(nil)
var _ usermemory.HybridShadowRepository = (*chatMemoryRepository)(nil)
var _ usermemory.L2SceneRepository = (*chatMemoryRepository)(nil)

type chatGovernanceMemoryRepository struct {
	*chatMemoryRepository
	policy          usermemory.ConversationMemoryPolicy
	updatedPolicy   usermemory.UpdateConversationPolicyInput
	updatePolicyErr error
}

func (r *chatGovernanceMemoryRepository) GovernanceSnapshot(context.Context) (usermemory.GovernanceSnapshot, error) {
	return usermemory.GovernanceSnapshot{}, nil
}
func (r *chatGovernanceMemoryRepository) CreateProject(context.Context, usermemory.CreateProjectInput) (usermemory.MemoryProject, error) {
	return usermemory.MemoryProject{}, nil
}
func (r *chatGovernanceMemoryRepository) UpdateProject(context.Context, usermemory.UpdateProjectInput) (usermemory.MemoryProject, error) {
	return usermemory.MemoryProject{}, nil
}
func (r *chatGovernanceMemoryRepository) GetConversationPolicy(context.Context, string) (usermemory.ConversationMemoryPolicy, error) {
	return r.policy, nil
}
func (r *chatGovernanceMemoryRepository) UpdateConversationPolicy(_ context.Context, input usermemory.UpdateConversationPolicyInput) (usermemory.ConversationMemoryPolicy, error) {
	r.updatedPolicy = input
	if r.updatePolicyErr != nil {
		return usermemory.ConversationMemoryPolicy{}, r.updatePolicyErr
	}
	return r.policy, nil
}
func (r *chatGovernanceMemoryRepository) CreateGovernanceMemory(context.Context, usermemory.GovernanceMemoryMutationInput) (usermemory.GovernanceMemory, error) {
	return usermemory.GovernanceMemory{}, nil
}
func (r *chatGovernanceMemoryRepository) UpdateGovernanceMemory(context.Context, usermemory.GovernanceMemoryMutationInput) (usermemory.GovernanceMemory, error) {
	return usermemory.GovernanceMemory{}, nil
}
func (r *chatGovernanceMemoryRepository) DeleteGovernanceMemory(context.Context, usermemory.GovernanceMemoryDeleteInput) (usermemory.MemoryDeletionProgress, error) {
	return usermemory.MemoryDeletionProgress{}, nil
}
func (r *chatGovernanceMemoryRepository) GovernanceMemoryDetail(context.Context, string) (usermemory.GovernanceMemoryDetail, error) {
	return usermemory.GovernanceMemoryDetail{}, nil
}
func (r *chatGovernanceMemoryRepository) DecideMemoryReview(context.Context, usermemory.MemoryReviewDecisionInput) (usermemory.MemoryReviewDecisionResult, error) {
	return usermemory.MemoryReviewDecisionResult{}, nil
}
func (r *chatGovernanceMemoryRepository) ListMessageActivities(context.Context, string, int) ([]usermemory.MemoryActivity, error) {
	return nil, nil
}

var _ usermemory.GovernanceRepository = (*chatGovernanceMemoryRepository)(nil)
