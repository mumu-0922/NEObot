package agentcronworker

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"neo-chat/mm-chat/backend/internal/strictjson"
)

const PlanSchemaVersion = "neo.agent-cron-worker-plan/v1"

var (
	ErrInvalidPlan = errors.New("AGENT_CRON_WORKER_PLAN_INVALID")
	planIDPattern  = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}_[a-z0-9]{16,64}$`)
	planUUID       = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	planDigest     = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

type Plan struct {
	SchemaVersion       string    `json:"schemaVersion"`
	Synthetic           bool      `json:"synthetic"`
	ActivationID        string    `json:"activationId"`
	TemplateID          string    `json:"templateId"`
	UserID              string    `json:"userId"`
	Revision            int64     `json:"revision"`
	RevisionFingerprint string    `json:"revisionFingerprint"`
	ValidFrom           time.Time `json:"validFrom"`
	ValidUntil          time.Time `json:"validUntil"`
	Owner               string    `json:"owner"`
	PollMillis          int       `json:"pollMillis"`
	LeaseSeconds        int       `json:"leaseSeconds"`
	BatchSize           int       `json:"batchSize"`
	RetentionHours      int       `json:"retentionHours"`
	MaintenanceEvery    int       `json:"maintenanceEvery"`
	DocumentFingerprint string    `json:"-"`
}

func LoadPlan(path string) (Plan, error) {
	raw, err := readPlanFile(path)
	if err != nil {
		return Plan{}, ErrInvalidPlan
	}
	var plan Plan
	if strictjson.Decode(raw, 64<<10, &plan) != nil {
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
		!validPlanID(plan.ActivationID, "activation") || !validPlanID(plan.TemplateID, "cron") ||
		!planUUID.MatchString(plan.UserID) || plan.Revision < 1 ||
		!planDigest.MatchString(plan.RevisionFingerprint) || plan.ValidFrom.IsZero() ||
		plan.ValidUntil.IsZero() || !plan.ValidUntil.After(plan.ValidFrom) ||
		plan.ValidUntil.Sub(plan.ValidFrom) > 7*24*time.Hour ||
		len(plan.Owner) < 1 || len(plan.Owner) > 128 ||
		plan.PollMillis < 100 || plan.PollMillis > 60_000 ||
		plan.LeaseSeconds < 5 || plan.LeaseSeconds > 300 ||
		plan.BatchSize < 1 || plan.BatchSize > 1000 ||
		plan.RetentionHours < 1 || plan.RetentionHours > 24*365 ||
		plan.MaintenanceEvery < 1 || plan.MaintenanceEvery > 10_000 {
		return ErrInvalidPlan
	}
	return nil
}

func (plan Plan) Config() Config {
	return Config{Owner: plan.Owner, PollInterval: time.Duration(plan.PollMillis) * time.Millisecond,
		LeaseDuration: time.Duration(plan.LeaseSeconds) * time.Second, BatchSize: plan.BatchSize,
		Retention:        time.Duration(plan.RetentionHours) * time.Hour,
		MaintenanceEvery: plan.MaintenanceEvery}
}

func validPlanID(value, prefix string) bool {
	return strings.HasPrefix(value, prefix+"_") && planIDPattern.MatchString(value)
}

func readPlanFile(path string) ([]byte, error) {
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
