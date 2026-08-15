package agentcronworker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadPlanRequiresOneBoundedSyntheticTarget(t *testing.T) {
	plan := validCronWorkerPlan()
	raw, _ := json.Marshal(plan)
	path := filepath.Join(t.TempDir(), "cron-plan.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadPlan(path)
	if err != nil || loaded.TemplateID != plan.TemplateID || !planDigest.MatchString(loaded.DocumentFingerprint) {
		t.Fatalf("LoadPlan() = %#v, %v", loaded, err)
	}
	for name, mutate := range map[string]func(*Plan){
		"not synthetic": func(value *Plan) { value.Synthetic = false },
		"wrong target":  func(value *Plan) { value.TemplateID = "cron_short" },
		"wide window":   func(value *Plan) { value.ValidUntil = value.ValidFrom.Add(8 * 24 * time.Hour) },
		"long lease":    func(value *Plan) { value.LeaseSeconds = 301 },
		"large batch":   func(value *Plan) { value.BatchSize = 1001 },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := validCronWorkerPlan()
			mutate(&candidate)
			if ValidatePlan(candidate) == nil {
				t.Fatal("ValidatePlan() error = nil")
			}
		})
	}
}

func validCronWorkerPlan() Plan {
	start := time.Date(2026, 8, 15, 8, 0, 0, 0, time.UTC)
	return Plan{SchemaVersion: PlanSchemaVersion, Synthetic: true,
		ActivationID: "activation_0123456789abcdef", TemplateID: "cron_0123456789abcdef",
		UserID: "88888888-8888-4888-8888-888888888888", Revision: 1,
		RevisionFingerprint: "sha256:" + strings.Repeat("a", 64),
		ValidFrom:           start, ValidUntil: start.Add(time.Hour), Owner: "agent-cron-worker",
		PollMillis: 1000, LeaseSeconds: 30, BatchSize: 100,
		RetentionHours: 720, MaintenanceEvery: 60}
}
