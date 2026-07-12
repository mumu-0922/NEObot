\set ON_ERROR_STOP on

CREATE EXTENSION IF NOT EXISTS pg_search VERSION '0.24.2';
CREATE EXTENSION IF NOT EXISTS vector VERSION '0.8.2';

DO $check_versions$
BEGIN
  IF current_setting('server_version_num')::integer / 10000 <> 16 THEN
    RAISE EXCEPTION 'expected PostgreSQL 16, got %', version();
  END IF;
  IF (SELECT extversion FROM pg_extension WHERE extname = 'pg_search') <> '0.24.2' THEN
    RAISE EXCEPTION 'unexpected pg_search version';
  END IF;
  IF (SELECT extversion FROM pg_extension WHERE extname = 'vector') <> '0.8.2' THEN
    RAISE EXCEPTION 'unexpected vector version';
  END IF;
  IF current_setting('shared_buffers') <> '256MB'
     OR current_setting('work_mem') <> '4MB'
     OR current_setting('maintenance_work_mem') <> '64MB' THEN
    RAISE EXCEPTION 'unsafe or unexpected memory settings';
  END IF;
END
$check_versions$;

DROP SCHEMA IF EXISTS phase15_bakeoff CASCADE;
CREATE SCHEMA phase15_bakeoff;
SET search_path = phase15_bakeoff, public, paradedb;

CREATE FUNCTION assert_true(condition boolean, message text)
RETURNS void
LANGUAGE plpgsql
AS $function$
BEGIN
  IF condition IS DISTINCT FROM true THEN
    RAISE EXCEPTION 'assertion failed: %', message;
  END IF;
END
$function$;

CREATE TABLE documents (
  document_id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL,
  current_version_id uuid NOT NULL
);

-- Keep each tokenizer in its own table/index lane. This prevents tokenizer
-- configuration changes from silently changing an unrelated lexical lane.
CREATE TABLE lexical_jieba (
  chunk_id bigint PRIMARY KEY,
  tenant_id uuid NOT NULL,
  document_version_id uuid NOT NULL,
  content pdb.jieba NOT NULL
);

CREATE TABLE lexical_lindera (
  chunk_id bigint PRIMARY KEY,
  tenant_id uuid NOT NULL,
  document_version_id uuid NOT NULL,
  content pdb.lindera(chinese) NOT NULL
);

CREATE TABLE lexical_chinese_compatible (
  chunk_id bigint PRIMARY KEY,
  tenant_id uuid NOT NULL,
  document_version_id uuid NOT NULL,
  content pdb.chinese_compatible NOT NULL
);

CREATE TABLE exact_keywords (
  chunk_id bigint PRIMARY KEY,
  tenant_id uuid NOT NULL,
  document_version_id uuid NOT NULL,
  exact_values text[] NOT NULL,
  exact_phrases text[] NOT NULL
);

CREATE TABLE vectors (
  chunk_id bigint PRIMARY KEY,
  tenant_id uuid NOT NULL,
  document_version_id uuid NOT NULL,
  embedding vector(1024) NOT NULL,
  embedding_half halfvec(2048) NOT NULL
);

CREATE FUNCTION vector_1024(seed integer)
RETURNS vector(1024)
LANGUAGE sql
IMMUTABLE
STRICT
PARALLEL SAFE
AS $function$
  SELECT (
    '[' || string_agg(
      CASE i
        WHEN 1 THEN ((seed % 101)::double precision / 100.0)::text
        WHEN 2 THEN sin(seed::double precision / 17.0)::text
        WHEN 3 THEN cos(seed::double precision / 17.0)::text
        WHEN 4 THEN ((seed % 13)::double precision / 13.0)::text
        ELSE '0'
      END,
      ',' ORDER BY i
    ) || ']'
  )::vector(1024)
  FROM generate_series(1, 1024) AS dimensions(i)
$function$;

CREATE FUNCTION halfvec_2048(seed integer)
RETURNS halfvec(2048)
LANGUAGE sql
IMMUTABLE
STRICT
PARALLEL SAFE
AS $function$
  SELECT (
    '[' || string_agg(
      CASE i
        WHEN 1 THEN ((seed % 101)::double precision / 100.0)::text
        WHEN 2 THEN sin(seed::double precision / 17.0)::text
        WHEN 3 THEN cos(seed::double precision / 17.0)::text
        WHEN 4 THEN ((seed % 13)::double precision / 13.0)::text
        ELSE '0'
      END,
      ',' ORDER BY i
    ) || ']'
  )::halfvec(2048)
  FROM generate_series(1, 2048) AS dimensions(i)
$function$;
