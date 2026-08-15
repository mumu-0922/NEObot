package agentlearningworker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/agentlearning"
	"neo-chat/mm-chat/backend/internal/agentrunner"
)

func TestLoadPlanRequiresExactDraftOnlyRunnerShape(t *testing.T) {
	plan := validPlan()
	raw, err := MarshalPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadPlan(path)
	if err != nil || loaded.ActivationID != plan.ActivationID || !fingerprintRE.MatchString(loaded.DocumentFingerprint) {
		t.Fatalf("LoadPlan() = %#v, %v", loaded, err)
	}

	for name, mutate := range map[string]func(*Plan){
		"not synthetic":   func(value *Plan) { value.Synthetic = false },
		"wrong caller":    func(value *Plan) { value.CallerIdentity = "spiffe://neo-chat/agent-runtime-control" },
		"network":         func(value *Plan) { value.Sandbox.NetworkMode = "bridge" },
		"capability":      func(value *Plan) { value.Sandbox.Capabilities = []string{"NET_ADMIN"} },
		"tool":            func(value *Plan) { value.ToolRegistry.Tools = []string{"delegate_task"} },
		"floating image":  func(value *Plan) { value.Sandbox.Image = "registry.example/checker:latest" },
		"arbitrary argv":  func(value *Plan) { value.Checks[0].Argv = []string{"/bin/sh", "-c"} },
		"duplicate suite": func(value *Plan) { value.Checks[1].SuiteFingerprint = value.Checks[0].SuiteFingerprint },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := validPlan()
			mutate(&candidate)
			if err := ValidatePlan(candidate); err == nil {
				t.Fatal("ValidatePlan() error = nil")
			}
		})
	}
}

func TestValidateResultBindsArtifactAndEveryDraftPlanField(t *testing.T) {
	plan := validPlan()
	runtime := &Runtime{plan: plan}
	attempt := agentrunner.AttemptRef{RunID: "run_0123456789abcdef",
		StepID: "step_0123456789abcdef", AttemptID: "attempt_0123456789abcdef",
		LeaseGeneration: 1, LeaseOwner: plan.RunnerID,
		LeaseToken: "lease_0123456789abcdefghijklmnopqrstuv"}
	draft := agentlearning.Draft{ID: plan.DraftID, UserID: plan.UserID,
		DraftFingerprint: plan.DraftFingerprint, CheckGeneration: 4}
	check := plan.Checks[0]
	payload := resultPayload{SchemaVersion: ResultVersion, ActivationID: plan.ActivationID,
		DraftID: plan.DraftID, DraftFingerprint: plan.DraftFingerprint,
		CheckGeneration: 4, Kind: check.Kind,
		ProposedPackageFingerprint: plan.ProposedPackageFingerprint,
		RuntimeBundleFingerprint:   plan.RuntimeBundleFingerprint,
		ArchiveFingerprint:         plan.ArchiveFingerprint,
		WorkspaceSnapshotID:        plan.Sandbox.WorkspaceSnapshotID,
		WorkspaceFingerprint:       plan.Sandbox.WorkspaceFingerprint,
		SuiteFingerprint:           check.SuiteFingerprint, Status: agentlearning.CheckPassed,
		ReasonCode: "ISOLATION_PASSED", DurationMillis: 12,
		Metrics: map[string]int64{"casesPassed": 4}}
	raw, _ := json.Marshal(payload)
	digest := sha256.Sum256(raw)
	result := agentrunner.ResultResult{Ready: true, Payload: raw,
		Receipt: &agentrunner.ArtifactReceipt{Attempt: attempt.Identity(), Name: ResultArtifact,
			MediaType: "application/json", Size: int64(len(raw)),
			Fingerprint: "sha256:" + hex.EncodeToString(digest[:])}}
	if _, err := runtime.validateResult(check.Kind, check, draft, attempt, result); err != nil {
		t.Fatalf("validateResult() error = %v", err)
	}

	result.Receipt.Fingerprint = fingerprint("f")
	if _, err := runtime.validateResult(check.Kind, check, draft, attempt, result); err == nil {
		t.Fatal("validateResult() accepted forged receipt")
	}
	result.Receipt.Fingerprint = "sha256:" + hex.EncodeToString(digest[:])
	var drift map[string]any
	_ = json.Unmarshal(raw, &drift)
	drift["workspaceFingerprint"] = fingerprint("e")
	result.Payload, _ = json.Marshal(drift)
	driftDigest := sha256.Sum256(result.Payload)
	result.Receipt.Size = int64(len(result.Payload))
	result.Receipt.Fingerprint = "sha256:" + hex.EncodeToString(driftDigest[:])
	if _, err := runtime.validateResult(check.Kind, check, draft, attempt, result); err == nil {
		t.Fatal("validateResult() accepted workspace drift")
	}
}

func validPlan() Plan {
	return Plan{SchemaVersion: PlanSchemaVersion, Synthetic: true,
		ActivationID: "activation_0123456789abcdef", DraftID: "draft_0123456789abcdef",
		UserID: "88888888-8888-4888-8888-888888888888", DraftFingerprint: fingerprint("1"),
		ProposedPackageFingerprint: fingerprint("2"), RuntimeBundleFingerprint: fingerprint("3"),
		ArchiveFingerprint: fingerprint("4"), RunnerID: "neo-runner-primary",
		CallerIdentity: CallerIdentity, RunnerSnapshotFingerprint: fingerprint("5"),
		GrantID: "grant_0123456789abcdef", GrantFingerprint: fingerprint("6"),
		ToolRegistry: agentrunner.ToolRegistry{Depth: 0, Tools: []string{}, RegistryFingerprint: fingerprint("7")},
		Sandbox: agentrunner.SandboxSpec{RuntimeBundleFingerprint: fingerprint("3"),
			PackageFingerprint: fingerprint("2"), Image: "registry.example/draft-checker@" + fingerprint("8"),
			UID: 10001, GID: 10001, RootfsReadOnly: true, NoNewPrivileges: true,
			Capabilities: []string{}, SeccompProfileFingerprint: fingerprint("9"), NetworkMode: "none",
			WorkspaceSnapshotID: "workspace_snapshot_0123456789abcdef", WorkspaceFingerprint: fingerprint("a"),
			Resources: agentrunner.ResourceLimits{CPUMillis: 500, MemoryMiB: 256, PIDs: 16,
				WallSeconds: 60, OutputBytes: 65536, ScratchBytes: 1 << 20}},
		Checks: []CheckPlan{
			{Kind: "isolation", SuiteFingerprint: fingerprint("b"),
				Argv: []string{"/opt/neo/bin/draft-isolation-check", "--result-artifact=" + ResultArtifact}},
			{Kind: "evaluation", SuiteFingerprint: fingerprint("c"),
				Argv: []string{"/opt/neo/bin/draft-evaluation-check", "--result-artifact=" + ResultArtifact}},
		}, LeaseSeconds: 120, ResultTimeoutSeconds: 60, ResultPollMillis: 250}
}

func fingerprint(value string) string { return "sha256:" + strings.Repeat(value, 64) }
