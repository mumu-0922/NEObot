-- Restore the pre-G7.5 readiness surface while leaving migration 012 objects in
-- place. Rollback of 012 owns dropping the search completeness function itself.

CREATE OR REPLACE FUNCTION knowledge_rag_worker_readiness()
RETURNS TABLE(
  consumer_ready BOOLEAN,
  projection_ready BOOLEAN,
  active_index_generation_id UUID,
  detail JSONB
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  WITH required_function(signature) AS (
    VALUES
      ('knowledge_claim_outbox(text,uuid,uuid,integer)'),
      ('knowledge_apply_and_ack_outbox(text,uuid,uuid,uuid,text,uuid,text,text)'),
      ('knowledge_retry_outbox(uuid,uuid,uuid,text,integer)'),
      ('knowledge_fail_outbox(uuid,uuid,uuid,text)'),
      ('knowledge_claim_processing_job(uuid,uuid,integer,text[])'),
      ('knowledge_heartbeat_processing_job(uuid,uuid,uuid,integer)'),
      ('knowledge_finish_processing_job(uuid,uuid,uuid,text,text,integer)'),
      ('knowledge_claim_collection_purge(uuid,uuid,integer)'),
      ('knowledge_enumerate_collection_purge(uuid,uuid,uuid,integer,integer)'),
      ('knowledge_claim_collection_purge_item(uuid,uuid,integer)'),
      ('knowledge_finish_collection_purge_item(uuid,uuid,uuid,boolean,text)'),
      ('knowledge_complete_collection_purge(uuid)')
  ), worker_capability AS (
    SELECT COALESCE(bool_and(
      to_regprocedure(signature) IS NOT NULL
      AND has_function_privilege(
        session_user,
        to_regprocedure(signature),
        'EXECUTE'
      )
    ), false) AS ready
    FROM required_function
  )
  SELECT
    worker_capability.ready,
    COALESCE(state.readiness = 'ready', false),
    head.active_index_generation_id,
    jsonb_build_object(
      'consumer', CASE
        WHEN worker_capability.ready THEN 'ready'
        ELSE 'not_ready'
      END,
      'projection', COALESCE(state.readiness, 'not_ready'),
      'headRevision', head.head_revision,
      'corpusProjectionRevision', head.corpus_projection_revision
    )
  FROM knowledge_corpus_projection_head head
  CROSS JOIN worker_capability
  LEFT JOIN knowledge_projection_state state
    ON state.index_generation_id = head.active_index_generation_id
  WHERE head.singleton_id = 1
$function$;

ALTER FUNCTION knowledge_rag_worker_readiness()
  OWNER TO rag_projection_owner;
REVOKE ALL ON FUNCTION knowledge_rag_worker_readiness() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION knowledge_rag_worker_readiness()
TO rag_worker_executor, rag_api_reader, go_api_runtime;
