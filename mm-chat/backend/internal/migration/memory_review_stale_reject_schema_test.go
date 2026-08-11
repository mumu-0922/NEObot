package migration

import (
	"strings"
	"testing"
)

func TestMemoryReviewStaleRejectMigrationContract(t *testing.T) {
	up := normalizePhase15SQL(readPhase15SQL(
		t, "072_memory_review_stale_reject.up.sql",
	))
	down := normalizePhase15SQL(readPhase15SQL(
		t, "072_memory_review_stale_reject.down.sql",
	))

	assertPhase15Fragments(t, up,
		"072 must bypass only target revision validation for reject",
		"memory_review_stale_reject_requires_071",
		"create or replace function memory_governance_decide_review",
		"if p_decision_kind <> 'reject' then",
		"v_epoch is distinct from v_suggestion.visibility_epoch",
		"v_scope_generation <> v_suggestion.scope_generation",
		"v_target_memory.revision <> v_target.expected_revision",
		"if p_decision_kind in ( 'keep_current' , 'reject' ) then",
		"candidate_content = null",
		"decision_kind = p_decision_kind",
		"result_code = v_result_code",
		"to go_api_runtime")

	guard := strings.Index(up, "if p_decision_kind <> 'reject' then")
	epochFence := strings.Index(up,
		"v_epoch is distinct from v_suggestion.visibility_epoch")
	scopeFence := strings.Index(up,
		"v_scope_generation <> v_suggestion.scope_generation")
	targetFence := strings.Index(up,
		"v_target_memory.revision <> v_target.expected_revision")
	decision := strings.Index(up,
		"if p_decision_kind in ( 'keep_current' , 'reject' ) then")
	if guard < 0 || epochFence <= guard || scopeFence <= epochFence ||
		targetFence <= scopeFence || decision <= targetFence {
		t.Fatalf(
			"072 stale-target ordering is unsafe: guard=%d epoch=%d scope=%d target=%d decision=%d",
			guard, epochFence, scopeFence, targetFence, decision,
		)
	}
	if strings.Contains(up, "p_decision_kind <> 'keep_current'") ||
		strings.Contains(up, "p_decision_kind not in ( 'reject' )") {
		t.Fatal("072 weakened a target-consuming Review decision")
	}

	assertPhase15Fragments(t, down,
		"072 down must restore the exact all-decision target fence",
		"memory_review_stale_reject_rollback_requires_060",
		"create or replace function memory_governance_decide_review",
		"v_target_memory.revision <> v_target.expected_revision",
		"if p_decision_kind in ( 'keep_current' , 'reject' ) then",
		"to go_api_runtime")
	if strings.Contains(down, "if p_decision_kind <> 'reject' then") {
		t.Fatal("072 down retained the stale-target reject bypass")
	}
	downTargetFence := strings.Index(down,
		"v_target_memory.revision <> v_target.expected_revision")
	downDecision := strings.Index(down,
		"if p_decision_kind in ( 'keep_current' , 'reject' ) then")
	if downTargetFence < 0 || downDecision <= downTargetFence {
		t.Fatalf("072 down target fence ordering = %d/%d",
			downTargetFence, downDecision)
	}
}
