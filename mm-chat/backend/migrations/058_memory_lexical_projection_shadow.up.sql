-- Memory v2 PR7 provider-free exact/CJK BM25 projection and 0% prompt
-- injection shadow comparison. The v1 in-process lexical reader remains the
-- only answer/Usage authority.

DO $memory_lexical_prerequisite$
BEGIN
  IF current_setting('server_version_num')::INTEGER / 10000 <> 17 THEN
    RAISE EXCEPTION USING
      ERRCODE = '0A000',
      MESSAGE = 'MEMORY_LEXICAL_REQUIRES_POSTGRESQL_17';
  END IF;
  IF NOT (
    'pg_textsearch' = ANY(regexp_split_to_array(
      current_setting('shared_preload_libraries'),
      '\s*,\s*'
    ))
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_LEXICAL_REQUIRES_PG_TEXTSEARCH_PRELOAD';
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_extension
    WHERE extname = 'pg_textsearch' AND extversion = '1.3.1'
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '0A000',
      MESSAGE = 'MEMORY_LEXICAL_REQUIRES_PG_TEXTSEARCH_1_3_1';
  END IF;
  IF to_regprocedure('knowledge_bm25_shadow_query_terms(text)') IS NULL
    OR to_regprocedure('knowledge_build_bm25_shadow_text(text,text[])') IS NULL
    OR to_regprocedure('knowledge_normalize_bm25_shadow_terms(text[])') IS NULL
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_LEXICAL_REQUIRES_REVIEWED_BM25_NORMALIZER';
  END IF;
END
$memory_lexical_prerequisite$;

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

CREATE TABLE user_memory_search_projections (
  memory_id UUID PRIMARY KEY,
  user_id UUID NOT NULL,
  memory_revision BIGINT NOT NULL CHECK (memory_revision >= 1),
  scope_type TEXT NOT NULL CHECK (
    scope_type IN ('global', 'project', 'conversation')
  ),
  project_id UUID,
  scope_conversation_id UUID,
  scope_generation BIGINT NOT NULL CHECK (scope_generation >= 1),
  sensitivity TEXT NOT NULL CHECK (sensitivity IN ('normal', 'sensitive')),
  visibility_epoch BIGINT NOT NULL CHECK (visibility_epoch >= 1),
  projection_generation BIGINT NOT NULL CHECK (projection_generation >= 1),
  retrieval_profile_id TEXT NOT NULL CHECK (
    retrieval_profile_id = 'memory_lexical_cjk_bm25_v1'
  ),
  content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  exact_terms TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[] CHECK (
    array_position(exact_terms, NULL) IS NULL
    AND cardinality(exact_terms) <= 64
    AND exact_terms = knowledge_normalize_bm25_shadow_terms(exact_terms)
  ),
  bm25_text TEXT NOT NULL CHECK (
    octet_length(bm25_text) BETWEEN 1 AND 131072
  ),
  lexical_status TEXT NOT NULL DEFAULT 'ready' CHECK (
    lexical_status IN ('ready', 'failed')
  ),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT user_memory_search_projection_scope_shape CHECK (
    (scope_type = 'global' AND project_id IS NULL
      AND scope_conversation_id IS NULL AND scope_generation = 1)
    OR (scope_type = 'project' AND project_id IS NOT NULL
      AND scope_conversation_id IS NULL)
    OR (scope_type = 'conversation' AND project_id IS NULL
      AND scope_conversation_id IS NOT NULL)
  ),
  CONSTRAINT user_memory_search_projection_timestamps_order
    CHECK (updated_at >= created_at),
  CONSTRAINT user_memory_search_projection_memory_owner_fk
    FOREIGN KEY (memory_id, user_id)
    REFERENCES user_memories(id, user_id) ON DELETE CASCADE,
  CONSTRAINT user_memory_search_projection_project_owner_fk
    FOREIGN KEY (project_id, user_id)
    REFERENCES projects(id, user_id) ON DELETE CASCADE,
  CONSTRAINT user_memory_search_projection_conversation_owner_fk
    FOREIGN KEY (scope_conversation_id, user_id)
    REFERENCES conversations(id, user_id) ON DELETE CASCADE,
  UNIQUE (memory_id, user_id)
);

CREATE INDEX idx_user_memory_search_projection_authority
  ON user_memory_search_projections(
    user_id, projection_generation, retrieval_profile_id,
    visibility_epoch, scope_type, project_id, scope_conversation_id,
    scope_generation, lexical_status, memory_id
  );
CREATE INDEX idx_user_memory_search_projection_exact
  ON user_memory_search_projections USING gin(exact_terms);
CREATE INDEX idx_user_memory_search_projection_bm25
  ON user_memory_search_projections
  USING bm25(bm25_text) WITH (text_config = 'simple');

CREATE TABLE message_memory_lexical_shadow_observations (
  id UUID PRIMARY KEY,
  assistant_message_id UUID NOT NULL UNIQUE,
  user_id UUID NOT NULL,
  conversation_id UUID NOT NULL,
  retrieval_profile_id TEXT NOT NULL CHECK (
    retrieval_profile_id = 'memory_lexical_cjk_bm25_v1'
  ),
  projection_generation BIGINT NOT NULL CHECK (projection_generation >= 1),
  query_sha256 TEXT NOT NULL CHECK (query_sha256 ~ '^[0-9a-f]{64}$'),
  baseline_sha256 TEXT NOT NULL CHECK (baseline_sha256 ~ '^[0-9a-f]{64}$'),
  status TEXT NOT NULL CHECK (status IN ('pending', 'completed', 'failed')),
  result_code TEXT NOT NULL CHECK (
    octet_length(result_code) BETWEEN 1 AND 128
    AND result_code = upper(result_code)
  ),
  baseline_count SMALLINT NOT NULL DEFAULT 0 CHECK (
    baseline_count BETWEEN 0 AND 5
  ),
  exact_count SMALLINT NOT NULL DEFAULT 0 CHECK (exact_count BETWEEN 0 AND 20),
  bm25_count SMALLINT NOT NULL DEFAULT 0 CHECK (bm25_count BETWEEN 0 AND 30),
  lexical_count SMALLINT NOT NULL DEFAULT 0 CHECK (
    lexical_count BETWEEN 0 AND 20
  ),
  overlap_count SMALLINT NOT NULL DEFAULT 0 CHECK (
    overlap_count BETWEEN 0 AND 5
  ),
  duration_millis INTEGER NOT NULL DEFAULT 0 CHECK (
    duration_millis BETWEEN 0 AND 120000
  ),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT message_memory_lexical_shadow_observation_timestamps_order
    CHECK (updated_at >= created_at),
  CONSTRAINT message_memory_lexical_shadow_observation_assistant_owner_fk
    FOREIGN KEY (assistant_message_id, user_id)
    REFERENCES messages(id, user_id) ON DELETE CASCADE,
  CONSTRAINT message_memory_lexical_shadow_observation_conversation_owner_fk
    FOREIGN KEY (conversation_id, user_id)
    REFERENCES conversations(id, user_id) ON DELETE CASCADE,
  UNIQUE (id, user_id)
);

CREATE INDEX idx_message_memory_lexical_shadow_observation_user_created
  ON message_memory_lexical_shadow_observations(user_id, created_at, id);

CREATE TABLE message_memory_lexical_shadow_results (
  observation_id UUID NOT NULL,
  user_id UUID NOT NULL,
  lane TEXT NOT NULL CHECK (lane IN ('v1', 'exact', 'bm25', 'lexical')),
  ordinal SMALLINT NOT NULL CHECK (
    ordinal >= 1
    AND (
      (lane = 'v1' AND ordinal <= 5)
      OR (lane = 'exact' AND ordinal <= 20)
      OR (lane = 'bm25' AND ordinal <= 30)
      OR (lane = 'lexical' AND ordinal <= 20)
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
  CONSTRAINT message_memory_lexical_shadow_result_observation_owner_fk
    FOREIGN KEY (observation_id, user_id)
    REFERENCES message_memory_lexical_shadow_observations(id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT message_memory_lexical_shadow_result_memory_owner_fk
    FOREIGN KEY (memory_id, user_id)
    REFERENCES user_memories(id, user_id) ON DELETE CASCADE
);

CREATE INDEX idx_message_memory_lexical_shadow_result_memory
  ON message_memory_lexical_shadow_results(user_id, memory_id, created_at);

CREATE FUNCTION memory_refresh_lexical_projection(p_memory_id UUID)
RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_memory user_memories%ROWTYPE;
  v_state user_memory_state%ROWTYPE;
  v_exact_terms TEXT[];
  v_bm25_text TEXT;
BEGIN
  IF p_memory_id IS NULL THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_LEXICAL_REFRESH_ARGUMENT_INVALID';
  END IF;

  SELECT memory.* INTO v_memory
  FROM user_memories memory
  WHERE memory.id = p_memory_id;
  IF NOT FOUND THEN
    RETURN false;
  END IF;

  SELECT state.* INTO v_state
  FROM user_memory_state state
  WHERE state.user_id = v_memory.user_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_LEXICAL_STATE_MISSING';
  END IF;

  IF v_memory.deleted_at IS NOT NULL
    OR NOT v_memory.enabled
    OR v_memory.lifecycle_status <> 'active'
    OR v_memory.content IS NULL
    OR octet_length(v_memory.content) NOT BETWEEN 1 AND 65536
    OR v_memory.content_hash IS NULL
    OR v_memory.visibility_epoch <> v_state.visibility_epoch
    OR (
      v_memory.scope_type = 'project'
      AND NOT EXISTS (
        SELECT 1
        FROM projects project
        WHERE project.id = v_memory.project_id
          AND project.user_id = v_memory.user_id
          AND project.lifecycle_status = 'active'
          AND project.scope_generation = v_memory.scope_generation
      )
    )
    OR (
      v_memory.scope_type = 'conversation'
      AND NOT EXISTS (
        SELECT 1
        FROM conversations conversation
        WHERE conversation.id = v_memory.scope_conversation_id
          AND conversation.user_id = v_memory.user_id
          AND conversation.deleted_at IS NULL
          AND conversation.memory_scope_generation = v_memory.scope_generation
      )
    )
  THEN
    DELETE FROM user_memory_search_projections projection
    WHERE projection.memory_id = v_memory.id;
    RETURN false;
  END IF;

  v_exact_terms := knowledge_bm25_shadow_query_terms(concat_ws(
    ' ',
    v_memory.normalized_content,
    array_to_string(v_memory.tags, ' '),
    v_memory.subject_key,
    v_memory.fact_key
  ));
  v_bm25_text := knowledge_build_bm25_shadow_text(
    concat_ws(
      ' ',
      v_memory.content,
      array_to_string(v_memory.tags, ' '),
      v_memory.subject_key,
      v_memory.fact_key
    ),
    v_exact_terms
  );

  INSERT INTO user_memory_search_projections (
    memory_id, user_id, memory_revision, scope_type, project_id,
    scope_conversation_id, scope_generation, sensitivity, visibility_epoch,
    projection_generation, retrieval_profile_id, content_hash, exact_terms,
    bm25_text, lexical_status
  ) VALUES (
    v_memory.id, v_memory.user_id, v_memory.revision, v_memory.scope_type,
    v_memory.project_id, v_memory.scope_conversation_id,
    v_memory.scope_generation, v_memory.sensitivity,
    v_memory.visibility_epoch, v_state.active_projection_generation,
    'memory_lexical_cjk_bm25_v1', v_memory.content_hash,
    v_exact_terms, v_bm25_text, 'ready'
  )
  ON CONFLICT (memory_id) DO UPDATE SET
    user_id = EXCLUDED.user_id,
    memory_revision = EXCLUDED.memory_revision,
    scope_type = EXCLUDED.scope_type,
    project_id = EXCLUDED.project_id,
    scope_conversation_id = EXCLUDED.scope_conversation_id,
    scope_generation = EXCLUDED.scope_generation,
    sensitivity = EXCLUDED.sensitivity,
    visibility_epoch = EXCLUDED.visibility_epoch,
    projection_generation = EXCLUDED.projection_generation,
    retrieval_profile_id = EXCLUDED.retrieval_profile_id,
    content_hash = EXCLUDED.content_hash,
    exact_terms = EXCLUDED.exact_terms,
    bm25_text = EXCLUDED.bm25_text,
    lexical_status = EXCLUDED.lexical_status,
    updated_at = clock_timestamp();
  RETURN true;
END
$function$;

CREATE FUNCTION memory_maintain_lexical_projection()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  PERFORM memory_refresh_lexical_projection(NEW.id);
  RETURN NEW;
END
$function$;

CREATE TRIGGER user_memories_lexical_projection_insert
AFTER INSERT ON user_memories
FOR EACH ROW EXECUTE FUNCTION memory_maintain_lexical_projection();

CREATE TRIGGER user_memories_lexical_projection_update
AFTER UPDATE OF
  content, normalized_content, tags, revision, visibility_epoch, content_hash,
  scope_type, project_id, scope_conversation_id, scope_generation, sensitivity,
  enabled, lifecycle_status, subject_key, fact_key, deleted_at
ON user_memories
FOR EACH ROW EXECUTE FUNCTION memory_maintain_lexical_projection();

CREATE FUNCTION memory_refresh_user_lexical_projections()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_memory_id UUID;
BEGIN
  FOR v_memory_id IN
    SELECT memory.id FROM user_memories memory
    WHERE memory.user_id = NEW.user_id
    ORDER BY memory.id
  LOOP
    PERFORM memory_refresh_lexical_projection(v_memory_id);
  END LOOP;
  RETURN NEW;
END
$function$;

CREATE TRIGGER user_memory_state_lexical_projection_update
AFTER UPDATE OF visibility_epoch, active_projection_generation
ON user_memory_state
FOR EACH ROW EXECUTE FUNCTION memory_refresh_user_lexical_projections();

CREATE FUNCTION memory_refresh_project_lexical_projections()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_memory_id UUID;
BEGIN
  FOR v_memory_id IN
    SELECT memory.id FROM user_memories memory
    WHERE memory.user_id = NEW.user_id AND memory.project_id = NEW.id
    ORDER BY memory.id
  LOOP
    PERFORM memory_refresh_lexical_projection(v_memory_id);
  END LOOP;
  RETURN NEW;
END
$function$;

CREATE TRIGGER projects_lexical_projection_update
AFTER UPDATE OF lifecycle_status, scope_generation
ON projects
FOR EACH ROW EXECUTE FUNCTION memory_refresh_project_lexical_projections();

CREATE FUNCTION memory_refresh_conversation_lexical_projections()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_memory_id UUID;
BEGIN
  FOR v_memory_id IN
    SELECT memory.id FROM user_memories memory
    WHERE memory.user_id = NEW.user_id
      AND memory.scope_conversation_id = NEW.id
    ORDER BY memory.id
  LOOP
    PERFORM memory_refresh_lexical_projection(v_memory_id);
  END LOOP;
  RETURN NEW;
END
$function$;

CREATE TRIGGER conversations_lexical_projection_update
AFTER UPDATE OF deleted_at, memory_scope_generation
ON conversations
FOR EACH ROW EXECUTE FUNCTION memory_refresh_conversation_lexical_projections();

CREATE FUNCTION memory_compare_lexical_shadow(
  p_observation_id UUID,
  p_user_id UUID,
  p_conversation_id UUID,
  p_assistant_message_id UUID,
  p_query_hash TEXT,
  p_query_text TEXT,
  p_v1_results JSONB,
  p_lexical_limit INTEGER
) RETURNS TABLE (
  observation_id UUID,
  profile_id TEXT,
  projection_generation BIGINT,
  status TEXT,
  result_code TEXT,
  baseline_count INTEGER,
  exact_count INTEGER,
  bm25_count INTEGER,
  lexical_count INTEGER,
  overlap_count INTEGER,
  duration_millis INTEGER
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_started_at TIMESTAMPTZ := clock_timestamp();
  v_query_hash TEXT;
  v_baseline_hash TEXT;
  v_existing message_memory_lexical_shadow_observations%ROWTYPE;
  v_conversation conversations%ROWTYPE;
  v_generation BIGINT;
  v_query_terms TEXT[];
  v_bm25_query TEXT;
  v_baseline_count INTEGER;
  v_exact_count INTEGER;
  v_bm25_count INTEGER;
  v_lexical_count INTEGER;
  v_overlap_count INTEGER;
  v_duration INTEGER;
BEGIN
  IF p_observation_id IS NULL OR p_user_id IS NULL
    OR p_conversation_id IS NULL OR p_assistant_message_id IS NULL
    OR p_query_hash IS NULL OR p_query_hash !~ '^[0-9a-f]{64}$'
    OR p_query_text IS NULL
    OR octet_length(p_query_text) NOT BETWEEN 1 AND 12000
    OR p_v1_results IS NULL OR jsonb_typeof(p_v1_results) <> 'array'
    OR jsonb_array_length(p_v1_results) > 5
    OR p_lexical_limit IS NULL OR p_lexical_limit NOT BETWEEN 1 AND 20
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_LEXICAL_SHADOW_ARGUMENT_INVALID';
  END IF;

  v_query_hash := encode(sha256(convert_to(p_query_text, 'UTF8')), 'hex');
  v_baseline_hash := encode(
    sha256(convert_to(p_v1_results::TEXT, 'UTF8')),
    'hex'
  );
  IF p_query_hash <> v_query_hash THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_LEXICAL_SHADOW_QUERY_HASH_INVALID';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM jsonb_array_elements(p_v1_results) item
    WHERE jsonb_typeof(item) <> 'object'
      OR ARRAY(
        SELECT key FROM jsonb_object_keys(item) key ORDER BY key
      ) <> ARRAY['memoryId', 'revision', 'scopeType']::TEXT[]
      OR COALESCE(item->>'memoryId', '') !~
        '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
      OR COALESCE(item->>'revision', '') !~ '^[1-9][0-9]*$'
      OR item->>'scopeType' <> 'global'
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_LEXICAL_SHADOW_BASELINE_INVALID';
  END IF;

  PERFORM pg_advisory_xact_lock(
    hashtext('memory_lexical_shadow'),
    hashtext(p_assistant_message_id::TEXT)
  );
  SELECT observation.* INTO v_existing
  FROM message_memory_lexical_shadow_observations observation
  WHERE observation.assistant_message_id = p_assistant_message_id
  FOR UPDATE;
  IF FOUND THEN
    IF v_existing.user_id <> p_user_id
      OR v_existing.conversation_id <> p_conversation_id
      OR v_existing.query_sha256 <> p_query_hash
      OR v_existing.baseline_sha256 <> v_baseline_hash
      OR v_existing.retrieval_profile_id <> 'memory_lexical_cjk_bm25_v1'
    THEN
      RAISE EXCEPTION USING
        ERRCODE = '40001',
        MESSAGE = 'MEMORY_LEXICAL_SHADOW_REPLAY_CONFLICT';
    END IF;
    RETURN QUERY SELECT
      v_existing.id,
      v_existing.retrieval_profile_id,
      v_existing.projection_generation,
      v_existing.status,
      v_existing.result_code,
      v_existing.baseline_count::INTEGER,
      v_existing.exact_count::INTEGER,
      v_existing.bm25_count::INTEGER,
      v_existing.lexical_count::INTEGER,
      v_existing.overlap_count::INTEGER,
      v_existing.duration_millis;
    RETURN;
  END IF;

  SELECT conversation.* INTO v_conversation
  FROM conversations conversation
  WHERE conversation.id = p_conversation_id
    AND conversation.user_id = p_user_id
    AND conversation.deleted_at IS NULL;
  IF NOT FOUND OR NOT EXISTS (
    SELECT 1
    FROM messages assistant
    JOIN messages source
      ON source.id = assistant.parent_message_id
     AND source.conversation_id = assistant.conversation_id
     AND source.user_id = assistant.user_id
     AND source.role = 'user'
     AND source.status = 'completed'
     AND source.deleted_at IS NULL
     AND source.content = p_query_text
    WHERE assistant.id = p_assistant_message_id
      AND assistant.conversation_id = p_conversation_id
      AND assistant.user_id = p_user_id
      AND assistant.role = 'assistant'
      AND assistant.status IN ('pending', 'streaming')
      AND assistant.deleted_at IS NULL
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_LEXICAL_SHADOW_SOURCE_INVALID';
  END IF;
  IF v_conversation.project_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM projects project
    WHERE project.id = v_conversation.project_id
      AND project.user_id = p_user_id
      AND project.lifecycle_status = 'active'
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_LEXICAL_SHADOW_SCOPE_INVALID';
  END IF;

  SELECT state.active_projection_generation INTO v_generation
  FROM user_memory_state state
  JOIN user_memory_settings settings ON settings.user_id = state.user_id
  WHERE state.user_id = p_user_id
    AND settings.enabled
    AND settings.search_enabled;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_LEXICAL_SHADOW_DISABLED';
  END IF;

  SELECT count(*) INTO v_baseline_count
  FROM jsonb_array_elements(p_v1_results);
  IF EXISTS (
    SELECT 1
    FROM jsonb_array_elements(p_v1_results) WITH ORDINALITY item(payload, ordinal)
    LEFT JOIN user_memories memory
      ON memory.id = (item.payload->>'memoryId')::UUID
     AND memory.user_id = p_user_id
     AND memory.revision = (item.payload->>'revision')::BIGINT
     AND memory.scope_type = item.payload->>'scopeType'
     AND memory.scope_type = 'global'
     AND memory.deleted_at IS NULL
     AND memory.enabled
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
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_LEXICAL_SHADOW_BASELINE_STALE';
  END IF;

  INSERT INTO message_memory_lexical_shadow_observations (
    id, assistant_message_id, user_id, conversation_id,
    retrieval_profile_id, projection_generation, query_sha256,
    baseline_sha256, status, result_code, baseline_count
  ) VALUES (
    p_observation_id, p_assistant_message_id, p_user_id, p_conversation_id,
    'memory_lexical_cjk_bm25_v1', v_generation, p_query_hash,
    v_baseline_hash, 'pending', 'PENDING', v_baseline_count
  );
  INSERT INTO message_memory_lexical_shadow_results (
    observation_id, user_id, lane, ordinal, memory_id, memory_revision,
    scope_type
  )
  SELECT
    p_observation_id,
    p_user_id,
    'v1',
    item.ordinal::SMALLINT,
    (item.payload->>'memoryId')::UUID,
    (item.payload->>'revision')::BIGINT,
    item.payload->>'scopeType'
  FROM jsonb_array_elements(p_v1_results) WITH ORDINALITY item(payload, ordinal)
  ORDER BY item.ordinal;

  v_query_terms := knowledge_bm25_shadow_query_terms(p_query_text);
  v_bm25_query := knowledge_build_bm25_shadow_text(
    p_query_text,
    v_query_terms
  );

  BEGIN
    WITH authorized AS MATERIALIZED (
      SELECT
        projection.memory_id,
        projection.memory_revision,
        projection.scope_type,
        projection.exact_terms,
        projection.bm25_text,
        memory.normalized_content,
        memory.importance,
        memory.updated_at
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
       AND state.active_projection_generation =
         projection.projection_generation
      JOIN user_memory_settings settings
        ON settings.user_id = memory.user_id
       AND settings.enabled
       AND settings.search_enabled
      LEFT JOIN projects scoped_project
        ON projection.scope_type = 'project'
       AND scoped_project.id = projection.project_id
       AND scoped_project.user_id = projection.user_id
       AND scoped_project.lifecycle_status = 'active'
       AND scoped_project.scope_generation = projection.scope_generation
      WHERE projection.user_id = p_user_id
        AND projection.retrieval_profile_id =
          'memory_lexical_cjk_bm25_v1'
        AND projection.projection_generation = v_generation
        AND projection.lexical_status = 'ready'
        AND memory.deleted_at IS NULL
        AND memory.enabled
        AND memory.lifecycle_status = 'active'
        AND (memory.valid_from IS NULL OR memory.valid_from <= now())
        AND (memory.valid_to IS NULL OR now() < memory.valid_to)
        AND (memory.expires_at IS NULL OR now() < memory.expires_at)
        AND (
          memory.sensitivity = 'normal'
          OR settings.sensitive_memory_enabled
        )
        AND (
          (projection.scope_type = 'global'
            AND projection.scope_generation = 1)
          OR (
            projection.scope_type = 'conversation'
            AND projection.scope_conversation_id = p_conversation_id
            AND projection.scope_generation =
              v_conversation.memory_scope_generation
          )
          OR (
            projection.scope_type = 'project'
            AND v_conversation.project_id IS NOT NULL
            AND projection.project_id = v_conversation.project_id
            AND scoped_project.id IS NOT NULL
          )
        )
    ), exact_unbounded AS MATERIALIZED (
      SELECT
        candidate.*,
        row_number() OVER (
          ORDER BY
            (candidate.normalized_content = lower(p_query_text)) DESC,
            (
              SELECT count(*) FROM unnest(candidate.exact_terms) term
              WHERE term = ANY(v_query_terms)
            ) DESC,
            candidate.importance DESC,
            candidate.updated_at DESC,
            candidate.memory_id
        )::INTEGER AS lane_rank
      FROM authorized candidate
      WHERE cardinality(v_query_terms) > 0
        AND candidate.exact_terms && v_query_terms
    ), exact_ranked AS MATERIALIZED (
      SELECT * FROM exact_unbounded ranked WHERE ranked.lane_rank <= 20
    ), bm25_probe AS MATERIALIZED (
      SELECT
        candidate.*,
        candidate.bm25_text <@> to_bm25query(
          v_bm25_query,
          'idx_user_memory_search_projection_bm25'
        ) AS raw_score
      FROM authorized candidate
      WHERE candidate.bm25_text <@> to_bm25query(
        v_bm25_query,
        'idx_user_memory_search_projection_bm25'
      ) < 0
      ORDER BY raw_score, candidate.memory_id
      LIMIT 30
    ), bm25_ranked AS MATERIALIZED (
      SELECT
        candidate.*,
        row_number() OVER (
          ORDER BY candidate.raw_score, candidate.importance DESC,
            candidate.updated_at DESC, candidate.memory_id
        )::INTEGER AS lane_rank
      FROM bm25_probe candidate
    ), lexical_base AS MATERIALIZED (
      SELECT
        COALESCE(exact.memory_id, bm25.memory_id) AS memory_id,
        COALESCE(exact.memory_revision, bm25.memory_revision) AS memory_revision,
        COALESCE(exact.scope_type, bm25.scope_type) AS scope_type,
        exact.lane_rank AS exact_rank,
        bm25.lane_rank AS bm25_rank
      FROM exact_ranked exact
      FULL JOIN bm25_ranked bm25 ON bm25.memory_id = exact.memory_id
    ), lexical_ranked_unbounded AS MATERIALIZED (
      SELECT
        candidate.*,
        row_number() OVER (
          ORDER BY
            (candidate.exact_rank IS NOT NULL) DESC,
            COALESCE(candidate.exact_rank, 2147483647),
            COALESCE(candidate.bm25_rank, 2147483647),
            candidate.memory_id
        )::INTEGER AS lane_rank
      FROM lexical_base candidate
    ), lexical_ranked AS MATERIALIZED (
      SELECT * FROM lexical_ranked_unbounded ranked
      WHERE ranked.lane_rank <= p_lexical_limit
    ), lane_rows AS (
      SELECT
        'exact'::TEXT AS lane,
        exact.lane_rank,
        exact.memory_id,
        exact.memory_revision,
        exact.scope_type
      FROM exact_ranked exact
      UNION ALL
      SELECT
        'bm25', bm25.lane_rank, bm25.memory_id,
        bm25.memory_revision, bm25.scope_type
      FROM bm25_ranked bm25
      UNION ALL
      SELECT
        'lexical', lexical.lane_rank, lexical.memory_id,
        lexical.memory_revision, lexical.scope_type
      FROM lexical_ranked lexical
    )
    INSERT INTO message_memory_lexical_shadow_results (
      observation_id, user_id, lane, ordinal, memory_id, memory_revision,
      scope_type
    )
    SELECT
      p_observation_id,
      p_user_id,
      lane_rows.lane,
      lane_rows.lane_rank::SMALLINT,
      lane_rows.memory_id,
      lane_rows.memory_revision,
      lane_rows.scope_type
    FROM lane_rows
    ORDER BY lane_rows.lane, lane_rows.lane_rank;
  EXCEPTION WHEN OTHERS THEN
    v_duration := least(
      120000,
      greatest(0, floor(extract(epoch FROM clock_timestamp() - v_started_at) * 1000)::INTEGER)
    );
    UPDATE message_memory_lexical_shadow_observations observation
    SET status = 'failed', result_code = 'SEARCH_FAILED',
        duration_millis = v_duration, updated_at = clock_timestamp()
    WHERE observation.id = p_observation_id;
    SELECT observation.* INTO v_existing
    FROM message_memory_lexical_shadow_observations observation
    WHERE observation.id = p_observation_id;
    RETURN QUERY SELECT
      v_existing.id,
      v_existing.retrieval_profile_id,
      v_existing.projection_generation,
      v_existing.status,
      v_existing.result_code,
      v_existing.baseline_count::INTEGER,
      v_existing.exact_count::INTEGER,
      v_existing.bm25_count::INTEGER,
      v_existing.lexical_count::INTEGER,
      v_existing.overlap_count::INTEGER,
      v_existing.duration_millis;
    RETURN;
  END;

  SELECT count(*) INTO v_exact_count
  FROM message_memory_lexical_shadow_results result
  WHERE result.observation_id = p_observation_id AND result.lane = 'exact';
  SELECT count(*) INTO v_bm25_count
  FROM message_memory_lexical_shadow_results result
  WHERE result.observation_id = p_observation_id AND result.lane = 'bm25';
  SELECT count(*) INTO v_lexical_count
  FROM message_memory_lexical_shadow_results result
  WHERE result.observation_id = p_observation_id AND result.lane = 'lexical';
  SELECT count(*) INTO v_overlap_count
  FROM message_memory_lexical_shadow_results baseline
  JOIN message_memory_lexical_shadow_results lexical
    ON lexical.observation_id = baseline.observation_id
   AND lexical.memory_id = baseline.memory_id
   AND lexical.lane = 'lexical'
  WHERE baseline.observation_id = p_observation_id
    AND baseline.lane = 'v1';
  v_duration := least(
    120000,
    greatest(0, floor(extract(epoch FROM clock_timestamp() - v_started_at) * 1000)::INTEGER)
  );

  UPDATE message_memory_lexical_shadow_observations observation
  SET status = 'completed', result_code = 'OK',
      exact_count = v_exact_count, bm25_count = v_bm25_count,
      lexical_count = v_lexical_count, overlap_count = v_overlap_count,
      duration_millis = v_duration, updated_at = clock_timestamp()
  WHERE observation.id = p_observation_id
  RETURNING observation.* INTO v_existing;

  RETURN QUERY SELECT
    v_existing.id,
    v_existing.retrieval_profile_id,
    v_existing.projection_generation,
    v_existing.status,
    v_existing.result_code,
    v_existing.baseline_count::INTEGER,
    v_existing.exact_count::INTEGER,
    v_existing.bm25_count::INTEGER,
    v_existing.lexical_count::INTEGER,
    v_existing.overlap_count::INTEGER,
    v_existing.duration_millis;
END
$function$;

-- Backfill the fixed generation-1 lexical projection. This remains derived
-- state; re-up safely rebuilds it when no shadow observation history exists.
DO $memory_lexical_backfill$
DECLARE
  v_memory_id UUID;
  v_eligible_count BIGINT;
  v_projection_count BIGINT;
BEGIN
  FOR v_memory_id IN
    SELECT memory.id FROM user_memories memory ORDER BY memory.id
  LOOP
    PERFORM memory_refresh_lexical_projection(v_memory_id);
  END LOOP;

  SELECT count(*) INTO v_eligible_count
  FROM user_memories memory
  JOIN user_memory_state state
    ON state.user_id = memory.user_id
   AND state.visibility_epoch = memory.visibility_epoch
  WHERE memory.deleted_at IS NULL
    AND memory.enabled
    AND memory.lifecycle_status = 'active'
    AND memory.content IS NOT NULL
    AND octet_length(memory.content) BETWEEN 1 AND 65536
    AND memory.content_hash IS NOT NULL
    AND (
      memory.scope_type <> 'project'
      OR EXISTS (
        SELECT 1
        FROM projects project
        WHERE project.id = memory.project_id
          AND project.user_id = memory.user_id
          AND project.lifecycle_status = 'active'
          AND project.scope_generation = memory.scope_generation
      )
    )
    AND (
      memory.scope_type <> 'conversation'
      OR EXISTS (
        SELECT 1
        FROM conversations conversation
        WHERE conversation.id = memory.scope_conversation_id
          AND conversation.user_id = memory.user_id
          AND conversation.deleted_at IS NULL
          AND conversation.memory_scope_generation = memory.scope_generation
      )
    );

  SELECT count(*) INTO v_projection_count
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
    ON state.user_id = projection.user_id
   AND state.visibility_epoch = projection.visibility_epoch
   AND state.active_projection_generation = projection.projection_generation
  WHERE projection.retrieval_profile_id = 'memory_lexical_cjk_bm25_v1'
    AND projection.lexical_status = 'ready';

  IF v_projection_count <> v_eligible_count THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_LEXICAL_BACKFILL_INCOMPLETE';
  END IF;
END
$memory_lexical_backfill$;

ALTER TABLE user_memory_search_projections OWNER TO memory_runtime_owner;
ALTER TABLE message_memory_lexical_shadow_observations
  OWNER TO memory_runtime_owner;
ALTER TABLE message_memory_lexical_shadow_results OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_refresh_lexical_projection(UUID)
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_maintain_lexical_projection()
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_refresh_user_lexical_projections()
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_refresh_project_lexical_projections()
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_refresh_conversation_lexical_projections()
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_compare_lexical_shadow(
  UUID, UUID, UUID, UUID, TEXT, TEXT, JSONB, INTEGER
) OWNER TO memory_runtime_owner;

GRANT SELECT, INSERT, UPDATE, DELETE ON user_memory_search_projections
  TO memory_runtime_owner;
GRANT SELECT, INSERT, UPDATE, DELETE
  ON message_memory_lexical_shadow_observations,
     message_memory_lexical_shadow_results
  TO memory_runtime_owner;
GRANT EXECUTE ON FUNCTION knowledge_normalize_bm25_shadow_terms(TEXT[])
  TO memory_runtime_owner;
GRANT EXECUTE ON FUNCTION knowledge_bm25_shadow_query_terms(TEXT)
  TO memory_runtime_owner;
GRANT EXECUTE ON FUNCTION knowledge_build_bm25_shadow_text(TEXT, TEXT[])
  TO memory_runtime_owner;

REVOKE ALL ON
  user_memory_search_projections,
  message_memory_lexical_shadow_observations,
  message_memory_lexical_shadow_results
FROM PUBLIC, go_api_runtime, memory_worker_runtime;

REVOKE ALL ON FUNCTION memory_refresh_lexical_projection(UUID)
  FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_maintain_lexical_projection()
  FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_refresh_user_lexical_projections()
  FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_refresh_project_lexical_projections()
  FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_refresh_conversation_lexical_projections()
  FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_compare_lexical_shadow(
  UUID, UUID, UUID, UUID, TEXT, TEXT, JSONB, INTEGER
) FROM PUBLIC, go_api_runtime, memory_worker_runtime;
GRANT EXECUTE ON FUNCTION memory_compare_lexical_shadow(
  UUID, UUID, UUID, UUID, TEXT, TEXT, JSONB, INTEGER
) TO go_api_runtime;
