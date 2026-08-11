-- Add an explicit, exact-user preview authority for the sole deployment owner.
-- Formal L2/L3 promotion remains evidence-gated and unchanged. Preview is
-- append-only audited, automatically revoked if another user is added, and
-- requires both existing runtime reader flags before any chat injection.

DO $memory_single_user_derived_reader_preview_prerequisite$
BEGIN
  IF to_regprocedure('memory_l2_scene_reader_authority(uuid,uuid,uuid,boolean)') IS NULL
    OR to_regprocedure('memory_l3_persona_reader_authority(uuid,uuid,uuid,boolean)') IS NULL
    OR to_regclass('memory_l2_scene_profiles') IS NULL
    OR to_regclass('memory_l3_persona_profiles') IS NULL
    OR to_regclass('user_memory_scenes') IS NULL
    OR to_regclass('user_memory_persona_versions') IS NULL
  THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_SINGLE_USER_DERIVED_READER_PREVIEW_REQUIRES_072';
  END IF;
END
$memory_single_user_derived_reader_preview_prerequisite$;

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

CREATE TABLE memory_single_user_derived_reader_events (
  event_id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  enabled BOOLEAN NOT NULL,
  reason_code TEXT NOT NULL CHECK (reason_code ~ '^[A-Z][A-Z0-9_]{0,63}$'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT memory_single_user_derived_reader_event_shape CHECK (
    (enabled AND reason_code = 'OWNER_ACCEPTED_PREVIEW')
    OR (NOT enabled AND reason_code <> 'OWNER_ACCEPTED_PREVIEW')
  )
);
CREATE INDEX idx_memory_single_user_derived_reader_latest
  ON memory_single_user_derived_reader_events(user_id, created_at DESC, event_id DESC);

CREATE FUNCTION memory_single_user_derived_reader_preview_enabled(p_user_id UUID)
RETURNS BOOLEAN
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  SELECT p_user_id IS NOT NULL
    AND (SELECT count(*) = 1 FROM users)
    AND COALESCE((
      SELECT event.enabled
      FROM memory_single_user_derived_reader_events event
      WHERE event.user_id = p_user_id
      ORDER BY event.created_at DESC, event.event_id DESC
      LIMIT 1
    ), false)
$function$;


CREATE OR REPLACE FUNCTION memory_l2_scene_reconcile_user(p_user_id UUID)
RETURNS VOID
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_active BOOLEAN;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  SELECT (
      (profile.lifecycle_status = 'active'
        AND state.active_retrieval_profile_id = 'memory_hybrid_bge_m3_rrf60_v1')
      OR memory_single_user_derived_reader_preview_enabled(p_user_id)
    ) AND settings.enabled AND settings.search_enabled
      AND settings.l2_mode <> 'off'
  INTO v_active
  FROM user_memory_state state
  JOIN user_memory_settings settings ON settings.user_id = state.user_id
  CROSS JOIN memory_l2_scene_profiles profile
  WHERE state.user_id = p_user_id AND profile.profile_id = 'memory_l2_scene_v1';
  v_active := COALESCE(v_active, false);
  UPDATE user_memory_scenes scene
  SET lifecycle_status = CASE
        WHEN scene.user_disabled THEN 'disabled'
        WHEN v_active THEN 'active'
        ELSE 'shadow'
      END,
      activated_at = CASE
        WHEN NOT scene.user_disabled AND v_active THEN COALESCE(scene.activated_at, v_now)
        ELSE scene.activated_at
      END,
      disabled_at = CASE
        WHEN scene.user_disabled THEN COALESCE(scene.disabled_at, v_now)
        ELSE NULL
      END,
      updated_at = v_now
  WHERE scene.user_id = p_user_id
    AND scene.lifecycle_status <> 'stale'
    AND scene.deleted_at IS NULL
    AND scene.generation = (
      SELECT active_l2_generation FROM user_memory_state WHERE user_id = p_user_id
    );
END
$function$;

CREATE OR REPLACE FUNCTION memory_l2_scene_reader_authority(
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
  v_profile memory_l2_scene_profiles%ROWTYPE;
  v_state user_memory_state%ROWTYPE;
  v_settings user_memory_settings%ROWTYPE;
  v_active BOOLEAN;
BEGIN
  IF p_user_id IS NULL OR p_conversation_id IS NULL
    OR p_assistant_message_id IS NULL OR p_active_requested IS NULL
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_L2_SCENE_READER_ARGUMENT_INVALID';
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
    RETURN QUERY SELECT false, 'disabled'::TEXT, 'memory_l2_scene_v1'::TEXT,
      'memory_l2_scene_hybrid_bge_m3_rrf60_v1'::TEXT, 0::BIGINT;
    RETURN;
  END IF;
  SELECT profile.* INTO v_profile FROM memory_l2_scene_profiles profile
  WHERE profile.profile_id = 'memory_l2_scene_v1';
  SELECT * INTO v_state FROM user_memory_state WHERE user_id = p_user_id;
  SELECT * INTO v_settings FROM user_memory_settings WHERE user_id = p_user_id;
  IF v_profile.profile_id IS NULL OR v_state.user_id IS NULL
    OR v_settings.user_id IS NULL OR NOT v_settings.enabled
    OR NOT v_settings.search_enabled OR v_settings.l2_mode = 'off'
  THEN
    RETURN QUERY SELECT false, 'disabled'::TEXT, 'memory_l2_scene_v1'::TEXT,
      'memory_l2_scene_hybrid_bge_m3_rrf60_v1'::TEXT,
      COALESCE(v_state.active_l2_generation, 0);
    RETURN;
  END IF;
  v_active := (
      v_profile.lifecycle_status = 'active'
      AND v_state.active_retrieval_profile_id = 'memory_hybrid_bge_m3_rrf60_v1'
    ) OR memory_single_user_derived_reader_preview_enabled(p_user_id);
  IF p_active_requested AND NOT v_active THEN
    RETURN QUERY SELECT false, 'disabled'::TEXT, v_profile.profile_id,
      v_profile.retrieval_profile_id, v_state.active_l2_generation;
    RETURN;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM user_memory_scenes scene
    LEFT JOIN projects project
      ON scene.scope_type = 'project' AND project.id = scene.project_id
     AND project.user_id = scene.user_id AND project.deleted_at IS NULL
     AND project.lifecycle_status = 'active'
     AND project.scope_generation = scene.scope_generation
    WHERE scene.user_id = p_user_id AND scene.deleted_at IS NULL
      AND scene.generation = v_state.active_l2_generation
      AND scene.visibility_epoch = v_state.visibility_epoch
      AND scene.profile_id = v_profile.profile_id
      AND (NOT p_active_requested OR scene.lifecycle_status = 'active')
      AND scene.lifecycle_status IN ('shadow', 'active')
      AND (scene.sensitivity = 'normal' OR v_settings.sensitive_memory_enabled)
      AND (scene.scope_type = 'global'
        OR (scene.scope_type = 'project' AND scene.project_id = v_conversation.project_id
          AND project.id IS NOT NULL))
      AND NOT EXISTS (
        SELECT 1 FROM user_memory_scene_members member
        LEFT JOIN user_memories memory
          ON memory.id = member.memory_id AND memory.user_id = member.user_id
        WHERE member.scene_id = scene.id AND (
          memory.id IS NULL OR memory.deleted_at IS NOT NULL OR NOT memory.enabled
          OR memory.lifecycle_status <> 'active'
          OR memory.revision <> member.memory_revision
          OR memory.content_hash <> member.memory_content_hash
          OR memory.scope_type <> scene.scope_type
          OR memory.project_id IS DISTINCT FROM scene.project_id
          OR memory.scope_generation <> scene.scope_generation
          OR memory.visibility_epoch <> scene.visibility_epoch
          OR (memory.valid_from IS NOT NULL AND memory.valid_from > clock_timestamp())
          OR (memory.valid_to IS NOT NULL AND memory.valid_to <= clock_timestamp())
          OR (memory.expires_at IS NOT NULL AND memory.expires_at <= clock_timestamp())
          OR (memory.sensitivity = 'sensitive'
            AND NOT v_settings.sensitive_memory_enabled)
        )
      )
  ) THEN
    RETURN QUERY SELECT false, CASE WHEN p_active_requested THEN 'active' ELSE 'shadow' END,
      v_profile.profile_id, v_profile.retrieval_profile_id,
      v_state.active_l2_generation;
    RETURN;
  END IF;
  RETURN QUERY SELECT true, CASE WHEN p_active_requested THEN 'active' ELSE 'shadow' END,
    v_profile.profile_id, v_profile.retrieval_profile_id,
    v_state.active_l2_generation;
END
$function$;

CREATE OR REPLACE FUNCTION memory_governance_l2_scene_snapshot(p_user_id UUID)
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
      'status', CASE
        WHEN memory_single_user_derived_reader_preview_enabled(p_user_id)
          THEN 'active'
        ELSE profile.lifecycle_status
      END,
      'generation', COALESCE(state.active_l2_generation, 1),
      'l1ReaderReady', COALESCE(
        state.active_retrieval_profile_id = 'memory_hybrid_bge_m3_rrf60_v1', false
      ) OR memory_single_user_derived_reader_preview_enabled(p_user_id),
      'active', (
        profile.lifecycle_status = 'active'
        AND COALESCE(state.active_retrieval_profile_id =
          'memory_hybrid_bge_m3_rrf60_v1', false)
      ) OR memory_single_user_derived_reader_preview_enabled(p_user_id),
      'activatedAt', memory_governance_epoch_millis(profile.activated_at),
      'rolledBackAt', memory_governance_epoch_millis(profile.rolled_back_at)
    ),
    'scenes', COALESCE((
      SELECT jsonb_agg(memory_governance_l2_scene_json(p_user_id, scene.id)
        ORDER BY scene.updated_at DESC, scene.id)
      FROM user_memory_scenes scene
      WHERE scene.user_id = p_user_id AND scene.deleted_at IS NULL
    ), '[]'::JSONB)
  )
  FROM memory_l2_scene_profiles profile
  LEFT JOIN user_memory_state state ON state.user_id = p_user_id
  WHERE profile.profile_id = 'memory_l2_scene_v1'
$function$;

CREATE OR REPLACE FUNCTION memory_l3_persona_reconcile_user(p_user_id UUID)
RETURNS VOID
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_active BOOLEAN;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  SELECT (
      (profile.lifecycle_status = 'active'
        AND state.active_retrieval_profile_id = 'memory_hybrid_bge_m3_rrf60_v1')
      OR memory_single_user_derived_reader_preview_enabled(p_user_id)
    ) AND settings.enabled AND settings.search_enabled
      AND settings.l3_mode <> 'off'
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

CREATE OR REPLACE FUNCTION memory_l3_persona_reader_authority(
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
  v_active := (
      v_profile.lifecycle_status = 'active'
      AND v_state.active_retrieval_profile_id = 'memory_hybrid_bge_m3_rrf60_v1'
    ) OR memory_single_user_derived_reader_preview_enabled(p_user_id);
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

CREATE OR REPLACE FUNCTION memory_governance_l3_persona_snapshot(p_user_id UUID)
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
      'status', CASE
        WHEN memory_single_user_derived_reader_preview_enabled(p_user_id)
          THEN 'active'
        ELSE profile.lifecycle_status
      END,
      'generation', COALESCE(state.active_l3_generation, 1),
      'l1ReaderReady', COALESCE(
        state.active_retrieval_profile_id = 'memory_hybrid_bge_m3_rrf60_v1', false
      ) OR memory_single_user_derived_reader_preview_enabled(p_user_id),
      'active', (
        profile.lifecycle_status = 'active'
        AND COALESCE(state.active_retrieval_profile_id =
          'memory_hybrid_bge_m3_rrf60_v1', false)
      ) OR memory_single_user_derived_reader_preview_enabled(p_user_id),
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


CREATE FUNCTION memory_single_user_derived_reader_artifact_reconcile()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF pg_trigger_depth() > 1 OR NEW.lifecycle_status <> 'shadow'
    OR NOT memory_single_user_derived_reader_preview_enabled(NEW.user_id)
  THEN
    RETURN NEW;
  END IF;
  IF TG_TABLE_NAME = 'user_memory_scenes' THEN
    PERFORM memory_l2_scene_reconcile_user(NEW.user_id);
  ELSIF TG_TABLE_NAME = 'user_memory_persona_versions' THEN
    PERFORM memory_l3_persona_reconcile_user(NEW.user_id);
  END IF;
  RETURN NEW;
END
$function$;

CREATE TRIGGER user_memory_scenes_single_user_preview_reconcile
AFTER INSERT OR UPDATE OF lifecycle_status ON user_memory_scenes
FOR EACH ROW WHEN (NEW.lifecycle_status = 'shadow')
EXECUTE FUNCTION memory_single_user_derived_reader_artifact_reconcile();

CREATE TRIGGER user_memory_persona_single_user_preview_reconcile
AFTER INSERT OR UPDATE OF lifecycle_status ON user_memory_persona_versions
FOR EACH ROW WHEN (NEW.lifecycle_status = 'shadow')
EXECUTE FUNCTION memory_single_user_derived_reader_artifact_reconcile();

CREATE FUNCTION memory_single_user_derived_reader_events_append_only()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  -- Per-user audit follows the existing account-erasure boundary. A direct
  -- event delete still sees its owning user and is rejected; the users(id)
  -- ON DELETE CASCADE reaches this trigger only after the parent row is gone.
  IF TG_OP = 'DELETE' AND NOT EXISTS (
    SELECT 1 FROM users WHERE id = OLD.user_id
  ) THEN
    RETURN OLD;
  END IF;
  RAISE EXCEPTION USING ERRCODE = '55000',
    MESSAGE = 'MEMORY_SINGLE_USER_DERIVED_READER_EVENTS_APPEND_ONLY';
END
$function$;
CREATE TRIGGER memory_single_user_derived_reader_events_append_only
BEFORE UPDATE OR DELETE ON memory_single_user_derived_reader_events
FOR EACH ROW EXECUTE FUNCTION memory_single_user_derived_reader_events_append_only();

CREATE FUNCTION memory_single_user_derived_reader_users_changed()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_user_id UUID;
BEGIN
  PERFORM pg_advisory_xact_lock(
    hashtext('memory_single_user_derived_reader_preview'), 0
  );
  IF (SELECT count(*) FROM users) > 1 THEN
    INSERT INTO memory_single_user_derived_reader_events(
      event_id, user_id, enabled, reason_code
    )
    SELECT gen_random_uuid(), latest.user_id, false, 'USER_POPULATION_CHANGED'
    FROM (
      SELECT DISTINCT ON (event.user_id) event.user_id, event.enabled
      FROM memory_single_user_derived_reader_events event
      ORDER BY event.user_id, event.created_at DESC, event.event_id DESC
    ) latest
    WHERE latest.enabled;
  END IF;
  FOR v_user_id IN SELECT state.user_id FROM user_memory_state state ORDER BY state.user_id
  LOOP
    PERFORM memory_l2_scene_reconcile_user(v_user_id);
    PERFORM memory_l3_persona_reconcile_user(v_user_id);
  END LOOP;
  RETURN NULL;
END
$function$;
CREATE TRIGGER users_single_user_derived_reader_population_changed
AFTER INSERT OR DELETE ON users
FOR EACH STATEMENT EXECUTE FUNCTION memory_single_user_derived_reader_users_changed();

CREATE FUNCTION memory_operator_set_single_user_derived_reader_preview(
  p_event_id UUID,
  p_user_id UUID,
  p_enabled BOOLEAN,
  p_authority TEXT
) RETURNS JSONB
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_existing memory_single_user_derived_reader_events%ROWTYPE;
  v_current BOOLEAN;
  v_reason TEXT;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  IF p_event_id IS NULL OR p_user_id IS NULL OR p_enabled IS NULL
    OR p_authority IS NULL
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_SINGLE_USER_DERIVED_READER_PREVIEW_ARGUMENT_INVALID';
  END IF;
  v_reason := CASE WHEN p_enabled THEN 'OWNER_ACCEPTED_PREVIEW' ELSE p_authority END;
  IF (p_enabled AND p_authority <>
      'I_ACCEPT_SINGLE_USER_L2_L3_READER_PREVIEW_WITHOUT_FORMAL_PROMOTION')
    OR (NOT p_enabled AND (
      p_authority !~ '^[A-Z][A-Z0-9_]{0,63}$'
      OR p_authority = 'OWNER_ACCEPTED_PREVIEW'
    ))
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_SINGLE_USER_DERIVED_READER_PREVIEW_AUTHORITY_INVALID';
  END IF;

  PERFORM pg_advisory_xact_lock(
    hashtext('memory_single_user_derived_reader_preview'), 0
  );
  LOCK TABLE memory_single_user_derived_reader_events IN SHARE ROW EXCLUSIVE MODE;
  SELECT event.* INTO v_existing
  FROM memory_single_user_derived_reader_events event
  WHERE event.event_id = p_event_id;
  IF FOUND THEN
    IF v_existing.user_id <> p_user_id OR v_existing.enabled <> p_enabled
      OR v_existing.reason_code <> v_reason
    THEN
      RAISE EXCEPTION USING ERRCODE = '40001',
        MESSAGE = 'MEMORY_SINGLE_USER_DERIVED_READER_PREVIEW_REPLAY_CONFLICT';
    END IF;
    RETURN jsonb_build_object(
      'eventId', v_existing.event_id::TEXT,
      'userId', v_existing.user_id::TEXT,
      'enabled', v_existing.enabled,
      'mode', 'single_user_preview',
      'replayed', true
    );
  END IF;

  IF NOT EXISTS (SELECT 1 FROM users WHERE id = p_user_id) THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_SINGLE_USER_DERIVED_READER_PREVIEW_USER_NOT_FOUND';
  END IF;
  IF p_enabled AND (SELECT count(*) FROM users) <> 1 THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_SINGLE_USER_DERIVED_READER_PREVIEW_REQUIRES_SOLE_USER';
  END IF;
  SELECT event.enabled INTO v_current
  FROM memory_single_user_derived_reader_events event
  WHERE event.user_id = p_user_id
  ORDER BY event.created_at DESC, event.event_id DESC
  LIMIT 1;
  IF COALESCE(v_current, false) = p_enabled THEN
    RAISE EXCEPTION USING ERRCODE = '40001',
      MESSAGE = CASE WHEN p_enabled
        THEN 'MEMORY_SINGLE_USER_DERIVED_READER_PREVIEW_ALREADY_ENABLED'
        ELSE 'MEMORY_SINGLE_USER_DERIVED_READER_PREVIEW_ALREADY_DISABLED' END;
  END IF;

  IF p_enabled THEN
    IF NOT EXISTS (
      SELECT 1 FROM user_memory_settings settings
      WHERE settings.user_id = p_user_id AND settings.enabled
        AND settings.search_enabled AND settings.l2_mode <> 'off'
        AND settings.l3_mode <> 'off'
    ) OR EXISTS (
      SELECT 1 FROM user_memory_scene_jobs
      WHERE user_id = p_user_id AND status IN ('pending','processing','dead_letter')
    ) OR EXISTS (
      SELECT 1 FROM user_memory_derived_embedding_jobs
      WHERE user_id = p_user_id AND status IN ('pending','processing','dead_letter')
    ) OR EXISTS (
      SELECT 1 FROM user_memory_persona_jobs
      WHERE user_id = p_user_id AND status IN ('pending','processing','dead_letter')
    ) OR EXISTS (
      SELECT 1 FROM user_memory_persona_embedding_jobs
      WHERE user_id = p_user_id AND status IN ('pending','processing','dead_letter')
    ) THEN
      RAISE EXCEPTION USING ERRCODE = '55000',
        MESSAGE = 'MEMORY_SINGLE_USER_DERIVED_READER_PREVIEW_NOT_READY';
    END IF;
    IF NOT EXISTS (
      SELECT 1
      FROM user_memory_scenes scene
      JOIN user_memory_state state ON state.user_id = scene.user_id
        AND state.visibility_epoch = scene.visibility_epoch
        AND state.active_l2_generation = scene.generation
      LEFT JOIN projects project
        ON scene.scope_type = 'project' AND project.id = scene.project_id
       AND project.user_id = scene.user_id AND project.deleted_at IS NULL
       AND project.lifecycle_status = 'active'
       AND project.scope_generation = scene.scope_generation
      JOIN user_memory_derived_search_projections projection
        ON projection.entity_id = scene.id AND projection.user_id = scene.user_id
       AND projection.entity_revision = scene.revision
       AND projection.content_hash = scene.content_hash
       AND projection.source_watermark = scene.source_watermark
       AND projection.generation = scene.generation
       AND projection.lexical_status = 'ready'
       AND projection.embedding_status = 'ready'
      JOIN user_memory_settings settings ON settings.user_id = scene.user_id
      WHERE scene.user_id = p_user_id AND scene.deleted_at IS NULL
        AND NOT scene.user_disabled AND scene.lifecycle_status IN ('shadow','active')
        AND (scene.sensitivity = 'normal' OR settings.sensitive_memory_enabled)
        AND (scene.scope_type = 'global'
          OR (scene.scope_type = 'project' AND project.id IS NOT NULL))
        AND NOT EXISTS (
          SELECT 1 FROM user_memory_scene_members member
          LEFT JOIN user_memories memory
            ON memory.id = member.memory_id AND memory.user_id = member.user_id
          WHERE member.scene_id = scene.id AND (
            memory.id IS NULL OR memory.deleted_at IS NOT NULL OR NOT memory.enabled
            OR memory.lifecycle_status <> 'active'
            OR memory.revision <> member.memory_revision
            OR memory.content_hash <> member.memory_content_hash
            OR memory.scope_type <> scene.scope_type
            OR memory.project_id IS DISTINCT FROM scene.project_id
            OR memory.scope_generation <> scene.scope_generation
            OR memory.visibility_epoch <> scene.visibility_epoch
            OR (memory.valid_from IS NOT NULL AND memory.valid_from > clock_timestamp())
            OR (memory.valid_to IS NOT NULL AND memory.valid_to <= clock_timestamp())
            OR (memory.expires_at IS NOT NULL AND memory.expires_at <= clock_timestamp())
            OR (memory.sensitivity = 'sensitive'
              AND NOT settings.sensitive_memory_enabled)
          )
        )
    ) OR NOT EXISTS (
      SELECT 1
      FROM user_memory_persona_versions persona
      JOIN user_memory_state state ON state.user_id = persona.user_id
        AND state.active_l3_generation = persona.generation
      JOIN user_memory_persona_search_projections projection
        ON projection.entity_id = persona.id AND projection.user_id = persona.user_id
       AND projection.entity_revision = persona.revision
       AND projection.content_hash = persona.content_hash
       AND projection.source_watermark = persona.source_watermark
       AND projection.generation = persona.generation
       AND projection.lexical_status = 'ready'
       AND projection.embedding_status = 'ready'
      WHERE persona.user_id = p_user_id AND NOT persona.user_disabled
        AND memory_l3_persona_version_current(
          persona.id, p_user_id, state.active_l3_generation, false
        )
    ) THEN
      RAISE EXCEPTION USING ERRCODE = '55000',
        MESSAGE = 'MEMORY_SINGLE_USER_DERIVED_READER_PREVIEW_ARTIFACT_NOT_READY';
    END IF;
  END IF;

  INSERT INTO memory_single_user_derived_reader_events(
    event_id, user_id, enabled, reason_code, created_at
  ) VALUES (p_event_id, p_user_id, p_enabled, v_reason, v_now);
  PERFORM memory_l2_scene_reconcile_user(p_user_id);
  PERFORM memory_l3_persona_reconcile_user(p_user_id);
  RETURN jsonb_build_object(
    'eventId', p_event_id::TEXT,
    'userId', p_user_id::TEXT,
    'enabled', p_enabled,
    'mode', 'single_user_preview',
    'replayed', false
  );
END
$function$;



DO $pin_memory_single_user_derived_reader_preview$
DECLARE
  schema_name TEXT := current_schema();
  function_name TEXT;
BEGIN
  ALTER TABLE memory_single_user_derived_reader_events OWNER TO memory_runtime_owner;
  FOREACH function_name IN ARRAY ARRAY[
    'memory_single_user_derived_reader_preview_enabled(uuid)',
    'memory_l2_scene_reconcile_user(uuid)',
    'memory_l2_scene_reader_authority(uuid,uuid,uuid,boolean)',
    'memory_governance_l2_scene_snapshot(uuid)',
    'memory_l3_persona_reconcile_user(uuid)',
    'memory_l3_persona_reader_authority(uuid,uuid,uuid,boolean)',
    'memory_governance_l3_persona_snapshot(uuid)',
    'memory_single_user_derived_reader_artifact_reconcile()',
    'memory_single_user_derived_reader_events_append_only()',
    'memory_single_user_derived_reader_users_changed()',
    'memory_operator_set_single_user_derived_reader_preview(uuid,uuid,boolean,text)'
  ] LOOP
    EXECUTE format('ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',
      schema_name, function_name, schema_name);
    EXECUTE format('ALTER FUNCTION %I.%s OWNER TO memory_runtime_owner',
      schema_name, function_name);
  END LOOP;
END
$pin_memory_single_user_derived_reader_preview$;

-- The NOLOGIN definer role needs only population visibility; application
-- logins still receive no users-table access and cannot assume this role.
GRANT SELECT ON TABLE users TO memory_runtime_owner;
REVOKE ALL ON TABLE memory_single_user_derived_reader_events
  FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_single_user_derived_reader_preview_enabled(UUID)
  FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_single_user_derived_reader_artifact_reconcile()
  FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_single_user_derived_reader_events_append_only()
  FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_single_user_derived_reader_users_changed()
  FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_operator_set_single_user_derived_reader_preview(
  UUID, UUID, BOOLEAN, TEXT
) FROM PUBLIC, go_api_runtime, memory_worker_runtime;
