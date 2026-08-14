DO $guard$
BEGIN
  IF EXISTS(SELECT 1 FROM agent_project_canary_resources)
     OR EXISTS(SELECT 1 FROM agent_project_mutation_receipts)
     OR EXISTS(SELECT 1 FROM agent_project_mutation_cleanups) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_PROJECT_MUTATION_DOWN_REQUIRES_EMPTY';
  END IF;
END
$guard$;
REVOKE EXECUTE ON FUNCTION
  agent_project_mutation_commit(TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,BYTEA,TEXT,TEXT),
  agent_project_mutation_status(TEXT,UUID,TEXT,TEXT,TEXT,TEXT),
  agent_project_mutation_cleanup(TEXT,UUID,TEXT,TEXT,TEXT)
  FROM agent_project_mutation_control;
REVOKE EXECUTE ON FUNCTION
  agent_effect_project_mutation_fence(TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT),
  agent_effect_project_cleanup_fence(TEXT,UUID,TEXT),
  agent_orchestrator_project_mutation_fence(UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,BIGINT)
  FROM agent_project_mutation_owner;
DO $schema_acl$ BEGIN
  EXECUTE format('REVOKE ALL ON SCHEMA %I FROM agent_project_mutation_owner,agent_project_mutation_control',current_schema());
END
$schema_acl$;
DROP FUNCTION agent_project_mutation_cleanup(TEXT,UUID,TEXT,TEXT,TEXT);
DROP FUNCTION agent_project_mutation_status(TEXT,UUID,TEXT,TEXT,TEXT,TEXT);
DROP FUNCTION agent_project_mutation_commit(TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,BYTEA,TEXT,TEXT);
DROP FUNCTION agent_project_canary_provision(TEXT,UUID,TEXT,TEXT,BYTEA,BIGINT);
DROP FUNCTION agent_effect_project_cleanup_fence(TEXT,UUID,TEXT);
DROP FUNCTION agent_orchestrator_project_mutation_fence(UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,BIGINT);
DROP FUNCTION agent_effect_project_mutation_fence(TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT);
DROP TABLE agent_project_mutation_cleanups;
DROP TABLE agent_project_mutation_receipts;
DROP TABLE agent_project_canary_resources;
DROP FUNCTION agent_project_mutation_receipt_guard();
DROP FUNCTION agent_project_mutation_fact_immutable();
DROP ROLE agent_project_mutation_control;
DROP ROLE agent_project_mutation_owner;
