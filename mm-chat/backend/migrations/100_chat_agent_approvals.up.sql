-- Durable CAS authority for interactive Chat Agent Tool approvals. Raw Tool
-- arguments and results never enter these rows; the append-only Agent event log
-- remains the presentation/replay authority.

CREATE TABLE chat_agent_conversation_tool_grants (
  conversation_id UUID NOT NULL,
  user_id UUID NOT NULL,
  tool_name TEXT NOT NULL,
  risk_class TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (conversation_id, tool_name, risk_class),
  CONSTRAINT chat_agent_conversation_tool_grants_owner_fk
    FOREIGN KEY (conversation_id, user_id)
    REFERENCES conversations(id, user_id) ON DELETE CASCADE,
  CONSTRAINT chat_agent_conversation_tool_grants_tool_bounded
    CHECK (btrim(tool_name) <> '' AND octet_length(tool_name) <= 128),
  CONSTRAINT chat_agent_conversation_tool_grants_risk_allowed
    CHECK (risk_class IN ('write', 'execute', 'external'))
);

CREATE TABLE chat_agent_approvals (
  id UUID PRIMARY KEY,
  turn_id UUID NOT NULL REFERENCES chat_agent_turns(id) ON DELETE CASCADE,
  user_id UUID NOT NULL,
  conversation_id UUID NOT NULL,
  message_id UUID NOT NULL,
  run_id UUID NOT NULL,
  execution_id TEXT NOT NULL,
  tool_name TEXT NOT NULL,
  risk_class TEXT NOT NULL,
  status TEXT NOT NULL,
  decision TEXT,
  revision BIGINT NOT NULL,
  allow_conversation BOOLEAN NOT NULL DEFAULT FALSE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  decided_at TIMESTAMPTZ,
  CONSTRAINT chat_agent_approvals_turn_execution_unique UNIQUE (turn_id, execution_id),
  CONSTRAINT chat_agent_approvals_conversation_owner_fk
    FOREIGN KEY (conversation_id, user_id)
    REFERENCES conversations(id, user_id) ON DELETE CASCADE,
  CONSTRAINT chat_agent_approvals_message_owner_fk
    FOREIGN KEY (message_id, user_id)
    REFERENCES messages(id, user_id) ON DELETE CASCADE,
  CONSTRAINT chat_agent_approvals_status_allowed
    CHECK (status IN ('pending', 'allowed', 'denied', 'expired')),
  CONSTRAINT chat_agent_approvals_decision_allowed
    CHECK (decision IS NULL OR decision IN (
      'allow_once', 'allow_conversation', 'deny', 'expired', 'restart_denied'
    )),
  CONSTRAINT chat_agent_approvals_revision_positive CHECK (revision >= 1),
  CONSTRAINT chat_agent_approvals_execution_bounded
    CHECK (btrim(execution_id) <> '' AND octet_length(execution_id) <= 256),
  CONSTRAINT chat_agent_approvals_tool_bounded
    CHECK (btrim(tool_name) <> '' AND octet_length(tool_name) <= 128),
  CONSTRAINT chat_agent_approvals_risk_allowed
    CHECK (risk_class IN ('write', 'execute', 'external')),
  CONSTRAINT chat_agent_approvals_expiry_bounded CHECK (
    expires_at > created_at AND expires_at <= created_at + interval '5 minutes'
  ),
  CONSTRAINT chat_agent_approvals_decision_shape CHECK (
    (status = 'pending' AND decision IS NULL AND decided_at IS NULL)
    OR (status <> 'pending' AND decision IS NOT NULL AND decided_at IS NOT NULL)
  ),
  CONSTRAINT chat_agent_approvals_decision_matches_status CHECK (
    (status = 'pending' AND decision IS NULL)
    OR (status = 'allowed' AND decision IN ('allow_once', 'allow_conversation'))
    OR (status = 'denied' AND decision IN ('deny', 'restart_denied'))
    OR (status = 'expired' AND decision = 'expired')
  ),
  CONSTRAINT chat_agent_approvals_conversation_policy_shape CHECK (
    decision <> 'allow_conversation' OR allow_conversation
  )
);

CREATE INDEX idx_chat_agent_approvals_user_pending
  ON chat_agent_approvals(user_id, expires_at, id) WHERE status = 'pending';

CREATE FUNCTION chat_agent_create_approval(
  p_approval_id UUID,
  p_turn_id UUID,
  p_user_id UUID,
  p_execution_id TEXT,
  p_tool_name TEXT,
  p_risk_class TEXT,
  p_allow_conversation BOOLEAN,
  p_expires_at TIMESTAMPTZ,
  p_occurred_at TIMESTAMPTZ
)
RETURNS SETOF chat_agent_approvals
LANGUAGE plpgsql
SECURITY DEFINER
AS $function$
DECLARE
  v_turn chat_agent_turns%ROWTYPE;
  v_approval chat_agent_approvals%ROWTYPE;
  v_granted BOOLEAN := FALSE;
BEGIN
  IF p_approval_id IS NULL OR p_turn_id IS NULL OR p_user_id IS NULL
     OR p_execution_id IS NULL OR btrim(p_execution_id) = ''
     OR octet_length(btrim(p_execution_id)) > 256
     OR p_tool_name IS NULL OR btrim(p_tool_name) = ''
     OR octet_length(btrim(p_tool_name)) > 128
     OR p_risk_class NOT IN ('write', 'execute', 'external')
     OR p_allow_conversation IS NULL OR p_expires_at IS NULL
     OR p_occurred_at IS NULL OR p_expires_at <= p_occurred_at
     OR p_expires_at > p_occurred_at + interval '5 minutes' THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_APPROVAL_INPUT_INVALID';
  END IF;

  SELECT * INTO v_turn
  FROM chat_agent_turns
  WHERE id = p_turn_id
  FOR UPDATE;
  IF NOT FOUND OR v_turn.status <> 'running' THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'CHAT_AGENT_APPROVAL_TURN_INVALID';
  END IF;
  IF v_turn.user_id <> p_user_id THEN
    RAISE EXCEPTION USING ERRCODE = '42501', MESSAGE = 'CHAT_AGENT_APPROVAL_AUTHORITY_INVALID';
  END IF;

  SELECT EXISTS (
    SELECT 1
    FROM chat_agent_conversation_tool_grants AS grant_row
    JOIN conversations AS conversation
      ON conversation.id = grant_row.conversation_id
     AND conversation.user_id = grant_row.user_id
     AND conversation.deleted_at IS NULL
    WHERE grant_row.conversation_id = v_turn.conversation_id
      AND grant_row.user_id = v_turn.user_id
      AND grant_row.tool_name = btrim(p_tool_name)
      AND grant_row.risk_class = p_risk_class
  ) INTO v_granted;

  INSERT INTO chat_agent_approvals (
    id, turn_id, user_id, conversation_id, message_id, run_id,
    execution_id, tool_name, risk_class, status, decision, revision,
    allow_conversation, expires_at, created_at, decided_at
  ) VALUES (
    p_approval_id, v_turn.id, v_turn.user_id, v_turn.conversation_id,
    v_turn.message_id, v_turn.run_id, btrim(p_execution_id), btrim(p_tool_name),
    p_risk_class, CASE WHEN v_granted THEN 'allowed' ELSE 'pending' END,
    CASE WHEN v_granted THEN 'allow_conversation' ELSE NULL END,
    1, p_allow_conversation, p_expires_at, p_occurred_at,
    CASE WHEN v_granted THEN p_occurred_at ELSE NULL END
  )
  ON CONFLICT (turn_id, execution_id) DO NOTHING
  RETURNING * INTO v_approval;

  IF NOT FOUND THEN
    SELECT * INTO v_approval
    FROM chat_agent_approvals
    WHERE turn_id = v_turn.id AND execution_id = btrim(p_execution_id);
    IF NOT FOUND OR v_approval.user_id <> v_turn.user_id
       OR v_approval.tool_name <> btrim(p_tool_name)
       OR v_approval.risk_class <> p_risk_class THEN
      RAISE EXCEPTION USING ERRCODE = '23505', MESSAGE = 'CHAT_AGENT_APPROVAL_REPLAY_CONFLICT';
    END IF;
  END IF;

  RETURN NEXT v_approval;
END
$function$;

CREATE FUNCTION chat_agent_decide_approval(
  p_approval_id UUID,
  p_user_id UUID,
  p_expected_revision BIGINT,
  p_decision TEXT,
  p_occurred_at TIMESTAMPTZ
)
RETURNS SETOF chat_agent_approvals
LANGUAGE plpgsql
SECURITY DEFINER
AS $function$
DECLARE
  v_approval chat_agent_approvals%ROWTYPE;
  v_status TEXT;
  v_decision TEXT;
BEGIN
  IF p_approval_id IS NULL OR p_user_id IS NULL
     OR p_expected_revision IS NULL OR p_expected_revision < 1
     OR p_decision NOT IN (
       'allow_once', 'allow_conversation', 'deny', 'expired', 'restart_denied'
     ) OR p_occurred_at IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_APPROVAL_INPUT_INVALID';
  END IF;

  SELECT approval.* INTO v_approval
  FROM chat_agent_approvals AS approval
  JOIN conversations AS conversation
    ON conversation.id = approval.conversation_id
   AND conversation.user_id = approval.user_id
   AND conversation.deleted_at IS NULL
  WHERE approval.id = p_approval_id
    AND approval.user_id = p_user_id
  FOR UPDATE OF approval;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_APPROVAL_NOT_FOUND';
  END IF;

  -- First decision wins. Replays and conflicting later tabs receive the
  -- current terminal row instead of mutating history.
  IF v_approval.status <> 'pending' THEN
    RETURN NEXT v_approval;
    RETURN;
  END IF;
  IF v_approval.revision <> p_expected_revision THEN
    RAISE EXCEPTION USING ERRCODE = '40001', MESSAGE = 'CHAT_AGENT_APPROVAL_STALE_REVISION';
  END IF;

  IF p_occurred_at >= v_approval.expires_at OR p_decision = 'expired' THEN
    v_status := 'expired';
    v_decision := 'expired';
  ELSIF p_decision = 'restart_denied' THEN
    v_status := 'denied';
    v_decision := 'restart_denied';
  ELSIF p_decision = 'deny' THEN
    v_status := 'denied';
    v_decision := 'deny';
  ELSE
    IF p_decision = 'allow_conversation' AND NOT v_approval.allow_conversation THEN
      RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'CHAT_AGENT_APPROVAL_SCOPE_FORBIDDEN';
    END IF;
    v_status := 'allowed';
    v_decision := p_decision;
  END IF;

  UPDATE chat_agent_approvals
  SET status = v_status,
      decision = v_decision,
      revision = revision + 1,
      decided_at = p_occurred_at
  WHERE id = v_approval.id
  RETURNING * INTO v_approval;

  IF v_approval.decision = 'allow_conversation' THEN
    INSERT INTO chat_agent_conversation_tool_grants (
      conversation_id, user_id, tool_name, risk_class, created_at
    ) VALUES (
      v_approval.conversation_id, v_approval.user_id,
      v_approval.tool_name, v_approval.risk_class, p_occurred_at
    )
    ON CONFLICT (conversation_id, tool_name, risk_class) DO NOTHING;
  END IF;

  RETURN NEXT v_approval;
END
$function$;

CREATE FUNCTION chat_agent_recover_approvals(p_decided_at TIMESTAMPTZ)
RETURNS INTEGER
LANGUAGE plpgsql
SECURITY DEFINER
AS $function$
DECLARE
  v_count INTEGER;
BEGIN
  IF p_decided_at IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'CHAT_AGENT_APPROVAL_INPUT_INVALID';
  END IF;
  UPDATE chat_agent_approvals
  SET status = 'denied',
      decision = 'restart_denied',
      revision = revision + 1,
      decided_at = p_decided_at
  WHERE status = 'pending';
  GET DIAGNOSTICS v_count = ROW_COUNT;
  RETURN v_count;
END
$function$;

DO $function_paths$
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.chat_agent_create_approval(UUID,UUID,UUID,TEXT,TEXT,TEXT,BOOLEAN,TIMESTAMPTZ,TIMESTAMPTZ) SET search_path TO %I, pg_catalog, pg_temp',
    current_schema(), current_schema()
  );
  EXECUTE format(
    'ALTER FUNCTION %I.chat_agent_decide_approval(UUID,UUID,BIGINT,TEXT,TIMESTAMPTZ) SET search_path TO %I, pg_catalog, pg_temp',
    current_schema(), current_schema()
  );
  EXECUTE format(
    'ALTER FUNCTION %I.chat_agent_recover_approvals(TIMESTAMPTZ) SET search_path TO %I, pg_catalog, pg_temp',
    current_schema(), current_schema()
  );
END
$function_paths$;

REVOKE ALL ON chat_agent_approvals, chat_agent_conversation_tool_grants
  FROM PUBLIC, go_api_runtime;

REVOKE ALL ON FUNCTION chat_agent_create_approval(
  UUID,UUID,UUID,TEXT,TEXT,TEXT,BOOLEAN,TIMESTAMPTZ,TIMESTAMPTZ
), chat_agent_decide_approval(
  UUID,UUID,BIGINT,TEXT,TIMESTAMPTZ
), chat_agent_recover_approvals(
  TIMESTAMPTZ
) FROM PUBLIC, go_api_runtime;

GRANT EXECUTE ON FUNCTION chat_agent_create_approval(
  UUID,UUID,UUID,TEXT,TEXT,TEXT,BOOLEAN,TIMESTAMPTZ,TIMESTAMPTZ
), chat_agent_decide_approval(
  UUID,UUID,BIGINT,TEXT,TIMESTAMPTZ
), chat_agent_recover_approvals(
  TIMESTAMPTZ
) TO go_api_runtime;

DO $schema_acl$
BEGIN
  EXECUTE format('GRANT USAGE ON SCHEMA %I TO go_api_runtime', current_schema());
  EXECUTE format('REVOKE CREATE ON SCHEMA %I FROM go_api_runtime', current_schema());
END
$schema_acl$;
