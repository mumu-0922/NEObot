-- Owner-private direct Skill installation. Direct candidates are structurally
-- validated and immutable, but are neither reviewed nor published to Store.

ALTER TABLE skill_package_candidates
  ADD COLUMN owner_user_id UUID REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE skill_package_candidates
  DROP CONSTRAINT skill_package_candidates_source_type_source_ref_key;

ALTER TABLE skill_package_candidates
  ADD CONSTRAINT skill_package_candidates_source_owner_unique
  UNIQUE NULLS NOT DISTINCT (source_type, source_ref, owner_user_id);

ALTER TABLE skill_package_candidates
  ADD CONSTRAINT skill_candidate_private_owner_check CHECK (
    owner_user_id IS NULL
    OR (status = 'validated' AND reviewed_by_user_id IS NULL)
  );

DROP TRIGGER trg_skill_installation_admission ON skill_installations;

CREATE OR REPLACE FUNCTION enforce_skill_installation_admission()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM skill_package_candidates candidate
    WHERE candidate.id = NEW.admission_id
      AND candidate.package_fingerprint = NEW.package_fingerprint
      AND (
        (candidate.status = 'admitted' AND candidate.owner_user_id IS NULL)
        OR (
          candidate.status = 'validated'
          AND candidate.owner_user_id = NEW.user_id
        )
      )
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '23514',
      MESSAGE = 'SKILL_INSTALLATION_AUTHORITY_REQUIRED';
  END IF;
  RETURN NEW;
END
$$;

CREATE TRIGGER trg_skill_installation_admission
BEFORE INSERT ON skill_installations
FOR EACH ROW EXECUTE FUNCTION enforce_skill_installation_admission();
