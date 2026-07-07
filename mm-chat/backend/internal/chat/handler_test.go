package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	testConversationID = "11111111-1111-4111-8111-111111111111"
	testMessageID      = "22222222-2222-4222-8222-222222222222"
)

func TestHandlerCreatesAndListsConversations(t *testing.T) {
	repo := newFakeRepository()
	handler := NewHandler(NewService(repo))

	rec := performRequest(
		handler,
		http.MethodPost,
		conversationsPath,
		`{"title":" First ","modelRef":{"providerId":"openai","modelId":"gpt-test"},"systemInstruction":"be terse","config":{"useSearch":true}}`,
	)
	assertStatus(t, rec, http.StatusCreated)

	var created ConversationDTO
	decodeBody(t, rec, &created)
	if created.ID != testConversationID {
		t.Fatalf("created id = %q, want %q", created.ID, testConversationID)
	}
	if created.Title != "First" {
		t.Fatalf("created title = %q, want First", created.Title)
	}
	if created.ModelRef == nil || created.ModelRef.ProviderID != "openai" || created.ModelRef.ModelID != "gpt-test" {
		t.Fatalf("created modelRef = %#v, want openai/gpt-test", created.ModelRef)
	}
	if created.MessageCount != 0 {
		t.Fatalf("created messageCount = %d, want 0", created.MessageCount)
	}
	if got := created.Config["useSearch"]; got != true {
		t.Fatalf("created config useSearch = %#v, want true", got)
	}

	rec = performRequest(handler, http.MethodGet, conversationsPath, "")
	assertStatus(t, rec, http.StatusOK)

	var listed Page[ConversationDTO]
	decodeBody(t, rec, &listed)
	if len(listed.Items) != 1 {
		t.Fatalf("listed items = %d, want 1; body=%s", len(listed.Items), rec.Body.String())
	}
	if listed.Items[0].ID != testConversationID {
		t.Fatalf("listed id = %q, want %q", listed.Items[0].ID, testConversationID)
	}
}

func TestHandlerCreatesAndListsMessages(t *testing.T) {
	repo := newFakeRepository()
	repo.conversations = append(repo.conversations, fakeConversation(testConversationID, "First", 0))
	handler := NewHandler(NewService(repo), WithProvider(NewMockProvider()))

	path := conversationsPath + "/" + testConversationID + "/messages"
	rec := performRequest(handler, http.MethodPost, path, `{"content":"hello"}`)
	assertStatus(t, rec, http.StatusCreated)

	var created ChatMessageDTO
	decodeBody(t, rec, &created)
	if created.ID != testMessageID {
		t.Fatalf("created id = %q, want %q", created.ID, testMessageID)
	}
	if created.Role != "user" {
		t.Fatalf("created role = %q, want user", created.Role)
	}
	if created.Status != "completed" {
		t.Fatalf("created status = %q, want completed", created.Status)
	}
	if created.SequenceNo != 0 {
		t.Fatalf("created sequenceNo = %d, want 0", created.SequenceNo)
	}

	rec = performRequest(handler, http.MethodPost, path, `{"content":"system note"}`)
	assertStatus(t, rec, http.StatusCreated)

	rec = performRequest(handler, http.MethodGet, path, "")
	assertStatus(t, rec, http.StatusOK)

	var listed Page[ChatMessageDTO]
	decodeBody(t, rec, &listed)
	if len(listed.Items) != 2 {
		t.Fatalf("listed messages = %d, want 2; body=%s", len(listed.Items), rec.Body.String())
	}
	if listed.Items[0].Content != "hello" || listed.Items[1].Content != "system note" {
		t.Fatalf("listed message contents = %#v", listed.Items)
	}
}

func TestHandlerStreamsMockAssistantAndPersistsMessages(t *testing.T) {
	repo := newFakeRepository()
	repo.conversations = append(repo.conversations, fakeConversation(testConversationID, "First", 0))
	repo.messages[testConversationID] = append(
		repo.messages[testConversationID],
		fakeMessage(testMessageID, testConversationID, 0, "user", "hello"),
	)
	handler := NewHandler(NewService(repo), WithProvider(NewMockProvider()))

	path := conversationsPath + "/" + testConversationID + "/stream"
	rec := performRequest(
		handler,
		http.MethodPost,
		path,
		`{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"mock-chat"},"idempotencyKey":"stream-key-1"}`,
	)

	assertStreamStatus(t, rec, http.StatusOK)
	body := rec.Body.String()
	for _, want := range []string{
		"event: message.started",
		"event: message.delta",
		"event: usage.updated",
		"event: message.completed",
		`"type":"message.completed"`,
		`"role":"assistant"`,
		`"content":"Mock response: hello"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("stream body missing %q; body=%s", want, body)
		}
	}

	messages := repo.messages[testConversationID]
	if len(messages) != 2 {
		t.Fatalf("persisted messages = %d, want 2; messages=%#v", len(messages), messages)
	}
	if messages[0].Role != "user" || messages[0].Content != "hello" {
		t.Fatalf("user message = %#v, want persisted hello user message", messages[0])
	}
	assistant := messages[1]
	if assistant.Role != "assistant" || assistant.Status != "completed" {
		t.Fatalf("assistant role/status = %s/%s, want assistant/completed", assistant.Role, assistant.Status)
	}
	if assistant.ParentMessageID != messages[0].ID {
		t.Fatalf("assistant parent = %q, want user message id %q", assistant.ParentMessageID, messages[0].ID)
	}
	if assistant.Content != "Mock response: hello" {
		t.Fatalf("assistant content = %q, want mock response", assistant.Content)
	}
	if assistant.ModelProvider != "mock" || assistant.ModelID != "mock-chat" {
		t.Fatalf("assistant model = %s/%s, want mock/mock-chat", assistant.ModelProvider, assistant.ModelID)
	}
}

func TestHandlerStreamsEmptyAssistantContent(t *testing.T) {
	repo := newFakeRepository()
	repo.conversations = append(repo.conversations, fakeConversation(testConversationID, "First", 0))
	repo.messages[testConversationID] = append(
		repo.messages[testConversationID],
		fakeMessage(testMessageID, testConversationID, 0, "user", "hello"),
	)
	handler := NewHandler(NewService(repo), WithProvider(emptyProvider{}))

	rec := performRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		`{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"empty"},"idempotencyKey":"stream-key-empty"}`,
	)

	assertStreamStatus(t, rec, http.StatusOK)
	if body := rec.Body.String(); !strings.Contains(body, `event: message.completed`) || !strings.Contains(body, `"content":""`) {
		t.Fatalf("empty provider stream did not complete with empty content; body=%s", body)
	}
	messages := repo.messages[testConversationID]
	if len(messages) != 2 || messages[1].Status != "completed" || messages[1].Content != "" {
		t.Fatalf("messages after empty provider = %#v", messages)
	}
}

func TestHandlerFinalizesFailedWhenProviderStartupFails(t *testing.T) {
	repo := newFakeRepository()
	repo.conversations = append(repo.conversations, fakeConversation(testConversationID, "First", 0))
	repo.messages[testConversationID] = append(
		repo.messages[testConversationID],
		fakeMessage(testMessageID, testConversationID, 0, "user", "hello"),
	)
	handler := NewHandler(NewService(repo), WithProvider(errorProvider{}))

	rec := performRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		`{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"error"},"idempotencyKey":"stream-key-error"}`,
	)

	assertStatus(t, rec, http.StatusBadGateway)
	assertErrorCode(t, rec, "PROVIDER_ERROR")
	messages := repo.messages[testConversationID]
	if len(messages) != 2 || messages[1].Status != "failed" {
		t.Fatalf("assistant message was not finalized failed after provider startup error: %#v", messages)
	}
}

func TestHandlerRejectsUnsupportedProviderBeforeAssistantPersistence(t *testing.T) {
	repo := newFakeRepository()
	repo.conversations = append(repo.conversations, fakeConversation(testConversationID, "First", 0))
	repo.messages[testConversationID] = append(
		repo.messages[testConversationID],
		fakeMessage(testMessageID, testConversationID, 0, "user", "hello"),
	)
	handler := NewHandler(NewService(repo), WithProvider(rejectingProvider{}))

	rec := performRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		`{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"anthropic","modelId":"claude-test"},"idempotencyKey":"stream-key-unsupported-provider"}`,
	)

	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorCode(t, rec, "UNSUPPORTED_PROVIDER")
	if got := len(repo.messages[testConversationID]); got != 1 {
		t.Fatalf("persisted messages = %d, want only original user message", got)
	}
}

func TestHandlerFinalizesCancelledWhenProviderStartupIsCancelled(t *testing.T) {
	repo := newFakeRepository()
	repo.conversations = append(repo.conversations, fakeConversation(testConversationID, "First", 0))
	repo.messages[testConversationID] = append(
		repo.messages[testConversationID],
		fakeMessage(testMessageID, testConversationID, 0, "user", "hello"),
	)
	handler := NewHandler(NewService(repo), WithProvider(startupCancelledProvider{}))

	_ = performRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		`{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"cancelled"},"idempotencyKey":"stream-key-startup-cancelled"}`,
	)

	messages := repo.messages[testConversationID]
	if len(messages) != 2 {
		t.Fatalf("persisted messages = %d, want user + cancelled assistant; messages=%#v", len(messages), messages)
	}
	if messages[1].Status != "cancelled" {
		t.Fatalf("assistant status = %q, want cancelled; messages=%#v", messages[1].Status, messages)
	}
	if _, ok := messages[1].Metadata["errorCode"]; ok {
		t.Fatalf("cancelled assistant metadata contains errorCode: %#v", messages[1].Metadata)
	}
}

func TestHandlerReturnsProviderRequiredForStreamWhenProviderIsNil(t *testing.T) {
	repo := newFakeRepository()
	repo.conversations = append(repo.conversations, fakeConversation(testConversationID, "First", 0))
	repo.messages[testConversationID] = append(
		repo.messages[testConversationID],
		fakeMessage(testMessageID, testConversationID, 0, "user", "hello"),
	)
	handler := NewHandler(NewService(repo))

	rec := performRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		`{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"mock-chat"},"idempotencyKey":"stream-key-1"}`,
	)

	assertStatus(t, rec, http.StatusServiceUnavailable)
	assertErrorCode(t, rec, "PROVIDER_REQUIRED")
}

func TestHandlerReturnsConflictForStreamIdempotencyReuse(t *testing.T) {
	repo := newFakeRepository()
	repo.conversations = append(repo.conversations, fakeConversation(testConversationID, "First", 0))
	repo.messages[testConversationID] = append(
		repo.messages[testConversationID],
		fakeMessage(testMessageID, testConversationID, 0, "user", "hello"),
	)
	handler := NewHandler(NewService(repo), WithProvider(NewMockProvider()))

	rec := performRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		`{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"mock-chat"},"idempotencyKey":"conflict"}`,
	)

	assertStatus(t, rec, http.StatusConflict)
	assertErrorCode(t, rec, "IDEMPOTENCY_CONFLICT")
}

func TestHandlerReturnsDatabaseRequiredWhenRepositoryIsNil(t *testing.T) {
	handler := NewHandler(NewService(nil))

	rec := performRequest(handler, http.MethodGet, conversationsPath, "")

	assertStatus(t, rec, http.StatusServiceUnavailable)
	assertErrorCode(t, rec, "DATABASE_REQUIRED")

	rec = performRequest(handler, http.MethodPost, conversationsPath, `{"title":`)

	assertStatus(t, rec, http.StatusServiceUnavailable)
	assertErrorCode(t, rec, "DATABASE_REQUIRED")

	rec = performRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/messages",
		`{"content":"hello"}`,
	)

	assertStatus(t, rec, http.StatusServiceUnavailable)
	assertErrorCode(t, rec, "DATABASE_REQUIRED")

	rec = performRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		`{"content":`,
	)

	assertStatus(t, rec, http.StatusServiceUnavailable)
	assertErrorCode(t, rec, "DATABASE_REQUIRED")
}

func TestHandlerRejectsInvalidJSON(t *testing.T) {
	repo := newFakeRepository()
	handler := NewHandler(NewService(repo))

	rec := performRequest(handler, http.MethodPost, conversationsPath, `{"title":`)

	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorCode(t, rec, "INVALID_JSON")

	rec = performRequest(handler, http.MethodPost, conversationsPath, `{} {}`)

	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorCode(t, rec, "INVALID_JSON")
}

func TestHandlerRejectsInvalidMessageInput(t *testing.T) {
	repo := newFakeRepository()
	repo.conversations = append(repo.conversations, fakeConversation(testConversationID, "First", 0))
	handler := NewHandler(NewService(repo))
	path := conversationsPath + "/" + testConversationID + "/messages"

	tests := []struct {
		name     string
		body     string
		wantCode string
	}{
		{name: "assistant role", body: `{"role":"assistant","content":"nope"}`, wantCode: "FORBIDDEN_MESSAGE_FIELD"},
		{name: "tool role", body: `{"role":"tool","content":"nope"}`, wantCode: "FORBIDDEN_MESSAGE_FIELD"},
		{name: "system role", body: `{"role":"system","content":"nope"}`, wantCode: "FORBIDDEN_MESSAGE_FIELD"},
		{name: "empty content", body: `{"content":"   "}`, wantCode: "EMPTY_CONTENT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := performRequest(handler, http.MethodPost, path, tt.body)
			assertStatus(t, rec, http.StatusBadRequest)
			assertErrorCode(t, rec, tt.wantCode)
		})
	}
}

func TestHandlerRejectsForbiddenConversationFields(t *testing.T) {
	repo := newFakeRepository()
	handler := NewHandler(NewService(repo))

	tests := []struct {
		name string
		body string
	}{
		{name: "user id", body: `{"title":"First","userId":"00000000-0000-0000-0000-000000000001"}`},
		{name: "owner id", body: `{"title":"First","ownerId":"00000000-0000-0000-0000-000000000001"}`},
		{name: "session id", body: `{"title":"First","sessionId":"session-1"}`},
		{name: "session", body: `{"title":"First","session":"session-1"}`},
		{name: "bearer token", body: `{"title":"First","bearerToken":"token"}`},
		{name: "access token", body: `{"title":"First","accessToken":"token"}`},
		{name: "authorization", body: `{"title":"First","authorization":"Bearer token"}`},
		{name: "impersonate user id", body: `{"title":"First","impersonateUserId":"00000000-0000-0000-0000-000000000001"}`},
		{name: "status", body: `{"title":"First","status":"deleted"}`},
		{name: "legacy model provider", body: `{"title":"First","modelProvider":"openai"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := performRequest(handler, http.MethodPost, conversationsPath, tt.body)
			assertStatus(t, rec, http.StatusBadRequest)
			assertErrorCode(t, rec, "VALIDATION_ERROR")
		})
	}
}

func TestHandlerRejectsForbiddenMessageFields(t *testing.T) {
	repo := newFakeRepository()
	repo.conversations = append(repo.conversations, fakeConversation(testConversationID, "First", 0))
	handler := NewHandler(NewService(repo))
	path := conversationsPath + "/" + testConversationID + "/messages"

	tests := []struct {
		name string
		body string
	}{
		{name: "owner id", body: `{"content":"hello","ownerId":"00000000-0000-0000-0000-000000000001"}`},
		{name: "session id", body: `{"content":"hello","sessionId":"session-1"}`},
		{name: "session", body: `{"content":"hello","session":"session-1"}`},
		{name: "bearer token", body: `{"content":"hello","bearerToken":"token"}`},
		{name: "access token", body: `{"content":"hello","accessToken":"token"}`},
		{name: "authorization", body: `{"content":"hello","authorization":"Bearer token"}`},
		{name: "impersonate user id", body: `{"content":"hello","impersonateUserId":"00000000-0000-0000-0000-000000000001"}`},
		{name: "status", body: `{"content":"hello","status":"streaming"}`},
		{name: "output blocks", body: `{"content":"hello","outputBlocks":[]}`},
		{name: "model ref", body: `{"content":"hello","modelRef":{"providerId":"openai","modelId":"gpt-test"}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := performRequest(handler, http.MethodPost, path, tt.body)
			assertStatus(t, rec, http.StatusBadRequest)
			assertErrorCode(t, rec, "FORBIDDEN_MESSAGE_FIELD")
		})
	}
}

func TestHandlerRejectsForbiddenStreamFields(t *testing.T) {
	repo := newFakeRepository()
	repo.conversations = append(repo.conversations, fakeConversation(testConversationID, "First", 0))
	handler := NewHandler(NewService(repo), WithProvider(NewMockProvider()))
	path := conversationsPath + "/" + testConversationID + "/stream"

	tests := []struct {
		name     string
		body     string
		wantCode string
	}{
		{name: "owner id", body: `{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"mock-chat"},"idempotencyKey":"stream-key-1","ownerId":"00000000-0000-0000-0000-000000000001"}`, wantCode: "FORBIDDEN_MESSAGE_FIELD"},
		{name: "role", body: `{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"mock-chat"},"idempotencyKey":"stream-key-1","role":"assistant"}`, wantCode: "FORBIDDEN_MESSAGE_FIELD"},
		{name: "content unsupported", body: `{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"mock-chat"},"idempotencyKey":"stream-key-1","content":"hello"}`, wantCode: "VALIDATION_ERROR"},
		{name: "status", body: `{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"mock-chat"},"idempotencyKey":"stream-key-1","status":"streaming"}`, wantCode: "FORBIDDEN_MESSAGE_FIELD"},
		{name: "invalid user message id", body: `{"userMessageId":"not-a-uuid","modelRef":{"providerId":"mock","modelId":"mock-chat"},"idempotencyKey":"stream-key-1"}`, wantCode: "INVALID_USER_MESSAGE_ID"},
		{name: "missing model ref", body: `{"userMessageId":"22222222-2222-4222-8222-222222222222","idempotencyKey":"stream-key-1"}`, wantCode: "MODEL_REF_REQUIRED"},
		{name: "missing idempotency key", body: `{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"mock-chat"}}`, wantCode: "IDEMPOTENCY_KEY_REQUIRED"},
		{name: "attachments unsupported", body: `{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"mock-chat"},"idempotencyKey":"stream-key-1","attachments":[]}`, wantCode: "VALIDATION_ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := performRequest(handler, http.MethodPost, path, tt.body)
			assertStatus(t, rec, http.StatusBadRequest)
			assertErrorCode(t, rec, tt.wantCode)
		})
	}
}

func TestHandlerReturnsNotFoundForUnknownConversation(t *testing.T) {
	repo := newFakeRepository()
	handler := NewHandler(NewService(repo))
	path := conversationsPath + "/" + testConversationID + "/messages"

	rec := performRequest(handler, http.MethodGet, path, "")

	assertStatus(t, rec, http.StatusNotFound)
	assertErrorCode(t, rec, "CONVERSATION_NOT_FOUND")
}

func TestHandlerReturnsConflictForIdempotencyReuse(t *testing.T) {
	repo := newFakeRepository()
	handler := NewHandler(NewService(repo))

	rec := performRequest(
		handler,
		http.MethodPost,
		conversationsPath,
		`{"title":"First","idempotencyKey":"conflict"}`,
	)

	assertStatus(t, rec, http.StatusConflict)
	assertErrorCode(t, rec, "IDEMPOTENCY_CONFLICT")
}

func TestHandlerRejectsUnsupportedMethods(t *testing.T) {
	repo := newFakeRepository()
	handler := NewHandler(NewService(repo))

	rec := performRequest(handler, http.MethodDelete, conversationsPath, "")
	assertStatus(t, rec, http.StatusMethodNotAllowed)
	if got := rec.Header().Get("Allow"); got != "GET, POST" {
		t.Fatalf("Allow = %q, want %q", got, "GET, POST")
	}
	assertErrorCode(t, rec, "METHOD_NOT_ALLOWED")

	path := conversationsPath + "/" + testConversationID + "/messages"
	rec = performRequest(handler, http.MethodPatch, path, "")
	assertStatus(t, rec, http.StatusMethodNotAllowed)
	if got := rec.Header().Get("Allow"); got != "GET, POST" {
		t.Fatalf("Allow = %q, want %q", got, "GET, POST")
	}
	assertErrorCode(t, rec, "METHOD_NOT_ALLOWED")

	path = conversationsPath + "/" + testConversationID + "/stream"
	rec = performRequest(handler, http.MethodGet, path, "")
	assertStatus(t, rec, http.StatusMethodNotAllowed)
	if got := rec.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("Allow = %q, want %q", got, http.MethodPost)
	}
	assertErrorCode(t, rec, "METHOD_NOT_ALLOWED")
}

func TestHandlerRejectsInvalidConversationID(t *testing.T) {
	repo := newFakeRepository()
	handler := NewHandler(NewService(repo))

	rec := performRequest(handler, http.MethodGet, conversationsPath+"/not-a-uuid/messages", "")

	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorCode(t, rec, "INVALID_CONVERSATION_ID")
}

func performRequest(handler http.Handler, method string, path string, body string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	handler.ServeHTTP(rec, req)
	return rec
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, want, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
}

func assertStreamStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, want, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", contentType)
	}
}

func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	var body ErrorResponse
	decodeBody(t, rec, &body)
	if body.Error.Code != want {
		t.Fatalf("error code = %q, want %q; body=%+v", body.Error.Code, want, body)
	}
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(destination); err != nil {
		t.Fatalf("decode response body: %v; body=%s", err, rec.Body.String())
	}
}

type fakeRepository struct {
	conversations []Conversation
	messages      map[string][]Message
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{messages: map[string][]Message{}}
}

func (f *fakeRepository) CreateConversation(
	_ context.Context,
	input CreateConversationInput,
) (Conversation, error) {
	if input.IdempotencyKey == "conflict" {
		return Conversation{}, ErrIdempotencyConflict
	}
	conversation := fakeConversation(testConversationID, input.Title, 0)
	conversation.ModelProvider = input.ModelProvider
	conversation.ModelID = input.ModelID
	conversation.SystemPrompt = input.SystemPrompt
	conversation.Metadata = input.Metadata
	f.conversations = append(f.conversations, conversation)
	return conversation, nil
}

func (f *fakeRepository) ListConversations(context.Context) ([]Conversation, error) {
	items := make([]Conversation, len(f.conversations))
	copy(items, f.conversations)
	return items, nil
}

func (f *fakeRepository) ListMessages(_ context.Context, conversationID string) ([]Message, error) {
	if !f.hasConversation(conversationID) {
		return nil, ErrConversationNotFound
	}
	items := make([]Message, len(f.messages[conversationID]))
	copy(items, f.messages[conversationID])
	return items, nil
}

func (f *fakeRepository) GetMessage(
	_ context.Context,
	conversationID string,
	messageID string,
) (Message, error) {
	if !f.hasConversation(conversationID) {
		return Message{}, ErrConversationNotFound
	}
	for _, message := range f.messages[conversationID] {
		if message.ID == messageID {
			return message, nil
		}
	}

	return Message{}, newValidationError("INVALID_USER_MESSAGE_ID", "user message not found")
}

func (f *fakeRepository) CreateMessage(
	_ context.Context,
	conversationID string,
	input CreateMessageInput,
) (Message, error) {
	if !f.hasConversation(conversationID) {
		return Message{}, ErrConversationNotFound
	}
	if input.IdempotencyKey == "conflict" {
		return Message{}, ErrIdempotencyConflict
	}
	messages := f.messages[conversationID]
	message := fakeMessage(testMessageID, conversationID, len(messages), input.Role, input.Content)
	message.ParentMessageID = input.ParentMessageID
	message.Metadata = input.Metadata
	f.messages[conversationID] = append(messages, message)
	for i := range f.conversations {
		if f.conversations[i].ID == conversationID {
			f.conversations[i].MessageCount = len(f.messages[conversationID])
		}
	}
	return message, nil
}

func (f *fakeRepository) CreateAssistantMessage(
	_ context.Context,
	conversationID string,
	input CreateAssistantMessageInput,
) (Message, error) {
	if !f.hasConversation(conversationID) {
		return Message{}, ErrConversationNotFound
	}
	if input.IdempotencyKey == "conflict" {
		return Message{}, ErrIdempotencyConflict
	}
	messages := f.messages[conversationID]
	id := input.ID
	if id == "" {
		id = "33333333-3333-4333-8333-333333333333"
	}
	message := fakeMessage(id, conversationID, len(messages), "assistant", "")
	message.ParentMessageID = input.ParentMessageID
	message.ModelProvider = input.ModelProvider
	message.ModelID = input.ModelID
	message.ProviderMessageID = input.ProviderMessageID
	message.Status = "streaming"
	message.Content = ""
	message.Metadata = input.Metadata
	message.IdempotencyKey = input.IdempotencyKey
	f.messages[conversationID] = append(messages, message)
	for i := range f.conversations {
		if f.conversations[i].ID == conversationID {
			f.conversations[i].MessageCount = len(f.messages[conversationID])
		}
	}
	return message, nil
}

func (f *fakeRepository) FinalizeAssistantMessage(
	_ context.Context,
	conversationID string,
	messageID string,
	input FinalizeAssistantMessageInput,
) (Message, error) {
	if !f.hasConversation(conversationID) {
		return Message{}, ErrConversationNotFound
	}
	for i := range f.messages[conversationID] {
		message := &f.messages[conversationID][i]
		if message.ID != messageID {
			continue
		}
		message.Status = input.Status
		message.Content = input.Content
		message.OutputBlocks = input.OutputBlocks
		message.Metadata = input.Metadata
		completedAt := testNow()
		message.CompletedAt = &completedAt
		message.UpdatedAt = completedAt
		return *message, nil
	}

	return Message{}, newValidationError("INVALID_MESSAGE_ID", "assistant message not found")
}

func (f *fakeRepository) hasConversation(conversationID string) bool {
	for _, conversation := range f.conversations {
		if conversation.ID == conversationID {
			return true
		}
	}
	return false
}

func fakeConversation(id string, title string, messageCount int) Conversation {
	now := testNow()
	return Conversation{
		ID:           id,
		UserID:       DevUserID,
		Title:        title,
		Status:       "active",
		Metadata:     map[string]any{},
		MessageCount: messageCount,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func fakeMessage(id string, conversationID string, sequenceNo int, role string, content string) Message {
	now := testNow()
	return Message{
		ID:             id,
		ConversationID: conversationID,
		UserID:         DevUserID,
		SequenceNo:     sequenceNo,
		Role:           role,
		Status:         "completed",
		Content:        content,
		OutputBlocks:   []any{},
		Metadata:       map[string]any{},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func testNow() time.Time {
	return time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
}

type emptyProvider struct{}

func (p emptyProvider) StreamChat(context.Context, ProviderRequest) (<-chan ProviderEvent, error) {
	ch := make(chan ProviderEvent)
	close(ch)
	return ch, nil
}

type errorProvider struct{}

func (p errorProvider) StreamChat(context.Context, ProviderRequest) (<-chan ProviderEvent, error) {
	return nil, errors.New("provider startup failed")
}

type rejectingProvider struct{}

func (p rejectingProvider) ResolveModelRef(ModelRef) (ModelRef, error) {
	return ModelRef{}, ValidationError{
		Code:    "UNSUPPORTED_PROVIDER",
		Message: "modelRef.providerId is not supported by the configured provider",
	}
}

func (p rejectingProvider) StreamChat(context.Context, ProviderRequest) (<-chan ProviderEvent, error) {
	panic("StreamChat should not be called after modelRef validation fails")
}

type startupCancelledProvider struct{}

func (p startupCancelledProvider) StreamChat(context.Context, ProviderRequest) (<-chan ProviderEvent, error) {
	return nil, context.Canceled
}
