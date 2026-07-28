package migration

import (
	"strings"
	"testing"
)

func TestMemoryPortabilityRetentionMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "061_memory_portability_retention.up.sql")
	down := readPhase15SQL(t, "061_memory_portability_retention.down.sql")

	for _, table := range []string{
		"memory_import_batches",
		"memory_deletion_replay_entries",
	} {
		if _, ok := phase15TableBody(up, table); !ok {
			t.Errorf("061 is missing %s", table)
		}
	}
	importDDL := phase15TableDDL(t, up, "memory_import_batches")
	assertPhase15Fragments(t, importDDL,
		"import batch authority must be user-bound and hash/count only",
		"user_id uuid not null references users",
		"package_hash text not null",
		"manifest_hash text not null",
		"mappings_hash text not null",
		"plan_hash text not null",
		"authority_state_hash text not null",
		"status in ( 'applying' , 'completed' )",
		"memory_count between 0 and 50000",
		"revision_count between 0 and 200000")
	deletionDDL := phase15TableDDL(t, up, "memory_deletion_replay_entries")
	assertPhase15Fragments(t, deletionDDL,
		"deletion replay authority must contain only opaque identity, hashes, and results",
		"manifest_id uuid primary key",
		"event_id uuid not null unique",
		"memory_id uuid not null unique",
		"tombstone_id uuid not null unique",
		"content_hash text not null",
		"entry_hash text not null",
		"result_code in ( 'replayed' , 'already_applied' , 'not_found' , 'hash_mismatch' )")
	for _, forbidden := range []string{
		"content text", "normalized_content", "prior_content", "query", "prompt",
		"embedding", "passphrase", "ciphertext", "plaintext",
	} {
		if strings.Contains(importDDL, forbidden) || strings.Contains(deletionDDL, forbidden) {
			t.Errorf("061 portability authority persists forbidden payload %q", forbidden)
		}
	}

	assertPhase15Fragments(t, up,
		"061 must extend only the explicit imported source and revision actor contracts",
		"source in ( 'manual' , 'ai' , 'direct_user' , 'import' )",
		"actor_type in ( 'user' , 'worker' , 'operator' , 'import' )",
		"prior_source in ( 'manual' , 'ai' , 'direct_user' , 'import' )")
	assertPhase15Fragments(t, up,
		"portability must expose deterministic snapshot, mapping, state, and ADD-only capabilities",
		"create function memory_portability_authority_state",
		"create function memory_portability_export_records",
		"create function memory_portability_resolve_project",
		"create function memory_portability_resolve_conversation",
		"create function memory_portability_resolve_memory",
		"create function memory_portability_completed_import",
		"create function memory_portability_begin_import",
		"create function memory_portability_create_project",
		"create function memory_portability_add_memory",
		"create function memory_portability_add_revision",
		"p_prior_superseded_by_memory_id uuid",
		"create function memory_portability_finalize_memory",
		"create function memory_portability_complete_import",
		"memory_import_state_stale",
		"memory_import_replay_conflict",
		"'import'",
		"memory_governance_is_secret")
	assertPhase15Fragments(t, up,
		"restore replay must be ID/hash fenced, plaintext-wiping, provider-free, and rebuild projections",
		"create function memory_portability_export_deletions",
		"create function memory_portability_replay_deletion",
		"create function memory_portability_rebuild_projections",
		"v_memory.content_hash <> v_content_hash",
		"memory_deletion_replay_conflict",
		"delete from user_memory_evidence",
		"prior_content_snapshot = null",
		"content = ''",
		"normalized_content = ''",
		"delete from user_memory_search_projections",
		"memory_refresh_lexical_projection")

	assertPhase15Fragments(t, up,
		"every portability capability must be pinned SECURITY DEFINER with a trusted search path",
		"security definer",
		"set search_path from current",
		"alter function %i.%s set search_path to %i , pg_catalog , pg_temp",
		"alter function %i.%s owner to memory_runtime_owner")
	assertPhase15Fragments(t, up,
		"runtime roles must have no direct portability table CRUD and only the reviewed API capability set",
		"revoke all on memory_import_batches , memory_deletion_replay_entries from public , go_api_runtime , memory_worker_runtime",
		"revoke all on function memory_portability_authority_state",
		"grant execute on function memory_portability_authority_state",
		"memory_portability_replay_deletion",
		"memory_portability_rebuild_projections",
		"to go_api_runtime")
	for _, forbidden := range []string{
		"grant select on memory_import_batches",
		"grant insert on memory_import_batches",
		"grant update on memory_import_batches",
		"grant delete on memory_import_batches",
		"grant select on memory_deletion_replay_entries",
		"grant insert on memory_deletion_replay_entries",
		"grant update on memory_deletion_replay_entries",
		"grant delete on memory_deletion_replay_entries",
		"to memory_worker_runtime",
	} {
		if strings.Contains(up, forbidden) {
			t.Errorf("061 grants forbidden portability authority: %s", forbidden)
		}
	}

	assertPhase15Fragments(t, down,
		"061 rollback must fail closed once import or deletion replay authority exists",
		"memory_portability_rollback_requires_no_import_history",
		"exists ( select 1 from memory_import_batches )",
		"exists ( select 1 from memory_deletion_replay_entries )",
		"exists ( select 1 from user_memories where source = 'import' )",
		"exists ( select 1 from user_memory_revisions where actor_type = 'import' )",
		"drop function memory_portability_rebuild_projections",
		"drop function memory_portability_replay_deletion",
		"drop table memory_import_batches",
		"drop table memory_deletion_replay_entries",
		"source in ( 'manual' , 'ai' , 'direct_user' )",
		"actor_type in ( 'user' , 'worker' , 'operator' )")
}
