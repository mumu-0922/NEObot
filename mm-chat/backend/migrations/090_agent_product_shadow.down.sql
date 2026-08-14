DO $guard$
BEGIN
  IF EXISTS(SELECT 1 FROM agent_product_run_cancellations)
     OR EXISTS(SELECT 1 FROM agent_artifacts)
     OR EXISTS(SELECT 1 FROM agent_shadow_policies)
     OR EXISTS(SELECT 1 FROM agent_shadow_user_opt_ins)
     OR EXISTS(SELECT 1 FROM agent_shadow_observations) THEN
    RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_PRODUCT_DOWN_DATA_EXISTS';
  END IF;
END
$guard$;

REVOKE EXECUTE ON FUNCTION agent_effect_decide_approval(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT),
  agent_effect_cancel(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT) FROM go_api_runtime;
REVOKE SELECT ON agent_cron_templates,agent_cron_revisions FROM go_api_runtime;
REVOKE EXECUTE ON FUNCTION agent_cron_create_revision(TEXT,UUID,BIGINT,TEXT,JSONB,TIMESTAMPTZ,TEXT,TEXT,TEXT,TEXT),
  agent_cron_set_lifecycle(TEXT,UUID,BIGINT,TEXT,TIMESTAMPTZ,INTEGER,TEXT,TEXT,TEXT,TEXT),
  agent_cron_revoke_approval(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT) FROM go_api_runtime;
REVOKE SELECT ON agent_learning_drafts,agent_learning_check_results,agent_learning_decisions FROM go_api_runtime;
REVOKE EXECUTE ON FUNCTION agent_learning_reject(TEXT,UUID,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT),
  agent_learning_promote(TEXT,UUID,BIGINT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,JSONB,TEXT) FROM go_api_runtime;

REVOKE SELECT ON users,agent_runs,agent_steps,agent_attempts,agent_run_events,
  agent_effect_intents,agent_delegation_authorities,agent_cron_templates,agent_cron_revisions,
  agent_learning_drafts,agent_learning_check_results,skill_package_candidates,skill_package_versions
  FROM agent_product_owner;
REVOKE UPDATE ON agent_runs,agent_steps FROM agent_product_owner;
REVOKE EXECUTE ON FUNCTION agent_orchestrator_active_kill_mode(TEXT[])
  FROM agent_product_owner;
REVOKE EXECUTE ON FUNCTION agent_orchestrator_append_event(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TIMESTAMPTZ,JSONB,TIMESTAMPTZ),
  agent_orchestrator_append_kill_switch(TEXT,TEXT,TEXT,TEXT,BOOLEAN,BIGINT,TEXT,TEXT,TEXT)
  FROM agent_product_owner;

REVOKE EXECUTE ON FUNCTION
  agent_product_get_artifact(UUID,TEXT,TEXT),
  agent_product_cancel_run(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT),
  agent_product_register_shadow_boot(TEXT),
  agent_product_update_shadow_policy(UUID,BIGINT,BOOLEAN,TEXT,UUID,TEXT,TEXT,INTEGER,INTEGER,INTEGER,TIMESTAMPTZ,TIMESTAMPTZ,TEXT),
  agent_product_set_shadow_opt_in(UUID,BIGINT,BIGINT,BOOLEAN,TEXT),
  agent_product_shadow_snapshot(UUID),
  agent_product_append_shadow_observation(TEXT,UUID,BIGINT,BIGINT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,INTEGER,INTEGER,INTEGER,INTEGER)
  FROM go_api_runtime;
REVOKE SELECT ON agent_product_runs,agent_product_steps,agent_product_attempts,agent_product_events,
  agent_product_approvals,agent_product_children,agent_product_artifacts,agent_product_schedules,
  agent_product_drafts,agent_product_draft_checks FROM go_api_runtime;
DO $schema_acl$ BEGIN EXECUTE format('REVOKE ALL ON SCHEMA %I FROM agent_product_owner',current_schema());END $schema_acl$;

DROP FUNCTION agent_product_append_shadow_observation(TEXT,UUID,BIGINT,BIGINT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,INTEGER,INTEGER,INTEGER,INTEGER);
DROP FUNCTION agent_product_shadow_snapshot(UUID);
DROP FUNCTION agent_product_set_shadow_opt_in(UUID,BIGINT,BIGINT,BOOLEAN,TEXT);
DROP FUNCTION agent_product_update_shadow_policy(UUID,BIGINT,BOOLEAN,TEXT,UUID,TEXT,TEXT,INTEGER,INTEGER,INTEGER,TIMESTAMPTZ,TIMESTAMPTZ,TEXT);
DROP FUNCTION agent_product_register_shadow_boot(TEXT);
DROP FUNCTION agent_product_cancel_run(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT);
DROP FUNCTION agent_product_get_artifact(UUID,TEXT,TEXT);
DROP VIEW agent_product_draft_checks;DROP VIEW agent_product_drafts;
DROP VIEW agent_product_schedules;DROP VIEW agent_product_artifacts;
DROP VIEW agent_product_children;DROP VIEW agent_product_approvals;
DROP VIEW agent_product_events;DROP VIEW agent_product_attempts;
DROP VIEW agent_product_steps;DROP VIEW agent_product_runs;
DROP TRIGGER trg_agent_shadow_observation_immutable ON agent_shadow_observations;
DROP TRIGGER trg_agent_shadow_opt_immutable ON agent_shadow_user_opt_ins;
DROP TRIGGER trg_agent_shadow_policy_immutable ON agent_shadow_policies;
DROP TRIGGER trg_agent_artifact_immutable ON agent_artifacts;
DROP TRIGGER trg_agent_product_cancellation_immutable ON agent_product_run_cancellations;
DROP TABLE agent_shadow_observations;DROP TABLE agent_shadow_boot_state;
DROP TABLE agent_shadow_user_opt_ins;DROP TABLE agent_shadow_policies;
DROP TABLE agent_artifacts;DROP TABLE agent_product_run_cancellations;
DROP FUNCTION agent_product_fact_immutable();
REVOKE agent_product_owner FROM go_api_runtime,agent_orchestrator_runtime,agent_runner_control,
  agent_effect_control,agent_delegation_control,agent_cron_control,agent_learning_control;
DROP ROLE agent_product_owner;
