-- Durable, append-only event authority for ordinary Chat Agent turns. This is
-- intentionally separate from the optional G20/G21 agent_run_events control
-- plane: Chat history, live process projection, and restart recovery share
-- this log without widening control-plane state.

CREATE TABLE chat_agent_turns (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  conversation_id UUID NOT NULL,
  message_id UUID NOT NULL,
  run_id UUID NOT NULL,
  status TEXT NOT NULL DEFAULT 'running',
  next_sequence BIGINT NOT NULL DEFAULT 1,
  started_at TIMESTAMPTZ NOT NULL,
  ended_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT chat_agent_turns_conversation_owner_fk
    FOREIGN KEY (conversation_id, user_id)
    REFERENCES conversations(id, user_id) ON DELETE CASCADE,
  CONSTRAINT chat_agent_turns_message_owner_fk
    FOREIGN KEY (message_id, user_id)
    REFERENCES messages(id, user_id) ON DELETE CASCADE,
  CONSTRAINT chat_agent_turns_message_unique UNIQUE (message_id),
  CONSTRAINT chat_agent_turns_run_unique UNIQUE (run_id),
  CONSTRAINT chat_agent_turns_status_allowed
    CHECK (status IN ('running', 'completed', 'failed', 'cancelled', 'interrupted')),
  CONSTRAINT chat_agent_turns_next_sequence_positive CHECK (next_sequence >= 1),
  CONSTRAINT chat_agent_turns_timestamps_order
    CHECK (updated_at >= created_at AND started_at >= created_at),
  CONSTRAINT chat_agent_turns_ended_after_started
    CHECK (ended_at IS NULL OR ended_at >= started_at),
  CONSTRAINT chat_agent_turns_terminal_shape CHECK (
    (status = 'running' AND ended_at IS NULL)
    OR (status <> 'running' AND ended_at IS NOT NULL)
  )
);

CREATE TABLE chat_agent_events (
  event_id UUID PRIMARY KEY,
  turn_id UUID NOT NULL REFERENCES chat_agent_turns(id) ON DELETE CASCADE,
  sequence BIGINT NOT NULL,
  event_type TEXT NOT NULL,
  step_sequence INTEGER,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  occurred_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT chat_agent_events_turn_sequence_unique UNIQUE (turn_id, sequence),
  CONSTRAINT chat_agent_events_turn_event_unique UNIQUE (turn_id, event_id),
  CONSTRAINT chat_agent_events_sequence_positive CHECK (sequence >= 1),
  CONSTRAINT chat_agent_events_step_sequence_positive
    CHECK (step_sequence IS NULL OR step_sequence >= 1),
  CONSTRAINT chat_agent_events_type_allowed CHECK (event_type IN (
    'turn.started', 'turn.ended',
    'step.started', 'step.ended',
    'assistant.message',
    'tool.called', 'tool.result',
    'goal.changed', 'goal.round.started',
    'context.replaced'
  )),
  CONSTRAINT chat_agent_events_payload_object CHECK (jsonb_typeof(payload) = 'object'),
  CONSTRAINT chat_agent_events_payload_bounded
    CHECK (octet_length(payload::text) <= 262144),
  CONSTRAINT chat_agent_events_occurred_after_turn_created
    CHECK (occurred_at >= created_at - interval '5 minutes')
);

CREATE INDEX idx_chat_agent_turns_user_conversation_started
  ON chat_agent_turns(user_id, conversation_id, started_at, id);
CREATE INDEX idx_chat_agent_turns_running_started
  ON chat_agent_turns(started_at, id) WHERE status = 'running';
CREATE INDEX idx_chat_agent_events_turn_sequence
  ON chat_agent_events(turn_id, sequence);

CREATE FUNCTION chat_agent_event_immutable()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $function$
BEGIN
  IF TG_OP = 'DELETE' AND NOT EXISTS (
    SELECT 1 FROM chat_agent_turns WHERE id = OLD.turn_id
  ) THEN
    -- Parent/user/conversation/message hard deletion owns the only removal
    -- path. Direct event deletion still sees its Turn and remains forbidden.
    RETURN OLD;
  END IF;
  RAISE EXCEPTION USING
    ERRCODE = '55000',
    MESSAGE = 'CHAT_AGENT_EVENT_IMMUTABLE';
END
$function$;

CREATE TRIGGER trg_chat_agent_event_immutable
BEFORE UPDATE OR DELETE ON chat_agent_events
FOR EACH ROW EXECUTE FUNCTION chat_agent_event_immutable();

CREATE FUNCTION chat_agent_start_turn(
  p_turn_id UUID,
  p_event_id UUID,
  p_user_id UUID,
  p_conversation_id UUID,
  p_message_id UUID,
  p_run_id UUID,
  p_occurred_at TIMESTAMPTZ
)
RETURNS TABLE (
  event_id UUID,
  turn_id UUID,
  user_id UUID,
  conversation_id UUID,
  message_id UUID,
  run_id UUID,
  sequence BIGINT,
  event_type TEXT,
  step_sequence INTEGER,
  payload JSONB,
  occurred_at TIMESTAMPTZ
)
LANGUAGE plpgsql
SECURITY DEFINER
AS $function$
DECLARE
  v_turn chat_agent_turns%ROWTYPE;
  v_event chat_agent_events%ROWTYPE;
  v_inserted BOOLEAN := FALSE;
BEGIN
  IF p_turn_id IS NULL OR p_event_id IS NULL OR p_user_id IS NULL
     OR p_conversation_id IS NULL OR p_message_id IS NULL OR p_run_id IS NULL
     OR p_occurred_at IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_TURN_INPUT_INVALID';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM messages AS message
    JOIN conversations AS conversation
      ON conversation.id = message.conversation_id
     AND conversation.user_id = p_user_id
     AND conversation.deleted_at IS NULL
    WHERE message.id = p_message_id
      AND message.conversation_id = p_conversation_id
      AND message.user_id = p_user_id
      AND message.role = 'assistant'
      AND message.deleted_at IS NULL
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '42501', MESSAGE = 'CHAT_AGENT_TURN_AUTHORITY_INVALID';
  END IF;

  INSERT INTO chat_agent_turns (
    id, user_id, conversation_id, message_id, run_id,
    status, next_sequence, started_at, created_at, updated_at
  ) VALUES (
    p_turn_id, p_user_id, p_conversation_id, p_message_id, p_run_id,
    'running', 2, p_occurred_at, p_occurred_at, p_occurred_at
  )
  ON CONFLICT (id) DO NOTHING
  RETURNING * INTO v_turn;
  v_inserted := FOUND;

  IF NOT v_inserted THEN
    SELECT * INTO v_turn
    FROM chat_agent_turns
    WHERE id = p_turn_id
    FOR UPDATE;
    IF NOT FOUND
       OR v_turn.user_id <> p_user_id
       OR v_turn.conversation_id <> p_conversation_id
       OR v_turn.message_id <> p_message_id
       OR v_turn.run_id <> p_run_id THEN
      RAISE EXCEPTION USING ERRCODE = '23505', MESSAGE = 'CHAT_AGENT_TURN_REPLAY_CONFLICT';
    END IF;
  END IF;

  INSERT INTO chat_agent_events (
    event_id, turn_id, sequence, event_type, payload, occurred_at, created_at
  ) VALUES (
    p_event_id, p_turn_id, 1, 'turn.started', '{"status":"running"}'::jsonb,
    p_occurred_at, p_occurred_at
  )
  ON CONFLICT (event_id) DO NOTHING
  RETURNING * INTO v_event;

  IF NOT FOUND THEN
    SELECT event.* INTO v_event
    FROM chat_agent_events AS event
    WHERE event.event_id = p_event_id;
    IF NOT FOUND OR v_event.turn_id <> p_turn_id OR v_event.sequence <> 1
       OR v_event.event_type <> 'turn.started' THEN
      RAISE EXCEPTION USING ERRCODE = '23505', MESSAGE = 'CHAT_AGENT_EVENT_REPLAY_CONFLICT';
    END IF;
  END IF;

  RETURN QUERY SELECT
    v_event.event_id, v_turn.id, v_turn.user_id, v_turn.conversation_id,
    v_turn.message_id, v_turn.run_id, v_event.sequence, v_event.event_type,
    v_event.step_sequence, v_event.payload, v_event.occurred_at;
END
$function$;

CREATE FUNCTION chat_agent_append_event(
  p_turn_id UUID,
  p_event_id UUID,
  p_event_type TEXT,
  p_step_sequence INTEGER,
  p_payload JSONB,
  p_occurred_at TIMESTAMPTZ
)
RETURNS TABLE (
  event_id UUID,
  turn_id UUID,
  user_id UUID,
  conversation_id UUID,
  message_id UUID,
  run_id UUID,
  sequence BIGINT,
  event_type TEXT,
  step_sequence INTEGER,
  payload JSONB,
  occurred_at TIMESTAMPTZ
)
LANGUAGE plpgsql
SECURITY DEFINER
AS $function$
DECLARE
  v_turn chat_agent_turns%ROWTYPE;
  v_event chat_agent_events%ROWTYPE;
  v_sequence BIGINT;
  v_terminal_status TEXT;
BEGIN
  IF p_turn_id IS NULL OR p_event_id IS NULL OR p_occurred_at IS NULL
     OR p_event_type IS NULL OR btrim(p_event_type) = ''
     OR p_payload IS NULL OR jsonb_typeof(p_payload) <> 'object'
     OR octet_length(p_payload::text) > 262144 THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_EVENT_INPUT_INVALID';
  END IF;
  IF p_event_type = 'turn.started' OR p_event_type NOT IN (
    'turn.ended', 'step.started', 'step.ended', 'assistant.message',
    'tool.called', 'tool.result', 'goal.changed', 'goal.round.started',
    'context.replaced'
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_EVENT_TYPE_INVALID';
  END IF;
  IF p_step_sequence IS NOT NULL AND p_step_sequence < 1 THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_EVENT_STEP_INVALID';
  END IF;

  SELECT * INTO v_turn
  FROM chat_agent_turns
  WHERE id = p_turn_id
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_TURN_NOT_FOUND';
  END IF;

  SELECT event.* INTO v_event
  FROM chat_agent_events AS event
  WHERE event.event_id = p_event_id;
  IF FOUND THEN
    IF v_event.turn_id <> p_turn_id OR v_event.event_type <> p_event_type
       OR v_event.step_sequence IS DISTINCT FROM p_step_sequence
       OR v_event.payload <> p_payload THEN
      RAISE EXCEPTION USING ERRCODE = '23505', MESSAGE = 'CHAT_AGENT_EVENT_REPLAY_CONFLICT';
    END IF;
    RETURN QUERY SELECT
      v_event.event_id, v_turn.id, v_turn.user_id, v_turn.conversation_id,
      v_turn.message_id, v_turn.run_id, v_event.sequence, v_event.event_type,
      v_event.step_sequence, v_event.payload, v_event.occurred_at;
    RETURN;
  END IF;

  IF v_turn.status <> 'running' THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'CHAT_AGENT_TURN_TERMINAL';
  END IF;

  v_sequence := v_turn.next_sequence;
  UPDATE chat_agent_turns
  SET next_sequence = next_sequence + 1, updated_at = GREATEST(updated_at, p_occurred_at)
  WHERE id = p_turn_id;

  INSERT INTO chat_agent_events (
    event_id, turn_id, sequence, event_type, step_sequence,
    payload, occurred_at, created_at
  ) VALUES (
    p_event_id, p_turn_id, v_sequence, p_event_type, p_step_sequence,
    p_payload, p_occurred_at, p_occurred_at
  )
  RETURNING * INTO v_event;

  IF p_event_type = 'turn.ended' THEN
    v_terminal_status := p_payload->>'status';
    IF v_terminal_status IS NULL
       OR v_terminal_status NOT IN ('completed', 'failed', 'cancelled', 'interrupted') THEN
      RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_TURN_STATUS_INVALID';
    END IF;
    UPDATE chat_agent_turns
    SET status = v_terminal_status,
        ended_at = p_occurred_at,
        updated_at = GREATEST(updated_at, p_occurred_at)
    WHERE id = p_turn_id;

    IF v_terminal_status = 'interrupted' THEN
      UPDATE messages
      SET status = 'failed',
          error_code = COALESCE(error_code, 'AGENT_RUN_INTERRUPTED'),
          metadata = COALESCE(metadata, '{}'::jsonb)
            || '{"errorCode":"AGENT_RUN_INTERRUPTED"}'::jsonb,
          completed_at = COALESCE(completed_at, p_occurred_at),
          updated_at = GREATEST(updated_at, p_occurred_at)
      WHERE id = v_turn.message_id
        AND conversation_id = v_turn.conversation_id
        AND user_id = v_turn.user_id
        AND status IN ('pending', 'streaming');
    END IF;
  END IF;

  RETURN QUERY SELECT
    v_event.event_id, v_turn.id, v_turn.user_id, v_turn.conversation_id,
    v_turn.message_id, v_turn.run_id, v_event.sequence, v_event.event_type,
    v_event.step_sequence, v_event.payload, v_event.occurred_at;
END
$function$;

DO $function_paths$
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.chat_agent_event_immutable() SET search_path TO %I, pg_catalog, pg_temp',
    current_schema(), current_schema()
  );
  EXECUTE format(
    'ALTER FUNCTION %I.chat_agent_start_turn(UUID,UUID,UUID,UUID,UUID,UUID,TIMESTAMPTZ) SET search_path TO %I, pg_catalog, pg_temp',
    current_schema(), current_schema()
  );
  EXECUTE format(
    'ALTER FUNCTION %I.chat_agent_append_event(UUID,UUID,TEXT,INTEGER,JSONB,TIMESTAMPTZ) SET search_path TO %I, pg_catalog, pg_temp',
    current_schema(), current_schema()
  );
END
$function_paths$;

REVOKE ALL ON chat_agent_turns, chat_agent_events FROM PUBLIC, go_api_runtime;
GRANT SELECT ON chat_agent_turns, chat_agent_events TO go_api_runtime;

REVOKE ALL ON FUNCTION chat_agent_start_turn(
  UUID,UUID,UUID,UUID,UUID,UUID,TIMESTAMPTZ
), chat_agent_append_event(
  UUID,UUID,TEXT,INTEGER,JSONB,TIMESTAMPTZ
), chat_agent_event_immutable()
FROM PUBLIC, go_api_runtime;
GRANT EXECUTE ON FUNCTION chat_agent_start_turn(
  UUID,UUID,UUID,UUID,UUID,UUID,TIMESTAMPTZ
), chat_agent_append_event(
  UUID,UUID,TEXT,INTEGER,JSONB,TIMESTAMPTZ
) TO go_api_runtime;

DO $schema_acl$
BEGIN
  EXECUTE format('GRANT USAGE ON SCHEMA %I TO go_api_runtime', current_schema());
  EXECUTE format('REVOKE CREATE ON SCHEMA %I FROM go_api_runtime', current_schema());
END
$schema_acl$;
