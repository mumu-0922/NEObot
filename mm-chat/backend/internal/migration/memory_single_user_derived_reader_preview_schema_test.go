package migration

import (
	"strings"
	"testing"
)

func TestMemorySingleUserDerivedReaderPreviewMigrationContract(t *testing.T) {
	up := normalizePhase15SQL(readPhase15SQL(
		t, "073_memory_single_user_derived_reader_preview.up.sql",
	))
	down := normalizePhase15SQL(readPhase15SQL(
		t, "073_memory_single_user_derived_reader_preview.down.sql",
	))

	assertPhase15Fragments(t, up,
		"073 must add only exact sole-user preview authority",
		"memory_single_user_derived_reader_preview_requires_072",
		"create table memory_single_user_derived_reader_events",
		"memory_single_user_derived_reader_events_append_only",
		"memory_single_user_derived_reader_preview_enabled",
		"select count ( * ) = 1 from users",
		"pg_advisory_xact_lock",
		"i_accept_single_user_l2_l3_reader_preview_without_formal_promotion",
		"memory_single_user_derived_reader_preview_artifact_not_ready",
		"memory_single_user_derived_reader_preview_requires_sole_user",
		"user_population_changed",
		"active_retrieval_profile_id = 'memory_hybrid_bge_m3_rrf60_v1'",
		"memory_l2_scene_reconcile_user",
		"memory_l3_persona_reconcile_user",
		"memory_l2_scene_reader_authority",
		"memory_l3_persona_reader_authority",
		"memory_governance_l2_scene_snapshot",
		"memory_governance_l3_persona_snapshot",
		"grant select on table users to memory_runtime_owner",
		"from public , go_api_runtime , memory_worker_runtime")

	if strings.Contains(up,
		"grant execute on function memory_operator_set_single_user_derived_reader_preview") {
		t.Fatal("073 exposed preview activation to a runtime role")
	}
	if strings.Contains(up, "update memory_l2_scene_profiles") ||
		strings.Contains(up, "update memory_l3_persona_profiles") ||
		strings.Contains(up, "set active_retrieval_profile_id") ||
		strings.Contains(up, "insert into memory_l2_scene_promotion_events") ||
		strings.Contains(up, "insert into memory_l3_persona_promotion_events") {
		t.Fatal("073 forged formal L1/L2/L3 promotion state")
	}

	assertPhase15Fragments(t, down,
		"073 down must restore the formal-promotion-only functions",
		"memory_single_user_derived_reader_preview_rollback_requires_empty_events",
		"create or replace function memory_l2_scene_reconcile_user",
		"create or replace function memory_l2_scene_reader_authority",
		"create or replace function memory_governance_l2_scene_snapshot",
		"create or replace function memory_l3_persona_reconcile_user",
		"create or replace function memory_l3_persona_reader_authority",
		"create or replace function memory_governance_l3_persona_snapshot",
		"drop table memory_single_user_derived_reader_events",
		"revoke select on table users from memory_runtime_owner")
	if strings.Contains(down,
		"memory_single_user_derived_reader_preview_enabled ( p_user_id") {
		t.Fatal("073 down retained preview authority in restored reader functions")
	}
}
