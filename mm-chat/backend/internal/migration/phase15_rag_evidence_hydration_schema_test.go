package migration

import "testing"

func TestPhase15RAGEvidenceHydrationReauthorizationContract(t *testing.T) {
	up := readPhase15SQL(t, "023_rag_evidence_hydration_reauthorization.up.sql")
	down := readPhase15SQL(t, "023_rag_evidence_hydration_reauthorization.down.sql")

	assertPhase15Fragments(t, up,
		"023 must replace the Go-only hydration function with a content-hash-bound result",
		"drop function if exists knowledge_reauthorize_and_hydrate_evidence",
		"create function knowledge_reauthorize_and_hydrate_evidence",
		"content_hash text",
		"source_text text",
		"locator jsonb")
	assertPhase15Fragments(t, up,
		"023 must require caller session and conversation authorization before hydration",
		"s.id = p_session_id",
		"s.user_id = p_actor_user_id",
		"s.revoked_at is null",
		"s.expires_at > clock_timestamp",
		"c.user_id = p_actor_user_id",
		"rag_hydration_not_authorized")
	assertPhase15Fragments(t, up,
		"023 must reauthorize current collection/document/version/projection fences",
		"corpus.active_index_generation_id = r.index_generation_id",
		"head.active_materialization_id = r.materialization_id",
		"m.status = 'published'",
		"document.current_version_id = r.document_version_id",
		"document.status = 'active'",
		"version.status = 'active'",
		"version.visibility_epoch = m.document_visibility_epoch",
		"version.content_hash = m.source_content_hash",
		"collection.acl_revision = m.collection_acl_revision",
		"collection.visibility_epoch = m.collection_visibility_epoch",
		"document.visibility_epoch = m.document_visibility_epoch")
	assertPhase15Fragments(t, up,
		"023 must bind hydrated text to the exact Python reference hashes",
		"source_span_hash text , content_hash text",
		"child.source_span_hash = r.source_span_hash",
		"child.content_hash = r.content_hash",
		"authorized.content_hash",
		"octet_length ( authorized.content ) <= 65536")
	assertPhase15Fragments(t, up,
		"023 must remain Go-only executable",
		"owner to rag_projection_owner",
		"revoke all on function knowledge_reauthorize_and_hydrate_evidence",
		"to go_evidence_hydrator , go_api_runtime")

	assertPhase15Fragments(t, down,
		"023 rollback must restore the previous source-span-only hydration contract",
		"drop function if exists knowledge_reauthorize_and_hydrate_evidence",
		"create function knowledge_reauthorize_and_hydrate_evidence",
		"source_span_hash text , source_text text , locator jsonb",
		"grant execute on function knowledge_reauthorize_and_hydrate_evidence")
	if containsPhase15String([]string{down}, "child.content_hash = r.content_hash") {
		t.Fatal("023 rollback must remove the new content-hash hydration fence")
	}
}
