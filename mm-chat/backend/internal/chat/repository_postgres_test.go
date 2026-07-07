package chat

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

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
