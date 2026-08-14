DO $guard$
BEGIN
  IF EXISTS(SELECT 1 FROM agent_cron_templates LIMIT 1)
     OR EXISTS(SELECT 1 FROM agent_cron_revisions LIMIT 1)
     OR EXISTS(SELECT 1 FROM agent_cron_approvals LIMIT 1)
     OR EXISTS(SELECT 1 FROM agent_cron_approval_revocations LIMIT 1)
     OR EXISTS(SELECT 1 FROM agent_cron_triggers LIMIT 1)
     OR EXISTS(SELECT 1 FROM agent_cron_audit_events LIMIT 1) THEN
    RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_CRON_DOWN_DATA_EXISTS';
  END IF;
END
$guard$;

REVOKE SELECT ON users,skill_installations,skill_package_candidates,skill_package_versions,agent_effect_grant_revocations,
  agent_kill_switches,agent_runs FROM agent_cron_owner;
REVOKE EXECUTE ON FUNCTION agent_orchestrator_enqueue_run(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT[]),
  agent_orchestrator_active_kill_mode(TEXT[]),agent_orchestrator_snapshot_sanitized(JSONB)
  FROM agent_cron_owner;

DROP FUNCTION agent_cron_prune(TIMESTAMPTZ,INTEGER);
DROP FUNCTION agent_cron_reconcile(TIMESTAMPTZ,INTEGER);
DROP FUNCTION agent_cron_release_trigger(TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ,TEXT);
DROP FUNCTION agent_cron_enqueue_trigger(TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT);
DROP FUNCTION agent_cron_claim_triggers(TEXT,TIMESTAMPTZ,INTEGER,INTEGER);
DROP FUNCTION agent_cron_advance_cursor(TEXT,BIGINT,TEXT,TEXT,BIGINT,TIMESTAMPTZ,TIMESTAMPTZ,TIMESTAMPTZ,JSONB);
DROP FUNCTION agent_cron_claim_due(TEXT,TIMESTAMPTZ,INTEGER,INTEGER);
DROP FUNCTION agent_cron_revoke_approval(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT);
DROP FUNCTION agent_cron_set_lifecycle(TEXT,UUID,BIGINT,TEXT,TIMESTAMPTZ,INTEGER,TEXT,TEXT,TEXT,TEXT);
DROP FUNCTION agent_cron_create_revision(TEXT,UUID,BIGINT,TEXT,JSONB,TIMESTAMPTZ,TEXT,TEXT,TEXT,TEXT);
DROP FUNCTION agent_cron_denial_reason(TEXT,BIGINT);
DROP FUNCTION agent_cron_append_audit(TEXT,TEXT,UUID,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB,TIMESTAMPTZ);
DROP TRIGGER trg_agent_cron_audit_immutable ON agent_cron_audit_events;
DROP TRIGGER trg_agent_cron_revocation_immutable ON agent_cron_approval_revocations;
DROP TRIGGER trg_agent_cron_approval_immutable ON agent_cron_approvals;
DROP TRIGGER trg_agent_cron_revision_immutable ON agent_cron_revisions;
DROP FUNCTION agent_cron_immutable();
DROP TABLE agent_cron_audit_events;
DROP TABLE agent_cron_triggers;
DROP TABLE agent_cron_approval_revocations;
DROP TABLE agent_cron_approvals;
DROP TABLE agent_cron_revisions;
DROP TABLE agent_cron_templates;
DROP FUNCTION agent_cron_audit_sanitized(JSONB);
DROP FUNCTION agent_cron_spec_valid(JSONB,UUID);
DROP FUNCTION agent_cron_document_sanitized(JSONB);

DO $schema_privileges$ BEGIN
  EXECUTE format('REVOKE USAGE ON SCHEMA %I FROM agent_cron_owner,agent_cron_control',current_schema());
END $schema_privileges$;

DO $roles$ DECLARE role_name TEXT;
BEGIN
  FOREACH role_name IN ARRAY ARRAY['agent_cron_control','agent_cron_owner'] LOOP
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
      IF EXISTS(SELECT 1 FROM pg_auth_members membership JOIN pg_roles parent ON parent.oid=membership.roleid
        JOIN pg_roles member ON member.oid=membership.member WHERE parent.rolname=role_name OR member.rolname=role_name) THEN
        RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_CRON_ROLE_MEMBERSHIP_EXISTS';END IF;
      EXECUTE format('DROP ROLE %I',role_name);
    END IF;
  END LOOP;
END
$roles$;
