DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM skill_installations)
     OR EXISTS (SELECT 1 FROM skill_package_candidates)
     OR EXISTS (SELECT 1 FROM skill_package_versions) THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'SKILL_SUPPLY_CHAIN_DOWN_DATA_EXISTS';
  END IF;
END
$$;

REVOKE ALL ON TABLE skill_installations, skill_package_candidates,
  skill_package_versions FROM go_api_runtime;

DROP TABLE IF EXISTS skill_installations;
DROP FUNCTION IF EXISTS enforce_skill_installation_admission();
DROP TABLE IF EXISTS skill_package_candidates;
DROP TABLE IF EXISTS skill_package_versions;
