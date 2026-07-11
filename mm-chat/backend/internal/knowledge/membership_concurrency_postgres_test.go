package knowledge

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/migration"
	"neo-chat/mm-chat/backend/internal/teams"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

const (
	membershipRaceRemoverID    = "12000000-0000-4000-8000-000000000001"
	membershipRaceTargetID     = "12000000-0000-4000-8000-000000000002"
	membershipRaceTeamID       = "22000000-0000-4000-8000-000000000001"
	membershipRaceCollectionID = "32000000-0000-4000-8000-000000000001"
	membershipRaceDocumentID   = "42000000-0000-4000-8000-000000000001"
	membershipRaceFileID       = "52000000-0000-4000-8000-000000000001"
	membershipRaceVersionID    = "62000000-0000-4000-8000-000000000001"
)

type membershipRaceMutation struct {
	name             string
	removalFirstWant error
	run              func(context.Context, *PostgresRepository) error
	assert           func(*testing.T, context.Context, *sql.DB, bool)
}

func TestPostgresMembershipRemovalSerializesTeamKnowledgeMutations(t *testing.T) {
	mutations := []membershipRaceMutation{
		{
			name: "collection update", removalFirstWant: ErrCollectionNotFound,
			run: func(ctx context.Context, repo *PostgresRepository) error {
				name := "Renamed"
				_, err := repo.UpdateCollection(ctx, UpdateCollectionRepositoryInput{
					CollectionID: membershipRaceCollectionID, ActorUserID: membershipRaceTargetID, Name: &name,
				})
				return err
			},
			assert: func(t *testing.T, ctx context.Context, db *sql.DB, mutationFirst bool) {
				t.Helper()
				var name string
				var aclRevision, visibilityEpoch, processingRevision int64
				var events, exactPayloads int
				if err := db.QueryRowContext(ctx, `SELECT name,acl_revision,visibility_epoch,collection_processing_revision FROM knowledge_collections WHERE id=$1`, membershipRaceCollectionID).Scan(&name, &aclRevision, &visibilityEpoch, &processingRevision); err != nil {
					t.Fatal(err)
				}
				if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.collection.updated'`, membershipRaceCollectionID).Scan(&events); err != nil {
					t.Fatal(err)
				}
				if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.collection.updated' AND (payload->>'aclRevision')::bigint=1 AND (payload->>'visibilityEpoch')::bigint=1 AND (payload->>'collectionProcessingRevision')::bigint=1`, membershipRaceCollectionID).Scan(&exactPayloads); err != nil {
					t.Fatal(err)
				}
				if aclRevision != 1 || visibilityEpoch != 1 || processingRevision != 1 {
					t.Fatalf("collection revisions = %d/%d/%d", aclRevision, visibilityEpoch, processingRevision)
				}
				if mutationFirst && (name != "Renamed" || events != 1 || exactPayloads != 1) {
					t.Fatalf("mutation-first collection name/events/payloads = %s/%d/%d", name, events, exactPayloads)
				}
				if !mutationFirst && (name != "Shared" || events != 0 || exactPayloads != 0) {
					t.Fatalf("removal-first collection name/events/payloads = %s/%d/%d", name, events, exactPayloads)
				}
			},
		},
		{
			name: "document delete", removalFirstWant: ErrDocumentNotFound,
			run: func(ctx context.Context, repo *PostgresRepository) error {
				return repo.DeleteDocument(ctx, DeleteDocumentRepositoryInput{
					DocumentID: membershipRaceDocumentID, ActorUserID: membershipRaceTargetID,
				})
			},
			assert: func(t *testing.T, ctx context.Context, db *sql.DB, mutationFirst bool) {
				t.Helper()
				var status string
				var deleted bool
				var documentEpoch, versionEpoch int64
				var versionStatus string
				var events, purgeJobs, exactPayloads int
				if err := db.QueryRowContext(ctx, `SELECT status,deleted_at IS NOT NULL,visibility_epoch FROM knowledge_documents WHERE id=$1`, membershipRaceDocumentID).Scan(&status, &deleted, &documentEpoch); err != nil {
					t.Fatal(err)
				}
				if err := db.QueryRowContext(ctx, `SELECT status,visibility_epoch FROM knowledge_document_versions WHERE id=$1`, membershipRaceVersionID).Scan(&versionStatus, &versionEpoch); err != nil {
					t.Fatal(err)
				}
				if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_processing_jobs WHERE document_id=$1 AND stage='purge' AND operation='purge'`, membershipRaceDocumentID).Scan(&purgeJobs); err != nil {
					t.Fatal(err)
				}
				if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1`, membershipRaceDocumentID).Scan(&events); err != nil {
					t.Fatal(err)
				}
				if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.document.tombstoned' AND (payload->>'documentVisibilityEpoch')::bigint=2 AND (payload->>'versionVisibilityEpoch')::bigint=2 AND (payload->>'collectionAclRevision')::bigint=1 AND (payload->>'collectionVisibilityEpoch')::bigint=1 AND (payload->>'collectionProcessingRevision')::bigint=1`, membershipRaceDocumentID).Scan(&exactPayloads); err != nil {
					t.Fatal(err)
				}
				if mutationFirst && (status != "tombstoned" || !deleted || documentEpoch != 2 || versionStatus != "tombstoned" || versionEpoch != 2 || purgeJobs != 1 || events != 1 || exactPayloads != 1) {
					t.Fatalf("mutation-first document = %s/%v/%d version=%s/%d purge/events/payloads=%d/%d/%d", status, deleted, documentEpoch, versionStatus, versionEpoch, purgeJobs, events, exactPayloads)
				}
				if !mutationFirst && (status != "active" || deleted || documentEpoch != 1 || versionStatus != "active" || versionEpoch != 1 || purgeJobs != 0 || events != 0 || exactPayloads != 0) {
					t.Fatalf("removal-first document = %s/%v/%d version=%s/%d purge/events/payloads=%d/%d/%d", status, deleted, documentEpoch, versionStatus, versionEpoch, purgeJobs, events, exactPayloads)
				}
			},
		},
		{
			name: "collection consent", removalFirstWant: ErrCollectionNotFound,
			run: func(ctx context.Context, repo *PostgresRepository) error {
				_, err := repo.PutCollectionConsent(ctx, PutCollectionConsentRepositoryInput{
					CollectionID: membershipRaceCollectionID, ActorUserID: membershipRaceTargetID,
					Processor: "mineru", Purposes: []string{"parse"},
					DataTypes: []string{"application/pdf"}, PolicyVersion: "v1",
				})
				return err
			},
			assert: func(t *testing.T, ctx context.Context, db *sql.DB, mutationFirst bool) {
				t.Helper()
				var consents, events int
				var revision int64
				if err := db.QueryRowContext(ctx, `SELECT count(*) FROM processing_consents WHERE collection_id=$1`, membershipRaceCollectionID).Scan(&consents); err != nil {
					t.Fatal(err)
				}
				if err := db.QueryRowContext(ctx, `SELECT collection_processing_revision FROM knowledge_collections WHERE id=$1`, membershipRaceCollectionID).Scan(&revision); err != nil {
					t.Fatal(err)
				}
				if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.collection.consent.changed'`, membershipRaceCollectionID).Scan(&events); err != nil {
					t.Fatal(err)
				}
				if mutationFirst && (consents != 1 || revision != 2 || events != 1) {
					t.Fatalf("mutation-first consent rows/revision/events = %d/%d/%d", consents, revision, events)
				}
				if !mutationFirst && (consents != 0 || revision != 1 || events != 0) {
					t.Fatalf("removal-first consent rows/revision/events = %d/%d/%d", consents, revision, events)
				}
			},
		},
	}

	for _, mutation := range mutations {
		mutation := mutation
		t.Run(mutation.name+"/removal-first", func(t *testing.T) {
			runMembershipKnowledgeRace(t, mutation, true)
		})
		t.Run(mutation.name+"/mutation-first", func(t *testing.T) {
			runMembershipKnowledgeRace(t, mutation, false)
		})
	}
}

func runMembershipKnowledgeRace(t *testing.T, mutation membershipRaceMutation, removalFirst bool) {
	t.Helper()
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatal(err)
	}
	seedMembershipKnowledgeRace(t, ctx, db)
	var applicationName string
	if err := db.QueryRowContext(ctx, `SHOW application_name`).Scan(&applicationName); err != nil {
		t.Fatal(err)
	}

	gateConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer gateConn.Close()
	gateTx, err := gateConn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer gateTx.Rollback()
	var gatePID int
	if err := gateTx.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&gatePID); err != nil {
		t.Fatal(err)
	}
	var lockedTeam string
	if err := gateTx.QueryRowContext(ctx, `SELECT id FROM teams WHERE id=$1 FOR UPDATE`, membershipRaceTeamID).Scan(&lockedTeam); err != nil {
		t.Fatal(err)
	}

	teamRepo := teams.NewPostgresRepository(db)
	knowledgeRepo := NewPostgresRepository(db)
	remove := func() error {
		return teamRepo.RemoveMember(ctx, teams.RemoveMemberRepositoryInput{
			TeamID: membershipRaceTeamID, ActorUserID: membershipRaceRemoverID, TargetUserID: membershipRaceTargetID,
		})
	}
	removeDone := make(chan error, 1)
	mutationDone := make(chan error, 1)

	if removalFirst {
		go func() { removeDone <- remove() }()
		removePID := waitForKnowledgeBlockedByPID(t, ctx, db, applicationName, gatePID, "FROM teams", "FOR UPDATE")
		go func() { mutationDone <- mutation.run(ctx, knowledgeRepo) }()
		_ = waitForKnowledgeBlockedByPID(t, ctx, db, applicationName, removePID, "FROM users", "FOR UPDATE")
	} else {
		go func() { mutationDone <- mutation.run(ctx, knowledgeRepo) }()
		mutationPID := waitForKnowledgeBlockedByPID(t, ctx, db, applicationName, gatePID, "FROM teams", "FOR UPDATE")
		go func() { removeDone <- remove() }()
		_ = waitForKnowledgeBlockedByPID(t, ctx, db, applicationName, mutationPID, "FROM users", "FOR UPDATE")
	}
	if err := gateTx.Commit(); err != nil {
		t.Fatalf("release team gate: %v", err)
	}
	if err := waitForKnowledgeRaceResult(t, ctx, removeDone); err != nil {
		t.Fatalf("membership removal: %v", err)
	}
	mutationErr := waitForKnowledgeRaceResult(t, ctx, mutationDone)
	if removalFirst {
		if !errors.Is(mutationErr, mutation.removalFirstWant) {
			t.Fatalf("removal-first mutation error = %v, want %v", mutationErr, mutation.removalFirstWant)
		}
	} else if mutationErr != nil {
		t.Fatalf("mutation-first mutation error = %v", mutationErr)
	}

	var membershipStatus string
	var membershipRevision int64
	var membershipEvents int
	if err := db.QueryRowContext(ctx, `SELECT status FROM team_memberships WHERE team_id=$1 AND user_id=$2`, membershipRaceTeamID, membershipRaceTargetID).Scan(&membershipStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT membership_revision FROM teams WHERE id=$1`, membershipRaceTeamID).Scan(&membershipRevision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='team.membership.changed' AND payload->>'operation'='removed'`, membershipRaceTeamID).Scan(&membershipEvents); err != nil {
		t.Fatal(err)
	}
	if membershipStatus != "removed" || membershipRevision != 2 || membershipEvents != 1 {
		t.Fatalf("membership status/revision/events = %s/%d/%d", membershipStatus, membershipRevision, membershipEvents)
	}
	mutation.assert(t, ctx, db, !removalFirst)
}

func seedMembershipKnowledgeRace(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	mustKnowledgeExec(t, ctx, db, `
INSERT INTO users(id,email,display_name) VALUES
  ($1,'race-remover@example.test','Race Remover'),
  ($2,'race-target@example.test','Race Target');
INSERT INTO teams(id,name,created_by_user_id) VALUES ($3,'Race Team',$1);
INSERT INTO team_memberships(team_id,user_id,role) VALUES
  ($3,$1,'admin'),($3,$2,'admin');
INSERT INTO knowledge_collections(id,name,scope,team_id)
VALUES ($4,'Shared','team',$3);
INSERT INTO knowledge_documents(id,collection_id,status)
VALUES ($5,$4,'processing');
INSERT INTO files(
  id,user_id,original_filename,mime_type,byte_size,sha256,upload_status,
  storage_backend,object_key,metadata
) VALUES ($6,$2,'race.pdf','application/pdf',10,$7,'available','local',$8,'{"purpose":"knowledge"}');
INSERT INTO knowledge_document_versions(
  id,document_id,file_id,source_version,status,content_hash
) VALUES ($9,$5,$6,1,'active',$7);
UPDATE knowledge_documents SET status='active',current_version_id=$9 WHERE id=$5
`, membershipRaceRemoverID, membershipRaceTargetID, membershipRaceTeamID,
		membershipRaceCollectionID, membershipRaceDocumentID, membershipRaceFileID,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"users/"+membershipRaceTargetID+"/files/"+membershipRaceFileID,
		membershipRaceVersionID)
	manifest := GovernanceManifest{
		Processor: "mineru", EndpointID: "default", ModelAPIVersion: "v1",
		AllowedPurposes: []string{"parse"}, AllowedDataTypes: []string{"application/pdf"},
		Region: "global", RetentionPolicy: "none", DeletionContract: "delete", TrainingUse: "disabled",
	}
	if _, err := NewGovernanceService(NewPostgresRepository(db)).Apply(ctx, manifest); err != nil {
		t.Fatalf("seed governance: %v", err)
	}
}

func waitForKnowledgeBlockedByPID(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	applicationName string,
	blockerPID int,
	queryFragments ...string,
) int {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var pid int
		args := []any{blockerPID, applicationName}
		query := `
SELECT pid FROM pg_stat_activity
WHERE pid <> $1
  AND application_name = $2
  AND wait_event_type = 'Lock'
  AND pg_blocking_pids(pid) = ARRAY[$1]::integer[]
`
		for _, fragment := range queryFragments {
			args = append(args, "%"+fragment+"%")
			query += "  AND query LIKE $" + fmt.Sprint(len(args)) + "\n"
		}
		query += "ORDER BY pid LIMIT 1"
		err := db.QueryRowContext(ctx, query, args...).Scan(&pid)
		if err == nil {
			return pid
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("query blocker pid %d: %v", blockerPID, err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("no session blocked by pid %d: %v", blockerPID, ctx.Err())
		case <-ticker.C:
		}
	}
}

func waitForKnowledgeRaceResult(t *testing.T, ctx context.Context, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		t.Fatalf("knowledge race timed out: %v", ctx.Err())
		return ctx.Err()
	}
}
