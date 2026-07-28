package usermemory

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

func TestPostgresDirectMemoryActionActivityAndUndo(t *testing.T) {
	db := openMemoryPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	service := NewService(repo)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userID, _ := newUUID()
	conversationID, _ := newUUID()
	sourceID, _ := newUUID()
	assistantID, _ := newUUID()
	if _, err := db.ExecContext(ctx, `
INSERT INTO users(id, display_name) VALUES ($1, 'direct action');
INSERT INTO conversations(id, user_id, title) VALUES ($2, $1, 'direct action');
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content,
  completed_at
) VALUES ($3, $2, $1, 1, 'user', 'completed',
  'Remember that I prefer concise replies', now());
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role,
  status, content
) VALUES ($4, $2, $1, $3, 2, 'assistant', 'streaming', '');
`, userID, conversationID, sourceID, assistantID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM users WHERE id = $1`, userID)
	})
	userCtx := auth.WithUser(ctx, auth.User{ID: userID, DisplayName: "direct"})
	assertProjectionRevision := func(memoryID string, wantRevision int64) {
		t.Helper()
		var count int
		var revision sql.NullInt64
		if err := db.QueryRowContext(ctx, `
SELECT count(*), max(memory_revision)
FROM user_memory_search_projections WHERE memory_id = $1
`, memoryID).Scan(&count, &revision); err != nil {
			t.Fatal(err)
		}
		if wantRevision == 0 {
			if count != 0 || revision.Valid {
				t.Fatalf("projection %s = count:%d revision:%#v, want removed",
					memoryID, count, revision)
			}
			return
		}
		if count != 1 || !revision.Valid || revision.Int64 != wantRevision {
			t.Fatalf("projection %s = count:%d revision:%#v, want %d",
				memoryID, count, revision, wantRevision)
		}
	}

	hydrated, err := service.HydrateDirectAction(userCtx, DirectActionHydrationInput{
		ConversationID:     conversationID,
		SourceMessageID:    sourceID,
		AssistantMessageID: assistantID,
	})
	if err != nil || len(hydrated.Memories) != 0 {
		t.Fatalf("HydrateDirectAction() = %#v/%v", hydrated, err)
	}
	created, err := service.ExecuteDirectAction(userCtx, DirectActionExecution{
		ConversationID:     conversationID,
		SourceMessageID:    sourceID,
		AssistantMessageID: assistantID,
		RequestedAction:    "remember",
		Candidate: &Candidate{
			Type: "preference", Content: "I prefer concise replies",
			Importance: 5, Tags: []string{"reply"},
		},
		Sensitivity: "normal",
		ScopeType:   "global",
		Confidence:  0.99,
	})
	if err != nil || created.Status != "applied" || created.ResultCode != "DIRECT_CREATED" ||
		created.MemoryID == "" || created.MemoryRevision != 1 || created.ActivityID == "" {
		t.Fatalf("ExecuteDirectAction() = %#v/%v", created, err)
	}
	assertProjectionRevision(created.MemoryID, 1)
	activities, err := service.ListActivities(userCtx, "", 50)
	if err != nil || len(activities) != 1 || activities[0].Action != "created" ||
		activities[0].MemoryContent != "I prefer concise replies" ||
		activities[0].UndoStatus != "available" {
		t.Fatalf("ListActivities() = %#v/%v", activities, err)
	}
	undone, err := service.UndoActivity(
		userCtx, created.ActivityID, created.MemoryRevision,
	)
	if err != nil || undone.Status != "undone" || undone.MemoryRevision != 2 {
		t.Fatalf("UndoActivity() = %#v/%v", undone, err)
	}
	assertProjectionRevision(created.MemoryID, 0)
	activities, err = service.ListActivities(userCtx, "", 50)
	if err != nil || len(activities) != 1 || !activities[0].MemoryDeleted ||
		activities[0].MemoryContent != "" || activities[0].UndoStatus != "undone" {
		t.Fatalf("activities after undo = %#v/%v", activities, err)
	}

	sequence := int64(3)
	addTurn := func(content string) (string, string) {
		t.Helper()
		source, sourceErr := newUUID()
		if sourceErr != nil {
			t.Fatal(sourceErr)
		}
		assistant, assistantErr := newUUID()
		if assistantErr != nil {
			t.Fatal(assistantErr)
		}
		if _, execErr := db.ExecContext(ctx, `
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content,
  completed_at
) VALUES ($1, $2, $3, $4, 'user', 'completed', $5, now());
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role,
  status, content
) VALUES ($6, $2, $3, $1, $7, 'assistant', 'streaming', '');
`, source, conversationID, userID, sequence, content, assistant, sequence+1); execErr != nil {
			t.Fatal(execErr)
		}
		sequence += 2
		return source, assistant
	}

	rememberSource, rememberAssistant := addTurn("Remember that I prefer dark themes")
	remembered, err := service.ExecuteDirectAction(userCtx, DirectActionExecution{
		ConversationID:     conversationID,
		SourceMessageID:    rememberSource,
		AssistantMessageID: rememberAssistant,
		RequestedAction:    "remember",
		Candidate: &Candidate{
			Type: "preference", Content: "I prefer dark themes",
			Importance: 4, Tags: []string{"display"},
		},
		Sensitivity: "normal",
		ScopeType:   "global",
		Confidence:  0.99,
	})
	if err != nil || remembered.Status != "applied" || remembered.MemoryRevision != 1 {
		t.Fatalf("remember for correction = %#v/%v", remembered, err)
	}
	assertProjectionRevision(remembered.MemoryID, 1)

	correctSource, correctAssistant := addTurn("Correct that memory to light themes")
	corrected, err := service.ExecuteDirectAction(userCtx, DirectActionExecution{
		ConversationID:     conversationID,
		SourceMessageID:    correctSource,
		AssistantMessageID: correctAssistant,
		RequestedAction:    "correct",
		Candidate: &Candidate{
			Type: "preference", Content: "I prefer light themes",
			Importance: 5, Tags: []string{"display", "theme"},
		},
		Sensitivity: "normal",
		ScopeType:   "global",
		Confidence:  0.99,
		Targets: []DirectActionTarget{{
			MemoryID: remembered.MemoryID, ExpectedRevision: remembered.MemoryRevision,
		}},
	})
	if err != nil || corrected.Status != "applied" || corrected.ResultCode != "DIRECT_CORRECTED" ||
		corrected.MemoryRevision != 2 {
		t.Fatalf("correct direct memory = %#v/%v", corrected, err)
	}
	assertProjectionRevision(remembered.MemoryID, 2)
	staleCreatedUndo, err := service.UndoActivity(
		userCtx, remembered.ActivityID, remembered.MemoryRevision,
	)
	if err != nil || staleCreatedUndo.Status != "review_required" ||
		staleCreatedUndo.ResultCode != "UNDO_STALE" || staleCreatedUndo.MemoryRevision != 2 {
		t.Fatalf("stale created undo = %#v/%v", staleCreatedUndo, err)
	}
	restored, err := service.UndoActivity(
		userCtx, corrected.ActivityID, corrected.MemoryRevision,
	)
	if err != nil || restored.Status != "undone" || restored.MemoryRevision != 3 {
		t.Fatalf("corrected undo = %#v/%v", restored, err)
	}
	assertProjectionRevision(remembered.MemoryID, 3)
	var restoredContent, restoredType, restoredSource, restoredAuthority, restoredScope string
	var restoredImportance int
	var restoredTagCount int
	var restoredTag string
	var restoredSourceMessage string
	if err := db.QueryRowContext(ctx, `
SELECT content, memory_type, importance, cardinality(tags), tags[1], source, authority_kind,
       scope_type, source_message_id::text
FROM user_memories WHERE id = $1
`, remembered.MemoryID).Scan(
		&restoredContent,
		&restoredType,
		&restoredImportance,
		&restoredTagCount,
		&restoredTag,
		&restoredSource,
		&restoredAuthority,
		&restoredScope,
		&restoredSourceMessage,
	); err != nil {
		t.Fatal(err)
	}
	if restoredContent != "I prefer dark themes" || restoredType != "preference" ||
		restoredImportance != 4 || restoredTagCount != 1 || restoredTag != "display" ||
		restoredSource != "direct_user" || restoredAuthority != "direct_user" ||
		restoredScope != "global" || restoredSourceMessage != rememberSource {
		t.Fatalf("restored typed snapshot = %q/%q/%d/%d:%q/%q/%q/%q/%q",
			restoredContent, restoredType, restoredImportance, restoredTagCount, restoredTag,
			restoredSource, restoredAuthority, restoredScope, restoredSourceMessage)
	}

	noopSource, noopAssistant := addTurn("Remember that I prefer dark themes")
	noop, err := service.ExecuteDirectAction(userCtx, DirectActionExecution{
		ConversationID:     conversationID,
		SourceMessageID:    noopSource,
		AssistantMessageID: noopAssistant,
		RequestedAction:    "remember",
		Candidate: &Candidate{
			Type: "preference", Content: "I prefer dark themes",
			Importance: 4, Tags: []string{"display"},
		},
		Sensitivity: "normal",
		ScopeType:   "global",
		Confidence:  0.99,
	})
	if err != nil || noop.Status != "noop" || noop.ResultCode != "EXACT_NOOP" ||
		noop.ActivityID != "" || noop.MemoryID != remembered.MemoryID || noop.MemoryRevision != 3 {
		t.Fatalf("exact direct noop = %#v/%v", noop, err)
	}

	projectSource, projectAssistant := addTurn("Remember this for the current project")
	projectReview, err := service.ExecuteDirectAction(userCtx, DirectActionExecution{
		ConversationID:     conversationID,
		SourceMessageID:    projectSource,
		AssistantMessageID: projectAssistant,
		RequestedAction:    "remember",
		Candidate: &Candidate{
			Type: "project", Content: "Use blue for this project",
			Importance: 3, Tags: []string{"project"},
		},
		Sensitivity: "normal",
		ScopeType:   "project",
		Confidence:  0.99,
	})
	if err != nil || projectReview.Status != "review_required" ||
		projectReview.ResultCode != "SCOPE_UNAVAILABLE" || projectReview.MemoryID != "" ||
		projectReview.ActivityID == "" {
		t.Fatalf("unavailable Project scope = %#v/%v", projectReview, err)
	}
	var resolvedProject sql.NullString
	if err := db.QueryRowContext(ctx, `
SELECT resolved_project_id::text FROM memory_user_actions WHERE id = $1
`, projectReview.ActionID).Scan(&resolvedProject); err != nil || resolvedProject.Valid {
		t.Fatalf("unavailable Project authority = %#v/%v", resolvedProject, err)
	}

	forgetSource, forgetAssistant := addTurn("Forget the dark theme memory")
	forgotten, err := service.ExecuteDirectAction(userCtx, DirectActionExecution{
		ConversationID:     conversationID,
		SourceMessageID:    forgetSource,
		AssistantMessageID: forgetAssistant,
		RequestedAction:    "forget",
		Sensitivity:        "normal",
		ScopeType:          "global",
		Confidence:         0.99,
		RequestHash:        hashMemoryActionValue("Forget the dark theme memory"),
		Targets: []DirectActionTarget{{
			MemoryID: remembered.MemoryID, ExpectedRevision: restored.MemoryRevision,
		}},
	})
	if err != nil || forgotten.Status != "applied" ||
		forgotten.ResultCode != "DIRECT_FORGOTTEN" || forgotten.MemoryRevision != 4 {
		t.Fatalf("direct forget = %#v/%v", forgotten, err)
	}
	assertProjectionRevision(remembered.MemoryID, 0)
	workerID, _ := newUUID()
	leaseToken, _ := newUUID()
	var purgeJobID string
	if err := db.QueryRowContext(ctx, `
SELECT job_id::text FROM memory_jobs
WHERE user_id = $1 AND stage = 'purge' AND target_memory_id = $2
`, userID, remembered.MemoryID).Scan(&purgeJobID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE memory_jobs
SET status = 'processing', attempt_count = 1,
    lease_owner = $2, lease_token = $3,
    lease_expires_at = now() + interval '1 minute', updated_at = now()
WHERE job_id = $1;
UPDATE memory_outbox outbox
SET status = 'processing', attempt_count = 1,
    lease_owner = $2, lease_token = $3,
    lease_expires_at = now() + interval '1 minute', updated_at = now()
FROM memory_jobs job
WHERE job.job_id = $1 AND outbox.event_id = job.event_id;
`, purgeJobID, workerID, leaseToken); err != nil {
		t.Fatal(err)
	}
	var purged bool
	if err := db.QueryRowContext(ctx, `
SELECT memory_worker_purge_memory($1, $2, $3)
`, purgeJobID, workerID, leaseToken).Scan(&purged); err != nil || !purged {
		t.Fatalf("provider-free direct purge = %t/%v", purged, err)
	}
	var canonicalPlaintext, evidenceCount, revisionSnapshotCount int
	var manifestResult string
	if err := db.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM user_memories memory
   WHERE memory.id = $1 AND (
     memory.content <> '' OR memory.normalized_content <> ''
     OR cardinality(memory.tags) > 0
     OR memory.source_conversation_id IS NOT NULL
     OR memory.source_message_id IS NOT NULL
     OR memory.extraction_profile_id IS NOT NULL
     OR memory.subject_key IS NOT NULL OR memory.fact_key IS NOT NULL
     OR memory.temporal_parser_version IS NOT NULL
   )),
  (SELECT count(*) FROM user_memory_evidence evidence WHERE evidence.memory_id = $1),
  (SELECT count(*) FROM user_memory_revisions revision
   WHERE revision.memory_id = $1 AND (
     revision.prior_content_snapshot IS NOT NULL
     OR revision.prior_memory_type IS NOT NULL
     OR revision.prior_normalized_content IS NOT NULL
     OR revision.prior_tags IS NOT NULL
     OR revision.prior_source_message_id IS NOT NULL
     OR revision.prior_subject_key IS NOT NULL
     OR revision.prior_fact_key IS NOT NULL
     OR revision.prior_temporal_parser_version IS NOT NULL
   )),
  (SELECT result_code FROM user_memory_deletion_manifests WHERE memory_id = $1)
`, remembered.MemoryID).Scan(
		&canonicalPlaintext, &evidenceCount, &revisionSnapshotCount, &manifestResult,
	); err != nil {
		t.Fatal(err)
	}
	if canonicalPlaintext != 0 || evidenceCount != 0 || revisionSnapshotCount != 0 ||
		manifestResult != "ONLINE_PURGED" {
		t.Fatalf("purge residue = canonical:%d evidence:%d revisions:%d manifest:%q",
			canonicalPlaintext, evidenceCount, revisionSnapshotCount, manifestResult)
	}
	assertProjectionRevision(remembered.MemoryID, 0)
}
