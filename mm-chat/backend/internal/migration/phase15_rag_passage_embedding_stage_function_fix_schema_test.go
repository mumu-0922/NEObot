package migration

import (
	"strings"
	"testing"
)

func TestPhase15RAGPassageEmbeddingStageFunctionFixContract(t *testing.T) {
	up := readPhase15SQL(t, "018_rag_passage_embedding_stage_function_fix.up.sql")
	down := readPhase15SQL(t, "018_rag_passage_embedding_stage_function_fix.down.sql")

	assertPhase15Fragments(t, up,
		"018 must replace the stage function without widening table grants",
		"create or replace function knowledge_stage_passage_embedding",
		"p_embedding_vector real[]",
		"embedding_vector = p_embedding_vector",
		"embedding_vector_sha256 = p_embedding_vector_sha256",
		"status = 'ready'",
		"ready_at = coalesce ( search.ready_at , clock_timestamp ( ) )",
		"search.embedding_model_id = 'jina-embeddings-v4'",
		"search.embedding_dimensions = 1024",
		"to rag_worker_executor")
	assertPhase15NotContains(t, up,
		"018 must not update immutable embedding metadata columns",
		"set embedding_model_id = 'jina-embeddings-v4'",
		"embedding_dimensions = 1024 , embedding_vector")

	assertPhase15Fragments(t, down,
		"018 rollback restores the migration-015 function shape",
		"create or replace function knowledge_stage_passage_embedding",
		"set embedding_model_id = 'jina-embeddings-v4'",
		"embedding_dimensions = 1024 , embedding_vector",
		"to rag_worker_executor")
}

func assertPhase15NotContains(
	t *testing.T,
	sql string,
	invariant string,
	fragments ...string,
) {
	t.Helper()

	for _, fragment := range fragments {
		if strings.Contains(sql, fragment) {
			t.Errorf("%s: forbidden SQL semantic fragment %q", invariant, fragment)
		}
	}
}
