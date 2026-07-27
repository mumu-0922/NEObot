-- This migration destroys the retired provider credential and establishes a
-- permanent non-reactivation control. Reversing it would violate the runtime
-- security contract and cannot reconstruct the deleted secret.
DO $irreversible_jina_runtime_retirement$
BEGIN
  RAISE EXCEPTION USING
    ERRCODE = '55000',
    MESSAGE = 'RAG_JINA_RUNTIME_RETIREMENT_IS_IRREVERSIBLE';
END
$irreversible_jina_runtime_retirement$;

