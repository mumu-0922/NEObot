DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM skill_package_candidates WHERE owner_user_id IS NOT NULL
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'DIRECT_SKILL_INSTALLATIONS_DOWN_DATA_EXISTS';
  END IF;
END
$$;

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
      AND candidate.status = 'admitted'
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '23514',
      MESSAGE = 'SKILL_INSTALLATION_ADMISSION_REQUIRED';
  END IF;
  RETURN NEW;
END
$$;

CREATE TRIGGER trg_skill_installation_admission
BEFORE INSERT ON skill_installations
FOR EACH ROW EXECUTE FUNCTION enforce_skill_installation_admission();

ALTER TABLE skill_package_candidates
  DROP CONSTRAINT skill_candidate_private_owner_check;

ALTER TABLE skill_package_candidates
  DROP CONSTRAINT skill_package_candidates_source_owner_unique;

ALTER TABLE skill_package_candidates
  ADD CONSTRAINT skill_package_candidates_source_type_source_ref_key
  UNIQUE (source_type, source_ref);

ALTER TABLE skill_package_candidates
  DROP COLUMN owner_user_id;
