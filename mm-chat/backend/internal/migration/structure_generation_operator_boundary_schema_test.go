package migration

import (
	"strings"
	"testing"
)

func TestStructureGenerationOperatorBoundaryIsManualAndLeastPrivilege(t *testing.T) {
	up := readPhase15SQL(t, "044_structure_generation_operator_boundary.up.sql")
	down := readPhase15SQL(t, "044_structure_generation_operator_boundary.down.sql")

	assertPhase15Fragments(t, up,
		"044 must register the exact v2 tokenizer and semantic-bound profile",
		"create table knowledge_structure_chunk_profile_descriptors",
		"606d6ac1cca428a05a7dccce0b172aabfba893f02431834cdc75775342db88b1",
		"223921b76ee99bde995b7ff738513eef100fb51d18c93597a113bcffe865b2a7",
		"d48a1992b71a810f377931afd97b5b28588e412918a3f2d9e445b019f29dc6e4",
		"3c17b8c1ddbed7b0a241dc43bdb24d3615526e94700c0971e585aa25519b409d",
		"knowledge_structure_chunk_profile_descriptors_immutable",
	)
	assertPhase15Fragments(t, up,
		"044 must expose source-free status/allocation and report-bound activation",
		"create function knowledge_structure_generation_operator_status",
		"create function knowledge_list_structure_generation_rebuild_documents",
		"create function knowledge_begin_registered_structure_generation_rebuild",
		"rag_structure_operator_profile_unregistered",
		"create table knowledge_structure_generation_activation_audits",
		"gate_report_sha256 text not null",
		"create function knowledge_activate_structure_generation_candidate",
		"knowledge_promote_index_generation",
	)
	assertPhase15Fragments(t, up,
		"044 must remove generation mutation from the API and grant only operator gateways",
		"revoke execute on function knowledge_begin_structure_generation_rebuild",
		"revoke execute on function knowledge_verify_structure_generation",
		"revoke execute on function knowledge_promote_index_generation",
		"from go_api_runtime",
		"to rag_replay_operator",
	)
	if strings.Contains(up, "grant execute on function knowledge_promote_index_generation") {
		t.Fatal("044 must not grant the raw promotion function to the operator")
	}
	assertPhase15Fragments(t, down,
		"044 rollback must remove operator gateways before restoring API compatibility",
		"from rag_replay_operator",
		"drop function knowledge_activate_structure_generation_candidate",
		"drop table knowledge_structure_generation_activation_audits",
		"drop table knowledge_structure_chunk_profile_descriptors",
		"grant execute on function knowledge_begin_structure_generation_rebuild",
		"to go_api_runtime",
	)
	assertPhase15Order(t, down,
		"from rag_replay_operator",
		"drop function knowledge_activate_structure_generation_candidate",
		"044 rollback must revoke operator execution before dropping gateways",
	)
}
