package migration

import (
	"strings"
	"testing"
)

func TestStructureGenerationAbandonmentIsAuditedAndLeastPrivilege(t *testing.T) {
	up := readPhase15SQL(t, "046_structure_generation_abandonment_audit.up.sql")
	down := readPhase15SQL(t, "046_structure_generation_abandonment_audit.down.sql")

	assertPhase15Fragments(t, up,
		"046 must bind abandonment to an immutable operator audit",
		"create table knowledge_structure_generation_abandonment_audits",
		"candidate_generation_id uuid primary key",
		"failure_code = 'operator_abandoned'",
		"operator_id uuid not null",
		"octet_length ( reason ) between 1 and 1024",
		"knowledge_structure_generation_abandonment_audits_immutable",
	)
	assertPhase15Fragments(t, up,
		"046 must reuse the exact manifest/head CAS and reject conflicting replay",
		"create function knowledge_abandon_structure_generation_candidate",
		"knowledge_fail_structure_generation",
		"p_expected_head_revision",
		"p_expected_manifest_hash",
		"rag_structure_abandon_replay_mismatch",
		"on conflict ( candidate_generation_id ) do nothing",
	)
	assertPhase15Fragments(t, up,
		"046 must replace raw failure access with the audited operator gateway",
		"knowledge_fail_structure_generation ( uuid , bigint , text , text ) from rag_replay_operator",
		"knowledge_abandon_structure_generation_candidate ( uuid , bigint , text , uuid , text ) from go_api_runtime",
		"knowledge_abandon_structure_generation_candidate ( uuid , bigint , text , uuid , text ) to rag_replay_operator",
	)
	if strings.Contains(up,
		"knowledge_fail_structure_generation ( uuid , bigint , text , text ) to rag_replay_operator") {
		t.Fatal("046 must not retain direct operator access to unaudited failure")
	}
	assertPhase15Order(t, down,
		"from rag_replay_operator",
		"drop function knowledge_abandon_structure_generation_candidate",
		"046 rollback must revoke the gateway before dropping it",
	)
	assertPhase15Fragments(t, down,
		"046 rollback must restore the migration 045 operator boundary",
		"drop table knowledge_structure_generation_abandonment_audits",
		"knowledge_fail_structure_generation ( uuid , bigint , text , text ) to rag_replay_operator",
	)
}
