package migration

import (
	"strings"
	"testing"
)

func TestMemoryGovernanceUIMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "060_memory_governance_ui.up.sql")
	down := readPhase15SQL(t, "060_memory_governance_ui.down.sql")

	if _, ok := phase15TableBody(up, "user_memory_review_decisions"); !ok {
		t.Fatal("060 is missing user_memory_review_decisions")
	}
	decisionDDL := phase15TableDDL(t, up, "user_memory_review_decisions")
	assertPhase15Fragments(t, decisionDDL,
		"Review decisions must be user-owned, replay-pinned, and plaintext-free",
		"unique ( suggestion_id )",
		"foreign key ( suggestion_id , user_id )",
		"decision_hash text not null",
		"result_code text not null",
		"result_memory_revision bigint")
	for _, forbidden := range []string{"candidate_content", "edited_content", "prompt", "query", "embedding", "raw_score"} {
		if strings.Contains(decisionDDL, forbidden) {
			t.Errorf("Review decision audit contains forbidden plaintext field %q", forbidden)
		}
	}

	assertPhase15Fragments(t, up,
		"060 must expose the complete current-user governance capability surface",
		"create function memory_governance_snapshot",
		"create function memory_governance_upsert_global_legacy",
		"create function memory_governance_update_global_legacy",
		"create function memory_governance_create_project",
		"create function memory_governance_update_project",
		"create function memory_governance_update_conversation_policy",
		"create function memory_governance_create_memory",
		"create function memory_governance_update_memory",
		"create function memory_governance_delete_memory",
		"create function memory_governance_memory_detail",
		"create function memory_governance_decide_review",
		"create function memory_governance_list_message_activities")
	assertPhase15Fragments(t, up,
		"Project, policy, and scoped Memory writes must be bounded and generation/revision fenced",
		"memory_governance_project_limit",
		"memory_governance_revision_stale",
		"memory_governance_scope_stale",
		"memory_scope_generation = v_generation",
		"memory_governance_scope_generation",
		"then 'move' else 'update' end",
		"v_scope_changed and not v_payload_changed",
		"project.lifecycle_status = 'active'",
		"project.id = v_conversation.project_id",
		"memory_delete_direct_scoped")
	assertPhase15Fragments(t, up,
		"Go and SQL governance writes must share the complete secret rejection boundary",
		"create function memory_governance_is_secret",
		"create function memory_governance_classify_sensitivity",
		"memory_governance_is_secret ( p_content )",
		"memory_governance_is_secret ( v_content )",
		"memory_governance_classify_sensitivity ( p_content )",
		"memory_governance_classify_sensitivity ( v_content )",
		"memory_governance_classify_sensitivity ( content ) in ( 'sensitive' , 'secret' )",
		"session[ _-]? ( id|token )",
		"authorization\\s*:\\s*bearer",
		"cookies?",
		"cvv")
	assertPhase15Fragments(t, up,
		"Review decisions must recheck expiry, epoch, scope, targets, evidence, and Sensitive authority",
		"v_suggestion.review_expires_at <= v_now",
		"v_epoch is distinct from v_suggestion.visibility_epoch",
		"v_scope_generation <> v_suggestion.scope_generation",
		"v_target_memory.revision <> v_target.expected_revision",
		"settings.sensitive_memory_enabled",
		"source.role = 'user'",
		"candidate_content = null",
		"purged_at = v_now")
	assertPhase15Fragments(t, up,
		"Governance reads must expose only bounded diagnostics and source-deleted markers",
		"limit 500",
		"limit 100",
		"limit 20",
		"'sourcedeleted'",
		"source.status = 'completed'",
		"source_conversation.deleted_at is null",
		"activity_conversation.deleted_at is null",
		"current_target.is_current",
		"current_memory.is_current",
		"state.visibility_epoch = memory.visibility_epoch",
		"'retention_expired'",
		"'profile'",
		"'estimatedtokens'",
		"'sourcekind'",
		"'scopetype'",
		"memory_governance_epoch_millis ( activity.created_at )")
	for _, forbidden := range []string{"'querytext'", "'rawscore'", "'embedding'", "'providersecret'"} {
		if strings.Contains(up, forbidden) {
			t.Errorf("060 governance response exposes forbidden field %q", forbidden)
		}
	}

	assertPhase15Fragments(t, up,
		"go_api_runtime must receive governed functions without legacy bypass or direct table CRUD",
		"revoke all on user_memory_review_decisions",
		"from public , go_api_runtime , memory_worker_runtime",
		"revoke execute on function memory_upsert_global_manual",
		"memory_update_global_manual",
		"from go_api_runtime",
		"grant execute on function memory_governance_snapshot",
		"memory_governance_upsert_global_legacy",
		"memory_governance_update_global_legacy",
		"to go_api_runtime")
	for _, forbidden := range []string{
		"grant select on projects to go_api_runtime",
		"grant insert on projects to go_api_runtime",
		"grant select on user_memory_review_suggestions to go_api_runtime",
		"grant select on user_memory_evidence to go_api_runtime",
		"grant select on user_memory_revisions to go_api_runtime",
	} {
		if strings.Contains(up, forbidden) {
			t.Errorf("060 grants forbidden table authority: %s", forbidden)
		}
	}

	assertPhase15Fragments(t, down,
		"060 rollback must preserve Review decision authority",
		"memory_governance_rollback_requires_no_decisions",
		"memory_governance_rollback_requires_legacy_reviews",
		"memory_governance_rollback_requires_no_move_revisions",
		"grant execute on function memory_upsert_global_manual",
		"memory_update_global_manual",
		"to go_api_runtime",
		"drop function memory_governance_update_global_legacy",
		"drop function memory_governance_upsert_global_legacy")
}
