-- G21.5 rollback is disposable-only. Production rollback disables the exact
-- profile/target and retains migration 094 plus its activation evidence.

DO $guard$
BEGIN
  IF EXISTS(SELECT 1 FROM agent_cron_worker_targets WHERE enabled)
     OR EXISTS(SELECT 1 FROM agent_learning_worker_targets WHERE enabled) THEN
    RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_WORKER_DOWN_ACTIVE_TARGET';
  END IF;
  IF EXISTS(SELECT 1 FROM agent_cron_worker_targets target
      JOIN agent_cron_templates template ON template.id=target.template_id
      WHERE template.cursor_claim_owner IS NOT NULL)
     OR EXISTS(SELECT 1 FROM agent_cron_worker_targets target
      JOIN agent_cron_triggers trigger ON trigger.template_id=target.template_id
        AND trigger.revision=target.revision WHERE trigger.state='claimed')
     OR EXISTS(SELECT 1 FROM agent_learning_worker_targets target
      JOIN agent_learning_drafts draft ON draft.id=target.draft_id
      WHERE draft.state='checking')
     OR EXISTS(SELECT 1 FROM agent_learning_worker_targets target
      JOIN agent_learning_cleanup_queue queue ON queue.draft_id=target.draft_id
      WHERE queue.state='claimed') THEN
    RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_WORKER_DOWN_LIVE_CLAIM';
  END IF;
  IF EXISTS(SELECT 1 FROM agent_learning_runner_attempts
      WHERE state NOT IN ('completed','failed')) THEN
    RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_WORKER_DOWN_UNRESOLVED_RUNNER_ATTEMPT';
  END IF;
  IF EXISTS(SELECT 1 FROM agent_learning_runner_attempts
      WHERE cleanup_state IN ('pending','failed'))
     OR EXISTS(SELECT 1 FROM agent_learning_worker_targets target
      JOIN agent_learning_cleanup_queue queue ON queue.draft_id=target.draft_id
      WHERE queue.state IN ('pending','failed')) THEN
    RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_WORKER_DOWN_CLEANUP_PENDING';
  END IF;
  IF EXISTS(SELECT 1 FROM pg_auth_members membership
      JOIN pg_roles role ON role.oid=membership.roleid
      WHERE role.rolname IN ('agent_cron_worker','agent_learning_worker')) THEN
    RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_WORKER_DOWN_LOGIN_MEMBERSHIP';
  END IF;
  IF EXISTS(SELECT 1 FROM agent_cron_worker_targets)
     OR EXISTS(SELECT 1 FROM agent_learning_worker_targets)
     OR EXISTS(SELECT 1 FROM agent_learning_runner_attempts)
     OR EXISTS(SELECT 1 FROM agent_learning_runner_results)
     OR EXISTS(SELECT 1 FROM agent_learning_runner_requests) THEN
    RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_WORKER_DOWN_RETAINED_ACTIVATION_FACTS';
  END IF;
END
$guard$;

REVOKE EXECUTE ON FUNCTION
  agent_cron_worker_get_target(TEXT),
  agent_cron_worker_claim_due(TEXT,TEXT,TIMESTAMPTZ,INTEGER,INTEGER),
  agent_cron_worker_advance_cursor(TEXT,TEXT,BIGINT,TEXT,TEXT,BIGINT,TIMESTAMPTZ,TIMESTAMPTZ,TIMESTAMPTZ,JSONB),
  agent_cron_worker_claim_triggers(TEXT,TEXT,TIMESTAMPTZ,INTEGER,INTEGER),
  agent_cron_worker_enqueue_trigger(TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT),
  agent_cron_worker_release_trigger(TEXT,TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ,TEXT),
  agent_cron_worker_reconcile(TEXT,TIMESTAMPTZ,INTEGER),
  agent_cron_worker_prune(TEXT,TIMESTAMPTZ,INTEGER)
  FROM agent_cron_worker;

REVOKE EXECUTE ON FUNCTION
  agent_learning_worker_get_target(TEXT),agent_learning_worker_get_draft(TEXT),
  agent_learning_worker_claim_checks(TEXT,TEXT,TIMESTAMPTZ,INTEGER,INTEGER),
  agent_learning_worker_begin_runner_check(TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TIMESTAMPTZ,INTEGER),
  agent_learning_worker_issue_runner_authority(TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,INTEGER),
  agent_learning_worker_complete_runner_request(TEXT,TEXT,TEXT,TEXT,TEXT,JSONB),
  agent_learning_worker_record_runner_launch(TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT),
  agent_learning_worker_record_runner_result(TEXT,TEXT,BIGINT,TEXT,BIGINT,TEXT,JSONB),
  agent_learning_worker_mark_runner_cancel_pending(TEXT,TEXT,BIGINT,TEXT),
  agent_learning_worker_complete_runner_cleanup(TEXT,TEXT,BIGINT,BOOLEAN,TEXT),
  agent_learning_worker_runner_inventory(TEXT,INTEGER),
  agent_learning_worker_reconcile_runner(TEXT,TIMESTAMPTZ,INTEGER),
  agent_learning_worker_complete_checks(TEXT,TEXT,TEXT,BIGINT,JSONB,TEXT),
  agent_learning_worker_release_check(TEXT,TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ,TEXT),
  agent_learning_worker_claim_cleanup(TEXT,TEXT,TIMESTAMPTZ,INTEGER,INTEGER),
  agent_learning_worker_complete_cleanup(TEXT,TEXT,TEXT,BIGINT,TEXT),
  agent_learning_worker_release_cleanup(TEXT,TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ),
  agent_learning_worker_reconcile(TEXT,TIMESTAMPTZ,INTEGER),
  agent_learning_worker_prune_runner(TEXT,TIMESTAMPTZ,INTEGER),
  agent_learning_worker_prune(TEXT,TIMESTAMPTZ,INTEGER)
  FROM agent_learning_worker;

DO $schema_acl$
BEGIN
  EXECUTE format('REVOKE ALL ON SCHEMA %I FROM agent_cron_worker,agent_learning_worker',current_schema());
END
$schema_acl$;

DROP FUNCTION agent_learning_worker_prune(TEXT,TIMESTAMPTZ,INTEGER);
DROP FUNCTION agent_learning_worker_prune_runner(TEXT,TIMESTAMPTZ,INTEGER);
DROP FUNCTION agent_learning_worker_reconcile(TEXT,TIMESTAMPTZ,INTEGER);
DROP FUNCTION agent_learning_worker_release_cleanup(TEXT,TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ);
DROP FUNCTION agent_learning_worker_complete_cleanup(TEXT,TEXT,TEXT,BIGINT,TEXT);
DROP FUNCTION agent_learning_worker_claim_cleanup(TEXT,TEXT,TIMESTAMPTZ,INTEGER,INTEGER);
DROP FUNCTION agent_learning_worker_release_check(TEXT,TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ,TEXT);
DROP FUNCTION agent_learning_worker_complete_checks(TEXT,TEXT,TEXT,BIGINT,JSONB,TEXT);
DROP FUNCTION agent_learning_worker_reconcile_runner(TEXT,TIMESTAMPTZ,INTEGER);
DROP FUNCTION agent_learning_worker_runner_inventory(TEXT,INTEGER);
DROP FUNCTION agent_learning_worker_complete_runner_cleanup(TEXT,TEXT,BIGINT,BOOLEAN,TEXT);
DROP FUNCTION agent_learning_worker_mark_runner_cancel_pending(TEXT,TEXT,BIGINT,TEXT);
DROP FUNCTION agent_learning_worker_record_runner_result(TEXT,TEXT,BIGINT,TEXT,BIGINT,TEXT,JSONB);
DROP FUNCTION agent_learning_worker_record_runner_launch(TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT);
DROP FUNCTION agent_learning_worker_complete_runner_request(TEXT,TEXT,TEXT,TEXT,TEXT,JSONB);
DROP FUNCTION agent_learning_worker_issue_runner_authority(TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,INTEGER);
DROP FUNCTION agent_learning_worker_begin_runner_check(TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TIMESTAMPTZ,INTEGER);
DROP FUNCTION agent_learning_worker_claim_checks(TEXT,TEXT,TIMESTAMPTZ,INTEGER,INTEGER);
DROP FUNCTION agent_learning_worker_get_draft(TEXT);
DROP FUNCTION agent_learning_worker_get_target(TEXT);
DROP FUNCTION agent_learning_worker_disable_target(TEXT,TEXT,TEXT);
DROP FUNCTION agent_learning_worker_provision_target(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TIMESTAMPTZ,TIMESTAMPTZ,TEXT,TEXT);

DROP FUNCTION agent_cron_worker_prune(TEXT,TIMESTAMPTZ,INTEGER);
DROP FUNCTION agent_cron_worker_reconcile(TEXT,TIMESTAMPTZ,INTEGER);
DROP FUNCTION agent_cron_worker_release_trigger(TEXT,TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ,TEXT);
DROP FUNCTION agent_cron_worker_enqueue_trigger(TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT);
DROP FUNCTION agent_cron_worker_claim_triggers(TEXT,TEXT,TIMESTAMPTZ,INTEGER,INTEGER);
DROP FUNCTION agent_cron_worker_advance_cursor(TEXT,TEXT,BIGINT,TEXT,TEXT,BIGINT,TIMESTAMPTZ,TIMESTAMPTZ,TIMESTAMPTZ,JSONB);
DROP FUNCTION agent_cron_worker_claim_due(TEXT,TEXT,TIMESTAMPTZ,INTEGER,INTEGER);
DROP FUNCTION agent_cron_worker_get_target(TEXT);
DROP FUNCTION agent_cron_worker_disable_target(TEXT,TEXT,TEXT);
DROP FUNCTION agent_cron_worker_provision_target(TEXT,TEXT,UUID,BIGINT,TEXT,TEXT,TIMESTAMPTZ,TIMESTAMPTZ,TEXT,TEXT);

DROP TABLE agent_learning_runner_requests;
DROP TABLE agent_learning_runner_results;
DROP TABLE agent_learning_runner_attempts;
DROP TABLE agent_learning_worker_targets;
DROP TABLE agent_cron_worker_targets;
DROP FUNCTION agent_learning_runner_result_immutable();
DROP FUNCTION agent_learning_runner_result_valid(JSONB);
DROP FUNCTION agent_worker_target_guard();

DROP ROLE agent_learning_worker;
DROP ROLE agent_cron_worker;
