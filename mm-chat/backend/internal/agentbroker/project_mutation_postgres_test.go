package agentbroker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentorchestrator"
)

func TestPostgresProjectMutationApprovalCASStatusCleanup(t *testing.T) {
	databaseURL := os.Getenv("MM_CHAT_AGENT_PROJECT_MUTATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("Project mutation PostgreSQL test URL is unset")
	}
	db := openArtifactTestDB(t, databaseURL)
	defer db.Close()
	runtimeDB := db
	if runtimeURL := os.Getenv("MM_CHAT_AGENT_PROJECT_MUTATION_RUNTIME_DATABASE_URL"); runtimeURL != "" {
		runtimeDB = openArtifactTestDB(t, runtimeURL)
		defer runtimeDB.Close()
	}
	ctx := context.Background()
	userID := "31313131-3131-4313-8313-313131313131"
	if _, err := db.ExecContext(ctx, `INSERT INTO users(id,display_name) VALUES($1,'Agent Project Mutation Test') ON CONFLICT(id) DO NOTHING`, userID); err != nil {
		t.Fatal(err)
	}
	baseline := []byte("synthetic baseline\n")
	resource := "project-canary/g21-3"
	pathValue := "g21-3-canary.txt"
	var baseRevision string
	if err := db.QueryRowContext(ctx, `SELECT baseline_revision FROM agent_project_canary_provision($1,$2::uuid,$3,$4,$5,$6)`,
		resource, userID, "project_31313131", pathValue, baseline, 1024).Scan(&baseRevision); err != nil {
		t.Fatal(err)
	}

	runID := "run_3131313131313131"
	stepID := "step_3131313131313131"
	now := time.Now().UTC()
	grant := testGrantAt(now)
	grant.GrantID = "grant_3131313131313131"
	grant.Subject = Subject{UserID: userID, ProjectID: "project_31313131", AssistantID: "assistant_31313131"}
	grant.Run.RunID = runID
	grant.Capabilities = []Capability{{Capability: "project.write", Actions: []string{"apply_patch"},
		Resources: Selector{Kind: "exact", Values: []string{resource}}, Approval: ApprovalPerCommit, MaxCalls: 1}}
	grant.Budget.MaxToolCalls = 1
	grantFingerprint, err := GrantFingerprint(grant)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := BuildRegistry([]ToolDefinition{{Identity: "project.patch", Capability: "project.write",
		Actions: []string{"apply_patch"}, Classification: ClassificationMutable}}, []string{"project.patch"}, grant, now)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("synthetic mutation\n")
	contentFingerprint := fingerprint("neo-project-file-v1", content)
	mutationFingerprint := ProjectMutationFingerprint(baseRevision, resource, pathValue, contentFingerprint)
	snapshot, _ := json.Marshal(map[string]any{"schemaVersion": "neo.agent-project-canary-snapshot/v1",
		"grantFingerprint": grantFingerprint, "registryFingerprint": registry.Fingerprint,
		"projectPolicy": map[string]any{"resource": resource, "path": pathValue, "maxBytes": 1024}})

	orchestrator := agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(runtimeDB))
	enqueued, err := orchestrator.EnqueueRunWithID(ctx, runID, agentorchestrator.EnqueueInput{UserID: userID,
		IdempotencyKey: "g21.3/project/mutation", Snapshot: snapshot,
		Steps: []agentorchestrator.StepPlan{{ID: stepID, Kind: "project_mutation"}}})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := orchestrator.AcquireStep(ctx, agentorchestrator.AcquireInput{UserID: userID,
		RunID: runID, StepID: stepID, LeaseOwner: "neo-runner-project-canary", LeaseDuration: 20 * time.Minute,
		Actor: agentorchestrator.Actor{Type: "orchestrator", ID: "g21.3-test"}, ReasonCode: "PROJECT_CANARY_TEST"})
	if err != nil {
		t.Fatal(err)
	}
	transitionArtifactAttempt(t, ctx, orchestrator, userID, lease, "leased", "starting")
	transitionArtifactAttempt(t, ctx, orchestrator, userID, lease, "starting", "running")
	authority := AttemptAuthority{UserID: userID, RunID: runID, StepID: stepID, AttemptID: lease.ID,
		Generation: lease.Generation, LeaseOwner: lease.LeaseOwner, LeaseToken: lease.Token,
		SnapshotFingerprint: enqueued.Run.SnapshotFingerprint, GrantFingerprint: grantFingerprint,
		RegistryFingerprint: registry.Fingerprint, KillSwitchEpoch: currentPostgresKillSwitchEpoch(t, ctx, runtimeDB)}
	projectStore := &projectMutationRecordingRepository{inner: NewPostgresProjectMutationRepository(runtimeDB)}
	projectExecutor, err := NewProjectEffectExecutor(projectStore, ProjectMutationAuthority{Resource: resource,
		BaseRevision: baseRevision, Path: pathValue, Content: content, ContentFingerprint: contentFingerprint,
		MutationFingerprint: mutationFingerprint})
	if err != nil {
		t.Fatal(err)
	}
	broker, err := NewService(NewPostgresRepository(runtimeDB), map[string]EffectExecutor{"project.patch": projectExecutor})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := broker.Prepare(ctx, PrepareInput{RequestID: "request_3131313131313131", Attempt: authority,
		Grant: grant, Registry: registry, ToolIdentity: "project.patch", Action: "apply_patch", Resource: resource,
		Arguments: projectExecutor.arguments, BaseRevision: baseRevision, TTL: 10 * time.Minute})
	if err != nil || prepared.State != IntentAwaitingApproval {
		t.Fatalf("Prepare = %#v, %v", prepared, err)
	}
	if request := executionRequest(prepared); !projectExecutor.matches(request) {
		t.Fatalf("prepared execution mismatch: tool=%q capability=%q action=%q resource=%q base=%q args=%s expected=%s",
			request.ToolIdentity, request.Capability, request.Action, request.Resource, request.BaseRevision,
			request.Arguments, projectExecutor.arguments)
	}
	approved, err := broker.DecideApproval(ctx, ApprovalInput{ApprovalID: "approval_3131313131313131",
		UserID: userID, IntentID: prepared.IntentID, IntentFingerprint: prepared.IntentFingerprint,
		Decision: "approved", ActorType: "operator", ActorID: "g21.3-test",
		ReasonCode: "PROJECT_CANARY_TEST", ExpectedRevision: 1})
	if err != nil || approved.State != IntentApproved {
		t.Fatalf("approval = %#v, %v", approved, err)
	}
	result, err := broker.Commit(ctx, CommitInput{Attempt: authority, IntentID: prepared.IntentID,
		IntentFingerprint: prepared.IntentFingerprint, ApprovalID: "approval_3131313131313131",
		IdempotencyKey: prepared.IdempotencyKey})
	if err != nil || result.Outcome != OutcomeCommitted || !validFingerprint(result.ReceiptFingerprint) {
		t.Fatalf("Commit = %#v, %v, store=%v", result, err, projectStore.lastErr)
	}
	replay, err := broker.Commit(ctx, CommitInput{Attempt: authority, IntentID: prepared.IntentID,
		IntentFingerprint: prepared.IntentFingerprint, ApprovalID: "approval_3131313131313131",
		IdempotencyKey: prepared.IdempotencyKey})
	if err != nil || replay.Outcome != OutcomeReplayed || replay.ReceiptFingerprint != result.ReceiptFingerprint {
		t.Fatalf("replay = %#v, %v", replay, err)
	}
	if found, revision, err := projectExecutor.ReconcileCleanup(ctx, userID, runID); err != nil || !found || revision != baseRevision {
		var intentState, intentReceipt, storedReceipt string
		_ = db.QueryRowContext(ctx, `SELECT state,COALESCE(receipt_fingerprint,'') FROM agent_effect_intents WHERE id=$1`,
			prepared.IntentID).Scan(&intentState, &intentReceipt)
		_ = db.QueryRowContext(ctx, `SELECT receipt_fingerprint FROM agent_project_mutation_receipts WHERE intent_id=$1`,
			prepared.IntentID).Scan(&storedReceipt)
		t.Fatalf("cleanup reconcile = %v/%v/%s, intent=%s/%s stored=%s result=%s", found, err, revision, intentState, intentReceipt, storedReceipt,
			result.ReceiptFingerprint)
	}
	if _, err := projectExecutor.Cleanup(ctx, executionRequest(prepared), result.ReceiptFingerprint); err != nil {
		t.Fatalf("cleanup replay = %v", err)
	}
	var state, revision string
	if err := db.QueryRowContext(ctx, `SELECT state,current_revision FROM agent_project_canary_resources WHERE resource_id=$1`, resource).Scan(&state, &revision); err != nil || state != "clean" || revision != baseRevision {
		t.Fatalf("resource after cleanup = %s %s, %v", state, revision, err)
	}
	collision := ProjectMutationAuthority{UserID: userID, RunID: runID, AttemptID: lease.ID,
		Generation: lease.Generation, SnapshotFingerprint: enqueued.Run.SnapshotFingerprint,
		GrantFingerprint: grantFingerprint, RegistryFingerprint: registry.Fingerprint,
		IntentID: prepared.IntentID, IdempotencyKey: prepared.IdempotencyKey, Resource: resource,
		BaseRevision: baseRevision, Path: pathValue, Content: []byte("drift"), ContentFingerprint: contentFingerprint,
		MutationFingerprint: mutationFingerprint}
	if _, err := projectStore.inner.CommitProjectMutation(ctx, collision); !errors.Is(err, ErrProjectMutationDenied) {
		t.Fatalf("content collision = %v", err)
	}
}

func TestPostgresProjectMutationStaleGrantAndKillFencesPerformZeroCAS(t *testing.T) {
	databaseURL := os.Getenv("MM_CHAT_AGENT_PROJECT_MUTATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("Project mutation PostgreSQL test URL is unset")
	}
	adminDB := openArtifactTestDB(t, databaseURL)
	defer adminDB.Close()
	runtimeDB := adminDB
	if runtimeURL := os.Getenv("MM_CHAT_AGENT_PROJECT_MUTATION_RUNTIME_DATABASE_URL"); runtimeURL != "" {
		runtimeDB = openArtifactTestDB(t, runtimeURL)
		defer runtimeDB.Close()
	}
	ctx := context.Background()

	tests := []struct {
		name      string
		identity  projectMutationPostgresIdentity
		expected  error
		beforeCAS func(context.Context, *testing.T, *sql.DB, *sql.DB, projectMutationPostgresFixture)
	}{
		{name: "stale lease", identity: projectMutationPostgresIdentity{
			userID: "41414141-4141-4414-8414-414141414141", runID: "run_4141414141414141",
			stepID: "step_4141414141414141", grantID: "grant_4141414141414141",
			requestID: "request_4141414141414141", approvalID: "approval_4141414141414141",
			resource: "project-canary/g21-3-stale", projectID: "project_41414141",
			path: "g21-3-stale.txt", idempotencyKey: "g21.3/project/stale",
		}, expected: ErrLeaseStale, beforeCAS: func(ctx context.Context, t *testing.T, adminDB, _ *sql.DB,
			fixture projectMutationPostgresFixture,
		) {
			t.Helper()
			if result, err := adminDB.ExecContext(ctx, `UPDATE agent_attempts
SET lease_expires_at=created_at WHERE id=$1`, fixture.lease.ID); err != nil {
				t.Fatal(err)
			} else if changed, err := result.RowsAffected(); err != nil || changed != 1 {
				t.Fatalf("expire lease rows = %d, %v", changed, err)
			}
		}},
		{name: "revoked Grant", identity: projectMutationPostgresIdentity{
			userID: "51515151-5151-4515-8515-515151515151", runID: "run_5151515151515151",
			stepID: "step_5151515151515151", grantID: "grant_5151515151515151",
			requestID: "request_5151515151515151", approvalID: "approval_5151515151515151",
			resource: "project-canary/g21-3-revoked", projectID: "project_51515151",
			path: "g21-3-revoked.txt", idempotencyKey: "g21.3/project/revoked",
		}, expected: ErrGrantDenied, beforeCAS: func(ctx context.Context, t *testing.T, _, runtimeDB *sql.DB,
			fixture projectMutationPostgresFixture,
		) {
			t.Helper()
			created, err := NewPostgresRepository(runtimeDB).RevokeGrant(ctx, GrantRevocationInput{
				GrantID: fixture.grant.GrantID, GrantFingerprint: fixture.grantFingerprint,
				ActorType: "operator", ActorID: "g21.3-test", ReasonCode: "PROJECT_CANARY_TEST",
			})
			if err != nil || !created {
				t.Fatalf("RevokeGrant = %v, %v", created, err)
			}
		}},
		{name: "Kill Switch epoch drift", identity: projectMutationPostgresIdentity{
			userID: "61616161-6161-4616-8616-616161616161", runID: "run_6161616161616161",
			stepID: "step_6161616161616161", grantID: "grant_6161616161616161",
			requestID: "request_6161616161616161", approvalID: "approval_6161616161616161",
			resource: "project-canary/g21-3-killed", projectID: "project_61616161",
			path: "g21-3-killed.txt", idempotencyKey: "g21.3/project/killed",
		}, expected: ErrKillSwitchActive, beforeCAS: func(ctx context.Context, t *testing.T, adminDB, _ *sql.DB,
			fixture projectMutationPostgresFixture,
		) {
			t.Helper()
			orchestrator := agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(adminDB))
			if _, err := orchestrator.AppendKillSwitch(ctx, agentorchestrator.KillSwitchInput{
				ScopeType: "run", ScopeValue: fixture.identity.runID, Mode: agentorchestrator.KillKill,
				Active: true, Actor: agentorchestrator.Actor{Type: "operator", ID: "g21.3-test"},
				ReasonCode: "PROJECT_CANARY_TEST",
			}); err != nil {
				t.Fatal(err)
			}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := preparePostgresProjectMutationFixture(t, ctx, adminDB, runtimeDB, test.identity)
			hookedStore := &projectMutationHookRepository{
				inner:        NewPostgresProjectMutationRepository(runtimeDB),
				beforeCommit: func() { test.beforeCAS(ctx, t, adminDB, runtimeDB, fixture) },
			}
			executor, err := NewProjectEffectExecutor(hookedStore, fixture.projectAuthority)
			if err != nil {
				t.Fatal(err)
			}
			broker, err := NewService(NewPostgresRepository(runtimeDB), map[string]EffectExecutor{"project.patch": executor})
			if err != nil {
				t.Fatal(err)
			}
			result, commitErr := broker.Commit(ctx, CommitInput{Attempt: fixture.attemptAuthority,
				IntentID: fixture.prepared.IntentID, IntentFingerprint: fixture.prepared.IntentFingerprint,
				ApprovalID: fixture.identity.approvalID, IdempotencyKey: fixture.prepared.IdempotencyKey})
			if commitErr != nil || result.Outcome != OutcomeRejected {
				t.Fatalf("Commit = %#v, %v", result, commitErr)
			}
			if hookedStore.commits != 1 || !errors.Is(hookedStore.lastErr, test.expected) {
				t.Fatalf("Project CAS fence = calls:%d error:%v, want %v", hookedStore.commits,
					hookedStore.lastErr, test.expected)
			}
			var receipts int
			var state, revision string
			if err := adminDB.QueryRowContext(ctx, `SELECT count(*) FROM agent_project_mutation_receipts
WHERE intent_id=$1`, fixture.prepared.IntentID).Scan(&receipts); err != nil {
				t.Fatal(err)
			}
			if err := adminDB.QueryRowContext(ctx, `SELECT state,current_revision
FROM agent_project_canary_resources WHERE resource_id=$1`, fixture.identity.resource).
				Scan(&state, &revision); err != nil {
				t.Fatal(err)
			}
			if receipts != 0 || state != "clean" || revision != fixture.baseRevision {
				t.Fatalf("fenced Project state = receipts:%d state:%s revision:%s base:%s",
					receipts, state, revision, fixture.baseRevision)
			}
		})
	}
}

type projectMutationPostgresIdentity struct {
	userID, runID, stepID, grantID, requestID, approvalID string
	resource, projectID, path, idempotencyKey             string
}

type projectMutationPostgresFixture struct {
	identity         projectMutationPostgresIdentity
	lease            agentorchestrator.Lease
	grant            CapabilityGrant
	grantFingerprint string
	baseRevision     string
	prepared         PreparedIntent
	attemptAuthority AttemptAuthority
	projectAuthority ProjectMutationAuthority
}

func preparePostgresProjectMutationFixture(t *testing.T, ctx context.Context, adminDB, runtimeDB *sql.DB,
	identity projectMutationPostgresIdentity,
) projectMutationPostgresFixture {
	t.Helper()
	if _, err := adminDB.ExecContext(ctx, `INSERT INTO users(id,display_name)
VALUES($1,'Agent Project Mutation Fence Test') ON CONFLICT(id) DO NOTHING`, identity.userID); err != nil {
		t.Fatal(err)
	}
	baseline := []byte("synthetic baseline\n")
	var baseRevision string
	if err := adminDB.QueryRowContext(ctx, `SELECT baseline_revision
FROM agent_project_canary_provision($1,$2::uuid,$3,$4,$5,$6)`, identity.resource,
		identity.userID, identity.projectID, identity.path, baseline, 1024).Scan(&baseRevision); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	grant := testGrantAt(now)
	grant.GrantID = identity.grantID
	grant.Subject = Subject{UserID: identity.userID, ProjectID: identity.projectID, AssistantID: "assistant_71717171"}
	grant.Run.RunID = identity.runID
	grant.Capabilities = []Capability{{Capability: "project.write", Actions: []string{"apply_patch"},
		Resources: Selector{Kind: "exact", Values: []string{identity.resource}}, Approval: ApprovalPerCommit, MaxCalls: 1}}
	grant.Budget.MaxToolCalls = 1
	grantFingerprint, err := GrantFingerprint(grant)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := BuildRegistry([]ToolDefinition{{Identity: "project.patch", Capability: "project.write",
		Actions: []string{"apply_patch"}, Classification: ClassificationMutable}}, []string{"project.patch"}, grant, now)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("synthetic mutation\n")
	contentFingerprint := fingerprint("neo-project-file-v1", content)
	mutationFingerprint := ProjectMutationFingerprint(baseRevision, identity.resource, identity.path, contentFingerprint)
	snapshot, err := json.Marshal(map[string]any{"schemaVersion": "neo.agent-project-canary-snapshot/v1",
		"grantFingerprint": grantFingerprint, "registryFingerprint": registry.Fingerprint,
		"projectPolicy": map[string]any{"resource": identity.resource, "path": identity.path, "maxBytes": 1024}})
	if err != nil {
		t.Fatal(err)
	}
	orchestrator := agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(runtimeDB))
	enqueued, err := orchestrator.EnqueueRunWithID(ctx, identity.runID, agentorchestrator.EnqueueInput{
		UserID: identity.userID, IdempotencyKey: identity.idempotencyKey, Snapshot: snapshot,
		Steps: []agentorchestrator.StepPlan{{ID: identity.stepID, Kind: "project_mutation"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := orchestrator.AcquireStep(ctx, agentorchestrator.AcquireInput{UserID: identity.userID,
		RunID: identity.runID, StepID: identity.stepID, LeaseOwner: "neo-runner-project-canary",
		LeaseDuration: 20 * time.Minute, Actor: agentorchestrator.Actor{Type: "orchestrator", ID: "g21.3-test"},
		ReasonCode: "PROJECT_CANARY_TEST"})
	if err != nil {
		t.Fatal(err)
	}
	transitionArtifactAttempt(t, ctx, orchestrator, identity.userID, lease, "leased", "starting")
	transitionArtifactAttempt(t, ctx, orchestrator, identity.userID, lease, "starting", "running")
	attemptAuthority := AttemptAuthority{UserID: identity.userID, RunID: identity.runID, StepID: identity.stepID,
		AttemptID: lease.ID, Generation: lease.Generation, LeaseOwner: lease.LeaseOwner, LeaseToken: lease.Token,
		SnapshotFingerprint: enqueued.Run.SnapshotFingerprint, GrantFingerprint: grantFingerprint,
		RegistryFingerprint: registry.Fingerprint, KillSwitchEpoch: currentPostgresKillSwitchEpoch(t, ctx, runtimeDB)}
	projectAuthority := ProjectMutationAuthority{Resource: identity.resource, BaseRevision: baseRevision,
		Path: identity.path, Content: content, ContentFingerprint: contentFingerprint,
		MutationFingerprint: mutationFingerprint}
	executor, err := NewProjectEffectExecutor(NewPostgresProjectMutationRepository(runtimeDB), projectAuthority)
	if err != nil {
		t.Fatal(err)
	}
	broker, err := NewService(NewPostgresRepository(runtimeDB), map[string]EffectExecutor{"project.patch": executor})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := broker.Prepare(ctx, PrepareInput{RequestID: identity.requestID, Attempt: attemptAuthority,
		Grant: grant, Registry: registry, ToolIdentity: "project.patch", Action: "apply_patch", Resource: identity.resource,
		Arguments: executor.arguments, BaseRevision: baseRevision, TTL: 10 * time.Minute})
	if err != nil || prepared.State != IntentAwaitingApproval {
		t.Fatalf("Prepare = %#v, %v", prepared, err)
	}
	approved, err := broker.DecideApproval(ctx, ApprovalInput{ApprovalID: identity.approvalID,
		UserID: identity.userID, IntentID: prepared.IntentID, IntentFingerprint: prepared.IntentFingerprint,
		Decision: "approved", ActorType: "operator", ActorID: "g21.3-test",
		ReasonCode: "PROJECT_CANARY_TEST", ExpectedRevision: 1})
	if err != nil || approved.State != IntentApproved {
		t.Fatalf("approval = %#v, %v", approved, err)
	}
	return projectMutationPostgresFixture{identity: identity, lease: lease, grant: grant,
		grantFingerprint: grantFingerprint, baseRevision: baseRevision, prepared: prepared,
		attemptAuthority: attemptAuthority, projectAuthority: projectAuthority}
}

type projectMutationHookRepository struct {
	inner        *PostgresProjectMutationRepository
	beforeCommit func()
	commits      int
	lastErr      error
}

func (repository *projectMutationHookRepository) CommitProjectMutation(ctx context.Context,
	authority ProjectMutationAuthority,
) (ProjectMutationReceipt, error) {
	repository.commits++
	if repository.beforeCommit != nil {
		repository.beforeCommit()
	}
	receipt, err := repository.inner.CommitProjectMutation(ctx, authority)
	repository.lastErr = err
	return receipt, err
}

func (repository *projectMutationHookRepository) ProjectMutationStatus(ctx context.Context,
	authority ProjectMutationAuthority,
) (ProjectMutationReceipt, error) {
	return repository.inner.ProjectMutationStatus(ctx, authority)
}

func (repository *projectMutationHookRepository) CleanupProjectMutation(ctx context.Context,
	authority ProjectMutationAuthority, receiptFingerprint string,
) (string, error) {
	return repository.inner.CleanupProjectMutation(ctx, authority, receiptFingerprint)
}

type projectMutationRecordingRepository struct {
	inner   *PostgresProjectMutationRepository
	lastErr error
}

func (repository *projectMutationRecordingRepository) CommitProjectMutation(ctx context.Context,
	authority ProjectMutationAuthority,
) (ProjectMutationReceipt, error) {
	receipt, err := repository.inner.CommitProjectMutation(ctx, authority)
	repository.lastErr = err
	return receipt, err
}

func (repository *projectMutationRecordingRepository) ProjectMutationStatus(ctx context.Context,
	authority ProjectMutationAuthority,
) (ProjectMutationReceipt, error) {
	receipt, err := repository.inner.ProjectMutationStatus(ctx, authority)
	repository.lastErr = err
	return receipt, err
}

func (repository *projectMutationRecordingRepository) CleanupProjectMutation(ctx context.Context,
	authority ProjectMutationAuthority, receiptFingerprint string,
) (string, error) {
	revision, err := repository.inner.CleanupProjectMutation(ctx, authority, receiptFingerprint)
	repository.lastErr = err
	return revision, err
}

func (repository *projectMutationRecordingRepository) PendingProjectMutationCleanup(ctx context.Context,
	userID, runID, resource string,
) (ProjectMutationCleanupCandidate, bool, error) {
	candidate, found, err := repository.inner.PendingProjectMutationCleanup(ctx, userID, runID, resource)
	repository.lastErr = err
	return candidate, found, err
}
