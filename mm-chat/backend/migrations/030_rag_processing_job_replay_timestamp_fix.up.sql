-- G11.9D.2.3b live proof hardening: use one timestamp for replay job fields so
-- available_at cannot precede created_at by a clock tick.

CREATE OR REPLACE FUNCTION knowledge_replay_processing_job(
  p_job_id UUID,
  p_expected_error_code TEXT,
  p_successor_job_id UUID,
  p_operator_id UUID,
  p_reason TEXT
) RETURNS UUID
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  failed_job knowledge_processing_jobs%ROWTYPE;
  replayed_at TIMESTAMPTZ := clock_timestamp();
BEGIN
  IF p_expected_error_code IS NULL
    OR p_expected_error_code !~ '^[A-Z0-9_]{1,64}$'
    OR p_successor_job_id IS NULL OR p_operator_id IS NULL
    OR p_successor_job_id = p_job_id
    OR p_reason IS NULL OR octet_length(p_reason) NOT BETWEEN 1 AND 1024
    OR length(trim(p_reason)) = 0
  THEN
    RAISE EXCEPTION USING ERRCODE='22023',
      MESSAGE='RAG_REPLAY_ARGUMENT_INVALID';
  END IF;

  SELECT * INTO failed_job
  FROM knowledge_processing_jobs
  WHERE id=p_job_id AND status='failed'
    AND error_code=p_expected_error_code
    AND NOT legacy_projection_unbound
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_REPLAY_PRECONDITION_FAILED';
  END IF;

  INSERT INTO knowledge_processing_jobs(
    id,collection_id,document_id,document_version_id,file_id,
    stage,operation,processor,endpoint_id,governance_profile_id,
    governance_revision,governance_head_revision,collection_consent_id,
    collection_consent_revision,collection_acl_revision,
    collection_visibility_epoch,collection_processing_revision,
    document_visibility_epoch,requested_by_user_id,caused_by_job_id,
    idempotency_scope,idempotency_key,request_hash,status,
    attempt_count,max_attempts,available_at,lease_owner,
    lease_expires_at,completed_at,error_code,created_at,updated_at,
    lease_token,index_generation_id,materialization_id,model_id,
    legacy_projection_unbound
  ) VALUES (
    p_successor_job_id,failed_job.collection_id,failed_job.document_id,
    failed_job.document_version_id,failed_job.file_id,
    failed_job.stage,failed_job.operation,failed_job.processor,
    failed_job.endpoint_id,failed_job.governance_profile_id,
    failed_job.governance_revision,failed_job.governance_head_revision,
    failed_job.collection_consent_id,failed_job.collection_consent_revision,
    failed_job.collection_acl_revision,failed_job.collection_visibility_epoch,
    failed_job.collection_processing_revision,
    failed_job.document_visibility_epoch,failed_job.requested_by_user_id,
    failed_job.id,'replay:'||failed_job.id::TEXT,
    p_successor_job_id::TEXT,failed_job.request_hash,'pending',0,
    failed_job.max_attempts,replayed_at,NULL,NULL,NULL,NULL,
    replayed_at,replayed_at,NULL,
    failed_job.index_generation_id,failed_job.materialization_id,
    failed_job.model_id,failed_job.legacy_projection_unbound
  );

  INSERT INTO knowledge_processing_job_replays(
    failed_job_id,successor_job_id,expected_error_code,
    operator_id,reason,replayed_at
  ) VALUES (
    p_job_id,p_successor_job_id,p_expected_error_code,
    p_operator_id,trim(p_reason),replayed_at
  );
  RETURN p_successor_job_id;
END
$function$;
