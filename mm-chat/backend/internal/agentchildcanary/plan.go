// Package agentchildcanary composes the durable Orchestrator, depth-one
// delegation authority and credential-free Runner for the G21.4 synthetic
// Parent/Child canary.
package agentchildcanary

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentdelegation"
	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

const PlanSchemaVersion = "neo.agent-child-run-canary-plan/v1"

var (
	ErrInvalidPlan     = errors.New("CHILD_CANARY_PLAN_INVALID")
	ErrUnavailable     = errors.New("CHILD_CANARY_UNAVAILABLE")
	ErrRecoveryPending = errors.New("CHILD_CANARY_RECOVERY_PENDING")
	uuidPattern        = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	idPattern          = regexp.MustCompile(`^(?:grant|project|assistant)_[a-z0-9]{8,64}$`)
	fingerprintPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	identifierPattern  = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)
)

type Model struct {
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`
}

type ToolDefinition struct {
	Identity       string   `json:"identity"`
	Capability     string   `json:"capability"`
	Actions        []string `json:"actions"`
	Classification string   `json:"classification"`
	Idempotent     bool     `json:"idempotent"`
}

type Plan struct {
	SchemaVersion            string                  `json:"schemaVersion"`
	Synthetic                bool                    `json:"synthetic"`
	UserID                   string                  `json:"userId"`
	ProjectID                string                  `json:"projectId"`
	AssistantID              string                  `json:"assistantId"`
	Model                    Model                   `json:"model"`
	PackageFingerprint       string                  `json:"packageFingerprint"`
	RuntimeBundleFingerprint string                  `json:"runtimeBundleFingerprint"`
	ParentIdempotencyKey     string                  `json:"parentIdempotencyKey"`
	ChildIdempotencyKey      string                  `json:"childIdempotencyKey"`
	ParentStepKind           string                  `json:"parentStepKind"`
	ChildStepKind            string                  `json:"childStepKind"`
	ParentGrantID            string                  `json:"parentGrantId"`
	DelegationResource       string                  `json:"delegationResource"`
	GrantWindowSeconds       int                     `json:"grantWindowSeconds"`
	ChildExpirySeconds       int                     `json:"childExpirySeconds"`
	ParentBudget             agentbroker.Budget      `json:"parentBudget"`
	ChildBudget              agentbroker.Budget      `json:"childBudget"`
	ToolCatalog              []ToolDefinition        `json:"toolCatalog"`
	ParentRequestedTools     []string                `json:"parentRequestedTools"`
	ChildRequestedTools      []string                `json:"childRequestedTools"`
	ParentSandbox            agentrunner.SandboxSpec `json:"parentSandbox"`
	ChildSandbox             agentrunner.SandboxSpec `json:"childSandbox"`
	ParentArgv               []string                `json:"parentArgv"`
	ChildArgv                []string                `json:"childArgv"`
	ParentLeaseSeconds       int                     `json:"parentLeaseSeconds"`
	ChildLeaseSeconds        int                     `json:"childLeaseSeconds"`
}

func LoadPlan(path string) (Plan, error) {
	raw, err := readPrivate(path)
	if err != nil {
		return Plan{}, ErrInvalidPlan
	}
	var plan Plan
	if strictjson.Decode(raw, 96<<10, &plan) != nil || ValidatePlan(plan) != nil {
		return Plan{}, ErrInvalidPlan
	}
	return plan, nil
}

func ValidatePlan(plan Plan) error {
	if plan.SchemaVersion != PlanSchemaVersion || !plan.Synthetic ||
		!uuidPattern.MatchString(plan.UserID) || !idPattern.MatchString(plan.ProjectID) ||
		!idPattern.MatchString(plan.AssistantID) || !validModel(plan.Model) ||
		!fingerprintPattern.MatchString(plan.PackageFingerprint) ||
		!fingerprintPattern.MatchString(plan.RuntimeBundleFingerprint) ||
		!strings.HasPrefix(plan.ParentIdempotencyKey, "g21.4-child-canary-parent-") ||
		!strings.HasPrefix(plan.ChildIdempotencyKey, "g21.4-child-canary-child-") ||
		plan.ParentIdempotencyKey == plan.ChildIdempotencyKey ||
		len(plan.ParentIdempotencyKey) > 128 || len(plan.ChildIdempotencyKey) > 128 ||
		plan.ParentStepKind != "child_canary_parent" || plan.ChildStepKind != "child_canary_work" ||
		!idPattern.MatchString(plan.ParentGrantID) || plan.DelegationResource != "g21.4/synthetic-child" ||
		plan.GrantWindowSeconds < 60 || plan.GrantWindowSeconds > 600 ||
		plan.ChildExpirySeconds < 30 || plan.ChildExpirySeconds >= plan.GrantWindowSeconds ||
		plan.ParentLeaseSeconds < 30 || plan.ParentLeaseSeconds > 300 ||
		plan.ChildLeaseSeconds < 15 || plan.ChildLeaseSeconds >= plan.ParentLeaseSeconds ||
		!strictBudgetSubset(plan.ChildBudget, plan.ParentBudget) ||
		!exactDelegationCatalog(plan.ToolCatalog) ||
		!exactTools(plan.ParentRequestedTools) || !exactTools(plan.ChildRequestedTools) ||
		!validSandbox(plan.ParentSandbox, plan.PackageFingerprint, plan.RuntimeBundleFingerprint, plan.ParentBudget) ||
		!validSandbox(plan.ChildSandbox, plan.PackageFingerprint, plan.RuntimeBundleFingerprint, plan.ChildBudget) ||
		!sandboxSubset(plan.ChildSandbox, plan.ParentSandbox) ||
		!exactArgv(plan.ParentArgv, "/opt/neo/bin/child-canary-parent") ||
		!exactArgv(plan.ChildArgv, "/opt/neo/bin/child-canary-child") {
		return ErrInvalidPlan
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	root, registry, err := plan.RootAuthority("run_0123456789abcdef", now)
	if err != nil || len(registry.Tools) != 1 || registry.Tools[0].Identity != "delegate_task" {
		return ErrInvalidPlan
	}
	child := plan.ChildGrantTemplate(now)
	child.Run = agentbroker.RunBinding{RunID: "run_fedcba9876543210", ParentRunID: root.Run.RunID, Depth: 1}
	child.GrantID = "grant_fedcba9876543210"
	childRegistry, err := agentbroker.BuildRegistry(plan.catalog(), plan.ChildRequestedTools, child, now)
	if err != nil || len(childRegistry.Tools) != 0 {
		return ErrInvalidPlan
	}
	return validateLaunchShape(plan, root, registry, child, childRegistry, now)
}

func (plan Plan) RootAuthority(runID string, now time.Time) (agentbroker.CapabilityGrant, agentbroker.ToolRegistry, error) {
	issuedAt := now.UTC().Add(-time.Second)
	grant := agentbroker.CapabilityGrant{
		SchemaVersion: agentbroker.GrantVersion,
		GrantID:       plan.ParentGrantID,
		Subject: agentbroker.Subject{UserID: plan.UserID, ProjectID: plan.ProjectID,
			AssistantID: plan.AssistantID},
		Run:                      agentbroker.RunBinding{RunID: runID, Depth: 0},
		PackageFingerprint:       plan.PackageFingerprint,
		RuntimeBundleFingerprint: plan.RuntimeBundleFingerprint,
		IssuedAt:                 issuedAt,
		ExpiresAt:                now.UTC().Add(time.Duration(plan.GrantWindowSeconds) * time.Second),
		Capabilities: []agentbroker.Capability{{Capability: "delegate_task", Actions: []string{"create"},
			Resources: agentbroker.Selector{Kind: "exact", Values: []string{plan.DelegationResource}},
			Approval:  agentbroker.ApprovalAutomatic, MaxCalls: 1}},
		Egress:  agentbroker.EgressPolicy{Mode: "none", Rules: []agentbroker.EgressRule{}},
		Secrets: []agentbroker.SecretGrant{}, Budget: plan.ParentBudget,
	}
	registry, err := agentbroker.BuildRegistry(plan.catalog(), plan.ParentRequestedTools, grant, now.UTC())
	return grant, registry, err
}

func (plan Plan) ChildGrantTemplate(now time.Time) agentbroker.CapabilityGrant {
	return agentbroker.CapabilityGrant{
		SchemaVersion: agentbroker.GrantVersion,
		GrantID:       "grant_childtemplate",
		Subject: agentbroker.Subject{UserID: plan.UserID, ProjectID: plan.ProjectID,
			AssistantID: plan.AssistantID},
		Run:                      agentbroker.RunBinding{RunID: "run_childtemplate", ParentRunID: "run_parenttemplate", Depth: 1},
		PackageFingerprint:       plan.PackageFingerprint,
		RuntimeBundleFingerprint: plan.RuntimeBundleFingerprint,
		IssuedAt:                 now.UTC().Add(-time.Second),
		ExpiresAt:                now.UTC().Add(time.Duration(plan.ChildExpirySeconds) * time.Second),
		Capabilities:             []agentbroker.Capability{},
		Egress:                   agentbroker.EgressPolicy{Mode: "none", Rules: []agentbroker.EgressRule{}},
		Secrets:                  []agentbroker.SecretGrant{}, Budget: plan.ChildBudget,
	}
}

func (plan Plan) catalog() []agentbroker.ToolDefinition {
	return []agentbroker.ToolDefinition{{Identity: "delegate_task", Capability: "delegate_task",
		Actions: []string{"create"}, Classification: agentbroker.ClassificationMutable}}
}

func validateLaunchShape(plan Plan, root agentbroker.CapabilityGrant, rootRegistry agentbroker.ToolRegistry,
	child agentbroker.CapabilityGrant, childRegistry agentbroker.ToolRegistry, now time.Time,
) error {
	rootGrantFingerprint, err := agentbroker.GrantFingerprint(root)
	if err != nil {
		return err
	}
	childGrantFingerprint, err := agentbroker.GrantFingerprint(child)
	if err != nil {
		return err
	}
	for _, launch := range []agentrunner.LaunchRequest{
		{Attempt: dummyAttempt(root.Run.RunID), Lineage: agentrunner.RunLineage{RootRunID: root.Run.RunID, Depth: 0},
			GrantID: root.GrantID, GrantFingerprint: rootGrantFingerprint,
			SnapshotFingerprint: "sha256:" + strings.Repeat("1", 64), Sandbox: plan.ParentSandbox,
			ToolRegistry: runnerRegistry(rootRegistry), Argv: append([]string(nil), plan.ParentArgv...)},
		{Attempt: dummyAttempt(child.Run.RunID), Lineage: agentrunner.RunLineage{RootRunID: root.Run.RunID,
			ParentRunID: root.Run.RunID, Depth: 1}, GrantID: child.GrantID,
			GrantFingerprint: childGrantFingerprint, SnapshotFingerprint: "sha256:" + strings.Repeat("2", 64),
			Sandbox: plan.ChildSandbox, ToolRegistry: runnerRegistry(childRegistry),
			Argv: append([]string(nil), plan.ChildArgv...)},
	} {
		if _, err := agentrunner.NewRequest(agentrunner.MethodLaunch, launch, now); err != nil {
			return err
		}
	}
	return nil
}

func runnerRegistry(registry agentbroker.ToolRegistry) agentrunner.ToolRegistry {
	tools := make([]string, 0, len(registry.Tools))
	for _, tool := range registry.Tools {
		tools = append(tools, tool.Identity)
	}
	return agentrunner.ToolRegistry{Depth: registry.Depth, Tools: tools, RegistryFingerprint: registry.Fingerprint}
}

func dummyAttempt(runID string) agentrunner.AttemptRef {
	return agentrunner.AttemptRef{RunID: runID, StepID: "step_0123456789abcdef",
		AttemptID: "attempt_0123456789abcdef", LeaseGeneration: 1,
		LeaseOwner: "neo-runner-primary", LeaseToken: "lease_0123456789abcdefghijklmnopqrstuv"}
}

func strictBudgetSubset(child, parent agentbroker.Budget) bool {
	return child.MaxWallSeconds > 0 && child.MaxWallSeconds < parent.MaxWallSeconds &&
		child.MaxModelTokens > 0 && child.MaxModelTokens < parent.MaxModelTokens &&
		child.MaxToolCalls > 0 && child.MaxToolCalls < parent.MaxToolCalls &&
		child.MaxArtifactBytes > 0 && child.MaxArtifactBytes < parent.MaxArtifactBytes
}

func validSandbox(value agentrunner.SandboxSpec, packageFingerprint, runtimeFingerprint string,
	budget agentbroker.Budget,
) bool {
	return value.PackageFingerprint == packageFingerprint && value.RuntimeBundleFingerprint == runtimeFingerprint &&
		strings.Contains(value.Image, "@sha256:") && value.UID >= 10000 && value.GID >= 10000 &&
		value.RootfsReadOnly && value.NoNewPrivileges && len(value.Capabilities) == 0 &&
		value.NetworkMode == "none" && fingerprintPattern.MatchString(value.SeccompProfileFingerprint) &&
		strings.HasPrefix(value.WorkspaceSnapshotID, "workspace_snapshot_") &&
		fingerprintPattern.MatchString(value.WorkspaceFingerprint) &&
		value.Resources.CPUMillis >= 50 && value.Resources.CPUMillis <= 1000 &&
		value.Resources.MemoryMiB >= 32 && value.Resources.MemoryMiB <= 512 &&
		value.Resources.PIDs >= 4 && value.Resources.PIDs <= 32 &&
		value.Resources.WallSeconds == int64(budget.MaxWallSeconds) &&
		value.Resources.OutputBytes >= 1024 && value.Resources.OutputBytes <= 65536 &&
		value.Resources.ScratchBytes >= 4096 && value.Resources.ScratchBytes <= 16<<20
}

func sandboxSubset(child, parent agentrunner.SandboxSpec) bool {
	return child.Resources.CPUMillis <= parent.Resources.CPUMillis &&
		child.Resources.MemoryMiB <= parent.Resources.MemoryMiB &&
		child.Resources.PIDs <= parent.Resources.PIDs &&
		child.Resources.WallSeconds < parent.Resources.WallSeconds &&
		child.Resources.OutputBytes <= parent.Resources.OutputBytes &&
		child.Resources.ScratchBytes <= parent.Resources.ScratchBytes &&
		child.WorkspaceSnapshotID != parent.WorkspaceSnapshotID
}

func exactDelegationCatalog(catalog []ToolDefinition) bool {
	return len(catalog) == 1 && catalog[0].Identity == "delegate_task" &&
		catalog[0].Capability == "delegate_task" && len(catalog[0].Actions) == 1 &&
		catalog[0].Actions[0] == "create" && catalog[0].Classification == agentbroker.ClassificationMutable &&
		!catalog[0].Idempotent
}

func exactTools(tools []string) bool { return len(tools) == 1 && tools[0] == "delegate_task" }

func exactArgv(argv []string, binary string) bool {
	return len(argv) == 2 && argv[0] == binary && argv[1] == "--wait-for-cancel"
}

func validModel(model Model) bool {
	return len(model.Provider) <= 64 && len(model.ModelID) <= 128 &&
		identifierPattern.MatchString(model.Provider) && identifierPattern.MatchString(model.ModelID)
}

func readPrivate(path string) ([]byte, error) {
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) {
		return nil, ErrInvalidPlan
	}
	descriptor, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(descriptor), path)
	if file == nil {
		_ = unix.Close(descriptor)
		return nil, ErrInvalidPlan
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() < 2 || info.Size() > 96<<10 {
		return nil, ErrInvalidPlan
	}
	raw, err := io.ReadAll(io.LimitReader(file, (96<<10)+1))
	if err != nil || int64(len(raw)) != info.Size() || len(raw) > 96<<10 {
		return nil, ErrInvalidPlan
	}
	return raw, nil
}

func MarshalPlan(plan Plan) ([]byte, error) { return json.Marshal(plan) }

func DelegationModel(model Model) agentdelegation.ModelBinding {
	return agentdelegation.ModelBinding{Provider: model.Provider, ModelID: model.ModelID}
}
