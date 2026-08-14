package agentdelegation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentorchestrator"
)

type flakyPostgresReaper struct {
	mu            sync.Mutex
	failRemaining int
	calls         []ReapTarget
}

func (reaper *flakyPostgresReaper) Reap(_ context.Context, target ReapTarget) error {
	reaper.mu.Lock()
	defer reaper.mu.Unlock()
	reaper.calls = append(reaper.calls, target)
	if reaper.failRemaining > 0 {
		reaper.failRemaining--
		return errors.New("held reaper failure")
	}
	return nil
}

func TestPostgresAgentDelegationDepthBudgetLaunchCascadeAndSettlement(t *testing.T) {
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MM_CHAT_TEST_DATABASE_URL is unset")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	db := stdlib.OpenDB(*config)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var valid bool
	if err := db.QueryRowContext(ctx, `SELECT agent_delegation_budget_valid('{"maxWallSeconds":1}'::jsonb)`).Scan(&valid); err != nil || valid {
		t.Fatalf("malformed budget valid=%v err=%v", valid, err)
	}
	userID := "66666666-6666-4666-8666-666666666666"
	_, _ = db.ExecContext(ctx, `INSERT INTO users(id,display_name) VALUES($1,'Agent Delegation Test') ON CONFLICT(id) DO NOTHING`, userID)
	orchestrator := agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(db))
	rootResult, err := orchestrator.EnqueueRun(ctx, agentorchestrator.EnqueueInput{UserID: userID, IdempotencyKey: "delegation-root", Snapshot: json.RawMessage(`{"schemaVersion":"neo.agent-snapshot/v1","held":true}`), Steps: []agentorchestrator.StepPlan{{Kind: "delegate"}}})
	if err != nil {
		t.Fatal(err)
	}
	parentLease, err := orchestrator.AcquireStep(ctx, agentorchestrator.AcquireInput{UserID: userID, RunID: rootResult.Run.ID, StepID: rootResult.Run.Steps[0].ID, LeaseOwner: "neo-runner-delegation", LeaseDuration: 20 * time.Minute, Actor: agentorchestrator.Actor{Type: "orchestrator", ID: "delegation-test"}, ReasonCode: "DELEGATION_TEST"})
	if err != nil {
		t.Fatal(err)
	}
	transition := agentorchestrator.TransitionInput{UserID: userID, RunID: rootResult.Run.ID, StepID: parentLease.StepID, AttemptID: parentLease.ID, Generation: parentLease.Generation, LeaseOwner: parentLease.LeaseOwner, LeaseToken: parentLease.Token, Actor: agentorchestrator.Actor{Type: "runner", ID: parentLease.LeaseOwner}, ReasonCode: "DELEGATION_TEST"}
	for _, states := range [][2]string{{agentorchestrator.AttemptLeased, agentorchestrator.AttemptStarting}, {agentorchestrator.AttemptStarting, agentorchestrator.AttemptRunning}} {
		transition.Expected, transition.To = states[0], states[1]
		if err := orchestrator.TransitionAttempt(ctx, transition); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	rootGrant := grantAt(now, rootResult.Run.ID, 0, "")
	rootGrant.Subject.UserID = userID
	rootRegistry, err := agentbroker.BuildRegistry(catalog(), []string{"delegate_task", "workspace_read"}, rootGrant, now)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(NewPostgresRepository(db), &fakeReaper{})
	rootAuthority, created, err := service.RegisterRoot(ctx, RegisterRootInput{UserID: userID, RunID: rootResult.Run.ID, Grant: rootGrant, Registry: rootRegistry, Model: ModelBinding{Provider: "fixture", ModelID: "fixture"}})
	if err != nil || !created {
		t.Fatalf("root=%#v created=%v err=%v", rootAuthority, created, err)
	}
	proposal := ChildProposal{UserID: userID, IdempotencyKey: "delegation-child", ParentRunID: rootResult.Run.ID, ParentAttempt: ParentAttempt{StepID: parentLease.StepID, AttemptID: parentLease.ID, Generation: parentLease.Generation, LeaseOwner: parentLease.LeaseOwner, LeaseToken: parentLease.Token}, Model: rootAuthority.Model, Grant: grantAt(now, "run_fedcba9876543210", 1, rootResult.Run.ID), RequestedTools: []string{"workspace_read"}, Catalog: catalog(), Steps: []agentorchestrator.StepPlan{{Kind: "work"}}}
	proposal.Grant.Subject.UserID = userID
	proposal.Grant.IssuedAt = rootAuthority.Grant.IssuedAt
	proposal.Grant.ExpiresAt = rootAuthority.Grant.ExpiresAt
	proposal.Grant.Capabilities = proposal.Grant.Capabilities[1:]
	proposal.Grant.Budget = agentbroker.Budget{MaxWallSeconds: 100, MaxModelTokens: 250, MaxToolCalls: 2, MaxArtifactBytes: 1024}
	if proposal.Grant.Subject != rootAuthority.Subject || proposal.Grant.PackageFingerprint != rootAuthority.PackageFingerprint ||
		proposal.Grant.RuntimeBundleFingerprint != rootAuthority.RuntimeBundleFingerprint || proposal.Model != rootAuthority.Model ||
		proposal.Grant.IssuedAt.Before(rootAuthority.Grant.IssuedAt) || proposal.Grant.ExpiresAt.After(rootAuthority.ExpiresAt) ||
		!budgetContained(proposal.Grant.Budget, rootAuthority.Budget) ||
		!capabilitiesContained(proposal.Grant.Capabilities, rootAuthority.Grant.Capabilities) ||
		!egressContained(proposal.Grant.Egress, rootAuthority.Grant.Egress) ||
		!secretsContained(proposal.Grant.Secrets, rootAuthority.Grant.Secrets) {
		t.Fatalf("preflight subset mismatch parent=%#v proposal=%#v", rootAuthority, proposal.Grant)
	}
	preflight, err := service.deriveChild(rootAuthority, proposal)
	if err != nil {
		t.Fatal(err)
	}
	preflightGrant, _ := json.Marshal(preflight.ChildGrant)
	rootGrantJSON, _ := json.Marshal(rootAuthority.Grant)
	var databaseSubset bool
	if err := db.QueryRowContext(ctx, `SELECT agent_delegation_json_subset($1::jsonb,$2::jsonb)`, string(preflightGrant), string(rootGrantJSON)).Scan(&databaseSubset); err != nil || !databaseSubset {
		t.Fatalf("database subset=%v err=%v child=%s parent=%s", databaseSubset, err, preflightGrant, rootGrantJSON)
	}
	prefixParent := rootAuthority.Grant
	prefixParent.Capabilities = append([]agentbroker.Capability(nil), prefixParent.Capabilities[1:]...)
	prefixParent.Capabilities[0].Resources = agentbroker.Selector{Kind: "prefix", Values: []string{"project/a_"}}
	prefixChild := proposal.Grant
	prefixChild.Capabilities = append([]agentbroker.Capability(nil), prefixChild.Capabilities...)
	prefixChild.Capabilities[0].Resources = agentbroker.Selector{Kind: "exact", Values: []string{"project/abc"}}
	prefixChild.IssuedAt = prefixParent.IssuedAt
	prefixChild.ExpiresAt = prefixParent.ExpiresAt
	prefixChildJSON, _ := json.Marshal(prefixChild)
	prefixParentJSON, _ := json.Marshal(prefixParent)
	if err := db.QueryRowContext(ctx, `SELECT agent_delegation_json_subset($1::jsonb,$2::jsonb)`, string(prefixChildJSON), string(prefixParentJSON)).Scan(&databaseSubset); err != nil || databaseSubset {
		t.Fatalf("underscore prefix widening subset=%v err=%v", databaseSubset, err)
	}
	registryDrift := preflight.ChildRegistry
	registryDrift.Tools = append([]agentbroker.RegistryTool(nil), registryDrift.Tools...)
	registryDrift.Tools[0].Actions = []string{"write"}
	registryDriftJSON, _ := json.Marshal(registryDrift)
	rootRegistryJSON, _ := json.Marshal(rootAuthority.Registry)
	if err := db.QueryRowContext(ctx, `SELECT agent_delegation_registry_subset($1::jsonb,$2::jsonb)`, string(registryDriftJSON), string(rootRegistryJSON)).Scan(&databaseSubset); err != nil || databaseSubset {
		t.Fatalf("registry action widening subset=%v err=%v", databaseSubset, err)
	}
	forgedDerivation := preflight
	forgedDerivation.ChildRegistry.Tools = append([]agentbroker.RegistryTool(nil), preflight.ChildRegistry.Tools...)
	forgedDerivation.ChildRegistry.Tools = append(forgedDerivation.ChildRegistry.Tools, agentbroker.RegistryTool{Identity: "workspace_write", Capability: "workspace.read", Actions: []string{"read"}, Resources: agentbroker.Selector{Kind: "prefix", Values: []string{"project/"}}, Approval: agentbroker.ApprovalAutomatic, MaxCalls: 1, Classification: agentbroker.ClassificationRead, Idempotent: true})
	forgedDerivation.ChildRegistry.Fingerprint = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	var forgedSnapshot map[string]any
	if err := json.Unmarshal(forgedDerivation.ChildSnapshot, &forgedSnapshot); err != nil {
		t.Fatal(err)
	}
	forgedSnapshot["registryFingerprint"] = forgedDerivation.ChildRegistry.Fingerprint
	forgedDerivation.ChildSnapshot, _ = json.Marshal(forgedSnapshot)
	forgedDerivation.ChildSnapshotFingerprint = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	forgedProposal := proposal
	forgedProposal.IdempotencyKey = "delegation-forged-registry"
	if _, _, err := service.repository.EnqueueChild(ctx, forgedDerivation, forgedProposal); !errors.Is(err, ErrSubsetViolation) {
		t.Fatalf("forged Registry subset error=%v", err)
	}
	child, err := service.EnqueueChild(ctx, proposal)
	if err != nil || !child.Created || child.Authority.Depth != 1 {
		t.Fatalf("child=%#v err=%v", child, err)
	}
	recursive := proposal
	recursive.IdempotencyKey = "delegation-depth-two"
	recursive.ParentRunID = child.Authority.RunID
	if _, err := service.EnqueueChild(ctx, recursive); !errors.Is(err, ErrDepthExceeded) {
		t.Fatalf("recursive delegation error=%v", err)
	}
	crossUser := proposal
	crossUser.IdempotencyKey = "delegation-cross-user"
	crossUser.UserID = "77777777-7777-4777-8777-777777777777"
	if _, err := service.EnqueueChild(ctx, crossUser); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user delegation error=%v", err)
	}
	replayed, err := service.EnqueueChild(ctx, proposal)
	if err != nil || replayed.Created || replayed.Authority.RunID != child.Authority.RunID {
		t.Fatalf("replay=%#v err=%v", replayed, err)
	}
	mismatchedReplay := proposal
	mismatchedReplay.Grant.Budget.MaxWallSeconds--
	if _, err := service.EnqueueChild(ctx, mismatchedReplay); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("mismatched replay error=%v", err)
	}
	widened := proposal
	widened.IdempotencyKey = "delegation-widened"
	widened.Grant.Budget.MaxToolCalls = rootGrant.Budget.MaxToolCalls + 1
	if _, err := service.EnqueueChild(ctx, widened); !errors.Is(err, ErrSubsetViolation) {
		t.Fatalf("widened error=%v", err)
	}
	type concurrentResult struct {
		result EnqueueResult
		err    error
	}
	var wg sync.WaitGroup
	results := make(chan concurrentResult, 2)
	for index := range 2 {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			candidate := proposal
			candidate.IdempotencyKey = "delegation-race-" + string(rune('a'+index))
			candidate.Grant.Budget = agentbroker.Budget{MaxWallSeconds: 150, MaxModelTokens: 400, MaxToolCalls: 5, MaxArtifactBytes: 2048}
			raced, raceErr := service.EnqueueChild(context.Background(), candidate)
			results <- concurrentResult{result: raced, err: raceErr}
		}(index)
	}
	wg.Wait()
	close(results)
	budgetDenied := 0
	var cascadeChild EnqueueResult
	for raced := range results {
		if errors.Is(raced.err, ErrBudgetExceeded) {
			budgetDenied++
		} else if raced.err != nil {
			t.Fatalf("race error=%v", raced.err)
		} else if !raced.result.Created {
			t.Fatalf("race result=%#v", raced.result)
		} else {
			cascadeChild = raced.result
		}
	}
	if budgetDenied != 1 || cascadeChild.Authority.RunID == "" {
		t.Fatalf("budget denied=%d cascade child=%#v", budgetDenied, cascadeChild)
	}
	childRun, err := orchestrator.GetRun(ctx, userID, child.Authority.RunID)
	if err != nil {
		t.Fatal(err)
	}
	childLease, err := orchestrator.AcquireStep(ctx, agentorchestrator.AcquireInput{UserID: userID, RunID: childRun.ID, StepID: childRun.Steps[0].ID, LeaseOwner: "neo-runner-child", LeaseDuration: 20 * time.Minute, Actor: agentorchestrator.Actor{Type: "orchestrator", ID: "delegation-test"}, ReasonCode: "DELEGATION_TEST"})
	if err != nil {
		t.Fatal(err)
	}
	tools := []string{}
	for _, tool := range child.Authority.Registry.Tools {
		tools = append(tools, tool.Identity)
	}
	admission := LaunchAdmissionInput{UserID: userID, RunID: childRun.ID, AttemptID: childLease.ID, Generation: childLease.Generation, LeaseOwner: childLease.LeaseOwner, LeaseToken: childLease.Token, SnapshotFingerprint: child.Authority.SnapshotFingerprint, GrantFingerprint: child.Authority.GrantFingerprint, RegistryFingerprint: child.Authority.RegistryFingerprint, RegistryTools: tools}
	if err := service.AdmitLaunch(ctx, admission); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*LaunchAdmissionInput){
		"snapshot": func(input *LaunchAdmissionInput) {
			input.SnapshotFingerprint = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		},
		"grant": func(input *LaunchAdmissionInput) {
			input.GrantFingerprint = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		},
		"registry": func(input *LaunchAdmissionInput) {
			input.RegistryFingerprint = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
		},
		"tool identity": func(input *LaunchAdmissionInput) {
			input.RegistryTools = append(append([]string(nil), input.RegistryTools...), "delegate_task")
		},
	} {
		t.Run("launch drift "+name, func(t *testing.T) {
			candidate := admission
			candidate.RegistryTools = append([]string(nil), admission.RegistryTools...)
			mutate(&candidate)
			if err := service.AdmitLaunch(ctx, candidate); !errors.Is(err, ErrSnapshotMismatch) {
				t.Fatalf("launch drift error=%v", err)
			}
		})
	}
	killSwitch, err := orchestrator.AppendKillSwitch(ctx, agentorchestrator.KillSwitchInput{ScopeType: "run", ScopeValue: childRun.ID, Mode: agentorchestrator.KillDenyNew, Active: true, ExpectedRevision: 0, Actor: agentorchestrator.Actor{Type: "operator", ID: "delegation-test"}, ReasonCode: "DELEGATION_TEST"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.AdmitLaunch(ctx, admission); !errors.Is(err, ErrKillSwitchActive) {
		t.Fatalf("Kill Switch launch error=%v", err)
	}
	if _, err := orchestrator.AppendKillSwitch(ctx, agentorchestrator.KillSwitchInput{ID: killSwitch.ID, ScopeType: killSwitch.ScopeType, ScopeValue: killSwitch.ScopeValue, Mode: killSwitch.Mode, Active: false, ExpectedRevision: killSwitch.Revision, Actor: agentorchestrator.Actor{Type: "operator", ID: "delegation-test"}, ReasonCode: "DELEGATION_TEST"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM agent_kill_switches WHERE switch_id=$1`, killSwitch.ID); err != nil {
		t.Fatal(err)
	}
	parentStale := admission
	if _, err := db.ExecContext(ctx, `UPDATE agent_attempts SET lease_expires_at=clock_timestamp() WHERE id=$1`, parentLease.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.AdmitLaunch(ctx, parentStale); !errors.Is(err, ErrParentStale) {
		t.Fatalf("stale parent error=%v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE agent_attempts SET lease_expires_at=clock_timestamp()+interval '20 minutes' WHERE id=$1`, parentLease.ID); err != nil {
		t.Fatal(err)
	}
	admission.LeaseToken = "lease_wrongwrongwrongwrongwrong000"
	if err := service.AdmitLaunch(ctx, admission); !errors.Is(err, ErrLeaseStale) {
		t.Fatalf("stale child error=%v", err)
	}
	if _, err := service.Settle(ctx, SettleInput{UserID: userID, ChildRunID: childRun.ID, Usage: agentbroker.Budget{}, Outcome: "succeeded"}); !errors.Is(err, ErrSettlementInvalid) {
		t.Fatalf("nonterminal settlement error=%v", err)
	}
	childTransition := agentorchestrator.TransitionInput{UserID: userID, RunID: childRun.ID, StepID: childLease.StepID, AttemptID: childLease.ID, Generation: childLease.Generation, LeaseOwner: childLease.LeaseOwner, LeaseToken: childLease.Token, Actor: agentorchestrator.Actor{Type: "runner", ID: childLease.LeaseOwner}, ReasonCode: "DELEGATION_TEST"}
	for _, states := range [][2]string{{agentorchestrator.AttemptLeased, agentorchestrator.AttemptStarting}, {agentorchestrator.AttemptStarting, agentorchestrator.AttemptRunning}, {agentorchestrator.AttemptRunning, agentorchestrator.AttemptPrepared}, {agentorchestrator.AttemptPrepared, agentorchestrator.AttemptCommitting}, {agentorchestrator.AttemptCommitting, agentorchestrator.AttemptSucceeded}} {
		childTransition.Expected, childTransition.To = states[0], states[1]
		if err := orchestrator.TransitionAttempt(ctx, childTransition); err != nil {
			t.Fatal(err)
		}
	}
	childTransition.Expected, childTransition.To = agentorchestrator.StepRunning, agentorchestrator.StepSucceeded
	if err := orchestrator.TransitionStep(ctx, childTransition); err != nil {
		t.Fatal(err)
	}
	childTransition.Expected, childTransition.To = agentorchestrator.RunRunning, agentorchestrator.RunSucceeded
	if err := orchestrator.TransitionRun(ctx, childTransition); err != nil {
		t.Fatal(err)
	}
	settled, err := service.Settle(ctx, SettleInput{UserID: userID, ChildRunID: childRun.ID, Usage: agentbroker.Budget{MaxWallSeconds: 50, MaxModelTokens: 100, MaxToolCalls: 1, MaxArtifactBytes: 512}, Outcome: "succeeded"})
	if err != nil || !settled {
		t.Fatalf("settled=%v err=%v", settled, err)
	}
	settled, err = service.Settle(ctx, SettleInput{UserID: userID, ChildRunID: childRun.ID, Usage: agentbroker.Budget{MaxWallSeconds: 50, MaxModelTokens: 100, MaxToolCalls: 1, MaxArtifactBytes: 512}, Outcome: "succeeded"})
	if err != nil || settled {
		t.Fatalf("settlement replay=%v err=%v", settled, err)
	}
	if _, err := service.Settle(ctx, SettleInput{UserID: userID, ChildRunID: childRun.ID, Usage: agentbroker.Budget{MaxWallSeconds: 51, MaxModelTokens: 100, MaxToolCalls: 1, MaxArtifactBytes: 512}, Outcome: "succeeded"}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("settlement mismatch error=%v", err)
	}
	queuedProposal := proposal
	queuedProposal.IdempotencyKey = "delegation-queued-cascade"
	queuedProposal.Grant.Budget = agentbroker.Budget{MaxWallSeconds: 50, MaxModelTokens: 100, MaxToolCalls: 1, MaxArtifactBytes: 512}
	queuedChild, err := service.EnqueueChild(ctx, queuedProposal)
	if err != nil || !queuedChild.Created {
		t.Fatalf("queued cascade child=%#v err=%v", queuedChild, err)
	}

	cascadeRun, err := orchestrator.GetRun(ctx, userID, cascadeChild.Authority.RunID)
	if err != nil {
		t.Fatal(err)
	}
	cascadeLease, err := orchestrator.AcquireStep(ctx, agentorchestrator.AcquireInput{UserID: userID, RunID: cascadeRun.ID, StepID: cascadeRun.Steps[0].ID, LeaseOwner: "neo-runner-cascade", LeaseDuration: 20 * time.Minute, Actor: agentorchestrator.Actor{Type: "orchestrator", ID: "delegation-test"}, ReasonCode: "DELEGATION_TEST"})
	if err != nil {
		t.Fatal(err)
	}
	cascadeTools := make([]string, 0, len(cascadeChild.Authority.Registry.Tools))
	for _, tool := range cascadeChild.Authority.Registry.Tools {
		cascadeTools = append(cascadeTools, tool.Identity)
	}
	cascadeAdmission := LaunchAdmissionInput{UserID: userID, RunID: cascadeRun.ID, AttemptID: cascadeLease.ID, Generation: cascadeLease.Generation, LeaseOwner: cascadeLease.LeaseOwner, LeaseToken: cascadeLease.Token, SnapshotFingerprint: cascadeChild.Authority.SnapshotFingerprint, GrantFingerprint: cascadeChild.Authority.GrantFingerprint, RegistryFingerprint: cascadeChild.Authority.RegistryFingerprint, RegistryTools: cascadeTools}
	cascadeTransition := agentorchestrator.TransitionInput{UserID: userID, RunID: cascadeRun.ID, StepID: cascadeLease.StepID, AttemptID: cascadeLease.ID, Generation: cascadeLease.Generation, LeaseOwner: cascadeLease.LeaseOwner, LeaseToken: cascadeLease.Token, Actor: agentorchestrator.Actor{Type: "runner", ID: cascadeLease.LeaseOwner}, ReasonCode: "DELEGATION_TEST"}
	cascadeTransition.Expected, cascadeTransition.To = agentorchestrator.AttemptLeased, agentorchestrator.AttemptStarting
	if err := orchestrator.TransitionAttempt(ctx, cascadeTransition); err != nil {
		t.Fatal(err)
	}
	if err := service.AdmitLaunch(ctx, cascadeAdmission); err != nil {
		t.Fatal(err)
	}
	cascadeTransition.Expected, cascadeTransition.To = agentorchestrator.AttemptStarting, agentorchestrator.AttemptRunning
	if err := orchestrator.TransitionAttempt(ctx, cascadeTransition); err != nil {
		t.Fatal(err)
	}
	reaper := &flakyPostgresReaper{failRemaining: 1}
	service.reaper = reaper
	targets, err := service.Cascade(ctx, CascadeInput{UserID: userID, ParentRunID: rootResult.Run.ID, Mode: "kill", ActorType: "operator", ActorID: "delegation-test", ReasonCode: "PARENT_KILLED"})
	if err != nil || len(targets) != 1 || targets[0].ChildRunID != cascadeRun.ID {
		t.Fatalf("cascade targets=%#v err=%v", targets, err)
	}
	cascaded, err := orchestrator.GetRun(ctx, userID, cascadeRun.ID)
	if err != nil || cascaded.State != agentorchestrator.RunKilled || len(cascaded.Steps) != 1 || cascaded.Steps[0].State != agentorchestrator.StepKilled || len(cascaded.Attempts) != 1 || cascaded.Attempts[0].State != agentorchestrator.AttemptKilled || cascaded.Attempts[0].LeaseExpiresAt.After(time.Now()) {
		t.Fatalf("cascaded run=%#v err=%v", cascaded, err)
	}
	cascadedAuthority, err := service.repository.GetAuthority(ctx, userID, cascadeRun.ID)
	if err != nil || cascadedAuthority.State != "killed" {
		t.Fatalf("cascaded authority=%#v err=%v", cascadedAuthority, err)
	}
	queuedCascaded, err := orchestrator.GetRun(ctx, userID, queuedChild.Authority.RunID)
	if err != nil || queuedCascaded.State != agentorchestrator.RunKilled || len(queuedCascaded.Steps) != 1 || queuedCascaded.Steps[0].State != agentorchestrator.StepKilled || len(queuedCascaded.Attempts) != 0 {
		t.Fatalf("queued cascaded run=%#v err=%v", queuedCascaded, err)
	}
	if err := service.AdmitLaunch(ctx, cascadeAdmission); !errors.Is(err, ErrSnapshotMismatch) {
		t.Fatalf("cascaded launch error=%v", err)
	}
	var reapState, reapError string
	var retryCount int
	if err := db.QueryRowContext(ctx, `SELECT state,retry_count,error_code FROM agent_delegation_reaps WHERE child_run_id=$1`, cascadeRun.ID).Scan(&reapState, &retryCount, &reapError); err != nil || reapState != "failed" || retryCount != 1 || reapError != "RUNTIME_UNAVAILABLE" {
		t.Fatalf("failed reap state=%s retries=%d code=%s err=%v", reapState, retryCount, reapError, err)
	}
	completed, err := service.Reconcile(ctx, 10)
	if err != nil || completed != 1 {
		t.Fatalf("reconcile completed=%d err=%v", completed, err)
	}
	var completedAt *time.Time
	if err := db.QueryRowContext(ctx, `SELECT state,retry_count,completed_at FROM agent_delegation_reaps WHERE child_run_id=$1`, cascadeRun.ID).Scan(&reapState, &retryCount, &completedAt); err != nil || reapState != "reaped" || retryCount != 2 || completedAt == nil {
		t.Fatalf("reaped state=%s retries=%d completed=%v err=%v", reapState, retryCount, completedAt, err)
	}

	recoveryProposal := proposal
	recoveryProposal.IdempotencyKey = "delegation-recovery"
	recoveryProposal.Grant.Budget = agentbroker.Budget{MaxWallSeconds: 50, MaxModelTokens: 100, MaxToolCalls: 1, MaxArtifactBytes: 512}
	recoveryChild, err := service.EnqueueChild(ctx, recoveryProposal)
	if err != nil || !recoveryChild.Created {
		t.Fatalf("recovery child=%#v err=%v", recoveryChild, err)
	}
	recoveryRun, err := orchestrator.GetRun(ctx, userID, recoveryChild.Authority.RunID)
	if err != nil {
		t.Fatal(err)
	}
	recoveryLease, err := orchestrator.AcquireStep(ctx, agentorchestrator.AcquireInput{UserID: userID, RunID: recoveryRun.ID, StepID: recoveryRun.Steps[0].ID, LeaseOwner: "neo-runner-recovery", LeaseDuration: 20 * time.Minute, Actor: agentorchestrator.Actor{Type: "orchestrator", ID: "delegation-test"}, ReasonCode: "DELEGATION_TEST"})
	if err != nil {
		t.Fatal(err)
	}
	recoveryTransition := agentorchestrator.TransitionInput{UserID: userID, RunID: recoveryRun.ID, StepID: recoveryLease.StepID, AttemptID: recoveryLease.ID, Generation: recoveryLease.Generation, LeaseOwner: recoveryLease.LeaseOwner, LeaseToken: recoveryLease.Token, Actor: agentorchestrator.Actor{Type: "runner", ID: recoveryLease.LeaseOwner}, ReasonCode: "DELEGATION_TEST", Expected: agentorchestrator.AttemptLeased, To: agentorchestrator.AttemptStarting}
	if err := orchestrator.TransitionAttempt(ctx, recoveryTransition); err != nil {
		t.Fatal(err)
	}
	recoveryTools := make([]string, 0, len(recoveryChild.Authority.Registry.Tools))
	for _, tool := range recoveryChild.Authority.Registry.Tools {
		recoveryTools = append(recoveryTools, tool.Identity)
	}
	recoveryAdmission := LaunchAdmissionInput{UserID: userID, RunID: recoveryRun.ID, AttemptID: recoveryLease.ID, Generation: recoveryLease.Generation, LeaseOwner: recoveryLease.LeaseOwner, LeaseToken: recoveryLease.Token, SnapshotFingerprint: recoveryChild.Authority.SnapshotFingerprint, GrantFingerprint: recoveryChild.Authority.GrantFingerprint, RegistryFingerprint: recoveryChild.Authority.RegistryFingerprint, RegistryTools: recoveryTools}
	if err := service.AdmitLaunch(ctx, recoveryAdmission); err != nil {
		t.Fatal(err)
	}
	recoveryTransition.Expected, recoveryTransition.To = agentorchestrator.AttemptStarting, agentorchestrator.AttemptRunning
	if err := orchestrator.TransitionAttempt(ctx, recoveryTransition); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE agent_attempts SET lease_expires_at=clock_timestamp() WHERE id=$1`, parentLease.ID); err != nil {
		t.Fatal(err)
	}
	completed, err = service.Reconcile(ctx, 10)
	if err != nil || completed != 1 {
		t.Fatalf("recovery reconcile completed=%d err=%v", completed, err)
	}
	recovered, err := orchestrator.GetRun(ctx, userID, recoveryRun.ID)
	if err != nil || recovered.State != agentorchestrator.RunKilled || len(recovered.Attempts) != 1 || recovered.Attempts[0].State != agentorchestrator.AttemptKilled {
		t.Fatalf("recovered run=%#v err=%v", recovered, err)
	}
	if err := service.AdmitLaunch(ctx, recoveryAdmission); !errors.Is(err, ErrSnapshotMismatch) {
		t.Fatalf("recovered launch error=%v", err)
	}
	if os.Getenv("MM_CHAT_AGENT_DELEGATION_RETAIN_FIXTURE") == "1" {
		return
	}
	_, _ = db.ExecContext(ctx, `DELETE FROM agent_runs WHERE id=$1;DELETE FROM agent_run_snapshots WHERE id=$2;DELETE FROM users WHERE id=$3`, rootResult.Run.ID, rootResult.Run.SnapshotID, userID)
}
