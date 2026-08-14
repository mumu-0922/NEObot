package agentcron

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/agentbroker"
)

func TestPostgresAgentCronRevisionClaimRestartOverlapAndAuthority(t *testing.T) {
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MM_CHAT_TEST_DATABASE_URL is unset")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	database := stdlib.OpenDB(*config)
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Second)
	userID := "88888888-8888-4888-8888-888888888888"
	retain := os.Getenv("MM_CHAT_AGENT_CRON_RETAIN_FIXTURE") == "1"
	indexOffset := 0
	if retain {
		indexOffset = 1000
	}
	cronIndex := func(index int) int { return indexOffset + index }
	mustCronExec(t, ctx, database, `INSERT INTO users(id,email,display_name) VALUES($1,$2,'Agent Cron fixture') ON CONFLICT(id) DO UPDATE SET deleted_at=NULL`,
		userID, "agent-cron@example.test")
	mustCronExec(t, ctx, database, `
INSERT INTO skill_package_versions(package_fingerprint,runtime_bundle_fingerprint,sbom_fingerprint,name,version,description,
  has_runtime,file_count,package_bytes,expanded_bytes,package_object_key,sbom_object_key)
VALUES($1,$2,$3,'cron-fixture','1.0.0','Cron fixture',true,1,1,1,$4,$5) ON CONFLICT(package_fingerprint) DO NOTHING
`, testPackage, testRuntime,
		"sha256:3333333333333333333333333333333333333333333333333333333333333333",
		"skill-packages/sha256/"+testPackage[7:]+".zip",
		"skill-sboms/sha256/"+"3333333333333333333333333333333333333333333333333333333333333333"+".cdx.json")
	mustCronExec(t, ctx, database, `
INSERT INTO skill_package_candidates(id,source_type,source_ref,source_artifact_sha256,source_object_key,package_fingerprint,
  status,admission_eligible,reviewed_by_user_id,review_reason)
VALUES($1,'official','cron-fixture','sha256:4444444444444444444444444444444444444444444444444444444444444444',
  'skill-quarantine/sha256/4444444444444444444444444444444444444444444444444444444444444444.zip',$2,
  'admitted',true,$3,'CRON_FIXTURE') ON CONFLICT(id) DO UPDATE SET status='admitted',admission_eligible=true
`, testAdmissionID, testPackage, userID)
	mustCronExec(t, ctx, database, `
INSERT INTO skill_installations(id,user_id,admission_id,package_fingerprint,skill_name)
VALUES($1,$2,$3,$4,'cron-fixture') ON CONFLICT(id) DO NOTHING
`, testInstallationID, userID, testAdmissionID, testPackage)

	repository := NewPostgresRepository(database)
	service := NewService(repository)
	service.now = func() time.Time { return now }

	createTemplateWith := func(index int, mutate func(*TemplateSpec)) Template {
		t.Helper()
		index = cronIndex(index)
		spec := postgresCronSpec(now, userID, index)
		if mutate != nil {
			mutate(&spec)
		}
		result, created, err := service.CreateRevision(ctx, CreateInput{
			TemplateID: fmt.Sprintf("cron_%016x", index), Spec: spec,
			Approval: Approval{ActorType: "operator", ActorID: "cron-integration", ReasonCode: "CRON_TEST_APPROVED"},
		})
		if err != nil || !created || result.CurrentRevision != 1 || result.State != TemplateActive {
			t.Fatalf("create template %d = %#v created=%v err=%v", index, result, created, err)
		}
		replayed, replayCreated, err := service.CreateRevision(ctx, CreateInput{
			TemplateID: result.ID, Spec: spec,
			Approval: Approval{ActorType: "operator", ActorID: "cron-integration", ReasonCode: "CRON_TEST_APPROVED"},
		})
		if err != nil || replayCreated || replayed.ID != result.ID {
			t.Fatalf("revision replay %d = %#v created=%v err=%v", index, replayed, replayCreated, err)
		}
		return result
	}
	createTemplate := func(index int) Template { return createTemplateWith(index, nil) }

	template := createTemplate(1)
	setCronDue(t, ctx, database, template.ID)
	request := ClaimRequest{Owner: "cron-worker-a", Now: time.Now().UTC(), LeaseDuration: 30 * time.Second, Limit: 100}
	cycle, err := service.RunCycle(ctx, request)
	if err != nil || cycle.Enqueued != 1 || cycle.Materialized == 0 {
		t.Fatalf("first cycle=%#v err=%v", cycle, err)
	}
	var triggerCount, runCount int
	mustCronQuery(t, ctx, database, `SELECT count(*) FROM agent_cron_triggers WHERE template_id=$1 AND state='enqueued'`, template.ID).Scan(&triggerCount)
	mustCronQuery(t, ctx, database, `SELECT count(*) FROM agent_runs WHERE idempotency_key LIKE $1`, "cron:"+template.ID+":%").Scan(&runCount)
	if triggerCount != 1 || runCount != 1 {
		t.Fatalf("first cycle triggers=%d runs=%d", triggerCount, runCount)
	}
	var enqueuedTriggerID, enqueuedRunID string
	mustCronQuery(t, ctx, database, `
SELECT id,run_id FROM agent_cron_triggers WHERE template_id=$1 AND state='enqueued'
`, template.ID).Scan(&enqueuedTriggerID, &enqueuedRunID)
	var replayState, replayReason, replayRunID string
	var replayCreated bool
	mustCronQuery(t, ctx, database, `
SELECT state,reason_code,run_id,created FROM agent_cron_enqueue_trigger(
  $1,'ack-lost-worker',0,'run_0000000000000000','snapshot_0000000000000000',
  $2,$2,'{}'::jsonb,'[]'::jsonb,ARRAY[]::text[],'cron_event_0000000000000000'
)
`, enqueuedTriggerID, testPackage).Scan(&replayState, &replayReason, &replayRunID, &replayCreated)
	if replayState != TriggerEnqueued || replayReason != "RUN_ENQUEUED" ||
		replayRunID != enqueuedRunID || replayCreated {
		t.Fatalf("enqueue acknowledgement replay state=%s reason=%s run=%s created=%v",
			replayState, replayReason, replayRunID, replayCreated)
	}
	mustCronQuery(t, ctx, database, `SELECT count(*) FROM agent_runs WHERE idempotency_key LIKE $1`, "cron:"+template.ID+":%").Scan(&runCount)
	if runCount != 1 {
		t.Fatalf("enqueue acknowledgement replay created %d Runs", runCount)
	}

	// The first Run remains queued. buffer_one must retain exactly one pending
	// occurrence and skip later occurrences instead of creating replacement Runs.
	setCronDue(t, ctx, database, template.ID)
	request.Now = time.Now().UTC()
	if _, err := service.RunCycle(ctx, request); err != nil {
		t.Fatal(err)
	}
	setCronDue(t, ctx, database, template.ID)
	request.Now = time.Now().UTC()
	if _, err := service.RunCycle(ctx, request); err != nil {
		t.Fatal(err)
	}
	var pending, bufferFull int
	mustCronQuery(t, ctx, database, `SELECT count(*) FROM agent_cron_triggers WHERE template_id=$1 AND state='pending'`, template.ID).Scan(&pending)
	mustCronQuery(t, ctx, database, `SELECT count(*) FROM agent_cron_triggers WHERE template_id=$1 AND reason_code='OVERLAP_BUFFER_FULL'`, template.ID).Scan(&bufferFull)
	if pending != 1 || bufferFull < 1 {
		t.Fatalf("buffer_one pending=%d buffer_full=%d", pending, bufferFull)
	}
	if err := removeCronTemplate(ctx, database, template.ID); err != nil {
		t.Fatal(err)
	}

	for _, overlapCase := range []struct {
		index      int
		policy     string
		wantRuns   int
		wantReason string
	}{
		{index: 30, policy: OverlapSkip, wantRuns: 1, wantReason: "OVERLAP_SKIPPED"},
		{index: 31, policy: OverlapAllow, wantRuns: 2, wantReason: "RUN_ENQUEUED"},
	} {
		overlapTemplate := createTemplateWith(overlapCase.index, func(spec *TemplateSpec) {
			spec.Policies.Overlap = overlapCase.policy
		})
		for cycleIndex := 0; cycleIndex < 2; cycleIndex++ {
			setCronDue(t, ctx, database, overlapTemplate.ID)
			cycleRequest := ClaimRequest{
				Owner: fmt.Sprintf("cron-worker-overlap-%d-%d", overlapCase.index, cycleIndex),
				Now:   time.Now().UTC(), LeaseDuration: 30 * time.Second, Limit: 100,
			}
			if _, cycleErr := service.RunCycle(ctx, cycleRequest); cycleErr != nil {
				t.Fatal(cycleErr)
			}
		}
		mustCronQuery(t, ctx, database, `SELECT count(*) FROM agent_runs WHERE idempotency_key LIKE $1`, "cron:"+overlapTemplate.ID+":%").Scan(&runCount)
		mustCronQuery(t, ctx, database, `SELECT count(*) FROM agent_cron_triggers WHERE template_id=$1 AND reason_code=$2`, overlapTemplate.ID, overlapCase.wantReason).Scan(&triggerCount)
		if runCount != overlapCase.wantRuns || triggerCount < 1 {
			t.Fatalf("overlap %s runs=%d reason_count=%d", overlapCase.policy, runCount, triggerCount)
		}
		if err := removeCronTemplate(ctx, database, overlapTemplate.ID); err != nil {
			t.Fatal(err)
		}
	}

	// Two schedulers race on one due cursor. Exactly one gets the durable claim.
	concurrentTemplate := createTemplate(2)
	setCronDue(t, ctx, database, concurrentTemplate.ID)
	var wait sync.WaitGroup
	counts := make(chan int, 2)
	errorsFound := make(chan error, 2)
	for _, owner := range []string{"cron-worker-race-a", "cron-worker-race-b"} {
		wait.Add(1)
		go func(owner string) {
			defer wait.Done()
			claims, claimErr := repository.ClaimDue(ctx, ClaimRequest{Owner: owner, Now: time.Now().UTC(), LeaseDuration: 30 * time.Second, Limit: 1})
			if claimErr != nil {
				errorsFound <- claimErr
				return
			}
			counts <- len(claims)
		}(owner)
	}
	wait.Wait()
	close(counts)
	close(errorsFound)
	for claimErr := range errorsFound {
		t.Fatal(claimErr)
	}
	claimed := 0
	for count := range counts {
		claimed += count
	}
	if claimed != 1 {
		t.Fatalf("concurrent due claims=%d", claimed)
	}

	// Expired claim recovery increments generation. The old claimant cannot
	// advance the same cursor after restart.
	mustCronExec(t, ctx, database, `UPDATE agent_cron_templates SET cursor_claim_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, concurrentTemplate.ID)
	newClaims, err := repository.ClaimDue(ctx, ClaimRequest{Owner: "cron-worker-restart", Now: time.Now().UTC(), LeaseDuration: 30 * time.Second, Limit: 1})
	if err != nil || len(newClaims) != 1 {
		t.Fatalf("restart claims=%#v err=%v", newClaims, err)
	}
	oldClaim := newClaims[0]
	oldClaim.ClaimOwner = "cron-worker-race-a"
	oldClaim.ClaimGeneration--
	decisions, next, err := planOccurrences(newClaims[0], time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Advance(ctx, AdvanceInput{Claim: oldClaim, ObservedAt: time.Now().UTC(), NextTriggerAt: next, Decisions: decisions}); !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("stale cursor claim error=%v", err)
	}
	if err := removeCronTemplate(ctx, database, concurrentTemplate.ID); err != nil {
		t.Fatal(err)
	}

	// Trigger claims are fenced by both generation and expiry. Reconciliation
	// makes the same occurrence retryable, while bounded releases eventually
	// terminalize it instead of creating a replacement occurrence or Run.
	retryTemplate := createTemplate(4)
	oldTrigger := materializeAndClaimCron(t, ctx, database, repository, service, retryTemplate.ID, "cron-worker-trigger-old")
	mustCronExec(t, ctx, database, `UPDATE agent_cron_triggers SET claim_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, oldTrigger.ID)
	if err := repository.ReleaseTrigger(ctx, ReleaseInput{
		TriggerID: oldTrigger.ID, ClaimOwner: oldTrigger.ClaimOwner, ClaimGeneration: oldTrigger.ClaimGeneration,
		ErrorCode: "SCHEDULER_UNAVAILABLE", RetryAt: time.Now().Add(time.Minute), AuditEventID: service.newID("cron_event"),
	}); !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("expired trigger release error=%v", err)
	}
	reconciled, err := service.Reconcile(ctx, time.Now().UTC(), 100)
	if err != nil || reconciled.TriggerClaimsReclaimed != 1 {
		t.Fatalf("trigger reconcile=%#v err=%v", reconciled, err)
	}
	oldEnvelope, err := service.prepareEnqueue(oldTrigger)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.EnqueueTrigger(ctx, oldEnvelope); !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("stale trigger enqueue error=%v", err)
	}
	for attempt := 1; attempt <= retryTemplate.Spec.Policies.MaxAttempts; attempt++ {
		claimedTriggers, claimErr := repository.ClaimTriggers(ctx, ClaimRequest{
			Owner: fmt.Sprintf("cron-worker-retry-%d", attempt), Now: time.Now().UTC(),
			LeaseDuration: 30 * time.Second, Limit: 100,
		})
		if claimErr != nil || len(claimedTriggers) != 1 || claimedTriggers[0].ID != oldTrigger.ID {
			t.Fatalf("retry claim %d=%#v err=%v", attempt, claimedTriggers, claimErr)
		}
		claimedTrigger := claimedTriggers[0]
		if releaseErr := repository.ReleaseTrigger(ctx, ReleaseInput{
			TriggerID: claimedTrigger.ID, ClaimOwner: claimedTrigger.ClaimOwner, ClaimGeneration: claimedTrigger.ClaimGeneration,
			ErrorCode: "SCHEDULER_UNAVAILABLE", RetryAt: time.Now().Add(time.Minute), AuditEventID: service.newID("cron_event"),
		}); releaseErr != nil {
			t.Fatal(releaseErr)
		}
		if attempt < retryTemplate.Spec.Policies.MaxAttempts {
			mustCronExec(t, ctx, database, `UPDATE agent_cron_triggers SET next_attempt_at=clock_timestamp()-interval '1 second' WHERE id=$1`, oldTrigger.ID)
		}
	}
	var retryState string
	var retryCount int
	mustCronQuery(t, ctx, database, `SELECT state,retry_count FROM agent_cron_triggers WHERE id=$1`, oldTrigger.ID).Scan(&retryState, &retryCount)
	if retryState != TriggerFailed || retryCount != retryTemplate.Spec.Policies.MaxAttempts {
		t.Fatalf("bounded retry state=%s count=%d", retryState, retryCount)
	}
	if err := removeCronTemplate(ctx, database, retryTemplate.ID); err != nil {
		t.Fatal(err)
	}

	// A Grant revoked after occurrence claim must become a sanitized skipped fact
	// before any normal Run is inserted.
	revokedTemplate := createTemplate(3)
	trigger := materializeAndClaimCron(t, ctx, database, repository, service, revokedTemplate.ID, "cron-worker-revoked")
	mustCronExec(t, ctx, database, `
INSERT INTO agent_effect_grant_revocations(grant_id,grant_fingerprint,actor_type,actor_id,reason_code,revoked_at)
VALUES($1,$2,'operator','cron-integration','CRON_GRANT_REVOKED',clock_timestamp())
`, trigger.Spec.Grant.GrantID, trigger.Spec.Grant.GrantFingerprint)
	envelope, err := service.prepareEnqueue(trigger)
	if err != nil {
		t.Fatal(err)
	}
	revokedResult, err := repository.EnqueueTrigger(ctx, envelope)
	if err != nil || revokedResult.State != TriggerSkipped || revokedResult.ReasonCode != "GRANT_REVOKED" {
		t.Fatalf("revoked trigger=%#v err=%v", revokedResult, err)
	}
	mustCronQuery(t, ctx, database, `SELECT count(*) FROM agent_runs WHERE idempotency_key LIKE $1`, "cron:"+revokedTemplate.ID+":%").Scan(&runCount)
	if runCount != 0 {
		t.Fatalf("revoked trigger created %d Runs", runCount)
	}
	if err := removeCronTemplate(ctx, database, revokedTemplate.ID); err != nil {
		t.Fatal(err)
	}

	assertDenied := func(trigger Trigger, want string) {
		t.Helper()
		envelope, prepareErr := service.prepareEnqueue(trigger)
		if prepareErr != nil {
			t.Fatal(prepareErr)
		}
		denied, enqueueErr := repository.EnqueueTrigger(ctx, envelope)
		if enqueueErr != nil || denied.State != TriggerSkipped || denied.ReasonCode != want {
			t.Fatalf("denial want=%s result=%#v err=%v", want, denied, enqueueErr)
		}
	}

	ownerTemplate := createTemplate(5)
	ownerTrigger := materializeAndClaimCron(t, ctx, database, repository, service, ownerTemplate.ID, "cron-worker-owner")
	mustCronExec(t, ctx, database, `UPDATE users SET deleted_at=clock_timestamp(),updated_at=clock_timestamp() WHERE id=$1`, userID)
	assertDenied(ownerTrigger, "OWNER_REVOKED")
	mustCronExec(t, ctx, database, `UPDATE users SET deleted_at=NULL,updated_at=clock_timestamp() WHERE id=$1`, userID)
	if err := removeCronTemplate(ctx, database, ownerTemplate.ID); err != nil {
		t.Fatal(err)
	}

	skillTemplate := createTemplate(6)
	skillTrigger := materializeAndClaimCron(t, ctx, database, repository, service, skillTemplate.ID, "cron-worker-skill")
	mustCronExec(t, ctx, database, `UPDATE skill_package_candidates SET status='rejected',updated_at=clock_timestamp() WHERE id=$1`, testAdmissionID)
	assertDenied(skillTrigger, "SKILL_REVOKED")
	mustCronExec(t, ctx, database, `UPDATE skill_package_candidates SET status='admitted',updated_at=clock_timestamp() WHERE id=$1`, testAdmissionID)
	if err := removeCronTemplate(ctx, database, skillTemplate.ID); err != nil {
		t.Fatal(err)
	}

	uninstalledTemplate := createTemplate(16)
	uninstalledTrigger := materializeAndClaimCron(t, ctx, database, repository, service, uninstalledTemplate.ID, "cron-worker-uninstalled")
	mustCronExec(t, ctx, database, `DELETE FROM skill_installations WHERE id=$1`, testInstallationID)
	assertDenied(uninstalledTrigger, "SKILL_REVOKED")
	mustCronExec(t, ctx, database, `
INSERT INTO skill_installations(id,user_id,admission_id,package_fingerprint,skill_name)
VALUES($1,$2,$3,$4,'cron-fixture')
`, testInstallationID, userID, testAdmissionID, testPackage)
	if err := removeCronTemplate(ctx, database, uninstalledTemplate.ID); err != nil {
		t.Fatal(err)
	}

	runtimeTemplate := createTemplate(17)
	runtimeTrigger := materializeAndClaimCron(t, ctx, database, repository, service, runtimeTemplate.ID, "cron-worker-runtime")
	mustCronExec(t, ctx, database, `UPDATE skill_package_versions SET runtime_bundle_fingerprint=$2 WHERE package_fingerprint=$1`,
		testPackage, "sha256:9999999999999999999999999999999999999999999999999999999999999999")
	assertDenied(runtimeTrigger, "SKILL_REVOKED")
	mustCronExec(t, ctx, database, `UPDATE skill_package_versions SET runtime_bundle_fingerprint=$2 WHERE package_fingerprint=$1`, testPackage, testRuntime)
	if err := removeCronTemplate(ctx, database, runtimeTemplate.ID); err != nil {
		t.Fatal(err)
	}

	approvalTemplate := createTemplate(7)
	approvalTrigger := materializeAndClaimCron(t, ctx, database, repository, service, approvalTemplate.ID, "cron-worker-approval")
	if created, revokeErr := service.RevokeApproval(ctx, RevokeApprovalInput{
		UserID: userID, ApprovalID: approvalTrigger.Spec.Automation.ApprovalID,
		ActorType: "operator", ActorID: "cron-integration", ReasonCode: "CRON_APPROVAL_REVOKED",
	}); revokeErr != nil || !created {
		t.Fatalf("approval revoke created=%v err=%v", created, revokeErr)
	}
	assertDenied(approvalTrigger, "APPROVAL_REVOKED")
	if err := removeCronTemplate(ctx, database, approvalTemplate.ID); err != nil {
		t.Fatal(err)
	}

	for killIndex, killCase := range []struct{ scopeType, scopeValue string }{
		{scopeType: "global", scopeValue: "*"},
		{scopeType: "scheduler", scopeValue: "*"},
		{scopeType: "user", scopeValue: userID},
		{scopeType: "project", scopeValue: "project_01234567"},
		{scopeType: "skill", scopeValue: testPackage},
	} {
		killTemplate := createTemplate(40 + killIndex)
		killTrigger := materializeAndClaimCron(t, ctx, database, repository, service, killTemplate.ID, fmt.Sprintf("cron-worker-kill-%d", killIndex))
		killSwitchID := fmt.Sprintf("switch_%016x", cronIndex(180+killIndex))
		appendCronKillSwitch(t, ctx, database, killSwitchID, killCase.scopeType, killCase.scopeValue, true, 0)
		assertDenied(killTrigger, "KILL_SWITCH_ACTIVE")
		appendCronKillSwitch(t, ctx, database, killSwitchID, killCase.scopeType, killCase.scopeValue, false, 1)
		if err := removeCronTemplate(ctx, database, killTemplate.ID); err != nil {
			t.Fatal(err)
		}
	}

	secretIndex := cronIndex(9)
	secretRef := fmt.Sprintf("secret_ref_%016x", secretIndex)
	secretSpec := postgresCronSpec(now, userID, secretIndex)
	secretSpec.Secrets = []agentbroker.SecretGrant{{
		Slot: "api", BrokerRef: secretRef, Actions: []string{"read"}, TTLSeconds: 60,
	}}
	secretTemplate, created, err := service.CreateRevision(ctx, CreateInput{
		TemplateID: fmt.Sprintf("cron_%016x", secretIndex), Spec: secretSpec,
		Approval: Approval{ActorType: "operator", ActorID: "cron-integration", ReasonCode: "CRON_TEST_APPROVED"},
	})
	if err != nil || !created {
		t.Fatalf("secret template=%#v created=%v err=%v", secretTemplate, created, err)
	}
	secretTrigger := materializeAndClaimCron(t, ctx, database, repository, service, secretTemplate.ID, "cron-worker-secret")
	secretSwitchID := fmt.Sprintf("switch_%016x", cronIndex(99))
	appendCronKillSwitch(t, ctx, database, secretSwitchID, "secret", secretRef, true, 0)
	assertDenied(secretTrigger, "SECRET_REVOKED")
	appendCronKillSwitch(t, ctx, database, secretSwitchID, "secret", secretRef, false, 1)
	if err := removeCronTemplate(ctx, database, secretTemplate.ID); err != nil {
		t.Fatal(err)
	}

	staleTemplate := createTemplate(10)
	staleTrigger := materializeAndClaimCron(t, ctx, database, repository, service, staleTemplate.ID, "cron-worker-stale")
	revisedSpec := staleTrigger.Spec
	revisedSpec.Model.ModelID = "gpt-5.5"
	revisedSpec.Automation.ApprovalID = fmt.Sprintf("approval_%016x", cronIndex(1010))
	revised, revisedCreated, err := service.CreateRevision(ctx, CreateInput{
		TemplateID: staleTemplate.ID, ExpectedRevision: 1, Spec: revisedSpec,
		Approval: Approval{ActorType: "operator", ActorID: "cron-integration", ReasonCode: "CRON_REVISION_EDITED"},
	})
	if err != nil || !revisedCreated || revised.CurrentRevision != 2 {
		t.Fatalf("revised=%#v created=%v err=%v", revised, revisedCreated, err)
	}
	assertDenied(staleTrigger, "STALE_TEMPLATE")
	if err := removeCronTemplate(ctx, database, staleTemplate.ID); err != nil {
		t.Fatal(err)
	}

	service.now = time.Now
	expiryNow := time.Now().UTC()
	expiryIndex := cronIndex(11)
	expirySpec := postgresCronSpec(expiryNow, userID, expiryIndex)
	expirySpec.Grant.ExpiresAt = expiryNow.Add(2 * time.Second)
	expiryTemplate, created, err := service.CreateRevision(ctx, CreateInput{
		TemplateID: fmt.Sprintf("cron_%016x", expiryIndex), Spec: expirySpec,
		Approval: Approval{ActorType: "operator", ActorID: "cron-integration", ReasonCode: "CRON_TEST_APPROVED"},
	})
	if err != nil || !created {
		t.Fatalf("expiry template=%#v created=%v err=%v", expiryTemplate, created, err)
	}
	expiryTrigger := materializeAndClaimCron(t, ctx, database, repository, service, expiryTemplate.ID, "cron-worker-expiry")
	time.Sleep(time.Until(expirySpec.Grant.ExpiresAt.Add(50 * time.Millisecond)))
	assertDenied(expiryTrigger, "TEMPLATE_EXPIRED")
	if err := removeCronTemplate(ctx, database, expiryTemplate.ID); err != nil {
		t.Fatal(err)
	}
	// The expiry case waits across wall-clock time. Refresh the frozen clock so a
	// run that began near a minute boundary still computes a future Cron cursor.
	now = time.Now().UTC().Truncate(time.Second)
	service.now = func() time.Time { return now }

	// Pause does not fire, resume advances beyond now without backfill, delete is
	// a tombstone, and bounded retention removes history before the tombstone.
	lifecycle := createTemplate(20)
	paused, err := service.SetLifecycle(ctx, LifecycleInput{
		TemplateID: lifecycle.ID, UserID: userID, ExpectedRevision: 1, To: TemplatePaused,
		ActorType: "operator", ActorID: "cron-integration", ReasonCode: "CRON_PAUSED",
	})
	if err != nil || paused.State != TemplatePaused {
		t.Fatalf("pause=%#v err=%v", paused, err)
	}
	resumed, err := service.Resume(ctx, paused, Approval{ActorType: "operator", ActorID: "cron-integration", ReasonCode: "CRON_RESUMED"})
	if err != nil || resumed.State != TemplateActive || resumed.NextTriggerAt == nil || !resumed.NextTriggerAt.After(now) {
		t.Fatalf("resume=%#v err=%v", resumed, err)
	}
	deleted, err := service.SetLifecycle(ctx, LifecycleInput{
		TemplateID: lifecycle.ID, UserID: userID, ExpectedRevision: 1, To: TemplateDeleted,
		ActorType: "operator", ActorID: "cron-integration", ReasonCode: "CRON_DELETED",
	})
	if err != nil || deleted.State != TemplateDeleted || deleted.NextTriggerAt != nil {
		t.Fatalf("delete=%#v err=%v", deleted, err)
	}
	time.Sleep(20 * time.Millisecond)
	cleanup, err := service.Prune(ctx, time.Now().UTC(), 100)
	if err != nil || cleanup.TemplatesPruned != 1 || cleanup.AuditsPruned < 1 {
		t.Fatalf("prune=%#v err=%v", cleanup, err)
	}

	if retain {
		retained := createTemplate(99)
		setCronDue(t, ctx, database, retained.ID)
		return
	}
	_, _ = database.ExecContext(ctx, `DELETE FROM users WHERE id=$1`, userID)
}

func postgresCronSpec(now time.Time, userID string, index int) TemplateSpec {
	spec := testSpec(now)
	spec.Owner.UserID = userID
	spec.Schedule.Expression = "* * * * *"
	spec.Automation.ApprovalID = fmt.Sprintf("approval_%016x", index)
	spec.Grant.GrantID = fmt.Sprintf("grant_%016x", index)
	spec.Input.Ref = fmt.Sprintf("input_ref_%016x", index)
	return spec
}

func appendCronKillSwitch(t *testing.T, ctx context.Context, database *sql.DB,
	switchID, scopeType, scopeValue string, active bool, expectedRevision int64,
) {
	t.Helper()
	row := database.QueryRowContext(ctx, `
SELECT revision FROM agent_orchestrator_append_kill_switch(
  $1,$2,$3,'deny_new',$4,$5,'operator','cron-integration','CRON_KILL_SWITCH'
)
`, switchID, scopeType, scopeValue, active, expectedRevision)
	var revision int64
	if err := row.Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if revision != expectedRevision+1 {
		t.Fatalf("kill switch revision=%d", revision)
	}
}

func setCronDue(t *testing.T, ctx context.Context, database *sql.DB, templateID string) {
	t.Helper()
	if _, err := database.ExecContext(ctx, `
UPDATE agent_cron_templates SET
  next_trigger_at=COALESCE(
    (SELECT max(scheduled_for)+interval '1 millisecond' FROM agent_cron_triggers WHERE template_id=$1),
    clock_timestamp()-interval '5 seconds'
  ),
  cursor_claim_owner=NULL,cursor_claim_expires_at=NULL
WHERE id=$1
`, templateID); err != nil {
		t.Fatal(err)
	}
}

func materializeAndClaimCron(t *testing.T, ctx context.Context, database *sql.DB,
	repository *PostgresRepository, service *Service, templateID, owner string,
) Trigger {
	t.Helper()
	setCronDue(t, ctx, database, templateID)
	now := time.Now().UTC()
	claims, err := repository.ClaimDue(ctx, ClaimRequest{Owner: owner, Now: now, LeaseDuration: 30 * time.Second, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	var claim DueClaim
	for _, candidate := range claims {
		if candidate.TemplateID == templateID {
			claim = candidate
		}
	}
	if claim.TemplateID == "" {
		t.Fatalf("template %s was not claimed: %#v", templateID, claims)
	}
	decisions, next, err := planOccurrences(claim, now)
	if err != nil {
		t.Fatal(err)
	}
	for index := range decisions {
		decisions[index].TriggerID = service.newID("cron_trigger")
		decisions[index].AuditEventID = service.newID("cron_event")
		decisions[index].OccurrenceFingerprint = occurrenceFingerprint(claim.TemplateID, claim.Revision, decisions[index].ScheduledFor)
	}
	if _, err := repository.Advance(ctx, AdvanceInput{Claim: claim, ObservedAt: now, NextTriggerAt: next, Decisions: decisions}); err != nil {
		t.Fatal(err)
	}
	triggers, err := repository.ClaimTriggers(ctx, ClaimRequest{Owner: owner, Now: now, LeaseDuration: 30 * time.Second, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, trigger := range triggers {
		if trigger.TemplateID == templateID {
			return trigger
		}
	}
	t.Fatalf("template %s trigger was not claimed: %#v", templateID, triggers)
	return Trigger{}
}

func removeCronTemplate(ctx context.Context, database *sql.DB, templateID string) error {
	_, err := database.ExecContext(ctx, `DELETE FROM agent_cron_templates WHERE id=$1`, templateID)
	return err
}

func mustCronQuery(t *testing.T, ctx context.Context, database *sql.DB,
	query string, arguments ...any,
) *sql.Row {
	t.Helper()
	return database.QueryRowContext(ctx, query, arguments...)
}

func mustCronExec(t *testing.T, ctx context.Context, database *sql.DB,
	query string, arguments ...any,
) {
	t.Helper()
	if _, err := database.ExecContext(ctx, query, arguments...); err != nil {
		t.Fatal(err)
	}
}
