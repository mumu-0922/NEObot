package migration

import "testing"

const (
	phase151DUpPath   = "006_phase15_knowledge_services.up.sql"
	phase151DDownPath = "006_phase15_knowledge_services.down.sql"
)

func TestPhase151DKnowledgeServicesSchemaContract(t *testing.T) {
	up := readPhase15SQL(t, phase151DUpPath)
	down := readPhase15SQL(t, phase151DDownPath)

	t.Run("adds bounded collection display and replay identity", func(t *testing.T) {
		ddl := phase15AlterTableDDL(up, "knowledge_collections")
		assertPhase15Fragments(t, ddl,
			"collections must preserve current frontend display fields",
			"description text not null default ''",
			"icon text not null default 'folder'",
			"color text not null default 'blue'",
			"octet_length ( description ) <= 8192",
			"knowledge_collections_icon_check",
			"knowledge_collections_color_check",
		)
		assertPhase15Fragments(t, ddl,
			"collection idempotency must bind actor key and canonical request hash",
			"created_by_user_id uuid references users ( id ) on delete restrict",
			"octet_length ( idempotency_key ) between 1 and 128",
			"create_request_hash ~ '^[0-9a-f]{64}$'",
		)
		assertPhase151CPartialUniqueIndex(t, up,
			"collection idempotency must be creator-scoped",
			"idx_knowledge_collections_creator_idempotency",
			"knowledge_collections",
			[]string{"created_by_user_id", "idempotency_key"},
			[]string{"idempotency_key is not null"},
		)
	})

	t.Run("adds independent document visibility and operation idempotency", func(t *testing.T) {
		documents := phase15AlterTableDDL(up, "knowledge_documents")
		versions := phase15AlterTableDDL(up, "knowledge_document_versions")
		assertPhase15Fragments(t, documents,
			"logical documents need an independent visibility fence",
			"visibility_epoch bigint not null default 1",
			"knowledge_documents_visibility_epoch_positive",
			"visibility_epoch >= 1",
		)
		for _, ddl := range []string{documents, versions} {
			assertPhase15Fragments(t, ddl,
				"document writes must preserve actor and idempotency state",
				"created_by_user_id uuid references users ( id ) on delete restrict",
				"idempotency_key text",
			)
		}
		assertPhase151CPartialUniqueIndex(t, up,
			"only one uploaded or processing version may exist per document",
			"idx_knowledge_document_versions_one_nonterminal",
			"knowledge_document_versions",
			[]string{"document_id"},
			[]string{"status in ( 'uploaded' , 'processing' )"},
		)
	})

	t.Run("creates stage-specific durable processing jobs", func(t *testing.T) {
		jobs := mustPhase15TableBody(t, up, "knowledge_processing_jobs")
		assertPhase15Columns(t, jobs, "knowledge_processing_jobs",
			"collection_id", "document_id", "document_version_id", "file_id",
			"stage", "operation", "processor", "endpoint_id",
			"governance_profile_id", "governance_revision", "governance_head_revision",
			"collection_consent_id", "collection_consent_revision",
			"collection_acl_revision", "collection_visibility_epoch",
			"collection_processing_revision", "document_visibility_epoch",
			"idempotency_scope", "idempotency_key", "request_hash",
			"status", "attempt_count", "max_attempts", "available_at",
			"lease_owner", "lease_expires_at", "completed_at", "error_code")
		assertPhase15Fragments(t, jobs,
			"processing and purge jobs must have disjoint authority shapes",
			"stage in ( 'parse' , 'passage_embedding' , 'purge' )",
			"operation in ( 'initial' , 'replace' , 'reprocess' , 'purge' )",
			"stage in ( 'parse' , 'passage_embedding' )",
			"collection_consent_id is not null",
			"stage = 'purge'",
			"collection_consent_id is null",
		)
		assertPhase15Fragments(t, jobs,
			"job retries must be leased bounded and terminally fenced",
			"status in ( 'pending' , 'processing' , 'succeeded' , 'failed' , 'cancelled' )",
			"max_attempts between 1 and 32",
			"status = 'processing'",
			"lease_owner is not null",
			"status = 'succeeded'",
			"completed_at is not null",
			"error_code is null or error_code ~ '^[a-z0-9_]{1 , 64}$'",
		)
		assertPhase15CompositeForeignKey(t,
			phase15TableDDL(t, up, "knowledge_processing_jobs"),
			"knowledge_document_versions",
			map[string]string{
				"document_id":         "document_id",
				"document_version_id": "id",
				"file_id":             "file_id",
			},
			"processing jobs must pin a version and file to the same logical document",
		)
		assertPhase15CompositeForeignKey(t,
			phase15TableDDL(t, up, "knowledge_processing_jobs"),
			"knowledge_documents",
			map[string]string{
				"collection_id": "collection_id",
				"document_id":   "id",
			},
			"processing jobs must pin the document to the authorized collection",
		)
		assertPhase15CompositeForeignKey(t,
			phase15TableDDL(t, up, "knowledge_processing_jobs"),
			"processing_consents",
			map[string]string{
				"collection_id":               "collection_id",
				"collection_consent_id":       "id",
				"processor":                   "processor",
				"endpoint_id":                 "endpoint_id",
				"governance_profile_id":       "governance_profile_id",
				"governance_revision":         "governance_revision",
				"governance_head_revision":    "governance_head_revision",
				"collection_consent_revision": "consent_revision",
			},
			"processing jobs must pin the exact collection consent and governance snapshot",
		)
		assertPhase15CompositeForeignKey(t,
			phase15TableDDL(t, up, "knowledge_processing_jobs"),
			"knowledge_processing_jobs",
			map[string]string{
				"document_id":         "document_id",
				"document_version_id": "document_version_id",
				"caused_by_job_id":    "id",
			},
			"a downstream job must be caused by a job for the same document version",
		)
		assertPhase151CPartialIndex(t, up,
			"pending jobs must be claimable by availability and id",
			"idx_knowledge_processing_jobs_pending",
			"knowledge_processing_jobs",
			[]string{"available_at", "id"},
			[]string{"status = 'pending'"},
		)
		assertPhase151CPartialIndex(t, up,
			"processing jobs must be reclaimable by lease expiry and id",
			"idx_knowledge_processing_jobs_processing",
			"knowledge_processing_jobs",
			[]string{"lease_expires_at", "id"},
			[]string{"status = 'processing'"},
		)
	})

	t.Run("down removes every phase 15.1d object", func(t *testing.T) {
		for _, table := range phase15CreatedTables(up) {
			if !phase15DropsTable(down, table) {
				t.Errorf("Phase 15.1D down does not drop up-created table %q", table)
			}
		}
		for _, index := range phase15CreatedIndexes(up) {
			if !phase15DropsIndex(down, index) {
				t.Errorf("Phase 15.1D down does not drop up-created index %q", index)
			}
		}
		assertPhase15Order(t, down,
			"drop table if exists knowledge_processing_jobs",
			"drop constraint if exists processing_consents_collection_job_binding_unique",
			"jobs must be removed before their supporting consent uniqueness",
		)
		for _, column := range []string{
			"description", "icon", "color", "created_by_user_id",
			"idempotency_key", "create_request_hash", "visibility_epoch",
			"request_hash", "error_code",
		} {
			assertPhase15Fragments(t, down,
				"Phase 15.1D down must remove every additive compatibility column",
				"drop column if exists "+column,
			)
		}
	})
}
