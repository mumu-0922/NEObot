// Package agentproductcanary owns the fixed G21.6 product-canary request,
// worker and execution-plan boundary.
package agentproductcanary

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/unix"

	"neo-chat/mm-chat/backend/internal/agentrootcanary"
	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

const PlanSchemaVersion = "neo.agent-product-canary-plan/v1"

var (
	ErrInvalidPlan = errors.New("PRODUCT_CANARY_PLAN_INVALID")
	idPattern      = regexp.MustCompile(`^[a-z][a-z0-9_]*_[a-z0-9]{16,64}$`)
	fpPattern      = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

type Plan struct {
	SchemaVersion    string                   `json:"schemaVersion"`
	ActivationID     string                   `json:"activationId"`
	StepKind         string                   `json:"stepKind"`
	GrantID          string                   `json:"grantId"`
	GrantFingerprint string                   `json:"grantFingerprint"`
	Snapshot         agentrootcanary.Snapshot `json:"snapshot"`
	Sandbox          agentrunner.SandboxSpec  `json:"sandbox"`
	ToolRegistry     agentrunner.ToolRegistry `json:"toolRegistry"`
	Argv             []string                 `json:"argv"`
	LeaseSeconds     int                      `json:"leaseSeconds"`
	Fingerprint      string                   `json:"-"`
}

func LoadPlan(path string) (Plan, error) {
	raw, err := readPrivate(path)
	if err != nil {
		return Plan{}, ErrInvalidPlan
	}
	var plan Plan
	if strictjson.Decode(raw, 64<<10, &plan) != nil {
		return Plan{}, ErrInvalidPlan
	}
	digest := sha256.Sum256(raw)
	plan.Fingerprint = "sha256:" + hex.EncodeToString(digest[:])
	if ValidatePlan(plan) != nil {
		return Plan{}, ErrInvalidPlan
	}
	return plan, nil
}

func ValidatePlan(plan Plan) error {
	if plan.SchemaVersion != PlanSchemaVersion || !idPattern.MatchString(plan.ActivationID) ||
		plan.StepKind != "product_canary" || !idPattern.MatchString(plan.GrantID) ||
		!fpPattern.MatchString(plan.GrantFingerprint) || !fpPattern.MatchString(plan.Fingerprint) {
		return ErrInvalidPlan
	}
	zero := "sha256:" + strings.Repeat("0", 64)
	for _, value := range []string{
		plan.GrantFingerprint, plan.Snapshot.PackageFingerprint,
		plan.Snapshot.RuntimeBundleFingerprint, plan.Snapshot.GrantFingerprint,
		plan.Snapshot.RegistryFingerprint, plan.Snapshot.WorkspaceFingerprint,
		plan.Sandbox.RuntimeBundleFingerprint, plan.Sandbox.PackageFingerprint,
		plan.Sandbox.SeccompProfileFingerprint, plan.Sandbox.WorkspaceFingerprint,
		plan.ToolRegistry.RegistryFingerprint,
	} {
		if value == zero {
			return ErrInvalidPlan
		}
	}
	if strings.HasSuffix(plan.Sandbox.Image, "@"+zero) {
		return ErrInvalidPlan
	}
	probe := plan.ExecutionPlan(Request{ID: "product_request_1234567890abcdef",
		UserID: "11111111-1111-4111-8111-111111111111", ActivationID: plan.ActivationID,
		PackageFingerprint:       plan.Sandbox.PackageFingerprint,
		RuntimeBundleFingerprint: plan.Sandbox.RuntimeBundleFingerprint,
		PlanFingerprint:          plan.Fingerprint})
	if agentrootcanary.ValidateProductPlan(probe) != nil {
		return ErrInvalidPlan
	}
	return nil
}

func (plan Plan) ExecutionPlan(request Request) agentrootcanary.Plan {
	return agentrootcanary.Plan{
		SchemaVersion: agentrootcanary.ProductPlanSchemaVersion,
		Synthetic:     false, UserID: request.UserID,
		IdempotencyKey: "g21.6-product-canary-" + strings.TrimPrefix(request.ID, "product_request_"),
		StepKind:       plan.StepKind, GrantID: plan.GrantID,
		GrantFingerprint: plan.GrantFingerprint, Snapshot: plan.Snapshot,
		Sandbox: plan.Sandbox, ToolRegistry: plan.ToolRegistry,
		Argv: append([]string(nil), plan.Argv...), LeaseSeconds: plan.LeaseSeconds,
	}
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
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 ||
		info.Size() < 2 || info.Size() > 64<<10 {
		return nil, ErrInvalidPlan
	}
	raw, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil || int64(len(raw)) != info.Size() || len(raw) > 64<<10 {
		return nil, ErrInvalidPlan
	}
	return raw, nil
}
