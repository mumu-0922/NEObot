DO $guard$
BEGIN
  IF EXISTS (SELECT 1 FROM chat_agent_events)
     OR EXISTS (SELECT 1 FROM chat_agent_turns) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'CHAT_AGENT_EVENT_LOG_DOWN_DATA_EXISTS';
  END IF;
END
$guard$;

REVOKE ALL ON FUNCTION chat_agent_start_turn(
  UUID,UUID,UUID,UUID,UUID,UUID,TIMESTAMPTZ
), chat_agent_append_event(
  UUID,UUID,TEXT,INTEGER,JSONB,TIMESTAMPTZ
) FROM go_api_runtime;
REVOKE ALL ON chat_agent_turns, chat_agent_events FROM go_api_runtime;

DROP FUNCTION chat_agent_append_event(UUID,UUID,TEXT,INTEGER,JSONB,TIMESTAMPTZ);
DROP FUNCTION chat_agent_start_turn(UUID,UUID,UUID,UUID,UUID,UUID,TIMESTAMPTZ);
DROP TRIGGER trg_chat_agent_event_immutable ON chat_agent_events;
DROP FUNCTION chat_agent_event_immutable();
DROP TABLE chat_agent_events;
DROP TABLE chat_agent_turns;
