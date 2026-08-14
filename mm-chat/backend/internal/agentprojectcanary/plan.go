// Package agentprojectcanary composes the G21.3 synthetic Project mutation
// plan with the durable Broker authority. It is not imported by API or Chat.
package agentprojectcanary

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentorchestrator"
	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

const (
	PlanSchemaVersion     = "neo.agent-project-mutation-canary-plan/v1"
	SnapshotSchemaVersion = "neo.agent-project-canary-snapshot/v1"
	ProjectCanaryIdentity = "spiffe://neo-chat/agent-runtime-project-canary"
	ActionProjectMutation = "project_mutation"
	maximumPlanBytes      = 128 << 10
	maximumMutationBytes  = 4096
)

var (
	ErrInvalidPlan      = errors.New("PROJECT_CANARY_PLAN_INVALID")
	planUUID            = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	planFingerprint     = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	planRequestIdentity = regexp.MustCompile(`^g21\.3-request-[a-z0-9][a-z0-9._-]{7,79}$`)
	planIdempotencyKey  = regexp.MustCompile(`^g21\.3-project-mutation-[a-z0-9][a-z0-9._-]{7,79}$`)
	projectResource     = regexp.MustCompile(`^project-canary/[a-z0-9][a-z0-9._-]{0,63}$`)
	projectRelativePath = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
)

type ProjectPolicy struct {
	Resource     string `json:"resource"`
	Path         string `json:"path"`
	BaseRevision string `json:"baseRevision"`
	Content      string `json:"content"`
	MaxBytes     int64  `json:"maxBytes"`
}

type ActionPlan struct {
	ID              string          `json:"id"`
	RequestIdentity string          `json:"requestIdentity"`
	IdempotencyKey  string          `json:"idempotencyKey"`
	ToolIdentity    string          `json:"toolIdentity"`
	Capability      string          `json:"capability"`
	Action          string          `json:"action"`
	Resource        string          `json:"resource"`
	Classification  string          `json:"classification"`
	Idempotent      bool            `json:"idempotent"`
	Approval        string          `json:"approval"`
	Arguments       json.RawMessage `json:"arguments"`
	BaseRevision    string          `json:"baseRevision"`
	TTLSeconds      int             `json:"ttlSeconds"`
	Project         ProjectPolicy   `json:"project"`
}

type Plan struct {
	SchemaVersion            string                  `json:"schemaVersion"`
	Synthetic                bool                    `json:"synthetic"`
	UserID                   string                  `json:"userId"`
	ProjectID                string                  `json:"projectId"`
	AssistantID              string                  `json:"assistantId"`
	PackageFingerprint       string                  `json:"packageFingerprint"`
	RuntimeBundleFingerprint string                  `json:"runtimeBundleFingerprint"`
	TargetFingerprint        string                  `json:"targetFingerprint"`
	IssuedAt                 time.Time               `json:"issuedAt"`
	ExpiresAt                time.Time               `json:"expiresAt"`
	Sandbox                  agentrunner.SandboxSpec `json:"sandbox"`
	Argv                     []string                `json:"argv"`
	LeaseSeconds             int                     `json:"leaseSeconds"`
	Action                   ActionPlan              `json:"action"`
}

type Snapshot struct {
	SchemaVersion       string `json:"schemaVersion"`
	ActionID            string `json:"actionId"`
	RequestIdentity     string `json:"requestIdentity"`
	IdempotencyKey      string `json:"idempotencyKey"`
	GrantFingerprint    string `json:"grantFingerprint"`
	RegistryFingerprint string `json:"registryFingerprint"`
	ProjectPolicy       struct {
		Resource string `json:"resource"`
		Path     string `json:"path"`
		MaxBytes int64  `json:"maxBytes"`
	} `json:"projectPolicy"`
}

type Binding struct {
	Plan                 ActionPlan
	RunID                string
	StepID               string
	Grant                agentbroker.CapabilityGrant
	Registry             agentbroker.ToolRegistry
	Snapshot             json.RawMessage
	SnapshotFingerprint  string
	ArgumentsFingerprint string
	ContentFingerprint   string
	MutationFingerprint  string
}

type Bindings struct {
	plan     Plan
	runnerID string
	action   Binding
}

func LoadPlan(path, runnerID string, now time.Time) (Plan, *Bindings, string, error) {
	raw, err := readPrivatePlan(path)
	if err != nil {
		return Plan{}, nil, "", ErrInvalidPlan
	}
	var plan Plan
	if strictjson.Decode(raw, maximumPlanBytes, &plan) != nil {
		return Plan{}, nil, "", ErrInvalidPlan
	}
	bindings, err := NewBindings(plan, runnerID, now)
	if err != nil {
		return Plan{}, nil, "", err
	}
	return plan, bindings, rawFingerprint(raw), nil
}

func NewBindings(plan Plan, runnerID string, now time.Time) (*Bindings, error) {
	if validatePlanShape(plan, runnerID, now.UTC()) != nil {
		return nil, ErrInvalidPlan
	}
	action := plan.Action
	runID := stablePlanID("run", plan.UserID, action.ID, action.IdempotencyKey)
	stepID := stablePlanID("step", plan.UserID, action.ID, action.IdempotencyKey)
	grant := agentbroker.CapabilityGrant{SchemaVersion: agentbroker.GrantVersion,
		GrantID: stablePlanID("grant", plan.UserID, action.ID, action.IdempotencyKey),
		Subject: agentbroker.Subject{UserID: plan.UserID, ProjectID: plan.ProjectID,
			AssistantID: plan.AssistantID}, Run: agentbroker.RunBinding{RunID: runID, Depth: 0},
		PackageFingerprint: plan.PackageFingerprint, RuntimeBundleFingerprint: plan.RuntimeBundleFingerprint,
		IssuedAt: plan.IssuedAt.UTC(), ExpiresAt: plan.ExpiresAt.UTC(),
		Capabilities: []agentbroker.Capability{{Capability: action.Capability,
			Actions: []string{action.Action}, Resources: agentbroker.Selector{Kind: "exact",
				Values: []string{action.Resource}}, Approval: action.Approval, MaxCalls: 1}},
		Egress:  agentbroker.EgressPolicy{Mode: "none", Rules: []agentbroker.EgressRule{}},
		Secrets: []agentbroker.SecretGrant{}, Budget: agentbroker.Budget{
			MaxWallSeconds: int(plan.Sandbox.Resources.WallSeconds), MaxModelTokens: 0,
			MaxToolCalls: 1, MaxArtifactBytes: 0}}
	grantFingerprint, err := agentbroker.GrantFingerprint(grant)
	if err != nil {
		return nil, ErrInvalidPlan
	}
	registry, err := agentbroker.BuildRegistry([]agentbroker.ToolDefinition{{
		Identity: action.ToolIdentity, Capability: action.Capability, Actions: []string{action.Action},
		Classification: action.Classification, Idempotent: action.Idempotent}},
		[]string{action.ToolIdentity}, grant, now.UTC())
	if err != nil {
		return nil, ErrInvalidPlan
	}
	snapshotValue := Snapshot{SchemaVersion: SnapshotSchemaVersion, ActionID: action.ID,
		RequestIdentity: action.RequestIdentity, IdempotencyKey: action.IdempotencyKey,
		GrantFingerprint: grantFingerprint, RegistryFingerprint: registry.Fingerprint}
	snapshotValue.ProjectPolicy.Resource = action.Project.Resource
	snapshotValue.ProjectPolicy.Path = action.Project.Path
	snapshotValue.ProjectPolicy.MaxBytes = action.Project.MaxBytes
	snapshot, err := json.Marshal(snapshotValue)
	if err != nil {
		return nil, ErrInvalidPlan
	}
	snapshotFingerprint, err := agentorchestrator.SnapshotFingerprint(snapshot)
	if err != nil {
		return nil, ErrInvalidPlan
	}
	argumentsFingerprint, err := agentbroker.ArgumentsFingerprint(action.Arguments)
	if err != nil {
		return nil, ErrInvalidPlan
	}
	contentFingerprint := domainFingerprint("neo-project-file-v1", []byte(action.Project.Content))
	mutationFingerprint := agentbroker.ProjectMutationFingerprint(action.BaseRevision, action.Resource,
		action.Project.Path, contentFingerprint)
	binding := Binding{Plan: action, RunID: runID, StepID: stepID, Grant: grant, Registry: registry,
		Snapshot: snapshot, SnapshotFingerprint: snapshotFingerprint, ArgumentsFingerprint: argumentsFingerprint,
		ContentFingerprint: contentFingerprint, MutationFingerprint: mutationFingerprint}
	launch := agentrunner.LaunchRequest{Attempt: agentrunner.AttemptRef{RunID: runID, StepID: stepID,
		AttemptID:       stablePlanID("attempt", plan.UserID, action.ID, action.IdempotencyKey),
		LeaseGeneration: 1, LeaseOwner: runnerID, LeaseToken: "lease_0123456789abcdefghijklmnopqrstuv"},
		Lineage: agentrunner.RunLineage{RootRunID: runID, Depth: 0}, GrantID: grant.GrantID,
		GrantFingerprint: grantFingerprint, SnapshotFingerprint: snapshotFingerprint,
		Sandbox: plan.Sandbox, ToolRegistry: agentrunner.ToolRegistry{Depth: 0,
			Tools: []string{action.ToolIdentity}, RegistryFingerprint: registry.Fingerprint},
		Argv: append([]string(nil), plan.Argv...)}
	if _, err := agentrunner.NewRequest(agentrunner.MethodLaunch, launch, now.UTC()); err != nil {
		return nil, ErrInvalidPlan
	}
	return &Bindings{plan: plan, runnerID: runnerID, action: binding}, nil
}

func (bindings *Bindings) Action() (Binding, bool) {
	if bindings == nil {
		return Binding{}, false
	}
	return bindings.action, true
}

func (bindings *Bindings) ResolvePrepare(request agentrunner.PrepareRequest, now time.Time) (PrepareBinding, error) {
	binding, ok := bindings.bindingForAttempt(request.Attempt)
	if !ok || !now.UTC().Before(binding.Grant.ExpiresAt) ||
		request.Authority.CallerIdentity != ProjectCanaryIdentity ||
		request.Authority.Method != agentrunner.MethodPrepare ||
		request.SnapshotFingerprint != binding.SnapshotFingerprint || request.GrantID != binding.Grant.GrantID ||
		request.GrantFingerprint != binding.GrantFingerprint() ||
		request.RegistryFingerprint != binding.Registry.Fingerprint ||
		request.ToolIdentity != binding.Plan.ToolIdentity || request.Capability != binding.Plan.Capability ||
		request.Action != binding.Plan.Action || request.Resource != binding.Plan.Resource ||
		request.ArgumentsFingerprint != binding.ArgumentsFingerprint ||
		request.TTLSeconds != binding.Plan.TTLSeconds || request.BaseRevision != binding.Plan.BaseRevision {
		return PrepareBinding{}, agentbroker.ErrSnapshotMismatch
	}
	actualArguments, err := agentbroker.ArgumentsFingerprint(request.Arguments)
	if err != nil || actualArguments != binding.ArgumentsFingerprint {
		return PrepareBinding{}, agentbroker.ErrSnapshotMismatch
	}
	return PrepareBinding{UserID: bindings.plan.UserID, Grant: binding.Grant, Registry: binding.Registry}, nil
}

func (bindings *Bindings) ResolveCommit(request agentrunner.CommitRequest, now time.Time) (string, error) {
	binding, ok := bindings.bindingForAttempt(request.Attempt)
	if !ok || !now.UTC().Before(binding.Grant.ExpiresAt) ||
		request.Authority.CallerIdentity != ProjectCanaryIdentity || request.Authority.Method != agentrunner.MethodCommit ||
		request.SnapshotFingerprint != binding.SnapshotFingerprint ||
		request.GrantFingerprint != binding.GrantFingerprint() || request.RegistryFingerprint != binding.Registry.Fingerprint {
		return "", agentbroker.ErrSnapshotMismatch
	}
	return bindings.plan.UserID, nil
}

func (bindings *Bindings) bindingForAttempt(attempt agentrunner.AttemptRef) (Binding, bool) {
	if bindings == nil || attempt.LeaseOwner != bindings.runnerID || attempt.RunID != bindings.action.RunID ||
		attempt.StepID != bindings.action.StepID {
		return Binding{}, false
	}
	return bindings.action, true
}

func (binding Binding) GrantFingerprint() string {
	fingerprint, _ := agentbroker.GrantFingerprint(binding.Grant)
	return fingerprint
}

func (binding Binding) MutationAuthority() agentbroker.ProjectMutationAuthority {
	return agentbroker.ProjectMutationAuthority{Resource: binding.Plan.Resource,
		BaseRevision: binding.Plan.BaseRevision, Path: binding.Plan.Project.Path,
		Content: []byte(binding.Plan.Project.Content), ContentFingerprint: binding.ContentFingerprint,
		MutationFingerprint: binding.MutationFingerprint}
}

func validatePlanShape(plan Plan, runnerID string, now time.Time) error {
	if plan.SchemaVersion != PlanSchemaVersion || !plan.Synthetic || !planUUID.MatchString(plan.UserID) ||
		!strings.HasPrefix(plan.ProjectID, "project_") || !strings.HasPrefix(plan.AssistantID, "assistant_") ||
		!planFingerprint.MatchString(plan.PackageFingerprint) ||
		!planFingerprint.MatchString(plan.RuntimeBundleFingerprint) || !planFingerprint.MatchString(plan.TargetFingerprint) ||
		plan.IssuedAt.IsZero() || plan.ExpiresAt.IsZero() || plan.IssuedAt.After(now) || !now.Before(plan.ExpiresAt) ||
		!plan.ExpiresAt.After(plan.IssuedAt) || strings.TrimSpace(runnerID) == "" || len(runnerID) > 128 ||
		plan.LeaseSeconds < 10 || plan.LeaseSeconds > 60 || len(plan.Argv) == 0 ||
		!strings.HasPrefix(filepath.Clean(plan.Argv[0]), "/opt/neo/bin/") ||
		plan.Sandbox.NetworkMode != "none" || !plan.Sandbox.RootfsReadOnly || !plan.Sandbox.NoNewPrivileges ||
		len(plan.Sandbox.Capabilities) != 0 || plan.Sandbox.PackageFingerprint != plan.PackageFingerprint ||
		plan.Sandbox.RuntimeBundleFingerprint != plan.RuntimeBundleFingerprint || validateActionShape(plan.Action) != nil {
		return ErrInvalidPlan
	}
	return nil
}

func validateActionShape(action ActionPlan) error {
	content := []byte(action.Project.Content)
	if action.ID != ActionProjectMutation || !planRequestIdentity.MatchString(action.RequestIdentity) ||
		!planIdempotencyKey.MatchString(action.IdempotencyKey) || action.ToolIdentity != "project.patch" ||
		action.Capability != "project.write" || action.Action != "apply_patch" ||
		action.Classification != agentbroker.ClassificationMutable || action.Idempotent ||
		action.Approval != agentbroker.ApprovalPerCommit || !projectResource.MatchString(action.Resource) ||
		action.Resource != action.Project.Resource || !planFingerprint.MatchString(action.BaseRevision) ||
		action.BaseRevision != action.Project.BaseRevision || !projectRelativePath.MatchString(action.Project.Path) ||
		len(content) < 1 || len(content) > maximumMutationBytes || !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 ||
		action.Project.MaxBytes < int64(len(content)) || action.Project.MaxBytes > maximumMutationBytes ||
		action.TTLSeconds < 60 || action.TTLSeconds > 3600 {
		return ErrInvalidPlan
	}
	contentFingerprint := domainFingerprint("neo-project-file-v1", content)
	mutationFingerprint := agentbroker.ProjectMutationFingerprint(action.BaseRevision, action.Resource,
		action.Project.Path, contentFingerprint)
	expected, err := json.Marshal(map[string]any{"contentFingerprint": contentFingerprint,
		"mutationFingerprint": mutationFingerprint, "path": action.Project.Path, "sizeBytes": len(content)})
	if err != nil {
		return ErrInvalidPlan
	}
	actualFingerprint, actualErr := agentbroker.ArgumentsFingerprint(action.Arguments)
	expectedFingerprint, expectedErr := agentbroker.ArgumentsFingerprint(expected)
	if actualErr != nil || expectedErr != nil || actualFingerprint != expectedFingerprint {
		return ErrInvalidPlan
	}
	return nil
}

func stablePlanID(prefix string, values ...string) string {
	digest := sha256.Sum256([]byte("neo-agent-project-canary-" + prefix + "\x00" + strings.Join(values, "\x00")))
	return prefix + "_" + hex.EncodeToString(digest[:16])
}

func readPrivatePlan(path string) ([]byte, error) {
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
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 ||
		info.Size() < 2 || info.Size() > maximumPlanBytes {
		return nil, ErrInvalidPlan
	}
	raw, err := io.ReadAll(io.LimitReader(file, maximumPlanBytes+1))
	if err != nil || int64(len(raw)) != info.Size() || len(raw) > maximumPlanBytes {
		return nil, ErrInvalidPlan
	}
	return raw, nil
}

func domainFingerprint(domain string, payload []byte) string {
	digest := sha256.Sum256(append(append([]byte(nil), domain...), append([]byte{0}, payload...)...))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func rawFingerprint(payload []byte) string {
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:])
}
