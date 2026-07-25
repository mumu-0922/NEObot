package migration

import "testing"

func TestRAGParentEvidenceHydrationSchemaContract(t *testing.T) {
	up := readPhase15SQL(t, "043_rag_parent_evidence_hydration.up.sql")
	down := readPhase15SQL(t, "043_rag_parent_evidence_hydration.down.sql")

	assertPhase15Fragments(t, up,
		"043 must hydrate matched Child and containing Parent under one authority fence",
		"child.content as child_source_text",
		"child.token_count as child_token_count",
		"parent.content as parent_source_text",
		"parent.token_count as parent_token_count",
		"child.source_span_hash = r.source_span_hash",
		"child.content_hash = r.content_hash",
		"corpus.active_index_generation_id = r.index_generation_id",
		"head.active_materialization_id = r.materialization_id",
		"m.status = 'published'",
	)
	assertPhase15Fragments(t, up,
		"043 must keep the hydration body bounded and Go-only",
		"octet_length ( authorized.child_source_text ) <= 65536",
		"octet_length ( authorized.parent_source_text ) <= 65536",
		"owner to rag_projection_owner",
		"revoke all on function knowledge_reauthorize_and_hydrate_evidence",
		"to go_evidence_hydrator , go_api_runtime",
	)
	for _, sql := range []string{up, down} {
		assertPhase15Fragments(t, sql,
			"043 function definitions must capture a hardened schema path",
			"quote_ident ( current_schema ( ) ) || ' , pg_catalog , pg_temp'",
			"set search_path from current",
		)
	}
	assertPhase15Fragments(t, down,
		"043 rollback must restore the 023 Child-only result",
		"content_hash text , source_text text , locator jsonb",
		"child.content_hash = r.content_hash",
	)
}
