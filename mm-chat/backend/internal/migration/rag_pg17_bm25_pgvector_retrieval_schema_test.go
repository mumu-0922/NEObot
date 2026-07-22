package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestRAGPG17BM25PGVectorRetrievalMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "038_pg17_bm25_pgvector_retrieval.up.sql")
	down := readPhase15SQL(t, "038_pg17_bm25_pgvector_retrieval.down.sql")

	t.Run("fails closed unless the reviewed PG17 extensions are available", func(t *testing.T) {
		assertPhase15Fragments(t, up,
			"038 must gate the PostgreSQL major, preload, and exact extension versions",
			"current_setting ( 'server_version_num' ) ::integer / 10000 <> 17",
			"message = 'rag_pg17_retrieval_requires_postgresql_17'",
			"current_setting ( 'shared_preload_libraries' )",
			"message = 'rag_pg17_retrieval_requires_pg_textsearch_preload'",
			"from pg_available_extension_versions",
			"name = 'vector' and version = '0.8.5'",
			"name = 'pg_textsearch' and version = '1.3.1'",
			"message = 'rag_pg17_retrieval_extension_version_unavailable'",
			"create extension if not exists vector version '0.8.5'",
			"create extension if not exists pg_textsearch version '1.3.1'")
	})

	t.Run("installs immutable vector and BM25 projections", func(t *testing.T) {
		assertPhase15Fragments(t, up,
			"038 must install both reviewed physical retrieval projections",
			"create view knowledge_pgvector_shadow_sources",
			"create table knowledge_child_vector_shadow_projections",
			"embedding_vector vector ( 1024 ) not null",
			"using hnsw ( embedding_vector vector_cosine_ops )",
			"create function knowledge_backfill_pgvector_shadow",
			"shadow.embedding_vector::real[] = source.embedding_vector",
			"message = 'rag_pgvector_shadow_backfill_incomplete'",
			"create view knowledge_bm25_shadow_build_sources",
			"create view knowledge_bm25_shadow_sources",
			"create table knowledge_child_bm25_shadow_projections",
			"using bm25 ( bm25_text )",
			"create function knowledge_backfill_bm25_shadow",
			"message = 'rag_bm25_shadow_backfill_incomplete'",
			"before update or delete on knowledge_child_vector_shadow_projections",
			"before update or delete on knowledge_child_bm25_shadow_projections")
	})

	t.Run("keeps hybrid reads bounded deterministic and reauthorized", func(t *testing.T) {
		assertPhase15Fragments(t, up,
			"038 must preserve the qualified candidate-driven hybrid reader",
			"create function knowledge_fetch_hybrid_shadow_diagnostics",
			"to_bm25query ( bm25_query , 'idx_knowledge_child_bm25_shadow_text' )",
			") < 0",
			"shadow.embedding_vector <=> p_query_embedding",
			"oversample_limit := least ( p_limit * 8 , 400 )",
			"cross join lateral",
			"offset 0",
			"1.0 / ( rrf_constant + bm25.lane_rank )",
			"1.0 / ( rrf_constant + dense.lane_rank )",
			"least ( coalesce ( fused.bm25_rank , 2147483647 )",
			"fused.child_chunk_id",
			"create or replace function knowledge_fetch_profiled_query_evidence_candidates",
			"selected_profile = 'pg17_bm25_pgvector_v1'",
			"from knowledge_fetch_hybrid_shadow_diagnostics")
	})

	t.Run("binds activation publication and generation cutover to readiness", func(t *testing.T) {
		assertPhase15Fragments(t, up,
			"038 must preserve the reviewed activation and write fences",
			"create function knowledge_assert_pg17_retrieval_profile_ready",
			"message = 'rag_retrieval_profile_backfill_incomplete'",
			"perform pg_advisory_xact_lock ( 1296912978 , 3 )",
			"perform pg_advisory_xact_lock ( 1296912978 , 4 )",
			"perform pg_advisory_xact_lock ( 1296912978 , 5 )",
			"create function knowledge_sync_pg17_retrieval_materialization",
			"message = 'rag_retrieval_materialization_sync_incomplete'",
			"after insert or update of active_materialization_id",
			"create function knowledge_assert_pg17_generation_ready",
			"message = 'rag_retrieval_generation_backfill_incomplete'",
			"before update of active_index_generation_id")
	})

	t.Run("keeps accelerator access narrower than runtime access", func(t *testing.T) {
		assertPhase15Fragments(t, up,
			"038 must expose references through hardened functions only",
			"quote_ident ( current_schema ( ) ) || ' , pg_catalog , pg_temp'",
			"revoke all on knowledge_pgvector_shadow_sources from public",
			"revoke all on knowledge_bm25_shadow_build_sources from public",
			"revoke all on knowledge_bm25_shadow_sources from public",
			"grant execute on function knowledge_backfill_pgvector_shadow",
			"to rag_replay_operator",
			"grant execute on function knowledge_backfill_bm25_shadow",
			"grant execute on function knowledge_fetch_hybrid_shadow_diagnostics",
			"grant execute on function knowledge_fetch_profiled_query_evidence_candidates",
			"to rag_worker_executor , go_api_runtime")
		if strings.Contains(up, "to go_api_runtime , rag_replay_operator") {
			t.Fatal("038 must not grant projection mutation to go_api_runtime")
		}
	})

	t.Run("rollback requires legacy and preserves extensions plus REAL arrays", func(t *testing.T) {
		assertPhase15Fragments(t, down,
			"038 rollback must fail closed and remove every PG17 retrieval layer",
			"message = 'rag_retrieval_profile_rollback_requires_legacy'",
			"drop trigger knowledge_corpus_head_pg17_retrieval_fence",
			"drop trigger knowledge_document_projection_head_pg17_retrieval",
			"drop function knowledge_assert_pg17_retrieval_profile_ready",
			"drop function if exists knowledge_fetch_hybrid_shadow_diagnostics",
			"drop table if exists knowledge_child_bm25_shadow_projections",
			"drop table if exists knowledge_child_vector_shadow_projections")
		for _, forbidden := range []string{
			"drop extension",
			"drop table knowledge_child_search_projections",
			"drop column embedding_vector",
		} {
			if strings.Contains(down, forbidden) {
				t.Fatalf("038 rollback must preserve compatibility data/capabilities; found %q", forbidden)
			}
		}
	})
}

func TestRAGPG17BM25PGVectorMigrationFreezesQualifiedOperationalDDL(t *testing.T) {
	upBytes, err := migrationfiles.FS.ReadFile(
		"038_pg17_bm25_pgvector_retrieval.up.sql",
	)
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile(
		"038_pg17_bm25_pgvector_retrieval.down.sql",
	)
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		migration string
		path      string
	}{
		{string(upBytes), "g18-pgvector-shadow/00-shadow-schema.up.sql"},
		{string(upBytes), "g18-hybrid-shadow/00-shadow-schema.up.sql"},
		{string(upBytes), "g18-profile-cutover/00-profile-router.up.sql"},
		{string(upBytes), "g18-profile-cutover/10-active-projection-maintenance.up.sql"},
		{string(upBytes), "g18-profile-cutover/15-generation-cutover-fence.up.sql"},
		{string(downBytes), "g18-profile-cutover/15-generation-cutover-fence.down.sql"},
		{string(downBytes), "g18-profile-cutover/10-active-projection-maintenance.down.sql"},
		{string(downBytes), "g18-profile-cutover/00-profile-router.down.sql"},
		{string(downBytes), "g18-hybrid-shadow/00-shadow-schema.down.sql"},
		{string(downBytes), "g18-pgvector-shadow/00-shadow-schema.down.sql"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.path, func(t *testing.T) {
			contents, readErr := os.ReadFile(filepath.Join(
				"..", "..", "..", "ops", testCase.path,
			))
			if readErr != nil {
				t.Fatal(readErr)
			}
			const psqlPreamble = "\\set ON_ERROR_STOP on\n\n"
			operational := strings.TrimSuffix(
				strings.TrimPrefix(string(contents), psqlPreamble),
				"\n",
			)
			if strings.HasPrefix(operational, "\\set ") {
				t.Fatal("operational source has an unexpected psql preamble")
			}
			if !strings.Contains(testCase.migration, operational) {
				t.Fatal("migration drifted from the qualified operational source")
			}
		})
	}
	if strings.Contains(string(upBytes), "\\set ") ||
		strings.Contains(string(downBytes), "\\set ") {
		t.Fatal("embedded migration must not require psql preprocessing")
	}
}
