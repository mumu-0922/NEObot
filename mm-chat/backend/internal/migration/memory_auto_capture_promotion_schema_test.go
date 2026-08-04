package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

const (
	memoryAutoCapturePromotionAppliedChecksum         = "7e37d06e2b1cf601ae33e02ecc5fdc817de7e8d76f09bc3fb15ed604c08ec663"
	memoryAutoCaptureAuthorityAppliedChecksum         = "4192deb1e6d0381239c2e1aa25633964968782dc61b6a3bbff963884bd42783c"
	memoryAutoCaptureToolProfileAppliedChecksum       = "31b3bd79cb7a59539cd808f897fe5b8afae07f7ee2a2e903d33ccebd0cc530ea"
	memoryAutoCaptureCompatibleProfileAppliedChecksum = "e5174d69a647ddaa886e0b04030736113ea629e991a1d2bf70957283ff1a272b"
)

func TestMemoryAutoCaptureAppliedChecksums(t *testing.T) {
	migrations, err := Load(migrationfiles.FS)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	want := map[int64]string{
		66: memoryAutoCapturePromotionAppliedChecksum,
		67: memoryAutoCaptureAuthorityAppliedChecksum,
		68: memoryAutoCaptureToolProfileAppliedChecksum,
		69: memoryAutoCaptureCompatibleProfileAppliedChecksum,
	}
	for _, migration := range migrations {
		expected, ok := want[migration.Version]
		if !ok {
			continue
		}
		if migration.Checksum != expected {
			t.Fatalf("migration %03d checksum = %q, want live applied %q",
				migration.Version, migration.Checksum, expected)
		}
		delete(want, migration.Version)
	}
	if len(want) != 0 {
		t.Fatalf("live-applied migrations not found: %#v", want)
	}
}

func TestMemoryAutoCapturePromotionMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "066_memory_auto_capture_promotion.up.sql")
	down := readPhase15SQL(t, "066_memory_auto_capture_promotion.down.sql")

	assertPhase15Fragments(t, up,
		"066 must expose one lease-fenced worker capability",
		"create function memory_worker_promote_capture_candidates",
		"security definer",
		"memory_worker_hydrate_capture_v2",
		"job.status = 'processing'",
		"job.lease_owner = p_worker_id",
		"job.lease_token = p_lease_token",
		"outbox.status = 'processing'",
		"outbox.visibility_epoch = job.visibility_epoch")
	assertPhase15Fragments(t, up,
		"066 must start only from the existing safe-add route and current settings",
		"suggestion.status = 'shadow'",
		"suggestion.disposition = 'shadow'",
		"suggestion.decision_reason_code = 'shadow_add'",
		"settings.enabled",
		"settings.auto_record_enabled",
		"v_suggestion.proposed_action <> 'add'",
		"v_suggestion.sensitivity <> 'normal'",
		"auto_promotion_review_required")
	assertPhase15Fragments(t, up,
		"066 must bind primary evidence, tombstone, exact, fact, scope, and epoch fences",
		"evidence.source_message_id = v_job.source_message_id",
		"evidence.source_content_hash = v_job.source_hash",
		"memory_governance_scope_generation",
		"state.visibility_epoch = v_suggestion.visibility_epoch",
		"user_memory_tombstones",
		"auto_promotion_tombstoned",
		"memory.normalized_content = v_suggestion.normalized_content",
		"memory.fact_key = v_suggestion.fact_key",
		"auto_promotion_conflict",
		"auto_promotion_evidence_stale",
		"evidence.source_content_hash")
	assertPhase15Fragments(t, up,
		"066 must reuse governance acceptance and preserve typed automatic audit",
		"memory_governance_decide_review",
		"'accept_new'",
		"authority_kind = 'auto'",
		"extraction_profile_id = v_suggestion.extraction_profile_id",
		"decision_kind = 'auto_accept'",
		"result_code = 'auto_captured'",
		"message_memory_activities")
	assertPhase15Fragments(t, up,
		"066 must keep the runtime role function-only",
		"owner to memory_runtime_owner",
		"revoke all on function memory_worker_promote_capture_candidates",
		"from public , go_api_runtime",
		"grant execute on function memory_worker_promote_capture_candidates",
		"to memory_worker_runtime")

	for _, forbidden := range []string{
		"GRANT SELECT ON user_memories TO memory_worker_runtime",
		"GRANT INSERT ON user_memories TO memory_worker_runtime",
		"GRANT UPDATE ON user_memories TO memory_worker_runtime",
		"GRANT DELETE ON user_memories TO memory_worker_runtime",
	} {
		if strings.Contains(up, forbidden) {
			t.Fatalf("066 grants forbidden table authority: %q", forbidden)
		}
	}

	assertPhase15Fragments(t, down,
		"066 rollback must preserve canonical auto-capture history",
		"memory_auto_capture_rollback_requires_no_promotions",
		"decision_kind = 'auto_accept'",
		"result_code = 'auto_captured'",
		"decision_kind in ( 'keep_current' , 'reject' )",
		"revoke all on function memory_worker_promote_capture_candidates",
		"drop function memory_worker_promote_capture_candidates")
}

func TestMemoryAutoCaptureAuthorityHardeningMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "067_memory_auto_capture_authority_hardening.up.sql")
	down := readPhase15SQL(t, "067_memory_auto_capture_authority_hardening.down.sql")

	assertPhase15Fragments(t, up,
		"067 must replace only the promotion authority",
		"create or replace function memory_worker_promote_capture_candidates",
		"security definer",
		"memory_worker_hydrate_capture_v2",
		"owner to memory_runtime_owner")
	assertPhase15Fragments(t, up,
		"067 must bind promotion to the required Tool profiles and complete batch",
		"memory-capture-candidate-tool-v3",
		"memory-capture-decision-tool-v2",
		"v_batch.proposal_count",
		"suggestion.extraction_profile_id <> v_batch.extraction_profile_id",
		"message = 'memory_profile_drift'")
	assertPhase15Fragments(t, up,
		"067 must bind candidate bytes and every evidence row",
		"auto_promotion_candidate_drift",
		"auto_promotion_evidence_stale",
		"left join messages evidence_message",
		"evidence_message.status <> 'completed'",
		"evidence_message.deleted_at is not null",
		"evidence_message.completed_at , evidence_message.created_at",
		"evidence.source_content_hash")
	assertPhase15Fragments(t, up,
		"067 must retain function-only runtime authority",
		"revoke all on function memory_worker_promote_capture_candidates",
		"from public , go_api_runtime",
		"grant execute on function memory_worker_promote_capture_candidates",
		"to memory_worker_runtime")

	for _, forbidden := range []string{
		"GRANT SELECT ON user_memories TO memory_worker_runtime",
		"GRANT INSERT ON user_memories TO memory_worker_runtime",
		"GRANT UPDATE ON user_memories TO memory_worker_runtime",
		"GRANT DELETE ON user_memories TO memory_worker_runtime",
	} {
		if strings.Contains(up, forbidden) {
			t.Fatalf("067 grants forbidden table authority: %q", forbidden)
		}
	}

	assertPhase15Fragments(t, down,
		"067 rollback must be guarded and restore the 066 function",
		"memory_auto_capture_authority_rollback_requires_no_promotions",
		"decision_kind = 'auto_accept'",
		"result_code = 'auto_captured'",
		"create or replace function memory_worker_promote_capture_candidates",
		"owner to memory_runtime_owner",
		"to memory_worker_runtime")
	if strings.Contains(strings.ToLower(down), "drop function memory_worker_promote_capture_candidates") {
		t.Fatal("067 rollback must restore, not drop, the 066 promotion function")
	}
}

func TestMemoryAutoCaptureToolEvidenceProfileMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "068_memory_auto_capture_tool_evidence_profile.up.sql")
	down := readPhase15SQL(t, "068_memory_auto_capture_tool_evidence_profile.down.sql")

	assertPhase15Fragments(t, up,
		"068 must bind promotion to the evidence-enumerated extraction profile",
		"create or replace function memory_worker_promote_capture_candidates",
		"memory-capture-candidate-tool-v4",
		"memory-capture-decision-tool-v2",
		"v_batch.proposal_count",
		"auto_promotion_candidate_drift",
		"auto_promotion_evidence_stale",
		"owner to memory_runtime_owner",
		"to memory_worker_runtime")
	if strings.Contains(up, "memory-capture-candidate-tool-v3") {
		t.Fatal("068 up must not authorize the superseded extraction profile v3")
	}

	assertPhase15Fragments(t, down,
		"068 rollback must be guarded and restore the 067 profile",
		"memory_auto_capture_tool_profile_rollback_requires_no_promotions",
		"decision_kind = 'auto_accept'",
		"result_code = 'auto_captured'",
		"create or replace function memory_worker_promote_capture_candidates",
		"memory-capture-candidate-tool-v3",
		"memory-capture-decision-tool-v2",
		"owner to memory_runtime_owner",
		"to memory_worker_runtime")
	if strings.Contains(strings.ToLower(down), "drop function memory_worker_promote_capture_candidates") {
		t.Fatal("068 rollback must restore, not drop, the 067 promotion function")
	}
}

func TestMemoryAutoCaptureCompatibleToolProfileMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "069_memory_auto_capture_compatible_tool_profile.up.sql")
	down := readPhase15SQL(t, "069_memory_auto_capture_compatible_tool_profile.down.sql")

	assertPhase15Fragments(t, up,
		"069 must bind promotion to the OpenAI-compatible evidence profile",
		"create or replace function memory_worker_promote_capture_candidates",
		"memory-capture-candidate-tool-v5",
		"memory-capture-decision-tool-v2",
		"v_batch.proposal_count",
		"auto_promotion_evidence_stale",
		"owner to memory_runtime_owner",
		"to memory_worker_runtime")
	if strings.Contains(up, "memory-capture-candidate-tool-v4") {
		t.Fatal("069 up must not authorize the Provider-rejected extraction profile v4")
	}

	assertPhase15Fragments(t, down,
		"069 rollback must be guarded and restore the 068 profile",
		"memory_auto_capture_compatible_profile_rollback_requires_no_promotions",
		"decision_kind = 'auto_accept'",
		"result_code = 'auto_captured'",
		"create or replace function memory_worker_promote_capture_candidates",
		"memory-capture-candidate-tool-v4",
		"owner to memory_runtime_owner",
		"to memory_worker_runtime")
	if strings.Contains(strings.ToLower(down), "drop function memory_worker_promote_capture_candidates") {
		t.Fatal("069 rollback must restore, not drop, the 068 promotion function")
	}
}
