-- Forward-only: Transcript v2 events are immutable history and may already use the
-- widened event-type constraint. Roll back presentation/runtime with the feature flag.
DO $$
BEGIN
  RAISE NOTICE '101_chat_agent_transcript_blocks is forward-only; schema retained';
END
$$;
