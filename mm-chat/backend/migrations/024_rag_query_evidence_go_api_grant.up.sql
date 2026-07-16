-- G7.8 Go strict-RAG chat assembles evidence candidates inside the API
-- process, then immediately reauthorizes/hydrates those references before any
-- answer provider sees source text.

GRANT EXECUTE ON FUNCTION knowledge_fetch_query_evidence_candidates(
  UUID[], TEXT, INTEGER
) TO go_api_runtime;
