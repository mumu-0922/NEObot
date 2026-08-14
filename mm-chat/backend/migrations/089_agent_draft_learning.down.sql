DO $guard$
BEGIN
  IF EXISTS(SELECT 1 FROM agent_learning_drafts) OR EXISTS(SELECT 1 FROM agent_learning_check_results)
     OR EXISTS(SELECT 1 FROM agent_learning_decisions) OR EXISTS(SELECT 1 FROM agent_learning_cleanup_queue)
     OR EXISTS(SELECT 1 FROM agent_learning_audit_events) OR EXISTS(SELECT 1 FROM skill_package_candidates WHERE source_type='learning') THEN
    RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_LEARNING_DOWN_DATA_EXISTS';END IF;
END
$guard$;

REVOKE EXECUTE ON FUNCTION agent_learning_create_draft(TEXT,UUID,TEXT,JSONB,TEXT,TIMESTAMPTZ,TEXT),
  agent_learning_claim_checks(TEXT,TIMESTAMPTZ,INTEGER,INTEGER),agent_learning_complete_checks(TEXT,TEXT,BIGINT,JSONB,TEXT),
  agent_learning_release_check(TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ,TEXT),agent_learning_reject(TEXT,UUID,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT),
  agent_learning_promote(TEXT,UUID,BIGINT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,JSONB,TEXT),
  agent_learning_claim_cleanup(TEXT,TIMESTAMPTZ,INTEGER,INTEGER),agent_learning_complete_cleanup(TEXT,TEXT,BIGINT,TEXT),
  agent_learning_release_cleanup(TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ),agent_learning_reconcile(TIMESTAMPTZ,INTEGER),
  agent_learning_prune(TIMESTAMPTZ,INTEGER) FROM agent_learning_control;
REVOKE SELECT ON agent_learning_drafts,agent_learning_check_results,agent_learning_decisions,agent_learning_cleanup_queue,agent_learning_audit_events FROM agent_learning_control;
REVOKE SELECT ON users,skill_package_versions,skill_package_candidates,agent_runs,agent_run_snapshots,agent_run_events,agent_kill_switches FROM agent_learning_owner;
REVOKE INSERT ON skill_package_versions,skill_package_candidates FROM agent_learning_owner;
REVOKE EXECUTE ON FUNCTION agent_orchestrator_active_kill_mode(TEXT[]) FROM agent_learning_owner;
DO $schema_acl$ BEGIN EXECUTE format('REVOKE ALL ON SCHEMA %I FROM agent_learning_owner,agent_learning_control',current_schema());END $schema_acl$;

DROP FUNCTION agent_learning_prune(TIMESTAMPTZ,INTEGER);DROP FUNCTION agent_learning_reconcile(TIMESTAMPTZ,INTEGER);
DROP FUNCTION agent_learning_release_cleanup(TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ);DROP FUNCTION agent_learning_complete_cleanup(TEXT,TEXT,BIGINT,TEXT);
DROP FUNCTION agent_learning_claim_cleanup(TEXT,TIMESTAMPTZ,INTEGER,INTEGER);
DROP FUNCTION agent_learning_promote(TEXT,UUID,BIGINT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,JSONB,TEXT);
DROP FUNCTION agent_learning_reject(TEXT,UUID,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT);
DROP FUNCTION agent_learning_release_check(TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ,TEXT);
DROP FUNCTION agent_learning_complete_checks(TEXT,TEXT,BIGINT,JSONB,TEXT);DROP FUNCTION agent_learning_claim_checks(TEXT,TIMESTAMPTZ,INTEGER,INTEGER);
DROP FUNCTION agent_learning_create_draft(TEXT,UUID,TEXT,JSONB,TEXT,TIMESTAMPTZ,TEXT);DROP FUNCTION agent_learning_source_denial(TEXT);
DROP FUNCTION agent_learning_append_audit(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,JSONB,TIMESTAMPTZ);
DROP TRIGGER trg_agent_learning_audit_immutable ON agent_learning_audit_events;
DROP TRIGGER trg_agent_learning_decision_immutable ON agent_learning_decisions;
DROP TRIGGER trg_agent_learning_check_immutable ON agent_learning_check_results;
DROP TABLE agent_learning_audit_events;DROP TABLE agent_learning_cleanup_queue;DROP TABLE agent_learning_decisions;
DROP TABLE agent_learning_check_results;DROP TABLE agent_learning_drafts;
DROP FUNCTION agent_learning_immutable();DROP FUNCTION agent_learning_metrics_valid(JSONB);DROP FUNCTION agent_learning_spec_valid(JSONB);
DROP FUNCTION agent_learning_tests_valid(JSONB);DROP FUNCTION agent_learning_evidence_valid(JSONB,TEXT);
DROP FUNCTION agent_learning_document_sanitized(JSONB);

ALTER TABLE skill_package_candidates DROP CONSTRAINT skill_candidate_source_type_check;
ALTER TABLE skill_package_candidates ADD CONSTRAINT skill_candidate_source_type_check
  CHECK(source_type IN ('official','lobehub','git','zip'));

REVOKE agent_learning_control FROM agent_learning_owner,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,
  agent_effect_control,agent_delegation_control,agent_cron_control;
REVOKE agent_learning_owner FROM agent_learning_control,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,
  agent_effect_control,agent_delegation_control,agent_cron_control;
DROP ROLE agent_learning_control;DROP ROLE agent_learning_owner;
