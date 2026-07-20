package chat

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/auth"
	filemeta "neo-chat/mm-chat/backend/internal/files"
	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

const testSHA256 = "b94d27b9934d3e08a52e52d7da7dabfadebca7838dfb27f4f9174e65a2f27f21"

func TestPostgresCreateMessagePersistsAttachments(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	fileRepo := filemeta.NewPostgresRepository(db)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conversation, err := repo.CreateConversation(ctx, CreateConversationInput{
		Title: "attachments",
	})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	fileID := mustTestUUID(t)
	fileRecord, err := fileRepo.CreateFile(ctx, filemeta.CreateFileInput{
		ID:               fileID,
		OriginalFilename: "hello.txt",
		MimeType:         "text/plain",
		ByteSize:         11,
		SHA256:           testSHA256,
		StorageBackend:   "local",
		ObjectKey:        "users/" + filemeta.DevUserID + "/files/" + fileID,
		Metadata:         map[string]any{"purpose": "chat"},
	})
	if err != nil {
		t.Fatalf("CreateFile() error = %v", err)
	}

	message, err := repo.CreateMessage(ctx, conversation.ID, CreateMessageInput{
		Role:    "user",
		Content: "with file",
		Attachments: []AttachmentInput{
			{FileID: fileRecord.ID, Purpose: "image"},
		},
	})
	if err != nil {
		t.Fatalf("CreateMessage() error = %v", err)
	}
	if len(message.Attachments) != 1 {
		t.Fatalf("message attachments = %#v, want one", message.Attachments)
	}
	attachment := message.Attachments[0]
	if attachment.FileID != fileRecord.ID || attachment.FileName != "hello.txt" || attachment.Purpose != "image" {
		t.Fatalf("created attachment = %#v", attachment)
	}

	var linkCount int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM message_attachments WHERE message_id = $1 AND file_id = $2 AND purpose = 'image'`,
		message.ID,
		fileRecord.ID,
	).Scan(&linkCount); err != nil {
		t.Fatalf("query message attachment link: %v", err)
	}
	if linkCount != 1 {
		t.Fatalf("message attachment rows = %d, want 1", linkCount)
	}

	listed, err := repo.ListMessages(ctx, conversation.ID)
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	if len(listed) != 1 || len(listed[0].Attachments) != 1 {
		t.Fatalf("listed messages = %#v, want attachment", listed)
	}
	got, err := repo.GetMessage(ctx, conversation.ID, message.ID)
	if err != nil {
		t.Fatalf("GetMessage() error = %v", err)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].SHA256 != testSHA256 {
		t.Fatalf("GetMessage() attachments = %#v", got.Attachments)
	}
}

func TestPostgresConversationContextSummaryRoundTrip(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	service := NewService(repo)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conversation, err := repo.CreateConversation(ctx, CreateConversationInput{
		Title: "context summary",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := repo.CreateMessage(ctx, conversation.ID, CreateMessageInput{
		Role: "user", Content: "remember cobalt",
	})
	if err != nil {
		t.Fatal(err)
	}
	last, err := repo.CreateAssistantMessage(ctx, conversation.ID, CreateAssistantMessageInput{
		ID: mustTestUUID(t), ParentMessageID: first.ID,
		ModelProvider: "mock", ModelID: "summary",
		IdempotencyKey: "context-summary-assistant-" + mustTestUUID(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FinalizeAssistantMessage(ctx, conversation.ID, last.ID, FinalizeAssistantMessageInput{
		Status: "completed", Content: "noted",
	}); err != nil {
		t.Fatal(err)
	}

	input := UpsertConversationContextSummaryInput{
		ModelProvider: "mock", ModelID: "summary",
		SourceFirstMessageID: first.ID, SourceLastMessageID: last.ID,
		SourceMessageCount: 2, SourceDigest: strings.Repeat("a", 64),
		Summary:               "The user asked to remember cobalt.",
		EstimatedSourceTokens: 12, EstimatedSummaryTokens: 8,
	}
	created, err := service.UpsertConversationContextSummary(ctx, conversation.ID, input)
	if err != nil {
		t.Fatalf("UpsertConversationContextSummary() error = %v", err)
	}
	if created.Version != 1 || created.SourceLastMessageID != last.ID {
		t.Fatalf("created summary = %#v", created)
	}
	loaded, found, err := service.GetConversationContextSummary(ctx, conversation.ID)
	if err != nil || !found || loaded.Summary != input.Summary {
		t.Fatalf("loaded summary = %#v, found=%v, err=%v", loaded, found, err)
	}

	input.Summary = "The user asked to remember cobalt and chose option B."
	updated, err := service.UpsertConversationContextSummary(ctx, conversation.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.Summary != input.Summary || updated.CreatedAt != created.CreatedAt {
		t.Fatalf("updated summary = %#v", updated)
	}

	otherConversation, err := repo.CreateConversation(ctx, CreateConversationInput{Title: "other"})
	if err != nil {
		t.Fatal(err)
	}
	otherMessage, err := repo.CreateMessage(ctx, otherConversation.ID, CreateMessageInput{
		Role: "user", Content: "foreign",
	})
	if err != nil {
		t.Fatal(err)
	}
	input.SourceLastMessageID = otherMessage.ID
	if _, err := service.UpsertConversationContextSummary(ctx, conversation.ID, input); err == nil {
		t.Fatal("cross-conversation summary boundary was accepted")
	}
}

func TestPostgresUpdateAndDeleteConversation(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conversation, err := repo.CreateConversation(ctx, CreateConversationInput{
		Title:        "original",
		SystemPrompt: "old prompt",
		Metadata:     map[string]any{"useSearch": true},
	})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}

	title := "renamed"
	systemPrompt := "new prompt"
	updated, err := repo.UpdateConversation(ctx, conversation.ID, UpdateConversationInput{
		Title:         &title,
		SystemPrompt:  &systemPrompt,
		MetadataMerge: map[string]any{"pinned": true, "useReasoning": true},
	})
	if err != nil {
		t.Fatalf("UpdateConversation() error = %v", err)
	}
	if updated.Title != "renamed" || updated.SystemPrompt != "new prompt" {
		t.Fatalf("updated conversation = %#v", updated)
	}
	if updated.Metadata["useSearch"] != true || updated.Metadata["useReasoning"] != true || updated.Metadata["pinned"] != true {
		t.Fatalf("updated metadata = %#v", updated.Metadata)
	}
	fetched, err := repo.GetConversation(ctx, conversation.ID)
	if err != nil {
		t.Fatalf("GetConversation() error = %v", err)
	}
	if fetched.ID != conversation.ID || fetched.Title != "renamed" {
		t.Fatalf("fetched conversation = %#v", fetched)
	}

	if err := repo.DeleteConversation(ctx, conversation.ID); err != nil {
		t.Fatalf("DeleteConversation() error = %v", err)
	}
	listed, err := repo.ListConversations(ctx)
	if err != nil {
		t.Fatalf("ListConversations() error = %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("listed conversations after delete = %d, want 0", len(listed))
	}
	if _, err := repo.UpdateConversation(ctx, conversation.ID, UpdateConversationInput{Title: &title}); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("UpdateConversation(deleted) error = %v, want ErrConversationNotFound", err)
	}
	if _, err := repo.GetConversation(ctx, conversation.ID); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("GetConversation(deleted) error = %v, want ErrConversationNotFound", err)
	}
}

func TestPostgresDuplicateConversationCopiesMessagesAndAttachments(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	fileRepo := filemeta.NewPostgresRepository(db)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conversation, err := repo.CreateConversation(ctx, CreateConversationInput{
		Title:         "source",
		ModelProvider: "openai",
		ModelID:       "gpt-test",
		SystemPrompt:  "be precise",
		Metadata:      map[string]any{"pinned": true, "useReasoning": true},
	})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	fileID := mustTestUUID(t)
	fileRecord, err := fileRepo.CreateFile(ctx, filemeta.CreateFileInput{
		ID:               fileID,
		OriginalFilename: "duplicate.txt",
		MimeType:         "text/plain",
		ByteSize:         9,
		SHA256:           testSHA256,
		StorageBackend:   "local",
		ObjectKey:        "users/" + filemeta.DevUserID + "/files/" + fileID,
		Metadata:         map[string]any{"purpose": "chat"},
	})
	if err != nil {
		t.Fatalf("CreateFile() error = %v", err)
	}
	userMessage, err := repo.CreateMessage(ctx, conversation.ID, CreateMessageInput{
		Role:    "user",
		Content: "prompt",
		Metadata: map[string]any{
			"treeParentMessageId": nil,
		},
		Attachments: []AttachmentInput{
			{FileID: fileRecord.ID, Purpose: "input"},
		},
	})
	if err != nil {
		t.Fatalf("CreateMessage() error = %v", err)
	}
	assistant, err := repo.CreateAssistantMessage(ctx, conversation.ID, CreateAssistantMessageInput{
		ID:              mustTestUUID(t),
		ParentMessageID: userMessage.ID,
		ModelProvider:   "openai",
		ModelID:         "gpt-test",
		IdempotencyKey:  "duplicate-assistant-" + mustTestUUID(t),
	})
	if err != nil {
		t.Fatalf("CreateAssistantMessage() error = %v", err)
	}
	if _, err := repo.FinalizeAssistantMessage(ctx, conversation.ID, assistant.ID, FinalizeAssistantMessageInput{
		Status:  "completed",
		Content: "answer [W1]",
		OutputBlocks: []any{map[string]any{
			"id": "web-sources", "type": "search", "isSearching": false,
			"sources": []any{map[string]any{
				"title": "Fixture", "url": "https://example.test/source", "content": "fresh",
				"metadata": map[string]any{"marker": "[W1]"},
			}},
			"images": []any{},
		}},
		Metadata: map[string]any{"runId": mustTestUUID(t)},
	}); err != nil {
		t.Fatalf("FinalizeAssistantMessage() error = %v", err)
	}

	duplicated, err := repo.DuplicateConversation(ctx, conversation.ID, DuplicateConversationInput{
		IdempotencyKey: "duplicate-" + mustTestUUID(t),
	})
	if err != nil {
		t.Fatalf("DuplicateConversation() error = %v", err)
	}
	if duplicated.ID == conversation.ID || duplicated.Title != "source (Copy)" {
		t.Fatalf("duplicated conversation = %#v", duplicated)
	}
	if duplicated.SystemPrompt != "be precise" || duplicated.ModelProvider != "openai" || duplicated.ModelID != "gpt-test" {
		t.Fatalf("duplicated model/prompt = %#v", duplicated)
	}
	if duplicated.Metadata["useReasoning"] != true || duplicated.Metadata["pinned"] != false {
		t.Fatalf("duplicated metadata = %#v", duplicated.Metadata)
	}
	if duplicated.MessageCount != 2 {
		t.Fatalf("duplicated messageCount = %d, want 2", duplicated.MessageCount)
	}

	messages, err := repo.ListMessages(ctx, duplicated.ID)
	if err != nil {
		t.Fatalf("ListMessages(duplicated) error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("duplicated messages = %d, want 2", len(messages))
	}
	if messages[0].ID == userMessage.ID || messages[0].ConversationID != duplicated.ID || messages[0].SequenceNo != 0 {
		t.Fatalf("duplicated user message = %#v", messages[0])
	}
	if len(messages[0].Attachments) != 1 || messages[0].Attachments[0].FileID != fileRecord.ID {
		t.Fatalf("duplicated attachments = %#v", messages[0].Attachments)
	}
	if messages[1].ParentMessageID != messages[0].ID || messages[1].Content != "answer [W1]" || len(messages[1].OutputBlocks) != 1 {
		t.Fatalf("duplicated assistant message = %#v", messages[1])
	}
	block, ok := messages[1].OutputBlocks[0].(map[string]any)
	sources, sourcesOK := block["sources"].([]any)
	if !ok || block["type"] != "search" || !sourcesOK || len(sources) != 1 {
		t.Fatalf("duplicated Search output block = %#v", messages[1].OutputBlocks)
	}
	source, sourceOK := sources[0].(map[string]any)
	metadata, metadataOK := source["metadata"].(map[string]any)
	if !sourceOK || !metadataOK || metadata["marker"] != "[W1]" {
		t.Fatalf("duplicated Search source = %#v", sources[0])
	}
}

func TestPostgresUpdateMessageContent(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conversation, err := repo.CreateConversation(ctx, CreateConversationInput{
		Title: "message edit",
	})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	userMessage, err := repo.CreateMessage(ctx, conversation.ID, CreateMessageInput{
		Role:    "user",
		Content: "prompt",
	})
	if err != nil {
		t.Fatalf("CreateMessage() error = %v", err)
	}
	assistant, err := repo.CreateAssistantMessage(ctx, conversation.ID, CreateAssistantMessageInput{
		ID:              mustTestUUID(t),
		ParentMessageID: userMessage.ID,
		IdempotencyKey:  "assistant-edit-" + mustTestUUID(t),
		Metadata:        map[string]any{"runId": mustTestUUID(t)},
	})
	if err != nil {
		t.Fatalf("CreateAssistantMessage() error = %v", err)
	}
	assistant, err = repo.FinalizeAssistantMessage(ctx, conversation.ID, assistant.ID, FinalizeAssistantMessageInput{
		Status:  "completed",
		Content: "old answer",
		OutputBlocks: []any{
			map[string]any{"id": "old-block", "type": "text", "content": "old answer"},
		},
		Metadata: assistant.Metadata,
	})
	if err != nil {
		t.Fatalf("FinalizeAssistantMessage() error = %v", err)
	}

	content := " edited answer "
	updated, err := repo.UpdateMessage(ctx, conversation.ID, assistant.ID, UpdateMessageInput{
		Content: &content,
	})
	if err != nil {
		t.Fatalf("UpdateMessage() error = %v", err)
	}
	if updated.Content != "edited answer" {
		t.Fatalf("updated content = %q, want edited answer", updated.Content)
	}
	if len(updated.OutputBlocks) != 0 {
		t.Fatalf("updated output blocks = %#v, want cleared", updated.OutputBlocks)
	}
	if updated.ParentMessageID != userMessage.ID {
		t.Fatalf("updated parent = %q, want %q", updated.ParentMessageID, userMessage.ID)
	}

	listed, err := repo.ListConversations(ctx)
	if err != nil {
		t.Fatalf("ListConversations() error = %v", err)
	}
	if len(listed) != 1 || listed[0].MessageCount != 2 {
		t.Fatalf("conversation message count after edit = %#v, want 2", listed)
	}

	if err := repo.DeleteMessage(ctx, conversation.ID, assistant.ID, DeleteMessageInput{}); err != nil {
		t.Fatalf("DeleteMessage() error = %v", err)
	}
	content = "cannot update"
	if _, err := repo.UpdateMessage(ctx, conversation.ID, assistant.ID, UpdateMessageInput{Content: &content}); !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("UpdateMessage(deleted) error = %v, want ErrMessageNotFound", err)
	}
}

func TestPostgresRepositoryEnforcesTwoUserIsolation(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	fileRepo := filemeta.NewPostgresRepository(db)

	userAID := mustTestUUID(t)
	userBID := mustTestUUID(t)
	sharedConversationKey := "shared-conversation-key-" + mustTestUUID(t)
	baseA := auth.WithUser(context.Background(), auth.User{
		ID:          userAID,
		DisplayName: "User A",
	})
	ctxA, cancelA := context.WithTimeout(baseA, 5*time.Second)
	defer cancelA()
	baseB := auth.WithUser(context.Background(), auth.User{
		ID:          userBID,
		DisplayName: "User B",
	})
	ctxB, cancelB := context.WithTimeout(baseB, 5*time.Second)
	defer cancelB()

	conversationA, err := repo.CreateConversation(ctxA, CreateConversationInput{
		Title:          "user A conversation",
		IdempotencyKey: sharedConversationKey,
	})
	if err != nil {
		t.Fatalf("CreateConversation(user A) error = %v", err)
	}
	initialB, err := repo.ListConversations(ctxB)
	if err != nil {
		t.Fatalf("ListConversations(user B initial) error = %v", err)
	}
	if len(initialB) != 0 {
		t.Fatalf("user B conversations = %#v, want no user A rows", initialB)
	}
	conversationB, err := repo.CreateConversation(ctxB, CreateConversationInput{
		Title:          "user B conversation",
		IdempotencyKey: sharedConversationKey,
	})
	if err != nil {
		t.Fatalf("CreateConversation(user B same idempotency key) error = %v", err)
	}
	if conversationA.ID == conversationB.ID || conversationA.UserID == conversationB.UserID {
		t.Fatalf("conversations were not isolated: %#v/%#v", conversationA, conversationB)
	}
	if conversationA.IdempotencyKey != sharedConversationKey || conversationB.IdempotencyKey != sharedConversationKey {
		t.Fatalf("conversation idempotency keys = %q/%q, want shared key %q", conversationA.IdempotencyKey, conversationB.IdempotencyKey, sharedConversationKey)
	}

	fileID := mustTestUUID(t)
	fileRecord, err := fileRepo.CreateFile(ctxA, filemeta.CreateFileInput{
		ID:               fileID,
		OriginalFilename: "a-only.txt",
		MimeType:         "text/plain",
		ByteSize:         11,
		SHA256:           testSHA256,
		StorageBackend:   "local",
		ObjectKey:        "users/" + conversationA.UserID + "/files/" + fileID,
		Metadata:         map[string]any{"purpose": "chat"},
	})
	if err != nil {
		t.Fatalf("CreateFile(user A) error = %v", err)
	}
	messageA, err := repo.CreateMessage(ctxA, conversationA.ID, CreateMessageInput{
		Role:    "user",
		Content: "user A message",
		Attachments: []AttachmentInput{
			{FileID: fileRecord.ID, Purpose: "input"},
		},
	})
	if err != nil {
		t.Fatalf("CreateMessage(user A) error = %v", err)
	}
	runID := mustTestUUID(t)
	assistantA, err := repo.CreateAssistantMessage(ctxA, conversationA.ID, CreateAssistantMessageInput{
		ID:              mustTestUUID(t),
		ParentMessageID: messageA.ID,
		IdempotencyKey:  "assistant-" + runID,
		Metadata: map[string]any{
			"runId": runID,
		},
	})
	if err != nil {
		t.Fatalf("CreateAssistantMessage(user A) error = %v", err)
	}

	listB, err := repo.ListConversations(ctxB)
	if err != nil {
		t.Fatalf("ListConversations(user B) error = %v", err)
	}
	if len(listB) != 1 || listB[0].ID != conversationB.ID {
		t.Fatalf("user B conversations = %#v, want only user B conversation", listB)
	}
	if _, err := repo.ListMessages(ctxB, conversationA.ID); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("ListMessages(user B on user A conversation) error = %v, want ErrConversationNotFound", err)
	}
	if _, err := repo.GetMessage(ctxB, conversationA.ID, messageA.ID); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("GetMessage(user B on user A conversation) error = %v, want ErrConversationNotFound", err)
	}
	if _, err := repo.CreateMessage(ctxB, conversationA.ID, CreateMessageInput{
		Role:    "user",
		Content: "cross-user write",
	}); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("CreateMessage(user B on user A conversation) error = %v, want ErrConversationNotFound", err)
	}
	if _, err := repo.CreateMessage(ctxB, conversationB.ID, CreateMessageInput{
		Role:    "user",
		Content: "cross-user attachment",
		Attachments: []AttachmentInput{
			{FileID: fileRecord.ID, Purpose: "input"},
		},
	}); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("CreateMessage(user B with user A file) error = %v, want ErrFileNotFound", err)
	}
	assertNoMessagesForConversation(t, ctxB, db, conversationB.ID)
	if _, err := repo.FinalizeAssistantMessage(ctxB, conversationA.ID, assistantA.ID, FinalizeAssistantMessageInput{
		Status:  "completed",
		Content: "cross-user finalize",
	}); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("FinalizeAssistantMessage(user B on user A assistant) error = %v, want ErrConversationNotFound", err)
	}
	if _, err := repo.CancelRun(ctxB, runID, CancelRunInput{Metadata: map[string]any{"cancelledBy": "user-b"}}); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("CancelRun(user B on user A run) error = %v, want ErrRunNotFound", err)
	}

	messagesA, err := repo.ListMessages(ctxA, conversationA.ID)
	if err != nil {
		t.Fatalf("ListMessages(user A after cross-user attempts) error = %v", err)
	}
	if len(messagesA) != 2 || len(messagesA[0].Attachments) != 1 || messagesA[1].Status != "streaming" {
		t.Fatalf("user A messages after cross-user attempts = %#v", messagesA)
	}
}

func TestPostgresDeleteMessageAndSubsequentMessages(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conversation, err := repo.CreateConversation(ctx, CreateConversationInput{Title: "delete messages"})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	first, err := repo.CreateMessage(ctx, conversation.ID, CreateMessageInput{Role: "user", Content: "one"})
	if err != nil {
		t.Fatalf("CreateMessage(first) error = %v", err)
	}
	second, err := repo.CreateMessage(ctx, conversation.ID, CreateMessageInput{Role: "user", Content: "two"})
	if err != nil {
		t.Fatalf("CreateMessage(second) error = %v", err)
	}
	third, err := repo.CreateMessage(ctx, conversation.ID, CreateMessageInput{Role: "user", Content: "three"})
	if err != nil {
		t.Fatalf("CreateMessage(third) error = %v", err)
	}

	if err := repo.DeleteMessage(ctx, conversation.ID, second.ID, DeleteMessageInput{}); err != nil {
		t.Fatalf("DeleteMessage(single) error = %v", err)
	}
	listed, err := repo.ListMessages(ctx, conversation.ID)
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	if gotIDs := messageIDs(listed); strings.Join(gotIDs, ",") != first.ID+","+third.ID {
		t.Fatalf("messages after single delete = %v", gotIDs)
	}

	if err := repo.DeleteMessage(ctx, conversation.ID, first.ID, DeleteMessageInput{DeleteSubsequent: true}); err != nil {
		t.Fatalf("DeleteMessage(subsequent) error = %v", err)
	}
	listed, err = repo.ListMessages(ctx, conversation.ID)
	if err != nil {
		t.Fatalf("ListMessages(after retract) error = %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("messages after retract = %d, want 0", len(listed))
	}
	if err := repo.DeleteMessage(ctx, conversation.ID, first.ID, DeleteMessageInput{}); !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("DeleteMessage(deleted) error = %v, want ErrMessageNotFound", err)
	}
}

func TestPostgresCreateMessageRejectsMissingOrDeletedAttachment(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	fileRepo := filemeta.NewPostgresRepository(db)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conversation, err := repo.CreateConversation(ctx, CreateConversationInput{
		Title: "missing attachments",
	})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}

	if _, err := repo.CreateMessage(ctx, conversation.ID, CreateMessageInput{
		Role:    "user",
		Content: "missing file",
		Attachments: []AttachmentInput{
			{FileID: mustTestUUID(t), Purpose: "input"},
		},
	}); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("CreateMessage() missing file error = %v, want ErrFileNotFound", err)
	}
	assertNoMessagesForConversation(t, ctx, db, conversation.ID)

	fileID := mustTestUUID(t)
	fileRecord, err := fileRepo.CreateFile(ctx, filemeta.CreateFileInput{
		ID:               fileID,
		OriginalFilename: "deleted.txt",
		MimeType:         "text/plain",
		ByteSize:         11,
		SHA256:           testSHA256,
		StorageBackend:   "local",
		ObjectKey:        "users/" + filemeta.DevUserID + "/files/" + fileID,
		Metadata:         map[string]any{"purpose": "chat"},
	})
	if err != nil {
		t.Fatalf("CreateFile() error = %v", err)
	}
	if _, err := fileRepo.MarkFileDeleted(ctx, fileRecord.ID); err != nil {
		t.Fatalf("MarkFileDeleted() error = %v", err)
	}

	if _, err := repo.CreateMessage(ctx, conversation.ID, CreateMessageInput{
		Role:    "user",
		Content: "deleted file",
		Attachments: []AttachmentInput{
			{FileID: fileRecord.ID, Purpose: "input"},
		},
	}); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("CreateMessage() deleted file error = %v, want ErrFileNotFound", err)
	}
	assertNoMessagesForConversation(t, ctx, db, conversation.ID)
}

func TestPostgresCreateMessageRollsBackWhenLaterAttachmentIsMissing(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	fileRepo := filemeta.NewPostgresRepository(db)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conversation, err := repo.CreateConversation(ctx, CreateConversationInput{
		Title: "attachment rollback",
	})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	fileID := mustTestUUID(t)
	fileRecord, err := fileRepo.CreateFile(ctx, filemeta.CreateFileInput{
		ID:               fileID,
		OriginalFilename: "kept.txt",
		MimeType:         "text/plain",
		ByteSize:         11,
		SHA256:           testSHA256,
		StorageBackend:   "local",
		ObjectKey:        "users/" + filemeta.DevUserID + "/files/" + fileID,
		Metadata:         map[string]any{"purpose": "chat"},
	})
	if err != nil {
		t.Fatalf("CreateFile() error = %v", err)
	}

	_, err = repo.CreateMessage(ctx, conversation.ID, CreateMessageInput{
		Role:    "user",
		Content: "valid then missing",
		Attachments: []AttachmentInput{
			{FileID: fileRecord.ID, Purpose: "input"},
			{FileID: mustTestUUID(t), Purpose: "image"},
		},
	})
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("CreateMessage() error = %v, want ErrFileNotFound", err)
	}
	assertNoMessagesForConversation(t, ctx, db, conversation.ID)

	var linkCount int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM message_attachments WHERE file_id = $1`,
		fileRecord.ID,
	).Scan(&linkCount); err != nil {
		t.Fatalf("query message attachment rollback count: %v", err)
	}
	if linkCount != 0 {
		t.Fatalf("message attachment rows after rollback = %d, want 0", linkCount)
	}
}

func TestPostgresCancelRunLocksConversationBeforeMessage(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conversation, err := repo.CreateConversation(ctx, CreateConversationInput{
		Title: "cancel lock order",
	})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	userMessage, err := repo.CreateMessage(ctx, conversation.ID, CreateMessageInput{
		Role:    "user",
		Content: "hello",
	})
	if err != nil {
		t.Fatalf("CreateMessage() error = %v", err)
	}

	runID := mustTestUUID(t)
	assistantID := mustTestUUID(t)
	assistant, err := repo.CreateAssistantMessage(ctx, conversation.ID, CreateAssistantMessageInput{
		ID:              assistantID,
		ParentMessageID: userMessage.ID,
		IdempotencyKey:  "assistant-" + runID,
		Metadata: map[string]any{
			"runId": runID,
		},
	})
	if err != nil {
		t.Fatalf("CreateAssistantMessage() error = %v", err)
	}

	lockTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer func() {
		_ = lockTx.Rollback()
	}()

	var lockedConversationID string
	if err := lockTx.QueryRowContext(
		ctx,
		`SELECT id FROM conversations WHERE id = $1 FOR UPDATE`,
		conversation.ID,
	).Scan(&lockedConversationID); err != nil {
		t.Fatalf("lock conversation: %v", err)
	}

	cancelDone := make(chan error, 1)
	go func() {
		_, err := repo.CancelRun(ctx, runID, CancelRunInput{
			Metadata: map[string]any{
				"runId":       runID,
				"cancelledBy": "api",
			},
		})
		cancelDone <- err
	}()

	select {
	case err := <-cancelDone:
		t.Fatalf("CancelRun() completed while conversation lock was held: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	if _, err := lockTx.ExecContext(ctx, `SET LOCAL lock_timeout = '250ms'`); err != nil {
		t.Fatalf("set lock_timeout: %v", err)
	}
	var lockedMessageID string
	if err := lockTx.QueryRowContext(
		ctx,
		`SELECT id FROM messages WHERE id = $1 FOR UPDATE`,
		assistant.ID,
	).Scan(&lockedMessageID); err != nil {
		t.Fatalf("message row was locked before conversation row; possible cancel/finalize deadlock: %v", err)
	}

	if err := lockTx.Commit(); err != nil {
		t.Fatalf("release lock transaction: %v", err)
	}

	select {
	case err := <-cancelDone:
		if err != nil {
			t.Fatalf("CancelRun() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("CancelRun() did not finish after conversation lock was released")
	}

	messages, err := repo.ListMessages(ctx, conversation.ID)
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	if len(messages) != 2 || messages[1].Status != "cancelled" {
		t.Fatalf("messages after cancel = %#v, want assistant cancelled", messages)
	}
	if messages[1].Metadata["cancelledBy"] != "api" {
		t.Fatalf("assistant metadata = %#v, want cancelledBy=api", messages[1].Metadata)
	}
}

func TestPostgresCancelRunMergesMetadataForAlreadyCancelledRun(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conversation, err := repo.CreateConversation(ctx, CreateConversationInput{
		Title: "cancel metadata merge",
	})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	userMessage, err := repo.CreateMessage(ctx, conversation.ID, CreateMessageInput{
		Role:    "user",
		Content: "hello",
	})
	if err != nil {
		t.Fatalf("CreateMessage() error = %v", err)
	}

	runID := mustTestUUID(t)
	assistant, err := repo.CreateAssistantMessage(ctx, conversation.ID, CreateAssistantMessageInput{
		ID:              mustTestUUID(t),
		ParentMessageID: userMessage.ID,
		IdempotencyKey:  "assistant-" + runID,
		Metadata: map[string]any{
			"runId": runID,
		},
	})
	if err != nil {
		t.Fatalf("CreateAssistantMessage() error = %v", err)
	}
	if _, err := repo.FinalizeAssistantMessage(ctx, conversation.ID, assistant.ID, FinalizeAssistantMessageInput{
		Status: "cancelled",
		Metadata: map[string]any{
			"runId": runID,
		},
	}); err != nil {
		t.Fatalf("FinalizeAssistantMessage() error = %v", err)
	}

	message, err := repo.CancelRun(ctx, runID, CancelRunInput{
		Metadata: map[string]any{
			"runId":       runID,
			"cancelledBy": "api",
		},
	})
	if err != nil {
		t.Fatalf("CancelRun() error = %v", err)
	}
	if message.Status != "cancelled" || message.Metadata["cancelledBy"] != "api" {
		t.Fatalf("CancelRun() message = %#v, want cancelled with merged metadata", message)
	}
}

func openPostgresIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()

	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set MM_CHAT_TEST_DATABASE_URL to run Postgres integration tests")
	}

	pgxConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse MM_CHAT_TEST_DATABASE_URL: %v", err)
	}
	pgxConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	db := stdlib.OpenDB(*pgxConfig)
	t.Cleanup(func() {
		_ = db.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping integration database: %v", err)
	}
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	return db
}

func messageIDs(messages []Message) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func mustTestUUID(t *testing.T) string {
	t.Helper()

	id, err := NewUUID()
	if err != nil {
		t.Fatalf("NewUUID() error = %v", err)
	}
	return id
}

func assertNoMessagesForConversation(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	conversationID string,
) {
	t.Helper()

	var messageCount int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM messages WHERE conversation_id = $1`,
		conversationID,
	).Scan(&messageCount); err != nil {
		t.Fatalf("query conversation message count: %v", err)
	}
	if messageCount != 0 {
		t.Fatalf("messages after failed attachment link = %d, want 0", messageCount)
	}
}
