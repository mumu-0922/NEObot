-- A Memory job intentionally outlives its source messages so deletion cannot
-- erase queue/audit evidence. Activity is a link-only projection, however, and
-- cannot reference an assistant row that no longer exists. Skip only that
-- unrepresentable projection while preserving the terminal job transition.

CREATE OR REPLACE FUNCTION memory_dead_letter_activity_trigger()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF NEW.status = 'dead_letter'
    AND OLD.status IS DISTINCT FROM NEW.status
    AND NEW.assistant_message_id IS NOT NULL
  THEN
    INSERT INTO message_memory_activities (
      id, assistant_message_id, ordinal, user_id,
      subject_type, subject_id, action, status, reason_code,
      source_kind, source_id
    )
    SELECT
      gen_random_uuid(), NEW.assistant_message_id,
      memory_next_activity_ordinal(NEW.assistant_message_id), NEW.user_id,
      'job', NEW.job_id, 'failed', 'failed', NEW.error_code,
      'memory_job', NEW.job_id
    FROM messages assistant
    WHERE assistant.id = NEW.assistant_message_id
      AND assistant.user_id = NEW.user_id
    ON CONFLICT (source_kind, source_id) DO NOTHING;
  END IF;
  RETURN NEW;
END
$function$;
