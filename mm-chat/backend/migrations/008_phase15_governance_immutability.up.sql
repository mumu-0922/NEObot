CREATE FUNCTION reject_processor_governance_profile_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'processor governance profiles are immutable'
    USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER processor_governance_profiles_immutable
BEFORE UPDATE OR DELETE ON processor_governance_profiles
FOR EACH ROW EXECUTE FUNCTION reject_processor_governance_profile_mutation();
