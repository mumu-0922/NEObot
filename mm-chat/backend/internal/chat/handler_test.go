package chat

import (
	"bytes"
	"context"
	"encoding/json"
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
	handler := NewHandler(NewService(repo))

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

func (f *fakeRepository) CreateMessage(
	_ context.Context,
	conversationID string,
	input CreateMessageInput,
) (Message, error) {
	if !f.hasConversation(conversationID) {
		return Message{}, ErrConversationNotFound
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
