DROP TRIGGER IF EXISTS processor_governance_profiles_immutable
  ON processor_governance_profiles;
DROP FUNCTION IF EXISTS reject_processor_governance_profile_mutation();
