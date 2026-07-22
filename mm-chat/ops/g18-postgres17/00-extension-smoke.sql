\set ON_ERROR_STOP on

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

DROP TABLE IF EXISTS g18_extension_smoke;
CREATE TABLE g18_extension_smoke (
  id BIGINT PRIMARY KEY,
  content TEXT NOT NULL,
  embedding VECTOR(3) NOT NULL
);
INSERT INTO g18_extension_smoke (id, content, embedding) VALUES
  (1, 'hybrid retrieval bm25 vector', '[1,0,0]'),
  (2, 'unrelated cooking recipe', '[0,1,0]');
CREATE INDEX g18_extension_smoke_bm25
  ON g18_extension_smoke USING bm25(content) WITH (text_config='simple');

DO $query_contract$
DECLARE
  lexical_winner BIGINT;
  dense_winner BIGINT;
BEGIN
  SELECT id INTO lexical_winner
  FROM g18_extension_smoke
  ORDER BY content <@> 'hybrid retrieval'
  LIMIT 1;
  IF lexical_winner <> 1 THEN
    RAISE EXCEPTION 'BM25 smoke winner = %, expected 1', lexical_winner;
  END IF;

  SELECT id INTO dense_winner
  FROM g18_extension_smoke
  ORDER BY embedding <=> '[1,0,0]'::vector, id
  LIMIT 1;
  IF dense_winner <> 1 THEN
    RAISE EXCEPTION 'vector smoke winner = %, expected 1', dense_winner;
  END IF;
END
$query_contract$;

DROP TABLE g18_extension_smoke;
SELECT 'PASS PostgreSQL 17 extension smoke' AS result;
