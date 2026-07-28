-- Add the fixed BGE-M3 vector lane, lease-fenced derived embedding jobs, and
-- zero-prompt-injection hybrid/RRF/rerank observations. The legacy v1 reader
-- remains the only prompt and Usage authority.

DO $memory_hybrid_prerequisite$
DECLARE
  v_server_version INTEGER := current_setting('server_version_num')::INTEGER;
  v_vector_version TEXT;
BEGIN
  IF v_server_version < 170000 OR v_server_version >= 180000 THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000', MESSAGE = 'MEMORY_HYBRID_REQUIRES_POSTGRESQL_17';
  END IF;
  SELECT extversion INTO v_vector_version
  FROM pg_extension WHERE extname = 'vector';
  IF v_vector_version IS DISTINCT FROM '0.8.5' THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000', MESSAGE = 'MEMORY_HYBRID_REQUIRES_PGVECTOR_0_8_5';
  END IF;
  IF to_regtype('vector') IS NULL
    OR to_regprocedure('memory_compare_lexical_shadow(uuid,uuid,uuid,uuid,text,text,jsonb,integer)') IS NULL
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000', MESSAGE = 'MEMORY_HYBRID_REQUIRES_LEXICAL_PROJECTION';
  END IF;
END
$memory_hybrid_prerequisite$;

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

ALTER TABLE user_memory_search_projections
  ADD COLUMN embedding_profile_id TEXT NOT NULL
    DEFAULT 'siliconflow_bge_m3_v1',
  ADD COLUMN embedding_model_id TEXT NOT NULL
    DEFAULT 'Pro/BAAI/bge-m3',
  ADD COLUMN embedding_dimensions SMALLINT NOT NULL DEFAULT 1024,
  ADD COLUMN embedding_status TEXT NOT NULL DEFAULT 'pending',
  ADD COLUMN embedding_vector vector(1024),
  ADD COLUMN embedding_error_code TEXT,
  ADD COLUMN embedding_updated_at TIMESTAMPTZ,
  ADD CONSTRAINT user_memory_search_projection_embedding_profile_check CHECK (
    embedding_profile_id = 'siliconflow_bge_m3_v1'
  ),
  ADD CONSTRAINT user_memory_search_projection_embedding_model_check CHECK (
    embedding_model_id = 'Pro/BAAI/bge-m3'
    AND embedding_dimensions = 1024
  ),
  ADD CONSTRAINT user_memory_search_projection_embedding_status_check CHECK (
    embedding_status IN ('pending', 'ready', 'failed')
  ),
  ADD CONSTRAINT user_memory_search_projection_embedding_error_check CHECK (
    embedding_error_code IS NULL
    OR embedding_error_code ~ '^[A-Z][A-Z0-9_]{0,63}$'
  ),
  ADD CONSTRAINT user_memory_search_projection_embedding_shape_check CHECK (
    (embedding_status = 'pending' AND embedding_vector IS NULL
      AND embedding_error_code IS NULL)
    OR (embedding_status = 'ready' AND embedding_vector IS NOT NULL
      AND embedding_error_code IS NULL AND embedding_updated_at IS NOT NULL)
    OR (embedding_status = 'failed' AND embedding_vector IS NULL
      AND embedding_error_code IS NOT NULL AND embedding_updated_at IS NOT NULL)
  );

CREATE INDEX idx_user_memory_search_projection_vector
  ON user_memory_search_projections
  USING hnsw (embedding_vector vector_cosine_ops)
  WITH (m = 16, ef_construction = 64)
  WHERE embedding_status = 'ready';

CREATE TABLE user_memory_embedding_jobs (
  job_id UUID PRIMARY KEY,
  memory_id UUID NOT NULL UNIQUE,
  user_id UUID NOT NULL,
  projection_generation BIGINT NOT NULL CHECK (projection_generation >= 1),
  memory_revision BIGINT NOT NULL CHECK (memory_revision >= 1),
  content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  visibility_epoch BIGINT NOT NULL CHECK (visibility_epoch >= 1),
  scope_type TEXT NOT NULL CHECK (
    scope_type IN ('global', 'project', 'conversation')
  ),
  project_id UUID,
  scope_conversation_id UUID,
  scope_generation BIGINT NOT NULL CHECK (scope_generation >= 1),
  embedding_profile_id TEXT NOT NULL CHECK (
    embedding_profile_id = 'siliconflow_bge_m3_v1'
  ),
  embedding_model_id TEXT NOT NULL CHECK (
    embedding_model_id = 'Pro/BAAI/bge-m3'
  ),
  embedding_dimensions SMALLINT NOT NULL CHECK (embedding_dimensions = 1024),
  provider_record_id UUID,
  provider_config_updated_at TIMESTAMPTZ,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (
    status IN ('pending', 'processing', 'completed', 'dead_letter')
  ),
  attempt_count INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 8 CHECK (max_attempts BETWEEN 1 AND 32),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  lease_owner UUID,
  lease_token UUID,
  lease_expires_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  error_code TEXT CHECK (
    error_code IS NULL OR error_code ~ '^[A-Z][A-Z0-9_]{0,63}$'
  ),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT user_memory_embedding_job_attempts_check CHECK (
    attempt_count BETWEEN 0 AND max_attempts
  ),
  CONSTRAINT user_memory_embedding_job_provider_shape CHECK (
    (provider_record_id IS NULL AND provider_config_updated_at IS NULL)
    OR (provider_record_id IS NOT NULL AND provider_config_updated_at IS NOT NULL)
  ),
  CONSTRAINT user_memory_embedding_job_scope_shape CHECK (
    (scope_type = 'global' AND project_id IS NULL
      AND scope_conversation_id IS NULL AND scope_generation = 1)
    OR (scope_type = 'project' AND project_id IS NOT NULL
      AND scope_conversation_id IS NULL)
    OR (scope_type = 'conversation' AND project_id IS NULL
      AND scope_conversation_id IS NOT NULL)
  ),
  CONSTRAINT user_memory_embedding_job_state_shape CHECK (
    (status = 'pending' AND lease_owner IS NULL AND lease_token IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NULL
      AND error_code IS NULL AND attempt_count < max_attempts)
    OR (status = 'processing' AND lease_owner IS NOT NULL
      AND lease_token IS NOT NULL AND lease_expires_at IS NOT NULL
      AND completed_at IS NULL AND error_code IS NULL
      AND attempt_count BETWEEN 1 AND max_attempts)
    OR (status = 'completed' AND lease_owner IS NULL AND lease_token IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NOT NULL
      AND error_code IS NULL AND attempt_count BETWEEN 1 AND max_attempts)
    OR (status = 'dead_letter' AND lease_owner IS NULL AND lease_token IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NOT NULL
      AND error_code IS NOT NULL AND attempt_count BETWEEN 1 AND max_attempts)
  ),
  CONSTRAINT user_memory_embedding_job_timestamps_order CHECK (
    updated_at >= created_at
    AND available_at >= created_at
    AND (lease_expires_at IS NULL OR lease_expires_at >= created_at)
    AND (completed_at IS NULL OR completed_at >= created_at)
  ),
  CONSTRAINT user_memory_embedding_job_projection_owner_fk
    FOREIGN KEY (memory_id, user_id)
    REFERENCES user_memory_search_projections(memory_id, user_id)
    ON DELETE CASCADE
);

CREATE INDEX idx_user_memory_embedding_jobs_claim
  ON user_memory_embedding_jobs(available_at, created_at, job_id)
  WHERE status = 'pending';
CREATE INDEX idx_user_memory_embedding_jobs_expired_lease
  ON user_memory_embedding_jobs(lease_expires_at, job_id)
  WHERE status = 'processing';
CREATE INDEX idx_user_memory_embedding_jobs_user_created
  ON user_memory_embedding_jobs(user_id, created_at, job_id);

CREATE FUNCTION memory_queue_embedding_projection()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  IF NEW.embedding_status <> 'pending' THEN
    RETURN NEW;
  END IF;
  INSERT INTO user_memory_embedding_jobs (
    job_id, memory_id, user_id, projection_generation, memory_revision,
    content_hash, visibility_epoch, scope_type, project_id,
    scope_conversation_id, scope_generation, embedding_profile_id,
    embedding_model_id, embedding_dimensions, status, attempt_count, max_attempts,
    available_at, created_at, updated_at
  ) VALUES (
    gen_random_uuid(), NEW.memory_id, NEW.user_id,
    NEW.projection_generation, NEW.memory_revision, NEW.content_hash,
    NEW.visibility_epoch, NEW.scope_type, NEW.project_id,
    NEW.scope_conversation_id, NEW.scope_generation,
    NEW.embedding_profile_id, NEW.embedding_model_id,
    NEW.embedding_dimensions, 'pending', 0, 8, v_now, v_now, v_now
  )
  ON CONFLICT (memory_id) DO UPDATE SET
    job_id = gen_random_uuid(),
    user_id = EXCLUDED.user_id,
    projection_generation = EXCLUDED.projection_generation,
    memory_revision = EXCLUDED.memory_revision,
    content_hash = EXCLUDED.content_hash,
    visibility_epoch = EXCLUDED.visibility_epoch,
    scope_type = EXCLUDED.scope_type,
    project_id = EXCLUDED.project_id,
    scope_conversation_id = EXCLUDED.scope_conversation_id,
    scope_generation = EXCLUDED.scope_generation,
    embedding_profile_id = EXCLUDED.embedding_profile_id,
    embedding_model_id = EXCLUDED.embedding_model_id,
    embedding_dimensions = EXCLUDED.embedding_dimensions,
    provider_record_id = NULL,
    provider_config_updated_at = NULL,
    status = 'pending',
    attempt_count = 0,
    max_attempts = 8,
    available_at = v_now,
    lease_owner = NULL,
    lease_token = NULL,
    lease_expires_at = NULL,
    completed_at = NULL,
    error_code = NULL,
    created_at = v_now,
    updated_at = v_now;
  RETURN NEW;
END
$function$;

CREATE TRIGGER user_memory_search_projection_embedding_queue
AFTER INSERT OR UPDATE OF
  projection_generation, memory_revision, content_hash,
  visibility_epoch, scope_type, project_id, scope_conversation_id,
  scope_generation,
  embedding_profile_id, embedding_model_id, embedding_dimensions,
  embedding_status
ON user_memory_search_projections
FOR EACH ROW EXECUTE FUNCTION memory_queue_embedding_projection();

CREATE FUNCTION memory_invalidate_vector_projection()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF OLD.revision IS DISTINCT FROM NEW.revision
    OR OLD.content_hash IS DISTINCT FROM NEW.content_hash
  THEN
    UPDATE user_memory_search_projections projection
    SET embedding_status = 'pending', embedding_vector = NULL,
        embedding_error_code = NULL, embedding_updated_at = NULL,
        updated_at = clock_timestamp()
    WHERE projection.memory_id = NEW.id
      AND projection.user_id = NEW.user_id
      AND projection.memory_revision = NEW.revision
      AND projection.content_hash = NEW.content_hash;
  END IF;
  RETURN NEW;
END
$function$;

-- PostgreSQL runs same-kind triggers alphabetically. The PR7 lexical refresh
-- trigger therefore completes before this vector invalidation trigger.
CREATE TRIGGER user_memories_vector_projection_invalidate
AFTER UPDATE OF revision, content_hash ON user_memories
FOR EACH ROW EXECUTE FUNCTION memory_invalidate_vector_projection();

INSERT INTO user_memory_embedding_jobs (
  job_id, memory_id, user_id, projection_generation, memory_revision,
  content_hash, visibility_epoch, scope_type, project_id,
  scope_conversation_id, scope_generation, embedding_profile_id,
  embedding_model_id, embedding_dimensions, status, attempt_count, max_attempts,
  available_at, created_at, updated_at
)
SELECT
  gen_random_uuid(), projection.memory_id, projection.user_id,
  projection.projection_generation, projection.memory_revision,
  projection.content_hash, projection.visibility_epoch,
  projection.scope_type, projection.project_id,
  projection.scope_conversation_id, projection.scope_generation,
  projection.embedding_profile_id,
  projection.embedding_model_id, projection.embedding_dimensions,
  'pending', 0, 8, clock_timestamp(), clock_timestamp(), clock_timestamp()
FROM user_memory_search_projections projection
ON CONFLICT (memory_id) DO NOTHING;

CREATE TABLE message_memory_hybrid_shadow_observations (
  id UUID PRIMARY KEY,
  assistant_message_id UUID NOT NULL UNIQUE,
  user_id UUID NOT NULL,
  conversation_id UUID NOT NULL,
  retrieval_profile_id TEXT NOT NULL CHECK (
    retrieval_profile_id = 'memory_hybrid_bge_m3_rrf60_v1'
  ),
  embedding_profile_id TEXT NOT NULL CHECK (
    embedding_profile_id = 'siliconflow_bge_m3_v1'
  ),
  projection_generation BIGINT NOT NULL CHECK (projection_generation >= 1),
  query_sha256 TEXT NOT NULL CHECK (query_sha256 ~ '^[0-9a-f]{64}$'),
  baseline_sha256 TEXT NOT NULL CHECK (baseline_sha256 ~ '^[0-9a-f]{64}$'),
  result_sha256 TEXT CHECK (result_sha256 IS NULL OR result_sha256 ~ '^[0-9a-f]{64}$'),
  status TEXT NOT NULL CHECK (status IN ('pending', 'completed', 'failed')),
  result_code TEXT NOT NULL CHECK (
    result_code ~ '^[A-Z][A-Z0-9_]{0,127}$'
  ),
  query_embedding_status TEXT NOT NULL CHECK (
    query_embedding_status IN (
      'ready', 'failed', 'unavailable', 'cutoff', 'redacted'
    )
  ),
  rerank_status TEXT NOT NULL DEFAULT 'pending' CHECK (
    rerank_status IN ('pending', 'applied', 'fallback', 'skipped')
  ),
  fallback_code TEXT NOT NULL DEFAULT 'NONE' CHECK (
    fallback_code ~ '^[A-Z][A-Z0-9_]{0,127}$'
  ),
  baseline_count SMALLINT NOT NULL DEFAULT 0 CHECK (baseline_count BETWEEN 0 AND 5),
  exact_count SMALLINT NOT NULL DEFAULT 0 CHECK (exact_count BETWEEN 0 AND 20),
  bm25_count SMALLINT NOT NULL DEFAULT 0 CHECK (bm25_count BETWEEN 0 AND 30),
  vector_count SMALLINT NOT NULL DEFAULT 0 CHECK (vector_count BETWEEN 0 AND 30),
  rrf_count SMALLINT NOT NULL DEFAULT 0 CHECK (rrf_count BETWEEN 0 AND 20),
  rerank_count SMALLINT NOT NULL DEFAULT 0 CHECK (rerank_count BETWEEN 0 AND 20),
  final_count SMALLINT NOT NULL DEFAULT 0 CHECK (final_count BETWEEN 0 AND 5),
  overlap_count SMALLINT NOT NULL DEFAULT 0 CHECK (overlap_count BETWEEN 0 AND 5),
  estimated_tokens SMALLINT NOT NULL DEFAULT 0 CHECK (estimated_tokens BETWEEN 0 AND 900),
  target_tokens_exceeded BOOLEAN NOT NULL DEFAULT false,
  duration_millis INTEGER NOT NULL DEFAULT 0 CHECK (duration_millis BETWEEN 0 AND 120000),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT message_memory_hybrid_shadow_observation_result_shape CHECK (
    (status = 'pending' AND result_sha256 IS NULL)
    OR (status IN ('completed', 'failed'))
  ),
  CONSTRAINT message_memory_hybrid_shadow_observation_timestamps_order CHECK (
    updated_at >= created_at
  ),
  CONSTRAINT message_memory_hybrid_shadow_observation_assistant_owner_fk
    FOREIGN KEY (assistant_message_id, user_id)
    REFERENCES messages(id, user_id) ON DELETE CASCADE,
  CONSTRAINT message_memory_hybrid_shadow_observation_conversation_owner_fk
    FOREIGN KEY (conversation_id, user_id)
    REFERENCES conversations(id, user_id) ON DELETE CASCADE,
  UNIQUE (id, user_id)
);

CREATE INDEX idx_message_memory_hybrid_shadow_observation_user_created
  ON message_memory_hybrid_shadow_observations(user_id, created_at, id);

CREATE TABLE message_memory_hybrid_shadow_results (
  observation_id UUID NOT NULL,
  user_id UUID NOT NULL,
  lane TEXT NOT NULL CHECK (
    lane IN ('v1', 'exact', 'bm25', 'vector', 'rrf', 'rerank', 'final')
  ),
  ordinal SMALLINT NOT NULL CHECK (
    ordinal >= 1 AND (
      (lane IN ('v1', 'final') AND ordinal <= 5)
      OR (lane IN ('exact', 'rrf', 'rerank') AND ordinal <= 20)
      OR (lane IN ('bm25', 'vector') AND ordinal <= 30)
    )
  ),
  memory_id UUID NOT NULL,
  memory_revision BIGINT NOT NULL CHECK (memory_revision >= 1),
  scope_type TEXT NOT NULL CHECK (
    scope_type IN ('global', 'project', 'conversation')
  ),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (observation_id, lane, ordinal),
  UNIQUE (observation_id, lane, memory_id),
  CONSTRAINT message_memory_hybrid_shadow_result_observation_owner_fk
    FOREIGN KEY (observation_id, user_id)
    REFERENCES message_memory_hybrid_shadow_observations(id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT message_memory_hybrid_shadow_result_memory_owner_fk
    FOREIGN KEY (memory_id, user_id)
    REFERENCES user_memories(id, user_id) ON DELETE CASCADE
);

CREATE INDEX idx_message_memory_hybrid_shadow_result_memory
  ON message_memory_hybrid_shadow_results(user_id, memory_id, created_at);

CREATE FUNCTION memory_worker_claim_embedding_job(
  p_worker_id UUID,
  p_lease_token UUID,
  p_lease_seconds INTEGER
) RETURNS TABLE (
  job_id UUID,
  user_id UUID,
  memory_id UUID,
  projection_generation BIGINT,
  memory_revision BIGINT,
  content_hash TEXT,
  visibility_epoch BIGINT,
  scope_type TEXT,
  project_id UUID,
  scope_conversation_id UUID,
  scope_generation BIGINT,
  embedding_profile_id TEXT,
  embedding_model_id TEXT,
  embedding_dimensions INTEGER,
  attempt_count INTEGER,
  max_attempts INTEGER,
  provider_record_id UUID,
  provider_config_updated_at TIMESTAMPTZ,
  lease_expires_at TIMESTAMPTZ
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_job_id UUID;
  v_provider_id UUID;
  v_provider_updated_at TIMESTAMPTZ;
  v_now TIMESTAMPTZ := clock_timestamp();
  v_lease_expires TIMESTAMPTZ;
BEGIN
  IF p_worker_id IS NULL OR p_lease_token IS NULL
    OR p_lease_seconds NOT BETWEEN 5 AND 900
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_EMBEDDING_LEASE_INVALID';
  END IF;
  v_lease_expires := v_now + make_interval(secs => p_lease_seconds);

  WITH expired_jobs AS MATERIALIZED (
    UPDATE user_memory_embedding_jobs job
    SET status = 'dead_letter', lease_owner = NULL, lease_token = NULL,
        lease_expires_at = NULL, completed_at = v_now,
        error_code = 'LEASE_EXPIRED', updated_at = v_now
    WHERE job.status = 'processing'
      AND job.lease_expires_at <= v_now
      AND job.attempt_count >= job.max_attempts
    RETURNING job.*
  )
  UPDATE user_memory_search_projections projection
  SET embedding_status = 'failed', embedding_vector = NULL,
      embedding_error_code = 'LEASE_EXPIRED',
      embedding_updated_at = v_now, updated_at = v_now
  FROM expired_jobs job
  WHERE projection.memory_id = job.memory_id
    AND projection.user_id = job.user_id
    AND projection.projection_generation = job.projection_generation
    AND projection.memory_revision = job.memory_revision
    AND projection.content_hash = job.content_hash
    AND projection.visibility_epoch = job.visibility_epoch
    AND projection.scope_type = job.scope_type
    AND projection.project_id IS NOT DISTINCT FROM job.project_id
    AND projection.scope_conversation_id IS NOT DISTINCT FROM
      job.scope_conversation_id
    AND projection.scope_generation = job.scope_generation
    AND projection.embedding_profile_id = job.embedding_profile_id
    AND projection.embedding_model_id = job.embedding_model_id
    AND projection.embedding_dimensions = job.embedding_dimensions
    AND projection.embedding_status = 'pending';

  WITH eligible_providers AS MATERIALIZED (
    SELECT provider.*
    FROM provider_configs provider
    WHERE provider.provider_id = 'RAG:SILICONFLOW'
      AND provider.deleted_at IS NULL
      AND provider.encrypted_secret_ref IS NOT NULL
      AND provider.config->>'kind' = 'rag'
      AND provider.config->>'ragProvider' = 'siliconflow'
      AND provider.config->>'enabled' = 'true'
      AND COALESCE(provider.config->>'connectionTestedAt', '') <> ''
      AND COALESCE(provider.config->>'connectionTestSha256', '') = encode(sha256(
        convert_to('rag-provider-connection/v1', 'UTF8') || decode('00', 'hex')
        || convert_to(provider.provider_id, 'UTF8') || decode('00', 'hex')
        || convert_to('siliconflow', 'UTF8') || decode('00', 'hex')
        || convert_to('https://api.siliconflow.cn/v1/embeddings', 'UTF8')
        || decode('00', 'hex')
        || convert_to('Pro/BAAI/bge-m3', 'UTF8') || decode('00', 'hex')
        || convert_to('1024', 'UTF8') || decode('00', 'hex')
        || convert_to('https://api.siliconflow.cn/v1/rerank', 'UTF8')
        || decode('00', 'hex')
        || convert_to('Pro/BAAI/bge-reranker-v2-m3', 'UTF8')
        || decode('00', 'hex')
        || convert_to(provider.encrypted_secret_ref, 'UTF8')
      ), 'hex')
  ), unique_provider_users AS MATERIALIZED (
    SELECT provider.user_id
    FROM eligible_providers provider
    GROUP BY provider.user_id
    HAVING count(*) = 1
  )
  SELECT candidate.job_id, provider.id, provider.updated_at
  INTO v_job_id, v_provider_id, v_provider_updated_at
  FROM user_memory_embedding_jobs candidate
  JOIN user_memory_search_projections projection
    ON projection.memory_id = candidate.memory_id
   AND projection.user_id = candidate.user_id
   AND projection.projection_generation = candidate.projection_generation
   AND projection.memory_revision = candidate.memory_revision
   AND projection.content_hash = candidate.content_hash
   AND projection.visibility_epoch = candidate.visibility_epoch
   AND projection.scope_type = candidate.scope_type
   AND projection.project_id IS NOT DISTINCT FROM candidate.project_id
   AND projection.scope_conversation_id IS NOT DISTINCT FROM
     candidate.scope_conversation_id
   AND projection.scope_generation = candidate.scope_generation
   AND projection.embedding_profile_id = candidate.embedding_profile_id
   AND projection.embedding_model_id = candidate.embedding_model_id
   AND projection.embedding_dimensions = candidate.embedding_dimensions
   AND projection.embedding_status = 'pending'
  JOIN user_memories memory
    ON memory.id = projection.memory_id
   AND memory.user_id = projection.user_id
   AND memory.revision = projection.memory_revision
   AND memory.content_hash = projection.content_hash
   AND memory.visibility_epoch = projection.visibility_epoch
   AND memory.scope_type = projection.scope_type
   AND memory.scope_generation = projection.scope_generation
  JOIN user_memory_state state
    ON state.user_id = memory.user_id
   AND state.visibility_epoch = memory.visibility_epoch
   AND state.active_projection_generation = projection.projection_generation
  JOIN user_memory_settings settings
    ON settings.user_id = memory.user_id
   AND settings.enabled AND settings.search_enabled
   AND (memory.sensitivity = 'normal' OR settings.sensitive_memory_enabled)
  JOIN unique_provider_users unique_provider
    ON unique_provider.user_id = candidate.user_id
  JOIN eligible_providers provider
    ON provider.user_id = unique_provider.user_id
  LEFT JOIN projects scoped_project
    ON projection.scope_type = 'project'
   AND scoped_project.id = projection.project_id
   AND scoped_project.user_id = projection.user_id
   AND scoped_project.lifecycle_status = 'active'
   AND scoped_project.scope_generation = projection.scope_generation
  LEFT JOIN conversations scoped_conversation
    ON projection.scope_type = 'conversation'
   AND scoped_conversation.id = projection.scope_conversation_id
   AND scoped_conversation.user_id = projection.user_id
   AND scoped_conversation.deleted_at IS NULL
   AND scoped_conversation.memory_scope_generation = projection.scope_generation
  WHERE (
      (
        candidate.status = 'pending'
        AND candidate.available_at <= v_now
        AND candidate.attempt_count < candidate.max_attempts
      ) OR (
        candidate.status = 'processing'
        AND candidate.lease_expires_at <= v_now
        AND candidate.attempt_count < candidate.max_attempts
      )
    )
    AND memory.deleted_at IS NULL
    AND memory.enabled
    AND memory.lifecycle_status = 'active'
    AND (memory.valid_from IS NULL OR memory.valid_from <= v_now)
    AND (memory.valid_to IS NULL OR v_now < memory.valid_to)
    AND (memory.expires_at IS NULL OR v_now < memory.expires_at)
    AND (
      projection.scope_type = 'global'
      OR (projection.scope_type = 'project' AND scoped_project.id IS NOT NULL)
      OR (projection.scope_type = 'conversation' AND scoped_conversation.id IS NOT NULL)
    )
  ORDER BY
    CASE WHEN candidate.status = 'processing'
      THEN candidate.lease_expires_at ELSE candidate.available_at END,
    candidate.created_at,
    candidate.job_id
  FOR UPDATE OF candidate SKIP LOCKED
  LIMIT 1;

  IF v_job_id IS NULL THEN
    RETURN;
  END IF;

  UPDATE user_memory_embedding_jobs job
  SET status = 'processing', attempt_count = job.attempt_count + 1,
      provider_record_id = v_provider_id,
      provider_config_updated_at = v_provider_updated_at,
      lease_owner = p_worker_id, lease_token = p_lease_token,
      lease_expires_at = v_lease_expires, completed_at = NULL,
      error_code = NULL, updated_at = v_now
  WHERE job.job_id = v_job_id;

  RETURN QUERY
  SELECT
    job.job_id, job.user_id, job.memory_id,
    job.projection_generation, job.memory_revision, job.content_hash,
    job.visibility_epoch, job.scope_type, job.project_id,
    job.scope_conversation_id, job.scope_generation,
    job.embedding_profile_id, job.embedding_model_id,
    job.embedding_dimensions::INTEGER, job.attempt_count, job.max_attempts,
    job.provider_record_id, job.provider_config_updated_at,
    job.lease_expires_at
  FROM user_memory_embedding_jobs job
  WHERE job.job_id = v_job_id;
END
$function$;

CREATE FUNCTION memory_worker_hydrate_embedding_job(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID
) RETURNS TABLE (
  user_id UUID,
  memory_id UUID,
  content TEXT,
  content_hash TEXT,
  memory_revision BIGINT,
  projection_generation BIGINT,
  visibility_epoch BIGINT,
  scope_type TEXT,
  project_id UUID,
  scope_conversation_id UUID,
  scope_generation BIGINT,
  embedding_profile_id TEXT,
  embedding_model_id TEXT,
  embedding_dimensions INTEGER,
  provider_record_id UUID,
  provider_id TEXT,
  provider_label TEXT,
  encrypted_secret_ref TEXT,
  provider_config JSONB,
  provider_config_updated_at TIMESTAMPTZ
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  RETURN QUERY
  SELECT
    job.user_id, job.memory_id, memory.content, job.content_hash,
    job.memory_revision, job.projection_generation,
    job.visibility_epoch, job.scope_type, job.project_id,
    job.scope_conversation_id, job.scope_generation,
    job.embedding_profile_id, job.embedding_model_id,
    job.embedding_dimensions::INTEGER, provider.id, provider.provider_id,
    provider.label, provider.encrypted_secret_ref, provider.config,
    provider.updated_at
  FROM user_memory_embedding_jobs job
  JOIN user_memory_search_projections projection
    ON projection.memory_id = job.memory_id
   AND projection.user_id = job.user_id
   AND projection.projection_generation = job.projection_generation
   AND projection.memory_revision = job.memory_revision
   AND projection.content_hash = job.content_hash
   AND projection.visibility_epoch = job.visibility_epoch
   AND projection.scope_type = job.scope_type
   AND projection.project_id IS NOT DISTINCT FROM job.project_id
   AND projection.scope_conversation_id IS NOT DISTINCT FROM
     job.scope_conversation_id
   AND projection.scope_generation = job.scope_generation
   AND projection.embedding_profile_id = job.embedding_profile_id
   AND projection.embedding_model_id = job.embedding_model_id
   AND projection.embedding_dimensions = job.embedding_dimensions
   AND projection.embedding_status = 'pending'
  JOIN user_memories memory
    ON memory.id = projection.memory_id
   AND memory.user_id = projection.user_id
   AND memory.revision = projection.memory_revision
   AND memory.content_hash = projection.content_hash
   AND memory.visibility_epoch = projection.visibility_epoch
   AND memory.scope_type = projection.scope_type
   AND memory.scope_generation = projection.scope_generation
  JOIN user_memory_state state
    ON state.user_id = memory.user_id
   AND state.visibility_epoch = memory.visibility_epoch
   AND state.active_projection_generation = projection.projection_generation
  JOIN user_memory_settings settings
    ON settings.user_id = memory.user_id
   AND settings.enabled AND settings.search_enabled
   AND (memory.sensitivity = 'normal' OR settings.sensitive_memory_enabled)
  JOIN provider_configs provider
    ON provider.id = job.provider_record_id
   AND provider.user_id = job.user_id
   AND provider.provider_id = 'RAG:SILICONFLOW'
   AND provider.updated_at = job.provider_config_updated_at
   AND provider.deleted_at IS NULL
  LEFT JOIN projects scoped_project
    ON projection.scope_type = 'project'
   AND scoped_project.id = projection.project_id
   AND scoped_project.user_id = projection.user_id
   AND scoped_project.lifecycle_status = 'active'
   AND scoped_project.scope_generation = projection.scope_generation
  LEFT JOIN conversations scoped_conversation
    ON projection.scope_type = 'conversation'
   AND scoped_conversation.id = projection.scope_conversation_id
   AND scoped_conversation.user_id = projection.user_id
   AND scoped_conversation.deleted_at IS NULL
   AND scoped_conversation.memory_scope_generation = projection.scope_generation
  WHERE job.job_id = p_job_id
    AND job.status = 'processing'
    AND job.lease_owner = p_worker_id
    AND job.lease_token = p_lease_token
    AND job.lease_expires_at > v_now
    AND memory.deleted_at IS NULL
    AND memory.enabled
    AND memory.lifecycle_status = 'active'
    AND (memory.valid_from IS NULL OR memory.valid_from <= v_now)
    AND (memory.valid_to IS NULL OR v_now < memory.valid_to)
    AND (memory.expires_at IS NULL OR v_now < memory.expires_at)
    AND (
      projection.scope_type = 'global'
      OR (projection.scope_type = 'project' AND scoped_project.id IS NOT NULL)
      OR (projection.scope_type = 'conversation' AND scoped_conversation.id IS NOT NULL)
    );
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_EMBEDDING_SOURCE_DRIFT';
  END IF;
END
$function$;

CREATE FUNCTION memory_worker_complete_embedding_job(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_embedding REAL[]
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_job user_memory_embedding_jobs%ROWTYPE;
  v_now TIMESTAMPTZ := clock_timestamp();
  v_norm DOUBLE PRECISION;
BEGIN
  IF p_embedding IS NULL OR cardinality(p_embedding) <> 1024 THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_EMBEDDING_VECTOR_INVALID';
  END IF;
  SELECT sqrt(sum(component::DOUBLE PRECISION * component::DOUBLE PRECISION))
  INTO v_norm FROM unnest(p_embedding) component;
  IF v_norm IS NULL OR v_norm <= 0 OR v_norm > 1e100 THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_EMBEDDING_VECTOR_INVALID';
  END IF;

  SELECT job.* INTO v_job
  FROM user_memory_embedding_jobs job
  JOIN provider_configs provider
    ON provider.id = job.provider_record_id
   AND provider.user_id = job.user_id
   AND provider.provider_id = 'RAG:SILICONFLOW'
   AND provider.updated_at = job.provider_config_updated_at
   AND provider.deleted_at IS NULL
  WHERE job.job_id = p_job_id
    AND job.status = 'processing'
    AND job.lease_owner = p_worker_id
    AND job.lease_token = p_lease_token
    AND job.lease_expires_at > v_now
  FOR UPDATE OF job;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_EMBEDDING_LEASE_LOST';
  END IF;

  UPDATE user_memory_search_projections projection
  SET embedding_vector = p_embedding::vector(1024),
      embedding_status = 'ready', embedding_error_code = NULL,
      embedding_updated_at = v_now, updated_at = v_now
  WHERE projection.memory_id = v_job.memory_id
    AND projection.user_id = v_job.user_id
    AND projection.projection_generation = v_job.projection_generation
    AND projection.memory_revision = v_job.memory_revision
    AND projection.content_hash = v_job.content_hash
    AND projection.visibility_epoch = v_job.visibility_epoch
    AND projection.scope_type = v_job.scope_type
    AND projection.project_id IS NOT DISTINCT FROM v_job.project_id
    AND projection.scope_conversation_id IS NOT DISTINCT FROM
      v_job.scope_conversation_id
    AND projection.scope_generation = v_job.scope_generation
    AND projection.embedding_profile_id = v_job.embedding_profile_id
    AND projection.embedding_model_id = v_job.embedding_model_id
    AND projection.embedding_dimensions = v_job.embedding_dimensions
    AND projection.embedding_status = 'pending'
    AND EXISTS (
      SELECT 1
      FROM user_memories memory
      JOIN user_memory_state state
        ON state.user_id = memory.user_id
       AND state.visibility_epoch = memory.visibility_epoch
       AND state.active_projection_generation = projection.projection_generation
      JOIN user_memory_settings settings
        ON settings.user_id = memory.user_id
       AND settings.enabled AND settings.search_enabled
       AND (memory.sensitivity = 'normal' OR settings.sensitive_memory_enabled)
      LEFT JOIN projects scoped_project
        ON projection.scope_type = 'project'
       AND scoped_project.id = projection.project_id
       AND scoped_project.user_id = projection.user_id
       AND scoped_project.lifecycle_status = 'active'
       AND scoped_project.scope_generation = projection.scope_generation
      LEFT JOIN conversations scoped_conversation
        ON projection.scope_type = 'conversation'
       AND scoped_conversation.id = projection.scope_conversation_id
       AND scoped_conversation.user_id = projection.user_id
       AND scoped_conversation.deleted_at IS NULL
       AND scoped_conversation.memory_scope_generation = projection.scope_generation
      WHERE memory.id = projection.memory_id
        AND memory.user_id = projection.user_id
        AND memory.revision = projection.memory_revision
        AND memory.content_hash = projection.content_hash
        AND memory.visibility_epoch = projection.visibility_epoch
        AND memory.scope_type = projection.scope_type
        AND memory.scope_generation = projection.scope_generation
        AND memory.deleted_at IS NULL AND memory.enabled
        AND memory.lifecycle_status = 'active'
        AND (memory.valid_from IS NULL OR memory.valid_from <= v_now)
        AND (memory.valid_to IS NULL OR v_now < memory.valid_to)
        AND (memory.expires_at IS NULL OR v_now < memory.expires_at)
        AND (
          projection.scope_type = 'global'
          OR (projection.scope_type = 'project' AND scoped_project.id IS NOT NULL)
          OR (projection.scope_type = 'conversation'
            AND scoped_conversation.id IS NOT NULL)
        )
    );
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_EMBEDDING_SOURCE_DRIFT';
  END IF;

  UPDATE user_memory_embedding_jobs job
  SET status = 'completed', lease_owner = NULL, lease_token = NULL,
      lease_expires_at = NULL, completed_at = v_now, error_code = NULL,
      updated_at = v_now
  WHERE job.job_id = p_job_id;
  RETURN true;
END
$function$;

CREATE FUNCTION memory_worker_retry_embedding_job(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_error_code TEXT,
  p_available_at TIMESTAMPTZ,
  p_terminal BOOLEAN
) RETURNS TEXT
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_job user_memory_embedding_jobs%ROWTYPE;
  v_now TIMESTAMPTZ := clock_timestamp();
  v_terminal BOOLEAN;
BEGIN
  IF p_error_code IS NULL OR p_error_code !~ '^[A-Z][A-Z0-9_]{0,63}$'
    OR p_available_at IS NULL OR p_available_at < v_now
    OR p_terminal IS NULL
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_EMBEDDING_RETRY_INVALID';
  END IF;
  SELECT * INTO v_job FROM user_memory_embedding_jobs job
  WHERE job.job_id = p_job_id
    AND job.status = 'processing'
    AND job.lease_owner = p_worker_id
    AND job.lease_token = p_lease_token
    AND job.lease_expires_at > v_now
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_EMBEDDING_LEASE_LOST';
  END IF;
  v_terminal := p_terminal OR v_job.attempt_count >= v_job.max_attempts;
  UPDATE user_memory_embedding_jobs job
  SET status = CASE WHEN v_terminal THEN 'dead_letter' ELSE 'pending' END,
      provider_record_id = NULL, provider_config_updated_at = NULL,
      lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
      completed_at = CASE WHEN v_terminal THEN v_now ELSE NULL END,
      error_code = CASE WHEN v_terminal THEN p_error_code ELSE NULL END,
      available_at = CASE WHEN v_terminal THEN job.available_at ELSE p_available_at END,
      updated_at = v_now
  WHERE job.job_id = p_job_id;
  IF v_terminal THEN
    UPDATE user_memory_search_projections projection
    SET embedding_status = 'failed', embedding_vector = NULL,
        embedding_error_code = p_error_code,
        embedding_updated_at = v_now, updated_at = v_now
    WHERE projection.memory_id = v_job.memory_id
      AND projection.user_id = v_job.user_id
      AND projection.projection_generation = v_job.projection_generation
      AND projection.memory_revision = v_job.memory_revision
      AND projection.content_hash = v_job.content_hash
      AND projection.visibility_epoch = v_job.visibility_epoch
      AND projection.scope_type = v_job.scope_type
      AND projection.project_id IS NOT DISTINCT FROM v_job.project_id
      AND projection.scope_conversation_id IS NOT DISTINCT FROM
        v_job.scope_conversation_id
      AND projection.scope_generation = v_job.scope_generation
      AND projection.embedding_status = 'pending';
    RETURN 'dead_letter';
  END IF;
  RETURN 'pending';
END
$function$;

CREATE FUNCTION memory_prepare_hybrid_shadow(
  p_observation_id UUID,
  p_user_id UUID,
  p_conversation_id UUID,
  p_assistant_message_id UUID,
  p_query_hash TEXT,
  p_query_text TEXT,
  p_v1_results JSONB,
  p_query_embedding REAL[],
  p_query_embedding_status TEXT
) RETURNS TABLE (
  observation_id UUID,
  profile_id TEXT,
  projection_generation BIGINT,
  status TEXT,
  result_code TEXT,
  replayed BOOLEAN,
  baseline_count INTEGER,
  exact_count INTEGER,
  bm25_count INTEGER,
  vector_count INTEGER,
  rrf_count INTEGER,
  rerank_count INTEGER,
  final_count INTEGER,
  overlap_count INTEGER,
  estimated_tokens INTEGER,
  target_tokens_exceeded BOOLEAN,
  fallback_code TEXT,
  duration_millis INTEGER,
  candidates JSONB
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_started_at TIMESTAMPTZ := clock_timestamp();
  v_existing message_memory_hybrid_shadow_observations%ROWTYPE;
  v_conversation conversations%ROWTYPE;
  v_generation BIGINT;
  v_query_terms TEXT[];
  v_bm25_query TEXT;
  v_baseline_hash TEXT;
  v_baseline_count INTEGER;
  v_exact_count INTEGER := 0;
  v_bm25_count INTEGER := 0;
  v_vector_count INTEGER := 0;
  v_rrf_count INTEGER := 0;
  v_duration INTEGER;
  v_candidates JSONB := '[]'::JSONB;
  v_fallback TEXT := 'NONE';
  v_norm DOUBLE PRECISION;
BEGIN
  IF p_observation_id IS NULL OR p_user_id IS NULL
    OR p_conversation_id IS NULL OR p_assistant_message_id IS NULL
    OR p_query_hash IS NULL OR p_query_hash !~ '^[0-9a-f]{64}$'
    OR p_query_text IS NULL OR octet_length(p_query_text) NOT BETWEEN 1 AND 12000
    OR p_v1_results IS NULL OR jsonb_typeof(p_v1_results) <> 'array'
    OR jsonb_array_length(p_v1_results) > 5
    OR p_query_embedding_status NOT IN (
      'ready', 'failed', 'unavailable', 'cutoff', 'redacted'
    )
    OR (p_query_embedding_status = 'ready'
      AND (p_query_embedding IS NULL OR cardinality(p_query_embedding) <> 1024))
    OR (p_query_embedding_status <> 'ready' AND p_query_embedding IS NOT NULL)
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_HYBRID_SHADOW_ARGUMENT_INVALID';
  END IF;
  IF p_query_hash <> encode(sha256(convert_to(p_query_text, 'UTF8')), 'hex') THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_HYBRID_SHADOW_QUERY_HASH_INVALID';
  END IF;
  IF p_query_embedding_status = 'ready' THEN
    SELECT sqrt(sum(component::DOUBLE PRECISION * component::DOUBLE PRECISION))
    INTO v_norm FROM unnest(p_query_embedding) component;
    IF v_norm IS NULL OR v_norm <= 0 OR v_norm > 1e100 THEN
      RAISE EXCEPTION USING
        ERRCODE = '22023', MESSAGE = 'MEMORY_HYBRID_SHADOW_VECTOR_INVALID';
    END IF;
  ELSE
    v_fallback := CASE p_query_embedding_status
      WHEN 'failed' THEN 'QUERY_EMBEDDING_FAILED'
      WHEN 'unavailable' THEN 'PROVIDER_UNAVAILABLE'
      WHEN 'cutoff' THEN 'HARD_CUTOFF'
      ELSE 'SECRET_REDACTED'
    END;
  END IF;
  IF EXISTS (
    SELECT 1 FROM jsonb_array_elements(p_v1_results) item
    WHERE jsonb_typeof(item) <> 'object'
      OR ARRAY(SELECT key FROM jsonb_object_keys(item) key ORDER BY key)
        <> ARRAY['memoryId', 'revision', 'scopeType']::TEXT[]
      OR COALESCE(item->>'memoryId', '') !~
        '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
      OR COALESCE(item->>'revision', '') !~ '^[1-9][0-9]*$'
      OR item->>'scopeType' <> 'global'
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_HYBRID_SHADOW_BASELINE_INVALID';
  END IF;

  v_baseline_hash := encode(sha256(convert_to(p_v1_results::TEXT, 'UTF8')), 'hex');
  PERFORM pg_advisory_xact_lock(
    hashtext('memory_hybrid_shadow'), hashtext(p_assistant_message_id::TEXT)
  );
  SELECT observation.* INTO v_existing
  FROM message_memory_hybrid_shadow_observations observation
  WHERE observation.assistant_message_id = p_assistant_message_id
  FOR UPDATE;
  IF FOUND THEN
    IF v_existing.user_id <> p_user_id
      OR v_existing.conversation_id <> p_conversation_id
      OR v_existing.query_sha256 <> p_query_hash
      OR v_existing.baseline_sha256 <> v_baseline_hash
      OR v_existing.retrieval_profile_id <> 'memory_hybrid_bge_m3_rrf60_v1'
      OR v_existing.embedding_profile_id <> 'siliconflow_bge_m3_v1'
    THEN
      RAISE EXCEPTION USING
        ERRCODE = '40001', MESSAGE = 'MEMORY_HYBRID_SHADOW_REPLAY_CONFLICT';
    END IF;
    IF v_existing.status <> 'pending' THEN
      RETURN QUERY SELECT
        v_existing.id, v_existing.retrieval_profile_id,
        v_existing.projection_generation, v_existing.status,
        v_existing.result_code, true,
        v_existing.baseline_count::INTEGER, v_existing.exact_count::INTEGER,
        v_existing.bm25_count::INTEGER, v_existing.vector_count::INTEGER,
        v_existing.rrf_count::INTEGER, v_existing.rerank_count::INTEGER,
        v_existing.final_count::INTEGER, v_existing.overlap_count::INTEGER,
        v_existing.estimated_tokens::INTEGER,
        v_existing.target_tokens_exceeded, v_existing.fallback_code,
        v_existing.duration_millis, '[]'::JSONB;
      RETURN;
    END IF;
  END IF;

  SELECT conversation.* INTO v_conversation
  FROM conversations conversation
  WHERE conversation.id = p_conversation_id
    AND conversation.user_id = p_user_id
    AND conversation.deleted_at IS NULL;
  IF NOT FOUND OR NOT EXISTS (
    SELECT 1 FROM messages assistant
    JOIN messages source
      ON source.id = assistant.parent_message_id
     AND source.conversation_id = assistant.conversation_id
     AND source.user_id = assistant.user_id
     AND source.role = 'user' AND source.status = 'completed'
     AND source.deleted_at IS NULL AND source.content = p_query_text
    WHERE assistant.id = p_assistant_message_id
      AND assistant.conversation_id = p_conversation_id
      AND assistant.user_id = p_user_id
      AND assistant.role = 'assistant'
      AND assistant.status IN ('pending', 'streaming')
      AND assistant.deleted_at IS NULL
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_HYBRID_SHADOW_SOURCE_INVALID';
  END IF;
  IF v_conversation.project_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM projects project
    WHERE project.id = v_conversation.project_id
      AND project.user_id = p_user_id
      AND project.lifecycle_status = 'active'
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_HYBRID_SHADOW_SCOPE_INVALID';
  END IF;
  SELECT state.active_projection_generation INTO v_generation
  FROM user_memory_state state
  JOIN user_memory_settings settings ON settings.user_id = state.user_id
  WHERE state.user_id = p_user_id AND settings.enabled AND settings.search_enabled;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_HYBRID_SHADOW_DISABLED';
  END IF;
  IF FOUND AND v_existing.projection_generation <> v_generation THEN
    RAISE EXCEPTION USING
      ERRCODE = '40001', MESSAGE = 'MEMORY_HYBRID_SHADOW_REPLAY_CONFLICT';
  END IF;

  SELECT count(*) INTO v_baseline_count FROM jsonb_array_elements(p_v1_results);
  IF EXISTS (
    SELECT 1
    FROM jsonb_array_elements(p_v1_results) item
    LEFT JOIN user_memories memory
      ON memory.id = (item->>'memoryId')::UUID
     AND memory.user_id = p_user_id
     AND memory.revision = (item->>'revision')::BIGINT
     AND memory.scope_type = 'global'
     AND memory.deleted_at IS NULL AND memory.enabled
     AND memory.lifecycle_status = 'active'
    LEFT JOIN user_memory_state state
      ON state.user_id = memory.user_id
     AND state.visibility_epoch = memory.visibility_epoch
    LEFT JOIN user_memory_settings settings
      ON settings.user_id = memory.user_id
     AND (memory.sensitivity = 'normal' OR settings.sensitive_memory_enabled)
    WHERE memory.id IS NULL OR state.user_id IS NULL OR settings.user_id IS NULL
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_HYBRID_SHADOW_BASELINE_STALE';
  END IF;

  IF v_existing.id IS NULL THEN
    INSERT INTO message_memory_hybrid_shadow_observations (
      id, assistant_message_id, user_id, conversation_id,
      retrieval_profile_id, embedding_profile_id, projection_generation,
      query_sha256, baseline_sha256, status, result_code,
      query_embedding_status, rerank_status, fallback_code, baseline_count
    ) VALUES (
      p_observation_id, p_assistant_message_id, p_user_id, p_conversation_id,
      'memory_hybrid_bge_m3_rrf60_v1', 'siliconflow_bge_m3_v1', v_generation,
      p_query_hash, v_baseline_hash, 'pending', 'PENDING',
      p_query_embedding_status, 'pending', v_fallback, v_baseline_count
    );
    INSERT INTO message_memory_hybrid_shadow_results (
      observation_id, user_id, lane, ordinal, memory_id, memory_revision, scope_type
    )
    SELECT p_observation_id, p_user_id, 'v1', item.ordinal::SMALLINT,
      (item.payload->>'memoryId')::UUID,
      (item.payload->>'revision')::BIGINT, item.payload->>'scopeType'
    FROM jsonb_array_elements(p_v1_results) WITH ORDINALITY item(payload, ordinal)
    ORDER BY item.ordinal;
  ELSE
    p_observation_id := v_existing.id;
    DELETE FROM message_memory_hybrid_shadow_results result
    WHERE result.observation_id = p_observation_id AND result.lane <> 'v1';
  END IF;

  v_query_terms := knowledge_bm25_shadow_query_terms(p_query_text);
  v_bm25_query := knowledge_build_bm25_shadow_text(p_query_text, v_query_terms);

  BEGIN
    WITH authorized AS MATERIALIZED (
      SELECT projection.memory_id, projection.memory_revision,
        projection.scope_type, projection.exact_terms, projection.bm25_text,
        memory.normalized_content, memory.importance, memory.updated_at
      FROM user_memory_search_projections projection
      JOIN user_memories memory
        ON memory.id = projection.memory_id
       AND memory.user_id = projection.user_id
       AND memory.revision = projection.memory_revision
       AND memory.content_hash = projection.content_hash
       AND memory.visibility_epoch = projection.visibility_epoch
       AND memory.scope_type = projection.scope_type
       AND memory.scope_generation = projection.scope_generation
       AND memory.sensitivity = projection.sensitivity
      JOIN user_memory_state state
        ON state.user_id = memory.user_id
       AND state.visibility_epoch = memory.visibility_epoch
       AND state.active_projection_generation = projection.projection_generation
      JOIN user_memory_settings settings
        ON settings.user_id = memory.user_id
       AND settings.enabled AND settings.search_enabled
      LEFT JOIN projects scoped_project
        ON projection.scope_type = 'project'
       AND scoped_project.id = projection.project_id
       AND scoped_project.user_id = projection.user_id
       AND scoped_project.lifecycle_status = 'active'
       AND scoped_project.scope_generation = projection.scope_generation
      WHERE projection.user_id = p_user_id
        AND projection.projection_generation = v_generation
        AND projection.retrieval_profile_id = 'memory_lexical_cjk_bm25_v1'
        AND projection.lexical_status = 'ready'
        AND memory.deleted_at IS NULL AND memory.enabled
        AND memory.lifecycle_status = 'active'
        AND (memory.valid_from IS NULL OR memory.valid_from <= now())
        AND (memory.valid_to IS NULL OR now() < memory.valid_to)
        AND (memory.expires_at IS NULL OR now() < memory.expires_at)
        AND (memory.sensitivity = 'normal' OR settings.sensitive_memory_enabled)
        AND (
          (projection.scope_type = 'global' AND projection.scope_generation = 1)
          OR (projection.scope_type = 'conversation'
            AND projection.scope_conversation_id = p_conversation_id
            AND projection.scope_generation = v_conversation.memory_scope_generation)
          OR (projection.scope_type = 'project'
            AND v_conversation.project_id IS NOT NULL
            AND projection.project_id = v_conversation.project_id
            AND scoped_project.id IS NOT NULL)
        )
    ), exact_unbounded AS MATERIALIZED (
      SELECT candidate.*, row_number() OVER (ORDER BY
        (candidate.normalized_content = lower(p_query_text)) DESC,
        (SELECT count(*) FROM unnest(candidate.exact_terms) term
          WHERE term = ANY(v_query_terms)) DESC,
        candidate.importance DESC, candidate.updated_at DESC,
        candidate.memory_id)::INTEGER AS lane_rank
      FROM authorized candidate
      WHERE cardinality(v_query_terms) > 0
        AND candidate.exact_terms && v_query_terms
    ), exact_ranked AS MATERIALIZED (
      SELECT * FROM exact_unbounded WHERE lane_rank <= 20
    ), bm25_probe AS MATERIALIZED (
      SELECT candidate.*, candidate.bm25_text <@> to_bm25query(
        v_bm25_query, 'idx_user_memory_search_projection_bm25'
      ) AS raw_score
      FROM authorized candidate
      WHERE candidate.bm25_text <@> to_bm25query(
        v_bm25_query, 'idx_user_memory_search_projection_bm25'
      ) < 0
      ORDER BY raw_score, candidate.memory_id LIMIT 30
    ), bm25_ranked AS MATERIALIZED (
      SELECT candidate.*, row_number() OVER (ORDER BY candidate.raw_score,
        candidate.importance DESC, candidate.updated_at DESC,
        candidate.memory_id)::INTEGER AS lane_rank
      FROM bm25_probe candidate
    ), lane_rows AS (
      SELECT 'exact'::TEXT lane, lane_rank, memory_id, memory_revision, scope_type
      FROM exact_ranked
      UNION ALL
      SELECT 'bm25', lane_rank, memory_id, memory_revision, scope_type
      FROM bm25_ranked
    )
    INSERT INTO message_memory_hybrid_shadow_results (
      observation_id, user_id, lane, ordinal, memory_id, memory_revision, scope_type
    )
    SELECT p_observation_id, p_user_id, lane_rows.lane,
      lane_rows.lane_rank::SMALLINT, lane_rows.memory_id,
      lane_rows.memory_revision, lane_rows.scope_type
    FROM lane_rows ORDER BY lane_rows.lane, lane_rows.lane_rank;
  EXCEPTION WHEN OTHERS THEN
    v_duration := least(120000, greatest(0,
      floor(extract(epoch FROM clock_timestamp() - v_started_at) * 1000)::INTEGER));
    UPDATE message_memory_hybrid_shadow_observations observation
    SET status = 'failed', result_code = 'LEXICAL_SEARCH_FAILED',
        rerank_status = 'skipped', fallback_code = 'LEXICAL_SEARCH_FAILED',
        duration_millis = v_duration, updated_at = clock_timestamp()
    WHERE observation.id = p_observation_id
    RETURNING observation.* INTO v_existing;
    RETURN QUERY SELECT v_existing.id, v_existing.retrieval_profile_id,
      v_existing.projection_generation, v_existing.status, v_existing.result_code,
      false, v_existing.baseline_count::INTEGER, v_existing.exact_count::INTEGER,
      v_existing.bm25_count::INTEGER, v_existing.vector_count::INTEGER,
      v_existing.rrf_count::INTEGER, v_existing.rerank_count::INTEGER,
      v_existing.final_count::INTEGER, v_existing.overlap_count::INTEGER,
      v_existing.estimated_tokens::INTEGER, v_existing.target_tokens_exceeded,
      v_existing.fallback_code, v_existing.duration_millis, '[]'::JSONB;
    RETURN;
  END;

  IF p_query_embedding_status = 'ready' THEN
    BEGIN
      WITH vector_probe AS MATERIALIZED (
        SELECT projection.memory_id, projection.memory_revision,
          projection.scope_type,
          projection.embedding_vector <=> p_query_embedding::vector(1024) AS distance,
          memory.importance, memory.updated_at
        FROM user_memory_search_projections projection
        JOIN user_memories memory
          ON memory.id = projection.memory_id
         AND memory.user_id = projection.user_id
         AND memory.revision = projection.memory_revision
         AND memory.content_hash = projection.content_hash
         AND memory.visibility_epoch = projection.visibility_epoch
         AND memory.scope_type = projection.scope_type
         AND memory.scope_generation = projection.scope_generation
         AND memory.sensitivity = projection.sensitivity
        JOIN user_memory_state state
          ON state.user_id = memory.user_id
         AND state.visibility_epoch = memory.visibility_epoch
         AND state.active_projection_generation = projection.projection_generation
        JOIN user_memory_settings settings
          ON settings.user_id = memory.user_id
         AND settings.enabled AND settings.search_enabled
        LEFT JOIN projects scoped_project
          ON projection.scope_type = 'project'
         AND scoped_project.id = projection.project_id
         AND scoped_project.user_id = projection.user_id
         AND scoped_project.lifecycle_status = 'active'
         AND scoped_project.scope_generation = projection.scope_generation
        WHERE projection.user_id = p_user_id
          AND projection.projection_generation = v_generation
          AND projection.embedding_profile_id = 'siliconflow_bge_m3_v1'
          AND projection.embedding_model_id = 'Pro/BAAI/bge-m3'
          AND projection.embedding_dimensions = 1024
          AND projection.embedding_status = 'ready'
          AND projection.embedding_vector IS NOT NULL
          AND memory.deleted_at IS NULL AND memory.enabled
          AND memory.lifecycle_status = 'active'
          AND (memory.valid_from IS NULL OR memory.valid_from <= now())
          AND (memory.valid_to IS NULL OR now() < memory.valid_to)
          AND (memory.expires_at IS NULL OR now() < memory.expires_at)
          AND (memory.sensitivity = 'normal' OR settings.sensitive_memory_enabled)
          AND (
            (projection.scope_type = 'global' AND projection.scope_generation = 1)
            OR (projection.scope_type = 'conversation'
              AND projection.scope_conversation_id = p_conversation_id
              AND projection.scope_generation = v_conversation.memory_scope_generation)
            OR (projection.scope_type = 'project'
              AND v_conversation.project_id IS NOT NULL
              AND projection.project_id = v_conversation.project_id
              AND scoped_project.id IS NOT NULL)
          )
        ORDER BY projection.embedding_vector <=> p_query_embedding::vector(1024),
          projection.memory_id
        LIMIT 30
      ), vector_ranked AS (
        SELECT candidate.*, row_number() OVER (ORDER BY candidate.distance,
          candidate.importance DESC, candidate.updated_at DESC,
          candidate.memory_id)::INTEGER AS lane_rank
        FROM vector_probe candidate
      )
      INSERT INTO message_memory_hybrid_shadow_results (
        observation_id, user_id, lane, ordinal, memory_id, memory_revision, scope_type
      )
      SELECT p_observation_id, p_user_id, 'vector', lane_rank::SMALLINT,
        memory_id, memory_revision, scope_type
      FROM vector_ranked ORDER BY lane_rank;
    EXCEPTION WHEN OTHERS THEN
      v_fallback := 'VECTOR_SEARCH_FAILED';
    END;
  END IF;

  WITH rrf_scores AS MATERIALIZED (
    SELECT result.memory_id, min(result.memory_revision) memory_revision,
      min(result.scope_type) scope_type,
      sum(1.0 / (60.0 + result.ordinal::DOUBLE PRECISION)) rrf_score,
      min(result.ordinal) FILTER (WHERE result.lane = 'exact') exact_rank,
      min(result.ordinal) FILTER (WHERE result.lane = 'bm25') bm25_rank,
      min(result.ordinal) FILTER (WHERE result.lane = 'vector') vector_rank
    FROM message_memory_hybrid_shadow_results result
    WHERE result.observation_id = p_observation_id
      AND result.lane IN ('exact', 'bm25', 'vector')
    GROUP BY result.memory_id
  ), rrf_ranked AS (
    SELECT score.*, row_number() OVER (ORDER BY score.rrf_score DESC,
      (score.exact_rank IS NOT NULL) DESC,
      COALESCE(score.exact_rank, 32767), COALESCE(score.bm25_rank, 32767),
      COALESCE(score.vector_rank, 32767), score.memory_id)::INTEGER lane_rank
    FROM rrf_scores score
  )
  INSERT INTO message_memory_hybrid_shadow_results (
    observation_id, user_id, lane, ordinal, memory_id, memory_revision, scope_type
  )
  SELECT p_observation_id, p_user_id, 'rrf', lane_rank::SMALLINT,
    memory_id, memory_revision, scope_type
  FROM rrf_ranked WHERE lane_rank <= 20 ORDER BY lane_rank;

  SELECT count(*) INTO v_exact_count
  FROM message_memory_hybrid_shadow_results result
  WHERE result.observation_id = p_observation_id AND result.lane = 'exact';
  SELECT count(*) INTO v_bm25_count
  FROM message_memory_hybrid_shadow_results result
  WHERE result.observation_id = p_observation_id AND result.lane = 'bm25';
  SELECT count(*) INTO v_vector_count
  FROM message_memory_hybrid_shadow_results result
  WHERE result.observation_id = p_observation_id AND result.lane = 'vector';
  SELECT count(*) INTO v_rrf_count
  FROM message_memory_hybrid_shadow_results result
  WHERE result.observation_id = p_observation_id AND result.lane = 'rrf';

  SELECT COALESCE(jsonb_agg(jsonb_build_object(
    'memoryId', result.memory_id::TEXT,
    'revision', result.memory_revision,
    'scopeType', result.scope_type,
    'content', memory.content
  ) ORDER BY result.ordinal), '[]'::JSONB)
  INTO v_candidates
  FROM message_memory_hybrid_shadow_results result
  JOIN user_memory_search_projections projection
    ON projection.memory_id = result.memory_id
   AND projection.user_id = result.user_id
   AND projection.memory_revision = result.memory_revision
   AND projection.scope_type = result.scope_type
   AND projection.projection_generation = v_generation
   AND projection.retrieval_profile_id = 'memory_lexical_cjk_bm25_v1'
   AND projection.lexical_status = 'ready'
  JOIN user_memories memory
    ON memory.id = result.memory_id AND memory.user_id = result.user_id
   AND memory.revision = result.memory_revision
   AND memory.content_hash = projection.content_hash
   AND memory.visibility_epoch = projection.visibility_epoch
   AND memory.scope_type = projection.scope_type
   AND memory.scope_generation = projection.scope_generation
   AND memory.sensitivity = projection.sensitivity
  JOIN user_memory_state state
    ON state.user_id = memory.user_id
   AND state.visibility_epoch = memory.visibility_epoch
   AND state.active_projection_generation = projection.projection_generation
  JOIN user_memory_settings settings
    ON settings.user_id = memory.user_id
   AND settings.enabled AND settings.search_enabled
  LEFT JOIN projects scoped_project
    ON projection.scope_type = 'project'
   AND scoped_project.id = projection.project_id
   AND scoped_project.user_id = projection.user_id
   AND scoped_project.lifecycle_status = 'active'
   AND scoped_project.scope_generation = projection.scope_generation
  WHERE result.observation_id = p_observation_id AND result.lane = 'rrf'
    AND memory.deleted_at IS NULL AND memory.enabled
    AND memory.lifecycle_status = 'active'
    AND (memory.valid_from IS NULL OR memory.valid_from <= now())
    AND (memory.valid_to IS NULL OR now() < memory.valid_to)
    AND (memory.expires_at IS NULL OR now() < memory.expires_at)
    AND (memory.sensitivity = 'normal' OR settings.sensitive_memory_enabled)
    AND (
      (projection.scope_type = 'global' AND projection.scope_generation = 1)
      OR (projection.scope_type = 'conversation'
        AND projection.scope_conversation_id = p_conversation_id
        AND projection.scope_generation = v_conversation.memory_scope_generation)
      OR (projection.scope_type = 'project'
        AND v_conversation.project_id IS NOT NULL
        AND projection.project_id = v_conversation.project_id
        AND scoped_project.id IS NOT NULL)
    );

  v_duration := least(120000, greatest(0,
    floor(extract(epoch FROM clock_timestamp() - v_started_at) * 1000)::INTEGER));
  UPDATE message_memory_hybrid_shadow_observations observation
  SET result_code = 'CANDIDATES_READY', fallback_code = v_fallback,
      exact_count = v_exact_count, bm25_count = v_bm25_count,
      vector_count = v_vector_count, rrf_count = v_rrf_count,
      duration_millis = v_duration, updated_at = clock_timestamp()
  WHERE observation.id = p_observation_id
  RETURNING observation.* INTO v_existing;

  RETURN QUERY SELECT v_existing.id, v_existing.retrieval_profile_id,
    v_existing.projection_generation, v_existing.status, v_existing.result_code,
    false, v_existing.baseline_count::INTEGER, v_existing.exact_count::INTEGER,
    v_existing.bm25_count::INTEGER, v_existing.vector_count::INTEGER,
    v_existing.rrf_count::INTEGER, v_existing.rerank_count::INTEGER,
    v_existing.final_count::INTEGER, v_existing.overlap_count::INTEGER,
    v_existing.estimated_tokens::INTEGER, v_existing.target_tokens_exceeded,
    v_existing.fallback_code, v_existing.duration_millis, v_candidates;
END
$function$;

CREATE FUNCTION memory_record_hybrid_shadow(
  p_observation_id UUID,
  p_user_id UUID,
  p_assistant_message_id UUID,
  p_rerank_status TEXT,
  p_fallback_code TEXT,
  p_rerank_results JSONB,
  p_final_results JSONB,
  p_estimated_tokens INTEGER,
  p_target_tokens_exceeded BOOLEAN,
  p_duration_millis INTEGER
) RETURNS TABLE (
  observation_id UUID,
  profile_id TEXT,
  projection_generation BIGINT,
  status TEXT,
  result_code TEXT,
  baseline_count INTEGER,
  exact_count INTEGER,
  bm25_count INTEGER,
  vector_count INTEGER,
  rrf_count INTEGER,
  rerank_count INTEGER,
  final_count INTEGER,
  overlap_count INTEGER,
  estimated_tokens INTEGER,
  target_tokens_exceeded BOOLEAN,
  fallback_code TEXT,
  duration_millis INTEGER
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_existing message_memory_hybrid_shadow_observations%ROWTYPE;
  v_conversation conversations%ROWTYPE;
  v_result_hash TEXT;
  v_rerank_count INTEGER;
  v_final_count INTEGER;
  v_overlap_count INTEGER;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  IF p_observation_id IS NULL OR p_user_id IS NULL OR p_assistant_message_id IS NULL
    OR p_rerank_status NOT IN ('applied', 'fallback', 'skipped')
    OR p_fallback_code IS NULL OR p_fallback_code !~ '^[A-Z][A-Z0-9_]{0,127}$'
    OR p_rerank_results IS NULL OR jsonb_typeof(p_rerank_results) <> 'array'
    OR jsonb_array_length(p_rerank_results) > 20
    OR p_final_results IS NULL OR jsonb_typeof(p_final_results) <> 'array'
    OR jsonb_array_length(p_final_results) > 5
    OR p_estimated_tokens NOT BETWEEN 0 AND 900
    OR p_target_tokens_exceeded IS NULL
    OR p_target_tokens_exceeded <> (p_estimated_tokens > 600)
    OR p_duration_millis NOT BETWEEN 0 AND 120000
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_HYBRID_SHADOW_RESULT_INVALID';
  END IF;
  IF EXISTS (
    SELECT 1 FROM (
      SELECT payload, ordinal FROM jsonb_array_elements(p_rerank_results)
        WITH ORDINALITY item(payload, ordinal)
      UNION ALL
      SELECT payload, ordinal FROM jsonb_array_elements(p_final_results)
        WITH ORDINALITY item(payload, ordinal)
    ) item
    WHERE jsonb_typeof(item.payload) <> 'object'
      OR ARRAY(SELECT key FROM jsonb_object_keys(item.payload) key ORDER BY key)
        <> ARRAY['memoryId', 'revision', 'scopeType']::TEXT[]
      OR COALESCE(item.payload->>'memoryId', '') !~
        '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
      OR COALESCE(item.payload->>'revision', '') !~ '^[1-9][0-9]*$'
      OR item.payload->>'scopeType' NOT IN ('global', 'project', 'conversation')
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_HYBRID_SHADOW_RESULT_INVALID';
  END IF;
  IF (p_rerank_status = 'applied' AND jsonb_array_length(p_rerank_results) = 0)
    OR (p_rerank_status <> 'applied'
      AND jsonb_array_length(p_rerank_results) <> 0)
    OR (SELECT count(DISTINCT item->>'memoryId')
        FROM jsonb_array_elements(p_rerank_results) item)
      <> jsonb_array_length(p_rerank_results)
    OR (SELECT count(DISTINCT item->>'memoryId')
        FROM jsonb_array_elements(p_final_results) item)
      <> jsonb_array_length(p_final_results)
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_HYBRID_SHADOW_RESULT_INVALID';
  END IF;

  v_result_hash := encode(sha256(convert_to(
    p_rerank_status || chr(31) || p_fallback_code || chr(31)
    || p_rerank_results::TEXT || chr(31) || p_final_results::TEXT || chr(31)
    || p_estimated_tokens::TEXT || chr(31) || p_target_tokens_exceeded::TEXT,
    'UTF8'
  )), 'hex');
  PERFORM pg_advisory_xact_lock(
    hashtext('memory_hybrid_shadow'), hashtext(p_assistant_message_id::TEXT)
  );
  SELECT observation.* INTO v_existing
  FROM message_memory_hybrid_shadow_observations observation
  WHERE observation.id = p_observation_id
    AND observation.assistant_message_id = p_assistant_message_id
    AND observation.user_id = p_user_id
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_HYBRID_SHADOW_OBSERVATION_INVALID';
  END IF;
  IF v_existing.status <> 'pending' THEN
    IF v_existing.result_sha256 IS DISTINCT FROM v_result_hash THEN
      RAISE EXCEPTION USING
        ERRCODE = '40001', MESSAGE = 'MEMORY_HYBRID_SHADOW_REPLAY_CONFLICT';
    END IF;
    RETURN QUERY SELECT v_existing.id, v_existing.retrieval_profile_id,
      v_existing.projection_generation, v_existing.status, v_existing.result_code,
      v_existing.baseline_count::INTEGER, v_existing.exact_count::INTEGER,
      v_existing.bm25_count::INTEGER, v_existing.vector_count::INTEGER,
      v_existing.rrf_count::INTEGER, v_existing.rerank_count::INTEGER,
      v_existing.final_count::INTEGER, v_existing.overlap_count::INTEGER,
      v_existing.estimated_tokens::INTEGER, v_existing.target_tokens_exceeded,
      v_existing.fallback_code, v_existing.duration_millis;
    RETURN;
  END IF;

  SELECT conversation.* INTO v_conversation
  FROM conversations conversation
  WHERE conversation.id = v_existing.conversation_id
    AND conversation.user_id = p_user_id
    AND conversation.deleted_at IS NULL;
  IF NOT FOUND OR NOT EXISTS (
    SELECT 1
    FROM messages assistant
    JOIN messages source
      ON source.id = assistant.parent_message_id
     AND source.conversation_id = assistant.conversation_id
     AND source.user_id = assistant.user_id
     AND source.role = 'user' AND source.status = 'completed'
     AND source.deleted_at IS NULL
     AND encode(sha256(convert_to(source.content, 'UTF8')), 'hex') =
       v_existing.query_sha256
    JOIN user_memory_state state
      ON state.user_id = assistant.user_id
     AND state.active_projection_generation = v_existing.projection_generation
    JOIN user_memory_settings settings
      ON settings.user_id = assistant.user_id
     AND settings.enabled AND settings.search_enabled
    WHERE assistant.id = p_assistant_message_id
      AND assistant.conversation_id = v_existing.conversation_id
      AND assistant.user_id = p_user_id
      AND assistant.role = 'assistant'
      AND assistant.status IN ('pending', 'streaming')
      AND assistant.deleted_at IS NULL
  ) OR (
    v_conversation.project_id IS NOT NULL AND NOT EXISTS (
      SELECT 1 FROM projects project
      WHERE project.id = v_conversation.project_id
        AND project.user_id = p_user_id
        AND project.lifecycle_status = 'active'
    )
  ) THEN
    UPDATE message_memory_hybrid_shadow_observations observation
    SET status = 'failed', result_code = 'RESULT_STALE',
        result_sha256 = v_result_hash, rerank_status = 'skipped',
        fallback_code = 'RESULT_STALE', duration_millis = p_duration_millis,
        updated_at = v_now
    WHERE observation.id = p_observation_id
    RETURNING observation.* INTO v_existing;
    RETURN QUERY SELECT v_existing.id, v_existing.retrieval_profile_id,
      v_existing.projection_generation, v_existing.status, v_existing.result_code,
      v_existing.baseline_count::INTEGER, v_existing.exact_count::INTEGER,
      v_existing.bm25_count::INTEGER, v_existing.vector_count::INTEGER,
      v_existing.rrf_count::INTEGER, v_existing.rerank_count::INTEGER,
      v_existing.final_count::INTEGER, v_existing.overlap_count::INTEGER,
      v_existing.estimated_tokens::INTEGER, v_existing.target_tokens_exceeded,
      v_existing.fallback_code, v_existing.duration_millis;
    RETURN;
  END IF;

  IF EXISTS (
    SELECT 1 FROM (
      SELECT payload FROM jsonb_array_elements(p_rerank_results) payload
      UNION ALL SELECT payload FROM jsonb_array_elements(p_final_results) payload
    ) submitted
    WHERE NOT EXISTS (
      SELECT 1
      FROM message_memory_hybrid_shadow_results rrf
      JOIN user_memory_search_projections projection
        ON projection.memory_id = rrf.memory_id
       AND projection.user_id = rrf.user_id
       AND projection.memory_revision = rrf.memory_revision
       AND projection.scope_type = rrf.scope_type
       AND projection.projection_generation = v_existing.projection_generation
       AND projection.retrieval_profile_id = 'memory_lexical_cjk_bm25_v1'
       AND projection.lexical_status = 'ready'
      JOIN user_memories memory
        ON memory.id = projection.memory_id
       AND memory.user_id = projection.user_id
       AND memory.revision = projection.memory_revision
       AND memory.content_hash = projection.content_hash
       AND memory.visibility_epoch = projection.visibility_epoch
       AND memory.scope_type = projection.scope_type
       AND memory.scope_generation = projection.scope_generation
       AND memory.sensitivity = projection.sensitivity
      JOIN user_memory_state state
        ON state.user_id = memory.user_id
       AND state.visibility_epoch = memory.visibility_epoch
       AND state.active_projection_generation = projection.projection_generation
      JOIN user_memory_settings settings
        ON settings.user_id = memory.user_id
       AND settings.enabled AND settings.search_enabled
      LEFT JOIN projects scoped_project
        ON projection.scope_type = 'project'
       AND scoped_project.id = projection.project_id
       AND scoped_project.user_id = projection.user_id
       AND scoped_project.lifecycle_status = 'active'
       AND scoped_project.scope_generation = projection.scope_generation
      WHERE rrf.observation_id = p_observation_id AND rrf.lane = 'rrf'
        AND rrf.memory_id = (submitted.payload->>'memoryId')::UUID
        AND rrf.memory_revision = (submitted.payload->>'revision')::BIGINT
        AND rrf.scope_type = submitted.payload->>'scopeType'
        AND memory.deleted_at IS NULL AND memory.enabled
        AND memory.lifecycle_status = 'active'
        AND (memory.valid_from IS NULL OR memory.valid_from <= v_now)
        AND (memory.valid_to IS NULL OR v_now < memory.valid_to)
        AND (memory.expires_at IS NULL OR v_now < memory.expires_at)
        AND (memory.sensitivity = 'normal' OR settings.sensitive_memory_enabled)
        AND (
          (projection.scope_type = 'global' AND projection.scope_generation = 1)
          OR (projection.scope_type = 'conversation'
            AND projection.scope_conversation_id = v_existing.conversation_id
            AND projection.scope_generation = v_conversation.memory_scope_generation)
          OR (projection.scope_type = 'project'
            AND v_conversation.project_id IS NOT NULL
            AND projection.project_id = v_conversation.project_id
            AND scoped_project.id IS NOT NULL)
        )
    )
  ) OR (
    p_rerank_status = 'applied'
    AND jsonb_array_length(p_rerank_results) <> v_existing.rrf_count
  ) OR EXISTS (
    SELECT 1 FROM jsonb_array_elements(p_final_results) final
    LEFT JOIN jsonb_array_elements(p_rerank_results) reranked
      ON reranked->>'memoryId' = final->>'memoryId'
    WHERE p_rerank_status = 'applied' AND reranked IS NULL
  ) THEN
    UPDATE message_memory_hybrid_shadow_observations observation
    SET status = 'failed', result_code = 'RESULT_STALE',
        result_sha256 = v_result_hash, rerank_status = 'skipped',
        fallback_code = 'RESULT_STALE', duration_millis = p_duration_millis,
        updated_at = clock_timestamp()
    WHERE observation.id = p_observation_id
    RETURNING observation.* INTO v_existing;
  ELSE
    INSERT INTO message_memory_hybrid_shadow_results (
      observation_id, user_id, lane, ordinal, memory_id, memory_revision, scope_type
    )
    SELECT p_observation_id, p_user_id, 'rerank', item.ordinal::SMALLINT,
      (item.payload->>'memoryId')::UUID,
      (item.payload->>'revision')::BIGINT, item.payload->>'scopeType'
    FROM jsonb_array_elements(p_rerank_results) WITH ORDINALITY item(payload, ordinal)
    ORDER BY item.ordinal;
    INSERT INTO message_memory_hybrid_shadow_results (
      observation_id, user_id, lane, ordinal, memory_id, memory_revision, scope_type
    )
    SELECT p_observation_id, p_user_id, 'final', item.ordinal::SMALLINT,
      (item.payload->>'memoryId')::UUID,
      (item.payload->>'revision')::BIGINT, item.payload->>'scopeType'
    FROM jsonb_array_elements(p_final_results) WITH ORDINALITY item(payload, ordinal)
    ORDER BY item.ordinal;

    SELECT count(*) INTO v_rerank_count
    FROM message_memory_hybrid_shadow_results result
    WHERE result.observation_id = p_observation_id AND result.lane = 'rerank';
    SELECT count(*) INTO v_final_count
    FROM message_memory_hybrid_shadow_results result
    WHERE result.observation_id = p_observation_id AND result.lane = 'final';
    SELECT count(*) INTO v_overlap_count
    FROM message_memory_hybrid_shadow_results baseline
    JOIN message_memory_hybrid_shadow_results final
      ON final.observation_id = baseline.observation_id
     AND final.lane = 'final' AND final.memory_id = baseline.memory_id
    WHERE baseline.observation_id = p_observation_id AND baseline.lane = 'v1';

    UPDATE message_memory_hybrid_shadow_observations observation
    SET status = 'completed',
        result_code = CASE WHEN v_final_count = 0 THEN 'NO_CANDIDATES' ELSE 'OK' END,
        result_sha256 = v_result_hash, rerank_status = p_rerank_status,
        fallback_code = p_fallback_code,
        rerank_count = v_rerank_count, final_count = v_final_count,
        overlap_count = v_overlap_count,
        estimated_tokens = p_estimated_tokens,
        target_tokens_exceeded = p_target_tokens_exceeded,
        duration_millis = p_duration_millis, updated_at = clock_timestamp()
    WHERE observation.id = p_observation_id
    RETURNING observation.* INTO v_existing;
  END IF;

  RETURN QUERY SELECT v_existing.id, v_existing.retrieval_profile_id,
    v_existing.projection_generation, v_existing.status, v_existing.result_code,
    v_existing.baseline_count::INTEGER, v_existing.exact_count::INTEGER,
    v_existing.bm25_count::INTEGER, v_existing.vector_count::INTEGER,
    v_existing.rrf_count::INTEGER, v_existing.rerank_count::INTEGER,
    v_existing.final_count::INTEGER, v_existing.overlap_count::INTEGER,
    v_existing.estimated_tokens::INTEGER, v_existing.target_tokens_exceeded,
    v_existing.fallback_code, v_existing.duration_millis;
END
$function$;

ALTER TABLE user_memory_embedding_jobs OWNER TO memory_runtime_owner;
ALTER TABLE message_memory_hybrid_shadow_observations OWNER TO memory_runtime_owner;
ALTER TABLE message_memory_hybrid_shadow_results OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_queue_embedding_projection() OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_invalidate_vector_projection() OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_worker_claim_embedding_job(UUID, UUID, INTEGER)
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_worker_hydrate_embedding_job(UUID, UUID, UUID)
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_worker_complete_embedding_job(UUID, UUID, UUID, REAL[])
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_worker_retry_embedding_job(
  UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN
) OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_prepare_hybrid_shadow(
  UUID, UUID, UUID, UUID, TEXT, TEXT, JSONB, REAL[], TEXT
) OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_record_hybrid_shadow(
  UUID, UUID, UUID, TEXT, TEXT, JSONB, JSONB, INTEGER, BOOLEAN, INTEGER
) OWNER TO memory_runtime_owner;

GRANT SELECT, INSERT, UPDATE, DELETE ON user_memory_embedding_jobs
  TO memory_runtime_owner;
GRANT SELECT, INSERT, UPDATE, DELETE
  ON message_memory_hybrid_shadow_observations,
     message_memory_hybrid_shadow_results
  TO memory_runtime_owner;
GRANT SELECT, INSERT, UPDATE ON user_memory_search_projections
  TO memory_runtime_owner;

REVOKE ALL ON
  user_memory_embedding_jobs,
  message_memory_hybrid_shadow_observations,
  message_memory_hybrid_shadow_results
FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_queue_embedding_projection()
  FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_invalidate_vector_projection()
  FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_worker_claim_embedding_job(UUID, UUID, INTEGER)
  FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_worker_hydrate_embedding_job(UUID, UUID, UUID)
  FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_worker_complete_embedding_job(UUID, UUID, UUID, REAL[])
  FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_worker_retry_embedding_job(
  UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN
) FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_prepare_hybrid_shadow(
  UUID, UUID, UUID, UUID, TEXT, TEXT, JSONB, REAL[], TEXT
) FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_record_hybrid_shadow(
  UUID, UUID, UUID, TEXT, TEXT, JSONB, JSONB, INTEGER, BOOLEAN, INTEGER
) FROM PUBLIC, go_api_runtime, memory_worker_runtime;

GRANT EXECUTE ON FUNCTION memory_worker_claim_embedding_job(UUID, UUID, INTEGER)
  TO memory_worker_runtime;
GRANT EXECUTE ON FUNCTION memory_worker_hydrate_embedding_job(UUID, UUID, UUID)
  TO memory_worker_runtime;
GRANT EXECUTE ON FUNCTION memory_worker_complete_embedding_job(UUID, UUID, UUID, REAL[])
  TO memory_worker_runtime;
GRANT EXECUTE ON FUNCTION memory_worker_retry_embedding_job(
  UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN
) TO memory_worker_runtime;
GRANT EXECUTE ON FUNCTION memory_prepare_hybrid_shadow(
  UUID, UUID, UUID, UUID, TEXT, TEXT, JSONB, REAL[], TEXT
) TO go_api_runtime;
GRANT EXECUTE ON FUNCTION memory_record_hybrid_shadow(
  UUID, UUID, UUID, TEXT, TEXT, JSONB, JSONB, INTEGER, BOOLEAN, INTEGER
) TO go_api_runtime;
