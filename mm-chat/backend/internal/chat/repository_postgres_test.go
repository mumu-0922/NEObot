package chat

import (
	"context"
	"database/sql"
	"errors"
	"os"
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
