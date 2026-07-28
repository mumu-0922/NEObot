package usermemory

import (
	"context"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

func TestPostgresLexicalShadowComparesWithoutChangingV1Results(t *testing.T) {
	db := openMemoryPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	userID, _ := newUUID()
	conversationID, _ := newUUID()
	sourceID, _ := newUUID()
	assistantID, _ := newUUID()
	query := "请继续用简洁风格回答"
	if _, err := db.ExecContext(ctx, `
INSERT INTO users(id, display_name) VALUES ($1, 'lexical shadow');
INSERT INTO conversations(id, user_id, title) VALUES ($2, $1, 'lexical shadow');
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content,
  completed_at
) VALUES ($3, $2, $1, 1, 'user', 'completed', $5, now());
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role,
  status, content
) VALUES ($4, $2, $1, $3, 2, 'assistant', 'streaming', '');
`, userID, conversationID, sourceID, assistantID, query); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM users WHERE id = $1`, userID)
	})

	service := NewService(NewPostgresRepository(db))
	userCtx := auth.WithUser(ctx, auth.User{ID: userID, DisplayName: "lexical"})
	enabled := true
	if _, err := service.UpdateSettings(userCtx, SettingsPatch{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateManual(userCtx, Candidate{
		Type: "preference", Content: "用户偏好简洁回答", Importance: 5,
		Tags: []string{"风格"},
	})
	if err != nil {
		t.Fatal(err)
	}

	items, summary, err := service.SearchRelevantWithShadow(
		userCtx,
		query,
		conversationID,
		assistantID,
		MaxSearchResults,
	)
	if err != nil || len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("v1 result = %#v/%v", items, err)
	}
	if summary.ProfileID != LexicalShadowProfileID || summary.Status != "completed" ||
		summary.ResultCode != "OK" || summary.BaselineCount != 1 ||
		summary.BM25Count < 1 || summary.LexicalCount < 1 || summary.OverlapCount != 1 {
		t.Fatalf("lexical summary = %#v", summary)
	}
	replayedItems, replayedSummary, err := service.SearchRelevantWithShadow(
		userCtx,
		query,
		conversationID,
		assistantID,
		MaxSearchResults,
	)
	if err != nil || len(replayedItems) != len(items) ||
		replayedSummary != summary {
		t.Fatalf("idempotent Go replay = items:%#v summary:%#v err:%v",
			replayedItems, replayedSummary, err)
	}

	var observationCount, resultCount int
	if err := db.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM message_memory_lexical_shadow_observations
   WHERE assistant_message_id = $1 AND user_id = $2),
  (SELECT count(*) FROM message_memory_lexical_shadow_results result
   JOIN message_memory_lexical_shadow_observations observation
     ON observation.id = result.observation_id
   WHERE observation.assistant_message_id = $1 AND result.memory_id = $3)
`, assistantID, userID, created.ID).Scan(&observationCount, &resultCount); err != nil {
		t.Fatal(err)
	}
	if observationCount != 1 || resultCount < 2 {
		t.Fatalf("normalized shadow links = observations:%d results:%d", observationCount, resultCount)
	}
}
