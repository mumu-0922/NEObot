package migration

import (
	"context"
	"strings"
	"testing"
	"time"
)

var phase151DAllMigrationFiles = append(
	append([]string{}, phase151CAllMigrationFiles...),
	phase151DUpPath,
	phase151DDownPath,
)

func TestPhase151DKnowledgeServicesReplay(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	baseRunner := NewRunner(db, phase151CMigrationFS(t, phase151CAllMigrationFiles...))
	if _, err := baseRunner.Up(ctx); err != nil {
		t.Fatalf("apply 001-005 migrations: %v", err)
	}

	const (
		userID          = "00000000-0000-0000-0000-000000001001"
		collectionID    = "00000000-0000-0000-0000-000000001101"
		newCollectionID = "00000000-0000-0000-0000-000000001102"
		documentID      = "00000000-0000-0000-0000-000000001201"
		versionID       = "00000000-0000-0000-0000-000000001301"
		fileID          = "00000000-0000-0000-0000-000000001401"
		profileID       = "00000000-0000-0000-0000-000000001501"
		consentID       = "00000000-0000-0000-0000-000000001601"
		jobID           = "00000000-0000-0000-0000-000000001701"
	)

	mustExecPhase151C(t, ctx, db, `
INSERT INTO users (id, email, display_name)
VALUES ($1, 'phase15d@example.test', 'Phase 15D User')
`, userID)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_collections (id, name, scope, owner_user_id)
VALUES ($1, 'Legacy Collection', 'personal', $2)
`, collectionID, userID)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_documents (id, collection_id)
VALUES ($1, $2)
`, documentID, collectionID)

	fullRunner := NewRunner(db, phase151CMigrationFS(t, phase151DAllMigrationFiles...))
	applied, err := fullRunner.Up(ctx)
	if err != nil {
		t.Fatalf("apply 006 migration: %v", err)
	}
	if len(applied) != 1 || applied[0].ID() != "006_phase15_knowledge_services" {
		t.Fatalf("apply 006 migration = %#v", applied)
	}

	for _, column := range []string{
		"description", "icon", "color", "created_by_user_id",
		"idempotency_key", "create_request_hash",
	} {
		assertPhase151CColumnExists(t, ctx, db, "knowledge_collections", column)
	}
	assertPhase151CColumnExists(t, ctx, db, "knowledge_documents", "visibility_epoch")
	assertPhase151CTableExists(t, ctx, db, "knowledge_processing_jobs")

	var description, icon, color string
	var creator *string
	if err := db.QueryRowContext(ctx, `
SELECT description, icon, color, created_by_user_id
FROM knowledge_collections
WHERE id = $1
`, collectionID).Scan(&description, &icon, &color, &creator); err != nil {
		t.Fatalf("read migrated collection defaults: %v", err)
	}
	if description != "" || icon != "Folder" || color != "blue" || creator != nil {
		t.Fatalf("legacy collection defaults = %q/%q/%q/%v", description, icon, color, creator)
	}

	requestHash := strings.Repeat("a", 64)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_collections (
  id, name, description, icon, color, scope, owner_user_id,
  created_by_user_id, idempotency_key, create_request_hash
) VALUES ($1, 'New Collection', 'Description', 'BookText', 'purple', 'personal', $2, $2, 'collection-1', $3)
`, newCollectionID, userID, requestHash)
	assertPhase151CUniqueViolation(t, mustExecPhase151CReturnError(ctx, db, `
INSERT INTO knowledge_collections (
  id, name, scope, owner_user_id, created_by_user_id, idempotency_key, create_request_hash
) VALUES ('00000000-0000-0000-0000-000000001103', 'Duplicate', 'personal', $1, $1, 'collection-1', $2)
`, userID, strings.Repeat("b", 64)))

	mustExecPhase151C(t, ctx, db, `
INSERT INTO files (
  id, user_id, original_filename, mime_type, byte_size, sha256,
  storage_backend, object_key, metadata
) VALUES ($1, $2, 'source.pdf', 'application/pdf', 10, $3, 'local', $4, '{"purpose":"knowledge"}'::jsonb)
`, fileID, userID, strings.Repeat("c", 64), "users/"+userID+"/files/"+fileID)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_document_versions (
  id, document_id, file_id, source_version, status, content_hash,
  created_by_user_id, idempotency_key, request_hash
) VALUES ($1, $2, $3, 1, 'uploaded', $4, $5, 'version-1', $6)
`, versionID, documentID, fileID, strings.Repeat("c", 64), userID, requestHash)
	assertPhase151CUniqueViolation(t, mustExecPhase151CReturnError(ctx, db, `
INSERT INTO knowledge_document_versions (
  id, document_id, file_id, source_version, status, content_hash,
  created_by_user_id, idempotency_key, request_hash
) VALUES ('00000000-0000-0000-0000-000000001302', $1, $2, 2, 'processing', $3, $4, 'version-2', $5)
`, documentID, fileID, strings.Repeat("c", 64), userID, requestHash))

	mustExecPhase151C(t, ctx, db, `
INSERT INTO processor_governance_profiles (
  id, processor, endpoint_id, model_api_version, allowed_purposes,
  allowed_data_types, region, retention_policy, deletion_contract,
  training_use, status, governance_revision, manifest_hash
) VALUES (
  $1, 'mineru', 'default', 'v1', ARRAY['parse'], ARRAY['application/pdf'],
  'global', 'none', 'delete-on-request', 'disabled', 'approved', 1, $2
)
`, profileID, strings.Repeat("d", 64))
	mustExecPhase151C(t, ctx, db, `
INSERT INTO processor_governance_heads (
  processor, endpoint_id, status, active_profile_id,
  active_governance_revision, head_revision
) VALUES ('mineru', 'default', 'active', $1, 1, 1)
`, profileID)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO processing_consents (
  id, scope, collection_id, processor, endpoint_id,
  governance_profile_id, governance_revision, governance_head_revision,
  purposes, data_types, policy_version, decision, consent_revision,
  granted_by_user_id
) VALUES (
  $1, 'collection', $2, 'mineru', 'default', $3, 1, 1,
  ARRAY['parse'], ARRAY['application/pdf'], 'v1', 'granted', 1, $4
)
`, consentID, collectionID, profileID, userID)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_processing_jobs (
  id, collection_id, document_id, document_version_id, file_id,
  stage, operation, processor, endpoint_id, governance_profile_id,
  governance_revision, governance_head_revision,
  collection_consent_id, collection_consent_revision,
  collection_acl_revision, collection_visibility_epoch,
  collection_processing_revision, document_visibility_epoch,
  requested_by_user_id, idempotency_scope, idempotency_key, request_hash
) VALUES (
  $1, $2, $3, $4, $5, 'parse', 'initial', 'mineru', 'default', $6,
  1, 1, $7, 1, 1, 1, 1, 1, $8, $9, 'job-1', $10
)
`, jobID, collectionID, documentID, versionID, fileID, profileID, consentID,
		userID, "document:"+documentID+":initial", requestHash)

	rolledBack, err := fullRunner.Down(ctx, false)
	if err != nil {
		t.Fatalf("rollback 006 migration: %v", err)
	}
	if len(rolledBack) != 1 || rolledBack[0].ID() != "006_phase15_knowledge_services" {
		t.Fatalf("rollback 006 migration = %#v", rolledBack)
	}
	assertPhase151CTableAbsent(t, ctx, db, "knowledge_processing_jobs")
	assertPhase151CColumnAbsent(t, ctx, db, "knowledge_collections", "description")
	assertPhase151CColumnAbsent(t, ctx, db, "knowledge_documents", "visibility_epoch")

	applied, err = fullRunner.Up(ctx)
	if err != nil {
		t.Fatalf("reapply 006 migration: %v", err)
	}
	if len(applied) != 1 || applied[0].ID() != "006_phase15_knowledge_services" {
		t.Fatalf("reapply 006 migration = %#v", applied)
	}
	assertPhase151CTableExists(t, ctx, db, "knowledge_processing_jobs")
	assertPhase151CColumnExists(t, ctx, db, "knowledge_documents", "visibility_epoch")
}
