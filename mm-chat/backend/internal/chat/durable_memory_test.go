package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

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
	}
	provider := &capturingProvider{}
	handler := NewHandler(
		NewService(chatRepo),
		WithProvider(provider),
		WithUserMemoryService(usermemory.NewService(memoryRepo)),
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
		strings.Contains(string(metadataJSON), memoryRepo.memories[0].Content) {
		t.Fatalf("assistant metadata = %s", metadataJSON)
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
}

func TestPrepareDurableMemoryDisabledDoesNotReadRows(t *testing.T) {
	repo := &chatMemoryRepository{settings: usermemory.DefaultSettings()}
	handler := NewHandler(
		NewService(&fakeRepository{}),
		WithUserMemoryService(usermemory.NewService(repo)),
	)
	systemPrompt, preparation := handler.prepareDurableMemory(
		context.Background(), "concise", "Base",
	)
	if systemPrompt != "Base" || len(preparation.Items) != 0 || repo.listCalls != 0 {
		t.Fatalf("disabled preparation = %q/%#v listCalls=%d", systemPrompt, preparation, repo.listCalls)
	}
}

func TestParseDurableMemoryCandidatesRejectsSecretsAndVagueContext(t *testing.T) {
	candidates, err := parseDurableMemoryCandidates(`Here is the result:
{"memories":[
  {"type":"preference","content":"Use concise answers","importance":5,"tags":["style"]},
  {"type":"fact","content":"My API key is sk-abcdefghijk","importance":5,"tags":[]},
  {"type":"fact","content":"The user has a login","importance":5,"tags":["password-secret"]},
  {"type":"context","content":"Current question context","importance":3,"tags":[]}
]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Content != "Use concise answers" {
		t.Fatalf("candidates = %#v", candidates)
	}
}

func TestDurableMemoryExtractionProviderFailureIsContained(t *testing.T) {
	repo := &chatMemoryRepository{settings: usermemory.Settings{
		Enabled: true, SearchEnabled: true, AutoRecordEnabled: true,
	}}
	provider := &failingMemoryExtractionProvider{called: make(chan struct{}, 1)}
	handler := NewHandler(
		NewService(&fakeRepository{}),
		WithUserMemoryService(usermemory.NewService(repo)),
	)
	handler.queueDurableMemoryExtraction(
		context.Background(), provider,
		ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		"33333333-3333-4333-8333-333333333333",
		"44444444-4444-4444-8444-444444444444",
		"Remember that I prefer concise answers",
	)
	select {
	case <-provider.called:
	case <-time.After(time.Second):
		t.Fatal("background extraction provider was not called")
	}
	if repo.createCalls != 0 {
		t.Fatalf("failed extraction created %d memories", repo.createCalls)
	}
}

type failingMemoryExtractionProvider struct {
	called chan struct{}
}

func (p *failingMemoryExtractionProvider) StreamChat(context.Context, ProviderRequest) (<-chan ProviderEvent, error) {
	p.called <- struct{}{}
	return nil, errors.New("fixture provider failure")
}

type chatMemoryRepository struct {
	settings    usermemory.Settings
	memories    []usermemory.Memory
	listCalls   int
	createCalls int
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

func chatMemoryFixture(id string, memoryType string, content string, importance int) usermemory.Memory {
	now := time.Now().UTC()
	return usermemory.Memory{
		ID: id, Type: memoryType, Content: content, Importance: importance,
		Tags: []string{}, Source: "manual", Enabled: true,
		CreatedAt: now, UpdatedAt: now,
	}
}
