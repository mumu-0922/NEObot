package agentbrokercanary

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentorchestrator"
	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/mcpclient"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

const (
	PlanSchemaVersion     = "neo.agent-broker-artifact-canary-plan/v1"
	SnapshotSchemaVersion = "neo.agent-broker-canary-snapshot/v1"
	BrokerCanaryIdentity  = "spiffe://neo-chat/agent-runtime-broker-canary"

	ActionProjectRead    = "project_read"
	ActionWorkspaceRead  = "workspace_read"
	ActionMCPRead        = "mcp_read"
	ActionArtifact       = "artifact_publish"
	ActionPossibleSend   = "possible_send"
	maximumPlanBytes     = 128 << 10
	maximumArtifactBytes = 32 << 20
)

var (
	ErrInvalidPlan  = errors.New("BROKER_CANARY_PLAN_INVALID")
	planUUID        = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	planFingerprint = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

type FileReadPlan struct {
	Root                string `json:"root"`
	RelativePath        string `json:"relativePath"`
	ExpectedBytes       int64  `json:"expectedBytes"`
	ExpectedFingerprint string `json:"expectedFingerprint"`
	MaxBytes            int64  `json:"maxBytes"`
}

type MCPReadPlan struct {
	ServerID       string            `json:"serverId"`
	ServerName     string            `json:"serverName"`
	ToolName       string            `json:"toolName"`
	CommandArgv    []string          `json:"commandArgv"`
	ToolPolicy     map[string]string `json:"toolPolicy"`
	MaxResultBytes int64             `json:"maxResultBytes"`
}

type ArtifactPlan struct {
	Name        string `json:"name"`
	MediaType   string `json:"mediaType"`
	SizeBytes   int64  `json:"sizeBytes"`
	Fingerprint string `json:"fingerprint"`
}

type ActionPlan struct {
	ID             string          `json:"id"`
	IdempotencyKey string          `json:"idempotencyKey"`
	ToolIdentity   string          `json:"toolIdentity"`
	Capability     string          `json:"capability"`
	Action         string          `json:"action"`
	Resource       string          `json:"resource"`
	Classification string          `json:"classification"`
	Idempotent     bool            `json:"idempotent"`
	Approval       string          `json:"approval"`
	Arguments      json.RawMessage `json:"arguments"`
	BaseRevision   string          `json:"baseRevision,omitempty"`
	TTLSeconds     int             `json:"ttlSeconds"`
	File           *FileReadPlan   `json:"file,omitempty"`
	MCP            *MCPReadPlan    `json:"mcp,omitempty"`
	Artifact       *ArtifactPlan   `json:"artifact,omitempty"`
}

type ArtifactPolicy struct {
	MaxBytes          int64    `json:"maxBytes"`
	AllowedMediaTypes []string `json:"allowedMediaTypes"`
}

type Plan struct {
	SchemaVersion            string                  `json:"schemaVersion"`
	Synthetic                bool                    `json:"synthetic"`
	UserID                   string                  `json:"userId"`
	ProjectID                string                  `json:"projectId"`
	AssistantID              string                  `json:"assistantId"`
	PackageFingerprint       string                  `json:"packageFingerprint"`
	RuntimeBundleFingerprint string                  `json:"runtimeBundleFingerprint"`
	IssuedAt                 time.Time               `json:"issuedAt"`
	ExpiresAt                time.Time               `json:"expiresAt"`
	ArtifactPolicy           ArtifactPolicy          `json:"artifactPolicy"`
	Sandbox                  agentrunner.SandboxSpec `json:"sandbox"`
	Argv                     []string                `json:"argv"`
	LeaseSeconds             int                     `json:"leaseSeconds"`
	Actions                  []ActionPlan            `json:"actions"`
}

type Snapshot struct {
	SchemaVersion       string         `json:"schemaVersion"`
	ActionID            string         `json:"actionId"`
	GrantFingerprint    string         `json:"grantFingerprint"`
	RegistryFingerprint string         `json:"registryFingerprint"`
	ArtifactPolicy      ArtifactPolicy `json:"artifactPolicy"`
}

type ActionBinding struct {
	Plan                 ActionPlan
	RunID                string
	StepID               string
	Grant                agentbroker.CapabilityGrant
	Registry             agentbroker.ToolRegistry
	Snapshot             json.RawMessage
	SnapshotFingerprint  string
	ArgumentsFingerprint string
}

type Bindings struct {
	plan     Plan
	runnerID string
	byRun    map[string]ActionBinding
	byAction map[string]ActionBinding
}

func LoadPlan(path, runnerID string, now time.Time) (Plan, *Bindings, error) {
	raw, err := readPrivatePlan(path)
	if err != nil {
		return Plan{}, nil, ErrInvalidPlan
	}
	var plan Plan
	if strictjson.Decode(raw, maximumPlanBytes, &plan) != nil {
		return Plan{}, nil, ErrInvalidPlan
	}
	bindings, err := NewBindings(plan, runnerID, now)
	if err != nil {
		return Plan{}, nil, err
	}
	return plan, bindings, nil
}

func NewBindings(plan Plan, runnerID string, now time.Time) (*Bindings, error) {
	if validatePlanShape(plan, runnerID, now.UTC()) != nil {
		return nil, ErrInvalidPlan
	}
	result := &Bindings{plan: plan, runnerID: runnerID,
		byRun:    make(map[string]ActionBinding, len(plan.Actions)),
		byAction: make(map[string]ActionBinding, len(plan.Actions))}
	for _, action := range plan.Actions {
		runID := stablePlanID("run", plan.UserID, action.ID, action.IdempotencyKey)
		stepID := stablePlanID("step", plan.UserID, action.ID, action.IdempotencyKey)
		grant := agentbroker.CapabilityGrant{SchemaVersion: agentbroker.GrantVersion,
			GrantID: stablePlanID("grant", plan.UserID, action.ID, action.IdempotencyKey),
			Subject: agentbroker.Subject{UserID: plan.UserID, ProjectID: plan.ProjectID,
				AssistantID: plan.AssistantID}, Run: agentbroker.RunBinding{RunID: runID, Depth: 0},
			PackageFingerprint:       plan.PackageFingerprint,
			RuntimeBundleFingerprint: plan.RuntimeBundleFingerprint,
			IssuedAt:                 plan.IssuedAt.UTC(), ExpiresAt: plan.ExpiresAt.UTC(),
			Capabilities: []agentbroker.Capability{{Capability: action.Capability,
				Actions: []string{action.Action}, Resources: agentbroker.Selector{Kind: "exact",
					Values: []string{action.Resource}}, Approval: action.Approval, MaxCalls: 1}},
			Egress:  agentbroker.EgressPolicy{Mode: "none", Rules: []agentbroker.EgressRule{}},
			Secrets: []agentbroker.SecretGrant{}, Budget: agentbroker.Budget{
				MaxWallSeconds: int(plan.Sandbox.Resources.WallSeconds), MaxModelTokens: 0,
				MaxToolCalls: 1, MaxArtifactBytes: plan.ArtifactPolicy.MaxBytes}}
		grantFingerprint, err := agentbroker.GrantFingerprint(grant)
		if err != nil {
			return nil, ErrInvalidPlan
		}
		registry, err := agentbroker.BuildRegistry([]agentbroker.ToolDefinition{{
			Identity: action.ToolIdentity, Capability: action.Capability,
			Actions: []string{action.Action}, Classification: action.Classification,
			Idempotent: action.Idempotent}}, []string{action.ToolIdentity}, grant, now.UTC())
		if err != nil {
			return nil, ErrInvalidPlan
		}
		snapshot, err := json.Marshal(Snapshot{SchemaVersion: SnapshotSchemaVersion,
			ActionID: action.ID, GrantFingerprint: grantFingerprint,
			RegistryFingerprint: registry.Fingerprint, ArtifactPolicy: plan.ArtifactPolicy})
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
		binding := ActionBinding{Plan: action, RunID: runID, StepID: stepID, Grant: grant,
			Registry: registry, Snapshot: snapshot, SnapshotFingerprint: snapshotFingerprint,
			ArgumentsFingerprint: argumentsFingerprint}
		launch := agentrunner.LaunchRequest{Attempt: agentrunner.AttemptRef{RunID: runID, StepID: stepID,
			AttemptID:       stablePlanID("attempt", plan.UserID, action.ID, action.IdempotencyKey),
			LeaseGeneration: 1, LeaseOwner: runnerID,
			LeaseToken: "lease_0123456789abcdefghijklmnopqrstuv"},
			Lineage: agentrunner.RunLineage{RootRunID: runID, Depth: 0}, GrantID: grant.GrantID,
			GrantFingerprint: grantFingerprint, SnapshotFingerprint: snapshotFingerprint,
			Sandbox: plan.Sandbox, ToolRegistry: agentrunner.ToolRegistry{Depth: 0,
				Tools: []string{action.ToolIdentity}, RegistryFingerprint: registry.Fingerprint},
			Argv: append([]string(nil), plan.Argv...)}
		if _, err := agentrunner.NewRequest(agentrunner.MethodLaunch, launch, now.UTC()); err != nil {
			return nil, ErrInvalidPlan
		}
		if _, duplicate := result.byRun[runID]; duplicate {
			return nil, ErrInvalidPlan
		}
		result.byRun[runID], result.byAction[action.ID] = binding, binding
	}
	return result, nil
}

func (bindings *Bindings) Action(id string) (ActionBinding, bool) {
	if bindings == nil {
		return ActionBinding{}, false
	}
	value, ok := bindings.byAction[id]
	return value, ok
}

func (bindings *Bindings) All() []ActionBinding {
	if bindings == nil {
		return nil
	}
	result := make([]ActionBinding, 0, len(bindings.byAction))
	for _, action := range bindings.plan.Actions {
		result = append(result, bindings.byAction[action.ID])
	}
	return result
}

func (bindings *Bindings) ResolvePrepare(request agentrunner.PrepareRequest, now time.Time) (PrepareBinding, error) {
	binding, ok := bindings.bindingForAttempt(request.Attempt)
	if !ok || !now.UTC().Before(binding.Grant.ExpiresAt) ||
		request.Authority.CallerIdentity != BrokerCanaryIdentity ||
		request.Authority.Method != agentrunner.MethodPrepare ||
		request.SnapshotFingerprint != binding.SnapshotFingerprint ||
		request.GrantID != binding.Grant.GrantID ||
		request.GrantFingerprint != binding.SnapshotGrantFingerprint() ||
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
	return PrepareBinding{UserID: bindings.plan.UserID, Grant: binding.Grant,
		Registry: binding.Registry}, nil
}

func (bindings *Bindings) ResolveCommit(request agentrunner.CommitRequest, now time.Time) (string, error) {
	binding, ok := bindings.bindingForAttempt(request.Attempt)
	if !ok || !now.UTC().Before(binding.Grant.ExpiresAt) ||
		request.Authority.CallerIdentity != BrokerCanaryIdentity ||
		request.Authority.Method != agentrunner.MethodCommit ||
		request.SnapshotFingerprint != binding.SnapshotFingerprint ||
		request.GrantFingerprint != binding.SnapshotGrantFingerprint() ||
		request.RegistryFingerprint != binding.Registry.Fingerprint {
		return "", agentbroker.ErrSnapshotMismatch
	}
	return bindings.plan.UserID, nil
}

func (bindings *Bindings) bindingForAttempt(attempt agentrunner.AttemptRef) (ActionBinding, bool) {
	if bindings == nil || attempt.LeaseOwner != bindings.runnerID {
		return ActionBinding{}, false
	}
	binding, ok := bindings.byRun[attempt.RunID]
	return binding, ok && attempt.StepID == binding.StepID
}

func (binding ActionBinding) SnapshotGrantFingerprint() string {
	fingerprint, _ := agentbroker.GrantFingerprint(binding.Grant)
	return fingerprint
}

func validatePlanShape(plan Plan, runnerID string, now time.Time) error {
	if plan.SchemaVersion != PlanSchemaVersion || !plan.Synthetic || !planUUID.MatchString(plan.UserID) ||
		!strings.HasPrefix(plan.ProjectID, "project_") || !strings.HasPrefix(plan.AssistantID, "assistant_") ||
		!planFingerprint.MatchString(plan.PackageFingerprint) ||
		!planFingerprint.MatchString(plan.RuntimeBundleFingerprint) ||
		plan.IssuedAt.IsZero() || plan.ExpiresAt.IsZero() || plan.IssuedAt.After(now) || !now.Before(plan.ExpiresAt) ||
		!plan.ExpiresAt.After(plan.IssuedAt) || strings.TrimSpace(runnerID) == "" || len(runnerID) > 128 ||
		plan.ArtifactPolicy.MaxBytes < 1 || plan.ArtifactPolicy.MaxBytes > maximumArtifactBytes ||
		len(plan.ArtifactPolicy.AllowedMediaTypes) == 0 || len(plan.ArtifactPolicy.AllowedMediaTypes) > 8 ||
		plan.LeaseSeconds < 10 || plan.LeaseSeconds > 60 || len(plan.Argv) == 0 ||
		!strings.HasPrefix(filepath.Clean(plan.Argv[0]), "/opt/neo/bin/") ||
		plan.Sandbox.NetworkMode != "none" || !plan.Sandbox.RootfsReadOnly || !plan.Sandbox.NoNewPrivileges ||
		len(plan.Sandbox.Capabilities) != 0 || plan.Sandbox.PackageFingerprint != plan.PackageFingerprint ||
		plan.Sandbox.RuntimeBundleFingerprint != plan.RuntimeBundleFingerprint || len(plan.Actions) != 5 {
		return ErrInvalidPlan
	}
	media := append([]string(nil), plan.ArtifactPolicy.AllowedMediaTypes...)
	sort.Strings(media)
	for index, value := range media {
		if value == "" || index > 0 && media[index-1] == value {
			return ErrInvalidPlan
		}
	}
	required := map[string]bool{ActionProjectRead: false, ActionWorkspaceRead: false,
		ActionMCPRead: false, ActionArtifact: false, ActionPossibleSend: false}
	for _, action := range plan.Actions {
		if _, ok := required[action.ID]; !ok || required[action.ID] ||
			!strings.HasPrefix(action.IdempotencyKey, "g21.2-broker-canary-") || len(action.IdempotencyKey) > 128 ||
			action.Resource == "" || len(action.Resource) > 256 || len(action.Arguments) == 0 ||
			action.TTLSeconds < 60 || action.TTLSeconds > 3600 || validateActionShape(action, plan) != nil {
			return ErrInvalidPlan
		}
		required[action.ID] = true
	}
	return nil
}

func validateActionShape(action ActionPlan, plan Plan) error {
	expected := map[string]struct {
		identity, capability, operation, classification, approval string
		idempotent                                                bool
	}{
		ActionProjectRead:   {"project.read", "project.read", "read_file", agentbroker.ClassificationRead, agentbroker.ApprovalAutomatic, true},
		ActionWorkspaceRead: {"workspace.read", "workspace.read", "read_file", agentbroker.ClassificationRead, agentbroker.ApprovalAutomatic, true},
		ActionMCPRead:       {"mcp.read", "mcp.read", "call", agentbroker.ClassificationRead, agentbroker.ApprovalAutomatic, true},
		ActionArtifact:      {"artifact.publish", "artifact.publish", "publish", agentbroker.ClassificationMutable, agentbroker.ApprovalAutomatic, false},
		ActionPossibleSend:  {"canary.possible_send", "mcp.read", "call", agentbroker.ClassificationRead, agentbroker.ApprovalAutomatic, true},
	}[action.ID]
	if action.ToolIdentity != expected.identity || action.Capability != expected.capability ||
		action.Action != expected.operation || action.Classification != expected.classification ||
		action.Approval != expected.approval || action.Idempotent != expected.idempotent {
		return ErrInvalidPlan
	}
	switch action.ID {
	case ActionProjectRead, ActionWorkspaceRead:
		if action.File == nil || action.MCP != nil || action.Artifact != nil || validateFilePlan(*action.File) != nil {
			return ErrInvalidPlan
		}
	case ActionMCPRead:
		if action.File != nil || action.MCP == nil || action.Artifact != nil || validateMCPPlan(*action.MCP) != nil {
			return ErrInvalidPlan
		}
	case ActionArtifact:
		if action.File != nil || action.MCP != nil || action.Artifact == nil ||
			action.Resource != action.Artifact.Name || action.Artifact.SizeBytes < 0 ||
			action.Artifact.SizeBytes > plan.ArtifactPolicy.MaxBytes ||
			!planFingerprint.MatchString(action.Artifact.Fingerprint) ||
			!containsString(plan.ArtifactPolicy.AllowedMediaTypes, action.Artifact.MediaType) {
			return ErrInvalidPlan
		}
	case ActionPossibleSend:
		if action.File != nil || action.MCP != nil || action.Artifact != nil {
			return ErrInvalidPlan
		}
	}
	return nil
}

func validateFilePlan(plan FileReadPlan) error {
	clean := filepath.Clean(plan.RelativePath)
	returnErr := !filepath.IsAbs(plan.Root) || clean == "." || filepath.IsAbs(clean) ||
		clean != plan.RelativePath || strings.HasPrefix(clean, "../") ||
		plan.ExpectedBytes < 0 || plan.MaxBytes < 1 || plan.MaxBytes > 8<<20 ||
		plan.ExpectedBytes > plan.MaxBytes || !planFingerprint.MatchString(plan.ExpectedFingerprint)
	if returnErr {
		return ErrInvalidPlan
	}
	return nil
}

func validateMCPPlan(plan MCPReadPlan) error {
	if plan.ServerID == "" || plan.ServerName == "" || plan.ToolName == "" ||
		len(plan.CommandArgv) == 0 || !strings.HasPrefix(filepath.Clean(plan.CommandArgv[0]), "/opt/mcp/") ||
		len(plan.ToolPolicy) != 1 || plan.ToolPolicy[plan.ToolName] != mcpclient.ClassificationRead ||
		plan.MaxResultBytes < 1 || plan.MaxResultBytes > 1<<20 {
		return ErrInvalidPlan
	}
	return nil
}

func stablePlanID(prefix string, values ...string) string {
	digest := sha256.Sum256([]byte("neo-agent-broker-canary-" + prefix + "\x00" + strings.Join(values, "\x00")))
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

func containsString(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
