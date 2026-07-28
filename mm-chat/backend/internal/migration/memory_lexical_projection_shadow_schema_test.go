package migration

import (
	"strings"
	"testing"
)

func TestMemoryLexicalProjectionShadowMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "058_memory_lexical_projection_shadow.up.sql")
	down := readPhase15SQL(t, "058_memory_lexical_projection_shadow.down.sql")

	for _, table := range []string{
		"user_memory_search_projections",
		"message_memory_lexical_shadow_observations",
		"message_memory_lexical_shadow_results",
	} {
		if _, ok := phase15TableBody(up, table); !ok {
			t.Errorf("058 is missing %s", table)
		}
	}

	for _, table := range []string{
		"message_memory_lexical_shadow_observations",
		"message_memory_lexical_shadow_results",
	} {
		body := phase15TableDDL(t, up, table)
		for _, forbidden := range []string{
			"query_text", "memory_content", "prompt", "embedding", "raw_score",
		} {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s contains forbidden durable payload %q", table, forbidden)
			}
		}
	}

	assertPhase15Fragments(t, up,
		"058 must reject an unsupported PostgreSQL or pg_textsearch runtime",
		"memory_lexical_requires_postgresql_17",
		"memory_lexical_requires_pg_textsearch_preload",
		"memory_lexical_requires_pg_textsearch_1_3_1",
		"knowledge_bm25_shadow_query_terms",
		"knowledge_build_bm25_shadow_text",
		"knowledge_normalize_bm25_shadow_terms")
	assertPhase15Fragments(t, up,
		"058 projection must bind current canonical and scope authority",
		"foreign key ( memory_id , user_id )",
		"references projects ( id , user_id ) on delete cascade",
		"references conversations ( id , user_id ) on delete cascade",
		"memory.revision = projection.memory_revision",
		"memory.content_hash = projection.content_hash",
		"state.visibility_epoch = projection.visibility_epoch",
		"state.active_projection_generation = projection.projection_generation",
		"scoped_project.scope_generation = projection.scope_generation")
	assertPhase15Fragments(t, up,
		"058 must maintain projection on every relevant authority mutation",
		"after insert on user_memories",
		"after update of content , normalized_content , tags , revision",
		"after update of visibility_epoch , active_projection_generation",
		"after update of lifecycle_status , scope_generation",
		"after update of deleted_at , memory_scope_generation")
	assertPhase15Fragments(t, up,
		"058 exact and BM25 lanes must remain separate and deterministic",
		"using gin ( exact_terms )",
		"using bm25 ( bm25_text ) with ( text_config = 'simple' )",
		"lane in ( 'v1' , 'exact' , 'bm25' , 'lexical' )",
		"memory_lexical_shadow_replay_conflict",
		"limit 30")
	assertPhase15Fragments(t, up,
		"058 must expose one narrow compare capability without direct table authority",
		"revoke all on user_memory_search_projections",
		"from public , go_api_runtime , memory_worker_runtime",
		"grant execute on function memory_compare_lexical_shadow",
		"to go_api_runtime")
	assertPhase15Fragments(t, down,
		"058 rollback must preserve reader and observation authority",
		"memory_lexical_rollback_requires_v1_reader",
		"memory_lexical_rollback_requires_empty_observations")
}
