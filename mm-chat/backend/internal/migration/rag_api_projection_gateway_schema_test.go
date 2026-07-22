package migration

import (
	"strings"
	"testing"
)

func TestRAGAPIProjectionGatewayMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "039_rag_api_projection_gateway.up.sql")
	down := readPhase15SQL(t, "039_rag_api_projection_gateway.down.sql")

	t.Run("installs three narrow hardened gateways", func(t *testing.T) {
		assertPhase15Fragments(t, up,
			"039 must install the API projection gateways with a trusted path",
			"quote_ident ( current_schema ( ) ) || ' , pg_catalog , pg_temp'",
			"create function knowledge_allocate_parse_materialization",
			"create function knowledge_is_document_version_actively_projected",
			"create function knowledge_resolve_purge_projection_binding",
			"security definer",
			"set search_path from current",
			"owner to rag_projection_owner",
			"from public",
			"to go_api_runtime",
		)
	})

	t.Run("derives write metadata from authority", func(t *testing.T) {
		assertPhase15Fragments(t, up,
			"039 must not trust API-supplied projection metadata",
			"from knowledge_documents document",
			"join knowledge_document_versions version",
			"join knowledge_collections collection",
			"join files source_file",
			"for update of head",
			"insert into knowledge_document_materializations",
			"authoritative_source_content_hash",
			"active_base_profile_hash",
		)
		for _, forbidden := range []string{
			"p_collection_id",
			"p_file_id",
			"p_source_content_hash",
			"p_base_profile_hash",
			"p_collection_acl_revision",
			"p_document_visibility_epoch",
		} {
			if strings.Contains(up, forbidden) {
				t.Fatalf("039 must derive authority metadata; found parameter %q", forbidden)
			}
		}
	})

	t.Run("does not broaden direct projection privileges", func(t *testing.T) {
		for _, relation := range []string{
			"knowledge_corpus_projection_head",
			"knowledge_index_generations",
			"knowledge_index_profiles",
			"knowledge_document_materializations",
			"knowledge_document_projection_heads",
		} {
			if strings.Contains(up, "on "+relation+" to go_api_runtime") {
				t.Fatalf("039 must not grant direct access on %s", relation)
			}
		}
	})

	t.Run("rollback removes only the gateways", func(t *testing.T) {
		assertPhase15Fragments(t, down,
			"039 down must remove all three API gateways",
			"drop function knowledge_resolve_purge_projection_binding",
			"drop function knowledge_is_document_version_actively_projected",
			"drop function knowledge_allocate_parse_materialization",
		)
		for _, forbidden := range []string{
			"drop table",
			"drop extension",
			"drop function knowledge_fetch_profiled_query_evidence_candidates",
		} {
			if strings.Contains(down, forbidden) {
				t.Fatalf("039 down crossed its rollback boundary: found %q", forbidden)
			}
		}
	})
}
