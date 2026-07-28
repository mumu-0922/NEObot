package usermemory

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

func TestPostgresMemoryPortabilityRoundTrip(t *testing.T) {
	db := openMemoryPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	codec, err := NewPortabilityPlanCodec(PortabilityPlanKeyring{
		ActiveKeyID: "test", Keys: map[string][]byte{"test": key},
	})
	if err != nil {
		t.Fatalf("NewPortabilityPlanCodec() error = %v", err)
	}
	service := NewService(
		repo,
		WithPortabilityPlanCodec(codec),
		WithPortabilityRelease("postgres-test"),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	userAID, _ := newUUID()
	userBID, _ := newUUID()
	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, display_name) VALUES
  ($1, 'portability source'), ($2, 'portability target')
`, userAID, userBID); err != nil {
		t.Fatalf("insert portability users: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{userAID, userBID})
	})
	ctxA := auth.WithUser(ctx, auth.User{ID: userAID, DisplayName: "A"})
	ctxB := auth.WithUser(ctx, auth.User{ID: userBID, DisplayName: "B"})
	project, err := service.CreateProject(ctxA, "Portable project", "project metadata")
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	created, err := service.CreateManual(ctxA, Candidate{
		Type: "preference", Content: "Prefer concise answers",
		Importance: 4, Tags: []string{"style"},
	})
	if err != nil {
		t.Fatalf("CreateManual() error = %v", err)
	}
	updated, err := service.Update(ctxA, created.ID, Candidate{
		Type: "preference", Content: "Keep answers concise",
		Importance: 5, Tags: []string{"style"},
	})
	if err != nil || updated.Revision != 2 {
		t.Fatalf("Update() = %#v/%v", updated, err)
	}
	replacement, err := service.CreateManual(ctxA, Candidate{
		Type: "preference", Content: "Use short answers",
		Importance: 5, Tags: []string{"style"},
	})
	if err != nil {
		t.Fatalf("CreateManual(replacement) error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
SELECT memory_governance_append_revision(memory, memory.content_hash, 'supersede')
FROM user_memories memory
WHERE memory.id = $1 AND memory.user_id = $2
`, created.ID, userAID); err != nil {
		t.Fatalf("append supersession fixture revision: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE user_memories
SET lifecycle_status = 'superseded', enabled = false,
  superseded_by_memory_id = $1, revision = revision + 1,
  updated_at = clock_timestamp()
WHERE id = $2 AND user_id = $3
`, replacement.ID, created.ID, userAID); err != nil {
		t.Fatalf("supersede fixture memory: %v", err)
	}
	currentContent := "Keep answers especially concise"
	currentHash := contentSHA256(currentContent)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin restored-current fixture: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
SELECT memory_governance_append_revision(memory, $1, 'update')
FROM user_memories memory
WHERE memory.id = $2 AND memory.user_id = $3
`, currentHash, created.ID, userAID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("append restored-current fixture revision: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE user_memories
SET content = $1, normalized_content = $2, content_hash = $3,
  enabled = true, lifecycle_status = 'active',
  superseded_by_memory_id = NULL, revision = revision + 1,
  updated_at = clock_timestamp()
WHERE id = $4 AND user_id = $5
`, currentContent, normalizeSearchText(currentContent), currentHash,
		created.ID, userAID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("restore current fixture memory: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit restored-current fixture: %v", err)
	}
	var sourceSupersessionTarget string
	if err := db.QueryRowContext(ctx, `
SELECT target.content
FROM user_memory_revisions revision
JOIN user_memories target
  ON target.id = revision.prior_superseded_by_memory_id
  AND target.user_id = revision.user_id
WHERE revision.memory_id = $1
  AND revision.prior_superseded_by_memory_id IS NOT NULL
`, created.ID).Scan(&sourceSupersessionTarget); err != nil ||
		sourceSupersessionTarget != replacement.Content {
		t.Fatalf(
			"source revision supersession target = %q/%v, want %q",
			sourceSupersessionTarget, err, replacement.Content,
		)
	}

	var encrypted bytes.Buffer
	exported, err := service.ExportMemoryPackage(
		ctxA, &encrypted, "fixture-passphrase", true,
	)
	if err != nil {
		t.Fatalf("ExportMemoryPackage() error = %v", err)
	}
	if exported.Manifest.Counts.Projects != 1 ||
		exported.Manifest.Counts.Memories != 2 ||
		exported.Manifest.Counts.Revisions != 3 {
		t.Fatalf("exported manifest = %#v", exported.Manifest)
	}
	portableMemoryRefs := make(map[string]string)
	var portableSupersessionTarget string
	if _, err := ParseEncryptedMemoryPackage(
		bytes.NewReader(encrypted.Bytes()),
		"fixture-passphrase",
		MemoryPackageVisitorFuncs{
			Memory: func(_ int, record PortableMemoryRecord) error {
				portableMemoryRefs[record.Ref] = record.Content
				return nil
			},
			Revision: func(_ int, record PortableRevisionRecord) error {
				if record.Prior != nil && record.Prior.SupersededByRef != "" {
					portableSupersessionTarget = record.Prior.SupersededByRef
				}
				return nil
			},
		},
	); err != nil || portableMemoryRefs[portableSupersessionTarget] != replacement.Content {
		t.Fatalf(
			"exported supersession ref = %q (%q)/%v, want target content %q",
			portableSupersessionTarget, portableMemoryRefs[portableSupersessionTarget],
			err, replacement.Content,
		)
	}
	mappings := ImportMappings{Projects: map[string]ImportProjectMapping{
		"project-000001": {Mode: "create"},
	}}
	dryRun, err := service.DryRunMemoryImport(
		ctxB, bytes.NewReader(encrypted.Bytes()), "fixture-passphrase", mappings,
	)
	if err != nil {
		t.Fatalf("DryRunMemoryImport() error = %v", err)
	}
	if dryRun.Counts["ADD"] != 2 || len(dryRun.ScopeRequirements) != 0 {
		t.Fatalf("dry-run = %#v", dryRun)
	}
	confirmed, err := service.ConfirmMemoryImport(
		ctxB,
		bytes.NewReader(encrypted.Bytes()),
		"fixture-passphrase",
		mappings,
		dryRun.PlanToken,
	)
	if err != nil {
		t.Fatalf("ConfirmMemoryImport() error = %v", err)
	}
	if confirmed.AddedProjects != 1 || confirmed.AddedMemories != 2 {
		t.Fatalf("confirmed = %#v", confirmed)
	}
	replayed, err := service.ConfirmMemoryImport(
		ctxB,
		bytes.NewReader(encrypted.Bytes()),
		"fixture-passphrase",
		mappings,
		dryRun.PlanToken,
	)
	if err != nil || replayed.ImportID != confirmed.ImportID ||
		replayed.AddedProjects != confirmed.AddedProjects ||
		replayed.AddedMemories != confirmed.AddedMemories {
		t.Fatalf("replayed confirm = %#v/%v, original=%#v", replayed, err, confirmed)
	}
	var source, authority, importedSupersededTarget string
	var revision int64
	var evidenceCount, importedRevisionCount, batchCount int
	if err := db.QueryRowContext(ctx, `
SELECT source, authority_kind, revision
FROM user_memories
WHERE user_id = $1 AND content = $2
`, userBID, currentContent).Scan(&source, &authority, &revision); err != nil {
		t.Fatalf("read imported memory: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM user_memory_evidence WHERE user_id = $1
`, userBID).Scan(&evidenceCount); err != nil {
		t.Fatalf("read imported evidence count: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM user_memory_revisions
WHERE user_id = $1 AND actor_type = 'import'
`, userBID).Scan(&importedRevisionCount); err != nil {
		t.Fatalf("read imported revision count: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT target.content
FROM user_memory_revisions revision
JOIN user_memories target
  ON target.id = revision.prior_superseded_by_memory_id
  AND target.user_id = revision.user_id
WHERE revision.user_id = $1
  AND revision.actor_type = 'import'
  AND revision.prior_superseded_by_memory_id IS NOT NULL
`, userBID).Scan(&importedSupersededTarget); err != nil {
		t.Fatalf("read imported revision supersession target: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM memory_import_batches
WHERE user_id = $1 AND status = 'completed'
`, userBID).Scan(&batchCount); err != nil {
		t.Fatalf("read import batch count: %v", err)
	}
	if source != "import" || authority != "import" || revision != 4 ||
		evidenceCount != 0 || importedRevisionCount != 3 || batchCount != 1 ||
		importedSupersededTarget != "Use short answers" {
		t.Fatalf(
			"imported source=%q authority=%q revision=%d evidence=%d revisions=%d batches=%d superseded_target=%q sourceProject=%s",
			source, authority, revision, evidenceCount, importedRevisionCount, batchCount,
			importedSupersededTarget, project.ID,
		)
	}
}

func TestPostgresMemoryPortabilityRuntimePrivileges(t *testing.T) {
	db := openMemoryPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	for _, role := range []string{"go_api_runtime", "memory_worker_runtime"} {
		for _, table := range []string{
			"memory_import_batches",
			"memory_deletion_replay_entries",
		} {
			var hasDirectCRUD bool
			if err := db.QueryRowContext(ctx, `
SELECT has_table_privilege($1, $2, 'SELECT')
  OR has_table_privilege($1, $2, 'INSERT')
  OR has_table_privilege($1, $2, 'UPDATE')
  OR has_table_privilege($1, $2, 'DELETE')
`, role, table).Scan(&hasDirectCRUD); err != nil {
				t.Fatalf("inspect %s privileges on %s: %v", role, table, err)
			}
			if hasDirectCRUD {
				t.Errorf("%s has forbidden direct CRUD on %s", role, table)
			}
		}
	}

	functions := []string{
		"memory_portability_authority_state(uuid)",
		"memory_portability_export_records(uuid,boolean)",
		"memory_portability_completed_import(uuid,uuid,text,text,text,text,text)",
		"memory_portability_begin_import(uuid,uuid,text,text,text,text,text,integer,integer,integer)",
		"memory_portability_replay_deletion(jsonb)",
		"memory_portability_rebuild_projections()",
	}
	for _, function := range functions {
		var apiCanExecute, workerCanExecute bool
		if err := db.QueryRowContext(ctx, `
SELECT has_function_privilege('go_api_runtime', $1, 'EXECUTE'),
  has_function_privilege('memory_worker_runtime', $1, 'EXECUTE')
`, function).Scan(&apiCanExecute, &workerCanExecute); err != nil {
			t.Fatalf("inspect runtime privileges on %s: %v", function, err)
		}
		if !apiCanExecute || workerCanExecute {
			t.Errorf(
				"function %s execute privileges api=%t worker=%t, want true/false",
				function, apiCanExecute, workerCanExecute,
			)
		}
	}
}

func TestPostgresMemoryDeletionReplayAndProjectionRebuild(t *testing.T) {
	db := openMemoryPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	service := NewService(repo)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	userID, _ := newUUID()
	manifestID, _ := newUUID()
	eventID, _ := newUUID()
	tombstoneID, _ := newUUID()
	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, display_name) VALUES ($1, 'deletion replay target')
`, userID); err != nil {
		t.Fatalf("insert replay user: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `
DELETE FROM memory_deletion_replay_entries WHERE manifest_id = $1;
DELETE FROM user_memory_deletion_manifests WHERE manifest_id = $1;
DELETE FROM users WHERE id = $2;
`, manifestID, userID)
	})
	userCtx := auth.WithUser(ctx, auth.User{ID: userID, DisplayName: "replay"})
	target, err := service.CreateManual(userCtx, Candidate{
		Type: "fact", Content: "Restore replay secret fixture",
		Importance: 4, Tags: []string{"restore"},
	})
	if err != nil {
		t.Fatalf("CreateManual(target) error = %v", err)
	}
	target, err = service.Update(userCtx, target.ID, Candidate{
		Type: "fact", Content: "Restore replay current fixture",
		Importance: 5, Tags: []string{"restore", "current"},
	})
	if err != nil {
		t.Fatalf("Update(target) error = %v", err)
	}
	if _, err := service.CreateManual(userCtx, Candidate{
		Type: "preference", Content: "Keep the surviving replay fixture",
		Importance: 3, Tags: []string{"survivor"},
	}); err != nil {
		t.Fatalf("CreateManual(survivor) error = %v", err)
	}

	entry := PortableDeletionEntry{
		Kind: "deletion", ManifestID: manifestID, EventID: eventID,
		UserID: userID, MemoryID: target.ID, TombstoneID: tombstoneID,
		ContentHash: contentSHA256(target.Content), ScopeGeneration: 1, VisibilityEpoch: 1,
		DeletedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		ResultCode: "PENDING",
	}
	var plaintext bytes.Buffer
	if err := WriteDeletionPackage(&plaintext, DeletionPackageManifest{
		CreatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		ExporterRelease: "postgres-test",
	}, []PortableDeletionEntry{entry}); err != nil {
		t.Fatalf("WriteDeletionPackage() error = %v", err)
	}
	var encrypted bytes.Buffer
	if err := EncryptPortabilityStream(
		&encrypted, "fixture-passphrase", func(writer io.Writer) error {
			_, err := writer.Write(plaintext.Bytes())
			return err
		},
	); err != nil {
		t.Fatalf("EncryptPortabilityStream() error = %v", err)
	}

	result, err := ReplayEncryptedDeletionPackage(
		ctx, repo, bytes.NewReader(encrypted.Bytes()), "fixture-passphrase",
	)
	if err != nil {
		t.Fatalf("ReplayEncryptedDeletionPackage() error = %v", err)
	}
	if result.Entries != 1 || result.Replayed != 1 ||
		result.ProjectionRebuilt != 1 {
		t.Fatalf("replay result = %#v", result)
	}

	var content, normalized, tags, manifestResult, replayResult string
	var enabled bool
	var deleted bool
	var evidenceCount, revisionPlaintext, targetProjection, survivorProjection int
	err = db.QueryRowContext(ctx, `
SELECT content, normalized_content, array_to_string(tags, ','), enabled,
  deleted_at IS NOT NULL
FROM user_memories WHERE id = $1 AND user_id = $2
`, target.ID, userID).Scan(&content, &normalized, &tags, &enabled, &deleted)
	if err != nil {
		t.Fatalf("read replayed memory: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM user_memory_evidence WHERE memory_id = $1),
  (SELECT count(*) FROM user_memory_revisions
    WHERE memory_id = $1 AND prior_content_snapshot IS NOT NULL),
  (SELECT count(*) FROM user_memory_search_projections WHERE memory_id = $1),
  (SELECT count(*) FROM user_memory_search_projections
    WHERE user_id = $2 AND memory_id <> $1),
  (SELECT result_code FROM user_memory_deletion_manifests WHERE manifest_id = $3),
  (SELECT result_code FROM memory_deletion_replay_entries WHERE manifest_id = $3)
`, target.ID, userID, manifestID).Scan(
		&evidenceCount, &revisionPlaintext, &targetProjection, &survivorProjection,
		&manifestResult, &replayResult,
	); err != nil {
		t.Fatalf("read replay authority: %v", err)
	}
	if content != "" || normalized != "" || tags != "" || enabled || !deleted ||
		evidenceCount != 0 || revisionPlaintext != 0 || targetProjection != 0 ||
		survivorProjection != 1 || manifestResult != "ONLINE_PURGED" ||
		replayResult != "REPLAYED" {
		t.Fatalf(
			"replayed content=%q normalized=%q tags=%q enabled=%t deleted=%t evidence=%d revision_plaintext=%d target_projection=%d survivor_projection=%d manifest=%q authority=%q",
			content, normalized, tags, enabled, deleted, evidenceCount,
			revisionPlaintext, targetProjection, survivorProjection,
			manifestResult, replayResult,
		)
	}

	replayed, err := ReplayEncryptedDeletionPackage(
		ctx, repo, bytes.NewReader(encrypted.Bytes()), "fixture-passphrase",
	)
	if err != nil || replayed.AlreadyApplied != 1 || replayed.Replayed != 0 {
		t.Fatalf("idempotent replay = %#v/%v", replayed, err)
	}

	var exported bytes.Buffer
	exportedManifest, err := ExportDeletionPackage(
		ctx, repo, &exported, "fixture-passphrase", "postgres-test",
	)
	if err != nil || exportedManifest.Count < 1 {
		t.Fatalf("ExportDeletionPackage() = %#v/%v", exportedManifest, err)
	}
	found := false
	if _, err := ParseEncryptedDeletionPackage(
		bytes.NewReader(exported.Bytes()), "fixture-passphrase",
		func(exportedEntry PortableDeletionEntry) error {
			if exportedEntry.ManifestID == manifestID {
				found = exportedEntry.ResultCode == "ONLINE_PURGED" &&
					exportedEntry.PurgedAt != ""
			}
			return nil
		},
	); err != nil || !found {
		t.Fatalf("parse exported deletions found=%t err=%v", found, err)
	}
}
