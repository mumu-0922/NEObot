-- Search-path hardening is intentionally retained on migration down. Restoring
-- "$user", public would reopen object-shadowing risk while older functions and
-- grants still exist. Reapplying this migration remains idempotent.

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

DO $hardening$
DECLARE
  target RECORD;
  schema_name TEXT := current_schema();
BEGIN
  FOR target IN
    SELECT
      namespace.nspname AS schema_name,
      function.proname AS function_name,
      pg_get_function_identity_arguments(function.oid) AS identity_arguments
    FROM pg_proc function
    JOIN pg_namespace namespace ON namespace.oid = function.pronamespace
    WHERE namespace.nspname = schema_name
      AND function.prosecdef
    ORDER BY function.oid
  LOOP
    EXECUTE format(
      'ALTER FUNCTION %I.%I(%s) SET search_path TO %I, pg_catalog, pg_temp',
      target.schema_name,
      target.function_name,
      target.identity_arguments,
      schema_name
    );
  END LOOP;
END
$hardening$;
