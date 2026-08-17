-- Retire the disconnected G20/G21 Agent control plane. The two singleton
-- state tables contain one migration-created control row; every fact table
-- must be empty before any object is removed.
DO $retire_legacy_agent_control_plane$
DECLARE
  legacy_tables CONSTANT TEXT[] := ARRAY[
    'agent_artifacts', 'agent_attempts', 'agent_cron_approval_revocations',
    'agent_cron_approvals', 'agent_cron_audit_events', 'agent_cron_revisions',
    'agent_cron_templates', 'agent_cron_triggers', 'agent_cron_worker_targets',
    'agent_delegation_authorities', 'agent_delegation_lineage', 'agent_delegation_reaps',
    'agent_delegation_settlements', 'agent_effect_approvals', 'agent_effect_cancellations',
    'agent_effect_grant_revocations', 'agent_effect_intents', 'agent_effect_receipts',
    'agent_kill_switch_state', 'agent_kill_switches', 'agent_learning_audit_events',
    'agent_learning_check_results', 'agent_learning_cleanup_queue', 'agent_learning_decisions',
    'agent_learning_drafts', 'agent_learning_runner_attempts', 'agent_learning_runner_requests',
    'agent_learning_runner_results', 'agent_learning_worker_targets', 'agent_product_canary_activations',
    'agent_product_canary_promotions', 'agent_product_canary_receipts', 'agent_product_canary_requests',
    'agent_product_run_cancellations', 'agent_project_canary_resources', 'agent_project_mutation_cleanups',
    'agent_project_mutation_receipts', 'agent_run_events', 'agent_run_snapshots',
    'agent_runner_requests', 'agent_runner_sandboxes', 'agent_runs',
    'agent_secret_handles', 'agent_shadow_boot_state', 'agent_shadow_observations',
    'agent_shadow_policies', 'agent_shadow_user_opt_ins', 'agent_steps'
  ]::TEXT[];
  legacy_fact_tables CONSTANT TEXT[] := ARRAY[
    'agent_artifacts', 'agent_attempts', 'agent_cron_approval_revocations',
    'agent_cron_approvals', 'agent_cron_audit_events', 'agent_cron_revisions',
    'agent_cron_templates', 'agent_cron_triggers', 'agent_cron_worker_targets',
    'agent_delegation_authorities', 'agent_delegation_lineage', 'agent_delegation_reaps',
    'agent_delegation_settlements', 'agent_effect_approvals', 'agent_effect_cancellations',
    'agent_effect_grant_revocations', 'agent_effect_intents', 'agent_effect_receipts',
    'agent_kill_switches', 'agent_learning_audit_events', 'agent_learning_check_results',
    'agent_learning_cleanup_queue', 'agent_learning_decisions', 'agent_learning_drafts',
    'agent_learning_runner_attempts', 'agent_learning_runner_requests', 'agent_learning_runner_results',
    'agent_learning_worker_targets', 'agent_product_canary_activations', 'agent_product_canary_promotions',
    'agent_product_canary_receipts', 'agent_product_canary_requests', 'agent_product_run_cancellations',
    'agent_project_canary_resources', 'agent_project_mutation_cleanups', 'agent_project_mutation_receipts',
    'agent_run_events', 'agent_run_snapshots', 'agent_runner_requests',
    'agent_runner_sandboxes', 'agent_runs', 'agent_secret_handles',
    'agent_shadow_observations', 'agent_shadow_policies', 'agent_shadow_user_opt_ins',
    'agent_steps'
  ]::TEXT[];
  legacy_views CONSTANT TEXT[] := ARRAY[
    'agent_product_approvals', 'agent_product_artifacts', 'agent_product_attempts',
    'agent_product_children', 'agent_product_draft_checks', 'agent_product_drafts',
    'agent_product_events', 'agent_product_runs', 'agent_product_schedules',
    'agent_product_steps'
  ]::TEXT[];
  legacy_functions CONSTANT TEXT[] := ARRAY[
    'agent_artifact_attach', 'agent_artifact_authorize', 'agent_cron_advance_cursor',
    'agent_cron_append_audit', 'agent_cron_audit_sanitized', 'agent_cron_claim_due',
    'agent_cron_claim_triggers', 'agent_cron_create_revision', 'agent_cron_denial_reason',
    'agent_cron_document_sanitized', 'agent_cron_enqueue_trigger', 'agent_cron_immutable',
    'agent_cron_prune', 'agent_cron_reconcile', 'agent_cron_release_trigger',
    'agent_cron_revoke_approval', 'agent_cron_set_lifecycle', 'agent_cron_spec_valid',
    'agent_cron_worker_advance_cursor', 'agent_cron_worker_claim_due', 'agent_cron_worker_claim_triggers',
    'agent_cron_worker_disable_target', 'agent_cron_worker_enqueue_trigger', 'agent_cron_worker_get_target',
    'agent_cron_worker_provision_target', 'agent_cron_worker_prune', 'agent_cron_worker_reconcile',
    'agent_cron_worker_release_trigger', 'agent_delegation_admit_launch', 'agent_delegation_budget_valid',
    'agent_delegation_cascade', 'agent_delegation_complete_reap', 'agent_delegation_enqueue_child',
    'agent_delegation_immutable', 'agent_delegation_json_subset', 'agent_delegation_reap_inventory',
    'agent_delegation_reconcile', 'agent_delegation_register_root', 'agent_delegation_registry_subset',
    'agent_delegation_registry_valid', 'agent_delegation_settle', 'agent_effect_artifact_fence',
    'agent_effect_cancel', 'agent_effect_claim_commit', 'agent_effect_complete_commit',
    'agent_effect_decide_approval', 'agent_effect_expire', 'agent_effect_get_intent',
    'agent_effect_immutable', 'agent_effect_list_committing', 'agent_effect_prepare',
    'agent_effect_project_cleanup_fence', 'agent_effect_project_mutation_fence', 'agent_effect_revoke_grant',
    'agent_learning_append_audit', 'agent_learning_claim_checks', 'agent_learning_claim_cleanup',
    'agent_learning_complete_checks', 'agent_learning_complete_cleanup', 'agent_learning_create_draft',
    'agent_learning_document_sanitized', 'agent_learning_evidence_valid', 'agent_learning_immutable',
    'agent_learning_metrics_valid', 'agent_learning_promote', 'agent_learning_prune',
    'agent_learning_reconcile', 'agent_learning_reject', 'agent_learning_release_check',
    'agent_learning_release_cleanup', 'agent_learning_runner_result_immutable', 'agent_learning_runner_result_valid',
    'agent_learning_source_denial', 'agent_learning_spec_valid', 'agent_learning_tests_valid',
    'agent_learning_worker_begin_runner_check', 'agent_learning_worker_claim_checks', 'agent_learning_worker_claim_cleanup',
    'agent_learning_worker_complete_checks', 'agent_learning_worker_complete_cleanup', 'agent_learning_worker_complete_runner_cleanup',
    'agent_learning_worker_complete_runner_request', 'agent_learning_worker_disable_target', 'agent_learning_worker_get_draft',
    'agent_learning_worker_get_target', 'agent_learning_worker_issue_runner_authority', 'agent_learning_worker_mark_runner_cancel_pending',
    'agent_learning_worker_provision_target', 'agent_learning_worker_prune', 'agent_learning_worker_prune_runner',
    'agent_learning_worker_reconcile', 'agent_learning_worker_reconcile_runner', 'agent_learning_worker_record_runner_launch',
    'agent_learning_worker_record_runner_result', 'agent_learning_worker_release_check', 'agent_learning_worker_release_cleanup',
    'agent_learning_worker_runner_inventory', 'agent_orchestrator_acquire_step', 'agent_orchestrator_active_kill_mode',
    'agent_orchestrator_append_event', 'agent_orchestrator_append_kill_switch', 'agent_orchestrator_artifact_fence',
    'agent_orchestrator_detail_sanitized', 'agent_orchestrator_effect_fence', 'agent_orchestrator_enqueue_run',
    'agent_orchestrator_event_immutable', 'agent_orchestrator_heartbeat_attempt', 'agent_orchestrator_observe_terminal_conflict',
    'agent_orchestrator_project_mutation_fence', 'agent_orchestrator_prune_terminal_runs', 'agent_orchestrator_rebuild_projection',
    'agent_orchestrator_recovery_runs', 'agent_orchestrator_resolve_kill_switch', 'agent_orchestrator_runner_recovery_attempts',
    'agent_orchestrator_snapshot_immutable', 'agent_orchestrator_snapshot_sanitized', 'agent_orchestrator_transition',
    'agent_orchestrator_transition_allowed', 'agent_orchestrator_validate_runner_authority', 'agent_product_append_shadow_observation',
    'agent_product_canary_activation_guard', 'agent_product_canary_disable_activation', 'agent_product_canary_enqueue',
    'agent_product_canary_provision_activation', 'agent_product_canary_record_promotion', 'agent_product_canary_request_guard',
    'agent_product_canary_status', 'agent_product_canary_worker_claim_requests', 'agent_product_canary_worker_complete_request',
    'agent_product_canary_worker_get_activation', 'agent_product_canary_worker_health', 'agent_product_canary_worker_reconcile',
    'agent_product_canary_worker_release_request', 'agent_product_cancel_run', 'agent_product_fact_immutable',
    'agent_product_get_artifact', 'agent_product_register_shadow_boot', 'agent_product_set_shadow_opt_in',
    'agent_product_shadow_snapshot', 'agent_product_update_shadow_policy', 'agent_product_validate_artifact_authority',
    'agent_project_canary_provision', 'agent_project_mutation_cleanup', 'agent_project_mutation_commit',
    'agent_project_mutation_fact_immutable', 'agent_project_mutation_receipt_guard', 'agent_project_mutation_status',
    'agent_runner_complete_request', 'agent_runner_expect_sandbox', 'agent_runner_issue_authority',
    'agent_runner_prune', 'agent_runner_recovery_sandboxes', 'agent_runner_response_sanitized',
    'agent_runner_update_sandbox', 'agent_secret_handle_consume', 'agent_secret_handle_create',
    'agent_secret_handle_revoke', 'agent_secret_handle_revoke_intent', 'agent_worker_target_guard'
  ]::TEXT[];
  legacy_roles CONSTANT TEXT[] := ARRAY[
    'agent_artifact_control', 'agent_cron_control', 'agent_cron_owner',
    'agent_cron_worker', 'agent_delegation_control', 'agent_delegation_owner',
    'agent_effect_control', 'agent_effect_owner', 'agent_learning_control',
    'agent_learning_owner', 'agent_learning_worker', 'agent_orchestrator_owner',
    'agent_orchestrator_runtime', 'agent_product_canary_worker', 'agent_product_owner',
    'agent_project_mutation_control', 'agent_project_mutation_owner', 'agent_runner_control',
    'agent_runner_owner'
  ]::TEXT[];
  object_name TEXT;
  object_count BIGINT;
  present_count INTEGER;
BEGIN
  SELECT count(*)::INTEGER INTO present_count
  FROM pg_class relation
  JOIN pg_namespace namespace ON namespace.oid = relation.relnamespace
  WHERE namespace.nspname = current_schema()
    AND relation.relkind IN ('r', 'p')
    AND relation.relname = ANY(legacy_tables);

  IF present_count = 0 THEN
    IF EXISTS (
      SELECT 1 FROM pg_class relation
      JOIN pg_namespace namespace ON namespace.oid = relation.relnamespace
      WHERE namespace.nspname = current_schema()
        AND relation.relkind IN ('v', 'm')
        AND relation.relname = ANY(legacy_views)
    ) OR EXISTS (
      SELECT 1 FROM pg_proc routine
      JOIN pg_namespace namespace ON namespace.oid = routine.pronamespace
      WHERE namespace.nspname = current_schema()
        AND routine.proname = ANY(legacy_functions)
    ) OR EXISTS (
      SELECT 1 FROM pg_roles role WHERE role.rolname = ANY(legacy_roles)
    ) THEN
      RAISE EXCEPTION USING
        ERRCODE = '55000',
        MESSAGE = 'LEGACY_AGENT_CONTROL_PLANE_PARTIAL_STATE';
    END IF;
    RETURN;
  END IF;

  IF present_count <> cardinality(legacy_tables) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'LEGACY_AGENT_CONTROL_PLANE_PARTIAL_STATE';
  END IF;

  SELECT count(*)::INTEGER INTO present_count
  FROM pg_class relation
  JOIN pg_namespace namespace ON namespace.oid = relation.relnamespace
  WHERE namespace.nspname = current_schema()
    AND relation.relkind IN ('v', 'm')
    AND relation.relname = ANY(legacy_views);
  IF present_count <> cardinality(legacy_views) THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'LEGACY_AGENT_CONTROL_PLANE_VIEW_DRIFT';
  END IF;

  SELECT count(*)::INTEGER INTO present_count
  FROM pg_proc routine
  JOIN pg_namespace namespace ON namespace.oid = routine.pronamespace
  WHERE namespace.nspname = current_schema()
    AND routine.proname = ANY(legacy_functions);
  IF present_count <> cardinality(legacy_functions) THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'LEGACY_AGENT_CONTROL_PLANE_FUNCTION_DRIFT';
  END IF;

  SELECT count(*)::INTEGER INTO present_count
  FROM pg_roles role
  WHERE role.rolname = ANY(legacy_roles);
  IF present_count <> cardinality(legacy_roles) THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'LEGACY_AGENT_CONTROL_PLANE_ROLE_DRIFT';
  END IF;

  EXECUTE 'LOCK TABLE ' || (
    SELECT string_agg(format('%I.%I', current_schema(), name), ', ' ORDER BY name)
    FROM unnest(legacy_tables) AS names(name)
  ) || ' IN ACCESS EXCLUSIVE MODE';

  FOREACH object_name IN ARRAY legacy_fact_tables LOOP
    EXECUTE format('SELECT count(*) FROM %I.%I', current_schema(), object_name)
      INTO object_count;
    IF object_count <> 0 THEN
      RAISE EXCEPTION USING
        ERRCODE = '55000',
        MESSAGE = format('LEGACY_AGENT_CONTROL_PLANE_DATA_EXISTS: %s', object_name),
        DETAIL = format('%s contains %s row(s)', object_name, object_count);
    END IF;
  END LOOP;

  SELECT count(*) INTO object_count FROM agent_kill_switch_state
  WHERE singleton IS TRUE;
  IF object_count <> 1 OR (SELECT count(*) FROM agent_kill_switch_state) <> 1 THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'LEGACY_AGENT_CONTROL_PLANE_SINGLETON_DRIFT',
      DETAIL = 'agent_kill_switch_state must contain only its bootstrap singleton';
  END IF;

  SELECT count(*) INTO object_count FROM agent_shadow_boot_state
  WHERE singleton IS TRUE;
  IF object_count <> 1 OR (SELECT count(*) FROM agent_shadow_boot_state) <> 1 THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'LEGACY_AGENT_CONTROL_PLANE_SINGLETON_DRIFT',
      DETAIL = 'agent_shadow_boot_state must contain only its bootstrap singleton';
  END IF;
END
$retire_legacy_agent_control_plane$;

DROP TRIGGER IF EXISTS trg_agent_artifact_immutable ON public.agent_artifacts;
DROP TRIGGER IF EXISTS trg_agent_cron_revocation_immutable ON public.agent_cron_approval_revocations;
DROP TRIGGER IF EXISTS trg_agent_cron_approval_immutable ON public.agent_cron_approvals;
DROP TRIGGER IF EXISTS trg_agent_cron_audit_immutable ON public.agent_cron_audit_events;
DROP TRIGGER IF EXISTS trg_agent_cron_revision_immutable ON public.agent_cron_revisions;
DROP TRIGGER IF EXISTS trg_agent_cron_worker_target_guard ON public.agent_cron_worker_targets;
DROP TRIGGER IF EXISTS trg_agent_delegation_settlement_immutable ON public.agent_delegation_settlements;
DROP TRIGGER IF EXISTS trg_agent_effect_approval_immutable ON public.agent_effect_approvals;
DROP TRIGGER IF EXISTS trg_agent_effect_cancellation_immutable ON public.agent_effect_cancellations;
DROP TRIGGER IF EXISTS trg_agent_effect_grant_revocation_immutable ON public.agent_effect_grant_revocations;
DROP TRIGGER IF EXISTS trg_agent_effect_receipt_immutable ON public.agent_effect_receipts;
DROP TRIGGER IF EXISTS trg_agent_learning_audit_immutable ON public.agent_learning_audit_events;
DROP TRIGGER IF EXISTS trg_agent_learning_check_immutable ON public.agent_learning_check_results;
DROP TRIGGER IF EXISTS trg_agent_learning_decision_immutable ON public.agent_learning_decisions;
DROP TRIGGER IF EXISTS trg_agent_learning_runner_result_immutable ON public.agent_learning_runner_results;
DROP TRIGGER IF EXISTS trg_agent_learning_worker_target_guard ON public.agent_learning_worker_targets;
DROP TRIGGER IF EXISTS trg_agent_product_canary_activation_guard ON public.agent_product_canary_activations;
DROP TRIGGER IF EXISTS trg_agent_product_canary_promotion_immutable ON public.agent_product_canary_promotions;
DROP TRIGGER IF EXISTS trg_agent_product_canary_receipt_immutable ON public.agent_product_canary_receipts;
DROP TRIGGER IF EXISTS trg_agent_product_canary_request_guard ON public.agent_product_canary_requests;
DROP TRIGGER IF EXISTS trg_agent_product_cancellation_immutable ON public.agent_product_run_cancellations;
DROP TRIGGER IF EXISTS agent_project_mutation_cleanups_immutable ON public.agent_project_mutation_cleanups;
DROP TRIGGER IF EXISTS agent_project_mutation_receipts_immutable ON public.agent_project_mutation_receipts;
DROP TRIGGER IF EXISTS trg_agent_event_immutable ON public.agent_run_events;
DROP TRIGGER IF EXISTS trg_agent_snapshot_immutable ON public.agent_run_snapshots;
DROP TRIGGER IF EXISTS trg_agent_shadow_observation_immutable ON public.agent_shadow_observations;
DROP TRIGGER IF EXISTS trg_agent_shadow_policy_immutable ON public.agent_shadow_policies;
DROP TRIGGER IF EXISTS trg_agent_shadow_opt_immutable ON public.agent_shadow_user_opt_ins;

DROP VIEW IF EXISTS public.agent_product_approvals, public.agent_product_artifacts, public.agent_product_attempts,
  public.agent_product_children, public.agent_product_draft_checks, public.agent_product_drafts,
  public.agent_product_events, public.agent_product_runs, public.agent_product_schedules,
  public.agent_product_steps;

DO $drop_legacy_agent_constraints$
DECLARE
  legacy_tables CONSTANT TEXT[] := ARRAY[
    'agent_artifacts', 'agent_attempts', 'agent_cron_approval_revocations',
    'agent_cron_approvals', 'agent_cron_audit_events', 'agent_cron_revisions',
    'agent_cron_templates', 'agent_cron_triggers', 'agent_cron_worker_targets',
    'agent_delegation_authorities', 'agent_delegation_lineage', 'agent_delegation_reaps',
    'agent_delegation_settlements', 'agent_effect_approvals', 'agent_effect_cancellations',
    'agent_effect_grant_revocations', 'agent_effect_intents', 'agent_effect_receipts',
    'agent_kill_switch_state', 'agent_kill_switches', 'agent_learning_audit_events',
    'agent_learning_check_results', 'agent_learning_cleanup_queue', 'agent_learning_decisions',
    'agent_learning_drafts', 'agent_learning_runner_attempts', 'agent_learning_runner_requests',
    'agent_learning_runner_results', 'agent_learning_worker_targets', 'agent_product_canary_activations',
    'agent_product_canary_promotions', 'agent_product_canary_receipts', 'agent_product_canary_requests',
    'agent_product_run_cancellations', 'agent_project_canary_resources', 'agent_project_mutation_cleanups',
    'agent_project_mutation_receipts', 'agent_run_events', 'agent_run_snapshots',
    'agent_runner_requests', 'agent_runner_sandboxes', 'agent_runs',
    'agent_secret_handles', 'agent_shadow_boot_state', 'agent_shadow_observations',
    'agent_shadow_policies', 'agent_shadow_user_opt_ins', 'agent_steps'
  ]::TEXT[];
  table_name TEXT;
  constraint_name TEXT;
BEGIN
  FOR table_name, constraint_name IN
    SELECT relation.relname, constraint_record.conname
    FROM pg_constraint constraint_record
    JOIN pg_class relation ON relation.oid = constraint_record.conrelid
    JOIN pg_namespace namespace ON namespace.oid = relation.relnamespace
    WHERE namespace.nspname = current_schema()
      AND relation.relname = ANY(legacy_tables)
    ORDER BY CASE constraint_record.contype WHEN 'f' THEN 0 ELSE 1 END,
      relation.relname, constraint_record.conname
  LOOP
    EXECUTE format(
      'ALTER TABLE %I.%I DROP CONSTRAINT %I',
      current_schema(), table_name, constraint_name
    );
  END LOOP;
END
$drop_legacy_agent_constraints$;

DO $drop_legacy_agent_functions$
DECLARE
  legacy_functions CONSTANT TEXT[] := ARRAY[
    'agent_artifact_attach', 'agent_artifact_authorize', 'agent_cron_advance_cursor',
    'agent_cron_append_audit', 'agent_cron_audit_sanitized', 'agent_cron_claim_due',
    'agent_cron_claim_triggers', 'agent_cron_create_revision', 'agent_cron_denial_reason',
    'agent_cron_document_sanitized', 'agent_cron_enqueue_trigger', 'agent_cron_immutable',
    'agent_cron_prune', 'agent_cron_reconcile', 'agent_cron_release_trigger',
    'agent_cron_revoke_approval', 'agent_cron_set_lifecycle', 'agent_cron_spec_valid',
    'agent_cron_worker_advance_cursor', 'agent_cron_worker_claim_due', 'agent_cron_worker_claim_triggers',
    'agent_cron_worker_disable_target', 'agent_cron_worker_enqueue_trigger', 'agent_cron_worker_get_target',
    'agent_cron_worker_provision_target', 'agent_cron_worker_prune', 'agent_cron_worker_reconcile',
    'agent_cron_worker_release_trigger', 'agent_delegation_admit_launch', 'agent_delegation_budget_valid',
    'agent_delegation_cascade', 'agent_delegation_complete_reap', 'agent_delegation_enqueue_child',
    'agent_delegation_immutable', 'agent_delegation_json_subset', 'agent_delegation_reap_inventory',
    'agent_delegation_reconcile', 'agent_delegation_register_root', 'agent_delegation_registry_subset',
    'agent_delegation_registry_valid', 'agent_delegation_settle', 'agent_effect_artifact_fence',
    'agent_effect_cancel', 'agent_effect_claim_commit', 'agent_effect_complete_commit',
    'agent_effect_decide_approval', 'agent_effect_expire', 'agent_effect_get_intent',
    'agent_effect_immutable', 'agent_effect_list_committing', 'agent_effect_prepare',
    'agent_effect_project_cleanup_fence', 'agent_effect_project_mutation_fence', 'agent_effect_revoke_grant',
    'agent_learning_append_audit', 'agent_learning_claim_checks', 'agent_learning_claim_cleanup',
    'agent_learning_complete_checks', 'agent_learning_complete_cleanup', 'agent_learning_create_draft',
    'agent_learning_document_sanitized', 'agent_learning_evidence_valid', 'agent_learning_immutable',
    'agent_learning_metrics_valid', 'agent_learning_promote', 'agent_learning_prune',
    'agent_learning_reconcile', 'agent_learning_reject', 'agent_learning_release_check',
    'agent_learning_release_cleanup', 'agent_learning_runner_result_immutable', 'agent_learning_runner_result_valid',
    'agent_learning_source_denial', 'agent_learning_spec_valid', 'agent_learning_tests_valid',
    'agent_learning_worker_begin_runner_check', 'agent_learning_worker_claim_checks', 'agent_learning_worker_claim_cleanup',
    'agent_learning_worker_complete_checks', 'agent_learning_worker_complete_cleanup', 'agent_learning_worker_complete_runner_cleanup',
    'agent_learning_worker_complete_runner_request', 'agent_learning_worker_disable_target', 'agent_learning_worker_get_draft',
    'agent_learning_worker_get_target', 'agent_learning_worker_issue_runner_authority', 'agent_learning_worker_mark_runner_cancel_pending',
    'agent_learning_worker_provision_target', 'agent_learning_worker_prune', 'agent_learning_worker_prune_runner',
    'agent_learning_worker_reconcile', 'agent_learning_worker_reconcile_runner', 'agent_learning_worker_record_runner_launch',
    'agent_learning_worker_record_runner_result', 'agent_learning_worker_release_check', 'agent_learning_worker_release_cleanup',
    'agent_learning_worker_runner_inventory', 'agent_orchestrator_acquire_step', 'agent_orchestrator_active_kill_mode',
    'agent_orchestrator_append_event', 'agent_orchestrator_append_kill_switch', 'agent_orchestrator_artifact_fence',
    'agent_orchestrator_detail_sanitized', 'agent_orchestrator_effect_fence', 'agent_orchestrator_enqueue_run',
    'agent_orchestrator_event_immutable', 'agent_orchestrator_heartbeat_attempt', 'agent_orchestrator_observe_terminal_conflict',
    'agent_orchestrator_project_mutation_fence', 'agent_orchestrator_prune_terminal_runs', 'agent_orchestrator_rebuild_projection',
    'agent_orchestrator_recovery_runs', 'agent_orchestrator_resolve_kill_switch', 'agent_orchestrator_runner_recovery_attempts',
    'agent_orchestrator_snapshot_immutable', 'agent_orchestrator_snapshot_sanitized', 'agent_orchestrator_transition',
    'agent_orchestrator_transition_allowed', 'agent_orchestrator_validate_runner_authority', 'agent_product_append_shadow_observation',
    'agent_product_canary_activation_guard', 'agent_product_canary_disable_activation', 'agent_product_canary_enqueue',
    'agent_product_canary_provision_activation', 'agent_product_canary_record_promotion', 'agent_product_canary_request_guard',
    'agent_product_canary_status', 'agent_product_canary_worker_claim_requests', 'agent_product_canary_worker_complete_request',
    'agent_product_canary_worker_get_activation', 'agent_product_canary_worker_health', 'agent_product_canary_worker_reconcile',
    'agent_product_canary_worker_release_request', 'agent_product_cancel_run', 'agent_product_fact_immutable',
    'agent_product_get_artifact', 'agent_product_register_shadow_boot', 'agent_product_set_shadow_opt_in',
    'agent_product_shadow_snapshot', 'agent_product_update_shadow_policy', 'agent_product_validate_artifact_authority',
    'agent_project_canary_provision', 'agent_project_mutation_cleanup', 'agent_project_mutation_commit',
    'agent_project_mutation_fact_immutable', 'agent_project_mutation_receipt_guard', 'agent_project_mutation_status',
    'agent_runner_complete_request', 'agent_runner_expect_sandbox', 'agent_runner_issue_authority',
    'agent_runner_prune', 'agent_runner_recovery_sandboxes', 'agent_runner_response_sanitized',
    'agent_runner_update_sandbox', 'agent_secret_handle_consume', 'agent_secret_handle_create',
    'agent_secret_handle_revoke', 'agent_secret_handle_revoke_intent', 'agent_worker_target_guard'
  ]::TEXT[];
  routine REGPROCEDURE;
BEGIN
  FOR routine IN
    SELECT function_oid::REGPROCEDURE
    FROM (
      SELECT routine.oid AS function_oid
      FROM pg_proc routine
      JOIN pg_namespace namespace ON namespace.oid = routine.pronamespace
      WHERE namespace.nspname = current_schema()
        AND routine.proname = ANY(legacy_functions)
      ORDER BY routine.proname, pg_get_function_identity_arguments(routine.oid)
    ) exact_functions
  LOOP
    EXECUTE 'DROP FUNCTION ' || routine::TEXT;
  END LOOP;
END
$drop_legacy_agent_functions$;

DROP TABLE IF EXISTS public.agent_artifacts, public.agent_attempts, public.agent_cron_approval_revocations,
  public.agent_cron_approvals, public.agent_cron_audit_events, public.agent_cron_revisions,
  public.agent_cron_templates, public.agent_cron_triggers, public.agent_cron_worker_targets,
  public.agent_delegation_authorities, public.agent_delegation_lineage, public.agent_delegation_reaps,
  public.agent_delegation_settlements, public.agent_effect_approvals, public.agent_effect_cancellations,
  public.agent_effect_grant_revocations, public.agent_effect_intents, public.agent_effect_receipts,
  public.agent_kill_switch_state, public.agent_kill_switches, public.agent_learning_audit_events,
  public.agent_learning_check_results, public.agent_learning_cleanup_queue, public.agent_learning_decisions,
  public.agent_learning_drafts, public.agent_learning_runner_attempts, public.agent_learning_runner_requests,
  public.agent_learning_runner_results, public.agent_learning_worker_targets, public.agent_product_canary_activations,
  public.agent_product_canary_promotions, public.agent_product_canary_receipts, public.agent_product_canary_requests,
  public.agent_product_run_cancellations, public.agent_project_canary_resources, public.agent_project_mutation_cleanups,
  public.agent_project_mutation_receipts, public.agent_run_events, public.agent_run_snapshots,
  public.agent_runner_requests, public.agent_runner_sandboxes, public.agent_runs,
  public.agent_secret_handles, public.agent_shadow_boot_state, public.agent_shadow_observations,
  public.agent_shadow_policies, public.agent_shadow_user_opt_ins, public.agent_steps;

DO $drop_legacy_agent_roles$
DECLARE
  legacy_roles CONSTANT TEXT[] := ARRAY[
    'agent_artifact_control', 'agent_cron_control', 'agent_cron_owner',
    'agent_cron_worker', 'agent_delegation_control', 'agent_delegation_owner',
    'agent_effect_control', 'agent_effect_owner', 'agent_learning_control',
    'agent_learning_owner', 'agent_learning_worker', 'agent_orchestrator_owner',
    'agent_orchestrator_runtime', 'agent_product_canary_worker', 'agent_product_owner',
    'agent_project_mutation_control', 'agent_project_mutation_owner', 'agent_runner_control',
    'agent_runner_owner'
  ]::TEXT[];
  role_name TEXT;
BEGIN
  FOREACH role_name IN ARRAY legacy_roles LOOP
    IF EXISTS (SELECT 1 FROM pg_roles role WHERE role.rolname = role_name) THEN
      EXECUTE format('REASSIGN OWNED BY %I TO CURRENT_USER', role_name);
      EXECUTE format('DROP OWNED BY %I', role_name);
      EXECUTE format('DROP ROLE %I', role_name);
    END IF;
  END LOOP;
END
$drop_legacy_agent_roles$;
