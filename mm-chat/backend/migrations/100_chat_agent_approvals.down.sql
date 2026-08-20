DO $guard$
BEGIN
  IF EXISTS (SELECT 1 FROM chat_agent_approvals)
     OR EXISTS (SELECT 1 FROM chat_agent_conversation_tool_grants) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'CHAT_AGENT_APPROVALS_DOWN_DATA_EXISTS';
  END IF;
END
$guard$;

REVOKE ALL ON FUNCTION chat_agent_create_approval(
  UUID,UUID,UUID,TEXT,TEXT,TEXT,BOOLEAN,TIMESTAMPTZ,TIMESTAMPTZ
), chat_agent_decide_approval(
  UUID,UUID,BIGINT,TEXT,TIMESTAMPTZ
), chat_agent_recover_approvals(
  TIMESTAMPTZ
) FROM go_api_runtime;
DROP FUNCTION chat_agent_recover_approvals(TIMESTAMPTZ);
DROP FUNCTION chat_agent_decide_approval(UUID,UUID,BIGINT,TEXT,TIMESTAMPTZ);
DROP FUNCTION chat_agent_create_approval(
  UUID,UUID,UUID,TEXT,TEXT,TEXT,BOOLEAN,TIMESTAMPTZ,TIMESTAMPTZ
);
DROP TABLE chat_agent_approvals;
DROP TABLE chat_agent_conversation_tool_grants;
