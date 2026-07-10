package migration

import (
	"regexp"
	"testing"
)

const (
	phase15UpPath   = "004_phase15_identity_knowledge_acl.up.sql"
	phase15DownPath = "004_phase15_identity_knowledge_acl.down.sql"
)

func TestPhase15IdentityKnowledgeACLSchemaContract(t *testing.T) {
	up := readPhase15SQL(t, phase15UpPath)
	down := readPhase15SQL(t, phase15DownPath)

	t.Run("creates authoritative tables", func(t *testing.T) {
		for _, table := range []string{
			"user_credentials",
			"credential_recovery_tokens",
			"teams",
			"team_memberships",
			"team_invites",
			"knowledge_collections",
			"knowledge_documents",
			"knowledge_document_versions",
			"user_query_consent_state",
			"processor_governance_profiles",
			"processor_governance_heads",
			"processing_consents",
			"knowledge_outbox",
		} {
			if _, ok := phase15TableBody(up, table); !ok {
				t.Errorf("missing Phase 15 authoritative table %q", table)
			}
		}
	})

	t.Run("collection has exactly one personal or team scope", func(t *testing.T) {
		body := mustPhase15TableBody(t, up, "knowledge_collections")
		assertPhase15Columns(t, body, "knowledge_collections",
			"scope", "owner_user_id", "team_id")
		assertPhase15Fragments(t, body,
			"knowledge_collections must constrain personal scope to an owner and no team",
			"check", "scope", "'personal'", "owner_user_id is not null", "team_id is null")
		assertPhase15Fragments(t, body,
			"knowledge_collections must constrain team scope to a team and no personal owner",
			"check", "scope", "'team'", "team_id is not null", "owner_user_id is null")
		assertPhase15ReferenceOnDeleteRestrict(t, body,
			"owner_user_id", "users",
			"personal collections must not be orphaned by user deletion")
		assertPhase15ReferenceOnDeleteRestrict(t, body,
			"team_id", "teams",
			"team collections must not be orphaned by team deletion")
	})

	t.Run("logical documents and immutable versions are split", func(t *testing.T) {
		documents := mustPhase15TableBody(t, up, "knowledge_documents")
		versions := mustPhase15TableBody(t, up, "knowledge_document_versions")

		assertPhase15Columns(t, documents, "knowledge_documents",
			"id", "collection_id", "current_version_id", "status",
			"current_version_status")
		assertPhase15Columns(t, versions, "knowledge_document_versions",
			"id", "document_id", "file_id", "source_version",
			"visibility_epoch", "status", "content_hash")
		assertPhase15Fragments(t, versions,
			"knowledge document versions must make (document_id, source_version) unique",
			"unique", "document_id", "source_version")
		assertPhase15Fragments(t, documents,
			"new logical documents must remain non-serving until their first version is active",
			"status text not null default 'processing'",
			"status = 'processing'", "current_version_id is null",
			"status = 'active'", "current_version_id is not null")
		assertPhase15Fragments(t, versions,
			"failed document versions must have an explicit terminal candidate state",
			"knowledge_document_versions_status_check", "status in", "'failed'")

		assertPhase15CompositeForeignKey(t,
			phase15TableDDL(t, up, "knowledge_documents"),
			"knowledge_document_versions",
			map[string]string{
				"id":                 "document_id",
				"current_version_id": "id",
			},
			"knowledge_documents.current_version_id must use a composite FK that pins the version to the same logical document",
		)
		assertPhase15Fragments(t, documents,
			"active logical documents must require an active current version",
			"current_version_status text generated always as",
			"when status = 'active' then 'active'")
		assertPhase15CompositeForeignKey(t,
			phase15TableDDL(t, up, "knowledge_documents"),
			"knowledge_document_versions",
			map[string]string{
				"id":                     "document_id",
				"current_version_id":     "id",
				"current_version_status": "status",
			},
			"active logical documents must pin current_version_id to an active version of the same document",
		)

		assertPhase15ReferenceOnDeleteRestrict(t, documents,
			"collection_id", "knowledge_collections",
			"logical documents must not outlive their authoritative collection")
		assertPhase15ReferenceOnDeleteRestrict(t, versions,
			"document_id", "knowledge_documents",
			"document versions must not be detached from their logical document")
		assertPhase15ReferenceOnDeleteRestrict(t, versions,
			"file_id", "files",
			"knowledge document file bindings must block direct file deletion")
	})

	t.Run("identity email uniqueness is case insensitive without cross-clock verification ordering", func(t *testing.T) {
		caseInsensitiveEmailIndex := regexp.MustCompile(
			`\bcreate\s+unique\s+index\s+[a-z_][a-z0-9_]*\s+` +
				`on\s+(?:public\.)?users\s*\(\s*lower\s*\(\s*email\s*\)\s*\)` +
				`(?:\s+where\s+email\s+is\s+not\s+null)?\s*;`,
		)
		if !caseInsensitiveEmailIndex.MatchString(up) {
			t.Error("users.email must have a case-insensitive unique index")
		}

		credentials := mustPhase15TableBody(t, up, "user_credentials")
		assertPhase15Fragments(t, credentials,
			"credentials require an Argon2id hash and verified mailbox timestamp",
			"password_hash text not null", "email_verified_at timestamptz not null",
			"password_hash ~ '^\\$argon2id\\$'")
		crossClockOrdering := regexp.MustCompile(
			`check\s*\([^)]*(?:email_verified_at[^)]*created_at|created_at[^)]*email_verified_at)`,
		)
		if crossClockOrdering.MatchString(credentials) {
			t.Error("email verification must not compare application time with the database-created timestamp")
		}
	})

	t.Run("collection query and governance revisions stay independent", func(t *testing.T) {
		assertPhase15Columns(t,
			mustPhase15TableBody(t, up, "teams"),
			"teams", "membership_revision")
		assertPhase15Columns(t,
			mustPhase15TableBody(t, up, "knowledge_collections"),
			"knowledge_collections", "acl_revision", "visibility_epoch",
			"collection_processing_revision")
		assertPhase15Columns(t,
			mustPhase15TableBody(t, up, "user_query_consent_state"),
			"user_query_consent_state", "query_consent_revision")
		assertPhase15Columns(t,
			mustPhase15TableBody(t, up, "processor_governance_profiles"),
			"processor_governance_profiles", "governance_revision")
		assertPhase15Columns(t,
			mustPhase15TableBody(t, up, "processor_governance_heads"),
			"processor_governance_heads", "head_revision")
	})

	t.Run("governance head shape and active profile are pinned", func(t *testing.T) {
		head := mustPhase15TableBody(t, up, "processor_governance_heads")
		assertPhase15Columns(t, head, "processor_governance_heads",
			"processor", "endpoint_id", "status", "active_profile_id",
			"active_governance_revision", "active_profile_status",
			"head_revision")
		assertPhase15Fragments(t, head,
			"active governance heads must have an active profile and revision",
			"check", "status", "'active'", "active_profile_id is not null",
			"active_governance_revision is not null")
		assertPhase15Fragments(t, head,
			"disabled governance heads must clear the active profile and revision",
			"check", "status", "'disabled'", "active_profile_id is null",
			"active_governance_revision is null")
		assertPhase15Fragments(t, head,
			"active governance heads must resolve only to approved profiles",
			"active_profile_status text generated always as",
			"when status = 'active' then 'approved'")
		assertPhase15CompositeForeignKey(t,
			phase15TableDDL(t, up, "processor_governance_heads"),
			"processor_governance_profiles",
			map[string]string{
				"active_profile_id":          "id",
				"processor":                  "processor",
				"endpoint_id":                "endpoint_id",
				"active_governance_revision": "governance_revision",
				"active_profile_status":      "status",
			},
			"active governance heads must bind an approved profile identity, "+
				"processor, endpoint, and governance revision with one composite FK",
		)
	})

	t.Run("consent pins one subject and one governance revision", func(t *testing.T) {
		consents := mustPhase15TableBody(t, up, "processing_consents")
		assertPhase15Columns(t, consents, "processing_consents",
			"scope", "collection_id", "user_id", "processor",
			"governance_profile_id", "governance_revision",
			"governance_head_revision", "purposes", "data_types",
			"policy_version", "decision")
		assertPhase15Fragments(t, consents,
			"collection consent must set collection_id and clear user_id",
			"check", "scope", "'collection'", "collection_id is not null",
			"user_id is null")
		assertPhase15Fragments(t, consents,
			"query consent must set user_id and clear collection_id",
			"check", "scope", "'query'", "user_id is not null",
			"collection_id is null")
		assertPhase15ArrayColumn(t, consents, "purposes")
		assertPhase15ArrayColumn(t, consents, "data_types")
		assertPhase15Fragments(t, consents,
			"collection consent purposes must exclude query-only embedding",
			"processing_consents_scope_purposes_check",
			"scope = 'collection'",
			"purposes <@ array[ 'parse' , 'passage_embedding' , 'rerank' , 'answer' ]::text[]")
		assertPhase15Fragments(t, consents,
			"query consent purposes must exclude collection ingestion",
			"processing_consents_scope_purposes_check",
			"scope = 'query'",
			"purposes <@ array[ 'query_embedding' , 'rerank' , 'answer' ]::text[]")
		assertPhase15CompositeForeignKey(t,
			phase15TableDDL(t, up, "processing_consents"),
			"processor_governance_profiles",
			map[string]string{
				"governance_profile_id": "id",
				"processor":             "processor",
				"endpoint_id":           "endpoint_id",
				"governance_revision":   "governance_revision",
			},
			"processing consent must bind its processor, endpoint, profile ID, and governance revision with one composite FK",
		)
		assertPhase15CompositeForeignKey(t,
			phase15TableDDL(t, up, "processing_consents"),
			"processor_governance_heads",
			map[string]string{
				"processor":   "processor",
				"endpoint_id": "endpoint_id",
			},
			"processing consent must bind processor and endpoint to the same governance head with one composite FK",
		)
	})

	t.Run("invitation and recovery persist token hashes only", func(t *testing.T) {
		for _, table := range []string{"team_invites", "credential_recovery_tokens"} {
			body := mustPhase15TableBody(t, up, table)
			assertPhase15Columns(t, body, table,
				"token_hash", "status", "expires_at", "revoked_at")
			if table == "team_invites" {
				assertPhase15Columns(t, body, table, "accepted_at")
				assertPhase15Fragments(t, body,
					"team invites must tie accepted/revoked states to exclusive timestamps",
					"status = 'pending'", "status = 'accepted'", "accepted_at is not null",
					"status = 'revoked'", "revoked_at is not null")
			} else {
				assertPhase15Columns(t, body, table, "used_at")
				assertPhase15Fragments(t, body,
					"recovery tokens must tie used/revoked states to exclusive timestamps",
					"status = 'active'", "status = 'used'", "used_at is not null",
					"status = 'revoked'", "revoked_at is not null")
			}
			if regexp.MustCompile(`\b(?:raw_)?token\b|\btoken_(?:plaintext|ciphertext|value)\b`).MatchString(body) {
				t.Errorf("%s must persist only hash-semantic token fields, never a raw or recoverable token", table)
			}
		}

		credentials := mustPhase15TableBody(t, up, "user_credentials")
		if !regexp.MustCompile(`\bpassword_hash\b`).MatchString(credentials) {
			t.Error("Phase 15 user credentials must persist password_hash rather than a raw password")
		}
	})

	t.Run("outbox exposes a monotonic watermark and independent event identity", func(t *testing.T) {
		outbox := mustPhase15TableBody(t, up, "knowledge_outbox")
		assertPhase15Columns(t, outbox, "knowledge_outbox",
			"id", "event_id", "status", "payload")
		assertPhase15Fragments(t, outbox,
			"knowledge_outbox.id must be a database allocation cursor for contiguous-prefix replay",
			"id bigserial primary key")
		assertPhase15Fragments(t, outbox,
			"knowledge_outbox.event_id must remain an independent idempotency identity",
			"event_id uuid not null unique")
		assertPhase15Fragments(t, outbox,
			"knowledge_outbox.payload must be a JSON object",
			"payload jsonb", "jsonb_typeof ( payload ) = 'object'")
		assertPhase15Fragments(t, outbox,
			"outbox states must fence claim and publication timestamps",
			"status = 'pending'", "locked_at is null", "status = 'processing'",
			"locked_at is not null", "status = 'published'",
			"published_at is not null", "status = 'failed'")

		pendingIndex := regexp.MustCompile(
			`(?s)\bcreate\s+(?:unique\s+)?index\b[^;]*\bon\s+` +
				`(?:public\.)?knowledge_outbox\b[^;]*\bwhere\b[^;]*` +
				`\bstatus\s*=\s*'pending'[^;]*;`,
		)
		if !pendingIndex.MatchString(up) {
			t.Error("knowledge_outbox must have a partial index for status = 'pending'")
		}
		assertPhase15Fragments(t, pendingIndex.FindString(up),
			"pending outbox scans must have the high-watermark ID available in their index",
			"available_at", "id")
	})

	t.Run("down removes every Phase 15 object in dependency order", func(t *testing.T) {
		createdTables := phase15CreatedTables(up)
		if len(createdTables) == 0 {
			t.Fatal("Phase 15 up migration contains no CREATE TABLE statements")
		}
		for _, table := range createdTables {
			if !phase15DropsTable(down, table) {
				t.Errorf("Phase 15 down migration does not drop up-created table %q", table)
			}
		}

		createdIndexes := phase15CreatedIndexes(up)
		if len(createdIndexes) == 0 {
			t.Fatal("Phase 15 up migration contains no CREATE INDEX statements")
		}
		for _, index := range createdIndexes {
			if !phase15DropsIndex(down, index) {
				t.Errorf("Phase 15 down migration does not drop up-created index %q", index)
			}
		}

		addedColumns := phase15UserAlterColumns(up, "add")
		if len(addedColumns) == 0 {
			t.Fatal("Phase 15 up migration does not add the required users identity columns")
		}
		droppedColumns := phase15UserAlterColumns(down, "drop")
		for _, column := range addedColumns {
			if !containsPhase15String(droppedColumns, column) {
				t.Errorf("Phase 15 down migration does not drop users.%s added by the up migration", column)
			}
		}

		for _, order := range []struct {
			before string
			after  string
			reason string
		}{
			{
				"drop table if exists processing_consents",
				"drop table if exists processor_governance_heads",
				"consents reference governance heads",
			},
			{
				"drop table if exists processor_governance_heads",
				"drop table if exists processor_governance_profiles",
				"governance heads reference profiles",
			},
			{
				"drop constraint if exists knowledge_documents_active_version_status_fk",
				"drop table if exists knowledge_document_versions",
				"the active-version status cycle must be broken explicitly",
			},
			{
				"drop constraint if exists knowledge_documents_current_version_same_document_fk",
				"drop table if exists knowledge_document_versions",
				"the deferred document/version cycle must be broken explicitly",
			},
			{
				"drop table if exists knowledge_document_versions",
				"drop table if exists knowledge_documents",
				"document versions reference logical documents",
			},
			{
				"drop table if exists knowledge_documents",
				"drop table if exists knowledge_collections",
				"logical documents reference collections",
			},
			{
				"drop table if exists knowledge_collections",
				"drop table if exists teams",
				"team collections reference teams",
			},
			{
				"drop table if exists team_invites",
				"drop table if exists teams",
				"team invitations reference teams",
			},
			{
				"drop table if exists team_memberships",
				"drop table if exists teams",
				"team memberships reference teams",
			},
			{
				"drop constraint if exists users_account_status_check",
				"drop column if exists account_status",
				"the account status check depends on the account_status column",
			},
		} {
			assertPhase15Order(t, down, order.before, order.after, order.reason)
		}
	})
}
