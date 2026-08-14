package agentrootcanary

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentorchestrator"
	"neo-chat/mm-chat/backend/internal/agentrunner"
)

const (
	testUserID    = "10000000-0000-4000-8000-000000000001"
	testRunID     = "run_0123456789abcdef"
	testStepID    = "step_0123456789abcdef"
	testAttemptID = "attempt_0123456789abcdef"
)

func TestServiceExecutesOneSignedRootCanaryAndReplaysTerminal(t *testing.T) {
	plan := testPlan()
	orchestrator := &fakeOrchestrator{run: agentorchestrator.Run{ID: testRunID,
		UserID: testUserID, SnapshotFingerprint: fp('9'), State: agentorchestrator.RunQueued,
		Steps: []agentorchestrator.Step{{ID: testStepID, RunID: testRunID, State: agentorchestrator.StepReady}}}}
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	_ = publicKey
	authority := &fakeAuthority{privateKey: privateKey}
	runnerState := &fakeRunnerState{}
	client := &fakeRunnerClient{runnerID: "neo-runner-primary", probeFingerprint: fp('8')}
	terminal := &fakeTerminal{}
	service, err := NewService(Config{RunnerID: "neo-runner-primary",
		CallerIdentity: "spiffe://neo-chat/agent-runtime-root-canary", PollInterval: time.Second,
		AuthorityTTL: 10 * time.Second, BatchSize: 100}, plan, fakeGate{}, orchestrator,
		authority, runnerState, client, terminal)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC) }
	result, err := service.Cycle(context.Background())
	if err != nil || !result.Completed || result.AttemptID != testAttemptID || !terminal.called {
		t.Fatalf("Cycle() = %#v, %v terminal=%v", result, err, terminal.called)
	}
	for _, method := range []string{agentrunner.MethodLaunch, agentrunner.MethodHeartbeat, agentrunner.MethodCancel} {
		if client.calls[method] != 1 || !client.signed[method] {
			t.Fatalf("method %s calls=%d signed=%v", method, client.calls[method], client.signed[method])
		}
	}
	if authority.completed != 3 || len(runnerState.transitions) != 2 {
		t.Fatalf("completed=%d transitions=%v", authority.completed, runnerState.transitions)
	}

	orchestrator.run.State = agentorchestrator.RunCanceled
	launches := client.calls[agentrunner.MethodLaunch]
	result, err = service.Cycle(context.Background())
	if err != nil || !result.Completed || client.calls[agentrunner.MethodLaunch] != launches {
		t.Fatalf("terminal replay = %#v, %v launches=%d", result, err, client.calls[agentrunner.MethodLaunch])
	}
}

func TestServiceFencesLiveTokenlessAttemptUntilExpiry(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	orchestrator := &fakeOrchestrator{run: agentorchestrator.Run{ID: testRunID,
		UserID: testUserID, SnapshotFingerprint: fp('9'), State: agentorchestrator.RunRunning,
		Steps: []agentorchestrator.Step{{ID: testStepID, RunID: testRunID, State: agentorchestrator.StepRunning}},
		Attempts: []agentorchestrator.Attempt{{ID: testAttemptID, State: agentorchestrator.AttemptRunning,
			Generation: 1, LeaseExpiresAt: now.Add(time.Minute)}}}}
	_, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	service, err := NewService(Config{RunnerID: "neo-runner-primary",
		CallerIdentity: "spiffe://neo-chat/agent-runtime-root-canary", PollInterval: time.Second,
		AuthorityTTL: 10 * time.Second, BatchSize: 100}, testPlan(), fakeGate{}, orchestrator,
		&fakeAuthority{privateKey: privateKey}, &fakeRunnerState{},
		&fakeRunnerClient{runnerID: "neo-runner-primary", probeFingerprint: fp('8')}, &fakeTerminal{})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	result, err := service.Cycle(context.Background())
	if !errors.Is(err, ErrRecoveryPending) || result.Generation != 1 || orchestrator.acquired != 0 {
		t.Fatalf("Cycle() = %#v, %v acquired=%d", result, err, orchestrator.acquired)
	}
}

func TestServiceReconcilesExpiredTokenlessAttemptAndReclaimsNextGeneration(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	newAttemptID := "attempt_fedcba9876543210"
	orchestrator := &fakeOrchestrator{run: agentorchestrator.Run{ID: testRunID,
		UserID: testUserID, SnapshotFingerprint: fp('9'), State: agentorchestrator.RunRunning,
		Steps: []agentorchestrator.Step{{ID: testStepID, RunID: testRunID,
			State: agentorchestrator.StepRunning, CurrentGeneration: 1}},
		Attempts: []agentorchestrator.Attempt{{ID: testAttemptID, State: agentorchestrator.AttemptRunning,
			Generation: 1, LeaseExpiresAt: now.Add(-time.Second)}}},
		leaseAttemptID: newAttemptID, leaseGeneration: 2}
	_, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	runnerState := &fakeRunnerState{recoveries: []agentrunner.RecoverySandbox{{
		ExpectedSandbox: agentrunner.ExpectedSandbox{SandboxID: "sandbox_oldcanary0001",
			RunnerID: "neo-runner-primary", ProbeFingerprint: fp('8')},
		LeaseExpired: true, LeaseExpiresAt: now.Add(-time.Second),
	}}}
	client := &fakeRunnerClient{runnerID: "neo-runner-primary", probeFingerprint: fp('8')}
	service, err := NewService(Config{RunnerID: "neo-runner-primary",
		CallerIdentity: "spiffe://neo-chat/agent-runtime-root-canary", PollInterval: time.Second,
		AuthorityTTL: 10 * time.Second, BatchSize: 100}, testPlan(), fakeGate{}, orchestrator,
		&fakeAuthority{privateKey: privateKey}, runnerState, client, &fakeTerminal{})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	result, err := service.Cycle(context.Background())
	if err != nil || !result.Completed || result.AttemptID != newAttemptID || result.Generation != 2 {
		t.Fatalf("expired reclaim = %#v, %v", result, err)
	}
	if orchestrator.acquired != 1 || client.calls[agentrunner.MethodReconcile] != 2 ||
		client.launchAttempt.AttemptID != newAttemptID || client.launchAttempt.LeaseGeneration != 2 {
		t.Fatalf("reclaim acquired=%d calls=%v launch=%#v",
			orchestrator.acquired, client.calls, client.launchAttempt)
	}
}

func TestServiceCancelsAndTerminalizesAfterRunnerHeartbeatFailure(t *testing.T) {
	orchestrator := rootCanaryOrchestratorFixture()
	_, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	client := &fakeRunnerClient{runnerID: "neo-runner-primary", probeFingerprint: fp('8'),
		errors: map[string]error{agentrunner.MethodHeartbeat: ErrUnavailable}}
	terminal := &fakeTerminal{}
	service, err := NewService(Config{RunnerID: "neo-runner-primary",
		CallerIdentity: "spiffe://neo-chat/agent-runtime-root-canary", PollInterval: time.Second,
		AuthorityTTL: 10 * time.Second, BatchSize: 100}, testPlan(), fakeGate{}, orchestrator,
		&fakeAuthority{privateKey: privateKey}, &fakeRunnerState{}, client, terminal)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC) }
	if _, err := service.Cycle(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("heartbeat failure = %v", err)
	}
	if !terminal.called || client.calls[agentrunner.MethodHeartbeat] != 1 ||
		client.calls[agentrunner.MethodCancel] != 1 || !client.signed[agentrunner.MethodCancel] {
		t.Fatalf("cleanup terminal=%v calls=%v signed=%v", terminal.called, client.calls, client.signed)
	}
}

func TestServiceCancelsAndTerminalizesAfterPostgresHeartbeatFailure(t *testing.T) {
	orchestrator := rootCanaryOrchestratorFixture()
	orchestrator.heartbeatErr = ErrUnavailable
	_, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	client := &fakeRunnerClient{runnerID: "neo-runner-primary", probeFingerprint: fp('8')}
	terminal := &fakeTerminal{}
	service, err := NewService(Config{RunnerID: "neo-runner-primary",
		CallerIdentity: "spiffe://neo-chat/agent-runtime-root-canary", PollInterval: time.Second,
		AuthorityTTL: 10 * time.Second, BatchSize: 100}, testPlan(), fakeGate{}, orchestrator,
		&fakeAuthority{privateKey: privateKey}, &fakeRunnerState{}, client, terminal)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC) }
	if _, err := service.Cycle(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("PostgreSQL heartbeat failure = %v", err)
	}
	if !terminal.called || client.calls[agentrunner.MethodHeartbeat] != 1 ||
		client.calls[agentrunner.MethodCancel] != 1 {
		t.Fatalf("cleanup terminal=%v calls=%v", terminal.called, client.calls)
	}
}

func TestLoadPlanRejectsUnknownFieldAndWritableFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "plan.json")
	raw, _ := json.Marshal(testPlan())
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPlan(path); err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	_ = json.Unmarshal(raw, &value)
	value["prompt"] = "untrusted"
	drifted, _ := json.Marshal(value)
	_ = os.WriteFile(path, drifted, 0o600)
	if _, err := LoadPlan(path); !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("unknown field error = %v", err)
	}
	_ = os.WriteFile(path, raw, 0o600)
	_ = os.Chmod(path, 0o666)
	if _, err := LoadPlan(path); !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("writable file error = %v", err)
	}
}

type fakeGate struct{}

func (fakeGate) Verify(time.Time) error { return nil }

type fakeOrchestrator struct {
	run             agentorchestrator.Run
	acquired        int
	heartbeatErr    error
	leaseAttemptID  string
	leaseGeneration int64
}

func (fake *fakeOrchestrator) EnqueueRun(context.Context, agentorchestrator.EnqueueInput) (agentorchestrator.EnqueueResult, error) {
	return agentorchestrator.EnqueueResult{Run: fake.run, Created: fake.run.State == agentorchestrator.RunQueued}, nil
}
func (fake *fakeOrchestrator) AcquireStep(context.Context, agentorchestrator.AcquireInput) (agentrunnerLease agentorchestrator.Lease, err error) {
	fake.acquired++
	attemptID, generation := fake.leaseAttemptID, fake.leaseGeneration
	if attemptID == "" {
		attemptID = testAttemptID
	}
	if generation == 0 {
		generation = 1
	}
	return agentorchestrator.Lease{Attempt: agentorchestrator.Attempt{ID: attemptID, RunID: testRunID,
		StepID: testStepID, Generation: generation, State: agentorchestrator.AttemptLeased,
		LeaseOwner: "neo-runner-primary", LeaseExpiresAt: time.Now().Add(time.Minute)},
		Token: "lease_0123456789abcdefghijklmnopqrstuv"}, nil
}
func (*fakeOrchestrator) TransitionAttempt(context.Context, agentorchestrator.TransitionInput) error {
	return nil
}
func (fake *fakeOrchestrator) HeartbeatAttempt(context.Context, agentorchestrator.HeartbeatInput) (time.Time, error) {
	return time.Now().Add(time.Minute), fake.heartbeatErr
}

type fakeAuthority struct {
	privateKey ed25519.PrivateKey
	completed  int
}

func (fake *fakeAuthority) IssueAuthority(_ context.Context, input agentrunner.IssueAuthorityInput) (agentrunner.AuthorityTicket, json.RawMessage, error) {
	claims := agentrunner.NewAuthorityClaims(input.CallerIdentity, input.RunnerID, input.Method,
		input.RequestID, input.Nonce, input.RequestFingerprint, input.SnapshotFingerprint,
		input.Attempt, 0, time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 14, 10, 0, 10, 0, time.UTC))
	ticket, err := agentrunner.SignAuthority(fake.privateKey, claims)
	return ticket, nil, err
}
func (fake *fakeAuthority) CompleteRequest(context.Context, string, string, string, string, []byte) error {
	fake.completed++
	return nil
}

type fakeRunnerState struct {
	transitions []string
	recoveries  []agentrunner.RecoverySandbox
}

func (*fakeRunnerState) ExpectSandbox(context.Context, agentrunner.ExpectedSandbox) error { return nil }
func (fake *fakeRunnerState) UpdateSandbox(_ context.Context, _, _ string, _ int64, expected, to, _ string) error {
	fake.transitions = append(fake.transitions, expected+"->"+to)
	return nil
}
func (fake *fakeRunnerState) RecoverySandboxes(context.Context, int) ([]agentrunner.RecoverySandbox, error) {
	return fake.recoveries, nil
}

type fakeRunnerClient struct {
	runnerID, probeFingerprint string
	calls                      map[string]int
	signed                     map[string]bool
	errors                     map[string]error
	launchAttempt              agentrunner.AttemptIdentity
}

func (fake *fakeRunnerClient) Call(_ context.Context, request agentrunner.Request) (agentrunner.Response, error) {
	if fake.calls == nil {
		fake.calls = map[string]int{}
		fake.signed = map[string]bool{}
	}
	fake.calls[request.Method]++
	response := agentrunner.Response{SchemaVersion: agentrunner.ProtocolVersion,
		Method: request.Method + ".result", RequestID: request.RequestID,
		SentAt: request.SentAt, Nonce: request.Nonce}
	switch request.Method {
	case agentrunner.MethodProbe:
		response.Body = agentrunner.ProbeResult{Ready: true, Runtime: "podman", RuntimeVersion: "test",
			Features: requiredFeatures, ProbeFingerprint: fake.probeFingerprint}
	case agentrunner.MethodReconcile:
		response.Body = agentrunner.ReconcileResult{RunnerID: fake.runnerID}
	case agentrunner.MethodList:
		response.Body = agentrunner.ListResult{RunnerID: fake.runnerID, Sandboxes: []agentrunner.SandboxDescriptor{}}
	case agentrunner.MethodLaunch:
		fake.signed[request.Method] = request.Launch.Authority.Signature != ""
		fake.launchAttempt = request.Launch.Attempt.Identity()
		response.Body = agentrunner.LaunchResult{Accepted: true, Attempt: request.Launch.Attempt.Identity(),
			SandboxID: "sandbox_0123456789abcdef", StartedAt: &request.SentAt}
	case agentrunner.MethodHeartbeat:
		fake.signed[request.Method] = request.Heartbeat.Authority.Signature != ""
		expires := request.SentAt.Add(30 * time.Second)
		response.Body = agentrunner.HeartbeatResult{Accepted: true, LeaseExpiresAt: &expires}
	case agentrunner.MethodCancel:
		fake.signed[request.Method] = request.Cancel.Authority.Signature != ""
		response.Body = agentrunner.CancelResult{Accepted: true, ObservedTerminal: "canceled"}
	}
	return response, fake.errors[request.Method]
}

type fakeTerminal struct{ called bool }

func (fake *fakeTerminal) FinalizeCanceled(context.Context, TerminalInput) error {
	fake.called = true
	return nil
}

func testPlan() Plan {
	sandbox := agentrunner.SandboxSpec{RuntimeBundleFingerprint: fp('1'), PackageFingerprint: fp('2'),
		Image: "registry.example/neo/root-canary@" + fp('3'), UID: 10001, GID: 10001,
		RootfsReadOnly: true, NoNewPrivileges: true, Capabilities: []string{},
		SeccompProfileFingerprint: fp('4'), NetworkMode: "none",
		WorkspaceSnapshotID: "workspace_snapshot_0123456789abcdef", WorkspaceFingerprint: fp('5'),
		Resources: agentrunner.ResourceLimits{CPUMillis: 250, MemoryMiB: 128, PIDs: 16,
			WallSeconds: 30, OutputBytes: 4096, ScratchBytes: 1 << 20}}
	registry := agentrunner.ToolRegistry{Depth: 0, Tools: []string{}, RegistryFingerprint: fp('6')}
	grant := fp('7')
	return Plan{SchemaVersion: PlanSchemaVersion, Synthetic: true, UserID: testUserID,
		IdempotencyKey: "g21.1-root-canary-fixture", StepKind: "root_canary",
		GrantID: "grant_0123456789abcdef", GrantFingerprint: grant,
		Snapshot: Snapshot{SchemaVersion: "neo.agent-snapshot/v1", Mode: "synthetic",
			PackageFingerprint: sandbox.PackageFingerprint, RuntimeBundleFingerprint: sandbox.RuntimeBundleFingerprint,
			GrantFingerprint: grant, RegistryFingerprint: registry.RegistryFingerprint,
			WorkspaceFingerprint: sandbox.WorkspaceFingerprint, NoEgress: true, NoSecrets: true, MaxWallSeconds: 30},
		Sandbox: sandbox, ToolRegistry: registry, Argv: []string{"/opt/neo/bin/root-canary", "--wait-for-cancel"}, LeaseSeconds: 30}
}

func rootCanaryOrchestratorFixture() *fakeOrchestrator {
	return &fakeOrchestrator{run: agentorchestrator.Run{ID: testRunID,
		UserID: testUserID, SnapshotFingerprint: fp('9'), State: agentorchestrator.RunQueued,
		Steps: []agentorchestrator.Step{{ID: testStepID, RunID: testRunID, State: agentorchestrator.StepReady}}}}
}

func fp(character byte) string { return "sha256:" + strings.Repeat(string(character), 64) }
