-- The repaired gateways are safe for every schema version that contains the
-- Chat Agent event log. Rollback removes only the migration ledger row and
-- intentionally keeps the known-good function bodies.
DO $chat_agent_event_log_function_repair_down$
BEGIN
  NULL;
END
$chat_agent_event_log_function_repair_down$;
