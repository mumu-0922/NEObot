-- Persisted same-session Goal authority for ordinary Chat Agent turns.
-- Activation remains process-local: a restored active Goal is deliberately
-- disarmed until a direct human turn performs an explicit resume mutation.

CREATE TABLE chat_agent_goals (
  conversation_id UUID PRIMARY KEY,
  id UUID NOT NULL UNIQUE,
  user_id UUID NOT NULL,
  objective TEXT NOT NULL,
  phase TEXT NOT NULL,
  revision BIGINT NOT NULL,
  rounds_started INTEGER NOT NULL DEFAULT 0,
  max_goal_rounds INTEGER NOT NULL,
  blocked_code TEXT,
  blocked_message TEXT,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT chat_agent_goals_conversation_owner_fk
    FOREIGN KEY (conversation_id, user_id)
    REFERENCES conversations(id, user_id) ON DELETE CASCADE,
  CONSTRAINT chat_agent_goals_phase_allowed
    CHECK (phase IN ('active', 'paused', 'blocked', 'complete')),
  CONSTRAINT chat_agent_goals_revision_positive CHECK (revision >= 1),
  CONSTRAINT chat_agent_goals_rounds_valid CHECK (
    rounds_started >= 0 AND max_goal_rounds >= 3
    AND max_goal_rounds <= 32 AND rounds_started <= max_goal_rounds
  ),
  CONSTRAINT chat_agent_goals_objective_bounded CHECK (
    btrim(objective) <> '' AND octet_length(objective) <= 8192
  ),
  CONSTRAINT chat_agent_goals_blocked_shape CHECK (
    (phase = 'blocked' AND blocked_code IS NOT NULL AND blocked_message IS NOT NULL)
    OR (phase <> 'blocked' AND blocked_code IS NULL AND blocked_message IS NULL)
  ),
  CONSTRAINT chat_agent_goals_blocked_code_valid CHECK (
    blocked_code IS NULL OR blocked_code ~ '^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$'
  ),
  CONSTRAINT chat_agent_goals_blocked_message_bounded CHECK (
    blocked_message IS NULL OR (
      btrim(blocked_message) <> '' AND octet_length(blocked_message) <= 4096
    )
  ),
  CONSTRAINT chat_agent_goals_timestamps_order CHECK (updated_at >= created_at)
);

CREATE INDEX idx_chat_agent_goals_user_phase_updated
  ON chat_agent_goals(user_id, phase, updated_at, conversation_id);

CREATE FUNCTION chat_agent_create_goal(
  p_turn_id UUID,
  p_event_id UUID,
  p_goal_id UUID,
  p_objective TEXT,
  p_max_goal_rounds INTEGER,
  p_occurred_at TIMESTAMPTZ
)
RETURNS SETOF chat_agent_goals
LANGUAGE plpgsql
SECURITY DEFINER
AS $function$
DECLARE
  v_turn chat_agent_turns%ROWTYPE;
  v_current chat_agent_goals%ROWTYPE;
  v_goal chat_agent_goals%ROWTYPE;
BEGIN
  IF p_turn_id IS NULL OR p_event_id IS NULL OR p_goal_id IS NULL
     OR p_occurred_at IS NULL OR p_objective IS NULL
     OR btrim(p_objective) = '' OR octet_length(btrim(p_objective)) > 8192
     OR p_max_goal_rounds IS NULL OR p_max_goal_rounds < 3
     OR p_max_goal_rounds > 32 THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_GOAL_INPUT_INVALID';
  END IF;

  SELECT * INTO v_turn
  FROM chat_agent_turns
  WHERE id = p_turn_id
  FOR UPDATE;
  IF NOT FOUND OR v_turn.status <> 'running' THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'CHAT_AGENT_GOAL_TURN_INVALID';
  END IF;

  PERFORM 1
  FROM conversations
  WHERE id = v_turn.conversation_id
    AND user_id = v_turn.user_id
    AND deleted_at IS NULL
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '42501', MESSAGE = 'CHAT_AGENT_GOAL_AUTHORITY_INVALID';
  END IF;

  SELECT * INTO v_current
  FROM chat_agent_goals
  WHERE conversation_id = v_turn.conversation_id
  FOR UPDATE;
  IF FOUND AND v_current.phase <> 'complete' THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'CHAT_AGENT_GOAL_ALREADY_EXISTS';
  END IF;

  INSERT INTO chat_agent_goals (
    conversation_id, id, user_id, objective, phase, revision,
    rounds_started, max_goal_rounds, created_at, updated_at
  ) VALUES (
    v_turn.conversation_id, p_goal_id, v_turn.user_id, btrim(p_objective),
    'active', 1, 0, p_max_goal_rounds, p_occurred_at, p_occurred_at
  )
  ON CONFLICT (conversation_id) DO UPDATE SET
    id = EXCLUDED.id,
    user_id = EXCLUDED.user_id,
    objective = EXCLUDED.objective,
    phase = EXCLUDED.phase,
    revision = EXCLUDED.revision,
    rounds_started = EXCLUDED.rounds_started,
    max_goal_rounds = EXCLUDED.max_goal_rounds,
    blocked_code = NULL,
    blocked_message = NULL,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at
  WHERE chat_agent_goals.phase = 'complete'
  RETURNING * INTO v_goal;

  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'CHAT_AGENT_GOAL_ALREADY_EXISTS';
  END IF;

  PERFORM 1 FROM chat_agent_append_event(
    p_turn_id,
    p_event_id,
    'goal.changed',
    NULL,
    jsonb_build_object(
      'operation', 'create',
      'goal', jsonb_build_object(
        'id', v_goal.id,
        'revision', v_goal.revision,
        'objective', v_goal.objective,
        'phase', v_goal.phase,
        'roundsStarted', v_goal.rounds_started,
        'maxGoalRounds', v_goal.max_goal_rounds
      )
    ),
    p_occurred_at
  );

  RETURN NEXT v_goal;
END
$function$;

CREATE FUNCTION chat_agent_change_goal(
  p_turn_id UUID,
  p_event_id UUID,
  p_goal_id UUID,
  p_expected_revision BIGINT,
  p_action TEXT,
  p_objective TEXT,
  p_max_goal_rounds INTEGER,
  p_blocked_reason TEXT,
  p_occurred_at TIMESTAMPTZ
)
RETURNS SETOF chat_agent_goals
LANGUAGE plpgsql
SECURITY DEFINER
AS $function$
DECLARE
  v_turn chat_agent_turns%ROWTYPE;
  v_goal chat_agent_goals%ROWTYPE;
BEGIN
  IF p_turn_id IS NULL OR p_event_id IS NULL OR p_goal_id IS NULL
     OR p_expected_revision IS NULL OR p_expected_revision < 1
     OR p_action IS NULL OR p_action NOT IN (
       'edit', 'pause', 'resume', 'complete', 'blocked'
     ) OR p_occurred_at IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_GOAL_INPUT_INVALID';
  END IF;

  SELECT * INTO v_turn
  FROM chat_agent_turns
  WHERE id = p_turn_id
  FOR UPDATE;
  IF NOT FOUND OR v_turn.status <> 'running' THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'CHAT_AGENT_GOAL_TURN_INVALID';
  END IF;

  SELECT * INTO v_goal
  FROM chat_agent_goals
  WHERE conversation_id = v_turn.conversation_id
    AND user_id = v_turn.user_id
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_GOAL_NOT_FOUND';
  END IF;
  IF v_goal.id <> p_goal_id OR v_goal.revision <> p_expected_revision THEN
    RAISE EXCEPTION USING ERRCODE = '40001', MESSAGE = 'CHAT_AGENT_GOAL_STALE_REVISION';
  END IF;

  CASE p_action
    WHEN 'edit' THEN
      IF (p_objective IS NULL OR btrim(p_objective) = '')
         AND p_max_goal_rounds IS NULL THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_GOAL_EDIT_INVALID';
      END IF;
      IF p_objective IS NOT NULL AND (
        btrim(p_objective) = '' OR octet_length(btrim(p_objective)) > 8192
      ) THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_GOAL_INPUT_INVALID';
      END IF;
      IF p_max_goal_rounds IS NOT NULL AND (
        p_max_goal_rounds < 3 OR p_max_goal_rounds > 32
        OR p_max_goal_rounds < v_goal.rounds_started
      ) THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_GOAL_ROUND_LIMIT_INVALID';
      END IF;
      UPDATE chat_agent_goals
      SET objective = COALESCE(NULLIF(btrim(p_objective), ''), objective),
          max_goal_rounds = COALESCE(p_max_goal_rounds, max_goal_rounds),
          revision = revision + 1,
          updated_at = GREATEST(updated_at, p_occurred_at)
      WHERE conversation_id = v_goal.conversation_id
      RETURNING * INTO v_goal;
    WHEN 'pause' THEN
      IF v_goal.phase <> 'active' THEN
        RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'CHAT_AGENT_GOAL_TRANSITION_INVALID';
      END IF;
      UPDATE chat_agent_goals
      SET phase = 'paused', revision = revision + 1,
          blocked_code = NULL, blocked_message = NULL,
          updated_at = GREATEST(updated_at, p_occurred_at)
      WHERE conversation_id = v_goal.conversation_id
      RETURNING * INTO v_goal;
    WHEN 'resume' THEN
      IF v_goal.phase NOT IN ('active', 'paused', 'blocked')
         OR v_goal.rounds_started >= v_goal.max_goal_rounds THEN
        RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'CHAT_AGENT_GOAL_TRANSITION_INVALID';
      END IF;
      UPDATE chat_agent_goals
      SET phase = 'active', revision = revision + 1,
          blocked_code = NULL, blocked_message = NULL,
          updated_at = GREATEST(updated_at, p_occurred_at)
      WHERE conversation_id = v_goal.conversation_id
      RETURNING * INTO v_goal;
    WHEN 'complete' THEN
      IF v_goal.phase NOT IN ('active', 'paused', 'blocked') THEN
        RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'CHAT_AGENT_GOAL_TRANSITION_INVALID';
      END IF;
      UPDATE chat_agent_goals
      SET phase = 'complete', revision = revision + 1,
          blocked_code = NULL, blocked_message = NULL,
          updated_at = GREATEST(updated_at, p_occurred_at)
      WHERE conversation_id = v_goal.conversation_id
      RETURNING * INTO v_goal;
    WHEN 'blocked' THEN
      IF v_goal.phase <> 'active' OR p_blocked_reason IS NULL
         OR btrim(p_blocked_reason) = ''
         OR octet_length(btrim(p_blocked_reason)) > 4096 THEN
        RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'CHAT_AGENT_GOAL_BLOCK_INVALID';
      END IF;
      UPDATE chat_agent_goals
      SET phase = 'blocked', revision = revision + 1,
          blocked_code = 'model-reported',
          blocked_message = btrim(p_blocked_reason),
          updated_at = GREATEST(updated_at, p_occurred_at)
      WHERE conversation_id = v_goal.conversation_id
      RETURNING * INTO v_goal;
  END CASE;

  PERFORM 1 FROM chat_agent_append_event(
    p_turn_id,
    p_event_id,
    'goal.changed',
    NULL,
    jsonb_strip_nulls(jsonb_build_object(
      'operation', p_action,
      'goal', jsonb_build_object(
        'id', v_goal.id,
        'revision', v_goal.revision,
        'objective', v_goal.objective,
        'phase', v_goal.phase,
        'roundsStarted', v_goal.rounds_started,
        'maxGoalRounds', v_goal.max_goal_rounds,
        'blockedReason', CASE WHEN v_goal.phase = 'blocked' THEN
          jsonb_build_object(
            'code', v_goal.blocked_code,
            'message', v_goal.blocked_message
          )
        ELSE NULL END
      )
    )),
    p_occurred_at
  );

  RETURN NEXT v_goal;
END
$function$;

CREATE FUNCTION chat_agent_cancel_goal(
  p_turn_id UUID,
  p_event_id UUID,
  p_goal_id UUID,
  p_expected_revision BIGINT,
  p_occurred_at TIMESTAMPTZ
)
RETURNS TABLE (goal_id UUID, revision BIGINT)
LANGUAGE plpgsql
SECURITY DEFINER
AS $function$
DECLARE
  v_turn chat_agent_turns%ROWTYPE;
  v_goal chat_agent_goals%ROWTYPE;
  v_cleared_revision BIGINT;
BEGIN
  IF p_turn_id IS NULL OR p_event_id IS NULL OR p_goal_id IS NULL
     OR p_expected_revision IS NULL OR p_expected_revision < 1
     OR p_occurred_at IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_GOAL_INPUT_INVALID';
  END IF;

  SELECT * INTO v_turn
  FROM chat_agent_turns
  WHERE id = p_turn_id
  FOR UPDATE;
  IF NOT FOUND OR v_turn.status <> 'running' THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'CHAT_AGENT_GOAL_TURN_INVALID';
  END IF;

  SELECT * INTO v_goal
  FROM chat_agent_goals
  WHERE conversation_id = v_turn.conversation_id
    AND user_id = v_turn.user_id
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_GOAL_NOT_FOUND';
  END IF;
  IF v_goal.id <> p_goal_id OR v_goal.revision <> p_expected_revision THEN
    RAISE EXCEPTION USING ERRCODE = '40001', MESSAGE = 'CHAT_AGENT_GOAL_STALE_REVISION';
  END IF;

  v_cleared_revision := v_goal.revision + 1;
  DELETE FROM chat_agent_goals WHERE conversation_id = v_goal.conversation_id;

  PERFORM 1 FROM chat_agent_append_event(
    p_turn_id,
    p_event_id,
    'goal.changed',
    NULL,
    jsonb_build_object(
      'operation', 'cancel',
      'goal', NULL,
      'cleared', jsonb_build_object(
        'id', v_goal.id,
        'revision', v_cleared_revision
      )
    ),
    p_occurred_at
  );

  RETURN QUERY SELECT v_goal.id, v_cleared_revision;
END
$function$;

CREATE FUNCTION chat_agent_start_goal_round(
  p_turn_id UUID,
  p_event_id UUID,
  p_goal_id UUID,
  p_expected_revision BIGINT,
  p_round INTEGER,
  p_occurred_at TIMESTAMPTZ
)
RETURNS SETOF chat_agent_goals
LANGUAGE plpgsql
SECURITY DEFINER
AS $function$
DECLARE
  v_turn chat_agent_turns%ROWTYPE;
  v_goal chat_agent_goals%ROWTYPE;
BEGIN
  IF p_turn_id IS NULL OR p_event_id IS NULL OR p_goal_id IS NULL
     OR p_expected_revision IS NULL OR p_expected_revision < 1
     OR p_round IS NULL OR p_round < 1 OR p_occurred_at IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_GOAL_INPUT_INVALID';
  END IF;

  SELECT * INTO v_turn
  FROM chat_agent_turns
  WHERE id = p_turn_id
  FOR UPDATE;
  IF NOT FOUND OR v_turn.status <> 'running' THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'CHAT_AGENT_GOAL_TURN_INVALID';
  END IF;

  SELECT * INTO v_goal
  FROM chat_agent_goals
  WHERE conversation_id = v_turn.conversation_id
    AND user_id = v_turn.user_id
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_GOAL_NOT_FOUND';
  END IF;
  IF v_goal.id <> p_goal_id OR v_goal.revision <> p_expected_revision THEN
    RAISE EXCEPTION USING ERRCODE = '40001', MESSAGE = 'CHAT_AGENT_GOAL_STALE_REVISION';
  END IF;
  IF v_goal.phase <> 'active' OR p_round <> v_goal.rounds_started + 1
     OR p_round > v_goal.max_goal_rounds THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'CHAT_AGENT_GOAL_ROUND_INVALID';
  END IF;

  UPDATE chat_agent_goals
  SET rounds_started = p_round,
      updated_at = GREATEST(updated_at, p_occurred_at)
  WHERE conversation_id = v_goal.conversation_id
  RETURNING * INTO v_goal;

  PERFORM 1 FROM chat_agent_append_event(
    p_turn_id,
    p_event_id,
    'goal.round.started',
    NULL,
    jsonb_build_object(
      'goalId', v_goal.id,
      'revision', v_goal.revision,
      'round', v_goal.rounds_started,
      'maxGoalRounds', v_goal.max_goal_rounds
    ),
    p_occurred_at
  );

  RETURN NEXT v_goal;
END
$function$;

DO $function_paths$
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.chat_agent_create_goal(UUID,UUID,UUID,TEXT,INTEGER,TIMESTAMPTZ) SET search_path TO %I, pg_catalog, pg_temp',
    current_schema(), current_schema()
  );
  EXECUTE format(
    'ALTER FUNCTION %I.chat_agent_change_goal(UUID,UUID,UUID,BIGINT,TEXT,TEXT,INTEGER,TEXT,TIMESTAMPTZ) SET search_path TO %I, pg_catalog, pg_temp',
    current_schema(), current_schema()
  );
  EXECUTE format(
    'ALTER FUNCTION %I.chat_agent_cancel_goal(UUID,UUID,UUID,BIGINT,TIMESTAMPTZ) SET search_path TO %I, pg_catalog, pg_temp',
    current_schema(), current_schema()
  );
  EXECUTE format(
    'ALTER FUNCTION %I.chat_agent_start_goal_round(UUID,UUID,UUID,BIGINT,INTEGER,TIMESTAMPTZ) SET search_path TO %I, pg_catalog, pg_temp',
    current_schema(), current_schema()
  );
END
$function_paths$;

REVOKE ALL ON chat_agent_goals FROM PUBLIC, go_api_runtime;
GRANT SELECT ON chat_agent_goals TO go_api_runtime;

REVOKE ALL ON FUNCTION chat_agent_create_goal(
  UUID,UUID,UUID,TEXT,INTEGER,TIMESTAMPTZ
), chat_agent_change_goal(
  UUID,UUID,UUID,BIGINT,TEXT,TEXT,INTEGER,TEXT,TIMESTAMPTZ
), chat_agent_cancel_goal(
  UUID,UUID,UUID,BIGINT,TIMESTAMPTZ
), chat_agent_start_goal_round(
  UUID,UUID,UUID,BIGINT,INTEGER,TIMESTAMPTZ
) FROM PUBLIC, go_api_runtime;

GRANT EXECUTE ON FUNCTION chat_agent_create_goal(
  UUID,UUID,UUID,TEXT,INTEGER,TIMESTAMPTZ
), chat_agent_change_goal(
  UUID,UUID,UUID,BIGINT,TEXT,TEXT,INTEGER,TEXT,TIMESTAMPTZ
), chat_agent_cancel_goal(
  UUID,UUID,UUID,BIGINT,TIMESTAMPTZ
), chat_agent_start_goal_round(
  UUID,UUID,UUID,BIGINT,INTEGER,TIMESTAMPTZ
) TO go_api_runtime;

DO $schema_acl$
BEGIN
  EXECUTE format('GRANT USAGE ON SCHEMA %I TO go_api_runtime', current_schema());
  EXECUTE format('REVOKE CREATE ON SCHEMA %I FROM go_api_runtime', current_schema());
END
$schema_acl$;
