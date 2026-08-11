package mcpclient

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresRepositoryLifecycleAndRetention(t *testing.T) {
	db := openMCPPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	userID, conversationID, runID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	serverID := uuid.NewString()
	mustExecMCP(t, ctx, db, `
INSERT INTO users (id, email, display_name) VALUES ($1, $2, 'MCP integration')
`, userID, "mcp-"+userID+"@example.test")
	mustExecMCP(t, ctx, db, `
INSERT INTO conversations (id, user_id, title) VALUES ($1, $2, 'MCP integration')
`, conversationID, userID)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM users WHERE id = $1`, userID)
	})

	repo := NewPostgresRepository(db)
	repo.newID = func() string { return serverID }
	server, err := repo.CreatePrivateServer(ctx, userID, CreateServerInput{
		Name: "Private MCP", EndpointURL: "https://mcp.example/tools", AuthType: AuthNone,
	})
	if err != nil || server.Ref.ID != serverID || server.Status != ServerStatusDraft {
		t.Fatalf("CreatePrivateServer() server=%#v error=%v", server, err)
	}
	listed, err := repo.ListPrivateServers(ctx, userID)
	if err != nil || len(listed) != 1 || listed[0].Ref.ID != serverID {
		t.Fatalf("ListPrivateServers() servers=%#v error=%v", listed, err)
	}

	selection, err := repo.ReplaceSelection(ctx, userID, Selection{
		ConversationID: conversationID,
		Mode:           SelectionModeCustom,
		Servers:        []SelectionServer{{Ref: server.Ref, DisabledTools: []string{"disabled"}}},
	})
	if err != nil || selection.Revision != 1 {
		t.Fatalf("ReplaceSelection() selection=%#v error=%v", selection, err)
	}
	gotSelection, found, err := repo.GetSelection(ctx, userID, conversationID)
	if err != nil || !found || len(gotSelection.Servers) != 1 || gotSelection.Servers[0].DisabledTools[0] != "disabled" {
		t.Fatalf("GetSelection() selection=%#v found=%v error=%v", gotSelection, found, err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	callID := uuid.NewString()
	call, err := repo.CreateCall(ctx, userID, CallRecord{
		ID: callID, ConversationID: conversationID, RunID: runID,
		ServerRef: server.Ref, ToolName: "lookup", ToolAlias: "mcp_private_lookup",
		Classification: ClassificationRead, Status: CallStatusQueued,
		Round: 1, Call: 1, ArgumentsSummary: map[string]any{"secret": "string"},
		RetainUntil: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateCall() error=%v", err)
	}
	call.Status = CallStatusSucceeded
	call.StartedAt, call.CompletedAt = &now, &now
	call.ResultSummary = "content_items=1 bytes=7 artifacts=1"
	objectKey := "mcp-results/" + conversationID + "/" + callID + "/001"
	if err := repo.FinishCall(ctx, userID, call, []Content{{Type: "text", ObjectKey: objectKey}}, []string{objectKey}, 7); err != nil {
		t.Fatalf("FinishCall() error=%v", err)
	}
	keys, err := repo.ListConversationObjectKeys(ctx, userID, conversationID)
	if err != nil || len(keys) != 1 || keys[0] != objectKey {
		t.Fatalf("ListConversationObjectKeys() keys=%v error=%v", keys, err)
	}
	expired, err := repo.ListExpiredCalls(ctx, now, 10)
	if err != nil || len(expired) != 1 || expired[0].ID != callID || len(expired[0].ObjectKeys) != 1 {
		t.Fatalf("ListExpiredCalls() calls=%#v error=%v", expired, err)
	}
	if err := repo.DeleteExpiredData(ctx, now, []string{callID}); err != nil {
		t.Fatalf("DeleteExpiredData() error=%v", err)
	}
	assertMCPRowCount(t, ctx, db, "mcp_tool_calls", "id", callID, 0)
	assertMCPRowCount(t, ctx, db, "mcp_tool_results", "call_id", callID, 0)

	activeCallID := uuid.NewString()
	if _, err := repo.CreateCall(ctx, userID, CallRecord{
		ID: activeCallID, ConversationID: conversationID, RunID: uuid.NewString(),
		ServerRef: server.Ref, ToolName: "lookup", ToolAlias: "mcp_private_lookup",
		Classification: ClassificationRead, Status: CallStatusQueued,
		Round: 1, Call: 1, ArgumentsSummary: map[string]any{}, RetainUntil: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("create active call: %v", err)
	}
	if err := repo.DeleteConversationData(ctx, userID, conversationID); err != nil {
		t.Fatalf("DeleteConversationData() error=%v", err)
	}
	assertMCPRowCount(t, ctx, db, "mcp_tool_calls", "id", activeCallID, 0)
	assertMCPRowCount(t, ctx, db, "mcp_conversation_selections", "conversation_id", conversationID, 0)

	credentialID := uuid.NewString()
	if _, err := repo.UpsertCredential(ctx, Credential{
		ID: credentialID, UserID: userID, ServerRef: server.Ref,
		Kind: AuthHeader, EncryptedSecretRef: "encrypted-fixture", Metadata: map[string]any{},
	}); err != nil {
		t.Fatalf("UpsertCredential() error=%v", err)
	}
	accountCallID := uuid.NewString()
	accountCall, err := repo.CreateCall(ctx, userID, CallRecord{
		ID: accountCallID, ConversationID: conversationID, RunID: uuid.NewString(),
		ServerRef: server.Ref, ToolName: "account_artifact", ToolAlias: "mcp_private_account_artifact",
		Classification: ClassificationRead, Status: CallStatusQueued,
		Round: 1, Call: 1, ArgumentsSummary: map[string]any{}, RetainUntil: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create account lifecycle call: %v", err)
	}
	accountCall.Status = CallStatusSucceeded
	accountCall.StartedAt, accountCall.CompletedAt = &now, &now
	accountObjectKey := "mcp-results/" + conversationID + "/" + accountCallID + "/001"
	if err := repo.FinishCall(ctx, userID, accountCall, []Content{{Type: "text", ObjectKey: accountObjectKey}}, []string{accountObjectKey}, 7); err != nil {
		t.Fatalf("finish account lifecycle call: %v", err)
	}
	mustExecMCP(t, ctx, db, `DELETE FROM users WHERE id = $1`, userID)
	assertMCPRowCount(t, ctx, db, "mcp_credentials", "id", credentialID, 0)
	assertMCPRowCount(t, ctx, db, "mcp_servers", "id", serverID, 0)
	assertMCPRowCount(t, ctx, db, "mcp_tool_calls", "id", accountCallID, 0)
	assertMCPRowCount(t, ctx, db, "mcp_artifact_cleanup_queue", "object_key", accountObjectKey, 1)
	objects := &memoryObjectStore{objects: map[string][]byte{accountObjectKey: []byte("fixture")}}
	cleanupService, err := NewService(DefaultConfig(), repo, nil, nil, objects, Catalog{}, nil)
	if err != nil {
		t.Fatalf("create cleanup-only service: %v", err)
	}
	if count, err := cleanupService.PruneExpiredData(ctx, 100); err != nil || count != 1 {
		t.Fatalf("prune account artifact count=%d error=%v", count, err)
	}
	if len(objects.objects) != 0 {
		t.Fatalf("account artifact remained: %#v", objects.objects)
	}
	assertMCPRowCount(t, ctx, db, "mcp_artifact_cleanup_queue", "object_key", accountObjectKey, 0)
}

func openMCPPostgresIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set MM_CHAT_TEST_DATABASE_URL to run MCP Postgres integration tests")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse MM_CHAT_TEST_DATABASE_URL: %v", err)
	}
	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	db := stdlib.OpenDB(*config)
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping MCP integration database: %v", err)
	}
	return db
}

func mustExecMCP(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("execute MCP fixture: %v", err)
	}
}

func assertMCPRowCount(t *testing.T, ctx context.Context, db *sql.DB, table, column, id string, want int) {
	t.Helper()
	allowed := map[string]map[string]bool{
		"mcp_tool_calls":              {"id": true},
		"mcp_tool_results":            {"call_id": true},
		"mcp_conversation_selections": {"conversation_id": true},
		"mcp_credentials":             {"id": true},
		"mcp_servers":                 {"id": true},
		"mcp_artifact_cleanup_queue":  {"object_key": true},
	}
	if !allowed[table][column] {
		t.Fatalf("unsafe test identifier %s.%s", table, column)
	}
	var got int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE `+column+` = $1`, id).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s rows=%d want=%d", table, got, want)
	}
}
