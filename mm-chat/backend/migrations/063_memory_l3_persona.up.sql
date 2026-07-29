-- Memory v2 PR12 L3 Persona shadow generation, derived hybrid retrieval, and
-- evidence-gated promotion. L1 remains the only canonical Memory authority.

DO $memory_l3_persona_prerequisite$
DECLARE
  v_server_version INTEGER := current_setting('server_version_num')::INTEGER;
  v_vector_version TEXT;
BEGIN
  IF v_server_version < 170000 OR v_server_version >= 180000 THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_L3_PERSONA_REQUIRES_POSTGRESQL_17';
  END IF;
  SELECT extversion INTO v_vector_version FROM pg_extension WHERE extname = 'vector';
  IF v_vector_version IS DISTINCT FROM '0.8.5'
    OR to_regprocedure('knowledge_bm25_shadow_query_terms(text)') IS NULL
    OR to_regprocedure('knowledge_build_bm25_shadow_text(text,text[])') IS NULL
    OR to_regprocedure('memory_prepare_hybrid_shadow(uuid,uuid,uuid,uuid,text,text,jsonb,real[],text)') IS NULL
    OR to_regprocedure('memory_governance_classify_sensitivity(text)') IS NULL
    OR to_regprocedure('memory_governance_l2_scene_snapshot(uuid)') IS NULL
  THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_L3_PERSONA_REQUIRES_PR8_PR9_AND_PR11';
  END IF;
END
$memory_l3_persona_prerequisite$;

SELECT set_config(
  'search_path', quote_ident(current_schema()) || ', pg_catalog, pg_temp', false
);

CREATE TABLE memory_l3_persona_profiles (
  profile_id TEXT PRIMARY KEY,
  synthesis_profile_id TEXT NOT NULL UNIQUE,
  retrieval_profile_id TEXT NOT NULL UNIQUE,
  lifecycle_status TEXT NOT NULL DEFAULT 'shadow' CHECK (
    lifecycle_status IN ('shadow', 'active', 'rolled_back')
  ),
  benchmark_report_sha256 TEXT CHECK (
    benchmark_report_sha256 IS NULL OR benchmark_report_sha256 ~ '^[0-9a-f]{64}$'
  ),
  canary_report_sha256 TEXT CHECK (
    canary_report_sha256 IS NULL OR canary_report_sha256 ~ '^[0-9a-f]{64}$'
  ),
  activated_at TIMESTAMPTZ,
  rolled_back_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT memory_l3_persona_profile_fixed CHECK (
    profile_id = 'memory_l3_persona_v1'
    AND synthesis_profile_id = 'memory_l3_persona_synthesis_v1'
    AND retrieval_profile_id = 'memory_l3_persona_hybrid_bge_m3_rrf60_v1'
  ),
  CONSTRAINT memory_l3_persona_profile_state_shape CHECK (
    (lifecycle_status = 'shadow' AND activated_at IS NULL AND rolled_back_at IS NULL)
    OR (lifecycle_status = 'active' AND activated_at IS NOT NULL AND rolled_back_at IS NULL
      AND benchmark_report_sha256 IS NOT NULL AND canary_report_sha256 IS NOT NULL)
    OR (lifecycle_status = 'rolled_back' AND activated_at IS NOT NULL
      AND rolled_back_at IS NOT NULL AND benchmark_report_sha256 IS NOT NULL
      AND canary_report_sha256 IS NOT NULL)
  ),
  CONSTRAINT memory_l3_persona_profile_timestamps CHECK (updated_at >= created_at)
);

INSERT INTO memory_l3_persona_profiles(
  profile_id, synthesis_profile_id, retrieval_profile_id
) VALUES (
  'memory_l3_persona_v1',
  'memory_l3_persona_synthesis_v1',
  'memory_l3_persona_hybrid_bge_m3_rrf60_v1'
);

CREATE TABLE user_memory_persona_versions (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  content TEXT NOT NULL,
  content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  token_count SMALLINT NOT NULL CHECK (token_count BETWEEN 1 AND 300),
  sensitivity TEXT NOT NULL CHECK (sensitivity IN ('normal', 'sensitive')),
  sensitive_input_included BOOLEAN NOT NULL,
  lifecycle_status TEXT NOT NULL DEFAULT 'shadow' CHECK (
    lifecycle_status IN ('shadow', 'active', 'disabled', 'stale')
  ),
  user_disabled BOOLEAN NOT NULL DEFAULT false,
  profile_id TEXT NOT NULL REFERENCES memory_l3_persona_profiles(profile_id),
  generation BIGINT NOT NULL CHECK (generation >= 1),
  visibility_epoch BIGINT NOT NULL CHECK (visibility_epoch >= 1),
  source_watermark TEXT NOT NULL CHECK (source_watermark ~ '^[0-9a-f]{64}$'),
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision >= 1),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  activated_at TIMESTAMPTZ,
  disabled_at TIMESTAMPTZ,
  stale_at TIMESTAMPTZ,
  purge_after TIMESTAMPTZ,
  purged_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ,
  CONSTRAINT user_memory_persona_versions_id_user_unique UNIQUE (id, user_id),
  CONSTRAINT user_memory_persona_versions_generation_unique UNIQUE (user_id, generation),
  CONSTRAINT user_memory_persona_versions_content_shape CHECK (
    (deleted_at IS NULL AND length(trim(content)) > 0 AND char_length(content) <= 4000)
    OR (deleted_at IS NOT NULL AND content = '')
  ),
  CONSTRAINT user_memory_persona_versions_lifecycle_shape CHECK (
    (lifecycle_status = 'shadow' AND NOT user_disabled AND stale_at IS NULL
      AND purge_after IS NULL AND deleted_at IS NULL)
    OR (lifecycle_status = 'active' AND NOT user_disabled AND activated_at IS NOT NULL
      AND stale_at IS NULL AND purge_after IS NULL AND deleted_at IS NULL)
    OR (lifecycle_status = 'disabled' AND user_disabled AND disabled_at IS NOT NULL
      AND stale_at IS NULL AND purge_after IS NULL AND deleted_at IS NULL)
    OR (lifecycle_status = 'stale' AND stale_at IS NOT NULL AND purge_after IS NOT NULL)
  ),
  CONSTRAINT user_memory_persona_versions_timestamps CHECK (
    updated_at >= created_at
    AND (purge_after IS NULL OR purge_after >= stale_at)
    AND (purged_at IS NULL OR purged_at >= stale_at)
    AND (deleted_at IS NULL OR deleted_at >= created_at)
  )
);

CREATE INDEX idx_user_memory_persona_versions_user_status
  ON user_memory_persona_versions(user_id, generation, lifecycle_status,
    updated_at DESC, id);
CREATE INDEX idx_user_memory_persona_versions_purge
  ON user_memory_persona_versions(purge_after, id)
  WHERE lifecycle_status = 'stale' AND purged_at IS NULL;

CREATE TABLE user_memory_persona_members (
  persona_id UUID NOT NULL,
  memory_id UUID NOT NULL,
  user_id UUID NOT NULL,
  memory_revision BIGINT NOT NULL CHECK (memory_revision >= 1),
  memory_content_hash TEXT NOT NULL CHECK (memory_content_hash ~ '^[0-9a-f]{64}$'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (persona_id, memory_id),
  CONSTRAINT user_memory_persona_members_persona_owner_fk
    FOREIGN KEY (persona_id, user_id) REFERENCES user_memory_persona_versions(id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT user_memory_persona_members_memory_owner_fk
    FOREIGN KEY (memory_id, user_id) REFERENCES user_memories(id, user_id)
    ON DELETE CASCADE
);
CREATE INDEX idx_user_memory_persona_members_memory
  ON user_memory_persona_members(user_id, memory_id, memory_revision, persona_id);

CREATE TABLE user_memory_persona_search_projections (
  entity_type TEXT NOT NULL CHECK (entity_type = 'l3_persona'),
  entity_id UUID NOT NULL,
  user_id UUID NOT NULL,
  entity_revision BIGINT NOT NULL CHECK (entity_revision >= 1),
  sensitivity TEXT NOT NULL CHECK (sensitivity IN ('normal', 'sensitive')),
  visibility_epoch BIGINT NOT NULL CHECK (visibility_epoch >= 1),
  generation BIGINT NOT NULL CHECK (generation >= 1),
  retrieval_profile_id TEXT NOT NULL CHECK (
    retrieval_profile_id = 'memory_l3_persona_hybrid_bge_m3_rrf60_v1'
  ),
  content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  source_watermark TEXT NOT NULL CHECK (source_watermark ~ '^[0-9a-f]{64}$'),
  exact_terms TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[] CHECK (
    array_position(exact_terms, NULL) IS NULL AND cardinality(exact_terms) <= 64
    AND exact_terms = knowledge_normalize_bm25_shadow_terms(exact_terms)
  ),
  bm25_text TEXT NOT NULL CHECK (octet_length(bm25_text) BETWEEN 1 AND 262144),
  lexical_status TEXT NOT NULL DEFAULT 'ready' CHECK (lexical_status IN ('ready', 'failed')),
  embedding_profile_id TEXT NOT NULL DEFAULT 'siliconflow_bge_m3_v1' CHECK (
    embedding_profile_id = 'siliconflow_bge_m3_v1'
  ),
  embedding_model_id TEXT NOT NULL DEFAULT 'Pro/BAAI/bge-m3' CHECK (
    embedding_model_id = 'Pro/BAAI/bge-m3'
  ),
  embedding_dimensions SMALLINT NOT NULL DEFAULT 1024 CHECK (embedding_dimensions = 1024),
  embedding_status TEXT NOT NULL DEFAULT 'pending' CHECK (
    embedding_status IN ('pending', 'ready', 'failed')
  ),
  embedding_vector vector(1024),
  embedding_error_code TEXT CHECK (
    embedding_error_code IS NULL OR embedding_error_code ~ '^[A-Z][A-Z0-9_]{0,63}$'
  ),
  embedding_updated_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (entity_type, entity_id),
  CONSTRAINT user_memory_persona_search_projection_entity_owner_fk
    FOREIGN KEY (entity_id, user_id) REFERENCES user_memory_persona_versions(id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT user_memory_persona_search_projection_embedding_shape CHECK (
    (embedding_status = 'pending' AND embedding_vector IS NULL
      AND embedding_error_code IS NULL)
    OR (embedding_status = 'ready' AND embedding_vector IS NOT NULL
      AND embedding_error_code IS NULL AND embedding_updated_at IS NOT NULL)
    OR (embedding_status = 'failed' AND embedding_vector IS NULL
      AND embedding_error_code IS NOT NULL AND embedding_updated_at IS NOT NULL)
  )
);
CREATE INDEX idx_user_memory_persona_search_authority
  ON user_memory_persona_search_projections(user_id, generation,
    retrieval_profile_id, visibility_epoch, lexical_status, entity_id);
CREATE INDEX idx_user_memory_persona_search_exact
  ON user_memory_persona_search_projections USING gin(exact_terms);
CREATE INDEX idx_user_memory_persona_search_bm25
  ON user_memory_persona_search_projections
  USING bm25(bm25_text) WITH (text_config = 'simple');
CREATE INDEX idx_user_memory_persona_search_vector
  ON user_memory_persona_search_projections
  USING hnsw (embedding_vector vector_cosine_ops)
  WITH (m = 16, ef_construction = 64)
  WHERE embedding_status = 'ready';

CREATE TABLE user_memory_persona_jobs (
  job_id UUID PRIMARY KEY,
  dedupe_key TEXT NOT NULL UNIQUE CHECK (octet_length(dedupe_key) BETWEEN 1 AND 256),
  stage TEXT NOT NULL CHECK (stage IN ('refresh', 'purge')),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  target_persona_id UUID,
  visibility_epoch BIGINT NOT NULL CHECK (visibility_epoch >= 1),
  generation BIGINT NOT NULL CHECK (generation >= 1),
  profile_id TEXT NOT NULL REFERENCES memory_l3_persona_profiles(profile_id),
  source_watermark TEXT CHECK (source_watermark IS NULL OR source_watermark ~ '^[0-9a-f]{64}$'),
  input_memory_ids UUID[] NOT NULL DEFAULT ARRAY[]::UUID[] CHECK (
    cardinality(input_memory_ids) <= 80
    AND array_position(input_memory_ids, NULL) IS NULL
  ),
  provider_record_id UUID,
  provider_config_updated_at TIMESTAMPTZ,
  model_id TEXT,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (
    status IN ('pending', 'processing', 'completed', 'dead_letter')
  ),
  attempt_count INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 8 CHECK (max_attempts BETWEEN 1 AND 128),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  lease_owner UUID,
  lease_token UUID,
  lease_expires_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  error_code TEXT CHECK (error_code IS NULL OR error_code ~ '^[A-Z][A-Z0-9_]{0,63}$'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT user_memory_persona_jobs_target_owner_fk
    FOREIGN KEY (target_persona_id, user_id) REFERENCES user_memory_persona_versions(id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT user_memory_persona_jobs_stage_shape CHECK (
    (stage = 'refresh' AND target_persona_id IS NULL
      AND source_watermark IS NOT NULL)
    OR (stage = 'purge' AND target_persona_id IS NOT NULL
      AND source_watermark IS NULL AND cardinality(input_memory_ids) = 0
      AND provider_record_id IS NULL
      AND provider_config_updated_at IS NULL AND model_id IS NULL)
  ),
  CONSTRAINT user_memory_persona_jobs_provider_shape CHECK (
    (provider_record_id IS NULL AND provider_config_updated_at IS NULL AND model_id IS NULL)
    OR (provider_record_id IS NOT NULL AND provider_config_updated_at IS NOT NULL
      AND length(trim(model_id)) BETWEEN 1 AND 256)
  ),
  CONSTRAINT user_memory_persona_jobs_state_shape CHECK (
    (status = 'pending' AND lease_owner IS NULL AND lease_token IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NULL AND error_code IS NULL)
    OR (status = 'processing' AND lease_owner IS NOT NULL AND lease_token IS NOT NULL
      AND lease_expires_at IS NOT NULL AND completed_at IS NULL AND error_code IS NULL)
    OR (status = 'completed' AND lease_owner IS NULL AND lease_token IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NOT NULL AND error_code IS NULL)
    OR (status = 'dead_letter' AND lease_owner IS NULL AND lease_token IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NOT NULL AND error_code IS NOT NULL)
  )
);
CREATE INDEX idx_user_memory_persona_jobs_claim
  ON user_memory_persona_jobs(available_at, created_at, job_id)
  WHERE status = 'pending';
CREATE INDEX idx_user_memory_persona_jobs_expired
  ON user_memory_persona_jobs(lease_expires_at, job_id)
  WHERE status = 'processing';

CREATE TABLE user_memory_persona_embedding_jobs (
  job_id UUID PRIMARY KEY,
  entity_type TEXT NOT NULL CHECK (entity_type = 'l3_persona'),
  entity_id UUID NOT NULL,
  user_id UUID NOT NULL,
  entity_revision BIGINT NOT NULL CHECK (entity_revision >= 1),
  content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  source_watermark TEXT NOT NULL CHECK (source_watermark ~ '^[0-9a-f]{64}$'),
  visibility_epoch BIGINT NOT NULL CHECK (visibility_epoch >= 1),
  generation BIGINT NOT NULL CHECK (generation >= 1),
  embedding_profile_id TEXT NOT NULL CHECK (embedding_profile_id = 'siliconflow_bge_m3_v1'),
  embedding_model_id TEXT NOT NULL CHECK (embedding_model_id = 'Pro/BAAI/bge-m3'),
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
  error_code TEXT CHECK (error_code IS NULL OR error_code ~ '^[A-Z][A-Z0-9_]{0,63}$'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (entity_type, entity_id),
  CONSTRAINT user_memory_persona_embedding_projection_owner_fk
    FOREIGN KEY (entity_type, entity_id)
    REFERENCES user_memory_persona_search_projections(entity_type, entity_id)
    ON DELETE CASCADE,
  CONSTRAINT user_memory_persona_embedding_state_shape CHECK (
    (status = 'pending' AND lease_owner IS NULL AND lease_token IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NULL AND error_code IS NULL)
    OR (status = 'processing' AND lease_owner IS NOT NULL AND lease_token IS NOT NULL
      AND lease_expires_at IS NOT NULL AND completed_at IS NULL AND error_code IS NULL)
    OR (status = 'completed' AND lease_owner IS NULL AND lease_token IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NOT NULL AND error_code IS NULL)
    OR (status = 'dead_letter' AND lease_owner IS NULL AND lease_token IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NOT NULL AND error_code IS NOT NULL)
  )
);
CREATE INDEX idx_user_memory_persona_embedding_claim
  ON user_memory_persona_embedding_jobs(available_at, created_at, job_id)
  WHERE status = 'pending';
CREATE INDEX idx_user_memory_persona_embedding_expired
  ON user_memory_persona_embedding_jobs(lease_expires_at, job_id)
  WHERE status = 'processing';

CREATE TABLE message_memory_l3_persona_observations (
  id UUID PRIMARY KEY,
  assistant_message_id UUID NOT NULL UNIQUE,
  user_id UUID NOT NULL,
  conversation_id UUID NOT NULL,
  mode TEXT NOT NULL CHECK (mode IN ('shadow', 'active')),
  profile_id TEXT NOT NULL CHECK (profile_id = 'memory_l3_persona_v1'),
  retrieval_profile_id TEXT NOT NULL CHECK (
    retrieval_profile_id = 'memory_l3_persona_hybrid_bge_m3_rrf60_v1'
  ),
  generation BIGINT NOT NULL CHECK (generation >= 1),
  query_sha256 TEXT NOT NULL CHECK (query_sha256 ~ '^[0-9a-f]{64}$'),
  result_sha256 TEXT CHECK (result_sha256 IS NULL OR result_sha256 ~ '^[0-9a-f]{64}$'),
  status TEXT NOT NULL CHECK (status IN ('pending', 'completed', 'failed')),
  result_code TEXT NOT NULL CHECK (result_code ~ '^[A-Z][A-Z0-9_]{0,127}$'),
  query_embedding_status TEXT NOT NULL CHECK (
    query_embedding_status IN ('ready', 'failed', 'unavailable', 'cutoff', 'redacted')
  ),
  rerank_status TEXT NOT NULL DEFAULT 'pending' CHECK (
    rerank_status IN ('pending', 'applied', 'fallback', 'skipped')
  ),
  fallback_code TEXT NOT NULL DEFAULT 'NONE' CHECK (
    fallback_code ~ '^[A-Z][A-Z0-9_]{0,127}$'
  ),
  exact_count SMALLINT NOT NULL DEFAULT 0 CHECK (exact_count BETWEEN 0 AND 5),
  bm25_count SMALLINT NOT NULL DEFAULT 0 CHECK (bm25_count BETWEEN 0 AND 5),
  vector_count SMALLINT NOT NULL DEFAULT 0 CHECK (vector_count BETWEEN 0 AND 5),
  rrf_count SMALLINT NOT NULL DEFAULT 0 CHECK (rrf_count BETWEEN 0 AND 5),
  rerank_count SMALLINT NOT NULL DEFAULT 0 CHECK (rerank_count BETWEEN 0 AND 5),
  final_count SMALLINT NOT NULL DEFAULT 0 CHECK (final_count BETWEEN 0 AND 1),
  injected_count SMALLINT NOT NULL DEFAULT 0 CHECK (injected_count BETWEEN 0 AND 1),
  estimated_tokens SMALLINT NOT NULL DEFAULT 0 CHECK (estimated_tokens BETWEEN 0 AND 300),
  duration_millis INTEGER NOT NULL DEFAULT 0 CHECK (duration_millis BETWEEN 0 AND 120000),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT message_memory_l3_persona_observation_assistant_owner_fk
    FOREIGN KEY (assistant_message_id, user_id) REFERENCES messages(id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT message_memory_l3_persona_observation_conversation_owner_fk
    FOREIGN KEY (conversation_id, user_id) REFERENCES conversations(id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT message_memory_l3_persona_observation_result_shape CHECK (
    (status = 'pending' AND result_sha256 IS NULL AND injected_count = 0)
    OR status IN ('completed', 'failed')
  ),
  CONSTRAINT message_memory_l3_persona_observation_mode_shape CHECK (
    mode = 'active' OR injected_count = 0
  ),
  UNIQUE (id, user_id)
);
CREATE INDEX idx_message_memory_l3_persona_observation_canary
  ON message_memory_l3_persona_observations(mode, status, created_at, id);

CREATE TABLE message_memory_l3_persona_results (
  observation_id UUID NOT NULL,
  user_id UUID NOT NULL,
  lane TEXT NOT NULL CHECK (
    lane IN ('exact', 'bm25', 'vector', 'rrf', 'rerank', 'final')
  ),
  ordinal SMALLINT NOT NULL CHECK (
    ordinal >= 1 AND (
      (lane = 'final' AND ordinal <= 1)
      OR (lane IN ('rrf', 'rerank') AND ordinal <= 5)
      OR (lane IN ('exact', 'bm25', 'vector') AND ordinal <= 5)
    )
  ),
  persona_id UUID NOT NULL,
  persona_revision BIGINT NOT NULL CHECK (persona_revision >= 1),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (observation_id, lane, ordinal),
  UNIQUE (observation_id, lane, persona_id),
  CONSTRAINT message_memory_l3_persona_result_observation_owner_fk
    FOREIGN KEY (observation_id, user_id)
    REFERENCES message_memory_l3_persona_observations(id, user_id) ON DELETE CASCADE,
  CONSTRAINT message_memory_l3_persona_result_persona_owner_fk
    FOREIGN KEY (persona_id, user_id)
    REFERENCES user_memory_persona_versions(id, user_id) ON DELETE CASCADE
);

CREATE TABLE memory_l3_persona_promotion_events (
  event_id UUID PRIMARY KEY,
  action TEXT NOT NULL CHECK (action IN ('promote', 'rollback')),
  profile_id TEXT NOT NULL REFERENCES memory_l3_persona_profiles(profile_id),
  benchmark_report_sha256 TEXT CHECK (
    benchmark_report_sha256 IS NULL OR benchmark_report_sha256 ~ '^[0-9a-f]{64}$'
  ),
  canary_report_sha256 TEXT CHECK (
    canary_report_sha256 IS NULL OR canary_report_sha256 ~ '^[0-9a-f]{64}$'
  ),
  benchmark_case_count INTEGER CHECK (
    benchmark_case_count IS NULL OR benchmark_case_count = 500
  ),
  canary_eligible_turns INTEGER CHECK (
    canary_eligible_turns IS NULL OR canary_eligible_turns >= 100
  ),
  canary_window_started_at TIMESTAMPTZ,
  canary_window_ended_at TIMESTAMPTZ,
  persona_consistency NUMERIC CHECK (
    persona_consistency IS NULL OR persona_consistency BETWEEN 0 AND 1
  ),
  false_injection_rate NUMERIC CHECK (
    false_injection_rate IS NULL OR false_injection_rate BETWEEN 0 AND 1
  ),
  token_saving_ratio NUMERIC CHECK (
    token_saving_ratio IS NULL OR token_saving_ratio BETWEEN 0 AND 1
  ),
  reason_code TEXT CHECK (
    reason_code IS NULL OR reason_code ~ '^[A-Z][A-Z0-9_]{0,63}$'
  ),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT memory_l3_persona_promotion_event_shape CHECK (
    (action = 'promote' AND benchmark_report_sha256 IS NOT NULL
      AND canary_report_sha256 IS NOT NULL AND benchmark_case_count = 500
      AND canary_eligible_turns >= 100 AND canary_window_started_at IS NOT NULL
      AND canary_window_ended_at >= canary_window_started_at + interval '7 days'
      AND persona_consistency >= 0.95 AND false_injection_rate <= 0.02
      AND token_saving_ratio >= 0.20
      AND reason_code IS NULL)
    OR (action = 'rollback' AND reason_code IS NOT NULL)
  )
);

CREATE FUNCTION memory_l3_persona_estimated_tokens(p_value TEXT)
RETURNS INTEGER
LANGUAGE plpgsql
IMMUTABLE
PARALLEL SAFE
AS $function$
DECLARE
  v_index INTEGER;
  v_ascii INTEGER := 0;
  v_non_ascii INTEGER := 0;
  v_character TEXT;
BEGIN
  IF p_value IS NULL OR p_value = '' THEN
    RETURN 0;
  END IF;
  FOR v_index IN 1..char_length(p_value) LOOP
    v_character := substr(p_value, v_index, 1);
    IF ascii(v_character) <= 127 THEN
      v_ascii := v_ascii + 1;
    ELSE
      v_non_ascii := v_non_ascii + 1;
    END IF;
  END LOOP;
  RETURN ((v_ascii + 3) / 4) + (v_non_ascii * 2) + 24;
END
$function$;

CREATE FUNCTION memory_l3_persona_source_watermark(p_user_id UUID)
RETURNS TEXT
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_epoch BIGINT;
  v_sensitive BOOLEAN;
  v_members TEXT;
BEGIN
  IF p_user_id IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L3_PERSONA_USER_INVALID';
  END IF;
  SELECT state.visibility_epoch, settings.sensitive_memory_enabled
  INTO v_epoch, v_sensitive
  FROM user_memory_state state
  JOIN user_memory_settings settings ON settings.user_id = state.user_id
  WHERE state.user_id = p_user_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_L3_PERSONA_STATE_MISSING';
  END IF;
  SELECT COALESCE(string_agg(
    memory.id::TEXT || ':' || memory.revision::TEXT || ':'
      || memory.content_hash || ':' || memory.memory_type || ':'
      || memory.sensitivity,
    ',' ORDER BY memory.id
  ), '') INTO v_members
  FROM user_memories memory
  WHERE memory.user_id = p_user_id
    AND memory.scope_type = 'global'
    AND memory.project_id IS NULL
    AND memory.scope_conversation_id IS NULL
    AND memory.scope_generation = 1
    AND memory.memory_type IN ('fact', 'preference', 'instruction', 'warning', 'decision')
    AND memory.visibility_epoch = v_epoch
    AND memory.deleted_at IS NULL AND memory.enabled
    AND memory.lifecycle_status = 'active'
    AND (memory.valid_from IS NULL OR memory.valid_from <= statement_timestamp())
    AND (memory.valid_to IS NULL OR statement_timestamp() < memory.valid_to)
    AND (memory.expires_at IS NULL OR statement_timestamp() < memory.expires_at)
    AND memory_governance_classify_sensitivity(memory.content) <> 'secret'
    AND (memory.sensitivity = 'normal' OR v_sensitive);
  RETURN encode(sha256(convert_to(
    p_user_id::TEXT || chr(31) || 'global' || chr(31) || v_epoch::TEXT
      || chr(31) || v_sensitive::TEXT || chr(31) || v_members,
    'UTF8'
  )), 'hex');
END
$function$;

CREATE FUNCTION memory_l3_persona_reconcile_user(p_user_id UUID)
RETURNS VOID
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_active BOOLEAN;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  SELECT profile.lifecycle_status = 'active'
    AND state.active_retrieval_profile_id = 'memory_hybrid_bge_m3_rrf60_v1'
    AND settings.enabled AND settings.search_enabled AND settings.l3_mode <> 'off'
  INTO v_active
  FROM user_memory_state state
  JOIN user_memory_settings settings ON settings.user_id = state.user_id
  CROSS JOIN memory_l3_persona_profiles profile
  WHERE state.user_id = p_user_id AND profile.profile_id = 'memory_l3_persona_v1';
  v_active := COALESCE(v_active, false);
  UPDATE user_memory_persona_versions persona
  SET lifecycle_status = CASE
        WHEN persona.user_disabled THEN 'disabled'
        WHEN v_active THEN 'active'
        ELSE 'shadow'
      END,
      activated_at = CASE
        WHEN NOT persona.user_disabled AND v_active
          THEN COALESCE(persona.activated_at, v_now)
        ELSE persona.activated_at
      END,
      disabled_at = CASE
        WHEN persona.user_disabled THEN COALESCE(persona.disabled_at, v_now)
        ELSE NULL
      END,
      updated_at = v_now
  WHERE persona.user_id = p_user_id
    AND persona.lifecycle_status <> 'stale'
    AND persona.deleted_at IS NULL
    AND persona.generation = (
      SELECT active_l3_generation FROM user_memory_state WHERE user_id = p_user_id
    );
END
$function$;

CREATE FUNCTION memory_l3_persona_advance_generation(p_user_id UUID)
RETURNS BIGINT
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_generation BIGINT;
BEGIN
  INSERT INTO user_memory_state(user_id) VALUES (p_user_id)
  ON CONFLICT (user_id) DO NOTHING;
  UPDATE user_memory_state state
  SET active_l3_generation = state.active_l3_generation + 1,
      updated_at = clock_timestamp()
  WHERE state.user_id = p_user_id
  RETURNING state.active_l3_generation INTO v_generation;
  IF v_generation IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_L3_PERSONA_STATE_MISSING';
  END IF;
  RETURN v_generation;
END
$function$;

CREATE FUNCTION memory_l3_persona_enqueue_user(p_user_id UUID)
RETURNS UUID
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_state user_memory_state%ROWTYPE;
  v_watermark TEXT;
  v_job_id UUID := gen_random_uuid();
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  SELECT * INTO v_state FROM user_memory_state
  WHERE user_id = p_user_id FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_L3_PERSONA_STATE_MISSING';
  END IF;
  v_watermark := memory_l3_persona_source_watermark(p_user_id);
  INSERT INTO user_memory_persona_jobs(
    job_id, dedupe_key, stage, user_id, visibility_epoch, generation,
    profile_id, source_watermark, status, attempt_count, max_attempts,
    available_at, created_at, updated_at
  ) VALUES (
    v_job_id, 'memory:l3:refresh:v1:' || p_user_id::TEXT, 'refresh', p_user_id,
    v_state.visibility_epoch, v_state.active_l3_generation,
    'memory_l3_persona_v1', v_watermark, 'pending', 0, 8, v_now, v_now, v_now
  )
  ON CONFLICT (dedupe_key) DO UPDATE SET
    job_id = EXCLUDED.job_id,
    visibility_epoch = EXCLUDED.visibility_epoch,
    generation = EXCLUDED.generation,
    profile_id = EXCLUDED.profile_id,
    source_watermark = EXCLUDED.source_watermark,
    input_memory_ids = ARRAY[]::UUID[],
    provider_record_id = NULL,
    provider_config_updated_at = NULL,
    model_id = NULL,
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
  RETURN v_job_id;
END
$function$;

CREATE FUNCTION memory_l3_persona_invalidate_user_at_generation(
  p_user_id UUID,
  p_generation BIGINT
) RETURNS VOID
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_persona RECORD;
  v_state user_memory_state%ROWTYPE;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  SELECT * INTO v_state FROM user_memory_state
  WHERE user_id = p_user_id AND active_l3_generation = p_generation;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '40001',
      MESSAGE = 'MEMORY_L3_PERSONA_GENERATION_STALE';
  END IF;
  FOR v_persona IN
    UPDATE user_memory_persona_versions persona
    SET lifecycle_status = 'stale',
        stale_at = COALESCE(persona.stale_at, v_now),
        purge_after = LEAST(
          COALESCE(persona.purge_after, v_now + interval '24 hours'),
          v_now + interval '24 hours'
        ),
        updated_at = v_now
    WHERE persona.user_id = p_user_id
      AND persona.lifecycle_status <> 'stale'
      AND persona.deleted_at IS NULL
    RETURNING persona.id, persona.generation
  LOOP
    DELETE FROM user_memory_persona_search_projections projection
    WHERE projection.entity_type = 'l3_persona'
      AND projection.entity_id = v_persona.id;
    INSERT INTO user_memory_persona_jobs(
      job_id, dedupe_key, stage, user_id, target_persona_id,
      visibility_epoch, generation, profile_id, status, attempt_count,
      max_attempts, available_at, created_at, updated_at
    ) VALUES (
      gen_random_uuid(), 'memory:l3:purge:v1:' || v_persona.id::TEXT,
      'purge', p_user_id, v_persona.id, v_state.visibility_epoch,
      v_persona.generation, 'memory_l3_persona_v1', 'pending', 0, 128,
      v_now + interval '24 hours', v_now, v_now
    )
    ON CONFLICT (dedupe_key) DO UPDATE SET
      job_id = EXCLUDED.job_id,
      status = 'pending', attempt_count = 0, max_attempts = 128,
      available_at = LEAST(user_memory_persona_jobs.available_at, EXCLUDED.available_at),
      lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
      completed_at = NULL, error_code = NULL, updated_at = v_now;
  END LOOP;
  PERFORM memory_l3_persona_enqueue_user(p_user_id);
END
$function$;

CREATE FUNCTION memory_l3_persona_invalidate_user(p_user_id UUID)
RETURNS VOID
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  PERFORM memory_l3_persona_invalidate_user_at_generation(
    p_user_id, memory_l3_persona_advance_generation(p_user_id)
  );
END
$function$;

CREATE FUNCTION memory_l3_persona_memory_changed()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_user_id UUID;
  v_old_relevant BOOLEAN := false;
  v_new_relevant BOOLEAN := false;
BEGIN
  -- Account deletion cascades through user_memory_state and user_memories in
  -- database-defined order. Once the owning user is gone, every L3 row is
  -- already cascade-owned; attempting to recreate state would violate the
  -- users foreign key and abort the account deletion.
  IF TG_OP = 'DELETE' AND NOT EXISTS (
    SELECT 1 FROM users WHERE id = OLD.user_id
  ) THEN
    RETURN OLD;
  END IF;
  IF TG_OP IN ('UPDATE', 'DELETE') THEN
    v_old_relevant := OLD.scope_type = 'global'
      AND OLD.memory_type IN ('fact', 'preference', 'instruction', 'warning', 'decision');
  END IF;
  IF TG_OP IN ('INSERT', 'UPDATE') THEN
    v_new_relevant := NEW.scope_type = 'global'
      AND NEW.memory_type IN ('fact', 'preference', 'instruction', 'warning', 'decision');
  END IF;
  IF NOT v_old_relevant AND NOT v_new_relevant THEN
    RETURN COALESCE(NEW, OLD);
  END IF;
  IF TG_OP = 'UPDATE' AND ROW(
      OLD.content_hash, OLD.revision, OLD.visibility_epoch, OLD.scope_type,
      OLD.project_id, OLD.scope_conversation_id, OLD.scope_generation,
      OLD.memory_type, OLD.sensitivity, OLD.enabled, OLD.lifecycle_status,
      OLD.valid_from, OLD.valid_to, OLD.expires_at, OLD.deleted_at
    ) IS NOT DISTINCT FROM ROW(
      NEW.content_hash, NEW.revision, NEW.visibility_epoch, NEW.scope_type,
      NEW.project_id, NEW.scope_conversation_id, NEW.scope_generation,
      NEW.memory_type, NEW.sensitivity, NEW.enabled, NEW.lifecycle_status,
      NEW.valid_from, NEW.valid_to, NEW.expires_at, NEW.deleted_at
    )
  THEN
    RETURN NEW;
  END IF;
  v_user_id := CASE WHEN TG_OP = 'DELETE' THEN OLD.user_id ELSE NEW.user_id END;
  PERFORM memory_l3_persona_invalidate_user(v_user_id);
  RETURN COALESCE(NEW, OLD);
END
$function$;

CREATE TRIGGER user_memories_l3_persona_insert
AFTER INSERT ON user_memories
FOR EACH ROW EXECUTE FUNCTION memory_l3_persona_memory_changed();
CREATE TRIGGER user_memories_l3_persona_update
AFTER UPDATE OF content_hash, revision, visibility_epoch, scope_type, project_id,
  scope_conversation_id, scope_generation, memory_type, sensitivity, enabled,
  lifecycle_status, valid_from, valid_to, expires_at, deleted_at
ON user_memories
FOR EACH ROW EXECUTE FUNCTION memory_l3_persona_memory_changed();
CREATE TRIGGER user_memories_l3_persona_delete
AFTER DELETE ON user_memories
FOR EACH ROW EXECUTE FUNCTION memory_l3_persona_memory_changed();

CREATE FUNCTION memory_l3_persona_settings_changed()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF OLD.sensitive_memory_enabled IS DISTINCT FROM NEW.sensitive_memory_enabled THEN
    PERFORM memory_l3_persona_invalidate_user(NEW.user_id);
  ELSE
    PERFORM memory_l3_persona_reconcile_user(NEW.user_id);
  END IF;
  RETURN NEW;
END
$function$;
CREATE TRIGGER user_memory_settings_l3_persona_update
AFTER UPDATE OF enabled, search_enabled, sensitive_memory_enabled, l3_mode
ON user_memory_settings
FOR EACH ROW EXECUTE FUNCTION memory_l3_persona_settings_changed();

CREATE FUNCTION memory_l3_persona_state_changed()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF OLD.visibility_epoch IS DISTINCT FROM NEW.visibility_epoch THEN
    PERFORM memory_l3_persona_invalidate_user(NEW.user_id);
  ELSIF OLD.active_retrieval_profile_id IS DISTINCT FROM NEW.active_retrieval_profile_id THEN
    PERFORM memory_l3_persona_reconcile_user(NEW.user_id);
  END IF;
  RETURN NEW;
END
$function$;
CREATE TRIGGER user_memory_state_l3_persona_update
AFTER UPDATE OF visibility_epoch, active_retrieval_profile_id
ON user_memory_state FOR EACH ROW EXECUTE FUNCTION memory_l3_persona_state_changed();

CREATE FUNCTION memory_queue_l3_persona_embedding()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  IF NEW.entity_type <> 'l3_persona' OR NEW.embedding_status <> 'pending' THEN
    RETURN NEW;
  END IF;
  INSERT INTO user_memory_persona_embedding_jobs(
    job_id, entity_type, entity_id, user_id, entity_revision, content_hash,
    source_watermark, visibility_epoch, generation, embedding_profile_id,
    embedding_model_id, embedding_dimensions, status, attempt_count,
    max_attempts, available_at, created_at, updated_at
  ) VALUES (
    gen_random_uuid(), NEW.entity_type, NEW.entity_id, NEW.user_id,
    NEW.entity_revision, NEW.content_hash, NEW.source_watermark,
    NEW.visibility_epoch, NEW.generation, NEW.embedding_profile_id,
    NEW.embedding_model_id, NEW.embedding_dimensions, 'pending', 0, 8,
    v_now, v_now, v_now
  )
  ON CONFLICT (entity_type, entity_id) DO UPDATE SET
    job_id = EXCLUDED.job_id, user_id = EXCLUDED.user_id,
    entity_revision = EXCLUDED.entity_revision,
    content_hash = EXCLUDED.content_hash,
    source_watermark = EXCLUDED.source_watermark,
    visibility_epoch = EXCLUDED.visibility_epoch,
    generation = EXCLUDED.generation,
    embedding_profile_id = EXCLUDED.embedding_profile_id,
    embedding_model_id = EXCLUDED.embedding_model_id,
    embedding_dimensions = EXCLUDED.embedding_dimensions,
    provider_record_id = NULL, provider_config_updated_at = NULL,
    status = 'pending', attempt_count = 0, max_attempts = 8,
    available_at = v_now, lease_owner = NULL, lease_token = NULL,
    lease_expires_at = NULL, completed_at = NULL, error_code = NULL,
    created_at = v_now, updated_at = v_now;
  RETURN NEW;
END
$function$;
CREATE TRIGGER user_memory_persona_search_embedding_queue
AFTER INSERT OR UPDATE OF entity_revision, content_hash, source_watermark,
  visibility_epoch, generation, embedding_profile_id, embedding_model_id,
  embedding_dimensions, embedding_status
ON user_memory_persona_search_projections
FOR EACH ROW EXECUTE FUNCTION memory_queue_l3_persona_embedding();

CREATE FUNCTION memory_worker_claim_l3_persona_job(
  p_worker_id UUID,
  p_lease_token UUID,
  p_lease_seconds INTEGER,
  p_refresh_enabled BOOLEAN
) RETURNS TABLE (
  job_id UUID,
  stage TEXT,
  user_id UUID,
  target_persona_id UUID,
  visibility_epoch BIGINT,
  generation BIGINT,
  profile_id TEXT,
  source_watermark TEXT,
  attempt_count INTEGER,
  max_attempts INTEGER,
  provider_record_id UUID,
  provider_config_updated_at TIMESTAMPTZ,
  model_id TEXT,
  lease_expires_at TIMESTAMPTZ
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_job user_memory_persona_jobs%ROWTYPE;
  v_now TIMESTAMPTZ := clock_timestamp();
  v_lease_expires TIMESTAMPTZ;
  v_provider provider_configs%ROWTYPE;
  v_task_model TEXT;
  v_provider_id TEXT;
  v_memory_count INTEGER := 0;
  v_expired_user UUID;
BEGIN
  IF p_worker_id IS NULL OR p_lease_token IS NULL
    OR p_lease_seconds NOT BETWEEN 5 AND 900 OR p_refresh_enabled IS NULL
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L3_PERSONA_LEASE_INVALID';
  END IF;
  v_lease_expires := v_now + make_interval(secs => p_lease_seconds);

  UPDATE user_memory_persona_jobs job
  SET status = CASE WHEN job.attempt_count >= job.max_attempts
        THEN 'dead_letter' ELSE 'pending' END,
      lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
      completed_at = CASE WHEN job.attempt_count >= job.max_attempts
        THEN v_now ELSE NULL END,
      error_code = CASE WHEN job.attempt_count >= job.max_attempts
        THEN 'LEASE_EXPIRED' ELSE NULL END,
      provider_record_id = NULL, provider_config_updated_at = NULL,
      model_id = NULL,
      available_at = CASE WHEN job.attempt_count >= job.max_attempts
        THEN job.available_at ELSE v_now END,
      updated_at = v_now
  WHERE job.status = 'processing' AND job.lease_expires_at <= v_now;

  FOR v_expired_user IN
    SELECT DISTINCT persona.user_id
    FROM user_memory_persona_versions persona
    JOIN user_memory_state state ON state.user_id = persona.user_id
    JOIN user_memory_settings settings ON settings.user_id = persona.user_id
    WHERE persona.lifecycle_status IN ('shadow', 'active', 'disabled')
      AND persona.deleted_at IS NULL
      AND persona.generation = state.active_l3_generation
      AND (
        persona.visibility_epoch <> state.visibility_epoch
        OR persona.source_watermark <> memory_l3_persona_source_watermark(persona.user_id)
        OR EXISTS (
          SELECT 1 FROM user_memory_persona_members member
          LEFT JOIN user_memories memory
            ON memory.id = member.memory_id AND memory.user_id = member.user_id
          WHERE member.persona_id = persona.id AND (
            memory.id IS NULL OR memory.deleted_at IS NOT NULL OR NOT memory.enabled
            OR memory.lifecycle_status <> 'active'
            OR memory.scope_type <> 'global'
            OR memory.project_id IS NOT NULL
            OR memory.scope_conversation_id IS NOT NULL
            OR memory.scope_generation <> 1
            OR memory.memory_type NOT IN (
              'fact', 'preference', 'instruction', 'warning', 'decision'
            )
            OR memory.revision <> member.memory_revision
            OR memory.content_hash <> member.memory_content_hash
            OR memory.visibility_epoch <> persona.visibility_epoch
            OR (memory.valid_from IS NOT NULL AND memory.valid_from > v_now)
            OR (memory.valid_to IS NOT NULL AND memory.valid_to <= v_now)
            OR (memory.expires_at IS NOT NULL AND memory.expires_at <= v_now)
            OR memory_governance_classify_sensitivity(memory.content) = 'secret'
            OR (memory.sensitivity = 'sensitive'
              AND NOT settings.sensitive_memory_enabled)
          )
        )
      )
    ORDER BY persona.user_id
    LIMIT 16
  LOOP
    PERFORM memory_l3_persona_invalidate_user(v_expired_user);
  END LOOP;

  SELECT candidate.* INTO v_job
  FROM user_memory_persona_jobs candidate
  LEFT JOIN user_memory_state state
    ON candidate.stage = 'refresh' AND state.user_id = candidate.user_id
   AND state.visibility_epoch = candidate.visibility_epoch
   AND state.active_l3_generation = candidate.generation
  LEFT JOIN user_memory_settings settings ON settings.user_id = candidate.user_id
  WHERE candidate.status = 'pending' AND candidate.available_at <= v_now
    AND candidate.attempt_count < candidate.max_attempts
    AND (
      candidate.stage = 'purge'
      OR (
        p_refresh_enabled AND candidate.stage = 'refresh'
        AND state.user_id IS NOT NULL
        AND settings.enabled AND settings.search_enabled
        AND settings.l3_mode <> 'off'
        AND candidate.source_watermark =
          memory_l3_persona_source_watermark(candidate.user_id)
      )
    )
  ORDER BY CASE candidate.stage WHEN 'purge' THEN 0 ELSE 1 END,
    candidate.available_at, candidate.created_at, candidate.job_id
  FOR UPDATE OF candidate SKIP LOCKED
  LIMIT 1;
  IF NOT FOUND THEN
    RETURN;
  END IF;

  IF v_job.stage = 'refresh' THEN
    SELECT count(*) INTO v_memory_count
    FROM user_memories memory
    JOIN user_memory_settings settings ON settings.user_id = memory.user_id
    WHERE memory.user_id = v_job.user_id
      AND memory.scope_type = 'global'
      AND memory.project_id IS NULL
      AND memory.scope_conversation_id IS NULL
      AND memory.scope_generation = 1
      AND memory.memory_type IN ('fact', 'preference', 'instruction', 'warning', 'decision')
      AND memory.visibility_epoch = v_job.visibility_epoch
      AND memory.deleted_at IS NULL AND memory.enabled
      AND memory.lifecycle_status = 'active'
      AND (memory.valid_from IS NULL OR memory.valid_from <= v_now)
      AND (memory.valid_to IS NULL OR v_now < memory.valid_to)
      AND (memory.expires_at IS NULL OR v_now < memory.expires_at)
      AND memory_governance_classify_sensitivity(memory.content) <> 'secret'
      AND (memory.sensitivity = 'normal' OR settings.sensitive_memory_enabled);
    IF v_memory_count >= 2 THEN
      SELECT trim(task.memory) INTO v_task_model
      FROM task_model_settings task WHERE task.user_id = v_job.user_id;
      IF COALESCE(v_task_model, '') = '' OR position(':' IN v_task_model) <= 1 THEN
        RETURN;
      END IF;
      v_provider_id := split_part(v_task_model, ':', 1);
      SELECT provider.* INTO v_provider
      FROM provider_configs provider
      WHERE provider.user_id = v_job.user_id
        AND provider.provider_id = v_provider_id
        AND provider.deleted_at IS NULL
        AND provider.encrypted_secret_ref IS NOT NULL
        AND COALESCE(provider.config->>'enabled', 'true') = 'true'
      ORDER BY provider.updated_at DESC, provider.created_at DESC
      LIMIT 1;
      IF NOT FOUND THEN
        RETURN;
      END IF;
      v_job.provider_record_id := v_provider.id;
      v_job.provider_config_updated_at := v_provider.updated_at;
      v_job.model_id := substring(v_task_model FROM position(':' IN v_task_model) + 1);
      IF length(trim(v_job.model_id)) NOT BETWEEN 1 AND 256 THEN
        RETURN;
      END IF;
    END IF;
  END IF;

  UPDATE user_memory_persona_jobs job
  SET status = 'processing', attempt_count = job.attempt_count + 1,
      provider_record_id = v_job.provider_record_id,
      provider_config_updated_at = v_job.provider_config_updated_at,
      model_id = v_job.model_id, input_memory_ids = ARRAY[]::UUID[],
      lease_owner = p_worker_id, lease_token = p_lease_token,
      lease_expires_at = v_lease_expires, completed_at = NULL,
      error_code = NULL, updated_at = v_now
  WHERE job.job_id = v_job.job_id
  RETURNING job.* INTO v_job;

  RETURN QUERY SELECT v_job.job_id, v_job.stage, v_job.user_id,
    v_job.target_persona_id, v_job.visibility_epoch, v_job.generation,
    v_job.profile_id, v_job.source_watermark, v_job.attempt_count,
    v_job.max_attempts, v_job.provider_record_id,
    v_job.provider_config_updated_at, v_job.model_id, v_job.lease_expires_at;
END
$function$;

CREATE FUNCTION memory_worker_hydrate_l3_persona_refresh(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID
) RETURNS TABLE (
  user_id UUID,
  visibility_epoch BIGINT,
  generation BIGINT,
  profile_id TEXT,
  source_watermark TEXT,
  sensitive_memory_enabled BOOLEAN,
  memories JSONB,
  provider_record_id UUID,
  provider_id TEXT,
  provider_label TEXT,
  encrypted_secret_ref TEXT,
  provider_config JSONB,
  provider_config_updated_at TIMESTAMPTZ,
  model_id TEXT
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_job user_memory_persona_jobs%ROWTYPE;
  v_memories JSONB;
  v_ids UUID[];
  v_sensitive BOOLEAN;
  v_provider provider_configs%ROWTYPE;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  SELECT job.* INTO v_job FROM user_memory_persona_jobs job
  JOIN user_memory_state state
    ON state.user_id = job.user_id
   AND state.visibility_epoch = job.visibility_epoch
   AND state.active_l3_generation = job.generation
  JOIN user_memory_settings settings
    ON settings.user_id = job.user_id AND settings.enabled
   AND settings.search_enabled AND settings.l3_mode <> 'off'
  WHERE job.job_id = p_job_id AND job.stage = 'refresh'
    AND job.status = 'processing' AND job.lease_owner = p_worker_id
    AND job.lease_token = p_lease_token AND job.lease_expires_at > v_now
    AND job.source_watermark = memory_l3_persona_source_watermark(job.user_id)
  FOR UPDATE OF job;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_L3_PERSONA_SOURCE_DRIFT';
  END IF;
  SELECT settings.sensitive_memory_enabled INTO v_sensitive
  FROM user_memory_settings settings WHERE settings.user_id = v_job.user_id;

  WITH bounded AS (
    SELECT memory.* FROM user_memories memory
    WHERE memory.user_id = v_job.user_id
      AND memory.scope_type = 'global'
      AND memory.project_id IS NULL
      AND memory.scope_conversation_id IS NULL
      AND memory.scope_generation = 1
      AND memory.memory_type IN ('fact', 'preference', 'instruction', 'warning', 'decision')
      AND memory.visibility_epoch = v_job.visibility_epoch
      AND memory.deleted_at IS NULL AND memory.enabled
      AND memory.lifecycle_status = 'active'
      AND (memory.valid_from IS NULL OR memory.valid_from <= v_now)
      AND (memory.valid_to IS NULL OR v_now < memory.valid_to)
      AND (memory.expires_at IS NULL OR v_now < memory.expires_at)
      AND memory_governance_classify_sensitivity(memory.content) <> 'secret'
      AND (memory.sensitivity = 'normal' OR v_sensitive)
    ORDER BY memory.importance DESC, memory.updated_at DESC, memory.id
    LIMIT 80
  )
  SELECT COALESCE(jsonb_agg(jsonb_build_object(
      'id', bounded.id::TEXT,
      'revision', bounded.revision,
      'type', bounded.memory_type,
      'content', bounded.content,
      'contentHash', bounded.content_hash,
      'sensitivity', bounded.sensitivity,
      'importance', bounded.importance,
      'updatedAt', bounded.updated_at
    ) ORDER BY bounded.importance DESC, bounded.updated_at DESC, bounded.id), '[]'::JSONB),
    COALESCE(array_agg(bounded.id ORDER BY bounded.id), ARRAY[]::UUID[])
  INTO v_memories, v_ids FROM bounded;

  IF cardinality(v_ids) >= 2 THEN
    SELECT provider.* INTO v_provider FROM provider_configs provider
    JOIN task_model_settings task ON task.user_id = provider.user_id
    WHERE provider.id = v_job.provider_record_id
      AND provider.user_id = v_job.user_id
      AND provider.updated_at = v_job.provider_config_updated_at
      AND provider.deleted_at IS NULL
      AND provider.encrypted_secret_ref IS NOT NULL
      AND COALESCE(provider.config->>'enabled', 'true') = 'true'
      AND trim(task.memory) = provider.provider_id || ':' || v_job.model_id;
    IF NOT FOUND THEN
      RAISE EXCEPTION USING ERRCODE = 'P0001',
        MESSAGE = 'MEMORY_L3_PERSONA_PROFILE_DRIFT';
    END IF;
  END IF;
  UPDATE user_memory_persona_jobs job
  SET input_memory_ids = v_ids, updated_at = v_now
  WHERE job.job_id = v_job.job_id;

  RETURN QUERY SELECT v_job.user_id, v_job.visibility_epoch, v_job.generation,
    v_job.profile_id, v_job.source_watermark, v_sensitive, v_memories,
    v_provider.id, v_provider.provider_id, v_provider.label,
    v_provider.encrypted_secret_ref, v_provider.config, v_provider.updated_at,
    v_job.model_id;
END
$function$;

CREATE FUNCTION memory_worker_complete_l3_persona_refresh(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_persona JSONB
) RETURNS JSONB
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_job user_memory_persona_jobs%ROWTYPE;
  v_existing user_memory_persona_versions%ROWTYPE;
  v_previous_disabled BOOLEAN := false;
  v_persona_id UUID;
  v_content TEXT;
  v_content_hash TEXT;
  v_token_count INTEGER;
  v_derived_sensitivity TEXT;
  v_member_ids UUID[];
  v_member_count INTEGER;
  v_members_current BOOLEAN;
  v_member_sensitive BOOLEAN;
  v_exact_terms TEXT[];
  v_bm25_text TEXT;
  v_active BOOLEAN;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  SELECT job.* INTO v_job FROM user_memory_persona_jobs job
  JOIN user_memory_state state
    ON state.user_id = job.user_id
   AND state.visibility_epoch = job.visibility_epoch
   AND state.active_l3_generation = job.generation
  JOIN user_memory_settings settings
    ON settings.user_id = job.user_id AND settings.enabled
   AND settings.search_enabled AND settings.l3_mode <> 'off'
  WHERE job.job_id = p_job_id AND job.stage = 'refresh'
    AND job.status = 'processing' AND job.lease_owner = p_worker_id
    AND job.lease_token = p_lease_token AND job.lease_expires_at > v_now
    AND job.source_watermark = memory_l3_persona_source_watermark(job.user_id)
  FOR UPDATE OF job;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_L3_PERSONA_SOURCE_DRIFT';
  END IF;

  IF p_persona IS NULL OR p_persona = 'null'::JSONB THEN
    UPDATE user_memory_persona_jobs job
    SET status = 'completed', lease_owner = NULL, lease_token = NULL,
        lease_expires_at = NULL, completed_at = v_now, error_code = NULL,
        updated_at = v_now
    WHERE job.job_id = v_job.job_id;
    RETURN jsonb_build_object(
      'personaCount', 0, 'generation', v_job.generation,
      'sourceWatermark', v_job.source_watermark
    );
  END IF;
  IF cardinality(v_job.input_memory_ids) < 2 THEN
    IF p_persona IS NOT NULL THEN
      RAISE EXCEPTION USING ERRCODE = '22023',
        MESSAGE = 'MEMORY_L3_PERSONA_UNAUTHORIZED_PROPOSAL';
    END IF;
  END IF;

  IF p_persona IS NULL OR jsonb_typeof(p_persona) <> 'object'
    OR ARRAY(SELECT key FROM jsonb_object_keys(p_persona) key ORDER BY key)
      <> ARRAY['content', 'memberMemoryIds']::TEXT[]
    OR jsonb_typeof(p_persona->'memberMemoryIds') <> 'array'
    OR NOT EXISTS (
      SELECT 1 FROM provider_configs provider
      JOIN task_model_settings task ON task.user_id = provider.user_id
      WHERE provider.id = v_job.provider_record_id
        AND provider.user_id = v_job.user_id
        AND provider.updated_at = v_job.provider_config_updated_at
        AND provider.deleted_at IS NULL
        AND provider.encrypted_secret_ref IS NOT NULL
        AND COALESCE(provider.config->>'enabled', 'true') = 'true'
        AND trim(task.memory) = provider.provider_id || ':' || v_job.model_id
    )
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L3_PERSONA_PROPOSAL_INVALID';
  END IF;

  v_content := trim(p_persona->>'content');
  v_content_hash := encode(sha256(convert_to(v_content, 'UTF8')), 'hex');
  v_token_count := memory_l3_persona_estimated_tokens(v_content);
  IF v_content = '' OR char_length(v_content) > 4000 OR v_token_count > 300
    OR memory_governance_classify_sensitivity(v_content) = 'secret'
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = CASE
        WHEN v_token_count > 300 THEN 'MEMORY_L3_PERSONA_TOKEN_BUDGET_EXCEEDED'
        WHEN memory_governance_classify_sensitivity(v_content) = 'secret'
          THEN 'MEMORY_L3_PERSONA_SECRET_REJECTED'
        ELSE 'MEMORY_L3_PERSONA_PROPOSAL_INVALID'
      END;
  END IF;
  IF EXISTS (
    SELECT 1 FROM jsonb_array_elements(p_persona->'memberMemoryIds') member
    WHERE jsonb_typeof(member) <> 'string'
      OR trim(member #>> '{}') !~
        '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L3_PERSONA_MEMBER_INVALID';
  END IF;
  SELECT COALESCE(array_agg((member #>> '{}')::UUID ORDER BY member #>> '{}'),
      ARRAY[]::UUID[]), count(DISTINCT member #>> '{}')
  INTO v_member_ids, v_member_count
  FROM jsonb_array_elements(p_persona->'memberMemoryIds') member;
  IF cardinality(v_member_ids) NOT BETWEEN 2 AND 50
    OR v_member_count <> cardinality(v_member_ids)
    OR NOT v_job.input_memory_ids @> v_member_ids
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L3_PERSONA_MEMBER_INVALID';
  END IF;
  SELECT count(*) = cardinality(v_member_ids),
    COALESCE(bool_or(memory.sensitivity = 'sensitive'), false)
  INTO v_members_current, v_member_sensitive
  FROM user_memories memory
  JOIN user_memory_settings settings ON settings.user_id = memory.user_id
  WHERE memory.id = ANY(v_member_ids)
    AND memory.user_id = v_job.user_id
    AND memory.scope_type = 'global'
    AND memory.project_id IS NULL
    AND memory.scope_conversation_id IS NULL
    AND memory.scope_generation = 1
    AND memory.memory_type IN ('fact', 'preference', 'instruction', 'warning', 'decision')
    AND memory.visibility_epoch = v_job.visibility_epoch
    AND memory.deleted_at IS NULL AND memory.enabled
    AND memory.lifecycle_status = 'active'
    AND (memory.valid_from IS NULL OR memory.valid_from <= v_now)
    AND (memory.valid_to IS NULL OR v_now < memory.valid_to)
    AND (memory.expires_at IS NULL OR v_now < memory.expires_at)
    AND memory_governance_classify_sensitivity(memory.content) <> 'secret'
    AND (memory.sensitivity = 'normal' OR settings.sensitive_memory_enabled);
  IF NOT v_members_current THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_L3_PERSONA_MEMBER_DRIFT';
  END IF;
  v_derived_sensitivity := CASE
    WHEN v_member_sensitive
      OR memory_governance_classify_sensitivity(v_content) = 'sensitive'
    THEN 'sensitive' ELSE 'normal' END;

  SELECT profile.lifecycle_status = 'active'
    AND state.active_retrieval_profile_id = 'memory_hybrid_bge_m3_rrf60_v1'
    AND settings.enabled AND settings.search_enabled AND settings.l3_mode <> 'off'
  INTO v_active
  FROM memory_l3_persona_profiles profile
  JOIN user_memory_state state ON state.user_id = v_job.user_id
  JOIN user_memory_settings settings ON settings.user_id = state.user_id
  WHERE profile.profile_id = v_job.profile_id;
  v_active := COALESCE(v_active, false);
  SELECT COALESCE(persona.user_disabled, false) INTO v_previous_disabled
  FROM user_memory_persona_versions persona
  WHERE persona.user_id = v_job.user_id
  ORDER BY persona.generation DESC, persona.created_at DESC
  LIMIT 1;
  v_previous_disabled := COALESCE(v_previous_disabled, false);

  SELECT persona.* INTO v_existing
  FROM user_memory_persona_versions persona
  WHERE persona.user_id = v_job.user_id AND persona.generation = v_job.generation
  FOR UPDATE;
  IF FOUND THEN
    v_persona_id := v_existing.id;
    IF v_existing.source_watermark <> v_job.source_watermark THEN
      RAISE EXCEPTION USING ERRCODE = '40001',
        MESSAGE = 'MEMORY_L3_PERSONA_VERSION_CONFLICT';
    END IF;
    UPDATE user_memory_persona_versions persona
    SET content = v_content, content_hash = v_content_hash,
      token_count = v_token_count, sensitivity = v_derived_sensitivity,
      sensitive_input_included = v_member_sensitive,
      lifecycle_status = CASE WHEN persona.user_disabled THEN 'disabled'
        WHEN v_active THEN 'active' ELSE 'shadow' END,
      visibility_epoch = v_job.visibility_epoch,
      source_watermark = v_job.source_watermark,
      revision = persona.revision + 1, updated_at = v_now,
      activated_at = CASE WHEN NOT persona.user_disabled AND v_active
        THEN COALESCE(persona.activated_at, v_now) ELSE persona.activated_at END,
      disabled_at = CASE WHEN persona.user_disabled
        THEN COALESCE(persona.disabled_at, v_now) ELSE NULL END,
      stale_at = NULL, purge_after = NULL, purged_at = NULL, deleted_at = NULL
    WHERE persona.id = v_existing.id;
  ELSE
    v_persona_id := gen_random_uuid();
    INSERT INTO user_memory_persona_versions(
      id, user_id, content, content_hash, token_count, sensitivity,
      sensitive_input_included, lifecycle_status, user_disabled, profile_id,
      generation, visibility_epoch, source_watermark, revision,
      created_at, updated_at, activated_at, disabled_at
    ) VALUES (
      v_persona_id, v_job.user_id, v_content, v_content_hash, v_token_count,
      v_derived_sensitivity, v_member_sensitive,
      CASE WHEN v_previous_disabled THEN 'disabled'
        WHEN v_active THEN 'active' ELSE 'shadow' END,
      v_previous_disabled, v_job.profile_id, v_job.generation,
      v_job.visibility_epoch, v_job.source_watermark, 1, v_now, v_now,
      CASE WHEN NOT v_previous_disabled AND v_active THEN v_now ELSE NULL END,
      CASE WHEN v_previous_disabled THEN v_now ELSE NULL END
    );
  END IF;

  DELETE FROM user_memory_persona_members member
  WHERE member.persona_id = v_persona_id;
  INSERT INTO user_memory_persona_members(
    persona_id, memory_id, user_id, memory_revision, memory_content_hash, created_at
  )
  SELECT v_persona_id, memory.id, memory.user_id,
    memory.revision, memory.content_hash, v_now
  FROM user_memories memory
  WHERE memory.id = ANY(v_member_ids) AND memory.user_id = v_job.user_id
  ORDER BY memory.id;

  v_exact_terms := knowledge_bm25_shadow_query_terms(v_content);
  v_bm25_text := knowledge_build_bm25_shadow_text(v_content, v_exact_terms);
  INSERT INTO user_memory_persona_search_projections(
    entity_type, entity_id, user_id, entity_revision, sensitivity,
    visibility_epoch, generation, retrieval_profile_id, content_hash,
    source_watermark, exact_terms, bm25_text, lexical_status,
    embedding_status, created_at, updated_at
  )
  SELECT 'l3_persona', persona.id, persona.user_id, persona.revision,
    persona.sensitivity, persona.visibility_epoch, persona.generation,
    'memory_l3_persona_hybrid_bge_m3_rrf60_v1', persona.content_hash,
    persona.source_watermark, v_exact_terms, v_bm25_text, 'ready', 'pending',
    v_now, v_now
  FROM user_memory_persona_versions persona
  WHERE persona.id = v_persona_id AND NOT persona.user_disabled
  ON CONFLICT (entity_type, entity_id) DO UPDATE SET
    user_id = EXCLUDED.user_id, entity_revision = EXCLUDED.entity_revision,
    sensitivity = EXCLUDED.sensitivity,
    visibility_epoch = EXCLUDED.visibility_epoch,
    generation = EXCLUDED.generation,
    retrieval_profile_id = EXCLUDED.retrieval_profile_id,
    content_hash = EXCLUDED.content_hash,
    source_watermark = EXCLUDED.source_watermark,
    exact_terms = EXCLUDED.exact_terms, bm25_text = EXCLUDED.bm25_text,
    lexical_status = 'ready', embedding_status = 'pending',
    embedding_vector = NULL, embedding_error_code = NULL,
    embedding_updated_at = NULL, updated_at = v_now;

  UPDATE user_memory_persona_jobs job
  SET status = 'completed', lease_owner = NULL, lease_token = NULL,
      lease_expires_at = NULL, completed_at = v_now, error_code = NULL,
      updated_at = v_now
  WHERE job.job_id = v_job.job_id;
  RETURN jsonb_build_object(
    'personaCount', 1, 'personaId', v_persona_id::TEXT,
    'generation', v_job.generation, 'tokenCount', v_token_count,
    'sourceWatermark', v_job.source_watermark
  );
END
$function$;

CREATE FUNCTION memory_worker_complete_l3_persona_purge(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_job user_memory_persona_jobs%ROWTYPE;
  v_persona user_memory_persona_versions%ROWTYPE;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  SELECT job.* INTO v_job FROM user_memory_persona_jobs job
  WHERE job.job_id = p_job_id AND job.stage = 'purge'
    AND job.status = 'processing' AND job.lease_owner = p_worker_id
    AND job.lease_token = p_lease_token AND job.lease_expires_at > v_now
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_L3_PERSONA_LEASE_LOST';
  END IF;
  SELECT persona.* INTO v_persona FROM user_memory_persona_versions persona
  WHERE persona.id = v_job.target_persona_id AND persona.user_id = v_job.user_id
  FOR UPDATE;
  IF FOUND AND v_persona.lifecycle_status = 'stale'
    AND v_persona.purge_after <= v_now AND v_persona.deleted_at IS NULL
  THEN
    DELETE FROM user_memory_persona_search_projections projection
    WHERE projection.entity_type = 'l3_persona' AND projection.entity_id = v_persona.id;
    DELETE FROM user_memory_persona_members member WHERE member.persona_id = v_persona.id;
    UPDATE user_memory_persona_versions persona
    SET content = '', purged_at = v_now, deleted_at = v_now, updated_at = v_now
    WHERE persona.id = v_persona.id;
  END IF;
  UPDATE user_memory_persona_jobs job
  SET status = 'completed', lease_owner = NULL, lease_token = NULL,
      lease_expires_at = NULL, completed_at = v_now, error_code = NULL,
      updated_at = v_now
  WHERE job.job_id = v_job.job_id;
  RETURN true;
END
$function$;

CREATE FUNCTION memory_worker_retry_l3_persona_job(
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
  v_job user_memory_persona_jobs%ROWTYPE;
  v_now TIMESTAMPTZ := clock_timestamp();
  v_terminal BOOLEAN;
BEGIN
  IF p_error_code !~ '^[A-Z][A-Z0-9_]{0,63}$'
    OR p_available_at < v_now OR p_terminal IS NULL
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L3_PERSONA_RETRY_INVALID';
  END IF;
  SELECT * INTO v_job FROM user_memory_persona_jobs job
  WHERE job.job_id = p_job_id AND job.status = 'processing'
    AND job.lease_owner = p_worker_id AND job.lease_token = p_lease_token
    AND job.lease_expires_at > v_now FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_L3_PERSONA_LEASE_LOST';
  END IF;
  v_terminal := p_terminal OR v_job.attempt_count >= v_job.max_attempts;
  UPDATE user_memory_persona_jobs job
  SET status = CASE WHEN v_terminal THEN 'dead_letter' ELSE 'pending' END,
      provider_record_id = NULL, provider_config_updated_at = NULL,
      model_id = NULL, input_memory_ids = ARRAY[]::UUID[],
      lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
      completed_at = CASE WHEN v_terminal THEN v_now ELSE NULL END,
      error_code = CASE WHEN v_terminal THEN p_error_code ELSE NULL END,
      available_at = CASE WHEN v_terminal THEN job.available_at ELSE p_available_at END,
      updated_at = v_now
  WHERE job.job_id = p_job_id;
  RETURN CASE WHEN v_terminal THEN 'dead_letter' ELSE 'pending' END;
END
$function$;

CREATE FUNCTION memory_worker_claim_l3_persona_embedding_job(
  p_worker_id UUID,
  p_lease_token UUID,
  p_lease_seconds INTEGER
) RETURNS TABLE (
  job_id UUID,
  user_id UUID,
  persona_id UUID,
  persona_revision BIGINT,
  content_hash TEXT,
  source_watermark TEXT,
  visibility_epoch BIGINT,
  generation BIGINT,
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
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L3_PERSONA_EMBEDDING_LEASE_INVALID';
  END IF;
  v_lease_expires := v_now + make_interval(secs => p_lease_seconds);

  WITH expired AS MATERIALIZED (
    UPDATE user_memory_persona_embedding_jobs job
    SET status = CASE WHEN job.attempt_count >= job.max_attempts
          THEN 'dead_letter' ELSE 'pending' END,
        lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
        completed_at = CASE WHEN job.attempt_count >= job.max_attempts
          THEN v_now ELSE NULL END,
        error_code = CASE WHEN job.attempt_count >= job.max_attempts
          THEN 'LEASE_EXPIRED' ELSE NULL END,
        provider_record_id = NULL, provider_config_updated_at = NULL,
        available_at = CASE WHEN job.attempt_count >= job.max_attempts
          THEN job.available_at ELSE v_now END,
        updated_at = v_now
    WHERE job.status = 'processing' AND job.lease_expires_at <= v_now
    RETURNING job.*
  )
  UPDATE user_memory_persona_search_projections projection
  SET embedding_status = 'failed', embedding_vector = NULL,
      embedding_error_code = 'LEASE_EXPIRED', embedding_updated_at = v_now,
      updated_at = v_now
  FROM expired job
  WHERE job.status = 'dead_letter'
    AND projection.entity_type = job.entity_type
    AND projection.entity_id = job.entity_id
    AND projection.entity_revision = job.entity_revision
    AND projection.content_hash = job.content_hash
    AND projection.source_watermark = job.source_watermark
    AND projection.generation = job.generation
    AND projection.embedding_status = 'pending';

  WITH eligible_providers AS MATERIALIZED (
    SELECT provider.* FROM provider_configs provider
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
    SELECT provider.user_id FROM eligible_providers provider
    GROUP BY provider.user_id HAVING count(*) = 1
  )
  SELECT job.job_id, provider.id, provider.updated_at
  INTO v_job_id, v_provider_id, v_provider_updated_at
  FROM user_memory_persona_embedding_jobs job
  JOIN user_memory_persona_search_projections projection
    ON projection.entity_type = job.entity_type
   AND projection.entity_id = job.entity_id
   AND projection.user_id = job.user_id
   AND projection.entity_revision = job.entity_revision
   AND projection.content_hash = job.content_hash
   AND projection.source_watermark = job.source_watermark
   AND projection.visibility_epoch = job.visibility_epoch
   AND projection.generation = job.generation
   AND projection.embedding_status = 'pending'
  JOIN user_memory_persona_versions persona
    ON persona.id = projection.entity_id AND persona.user_id = projection.user_id
   AND persona.revision = projection.entity_revision
   AND persona.content_hash = projection.content_hash
   AND persona.source_watermark = projection.source_watermark
   AND persona.visibility_epoch = projection.visibility_epoch
   AND persona.generation = projection.generation
   AND persona.source_watermark = memory_l3_persona_source_watermark(persona.user_id)
  JOIN user_memory_state state
    ON state.user_id = persona.user_id
   AND state.visibility_epoch = persona.visibility_epoch
   AND state.active_l3_generation = persona.generation
  JOIN user_memory_settings settings
    ON settings.user_id = persona.user_id AND settings.enabled
   AND settings.search_enabled AND settings.l3_mode <> 'off'
   AND (persona.sensitivity = 'normal' OR settings.sensitive_memory_enabled)
  JOIN unique_provider_users unique_provider ON unique_provider.user_id = job.user_id
  JOIN eligible_providers provider ON provider.user_id = unique_provider.user_id
  WHERE job.status = 'pending' AND job.available_at <= v_now
    AND job.attempt_count < job.max_attempts
    AND persona.deleted_at IS NULL
    AND persona.lifecycle_status IN ('shadow', 'active')
    AND NOT EXISTS (
      SELECT 1 FROM user_memory_persona_members member
      LEFT JOIN user_memories memory
        ON memory.id = member.memory_id AND memory.user_id = member.user_id
      WHERE member.persona_id = persona.id AND (
        memory.id IS NULL OR memory.deleted_at IS NOT NULL OR NOT memory.enabled
        OR memory.lifecycle_status <> 'active'
        OR memory.revision <> member.memory_revision
        OR memory.content_hash <> member.memory_content_hash
        OR memory.scope_type <> 'global'
        OR memory.project_id IS NOT NULL
        OR memory.scope_conversation_id IS NOT NULL
        OR memory.scope_generation <> 1
        OR memory.memory_type NOT IN (
          'fact', 'preference', 'instruction', 'warning', 'decision'
        )
        OR memory.visibility_epoch <> persona.visibility_epoch
        OR memory_governance_classify_sensitivity(memory.content) = 'secret'
        OR (memory.valid_from IS NOT NULL AND memory.valid_from > v_now)
        OR (memory.valid_to IS NOT NULL AND memory.valid_to <= v_now)
        OR (memory.expires_at IS NOT NULL AND memory.expires_at <= v_now)
        OR (memory.sensitivity = 'sensitive' AND NOT settings.sensitive_memory_enabled)
      )
    )
  ORDER BY job.available_at, job.created_at, job.job_id
  FOR UPDATE OF job SKIP LOCKED LIMIT 1;
  IF v_job_id IS NULL THEN
    RETURN;
  END IF;
  UPDATE user_memory_persona_embedding_jobs job
  SET status = 'processing', attempt_count = job.attempt_count + 1,
      provider_record_id = v_provider_id,
      provider_config_updated_at = v_provider_updated_at,
      lease_owner = p_worker_id, lease_token = p_lease_token,
      lease_expires_at = v_lease_expires, completed_at = NULL,
      error_code = NULL, updated_at = v_now
  WHERE job.job_id = v_job_id;
  RETURN QUERY SELECT job.job_id, job.user_id, job.entity_id,
    job.entity_revision, job.content_hash, job.source_watermark,
    job.visibility_epoch, job.generation, job.embedding_profile_id,
    job.embedding_model_id, job.embedding_dimensions::INTEGER,
    job.attempt_count, job.max_attempts, job.provider_record_id,
    job.provider_config_updated_at, job.lease_expires_at
  FROM user_memory_persona_embedding_jobs job WHERE job.job_id = v_job_id;
END
$function$;

CREATE FUNCTION memory_worker_hydrate_l3_persona_embedding_job(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID
) RETURNS TABLE (
  user_id UUID,
  persona_id UUID,
  content TEXT,
  content_hash TEXT,
  persona_revision BIGINT,
  source_watermark TEXT,
  visibility_epoch BIGINT,
  generation BIGINT,
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
  RETURN QUERY SELECT job.user_id, job.entity_id, persona.content,
    job.content_hash, job.entity_revision, job.source_watermark,
    job.visibility_epoch, job.generation, job.embedding_profile_id,
    job.embedding_model_id, job.embedding_dimensions::INTEGER,
    provider.id, provider.provider_id, provider.label,
    provider.encrypted_secret_ref, provider.config, provider.updated_at
  FROM user_memory_persona_embedding_jobs job
  JOIN user_memory_persona_search_projections projection
    ON projection.entity_type = job.entity_type
   AND projection.entity_id = job.entity_id
   AND projection.user_id = job.user_id
   AND projection.entity_revision = job.entity_revision
   AND projection.content_hash = job.content_hash
   AND projection.source_watermark = job.source_watermark
   AND projection.visibility_epoch = job.visibility_epoch
   AND projection.generation = job.generation
   AND projection.embedding_status = 'pending'
  JOIN user_memory_persona_versions persona
    ON persona.id = projection.entity_id AND persona.user_id = projection.user_id
   AND persona.revision = projection.entity_revision
   AND persona.content_hash = projection.content_hash
   AND persona.source_watermark = projection.source_watermark
   AND persona.visibility_epoch = projection.visibility_epoch
   AND persona.generation = projection.generation
   AND persona.source_watermark = memory_l3_persona_source_watermark(persona.user_id)
  JOIN user_memory_state state
    ON state.user_id = persona.user_id
   AND state.visibility_epoch = persona.visibility_epoch
   AND state.active_l3_generation = persona.generation
  JOIN user_memory_settings settings
    ON settings.user_id = persona.user_id AND settings.enabled
   AND settings.search_enabled AND settings.l3_mode <> 'off'
   AND (persona.sensitivity = 'normal' OR settings.sensitive_memory_enabled)
  JOIN provider_configs provider
    ON provider.id = job.provider_record_id AND provider.user_id = job.user_id
   AND provider.updated_at = job.provider_config_updated_at
   AND provider.provider_id = 'RAG:SILICONFLOW' AND provider.deleted_at IS NULL
  WHERE job.job_id = p_job_id AND job.status = 'processing'
    AND job.lease_owner = p_worker_id AND job.lease_token = p_lease_token
    AND job.lease_expires_at > v_now AND persona.deleted_at IS NULL
    AND persona.lifecycle_status IN ('shadow', 'active')
    AND NOT EXISTS (
      SELECT 1 FROM user_memory_persona_members member
      LEFT JOIN user_memories memory
        ON memory.id = member.memory_id AND memory.user_id = member.user_id
      WHERE member.persona_id = persona.id AND (
        memory.id IS NULL OR memory.deleted_at IS NOT NULL OR NOT memory.enabled
        OR memory.lifecycle_status <> 'active'
        OR memory.revision <> member.memory_revision
        OR memory.content_hash <> member.memory_content_hash
        OR memory.scope_type <> 'global'
        OR memory.project_id IS NOT NULL
        OR memory.scope_conversation_id IS NOT NULL
        OR memory.scope_generation <> 1
        OR memory.memory_type NOT IN (
          'fact', 'preference', 'instruction', 'warning', 'decision'
        )
        OR memory.visibility_epoch <> persona.visibility_epoch
        OR memory_governance_classify_sensitivity(memory.content) = 'secret'
        OR (memory.valid_from IS NOT NULL AND memory.valid_from > v_now)
        OR (memory.valid_to IS NOT NULL AND memory.valid_to <= v_now)
        OR (memory.expires_at IS NOT NULL AND memory.expires_at <= v_now)
        OR (memory.sensitivity = 'sensitive' AND NOT settings.sensitive_memory_enabled)
      )
    );
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_L3_PERSONA_EMBEDDING_SOURCE_DRIFT';
  END IF;
END
$function$;

CREATE FUNCTION memory_worker_complete_l3_persona_embedding_job(
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
  v_job user_memory_persona_embedding_jobs%ROWTYPE;
  v_now TIMESTAMPTZ := clock_timestamp();
  v_norm DOUBLE PRECISION;
BEGIN
  IF p_embedding IS NULL OR cardinality(p_embedding) <> 1024 THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L3_PERSONA_EMBEDDING_VECTOR_INVALID';
  END IF;
  SELECT sqrt(sum(component::DOUBLE PRECISION * component::DOUBLE PRECISION))
  INTO v_norm FROM unnest(p_embedding) component;
  IF v_norm IS NULL OR v_norm <= 0 OR v_norm > 1e100 THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L3_PERSONA_EMBEDDING_VECTOR_INVALID';
  END IF;
  SELECT job.* INTO v_job FROM user_memory_persona_embedding_jobs job
  JOIN provider_configs provider
    ON provider.id = job.provider_record_id AND provider.user_id = job.user_id
   AND provider.updated_at = job.provider_config_updated_at
   AND provider.provider_id = 'RAG:SILICONFLOW' AND provider.deleted_at IS NULL
  WHERE job.job_id = p_job_id AND job.status = 'processing'
    AND job.lease_owner = p_worker_id AND job.lease_token = p_lease_token
    AND job.lease_expires_at > v_now FOR UPDATE OF job;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_L3_PERSONA_EMBEDDING_LEASE_LOST';
  END IF;
  UPDATE user_memory_persona_search_projections projection
  SET embedding_vector = p_embedding::vector(1024),
      embedding_status = 'ready', embedding_error_code = NULL,
      embedding_updated_at = v_now, updated_at = v_now
  WHERE projection.entity_type = v_job.entity_type
    AND projection.entity_id = v_job.entity_id
    AND projection.user_id = v_job.user_id
    AND projection.entity_revision = v_job.entity_revision
    AND projection.content_hash = v_job.content_hash
    AND projection.source_watermark = v_job.source_watermark
    AND projection.visibility_epoch = v_job.visibility_epoch
    AND projection.generation = v_job.generation
    AND projection.embedding_status = 'pending'
    AND EXISTS (
      SELECT 1 FROM user_memory_persona_versions persona
      JOIN user_memory_state state
        ON state.user_id = persona.user_id
       AND state.visibility_epoch = persona.visibility_epoch
       AND state.active_l3_generation = persona.generation
      JOIN user_memory_settings settings
        ON settings.user_id = persona.user_id AND settings.enabled
       AND settings.search_enabled AND settings.l3_mode <> 'off'
       AND (persona.sensitivity = 'normal' OR settings.sensitive_memory_enabled)
      WHERE persona.id = projection.entity_id AND persona.user_id = projection.user_id
        AND persona.revision = projection.entity_revision
        AND persona.content_hash = projection.content_hash
        AND persona.source_watermark = projection.source_watermark
        AND persona.visibility_epoch = projection.visibility_epoch
        AND persona.generation = projection.generation
        AND persona.source_watermark = memory_l3_persona_source_watermark(persona.user_id)
        AND persona.deleted_at IS NULL
        AND persona.lifecycle_status IN ('shadow', 'active')
            AND NOT EXISTS (
          SELECT 1 FROM user_memory_persona_members member
          LEFT JOIN user_memories memory
            ON memory.id = member.memory_id AND memory.user_id = member.user_id
          WHERE member.persona_id = persona.id AND (
            memory.id IS NULL OR memory.deleted_at IS NOT NULL OR NOT memory.enabled
            OR memory.lifecycle_status <> 'active'
            OR memory.revision <> member.memory_revision
            OR memory.content_hash <> member.memory_content_hash
            OR memory.scope_type <> 'global'
        OR memory.project_id IS NOT NULL
        OR memory.scope_conversation_id IS NOT NULL
        OR memory.scope_generation <> 1
        OR memory.memory_type NOT IN (
          'fact', 'preference', 'instruction', 'warning', 'decision'
        )
        OR memory.visibility_epoch <> persona.visibility_epoch
        OR memory_governance_classify_sensitivity(memory.content) = 'secret'
            OR (memory.valid_from IS NOT NULL AND memory.valid_from > v_now)
            OR (memory.valid_to IS NOT NULL AND memory.valid_to <= v_now)
            OR (memory.expires_at IS NOT NULL AND memory.expires_at <= v_now)
            OR (memory.sensitivity = 'sensitive'
              AND NOT settings.sensitive_memory_enabled)
          )
        )
    );
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_L3_PERSONA_EMBEDDING_SOURCE_DRIFT';
  END IF;
  UPDATE user_memory_persona_embedding_jobs job
  SET status = 'completed', lease_owner = NULL, lease_token = NULL,
      lease_expires_at = NULL, completed_at = v_now, error_code = NULL,
      updated_at = v_now WHERE job.job_id = v_job.job_id;
  RETURN true;
END
$function$;

CREATE FUNCTION memory_worker_retry_l3_persona_embedding_job(
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
  v_job user_memory_persona_embedding_jobs%ROWTYPE;
  v_now TIMESTAMPTZ := clock_timestamp();
  v_terminal BOOLEAN;
BEGIN
  IF p_error_code !~ '^[A-Z][A-Z0-9_]{0,63}$'
    OR p_available_at < v_now OR p_terminal IS NULL
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L3_PERSONA_EMBEDDING_RETRY_INVALID';
  END IF;
  SELECT * INTO v_job FROM user_memory_persona_embedding_jobs job
  WHERE job.job_id = p_job_id AND job.status = 'processing'
    AND job.lease_owner = p_worker_id AND job.lease_token = p_lease_token
    AND job.lease_expires_at > v_now FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_L3_PERSONA_EMBEDDING_LEASE_LOST';
  END IF;
  v_terminal := p_terminal OR v_job.attempt_count >= v_job.max_attempts;
  UPDATE user_memory_persona_embedding_jobs job
  SET status = CASE WHEN v_terminal THEN 'dead_letter' ELSE 'pending' END,
      provider_record_id = NULL, provider_config_updated_at = NULL,
      lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
      completed_at = CASE WHEN v_terminal THEN v_now ELSE NULL END,
      error_code = CASE WHEN v_terminal THEN p_error_code ELSE NULL END,
      available_at = CASE WHEN v_terminal THEN job.available_at ELSE p_available_at END,
      updated_at = v_now WHERE job.job_id = p_job_id;
  IF v_terminal THEN
    UPDATE user_memory_persona_search_projections projection
    SET embedding_status = 'failed', embedding_vector = NULL,
      embedding_error_code = p_error_code, embedding_updated_at = v_now,
      updated_at = v_now
    WHERE projection.entity_type = v_job.entity_type
      AND projection.entity_id = v_job.entity_id
      AND projection.entity_revision = v_job.entity_revision
      AND projection.content_hash = v_job.content_hash
      AND projection.source_watermark = v_job.source_watermark
      AND projection.generation = v_job.generation
      AND projection.embedding_status = 'pending';
  END IF;
  RETURN CASE WHEN v_terminal THEN 'dead_letter' ELSE 'pending' END;
END
$function$;

CREATE FUNCTION memory_l3_persona_version_current(
  p_persona_id UUID,
  p_user_id UUID,
  p_generation BIGINT,
  p_require_active BOOLEAN
) RETURNS BOOLEAN
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  SELECT EXISTS (
    SELECT 1
    FROM user_memory_persona_versions persona
    JOIN user_memory_state state
      ON state.user_id = persona.user_id
     AND state.visibility_epoch = persona.visibility_epoch
     AND state.active_l3_generation = persona.generation
    JOIN user_memory_settings settings ON settings.user_id = persona.user_id
    WHERE persona.id = p_persona_id
      AND persona.user_id = p_user_id
      AND persona.generation = p_generation
      AND persona.deleted_at IS NULL
      AND persona.lifecycle_status IN ('shadow', 'active')
      AND (NOT p_require_active OR persona.lifecycle_status = 'active')
      AND persona.profile_id = 'memory_l3_persona_v1'
      AND persona.token_count BETWEEN 1 AND 300
      AND persona.token_count = memory_l3_persona_estimated_tokens(persona.content)
      AND persona.content_hash = encode(sha256(convert_to(persona.content, 'UTF8')), 'hex')
      AND persona.source_watermark = memory_l3_persona_source_watermark(persona.user_id)
      AND (persona.sensitivity = 'normal' OR settings.sensitive_memory_enabled)
      AND (SELECT count(*) FROM user_memory_persona_members member
        WHERE member.persona_id = persona.id) BETWEEN 2 AND 50
      AND NOT EXISTS (
        SELECT 1 FROM user_memory_persona_members member
        LEFT JOIN user_memories memory
          ON memory.id = member.memory_id AND memory.user_id = member.user_id
        WHERE member.persona_id = persona.id AND (
          memory.id IS NULL OR memory.deleted_at IS NOT NULL OR NOT memory.enabled
          OR memory.lifecycle_status <> 'active'
          OR memory.scope_type <> 'global'
          OR memory.project_id IS NOT NULL
          OR memory.scope_conversation_id IS NOT NULL
          OR memory.scope_generation <> 1
          OR memory.memory_type NOT IN (
            'fact', 'preference', 'instruction', 'warning', 'decision'
          )
          OR memory.revision <> member.memory_revision
          OR memory.content_hash <> member.memory_content_hash
          OR memory.visibility_epoch <> persona.visibility_epoch
          OR (memory.valid_from IS NOT NULL AND memory.valid_from > statement_timestamp())
          OR (memory.valid_to IS NOT NULL AND memory.valid_to <= statement_timestamp())
          OR (memory.expires_at IS NOT NULL AND memory.expires_at <= statement_timestamp())
          OR memory_governance_classify_sensitivity(memory.content) = 'secret'
          OR (memory.sensitivity = 'sensitive'
            AND NOT settings.sensitive_memory_enabled)
        )
      )
  )
$function$;

CREATE FUNCTION memory_l3_persona_reader_authority(
  p_user_id UUID,
  p_conversation_id UUID,
  p_assistant_message_id UUID,
  p_active_requested BOOLEAN
) RETURNS TABLE (
  allowed BOOLEAN,
  mode TEXT,
  profile_id TEXT,
  retrieval_profile_id TEXT,
  generation BIGINT
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_conversation conversations%ROWTYPE;
  v_profile memory_l3_persona_profiles%ROWTYPE;
  v_state user_memory_state%ROWTYPE;
  v_settings user_memory_settings%ROWTYPE;
  v_effective_use BOOLEAN;
  v_active BOOLEAN;
BEGIN
  IF p_user_id IS NULL OR p_conversation_id IS NULL
    OR p_assistant_message_id IS NULL OR p_active_requested IS NULL
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L3_PERSONA_READER_ARGUMENT_INVALID';
  END IF;
  SELECT conversation.* INTO v_conversation FROM conversations conversation
  WHERE conversation.id = p_conversation_id AND conversation.user_id = p_user_id
    AND conversation.deleted_at IS NULL;
  IF NOT FOUND OR NOT EXISTS (
    SELECT 1 FROM messages assistant
    JOIN messages source
      ON source.id = assistant.parent_message_id
     AND source.user_id = assistant.user_id
     AND source.conversation_id = assistant.conversation_id
     AND source.role = 'user' AND source.status = 'completed'
     AND source.deleted_at IS NULL
    WHERE assistant.id = p_assistant_message_id
      AND assistant.user_id = p_user_id
      AND assistant.conversation_id = p_conversation_id
      AND assistant.role = 'assistant'
      AND assistant.status IN ('pending', 'streaming')
      AND assistant.deleted_at IS NULL
  ) THEN
    RETURN QUERY SELECT false, 'disabled'::TEXT, 'memory_l3_persona_v1'::TEXT,
      'memory_l3_persona_hybrid_bge_m3_rrf60_v1'::TEXT, 0::BIGINT;
    RETURN;
  END IF;
  SELECT profile.* INTO v_profile FROM memory_l3_persona_profiles profile
  WHERE profile.profile_id = 'memory_l3_persona_v1';
  SELECT * INTO v_state FROM user_memory_state WHERE user_id = p_user_id;
  SELECT * INTO v_settings FROM user_memory_settings WHERE user_id = p_user_id;
  v_effective_use := COALESCE(v_settings.enabled, false) AND
    CASE v_conversation.memory_use_mode
      WHEN 'on' THEN true
      WHEN 'off' THEN false
      ELSE COALESCE(v_settings.search_enabled, true)
    END;
  IF v_profile.profile_id IS NULL OR v_state.user_id IS NULL
    OR v_settings.user_id IS NULL OR NOT v_effective_use
    OR v_settings.l3_mode = 'off'
  THEN
    RETURN QUERY SELECT false, 'disabled'::TEXT, 'memory_l3_persona_v1'::TEXT,
      'memory_l3_persona_hybrid_bge_m3_rrf60_v1'::TEXT,
      COALESCE(v_state.active_l3_generation, 0);
    RETURN;
  END IF;
  v_active := v_profile.lifecycle_status = 'active'
    AND v_state.active_retrieval_profile_id = 'memory_hybrid_bge_m3_rrf60_v1';
  IF p_active_requested AND NOT v_active THEN
    RETURN QUERY SELECT false, 'disabled'::TEXT, v_profile.profile_id,
      v_profile.retrieval_profile_id, v_state.active_l3_generation;
    RETURN;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM user_memory_persona_versions persona
    WHERE persona.user_id = p_user_id
      AND persona.generation = v_state.active_l3_generation
      AND memory_l3_persona_version_current(
        persona.id, p_user_id, v_state.active_l3_generation, p_active_requested
      )
  ) THEN
    RETURN QUERY SELECT false,
      CASE WHEN p_active_requested THEN 'active' ELSE 'shadow' END,
      v_profile.profile_id, v_profile.retrieval_profile_id,
      v_state.active_l3_generation;
    RETURN;
  END IF;
  RETURN QUERY SELECT true,
    CASE WHEN p_active_requested THEN 'active' ELSE 'shadow' END,
    v_profile.profile_id, v_profile.retrieval_profile_id,
    v_state.active_l3_generation;
END
$function$;

CREATE FUNCTION memory_prepare_l3_persona_search(
  p_observation_id UUID,
  p_user_id UUID,
  p_conversation_id UUID,
  p_assistant_message_id UUID,
  p_query_hash TEXT,
  p_query_text TEXT,
  p_query_embedding REAL[],
  p_query_embedding_status TEXT,
  p_active_requested BOOLEAN
) RETURNS TABLE (
  observation_id UUID,
  mode TEXT,
  profile_id TEXT,
  generation BIGINT,
  status TEXT,
  result_code TEXT,
  replayed BOOLEAN,
  exact_count INTEGER,
  bm25_count INTEGER,
  vector_count INTEGER,
  rrf_count INTEGER,
  fallback_code TEXT,
  candidates JSONB
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_authority RECORD;
  v_existing message_memory_l3_persona_observations%ROWTYPE;
  v_query_terms TEXT[];
  v_bm25_query TEXT;
  v_fallback TEXT := 'NONE';
  v_norm DOUBLE PRECISION;
  v_exact_count INTEGER := 0;
  v_bm25_count INTEGER := 0;
  v_vector_count INTEGER := 0;
  v_rrf_count INTEGER := 0;
  v_candidates JSONB := '[]'::JSONB;
BEGIN
  IF p_observation_id IS NULL OR p_query_hash !~ '^[0-9a-f]{64}$'
    OR p_query_text IS NULL OR octet_length(p_query_text) NOT BETWEEN 1 AND 12000
    OR p_query_hash <> encode(sha256(convert_to(p_query_text, 'UTF8')), 'hex')
    OR p_query_embedding_status NOT IN ('ready','failed','unavailable','cutoff','redacted')
    OR (p_query_embedding_status = 'ready'
      AND (p_query_embedding IS NULL OR cardinality(p_query_embedding) <> 1024))
    OR (p_query_embedding_status <> 'ready' AND p_query_embedding IS NOT NULL)
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L3_PERSONA_SEARCH_ARGUMENT_INVALID';
  END IF;
  IF p_query_embedding_status = 'ready' THEN
    SELECT sqrt(sum(component::DOUBLE PRECISION * component::DOUBLE PRECISION))
    INTO v_norm FROM unnest(p_query_embedding) component;
    IF v_norm IS NULL OR v_norm <= 0 OR v_norm > 1e100 THEN
      RAISE EXCEPTION USING ERRCODE = '22023',
        MESSAGE = 'MEMORY_L3_PERSONA_SEARCH_VECTOR_INVALID';
    END IF;
  ELSE
    v_fallback := CASE p_query_embedding_status
      WHEN 'failed' THEN 'QUERY_EMBEDDING_FAILED'
      WHEN 'unavailable' THEN 'PROVIDER_UNAVAILABLE'
      WHEN 'cutoff' THEN 'HARD_CUTOFF'
      ELSE 'SECRET_REDACTED' END;
  END IF;
  SELECT * INTO v_authority FROM memory_l3_persona_reader_authority(
    p_user_id, p_conversation_id, p_assistant_message_id, p_active_requested
  );
  IF NOT COALESCE(v_authority.allowed, false) THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_L3_PERSONA_READER_DISABLED';
  END IF;

  PERFORM pg_advisory_xact_lock(
    hashtext('memory_l3_persona_search'), hashtext(p_assistant_message_id::TEXT)
  );
  SELECT * INTO v_existing FROM message_memory_l3_persona_observations
  WHERE assistant_message_id = p_assistant_message_id FOR UPDATE;
  IF FOUND THEN
    IF v_existing.user_id <> p_user_id
      OR v_existing.conversation_id <> p_conversation_id
      OR v_existing.query_sha256 <> p_query_hash
      OR v_existing.mode <> v_authority.mode
      OR v_existing.generation <> v_authority.generation
    THEN
      RAISE EXCEPTION USING ERRCODE = '40001',
        MESSAGE = 'MEMORY_L3_PERSONA_SEARCH_REPLAY_CONFLICT';
    END IF;
    IF v_existing.status <> 'pending' THEN
      RETURN QUERY SELECT v_existing.id, v_existing.mode,
        v_existing.profile_id, v_existing.generation, v_existing.status,
        v_existing.result_code, true, v_existing.exact_count::INTEGER,
        v_existing.bm25_count::INTEGER, v_existing.vector_count::INTEGER,
        v_existing.rrf_count::INTEGER, v_existing.fallback_code, '[]'::JSONB;
      RETURN;
    END IF;
    p_observation_id := v_existing.id;
    DELETE FROM message_memory_l3_persona_results result
    WHERE result.observation_id = p_observation_id;
  ELSE
    INSERT INTO message_memory_l3_persona_observations(
      id, assistant_message_id, user_id, conversation_id, mode, profile_id,
      retrieval_profile_id, generation, query_sha256, status, result_code,
      query_embedding_status, rerank_status, fallback_code
    ) VALUES (
      p_observation_id, p_assistant_message_id, p_user_id, p_conversation_id,
      v_authority.mode, v_authority.profile_id,
      'memory_l3_persona_hybrid_bge_m3_rrf60_v1', v_authority.generation,
      p_query_hash, 'pending', 'PENDING', p_query_embedding_status,
      'pending', v_fallback
    );
  END IF;
  v_query_terms := knowledge_bm25_shadow_query_terms(p_query_text);
  v_bm25_query := knowledge_build_bm25_shadow_text(p_query_text, v_query_terms);

  WITH authorized AS MATERIALIZED (
    SELECT projection.*, persona.content, persona.updated_at AS persona_updated_at
    FROM user_memory_persona_search_projections projection
    JOIN user_memory_persona_versions persona
      ON persona.id = projection.entity_id AND persona.user_id = projection.user_id
     AND persona.revision = projection.entity_revision
     AND persona.content_hash = projection.content_hash
     AND persona.source_watermark = projection.source_watermark
     AND persona.visibility_epoch = projection.visibility_epoch
     AND persona.generation = projection.generation
    WHERE persona.user_id = p_user_id
      AND projection.generation = v_authority.generation
      AND projection.retrieval_profile_id = v_authority.retrieval_profile_id
      AND projection.lexical_status = 'ready'
      AND memory_l3_persona_version_current(
        persona.id, p_user_id, v_authority.generation,
        v_authority.mode = 'active'
      )
  ), exact_ranked AS MATERIALIZED (
    SELECT candidate.*, row_number() OVER (ORDER BY
      (SELECT count(*) FROM unnest(candidate.exact_terms) term
        WHERE term = ANY(v_query_terms)) DESC,
      candidate.persona_updated_at DESC, candidate.entity_id)::INTEGER lane_rank
    FROM authorized candidate
    WHERE cardinality(v_query_terms) > 0 AND candidate.exact_terms && v_query_terms
    LIMIT 5
  ), bm25_probe AS MATERIALIZED (
    SELECT candidate.*, candidate.bm25_text <@> to_bm25query(
      v_bm25_query, 'idx_user_memory_persona_search_bm25'
    ) AS raw_score
    FROM authorized candidate
    WHERE candidate.bm25_text <@> to_bm25query(
      v_bm25_query, 'idx_user_memory_persona_search_bm25'
    ) < 0
    ORDER BY raw_score, candidate.entity_id LIMIT 5
  ), bm25_ranked AS MATERIALIZED (
    SELECT candidate.*, row_number() OVER (
      ORDER BY candidate.raw_score, candidate.persona_updated_at DESC,
        candidate.entity_id
    )::INTEGER lane_rank FROM bm25_probe candidate
  ), lanes AS (
    SELECT 'exact'::TEXT lane, lane_rank, entity_id, entity_revision
    FROM exact_ranked WHERE lane_rank <= 5
    UNION ALL
    SELECT 'bm25', lane_rank, entity_id, entity_revision FROM bm25_ranked
  )
  INSERT INTO message_memory_l3_persona_results(
    observation_id, user_id, lane, ordinal, persona_id, persona_revision
  ) SELECT p_observation_id, p_user_id, lane, lane_rank::SMALLINT,
      entity_id, entity_revision FROM lanes ORDER BY lane, lane_rank;

  IF p_query_embedding_status = 'ready' THEN
    WITH vector_ranked AS (
      SELECT projection.entity_id, projection.entity_revision,
        row_number() OVER (ORDER BY
          projection.embedding_vector <=> p_query_embedding::vector(1024),
          projection.entity_id)::INTEGER lane_rank
      FROM user_memory_persona_search_projections projection
      JOIN user_memory_persona_versions persona
        ON persona.id = projection.entity_id AND persona.user_id = projection.user_id
       AND persona.revision = projection.entity_revision
       AND persona.content_hash = projection.content_hash
       AND persona.source_watermark = projection.source_watermark
       AND persona.visibility_epoch = projection.visibility_epoch
       AND persona.generation = projection.generation
      WHERE persona.user_id = p_user_id
        AND projection.generation = v_authority.generation
        AND projection.retrieval_profile_id = v_authority.retrieval_profile_id
        AND projection.embedding_status = 'ready'
        AND projection.embedding_vector IS NOT NULL
        AND memory_l3_persona_version_current(
          persona.id, p_user_id, v_authority.generation,
          v_authority.mode = 'active'
        )
      ORDER BY projection.embedding_vector <=> p_query_embedding::vector(1024),
        projection.entity_id LIMIT 5
    )
    INSERT INTO message_memory_l3_persona_results(
      observation_id, user_id, lane, ordinal, persona_id, persona_revision
    ) SELECT p_observation_id, p_user_id, 'vector', lane_rank::SMALLINT,
      entity_id, entity_revision FROM vector_ranked ORDER BY lane_rank;
  END IF;

  WITH scores AS MATERIALIZED (
    SELECT result.persona_id, min(result.persona_revision) persona_revision,
      sum(1.0 / (60.0 + result.ordinal::DOUBLE PRECISION)) rrf_score,
      min(result.ordinal) FILTER (WHERE result.lane = 'exact') exact_rank,
      min(result.ordinal) FILTER (WHERE result.lane = 'bm25') bm25_rank,
      min(result.ordinal) FILTER (WHERE result.lane = 'vector') vector_rank
    FROM message_memory_l3_persona_results result
    WHERE result.observation_id = p_observation_id
      AND result.lane IN ('exact','bm25','vector') GROUP BY result.persona_id
  ), ranked AS (
    SELECT score.*, row_number() OVER (ORDER BY score.rrf_score DESC,
      (score.exact_rank IS NOT NULL) DESC, COALESCE(score.exact_rank,32767),
      COALESCE(score.bm25_rank,32767), COALESCE(score.vector_rank,32767),
      score.persona_id)::INTEGER lane_rank FROM scores score
  )
  INSERT INTO message_memory_l3_persona_results(
    observation_id, user_id, lane, ordinal, persona_id, persona_revision
  ) SELECT p_observation_id, p_user_id, 'rrf', lane_rank::SMALLINT,
    persona_id, persona_revision FROM ranked WHERE lane_rank <= 5 ORDER BY lane_rank;

  SELECT count(*) INTO v_exact_count FROM message_memory_l3_persona_results result
    WHERE result.observation_id = p_observation_id AND result.lane = 'exact';
  SELECT count(*) INTO v_bm25_count FROM message_memory_l3_persona_results result
    WHERE result.observation_id = p_observation_id AND result.lane = 'bm25';
  SELECT count(*) INTO v_vector_count FROM message_memory_l3_persona_results result
    WHERE result.observation_id = p_observation_id AND result.lane = 'vector';
  SELECT count(*) INTO v_rrf_count FROM message_memory_l3_persona_results result
    WHERE result.observation_id = p_observation_id AND result.lane = 'rrf';
  SELECT COALESCE(jsonb_agg(jsonb_build_object(
      'personaId', result.persona_id::TEXT,
      'revision', result.persona_revision,
      'content', persona.content
    ) ORDER BY result.ordinal), '[]'::JSONB)
  INTO v_candidates
  FROM message_memory_l3_persona_results result
  JOIN user_memory_persona_versions persona
    ON persona.id = result.persona_id AND persona.user_id = result.user_id
   AND persona.revision = result.persona_revision
  WHERE result.observation_id = p_observation_id AND result.lane = 'rrf';
  UPDATE message_memory_l3_persona_observations observation
  SET result_code = 'CANDIDATES_READY', fallback_code = v_fallback,
    exact_count = v_exact_count, bm25_count = v_bm25_count,
    vector_count = v_vector_count, rrf_count = v_rrf_count,
    updated_at = clock_timestamp()
  WHERE observation.id = p_observation_id;
  RETURN QUERY SELECT p_observation_id, v_authority.mode,
    v_authority.profile_id, v_authority.generation, 'pending'::TEXT,
    'CANDIDATES_READY'::TEXT, false, v_exact_count, v_bm25_count,
    v_vector_count, v_rrf_count, v_fallback, v_candidates;
END
$function$;

CREATE FUNCTION memory_record_l3_persona_search(
  p_observation_id UUID,
  p_user_id UUID,
  p_assistant_message_id UUID,
  p_rerank_status TEXT,
  p_fallback_code TEXT,
  p_reranked JSONB,
  p_final JSONB,
  p_estimated_tokens INTEGER,
  p_duration_millis INTEGER
) RETURNS TABLE (
  observation_id UUID,
  mode TEXT,
  profile_id TEXT,
  generation BIGINT,
  status TEXT,
  result_code TEXT,
  rerank_count INTEGER,
  final_count INTEGER,
  injected_count INTEGER,
  estimated_tokens INTEGER,
  fallback_code TEXT,
  duration_millis INTEGER,
  final_personas JSONB
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_observation message_memory_l3_persona_observations%ROWTYPE;
  v_authority RECORD;
  v_result_hash TEXT;
  v_rerank_count INTEGER;
  v_final_count INTEGER;
  v_injected_count INTEGER;
  v_recomputed_tokens INTEGER := 0;
  v_final_personas JSONB := '[]'::JSONB;
BEGIN
  IF p_rerank_status NOT IN ('applied','fallback','skipped')
    OR p_fallback_code !~ '^[A-Z][A-Z0-9_]{0,127}$'
    OR p_reranked IS NULL OR jsonb_typeof(p_reranked) <> 'array'
    OR jsonb_array_length(p_reranked) > 5
    OR p_final IS NULL OR jsonb_typeof(p_final) <> 'array'
    OR jsonb_array_length(p_final) > 1
    OR p_estimated_tokens NOT BETWEEN 0 AND 300
    OR p_duration_millis NOT BETWEEN 0 AND 120000
    OR (p_rerank_status = 'applied' AND jsonb_array_length(p_reranked) = 0)
    OR (p_rerank_status <> 'applied' AND jsonb_array_length(p_reranked) <> 0)
    OR EXISTS (
      SELECT 1 FROM jsonb_array_elements(p_reranked || p_final) item
      WHERE jsonb_typeof(item) <> 'object'
        OR ARRAY(SELECT key FROM jsonb_object_keys(item) key ORDER BY key)
          <> ARRAY['personaId','revision']::TEXT[]
        OR COALESCE(item->>'personaId','') !~
          '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
        OR COALESCE(item->>'revision','') !~ '^[1-9][0-9]*$'
    )
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L3_PERSONA_SEARCH_RESULT_INVALID';
  END IF;
  IF EXISTS (
    SELECT item->>'personaId' FROM jsonb_array_elements(p_reranked) item
    GROUP BY item->>'personaId' HAVING count(*) > 1
  ) OR EXISTS (
    SELECT item->>'personaId' FROM jsonb_array_elements(p_final) item
    GROUP BY item->>'personaId' HAVING count(*) > 1
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L3_PERSONA_SEARCH_RESULT_INVALID';
  END IF;
  v_result_hash := encode(sha256(convert_to(jsonb_build_object(
    'rerankStatus', p_rerank_status, 'fallbackCode', p_fallback_code,
    'reranked', p_reranked, 'final', p_final,
    'estimatedTokens', p_estimated_tokens
  )::TEXT, 'UTF8')), 'hex');
  SELECT * INTO v_observation FROM message_memory_l3_persona_observations observation
  WHERE observation.id = p_observation_id
    AND observation.user_id = p_user_id
    AND observation.assistant_message_id = p_assistant_message_id
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_L3_PERSONA_SEARCH_NOT_FOUND';
  END IF;
  IF v_observation.status <> 'pending' THEN
    IF v_observation.result_sha256 <> v_result_hash THEN
      RAISE EXCEPTION USING ERRCODE = '40001',
        MESSAGE = 'MEMORY_L3_PERSONA_SEARCH_REPLAY_CONFLICT';
    END IF;
    IF v_observation.mode = 'active' THEN
      SELECT COALESCE(jsonb_agg(jsonb_build_object(
        'personaId', result.persona_id::TEXT,
        'revision', result.persona_revision,
        'content', persona.content
      ) ORDER BY result.ordinal), '[]'::JSONB)
      INTO v_final_personas
      FROM message_memory_l3_persona_results result
      JOIN user_memory_persona_versions persona
        ON persona.id = result.persona_id AND persona.user_id = result.user_id
       AND persona.revision = result.persona_revision
      WHERE result.observation_id = v_observation.id AND result.lane = 'final'
        AND memory_l3_persona_version_current(
          persona.id, p_user_id, v_observation.generation, true
        );
    END IF;
    RETURN QUERY SELECT v_observation.id, v_observation.mode,
      v_observation.profile_id, v_observation.generation,
      v_observation.status, v_observation.result_code,
      v_observation.rerank_count::INTEGER, v_observation.final_count::INTEGER,
      v_observation.injected_count::INTEGER,
      v_observation.estimated_tokens::INTEGER, v_observation.fallback_code,
      v_observation.duration_millis, v_final_personas;
    RETURN;
  END IF;
  SELECT * INTO v_authority FROM memory_l3_persona_reader_authority(
    p_user_id, v_observation.conversation_id, p_assistant_message_id,
    v_observation.mode = 'active'
  );
  IF NOT COALESCE(v_authority.allowed, false)
    OR v_authority.mode <> v_observation.mode
    OR v_authority.generation <> v_observation.generation
  THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_L3_PERSONA_SEARCH_RESULT_STALE';
  END IF;
  IF EXISTS (
    SELECT 1 FROM jsonb_array_elements(p_reranked || p_final) item
    LEFT JOIN message_memory_l3_persona_results result
      ON result.observation_id = p_observation_id AND result.lane = 'rrf'
     AND result.persona_id = (item->>'personaId')::UUID
     AND result.persona_revision = (item->>'revision')::BIGINT
    WHERE result.persona_id IS NULL
  ) OR EXISTS (
    SELECT 1 FROM jsonb_array_elements(p_final) item
    LEFT JOIN jsonb_array_elements(p_reranked) reranked(payload)
      ON reranked.payload->>'personaId' = item->>'personaId'
     AND reranked.payload->>'revision' = item->>'revision'
    WHERE jsonb_array_length(p_reranked) > 0 AND reranked.payload IS NULL
  ) OR EXISTS (
    SELECT 1 FROM jsonb_array_elements(p_reranked || p_final) item
    WHERE NOT memory_l3_persona_version_current(
      (item->>'personaId')::UUID, p_user_id, v_observation.generation,
      v_observation.mode = 'active'
    )
  ) THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_L3_PERSONA_SEARCH_RESULT_STALE';
  END IF;
  SELECT COALESCE(sum(persona.token_count), 0)::INTEGER
  INTO v_recomputed_tokens
  FROM jsonb_array_elements(p_final) item
  JOIN user_memory_persona_versions persona
    ON persona.id = (item->>'personaId')::UUID
   AND persona.user_id = p_user_id
   AND persona.revision = (item->>'revision')::BIGINT;
  IF v_recomputed_tokens <> p_estimated_tokens OR v_recomputed_tokens > 300 THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L3_PERSONA_TOKEN_COUNT_INVALID';
  END IF;

  DELETE FROM message_memory_l3_persona_results result
  WHERE result.observation_id = p_observation_id
    AND result.lane IN ('rerank','final');
  INSERT INTO message_memory_l3_persona_results(
    observation_id, user_id, lane, ordinal, persona_id, persona_revision
  ) SELECT p_observation_id, p_user_id, 'rerank', ordinal::SMALLINT,
      (item->>'personaId')::UUID, (item->>'revision')::BIGINT
    FROM jsonb_array_elements(p_reranked) WITH ORDINALITY value(item, ordinal)
    ORDER BY ordinal;
  INSERT INTO message_memory_l3_persona_results(
    observation_id, user_id, lane, ordinal, persona_id, persona_revision
  ) SELECT p_observation_id, p_user_id, 'final', ordinal::SMALLINT,
      (item->>'personaId')::UUID, (item->>'revision')::BIGINT
    FROM jsonb_array_elements(p_final) WITH ORDINALITY value(item, ordinal)
    ORDER BY ordinal;
  v_rerank_count := jsonb_array_length(p_reranked);
  v_final_count := jsonb_array_length(p_final);
  v_injected_count := CASE WHEN v_observation.mode = 'active'
    THEN v_final_count ELSE 0 END;
  UPDATE message_memory_l3_persona_observations observation
  SET result_sha256 = v_result_hash, status = 'completed',
      result_code = CASE WHEN v_final_count > 0 THEN 'COMPLETED' ELSE 'NO_MATCH' END,
      rerank_status = p_rerank_status, fallback_code = p_fallback_code,
      rerank_count = v_rerank_count, final_count = v_final_count,
      injected_count = v_injected_count, estimated_tokens = v_recomputed_tokens,
      duration_millis = p_duration_millis, updated_at = clock_timestamp()
  WHERE observation.id = p_observation_id
  RETURNING observation.* INTO v_observation;
  IF v_observation.mode = 'active' THEN
    SELECT COALESCE(jsonb_agg(jsonb_build_object(
      'personaId', result.persona_id::TEXT,
      'revision', result.persona_revision,
      'content', persona.content
    ) ORDER BY result.ordinal), '[]'::JSONB)
    INTO v_final_personas
    FROM message_memory_l3_persona_results result
    JOIN user_memory_persona_versions persona
      ON persona.id = result.persona_id AND persona.user_id = result.user_id
     AND persona.revision = result.persona_revision
    WHERE result.observation_id = p_observation_id AND result.lane = 'final'
      AND memory_l3_persona_version_current(
        persona.id, p_user_id, v_observation.generation, true
      );
  END IF;
  RETURN QUERY SELECT v_observation.id, v_observation.mode,
    v_observation.profile_id, v_observation.generation,
    v_observation.status, v_observation.result_code,
    v_observation.rerank_count::INTEGER, v_observation.final_count::INTEGER,
    v_observation.injected_count::INTEGER,
    v_observation.estimated_tokens::INTEGER, v_observation.fallback_code,
    v_observation.duration_millis, v_final_personas;
END
$function$;

CREATE FUNCTION memory_governance_l3_persona_json(
  p_user_id UUID,
  p_persona_id UUID
) RETURNS JSONB
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  SELECT jsonb_strip_nulls(jsonb_build_object(
    'id', persona.id::TEXT,
    'content', CASE WHEN persona.deleted_at IS NULL THEN persona.content ELSE '' END,
    'contentHash', persona.content_hash,
    'tokenCount', persona.token_count,
    'sensitivity', persona.sensitivity,
    'sensitiveInputIncluded', persona.sensitive_input_included,
    'status', persona.lifecycle_status,
    'userDisabled', persona.user_disabled,
    'profileId', persona.profile_id,
    'generation', persona.generation,
    'sourceWatermark', persona.source_watermark,
    'revision', persona.revision,
    'memberCount', (SELECT count(*) FROM user_memory_persona_members member
      WHERE member.persona_id = persona.id),
    'sourcesCurrent', persona.source_watermark =
      memory_l3_persona_source_watermark(persona.user_id)
      AND NOT EXISTS (
        SELECT 1 FROM user_memory_persona_members member
        LEFT JOIN user_memories memory
          ON memory.id = member.memory_id AND memory.user_id = member.user_id
        LEFT JOIN user_memory_settings settings ON settings.user_id = persona.user_id
        WHERE member.persona_id = persona.id AND (
          memory.id IS NULL OR memory.deleted_at IS NOT NULL OR NOT memory.enabled
          OR memory.lifecycle_status <> 'active'
          OR memory.scope_type <> 'global'
          OR memory.project_id IS NOT NULL
          OR memory.scope_conversation_id IS NOT NULL
          OR memory.scope_generation <> 1
          OR memory.memory_type NOT IN (
            'fact', 'preference', 'instruction', 'warning', 'decision'
          )
          OR memory.revision <> member.memory_revision
          OR memory.content_hash <> member.memory_content_hash
          OR memory.visibility_epoch <> persona.visibility_epoch
          OR (memory.valid_from IS NOT NULL AND memory.valid_from > now())
          OR (memory.valid_to IS NOT NULL AND memory.valid_to <= now())
          OR (memory.expires_at IS NOT NULL AND memory.expires_at <= now())
          OR memory_governance_classify_sensitivity(memory.content) = 'secret'
          OR (memory.sensitivity = 'sensitive'
            AND NOT COALESCE(settings.sensitive_memory_enabled, false))
        )
      ),
    'createdAt', memory_governance_epoch_millis(persona.created_at),
    'updatedAt', memory_governance_epoch_millis(persona.updated_at),
    'activatedAt', memory_governance_epoch_millis(persona.activated_at),
    'disabledAt', memory_governance_epoch_millis(persona.disabled_at),
    'staleAt', memory_governance_epoch_millis(persona.stale_at),
    'purgeAfter', memory_governance_epoch_millis(persona.purge_after)
  ))
  FROM user_memory_persona_versions persona
  WHERE persona.id = p_persona_id AND persona.user_id = p_user_id
    AND persona.deleted_at IS NULL
$function$;

CREATE FUNCTION memory_governance_l3_persona_snapshot(p_user_id UUID)
RETURNS JSONB
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  SELECT jsonb_build_object(
    'profile', jsonb_build_object(
      'profileId', profile.profile_id,
      'synthesisProfileId', profile.synthesis_profile_id,
      'retrievalProfileId', profile.retrieval_profile_id,
      'status', profile.lifecycle_status,
      'generation', COALESCE(state.active_l3_generation, 1),
      'l1ReaderReady', COALESCE(
        state.active_retrieval_profile_id = 'memory_hybrid_bge_m3_rrf60_v1', false
      ),
      'active', profile.lifecycle_status = 'active'
        AND COALESCE(state.active_retrieval_profile_id =
          'memory_hybrid_bge_m3_rrf60_v1', false),
      'activatedAt', memory_governance_epoch_millis(profile.activated_at),
      'rolledBackAt', memory_governance_epoch_millis(profile.rolled_back_at)
    ),
    'persona', (
      SELECT memory_governance_l3_persona_json(p_user_id, persona.id)
      FROM user_memory_persona_versions persona
      WHERE persona.user_id = p_user_id
        AND persona.generation = COALESCE(state.active_l3_generation, 1)
        AND persona.deleted_at IS NULL
      ORDER BY persona.updated_at DESC, persona.id
      LIMIT 1
    )
  )
  FROM memory_l3_persona_profiles profile
  LEFT JOIN user_memory_state state ON state.user_id = p_user_id
  WHERE profile.profile_id = 'memory_l3_persona_v1'
$function$;

CREATE FUNCTION memory_governance_l3_persona_detail(
  p_user_id UUID,
  p_persona_id UUID
) RETURNS JSONB
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_persona user_memory_persona_versions%ROWTYPE;
  v_sensitive BOOLEAN;
  v_members JSONB;
BEGIN
  SELECT * INTO v_persona FROM user_memory_persona_versions persona
  WHERE persona.id = p_persona_id AND persona.user_id = p_user_id
    AND persona.deleted_at IS NULL;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_L3_PERSONA_NOT_FOUND';
  END IF;
  SELECT sensitive_memory_enabled INTO v_sensitive
  FROM user_memory_settings WHERE user_id = p_user_id;
  SELECT COALESCE(jsonb_agg(jsonb_build_object(
    'memoryId', member.memory_id::TEXT,
    'revision', member.memory_revision,
    'contentHash', member.memory_content_hash,
    'current', authority.is_current,
    'sourceDeleted', NOT authority.is_current,
    'memory', CASE WHEN authority.is_current
      THEN memory_governance_memory_json(p_user_id, memory.id) ELSE NULL END,
    'evidence', CASE WHEN authority.is_current
      THEN COALESCE((
        SELECT jsonb_agg(jsonb_build_object(
          'messageId', evidence.source_message_id::TEXT,
          'conversationId', evidence.source_conversation_id::TEXT,
          'role', evidence.evidence_role,
          'sourceDeleted', source.id IS NULL OR conversation.id IS NULL,
          'sourceExcerpt', CASE WHEN source.id IS NOT NULL
            AND conversation.id IS NOT NULL THEN left(source.content, 280) ELSE NULL END,
          'observedAt', memory_governance_epoch_millis(evidence.observed_at)
        ) ORDER BY evidence.observed_at, evidence.source_message_id)
        FROM user_memory_evidence evidence
        LEFT JOIN messages source
          ON source.id = evidence.source_message_id
         AND source.user_id = evidence.user_id AND source.deleted_at IS NULL
        LEFT JOIN conversations conversation
          ON conversation.id = evidence.source_conversation_id
         AND conversation.user_id = evidence.user_id
         AND conversation.deleted_at IS NULL
        WHERE evidence.memory_id = memory.id AND evidence.user_id = p_user_id
      ), '[]'::JSONB) ELSE '[]'::JSONB END
  ) ORDER BY member.memory_id), '[]'::JSONB)
  INTO v_members
  FROM user_memory_persona_members member
  LEFT JOIN user_memories memory
    ON memory.id = member.memory_id AND memory.user_id = member.user_id
  CROSS JOIN LATERAL (
    SELECT memory.id IS NOT NULL
      AND memory.deleted_at IS NULL AND memory.enabled
      AND memory.lifecycle_status = 'active'
      AND memory.scope_type = 'global'
      AND memory.project_id IS NULL
      AND memory.scope_conversation_id IS NULL
      AND memory.scope_generation = 1
      AND memory.memory_type IN (
        'fact', 'preference', 'instruction', 'warning', 'decision'
      )
      AND memory.revision = member.memory_revision
      AND memory.content_hash = member.memory_content_hash
      AND memory.visibility_epoch = v_persona.visibility_epoch
      AND (memory.valid_from IS NULL OR memory.valid_from <= now())
      AND (memory.valid_to IS NULL OR now() < memory.valid_to)
      AND (memory.expires_at IS NULL OR now() < memory.expires_at)
      AND memory_governance_classify_sensitivity(memory.content) <> 'secret'
      AND (memory.sensitivity = 'normal' OR v_sensitive)
      AS is_current
  ) authority
  WHERE member.persona_id = v_persona.id AND member.user_id = p_user_id;
  RETURN jsonb_build_object(
    'persona', memory_governance_l3_persona_json(p_user_id, p_persona_id),
    'members', v_members
  );
END
$function$;

CREATE FUNCTION memory_governance_set_l3_persona_enabled(
  p_user_id UUID,
  p_persona_id UUID,
  p_expected_revision BIGINT,
  p_enabled BOOLEAN
) RETURNS JSONB
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_persona user_memory_persona_versions%ROWTYPE;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  IF p_expected_revision < 1 OR p_enabled IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L3_PERSONA_MUTATION_INVALID';
  END IF;
  SELECT * INTO v_persona FROM user_memory_persona_versions persona
  JOIN user_memory_state state
    ON state.user_id = persona.user_id
   AND state.active_l3_generation = persona.generation
  WHERE persona.id = p_persona_id AND persona.user_id = p_user_id
    AND persona.deleted_at IS NULL AND persona.lifecycle_status <> 'stale'
  FOR UPDATE OF persona;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_L3_PERSONA_NOT_FOUND';
  END IF;
  IF v_persona.revision <> p_expected_revision THEN
    RAISE EXCEPTION USING ERRCODE = '40001',
      MESSAGE = 'MEMORY_L3_PERSONA_REVISION_STALE';
  END IF;
  IF NOT p_enabled THEN
    UPDATE user_memory_persona_versions persona
    SET user_disabled = true, lifecycle_status = 'disabled',
      revision = persona.revision + 1, disabled_at = v_now,
      activated_at = persona.activated_at,
      stale_at = NULL, purge_after = NULL, updated_at = v_now
    WHERE persona.id = v_persona.id;
    DELETE FROM user_memory_persona_search_projections projection
    WHERE projection.entity_type = 'l3_persona'
      AND projection.entity_id = v_persona.id;
  ELSE
    UPDATE user_memory_persona_versions persona
    SET user_disabled = false, lifecycle_status = 'shadow',
      revision = persona.revision + 1, disabled_at = NULL,
      stale_at = NULL, purge_after = NULL, updated_at = v_now
    WHERE persona.id = v_persona.id;
    PERFORM memory_l3_persona_invalidate_user(p_user_id);
  END IF;
  RETURN memory_governance_l3_persona_json(p_user_id, p_persona_id);
END
$function$;

CREATE FUNCTION memory_governance_rebuild_l3_persona(
  p_user_id UUID,
  p_persona_id UUID
) RETURNS JSONB
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_generation BIGINT;
  v_job_id UUID;
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM user_memory_persona_versions persona
    WHERE persona.id = p_persona_id AND persona.user_id = p_user_id
      AND persona.deleted_at IS NULL
  ) THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_L3_PERSONA_NOT_FOUND';
  END IF;
  PERFORM memory_l3_persona_invalidate_user(p_user_id);
  SELECT active_l3_generation INTO v_generation FROM user_memory_state
  WHERE user_id = p_user_id;
  SELECT job_id INTO v_job_id FROM user_memory_persona_jobs
  WHERE dedupe_key = 'memory:l3:refresh:v1:' || p_user_id::TEXT;
  RETURN jsonb_build_object('jobId', v_job_id::TEXT, 'generation', v_generation);
END
$function$;

CREATE FUNCTION memory_governance_rebuild_l3_personas(p_user_id UUID)
RETURNS JSONB
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_generation BIGINT;
  v_job_id UUID;
BEGIN
  PERFORM memory_l3_persona_invalidate_user(p_user_id);
  SELECT active_l3_generation INTO v_generation FROM user_memory_state
  WHERE user_id = p_user_id;
  SELECT job_id INTO v_job_id FROM user_memory_persona_jobs
  WHERE dedupe_key = 'memory:l3:refresh:v1:' || p_user_id::TEXT;
  RETURN jsonb_build_object(
    'generation', v_generation, 'jobCount', CASE WHEN v_job_id IS NULL THEN 0 ELSE 1 END,
    'jobId', v_job_id::TEXT
  );
END
$function$;

CREATE FUNCTION memory_operator_promote_l3_persona(
  p_event_id UUID,
  p_benchmark JSONB,
  p_canary JSONB
) RETURNS JSONB
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_profile memory_l3_persona_profiles%ROWTYPE;
  v_benchmark_report JSONB;
  v_benchmark_hash TEXT;
  v_canary_hash TEXT;
  v_window_start TIMESTAMPTZ;
  v_window_end TIMESTAMPTZ;
  v_eligible INTEGER;
  v_reviewed INTEGER;
  v_benchmark_passed BOOLEAN;
  v_total_reviewed INTEGER;
  v_development_count INTEGER;
  v_validation_count INTEGER;
  v_holdout_count INTEGER;
  v_benchmark_consistency NUMERIC;
  v_benchmark_false_injection NUMERIC;
  v_benchmark_token_saving NUMERIC;
  v_p95_latency INTEGER;
  v_p99_latency INTEGER;
  v_maximum_prompt_tokens INTEGER;
  v_hard_cutoff_violations INTEGER;
  v_benchmark_cross_user_leaks INTEGER;
  v_benchmark_deleted_leaks INTEGER;
  v_benchmark_secret_leaks INTEGER;
  v_untrusted_source_leaks INTEGER;
  v_benchmark_unauthorized_egress INTEGER;
  v_canary_consistency NUMERIC;
  v_canary_false_injection NUMERIC;
  v_canary_token_saving NUMERIC;
  v_canary_cross_user_leaks INTEGER;
  v_canary_deleted_leaks INTEGER;
  v_canary_secret_leaks INTEGER;
  v_canary_unauthorized_egress INTEGER;
  v_db_count INTEGER;
  v_db_start TIMESTAMPTZ;
  v_db_end TIMESTAMPTZ;
  v_user_id UUID;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  IF p_event_id IS NULL OR p_benchmark IS NULL OR p_canary IS NULL
    OR jsonb_typeof(p_benchmark) <> 'object'
    OR jsonb_typeof(p_canary) <> 'object'
    OR ARRAY(SELECT key FROM jsonb_object_keys(p_benchmark) key ORDER BY key)
      <> ARRAY['report','reportSha256']::TEXT[]
    OR ARRAY(SELECT key FROM jsonb_object_keys(p_canary) key ORDER BY key)
      <> ARRAY['crossUserLeakCount','deletedMemoryLeakCount','eligibleTurns',
        'falseInjectionRate','personaConsistency','reportSha256','reviewedTurns',
        'secretLeakCount','tokenSavingRatio','unauthorizedProviderEgressCount',
        'windowEndedAt','windowStartedAt']::TEXT[]
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L3_PERSONA_PROMOTION_EVIDENCE_INVALID';
  END IF;
  v_benchmark_report := p_benchmark->'report';
  v_benchmark_hash := p_benchmark->>'reportSha256';
  v_canary_hash := p_canary->>'reportSha256';
  BEGIN
    v_window_start := (p_canary->>'windowStartedAt')::TIMESTAMPTZ;
    v_window_end := (p_canary->>'windowEndedAt')::TIMESTAMPTZ;
    v_eligible := (p_canary->>'eligibleTurns')::INTEGER;
    v_reviewed := (p_canary->>'reviewedTurns')::INTEGER;
    v_benchmark_passed := (v_benchmark_report->>'passed')::BOOLEAN;
    v_total_reviewed := (v_benchmark_report#>>'{golden,totalReviewed}')::INTEGER;
    v_development_count :=
      (v_benchmark_report#>>'{golden,developmentCount}')::INTEGER;
    v_validation_count :=
      (v_benchmark_report#>>'{golden,validationCount}')::INTEGER;
    v_holdout_count := (v_benchmark_report#>>'{golden,holdoutCount}')::INTEGER;
    v_benchmark_consistency :=
      (v_benchmark_report#>>'{profile,metrics,personaConsistency}')::NUMERIC;
    v_benchmark_false_injection :=
      (v_benchmark_report#>>'{profile,metrics,falseInjectionRate}')::NUMERIC;
    v_benchmark_token_saving :=
      (v_benchmark_report#>>'{profile,metrics,tokenSavingRatio}')::NUMERIC;
    v_p95_latency :=
      (v_benchmark_report#>>'{profile,budgets,p95LatencyMilliseconds}')::INTEGER;
    v_p99_latency :=
      (v_benchmark_report#>>'{profile,budgets,p99LatencyMilliseconds}')::INTEGER;
    v_maximum_prompt_tokens :=
      (v_benchmark_report#>>'{profile,budgets,maximumPromptMemoryTokens}')::INTEGER;
    v_hard_cutoff_violations :=
      (v_benchmark_report#>>'{profile,budgets,hardCutoffViolationCount}')::INTEGER;
    v_benchmark_cross_user_leaks :=
      (v_benchmark_report#>>'{profile,safety,crossUserLeakCount}')::INTEGER;
    v_benchmark_deleted_leaks :=
      (v_benchmark_report#>>'{profile,safety,deletedMemoryLeakCount}')::INTEGER;
    v_benchmark_secret_leaks :=
      (v_benchmark_report#>>'{profile,safety,secretLeakCount}')::INTEGER;
    v_untrusted_source_leaks :=
      (v_benchmark_report#>>'{profile,safety,untrustedSourceLeakCount}')::INTEGER;
    v_benchmark_unauthorized_egress :=
      (v_benchmark_report#>>'{profile,safety,unauthorizedProviderEgressCount}')::INTEGER;
    v_canary_consistency := (p_canary->>'personaConsistency')::NUMERIC;
    v_canary_false_injection := (p_canary->>'falseInjectionRate')::NUMERIC;
    v_canary_token_saving := (p_canary->>'tokenSavingRatio')::NUMERIC;
    v_canary_cross_user_leaks := (p_canary->>'crossUserLeakCount')::INTEGER;
    v_canary_deleted_leaks := (p_canary->>'deletedMemoryLeakCount')::INTEGER;
    v_canary_secret_leaks := (p_canary->>'secretLeakCount')::INTEGER;
    v_canary_unauthorized_egress :=
      (p_canary->>'unauthorizedProviderEgressCount')::INTEGER;
  EXCEPTION WHEN OTHERS THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L3_PERSONA_PROMOTION_EVIDENCE_INVALID';
  END;
  IF v_benchmark_hash !~ '^[0-9a-f]{64}$'
    OR v_canary_hash !~ '^[0-9a-f]{64}$'
    OR jsonb_typeof(v_benchmark_report) <> 'object'
    OR v_benchmark_report->>'schemaVersion' <> 'neo-chat.memory-benchmark-report.v1'
    OR COALESCE(v_benchmark_passed, false) IS NOT TRUE
    OR v_benchmark_report#>>'{profile,profileId}' <>
      'memory_l3_persona_hybrid_bge_m3_rrf60_v1'
    OR COALESCE(v_total_reviewed, 0) <> 500
    OR COALESCE(v_development_count, 0) <> 300
    OR COALESCE(v_validation_count, 0) <> 100
    OR COALESCE(v_holdout_count, 0) <> 100
    OR COALESCE(v_benchmark_consistency, 0) < 0.95
    OR COALESCE(v_benchmark_false_injection, 1) > 0.02
    OR COALESCE(v_benchmark_token_saving, 0) < 0.20
    OR COALESCE(v_p95_latency, 999999) > 900
    OR COALESCE(v_p99_latency, 999999) > 1500
    OR COALESCE(v_maximum_prompt_tokens, 999999) > 300
    OR COALESCE(v_hard_cutoff_violations, 1) <> 0
    OR COALESCE(v_benchmark_cross_user_leaks, 1) <> 0
    OR COALESCE(v_benchmark_deleted_leaks, 1) <> 0
    OR COALESCE(v_benchmark_secret_leaks, 1) <> 0
    OR COALESCE(v_untrusted_source_leaks, 1) <> 0
    OR COALESCE(v_benchmark_unauthorized_egress, 1) <> 0
  THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_L3_PERSONA_PROMOTION_BENCHMARK_FAILED';
  END IF;
  IF v_eligible < 100 OR v_reviewed < 100 OR v_reviewed > v_eligible
    OR v_window_end < v_window_start + interval '7 days'
    OR v_window_end > v_now + interval '5 minutes'
    OR COALESCE(v_canary_consistency, 0) < 0.95
    OR COALESCE(v_canary_false_injection, 1) > 0.02
    OR COALESCE(v_canary_token_saving, 0) < 0.20
    OR COALESCE(v_canary_cross_user_leaks, 1) <> 0
    OR COALESCE(v_canary_deleted_leaks, 1) <> 0
    OR COALESCE(v_canary_secret_leaks, 1) <> 0
    OR COALESCE(v_canary_unauthorized_egress, 1) <> 0
  THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_L3_PERSONA_PROMOTION_CANARY_FAILED';
  END IF;
  SELECT count(*), min(created_at), max(created_at)
  INTO v_db_count, v_db_start, v_db_end
  FROM message_memory_l3_persona_observations observation
  WHERE observation.mode = 'shadow' AND observation.status = 'completed'
    AND observation.created_at BETWEEN v_window_start AND v_window_end;
  IF v_db_count < v_eligible OR v_db_start IS NULL OR v_db_end IS NULL
    OR v_db_end < v_db_start + interval '7 days'
    OR EXISTS (SELECT 1 FROM user_memory_persona_jobs WHERE status = 'dead_letter')
    OR EXISTS (SELECT 1 FROM user_memory_persona_embedding_jobs
      WHERE status = 'dead_letter')
    OR NOT EXISTS (
      SELECT 1 FROM user_memory_state
      WHERE active_retrieval_profile_id = 'memory_hybrid_bge_m3_rrf60_v1'
    )
    OR EXISTS (
      SELECT 1 FROM message_memory_l3_persona_observations observation
      LEFT JOIN user_memory_state state ON state.user_id = observation.user_id
      WHERE observation.mode = 'shadow' AND observation.status = 'completed'
        AND observation.created_at BETWEEN v_window_start AND v_window_end
        AND (state.user_id IS NULL OR state.active_retrieval_profile_id IS DISTINCT FROM
          'memory_hybrid_bge_m3_rrf60_v1')
    )
  THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_L3_PERSONA_PROMOTION_RUNTIME_GATE_FAILED';
  END IF;
  SELECT profile.* INTO v_profile FROM memory_l3_persona_profiles profile
  WHERE profile.profile_id = 'memory_l3_persona_v1' FOR UPDATE;
  IF v_profile.lifecycle_status = 'active' THEN
    RAISE EXCEPTION USING ERRCODE = '40001',
      MESSAGE = 'MEMORY_L3_PERSONA_ALREADY_ACTIVE';
  END IF;
  INSERT INTO memory_l3_persona_promotion_events(
    event_id, action, profile_id, benchmark_report_sha256,
    canary_report_sha256, benchmark_case_count, canary_eligible_turns,
    canary_window_started_at, canary_window_ended_at, persona_consistency,
    false_injection_rate, token_saving_ratio, created_at
  ) VALUES (
    p_event_id, 'promote', v_profile.profile_id, v_benchmark_hash,
    v_canary_hash, 500, v_eligible, v_window_start, v_window_end,
    LEAST(v_benchmark_consistency, v_canary_consistency),
    GREATEST(v_benchmark_false_injection, v_canary_false_injection),
    LEAST(v_benchmark_token_saving, v_canary_token_saving), v_now
  );
  UPDATE memory_l3_persona_profiles profile
  SET lifecycle_status = 'active', benchmark_report_sha256 = v_benchmark_hash,
    canary_report_sha256 = v_canary_hash, activated_at = v_now,
    rolled_back_at = NULL, updated_at = v_now
  WHERE profile.profile_id = v_profile.profile_id;
  FOR v_user_id IN SELECT user_id FROM user_memory_state ORDER BY user_id LOOP
    PERFORM memory_l3_persona_reconcile_user(v_user_id);
  END LOOP;
  RETURN jsonb_build_object('profileId', v_profile.profile_id,
    'status', 'active', 'activatedAt', memory_governance_epoch_millis(v_now));
END
$function$;

CREATE FUNCTION memory_operator_rollback_l3_persona(
  p_event_id UUID,
  p_reason_code TEXT
) RETURNS JSONB
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_profile memory_l3_persona_profiles%ROWTYPE;
  v_user_id UUID;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  IF p_event_id IS NULL OR p_reason_code !~ '^[A-Z][A-Z0-9_]{0,63}$'
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L3_PERSONA_ROLLBACK_INVALID';
  END IF;
  SELECT profile.* INTO v_profile FROM memory_l3_persona_profiles profile
  WHERE profile.profile_id = 'memory_l3_persona_v1' FOR UPDATE;
  IF v_profile.lifecycle_status <> 'active' THEN
    RAISE EXCEPTION USING ERRCODE = '40001',
      MESSAGE = 'MEMORY_L3_PERSONA_NOT_ACTIVE';
  END IF;
  INSERT INTO memory_l3_persona_promotion_events(
    event_id, action, profile_id, benchmark_report_sha256,
    canary_report_sha256, reason_code, created_at
  ) VALUES (
    p_event_id, 'rollback', v_profile.profile_id,
    v_profile.benchmark_report_sha256, v_profile.canary_report_sha256,
    p_reason_code, v_now
  );
  UPDATE memory_l3_persona_profiles profile
  SET lifecycle_status = 'rolled_back', rolled_back_at = v_now,
    updated_at = v_now WHERE profile.profile_id = v_profile.profile_id;
  FOR v_user_id IN SELECT user_id FROM user_memory_state ORDER BY user_id LOOP
    PERFORM memory_l3_persona_reconcile_user(v_user_id);
  END LOOP;
  RETURN jsonb_build_object('profileId', v_profile.profile_id,
    'status', 'rolled_back',
    'rolledBackAt', memory_governance_epoch_millis(v_now),
    'reasonCode', p_reason_code);
END
$function$;

CREATE FUNCTION memory_l3_persona_promotion_append_only_guard()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  RAISE EXCEPTION USING ERRCODE = '55000',
    MESSAGE = 'MEMORY_L3_PERSONA_PROMOTION_EVENTS_APPEND_ONLY';
END
$function$;
CREATE TRIGGER memory_l3_persona_promotion_events_append_only
BEFORE UPDATE OR DELETE ON memory_l3_persona_promotion_events
FOR EACH ROW EXECUTE FUNCTION memory_l3_persona_promotion_append_only_guard();

DO $memory_l3_persona_backfill$
DECLARE
  user_record RECORD;
BEGIN
  INSERT INTO user_memory_state(user_id)
  SELECT settings.user_id FROM user_memory_settings settings
  ON CONFLICT (user_id) DO NOTHING;

  FOR user_record IN
    SELECT memory.user_id
    FROM user_memories memory
    JOIN user_memory_state state ON state.user_id = memory.user_id
    JOIN user_memory_settings settings ON settings.user_id = memory.user_id
    WHERE memory.scope_type = 'global'
      AND memory.project_id IS NULL
      AND memory.scope_conversation_id IS NULL
      AND memory.scope_generation = 1
      AND memory.memory_type IN ('fact', 'preference', 'instruction', 'warning', 'decision')
      AND memory.deleted_at IS NULL AND memory.enabled
      AND memory.lifecycle_status = 'active'
      AND memory.visibility_epoch = state.visibility_epoch
      AND (memory.valid_from IS NULL OR memory.valid_from <= clock_timestamp())
      AND (memory.valid_to IS NULL OR clock_timestamp() < memory.valid_to)
      AND (memory.expires_at IS NULL OR clock_timestamp() < memory.expires_at)
      AND memory_governance_classify_sensitivity(memory.content) <> 'secret'
      AND (memory.sensitivity = 'normal' OR settings.sensitive_memory_enabled)
    GROUP BY memory.user_id
    HAVING count(*) >= 2
    ORDER BY memory.user_id
  LOOP
    PERFORM memory_l3_persona_enqueue_user(user_record.user_id);
  END LOOP;
END
$memory_l3_persona_backfill$;

DO $pin_memory_l3_persona_functions$
DECLARE
  schema_name TEXT := current_schema();
  function_identity TEXT;
BEGIN
  FOREACH function_identity IN ARRAY ARRAY[
    'memory_l3_persona_estimated_tokens(text)',
    'memory_l3_persona_source_watermark(uuid)',
    'memory_l3_persona_reconcile_user(uuid)',
    'memory_l3_persona_advance_generation(uuid)',
    'memory_l3_persona_enqueue_user(uuid)',
    'memory_l3_persona_invalidate_user_at_generation(uuid,bigint)',
    'memory_l3_persona_invalidate_user(uuid)',
    'memory_l3_persona_memory_changed()',
    'memory_l3_persona_settings_changed()',
    'memory_l3_persona_state_changed()',
    'memory_queue_l3_persona_embedding()',
    'memory_worker_claim_l3_persona_job(uuid,uuid,integer,boolean)',
    'memory_worker_hydrate_l3_persona_refresh(uuid,uuid,uuid)',
    'memory_worker_complete_l3_persona_refresh(uuid,uuid,uuid,jsonb)',
    'memory_worker_complete_l3_persona_purge(uuid,uuid,uuid)',
    'memory_worker_retry_l3_persona_job(uuid,uuid,uuid,text,timestamp with time zone,boolean)',
    'memory_worker_claim_l3_persona_embedding_job(uuid,uuid,integer)',
    'memory_worker_hydrate_l3_persona_embedding_job(uuid,uuid,uuid)',
    'memory_worker_complete_l3_persona_embedding_job(uuid,uuid,uuid,real[])',
    'memory_worker_retry_l3_persona_embedding_job(uuid,uuid,uuid,text,timestamp with time zone,boolean)',
    'memory_l3_persona_version_current(uuid,uuid,bigint,boolean)',
    'memory_l3_persona_reader_authority(uuid,uuid,uuid,boolean)',
    'memory_prepare_l3_persona_search(uuid,uuid,uuid,uuid,text,text,real[],text,boolean)',
    'memory_record_l3_persona_search(uuid,uuid,uuid,text,text,jsonb,jsonb,integer,integer)',
    'memory_governance_l3_persona_json(uuid,uuid)',
    'memory_governance_l3_persona_snapshot(uuid)',
    'memory_governance_l3_persona_detail(uuid,uuid)',
    'memory_governance_set_l3_persona_enabled(uuid,uuid,bigint,boolean)',
    'memory_governance_rebuild_l3_persona(uuid,uuid)',
    'memory_governance_rebuild_l3_personas(uuid)',
    'memory_operator_promote_l3_persona(uuid,jsonb,jsonb)',
    'memory_operator_rollback_l3_persona(uuid,text)',
    'memory_l3_persona_promotion_append_only_guard()'
  ] LOOP
    EXECUTE format(
      'ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',
      schema_name, function_identity, schema_name
    );
    EXECUTE format(
      'ALTER FUNCTION %I.%s OWNER TO memory_runtime_owner',
      schema_name, function_identity
    );
  END LOOP;
END
$pin_memory_l3_persona_functions$;

ALTER TABLE memory_l3_persona_profiles OWNER TO memory_runtime_owner;
ALTER TABLE user_memory_persona_versions OWNER TO memory_runtime_owner;
ALTER TABLE user_memory_persona_members OWNER TO memory_runtime_owner;
ALTER TABLE user_memory_persona_search_projections OWNER TO memory_runtime_owner;
ALTER TABLE user_memory_persona_jobs OWNER TO memory_runtime_owner;
ALTER TABLE user_memory_persona_embedding_jobs OWNER TO memory_runtime_owner;
ALTER TABLE message_memory_l3_persona_observations OWNER TO memory_runtime_owner;
ALTER TABLE message_memory_l3_persona_results OWNER TO memory_runtime_owner;
ALTER TABLE memory_l3_persona_promotion_events OWNER TO memory_runtime_owner;

REVOKE ALL ON
  memory_l3_persona_profiles,
  user_memory_persona_versions,
  user_memory_persona_members,
  user_memory_persona_search_projections,
  user_memory_persona_jobs,
  user_memory_persona_embedding_jobs,
  message_memory_l3_persona_observations,
  message_memory_l3_persona_results,
  memory_l3_persona_promotion_events
FROM PUBLIC, go_api_runtime, memory_worker_runtime;

REVOKE ALL ON FUNCTION
  memory_l3_persona_estimated_tokens(TEXT),
  memory_l3_persona_source_watermark(UUID),
  memory_l3_persona_reconcile_user(UUID),
  memory_l3_persona_advance_generation(UUID),
  memory_l3_persona_enqueue_user(UUID),
  memory_l3_persona_invalidate_user_at_generation(UUID, BIGINT),
  memory_l3_persona_invalidate_user(UUID),
  memory_l3_persona_memory_changed(),
  memory_l3_persona_settings_changed(),
  memory_l3_persona_state_changed(),
  memory_queue_l3_persona_embedding(),
  memory_worker_claim_l3_persona_job(UUID, UUID, INTEGER, BOOLEAN),
  memory_worker_hydrate_l3_persona_refresh(UUID, UUID, UUID),
  memory_worker_complete_l3_persona_refresh(UUID, UUID, UUID, JSONB),
  memory_worker_complete_l3_persona_purge(UUID, UUID, UUID),
  memory_worker_retry_l3_persona_job(UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN),
  memory_worker_claim_l3_persona_embedding_job(UUID, UUID, INTEGER),
  memory_worker_hydrate_l3_persona_embedding_job(UUID, UUID, UUID),
  memory_worker_complete_l3_persona_embedding_job(UUID, UUID, UUID, REAL[]),
  memory_worker_retry_l3_persona_embedding_job(UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN),
  memory_l3_persona_version_current(UUID, UUID, BIGINT, BOOLEAN),
  memory_l3_persona_reader_authority(UUID, UUID, UUID, BOOLEAN),
  memory_prepare_l3_persona_search(UUID, UUID, UUID, UUID, TEXT, TEXT, REAL[], TEXT, BOOLEAN),
  memory_record_l3_persona_search(UUID, UUID, UUID, TEXT, TEXT, JSONB, JSONB, INTEGER, INTEGER),
  memory_governance_l3_persona_json(UUID, UUID),
  memory_governance_l3_persona_snapshot(UUID),
  memory_governance_l3_persona_detail(UUID, UUID),
  memory_governance_set_l3_persona_enabled(UUID, UUID, BIGINT, BOOLEAN),
  memory_governance_rebuild_l3_persona(UUID, UUID),
  memory_governance_rebuild_l3_personas(UUID),
  memory_operator_promote_l3_persona(UUID, JSONB, JSONB),
  memory_operator_rollback_l3_persona(UUID, TEXT),
  memory_l3_persona_promotion_append_only_guard()
FROM PUBLIC, go_api_runtime, memory_worker_runtime;

GRANT EXECUTE ON FUNCTION
  memory_worker_claim_l3_persona_job(UUID, UUID, INTEGER, BOOLEAN),
  memory_worker_hydrate_l3_persona_refresh(UUID, UUID, UUID),
  memory_worker_complete_l3_persona_refresh(UUID, UUID, UUID, JSONB),
  memory_worker_complete_l3_persona_purge(UUID, UUID, UUID),
  memory_worker_retry_l3_persona_job(UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN),
  memory_worker_claim_l3_persona_embedding_job(UUID, UUID, INTEGER),
  memory_worker_hydrate_l3_persona_embedding_job(UUID, UUID, UUID),
  memory_worker_complete_l3_persona_embedding_job(UUID, UUID, UUID, REAL[]),
  memory_worker_retry_l3_persona_embedding_job(UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN)
TO memory_worker_runtime;

GRANT EXECUTE ON FUNCTION
  memory_l3_persona_reader_authority(UUID, UUID, UUID, BOOLEAN),
  memory_prepare_l3_persona_search(UUID, UUID, UUID, UUID, TEXT, TEXT, REAL[], TEXT, BOOLEAN),
  memory_record_l3_persona_search(UUID, UUID, UUID, TEXT, TEXT, JSONB, JSONB, INTEGER, INTEGER),
  memory_governance_l3_persona_snapshot(UUID),
  memory_governance_l3_persona_detail(UUID, UUID),
  memory_governance_set_l3_persona_enabled(UUID, UUID, BIGINT, BOOLEAN),
  memory_governance_rebuild_l3_persona(UUID, UUID),
  memory_governance_rebuild_l3_personas(UUID)
TO go_api_runtime;

-- Promotion and rollback deliberately receive no runtime grant. Only the
-- migration owner/function owner may execute these explicit capabilities.
DO $memory_l3_persona_schema_privileges$
DECLARE
  schema_name TEXT := current_schema();
BEGIN
  EXECUTE format(
    'REVOKE CREATE ON SCHEMA %I FROM memory_runtime_owner, memory_worker_runtime, go_api_runtime',
    schema_name
  );
END
$memory_l3_persona_schema_privileges$;
