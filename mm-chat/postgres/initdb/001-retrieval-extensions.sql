\set ON_ERROR_STOP on

CREATE EXTENSION IF NOT EXISTS vector VERSION '0.8.5';
CREATE EXTENSION IF NOT EXISTS pg_textsearch VERSION '1.3.1';

DO $extension_contract$
BEGIN
  IF current_setting('server_version_num')::integer / 10000 <> 17 THEN
    RAISE EXCEPTION 'expected PostgreSQL 17, got %', version();
  END IF;
  IF current_setting('shared_preload_libraries') !~ '(^|,)pg_textsearch(,|$)' THEN
    RAISE EXCEPTION 'pg_textsearch is not preloaded';
  END IF;
  IF (SELECT extversion FROM pg_extension WHERE extname = 'pg_textsearch')
      <> '1.3.1' THEN
    RAISE EXCEPTION 'unexpected pg_textsearch version';
  END IF;
  IF (SELECT extversion FROM pg_extension WHERE extname = 'vector')
      <> '0.8.5' THEN
    RAISE EXCEPTION 'unexpected vector version';
  END IF;
END
$extension_contract$;
