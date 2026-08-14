\set ON_ERROR_STOP on

-- G20.9 is an operator-run data cutover, not a schema migration. The default
-- invocation is read-only. Apply requires all three explicit psql variables:
--
--   psql "$DATABASE_URL" \
--     --variable=cutover_apply=true \
--     --variable=expected_count=42 \
--     --variable=backup_fingerprint=sha256:<64-lowercase-hex> \
--     --file scripts/cutover-legacy-skills.sql
--
-- Rollback is a restore of the matching full database backup together with the
-- previous application images. There is intentionally no synthetic down SQL.

\if :{?cutover_apply}
\else
  \set cutover_apply false
\endif
\if :{?expected_count}
\else
  \set expected_count -1
\endif
\if :{?backup_fingerprint}
\else
  \set backup_fingerprint ''
\endif

BEGIN;
LOCK TABLE conversations IN SHARE ROW EXCLUSIVE MODE;

CREATE TEMP TABLE legacy_skill_cutover_guard (
  expected_count BIGINT NOT NULL,
  backup_fingerprint TEXT NOT NULL,
  target_count BIGINT NOT NULL,
  updated_count BIGINT NOT NULL DEFAULT 0
) ON COMMIT DROP;

INSERT INTO legacy_skill_cutover_guard (
  expected_count,
  backup_fingerprint,
  target_count
)
SELECT
  :'expected_count'::BIGINT,
  :'backup_fingerprint',
  count(*)
FROM conversations
WHERE metadata ? 'activeSkills';

SELECT
  target_count AS legacy_skill_conversations,
  CASE WHEN :cutover_apply::BOOLEAN THEN 'apply' ELSE 'dry-run' END AS mode
FROM legacy_skill_cutover_guard;

\if :cutover_apply
DO $guard$
DECLARE
  guard legacy_skill_cutover_guard%ROWTYPE;
BEGIN
  SELECT * INTO STRICT guard FROM legacy_skill_cutover_guard;
  IF guard.expected_count < 0 THEN
    RAISE EXCEPTION 'LEGACY_SKILL_EXPECTED_COUNT_REQUIRED';
  END IF;
  IF guard.expected_count <> guard.target_count THEN
    RAISE EXCEPTION 'LEGACY_SKILL_EXPECTED_COUNT_MISMATCH expected=% actual=%',
      guard.expected_count, guard.target_count;
  END IF;
  IF guard.backup_fingerprint !~ '^sha256:[0-9a-f]{64}$' THEN
    RAISE EXCEPTION 'LEGACY_SKILL_BACKUP_FINGERPRINT_REQUIRED';
  END IF;
END
$guard$;

WITH updated AS (
  UPDATE conversations
  SET metadata = metadata - 'activeSkills'
  WHERE metadata ? 'activeSkills'
  RETURNING 1
)
UPDATE legacy_skill_cutover_guard
SET updated_count = (SELECT count(*) FROM updated);

DO $verify$
DECLARE
  guard legacy_skill_cutover_guard%ROWTYPE;
  remaining BIGINT;
BEGIN
  SELECT * INTO STRICT guard FROM legacy_skill_cutover_guard;
  SELECT count(*) INTO remaining
  FROM conversations
  WHERE metadata ? 'activeSkills';

  IF guard.updated_count <> guard.expected_count THEN
    RAISE EXCEPTION 'LEGACY_SKILL_UPDATED_COUNT_MISMATCH expected=% actual=%',
      guard.expected_count, guard.updated_count;
  END IF;
  IF remaining <> 0 THEN
    RAISE EXCEPTION 'LEGACY_SKILL_SELECTION_REMAINS count=%', remaining;
  END IF;
END
$verify$;

SELECT
  updated_count AS removed_legacy_skill_selections,
  backup_fingerprint
FROM legacy_skill_cutover_guard;
COMMIT;
\else
\echo 'Legacy Skill cutover dry-run only; no rows were modified.'
ROLLBACK;
\endif
