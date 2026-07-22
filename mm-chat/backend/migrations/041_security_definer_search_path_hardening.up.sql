-- Earlier post-010 migrations captured the session default ("$user", public)
-- through SET search_path FROM CURRENT. Harden every existing SECURITY DEFINER
-- function in the application schema without changing ownership or grants.

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

  IF EXISTS (
    SELECT 1
    FROM pg_proc function
    JOIN pg_namespace namespace ON namespace.oid = function.pronamespace
    WHERE namespace.nspname = schema_name
      AND function.prosecdef
      AND function.proconfig IS DISTINCT FROM ARRAY[
        'search_path=' || quote_ident(schema_name) || ', pg_catalog, pg_temp'
      ]
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '42501',
      MESSAGE = 'SECURITY_DEFINER_SEARCH_PATH_HARDENING_INCOMPLETE';
  END IF;
END
$hardening$;
