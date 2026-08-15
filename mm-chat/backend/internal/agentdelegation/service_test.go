package agentdelegation

import (
	"context"
	"errors"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentorchestrator"
)

const (
	testUser    = "123e4567-e89b-42d3-a456-426614174000"
	testRoot    = "run_0123456789abcdef"
	testPkg     = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	testRuntime = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

type memoryRepository struct {
	root       Authority
	child      Authority
	derivation Derivation
	targets    []ReapTarget
	completed  map[string]bool
	recovered  int
}

func (r *memoryRepository) RegisterRoot(_ context.Context, v Authority) (Authority, bool, error) {
	r.root = v
	return v, true, nil
}
func (r *memoryRepository) GetAuthority(_ context.Context, _, runID string) (Authority, error) {
	if runID == r.root.RunID {
		return r.root, nil
	}
	return Authority{}, ErrNotFound
}
func (r *memoryRepository) ListChildren(context.Context, string, string) ([]ChildLineage, error) {
	return nil, nil
}
func (r *memoryRepository) EnqueueChild(_ context.Context, d Derivation, _ ChildProposal) (Authority, bool, error) {
	r.derivation = d
	r.child = Authority{RunID: d.ChildRunID, RootRunID: d.Parent.RootRunID, ParentRunID: d.Parent.RunID, Depth: 1, UserID: d.Parent.UserID, Subject: d.ChildGrant.Subject, Model: d.Parent.Model, Grant: d.ChildGrant, Registry: d.ChildRegistry, Budget: d.Reservation}
	return r.child, true, nil
}
func (*memoryRepository) AdmitLaunch(context.Context, LaunchAdmissionInput, string) error { return nil }
func (*memoryRepository) Settle(context.Context, SettleInput, string) (bool, error)       { return true, nil }
func (r *memoryRepository) Cascade(context.Context, CascadeInput) ([]ReapTarget, error) {
	return r.targets, nil
}
func (r *memoryRepository) Recover(context.Context, int) (int, error) {
	r.recovered++
	return r.recovered, nil
}
func (r *memoryRepository) ListPendingReaps(context.Context, int) ([]ReapTarget, error) {
	return r.targets, nil
}
func (r *memoryRepository) CompleteReap(_ context.Context, id string, ok bool, _ string) error {
	if r.completed == nil {
		r.completed = map[string]bool{}
	}
	r.completed[id] = ok
	return nil
}

type fakeReaper struct{ fail map[string]bool }

func (r *fakeReaper) Reap(_ context.Context, target ReapTarget) error {
	if r.fail[target.ReapID] {
		return errors.New("held failure")
	}
	return nil
}

func TestDeriveChildNarrowsAuthorityAndRemovesDelegation(t *testing.T) {
	now := time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC)
	repo := &memoryRepository{}
	service := NewService(repo, &fakeReaper{})
	service.now = func() time.Time { return now }
	counter := 0
	service.newID = func(prefix string) string { counter++; return prefix + "_0123456789abcde" + string(rune('a'+counter)) }
	rootGrant := grantAt(now, testRoot, 0, "")
	rootRegistry, err := agentbroker.BuildRegistry(catalog(), []string{"delegate_task", "workspace_read"}, rootGrant, now)
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := service.RegisterRoot(context.Background(), RegisterRootInput{UserID: testUser, RunID: testRoot, Grant: rootGrant, Registry: rootRegistry, Model: ModelBinding{Provider: "fixture", ModelID: "fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	root.SnapshotID = "snapshot_0123456789abcdef"
	root.SnapshotFingerprint = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	repo.root = root
	childGrant := grantAt(now, "run_fedcba9876543210", 1, testRoot)
	childGrant.Budget = agentbroker.Budget{MaxWallSeconds: 30, MaxModelTokens: 100, MaxToolCalls: 2, MaxArtifactBytes: 1024}
	childGrant.Capabilities = childGrant.Capabilities[1:]
	result, err := service.EnqueueChild(context.Background(), ChildProposal{UserID: testUser, IdempotencyKey: "child-one", ParentRunID: testRoot, ParentAttempt: ParentAttempt{StepID: "step_0123456789abcdef", AttemptID: "attempt_0123456789abcdef", Generation: 1, LeaseOwner: "runner", LeaseToken: "lease_0123456789abcdef01234567"}, Model: root.Model, Grant: childGrant, RequestedTools: []string{"delegate_task", "workspace_read"}, Catalog: catalog(), Steps: []agentorchestrator.StepPlan{{Kind: "work"}}})
	if err != nil || !result.Created || result.Authority.Depth != 1 {
		t.Fatalf("enqueue=%#v err=%v", result, err)
	}
	if len(repo.derivation.ChildRegistry.Tools) != 1 || repo.derivation.ChildRegistry.Tools[0].Identity != "workspace_read" {
		t.Fatalf("child registry=%#v", repo.derivation.ChildRegistry.Tools)
	}
}

func TestRegisterRootRejectsAuthenticatedUserDrift(t *testing.T) {
	now := time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC)
	grant := grantAt(now, testRoot, 0, "")
	registry, err := agentbroker.BuildRegistry(catalog(), []string{"workspace_read"}, grant, now)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(&memoryRepository{}, &fakeReaper{})
	service.now = func() time.Time { return now }
	_, _, err = service.RegisterRoot(context.Background(), RegisterRootInput{UserID: "77777777-7777-4777-8777-777777777777",
		RunID: testRoot, Grant: grant, Registry: registry, Model: ModelBinding{Provider: "fixture", ModelID: "fixture"}})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("cross-user root registration error=%v", err)
	}
}

func TestDeriveChildRejectsWidening(t *testing.T) {
	now := time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC)
	rootGrant := grantAt(now, testRoot, 0, "")
	rootRegistry, _ := agentbroker.BuildRegistry(catalog(), []string{"workspace_read"}, rootGrant, now)
	rootFingerprint, _ := agentbroker.GrantFingerprint(rootGrant)
	root := Authority{RunID: testRoot, RootRunID: testRoot, Depth: 0, UserID: testUser, Subject: rootGrant.Subject, Model: ModelBinding{Provider: "fixture", ModelID: "fixture"}, PackageFingerprint: testPkg, RuntimeBundleFingerprint: testRuntime, Grant: rootGrant, GrantFingerprint: rootFingerprint, Registry: rootRegistry, RegistryFingerprint: rootRegistry.Fingerprint, ExpiresAt: rootGrant.ExpiresAt, Budget: rootGrant.Budget, State: "active"}
	base := ChildProposal{UserID: testUser, ParentRunID: testRoot, Model: root.Model, Grant: grantAt(now, "run_fedcba9876543210", 1, testRoot), RequestedTools: []string{"workspace_read"}, Catalog: catalog(), Steps: []agentorchestrator.StepPlan{{Kind: "work"}}}
	base.Grant.Capabilities = base.Grant.Capabilities[1:]
	tests := map[string]func(*ChildProposal){"model": func(v *ChildProposal) { v.Model.ModelID = "wider" }, "budget": func(v *ChildProposal) { v.Grant.Budget.MaxToolCalls = root.Budget.MaxToolCalls + 1 }, "egress": func(v *ChildProposal) {
		v.Grant.Egress = agentbroker.EgressPolicy{Mode: "allowlist", Rules: []agentbroker.EgressRule{{Scheme: "https", Host: "example.com", Ports: []int{443}}}}
	}, "secret": func(v *ChildProposal) {
		v.Grant.Secrets = []agentbroker.SecretGrant{{Slot: "token", BrokerRef: "secret_ref_0123456789abcdef", Actions: []string{"read"}, TTLSeconds: 60}}
	}, "capability": func(v *ChildProposal) { v.Grant.Capabilities[0].Actions = []string{"read", "write"} }}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := base
			candidate.Grant.Capabilities = append([]agentbroker.Capability(nil), base.Grant.Capabilities...)
			mutate(&candidate)
			service := NewService(&memoryRepository{root: root}, &fakeReaper{})
			service.now = func() time.Time { return now }
			if _, err := service.deriveChild(root, candidate); !errors.Is(err, ErrSubsetViolation) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestDeriveChildRejectsRegistryRebinding(t *testing.T) {
	now := time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC)
	rootGrant := grantAt(now, testRoot, 0, "")
	rootGrant.Capabilities[1].Actions = []string{"read", "write"}
	rootRegistry, err := agentbroker.BuildRegistry(catalog(), []string{"workspace_read"}, rootGrant, now)
	if err != nil {
		t.Fatal(err)
	}
	rootFingerprint, _ := agentbroker.GrantFingerprint(rootGrant)
	root := Authority{RunID: testRoot, RootRunID: testRoot, Depth: 0, UserID: testUser,
		Subject: rootGrant.Subject, Model: ModelBinding{Provider: "fixture", ModelID: "fixture"},
		PackageFingerprint: testPkg, RuntimeBundleFingerprint: testRuntime, Grant: rootGrant,
		GrantFingerprint: rootFingerprint, Registry: rootRegistry, RegistryFingerprint: rootRegistry.Fingerprint,
		ExpiresAt: rootGrant.ExpiresAt, Budget: rootGrant.Budget, State: "active"}
	childGrant := grantAt(now, "run_fedcba9876543210", 1, testRoot)
	childGrant.Capabilities = childGrant.Capabilities[1:]
	childGrant.Capabilities[0].Actions = []string{"write"}
	proposal := ChildProposal{UserID: testUser, ParentRunID: testRoot, Model: root.Model, Grant: childGrant,
		RequestedTools: []string{"workspace_read"}, Catalog: []agentbroker.ToolDefinition{{Identity: "workspace_read",
			Capability: "workspace.read", Actions: []string{"write"}, Classification: agentbroker.ClassificationRead,
			Idempotent: true}}, Steps: []agentorchestrator.StepPlan{{Kind: "work"}}}
	service := NewService(&memoryRepository{root: root}, &fakeReaper{})
	service.now = func() time.Time { return now }
	if _, err := service.deriveChild(root, proposal); !errors.Is(err, ErrSubsetViolation) {
		t.Fatalf("registry action rebinding error=%v", err)
	}
}

func TestCascadeRetainsReapFailure(t *testing.T) {
	repo := &memoryRepository{targets: []ReapTarget{{ReapID: "reap_0123456789abcdef"}, {ReapID: "reap_fedcba9876543210"}}}
	service := NewService(repo, &fakeReaper{fail: map[string]bool{"reap_fedcba9876543210": true}})
	targets, err := service.Cascade(context.Background(), CascadeInput{UserID: testUser, ParentRunID: testRoot, Mode: "kill", ActorType: "operator", ActorID: "delegation-test", ReasonCode: "PARENT_KILLED"})
	if err != nil || len(targets) != 2 || !repo.completed[targets[0].ReapID] || repo.completed[targets[1].ReapID] {
		t.Fatalf("completed=%#v err=%v", repo.completed, err)
	}
}

func TestReconcileDiscoversParentsBeforeRetryingReaps(t *testing.T) {
	repo := &memoryRepository{targets: []ReapTarget{{ReapID: "reap_0123456789abcdef"}}}
	service := NewService(repo, &fakeReaper{})
	completed, err := service.Reconcile(context.Background(), 10)
	if err != nil || completed != 1 || repo.recovered != 1 || !repo.completed["reap_0123456789abcdef"] {
		t.Fatalf("completed=%d recovered=%d reaps=%#v err=%v", completed, repo.recovered, repo.completed, err)
	}
}

func grantAt(now time.Time, runID string, depth int, parent string) agentbroker.CapabilityGrant {
	return agentbroker.CapabilityGrant{SchemaVersion: agentbroker.GrantVersion, GrantID: "grant_0123456789abcdef", Subject: agentbroker.Subject{UserID: testUser, ProjectID: "project_01234567", AssistantID: "assistant_01234567"}, Run: agentbroker.RunBinding{RunID: runID, ParentRunID: parent, Depth: depth}, PackageFingerprint: testPkg, RuntimeBundleFingerprint: testRuntime, IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), Capabilities: []agentbroker.Capability{{Capability: "delegate_task", Actions: []string{"create"}, Resources: agentbroker.Selector{Kind: "exact", Values: []string{"child"}}, Approval: agentbroker.ApprovalAutomatic, MaxCalls: 3}, {Capability: "workspace.read", Actions: []string{"read"}, Resources: agentbroker.Selector{Kind: "prefix", Values: []string{"project/"}}, Approval: agentbroker.ApprovalAutomatic, MaxCalls: 10}}, Egress: agentbroker.EgressPolicy{Mode: "none", Rules: []agentbroker.EgressRule{}}, Secrets: []agentbroker.SecretGrant{}, Budget: agentbroker.Budget{MaxWallSeconds: 300, MaxModelTokens: 1000, MaxToolCalls: 10, MaxArtifactBytes: 4096}}
}
func catalog() []agentbroker.ToolDefinition {
	return []agentbroker.ToolDefinition{{Identity: "delegate_task", Capability: "delegate_task", Actions: []string{"create"}, Classification: agentbroker.ClassificationMutable}, {Identity: "workspace_read", Capability: "workspace.read", Actions: []string{"read"}, Classification: agentbroker.ClassificationRead, Idempotent: true}}
}
