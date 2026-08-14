// Package agentrootcanary composes the durable Orchestrator and rootless
// Runner for the separately activated G21.1 synthetic Root Run canary.
package agentrootcanary

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/strictjson"

	"golang.org/x/sys/unix"
)

const PlanSchemaVersion = "neo.agent-root-run-canary-plan/v1"

var (
	ErrInvalidPlan     = errors.New("ROOT_CANARY_PLAN_INVALID")
	ErrUnavailable     = errors.New("ROOT_CANARY_UNAVAILABLE")
	ErrRecoveryPending = errors.New("ROOT_CANARY_RECOVERY_PENDING")
	uuidPattern        = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

type Snapshot struct {
	SchemaVersion            string `json:"schemaVersion"`
	Mode                     string `json:"mode"`
	PackageFingerprint       string `json:"packageFingerprint"`
	RuntimeBundleFingerprint string `json:"runtimeBundleFingerprint"`
	GrantFingerprint         string `json:"grantFingerprint"`
	RegistryFingerprint      string `json:"registryFingerprint"`
	WorkspaceFingerprint     string `json:"workspaceFingerprint"`
	NoEgress                 bool   `json:"noEgress"`
	NoSecrets                bool   `json:"noSecrets"`
	MaxWallSeconds           int64  `json:"maxWallSeconds"`
}

type Plan struct {
	SchemaVersion    string                   `json:"schemaVersion"`
	Synthetic        bool                     `json:"synthetic"`
	UserID           string                   `json:"userId"`
	IdempotencyKey   string                   `json:"idempotencyKey"`
	StepKind         string                   `json:"stepKind"`
	GrantID          string                   `json:"grantId"`
	GrantFingerprint string                   `json:"grantFingerprint"`
	Snapshot         Snapshot                 `json:"snapshot"`
	Sandbox          agentrunner.SandboxSpec  `json:"sandbox"`
	ToolRegistry     agentrunner.ToolRegistry `json:"toolRegistry"`
	Argv             []string                 `json:"argv"`
	LeaseSeconds     int                      `json:"leaseSeconds"`
}

func LoadPlan(path string) (Plan, error) {
	raw, err := readPrivate(path)
	if err != nil {
		return Plan{}, ErrInvalidPlan
	}
	var plan Plan
	if strictjson.Decode(raw, 64<<10, &plan) != nil || ValidatePlan(plan) != nil {
		return Plan{}, ErrInvalidPlan
	}
	return plan, nil
}

func ValidatePlan(plan Plan) error {
	if plan.SchemaVersion != PlanSchemaVersion || !plan.Synthetic || !uuidPattern.MatchString(plan.UserID) ||
		!strings.HasPrefix(plan.IdempotencyKey, "g21.1-root-canary-") || len(plan.IdempotencyKey) > 128 ||
		plan.StepKind != "root_canary" || plan.LeaseSeconds < 10 || plan.LeaseSeconds > 60 ||
		plan.Snapshot.SchemaVersion != "neo.agent-snapshot/v1" || plan.Snapshot.Mode != "synthetic" ||
		!plan.Snapshot.NoEgress || !plan.Snapshot.NoSecrets || plan.Snapshot.MaxWallSeconds != plan.Sandbox.Resources.WallSeconds ||
		plan.Snapshot.PackageFingerprint != plan.Sandbox.PackageFingerprint ||
		plan.Snapshot.RuntimeBundleFingerprint != plan.Sandbox.RuntimeBundleFingerprint ||
		plan.Snapshot.GrantFingerprint != plan.GrantFingerprint ||
		plan.Snapshot.RegistryFingerprint != plan.ToolRegistry.RegistryFingerprint ||
		plan.Snapshot.WorkspaceFingerprint != plan.Sandbox.WorkspaceFingerprint ||
		plan.ToolRegistry.Depth != 0 || len(plan.ToolRegistry.Tools) != 0 || len(plan.Argv) < 1 ||
		!strings.HasPrefix(filepath.Clean(plan.Argv[0]), "/opt/neo/bin/") {
		return ErrInvalidPlan
	}
	snapshot, err := json.Marshal(plan.Snapshot)
	if err != nil || len(snapshot) == 0 {
		return ErrInvalidPlan
	}
	dummy := agentrunner.LaunchRequest{
		Attempt: agentrunner.AttemptRef{RunID: "run_0123456789abcdef", StepID: "step_0123456789abcdef",
			AttemptID: "attempt_0123456789abcdef", LeaseGeneration: 1,
			LeaseOwner: "neo-runner-primary", LeaseToken: "lease_0123456789abcdefghijklmnopqrstuv"},
		Lineage: agentrunner.RunLineage{RootRunID: "run_0123456789abcdef", Depth: 0},
		GrantID: plan.GrantID, GrantFingerprint: plan.GrantFingerprint,
		SnapshotFingerprint: "sha256:" + strings.Repeat("1", 64), Sandbox: plan.Sandbox,
		ToolRegistry: plan.ToolRegistry, Argv: plan.Argv,
	}
	if _, err := agentrunner.NewRequest(agentrunner.MethodLaunch, dummy, time.Unix(1, 0).UTC()); err != nil {
		return ErrInvalidPlan
	}
	return nil
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
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() < 2 || info.Size() > 64<<10 {
		return nil, ErrInvalidPlan
	}
	raw, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil || int64(len(raw)) != info.Size() || len(raw) > 64<<10 {
		return nil, ErrInvalidPlan
	}
	return raw, nil
}
