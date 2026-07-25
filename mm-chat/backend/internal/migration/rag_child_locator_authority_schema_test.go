package migration

import (
	"strings"
	"testing"
)

func TestRAGChildLocatorAuthoritySchemaContract(t *testing.T) {
	up := readPhase15SQL(t, "045_rag_child_locator_authority.up.sql")
	down := readPhase15SQL(t, "045_rag_child_locator_authority.down.sql")

	assertPhase15Fragments(t, up,
		"045 must validate canonical locator summaries before publication or hydration",
		"create function knowledge_locator_summary_is_valid",
		"g7.4-locator-summary.v1",
		"locatoraggregatehashes",
		"text_offset",
		"line_range",
		"page_bbox",
		"slide_shape",
		"sheet_cell",
		"ooxml_part_xpath",
		"revoke all on function knowledge_locator_summary_is_valid",
	)
	assertPhase15Fragments(t, up,
		"045 verifier must validate Parent context and exact Child citation locators independently",
		"create or replace function knowledge_verify_structure_generation",
		"knowledge_locator_summary_is_valid ( parent.locator_summary )",
		"knowledge_locator_summary_is_valid ( search.locator_summary )",
		"search.source_span_hash=child.source_span_hash",
		"search.chunk_profile_hash=child.chunk_profile_hash",
		"search.content_hash=child.content_hash",
	)
	if strings.Contains(up, "search.locator_summary=parent.locator_summary") {
		t.Fatal("045 must not require the exact Child locator to equal its Parent locator")
	}

	assertPhase15Fragments(t, up,
		"045 hydration must reauthorize the exact ready Child Search projection",
		"search.locator_summary as child_locator_summary",
		"authorized.child_locator_summary",
		"search.child_chunk_id = child.id",
		"search.parent_chunk_id = parent.id",
		"search.materialization_id = child.materialization_id",
		"search.index_generation_id = child.index_generation_id",
		"search.collection_id = r.collection_id",
		"search.document_id = child.document_id",
		"search.document_version_id = child.document_version_id",
		"search.source_span_hash = child.source_span_hash",
		"search.chunk_profile_hash = child.chunk_profile_hash",
		"search.content_hash = child.content_hash",
		"search.status = 'ready'",
		"knowledge_locator_summary_is_valid ( search.locator_summary )",
	)
	assertPhase15Fragments(t, up,
		"045 must retain migration 044 role boundaries",
		"from go_api_runtime",
		"to rag_replay_operator",
		"to go_evidence_hydrator , go_api_runtime",
		"quote_ident ( current_schema ( ) ) || ' , pg_catalog , pg_temp'",
		"set search_path from current",
	)

	assertPhase15Fragments(t, down,
		"045 down must restore the migration 044 Parent-locator compatibility behavior",
		"search.locator_summary=parent.locator_summary",
		"parent.locator_summary",
		"from go_api_runtime",
		"to rag_replay_operator",
		"drop function knowledge_locator_summary_is_valid",
	)
	assertPhase15Order(t, down,
		"create or replace function knowledge_reauthorize_and_hydrate_evidence",
		"drop function knowledge_locator_summary_is_valid",
		"the restored functions must stop using the validator before it is dropped",
	)
}
