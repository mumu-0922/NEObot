package agentchildcanary

import (
	"testing"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentrunner"
)

func TestPlanBuildsOneToolParentAndPhysicallyEmptyChild(t *testing.T) {
	plan := validPlan()
	if err := ValidatePlan(plan); err != nil {
		t.Fatal(err)
	}
	root, rootRegistry, err := plan.RootAuthority("run_0123456789abcdef", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.Capabilities) != 1 || root.Capabilities[0].Capability != "delegate_task" ||
		len(rootRegistry.Tools) != 1 || rootRegistry.Tools[0].Identity != "delegate_task" {
		t.Fatalf("root authority = %#v %#v", root, rootRegistry)
	}
	child := plan.ChildGrantTemplate(testNow)
	child.Run = agentbroker.RunBinding{RunID: "run_fedcba9876543210", ParentRunID: root.Run.RunID, Depth: 1}
	child.GrantID = "grant_fedcba9876543210"
	childRegistry, err := agentbroker.BuildRegistry(plan.catalog(), plan.ChildRequestedTools, child, testNow)
	if err != nil || len(childRegistry.Tools) != 0 {
		t.Fatalf("child Registry = %#v, %v", childRegistry, err)
	}
}

func TestPlanRejectsWideningAndExecutableChildInputs(t *testing.T) {
	tests := map[string]func(*Plan){
		"budget":      func(plan *Plan) { plan.ChildBudget.MaxToolCalls = plan.ParentBudget.MaxToolCalls },
		"expiry":      func(plan *Plan) { plan.ChildExpirySeconds = plan.GrantWindowSeconds },
		"network":     func(plan *Plan) { plan.ChildSandbox.NetworkMode = "brokered" },
		"capability":  func(plan *Plan) { plan.ChildSandbox.Capabilities = []string{"CAP_NET_RAW"} },
		"argument":    func(plan *Plan) { plan.ChildArgv = append(plan.ChildArgv, "user-input") },
		"alias":       func(plan *Plan) { plan.ToolCatalog[0].Identity = "innocent_alias" },
		"second tool": func(plan *Plan) { plan.ChildRequestedTools = append(plan.ChildRequestedTools, "workspace_read") },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			plan := validPlan()
			mutate(&plan)
			if ValidatePlan(plan) == nil {
				t.Fatal("invalid plan accepted")
			}
		})
	}
}

var testNow = mustTime("2026-08-15T02:00:00Z")

func validPlan() Plan {
	parentBudget := agentbroker.Budget{MaxWallSeconds: 120, MaxModelTokens: 1000, MaxToolCalls: 2, MaxArtifactBytes: 8192}
	childBudget := agentbroker.Budget{MaxWallSeconds: 30, MaxModelTokens: 250, MaxToolCalls: 1, MaxArtifactBytes: 2048}
	base := agentrunner.SandboxSpec{RuntimeBundleFingerprint: fingerprint('2'), PackageFingerprint: fingerprint('1'),
		Image: "registry.invalid/neo/child-canary@sha256:" + repeat("3", 64), UID: 10001, GID: 10001,
		RootfsReadOnly: true, NoNewPrivileges: true, Capabilities: []string{},
		SeccompProfileFingerprint: fingerprint('4'), NetworkMode: "none",
		WorkspaceSnapshotID: "workspace_snapshot_0000000000000004", WorkspaceFingerprint: fingerprint('5'),
		Resources: agentrunner.ResourceLimits{CPUMillis: 250, MemoryMiB: 128, PIDs: 16,
			WallSeconds: 120, OutputBytes: 4096, ScratchBytes: 1 << 20}}
	child := base
	child.WorkspaceSnapshotID = "workspace_snapshot_0000000000000005"
	child.WorkspaceFingerprint = fingerprint('6')
	child.Resources = agentrunner.ResourceLimits{CPUMillis: 125, MemoryMiB: 64, PIDs: 8,
		WallSeconds: 30, OutputBytes: 2048, ScratchBytes: 1 << 20}
	return Plan{SchemaVersion: PlanSchemaVersion, Synthetic: true,
		UserID: "00000000-0000-4000-8000-000000000004", ProjectID: "project_00000004",
		AssistantID: "assistant_00000004", Model: Model{Provider: "fixture", ModelID: "fixture-model"},
		PackageFingerprint: fingerprint('1'), RuntimeBundleFingerprint: fingerprint('2'),
		ParentIdempotencyKey: "g21.4-child-canary-parent-template",
		ChildIdempotencyKey:  "g21.4-child-canary-child-template",
		ParentStepKind:       "child_canary_parent", ChildStepKind: "child_canary_work",
		ParentGrantID: "grant_0000000000000004", DelegationResource: "g21.4/synthetic-child",
		GrantWindowSeconds: 300, ChildExpirySeconds: 120,
		ParentBudget: parentBudget, ChildBudget: childBudget,
		ToolCatalog: []ToolDefinition{{Identity: "delegate_task", Capability: "delegate_task",
			Actions: []string{"create"}, Classification: agentbroker.ClassificationMutable}},
		ParentRequestedTools: []string{"delegate_task"}, ChildRequestedTools: []string{"delegate_task"},
		ParentSandbox: base, ChildSandbox: child,
		ParentArgv:         []string{"/opt/neo/bin/child-canary-parent", "--wait-for-cancel"},
		ChildArgv:          []string{"/opt/neo/bin/child-canary-child", "--wait-for-cancel"},
		ParentLeaseSeconds: 120, ChildLeaseSeconds: 30}
}
