package migration

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestMemoryDeadLetterOrphanActivityMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "109_memory_dead_letter_orphan_activity.up.sql")
	down := readPhase15SQL(t, "109_memory_dead_letter_orphan_activity.down.sql")

	assertPhase15Fragments(t, up,
		"109 must retain dead-letter evidence while projecting only owned live assistants",
		"create or replace function memory_dead_letter_activity_trigger",
		"from messages assistant",
		"assistant.id = new.assistant_message_id",
		"assistant.user_id = new.user_id",
		"on conflict ( source_kind , source_id ) do nothing",
		"return new")
	for _, forbidden := range []string{
		"delete from memory_jobs",
		"update memory_jobs",
		"alter table message_memory_activities",
		"drop constraint message_memory_activities_assistant_owner_fk",
	} {
		if strings.Contains(up, forbidden) {
			t.Errorf("109 contains forbidden authority change %q", forbidden)
		}
	}
	assertPhase15Fragments(t, down,
		"109 down must restore the prior trigger body without rewriting history",
		"create or replace function memory_dead_letter_activity_trigger",
		"values (",
		"return new")
}

func TestMemoryDeadLetterOrphanActivityLivePostgres(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	files := phase15MigrationFSThrough(t, 37)
	for _, path := range []string{
		"053_memory_project_scope_settings.up.sql",
		"053_memory_project_scope_settings.down.sql",
		"054_memory_outbox_jobs_worker.up.sql",
		"054_memory_outbox_jobs_worker.down.sql",
		"055_memory_provenance_deletion.up.sql",
		"055_memory_provenance_deletion.down.sql",
		"056_memory_candidate_review_shadow.up.sql",
		"056_memory_candidate_review_shadow.down.sql",
		"057_memory_actions_activity_usage.up.sql",
		"057_memory_actions_activity_usage.down.sql",
		"109_memory_dead_letter_orphan_activity.up.sql",
		"109_memory_dead_letter_orphan_activity.down.sql",
	} {
		data, err := migrationfiles.FS.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		files[path] = &fstest.MapFile{Data: data}
	}
	runner := NewRunner(db, files)
	if _, err := runner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Up(ctx); err != nil {
		t.Fatalf("apply through 109: %v", err)
	}

	const (
		userID            = "19000000-0000-4000-8000-000000000001"
		conversationID    = "29000000-0000-4000-8000-000000000001"
		orphanSourceID    = "39000000-0000-4000-8000-000000000001"
		orphanAssistantID = "39000000-0000-4000-8000-000000000002"
		liveSourceID      = "39000000-0000-4000-8000-000000000003"
		liveAssistantID   = "39000000-0000-4000-8000-000000000004"
		providerID        = "49000000-0000-4000-8000-000000000001"
		orphanEventID     = "59000000-0000-4000-8000-000000000001"
		orphanJobID       = "69000000-0000-4000-8000-000000000001"
		liveEventID       = "59000000-0000-4000-8000-000000000002"
		liveJobID         = "69000000-0000-4000-8000-000000000002"
		workerID          = "79000000-0000-4000-8000-000000000001"
		leaseToken        = "89000000-0000-4000-8000-000000000001"
	)

	mustExecPhase151C(t, ctx, db, `
INSERT INTO users(id, display_name) VALUES ($1, 'orphan activity');
INSERT INTO conversations(id, user_id, title) VALUES ($2, $1, 'orphan activity');
INSERT INTO user_memory_settings(
  user_id, enabled, search_enabled, auto_record_enabled,
  sensitive_memory_enabled
) VALUES ($1, true, true, true, true);
INSERT INTO provider_configs(
  id, user_id, provider_id, label, encrypted_secret_ref, config
) VALUES (
  $3, $1, 'fixture', 'Fixture', '{}',
  '{"kind":"model","type":"OpenAI Compatible","enabled":true}'::jsonb
);
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content, completed_at
) VALUES
  ($4, $2, $1, 1, 'user', 'completed', 'orphan source', now()),
  ($5, $2, $1, 3, 'user', 'completed', 'live source', now());
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no,
  role, status, content, completed_at
) VALUES
  ($6, $2, $1, $4, 2, 'assistant', 'completed', 'orphan answer', now()),
  ($7, $2, $1, $5, 4, 'assistant', 'completed', 'live answer', now());
SELECT memory_append_turn_completed_event(
  $8, $9, $1, $2, $4, $6,
  'server-stored', 'fixture', 'fixture-model', 2::smallint
);
SELECT memory_append_turn_completed_event(
  $10, $11, $1, $2, $5, $7,
  'server-stored', 'fixture', 'fixture-model', 2::smallint
);
UPDATE memory_jobs
SET status = 'processing', attempt_count = max_attempts,
    lease_owner = $12, lease_token = $13,
    created_at = now() - interval '2 minutes',
    available_at = now() - interval '2 minutes',
    lease_expires_at = now() - interval '1 minute', updated_at = now()
WHERE job_id IN ($9, $11);
UPDATE memory_outbox outbox
SET status = 'processing', attempt_count = job.max_attempts,
    lease_owner = $12, lease_token = $13,
    created_at = now() - interval '2 minutes',
    available_at = now() - interval '2 minutes',
    lease_expires_at = now() - interval '1 minute', updated_at = now()
FROM memory_jobs job
WHERE outbox.event_id = job.event_id AND job.job_id IN ($9, $11);
DELETE FROM messages WHERE id = $6;
`, userID, conversationID, providerID, orphanSourceID, liveSourceID,
		orphanAssistantID, liveAssistantID, orphanEventID, orphanJobID,
		liveEventID, liveJobID, workerID, leaseToken)

	var claimed int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM memory_worker_claim_job(
  $1::uuid, $2::uuid, 120
)
`, workerID, leaseToken).Scan(&claimed); err != nil {
		t.Fatalf("terminalize expired final attempts: %v", err)
	}
	if claimed != 0 {
		t.Fatalf("claimed jobs = %d, want 0", claimed)
	}

	var deadLetters, orphanActivities, liveActivities int
	if err := db.QueryRowContext(ctx, `
SELECT
  count(*) FILTER (
    WHERE status = 'dead_letter' AND error_code = 'LEASE_EXPIRED'
  ),
  (SELECT count(*) FROM message_memory_activities
   WHERE source_kind = 'memory_job' AND source_id = $1),
  (SELECT count(*) FROM message_memory_activities
   WHERE source_kind = 'memory_job' AND source_id = $2)
FROM memory_jobs
WHERE job_id IN ($1, $2)
`, orphanJobID, liveJobID).Scan(
		&deadLetters, &orphanActivities, &liveActivities,
	); err != nil {
		t.Fatal(err)
	}
	if deadLetters != 2 || orphanActivities != 0 || liveActivities != 1 {
		t.Fatalf("dead letters/orphan activity/live activity = %d/%d/%d",
			deadLetters, orphanActivities, liveActivities)
	}

	rolledBack, err := runner.Down(ctx, false)
	if err != nil || len(rolledBack) != 1 || rolledBack[0].Version != 109 {
		t.Fatalf("down 109 = %#v/%v", rolledBack, err)
	}
	reapplied, err := runner.Up(ctx)
	if err != nil || len(reapplied) != 1 || reapplied[0].Version != 109 {
		t.Fatalf("reapply 109 = %#v/%v", reapplied, err)
	}
	var replayJobs, replayActivities int
	if err := db.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM memory_jobs WHERE job_id IN ($1, $2)),
  (SELECT count(*) FROM message_memory_activities
   WHERE source_kind = 'memory_job' AND source_id IN ($1, $2))
`, orphanJobID, liveJobID).Scan(&replayJobs, &replayActivities); err != nil {
		t.Fatal(err)
	}
	if replayJobs != 2 || replayActivities != 1 {
		t.Fatalf("replayed jobs/activities = %d/%d", replayJobs, replayActivities)
	}
}
