-- Forward-repair terminal RAG failure projection without changing the parent
-- Document lifecycle. Failed initial/replacement Versions remain explicit and
-- reprocessable while an already-active current Version stays available.

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

WITH ranked_job AS (
  SELECT
    job.document_id,
    job.document_version_id,
    job.status,
    job.error_code,
    job.updated_at,
    row_number() OVER (
      PARTITION BY job.document_id, job.document_version_id
      ORDER BY job.created_at DESC, job.id DESC
    ) AS job_rank
  FROM knowledge_processing_jobs AS job
  WHERE job.stage IN ('parse', 'passage_embedding')
    AND NOT job.legacy_projection_unbound
), latest_failure AS (
  SELECT
    ranked_job.document_id,
    ranked_job.document_version_id,
    ranked_job.error_code,
    ranked_job.updated_at
  FROM ranked_job
  WHERE ranked_job.job_rank = 1
    AND ranked_job.status = 'failed'
    AND ranked_job.error_code IS NOT NULL
)
UPDATE knowledge_document_versions AS version
SET status = 'failed',
    error_code = latest_failure.error_code,
    updated_at = GREATEST(version.updated_at, latest_failure.updated_at)
FROM latest_failure
WHERE version.document_id = latest_failure.document_id
  AND version.id = latest_failure.document_version_id
  AND version.status IN ('uploaded', 'processing')
  AND NOT EXISTS (
    SELECT 1
    FROM knowledge_processing_jobs AS live_job
    WHERE live_job.document_id = version.document_id
      AND live_job.document_version_id = version.id
      AND live_job.status IN ('pending', 'processing')
  );

CREATE OR REPLACE FUNCTION knowledge_claim_processing_job(
  p_worker_id UUID,
  p_lease_token UUID,
  p_lease_seconds INTEGER,
  p_allowed_stages TEXT[]
) RETURNS SETOF knowledge_processing_jobs
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF p_worker_id IS NULL OR p_lease_token IS NULL
    OR p_lease_token = '00000000-0000-0000-0000-000000000000'::UUID
    OR p_lease_seconds IS NULL OR p_lease_seconds NOT BETWEEN 1 AND 3600
    OR cardinality(p_allowed_stages) < 1
    OR p_allowed_stages IS NULL
    OR array_position(p_allowed_stages, NULL) IS NOT NULL
    OR NOT (p_allowed_stages <@ ARRAY['parse', 'passage_embedding', 'purge']::TEXT[])
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'RAG_JOB_CLAIM_ARGUMENT_INVALID';
  END IF;

  WITH exhausted_job AS (
    UPDATE knowledge_processing_jobs AS job
    SET status = 'failed', lease_owner = NULL, lease_token = NULL,
        lease_expires_at = NULL, completed_at = clock_timestamp(),
        error_code = 'MAX_ATTEMPTS_EXCEEDED', updated_at = clock_timestamp()
    WHERE job.status = 'processing'
      AND job.lease_expires_at <= clock_timestamp()
      AND job.attempt_count >= job.max_attempts
    RETURNING job.id, job.document_id, job.document_version_id,
      job.stage, job.error_code, job.updated_at
  )
  UPDATE knowledge_document_versions AS version
  SET status = 'failed',
      error_code = exhausted_job.error_code,
      updated_at = GREATEST(version.updated_at, exhausted_job.updated_at)
  FROM exhausted_job
  WHERE exhausted_job.stage IN ('parse', 'passage_embedding')
    AND version.document_id = exhausted_job.document_id
    AND version.id = exhausted_job.document_version_id
    AND version.status IN ('uploaded', 'processing')
    AND NOT EXISTS (
      SELECT 1
      FROM knowledge_processing_jobs AS other_job
      WHERE other_job.document_id = exhausted_job.document_id
        AND other_job.document_version_id = exhausted_job.document_version_id
        AND other_job.id <> exhausted_job.id
        AND other_job.status IN ('pending', 'processing')
    );

  RETURN QUERY
  WITH candidate AS (
    SELECT id
    FROM knowledge_processing_jobs
    WHERE NOT legacy_projection_unbound
      AND stage = ANY(p_allowed_stages)
      AND attempt_count < max_attempts
      AND (
        (status = 'pending' AND available_at <= clock_timestamp())
        OR (status = 'processing' AND lease_expires_at <= clock_timestamp())
      )
    ORDER BY available_at, created_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT 1
  )
  UPDATE knowledge_processing_jobs AS job
  SET status = 'processing', attempt_count = job.attempt_count + 1,
      lease_owner = p_worker_id, lease_token = p_lease_token,
      lease_expires_at = clock_timestamp() + make_interval(secs => p_lease_seconds),
      completed_at = NULL, error_code = NULL, updated_at = clock_timestamp()
  FROM candidate
  WHERE job.id = candidate.id
  RETURNING job.*;
END
$function$;

CREATE OR REPLACE FUNCTION knowledge_finish_processing_job(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_outcome TEXT,
  p_error_code TEXT,
  p_retry_after_seconds INTEGER
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  attempts INTEGER;
  maximum INTEGER;
  job_stage TEXT;
  job_document_id UUID;
  job_document_version_id UUID;
  terminal_error_code TEXT;
BEGIN
  IF p_outcome IS NULL
    OR p_outcome NOT IN ('succeeded', 'retry', 'failed', 'cancelled')
    OR p_retry_after_seconds IS NULL
    OR p_retry_after_seconds NOT BETWEEN 0 AND 86400
    OR (
      p_outcome IN ('retry', 'failed')
      AND (p_error_code IS NULL OR p_error_code !~ '^[A-Z0-9_]{1,64}$')
    )
    OR (p_outcome IN ('succeeded', 'cancelled') AND p_error_code IS NOT NULL)
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'RAG_JOB_FINISH_ARGUMENT_INVALID';
  END IF;

  SELECT
    job.attempt_count,
    job.max_attempts,
    job.stage,
    job.document_id,
    job.document_version_id
  INTO
    attempts,
    maximum,
    job_stage,
    job_document_id,
    job_document_version_id
  FROM knowledge_processing_jobs AS job
  WHERE job.id = p_job_id AND job.status = 'processing'
    AND job.lease_owner = p_worker_id AND job.lease_token = p_lease_token
    AND job.lease_expires_at > clock_timestamp()
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_JOB_LEASE';
  END IF;

  IF p_outcome = 'retry' AND attempts < maximum THEN
    UPDATE knowledge_processing_jobs
    SET status = 'pending', available_at = clock_timestamp()
          + make_interval(secs => p_retry_after_seconds),
        lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
        completed_at = NULL, error_code = NULL, updated_at = clock_timestamp()
    WHERE id = p_job_id;
  ELSIF p_outcome = 'retry' THEN
    terminal_error_code := 'MAX_ATTEMPTS_EXCEEDED';
    UPDATE knowledge_processing_jobs
    SET status = 'failed', lease_owner = NULL, lease_token = NULL,
        lease_expires_at = NULL, completed_at = clock_timestamp(),
        error_code = terminal_error_code, updated_at = clock_timestamp()
    WHERE id = p_job_id;
  ELSE
    IF p_outcome = 'failed' THEN
      terminal_error_code := p_error_code;
    END IF;
    UPDATE knowledge_processing_jobs
    SET status = p_outcome, lease_owner = NULL, lease_token = NULL,
        lease_expires_at = NULL, completed_at = clock_timestamp(),
        error_code = CASE WHEN p_outcome = 'failed' THEN p_error_code ELSE NULL END,
        updated_at = clock_timestamp()
    WHERE id = p_job_id;
  END IF;

  IF terminal_error_code IS NOT NULL
    AND job_stage IN ('parse', 'passage_embedding')
  THEN
    UPDATE knowledge_document_versions
    SET status = 'failed',
        error_code = terminal_error_code,
        updated_at = clock_timestamp()
    WHERE document_id = job_document_id
      AND id = job_document_version_id
      AND status IN ('uploaded', 'processing');
  END IF;

  RETURN true;
END
$function$;

ALTER FUNCTION knowledge_claim_processing_job(UUID, UUID, INTEGER, TEXT[])
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_finish_processing_job(
  UUID, UUID, UUID, TEXT, TEXT, INTEGER
) OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION knowledge_claim_processing_job(
  UUID, UUID, INTEGER, TEXT[]
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_finish_processing_job(
  UUID, UUID, UUID, TEXT, TEXT, INTEGER
) FROM PUBLIC;

GRANT UPDATE(status, error_code, updated_at)
  ON knowledge_document_versions
TO rag_projection_owner;
GRANT EXECUTE ON FUNCTION knowledge_claim_processing_job(
  UUID, UUID, INTEGER, TEXT[]
) TO rag_worker_executor;
GRANT EXECUTE ON FUNCTION knowledge_finish_processing_job(
  UUID, UUID, UUID, TEXT, TEXT, INTEGER
) TO rag_worker_executor;
