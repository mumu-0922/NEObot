// Package agentlearningworker runs the G21.5 exact-target Draft isolation and
// evaluation checks through the credential-free rootless Runner.
package agentlearningworker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/unix"

	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

const (
	PlanSchemaVersion = "neo.agent-draft-learning-worker-plan/v1"
	ResultVersion     = "neo.agent-draft-runner-result/v1"
	CallerIdentity    = "spiffe://neo-chat/agent-runtime-draft-learning"
	ResultArtifact    = "draft-check-result.json"
)

var (
	ErrInvalidPlan = errors.New("DRAFT_LEARNING_PLAN_INVALID")
	fingerprintRE  = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	uuidRE         = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	idRE           = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}_[a-z0-9]{16,64}$`)
)

type CheckPlan struct {
	Kind             string   `json:"kind"`
	SuiteFingerprint string   `json:"suiteFingerprint"`
	Argv             []string `json:"argv"`
}

type Plan struct {
	SchemaVersion              string                   `json:"schemaVersion"`
	Synthetic                  bool                     `json:"synthetic"`
	ActivationID               string                   `json:"activationId"`
	DraftID                    string                   `json:"draftId"`
	UserID                     string                   `json:"userId"`
	DraftFingerprint           string                   `json:"draftFingerprint"`
	ProposedPackageFingerprint string                   `json:"proposedPackageFingerprint"`
	RuntimeBundleFingerprint   string                   `json:"runtimeBundleFingerprint"`
	ArchiveFingerprint         string                   `json:"archiveFingerprint"`
	RunnerID                   string                   `json:"runnerId"`
	CallerIdentity             string                   `json:"callerIdentity"`
	RunnerSnapshotFingerprint  string                   `json:"runnerSnapshotFingerprint"`
	GrantID                    string                   `json:"grantId"`
	GrantFingerprint           string                   `json:"grantFingerprint"`
	ToolRegistry               agentrunner.ToolRegistry `json:"toolRegistry"`
	Sandbox                    agentrunner.SandboxSpec  `json:"sandbox"`
	Checks                     []CheckPlan              `json:"checks"`
	LeaseSeconds               int                      `json:"leaseSeconds"`
	ResultTimeoutSeconds       int                      `json:"resultTimeoutSeconds"`
	ResultPollMillis           int                      `json:"resultPollMillis"`
	DocumentFingerprint        string                   `json:"-"`
}

func LoadPlan(path string) (Plan, error) {
	raw, err := readPrivate(path, 96<<10)
	if err != nil {
		return Plan{}, ErrInvalidPlan
	}
	var plan Plan
	if strictjson.Decode(raw, 96<<10, &plan) != nil {
		return Plan{}, ErrInvalidPlan
	}
	digest := sha256.Sum256(raw)
	plan.DocumentFingerprint = "sha256:" + hex.EncodeToString(digest[:])
	if ValidatePlan(plan) != nil {
		return Plan{}, ErrInvalidPlan
	}
	return plan, nil
}

func ValidatePlan(plan Plan) error {
	if plan.SchemaVersion != PlanSchemaVersion || !plan.Synthetic ||
		!validID(plan.ActivationID, "activation") || !validID(plan.DraftID, "draft") ||
		!uuidRE.MatchString(plan.UserID) || !fingerprintRE.MatchString(plan.DraftFingerprint) ||
		!fingerprintRE.MatchString(plan.ProposedPackageFingerprint) ||
		!fingerprintRE.MatchString(plan.RuntimeBundleFingerprint) ||
		!fingerprintRE.MatchString(plan.ArchiveFingerprint) ||
		len(plan.RunnerID) < 1 || len(plan.RunnerID) > 128 ||
		plan.CallerIdentity != CallerIdentity ||
		!fingerprintRE.MatchString(plan.RunnerSnapshotFingerprint) ||
		!validID(plan.GrantID, "grant") || !fingerprintRE.MatchString(plan.GrantFingerprint) ||
		plan.ToolRegistry.Depth != 0 || len(plan.ToolRegistry.Tools) != 0 ||
		!fingerprintRE.MatchString(plan.ToolRegistry.RegistryFingerprint) ||
		plan.LeaseSeconds < 30 || plan.LeaseSeconds > 300 ||
		plan.ResultTimeoutSeconds < 5 || plan.ResultTimeoutSeconds > plan.LeaseSeconds ||
		plan.ResultPollMillis < 100 || plan.ResultPollMillis > 5000 ||
		!validSandbox(plan) || !validChecks(plan.Checks) {
		return ErrInvalidPlan
	}
	return nil
}

func (plan Plan) Check(kind string) (CheckPlan, bool) {
	for _, check := range plan.Checks {
		if check.Kind == kind {
			check.Argv = append([]string(nil), check.Argv...)
			return check, true
		}
	}
	return CheckPlan{}, false
}

func validSandbox(plan Plan) bool {
	value := plan.Sandbox
	return value.PackageFingerprint == plan.ProposedPackageFingerprint &&
		value.RuntimeBundleFingerprint == plan.RuntimeBundleFingerprint &&
		strings.Contains(value.Image, "@sha256:") && value.UID >= 10000 && value.GID >= 10000 &&
		value.RootfsReadOnly && value.NoNewPrivileges && len(value.Capabilities) == 0 &&
		value.NetworkMode == "none" && fingerprintRE.MatchString(value.SeccompProfileFingerprint) &&
		validID(value.WorkspaceSnapshotID, "workspace_snapshot") &&
		fingerprintRE.MatchString(value.WorkspaceFingerprint) &&
		value.Resources.CPUMillis >= 50 && value.Resources.CPUMillis <= 1000 &&
		value.Resources.MemoryMiB >= 32 && value.Resources.MemoryMiB <= 512 &&
		value.Resources.PIDs >= 4 && value.Resources.PIDs <= 32 &&
		value.Resources.WallSeconds >= 5 && value.Resources.WallSeconds <= 120 &&
		value.Resources.OutputBytes >= 1024 && value.Resources.OutputBytes <= 65536 &&
		value.Resources.ScratchBytes >= 4096 && value.Resources.ScratchBytes <= 16<<20
}

func validChecks(checks []CheckPlan) bool {
	if len(checks) != 2 || checks[0].Kind != "isolation" || checks[1].Kind != "evaluation" {
		return false
	}
	binaries := map[string]string{
		"isolation":  "/opt/neo/bin/draft-isolation-check",
		"evaluation": "/opt/neo/bin/draft-evaluation-check",
	}
	for _, check := range checks {
		if !fingerprintRE.MatchString(check.SuiteFingerprint) || len(check.Argv) != 2 ||
			check.Argv[0] != binaries[check.Kind] || check.Argv[1] != "--result-artifact="+ResultArtifact {
			return false
		}
	}
	return checks[0].SuiteFingerprint != checks[1].SuiteFingerprint
}

func validID(value, prefix string) bool {
	return strings.HasPrefix(value, prefix+"_") && idRE.MatchString(value)
}

func readPrivate(path string, limit int64) ([]byte, error) {
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) || limit < 2 {
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
		info.Size() < 2 || info.Size() > limit {
		return nil, ErrInvalidPlan
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(raw)) != info.Size() || int64(len(raw)) > limit {
		return nil, ErrInvalidPlan
	}
	return raw, nil
}

func MarshalPlan(plan Plan) ([]byte, error) {
	plan.DocumentFingerprint = ""
	return json.Marshal(plan)
}
