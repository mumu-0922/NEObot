package agentprojectcanary

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentrunner"
)

const testProjectRunner = "neo-runner-project-canary"

func TestPlanAdmitsOnlyOneExactProjectMutation(t *testing.T) {
	now := time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC)
	plan := validProjectPlan(t, now)
	bindings, err := NewBindings(plan, testProjectRunner, now)
	if err != nil {
		t.Fatal(err)
	}
	binding, ok := bindings.Action()
	if !ok || binding.Plan.ToolIdentity != "project.patch" || binding.Plan.Approval != agentbroker.ApprovalPerCommit ||
		binding.Plan.Idempotent || binding.Registry.Tools[0].Classification != agentbroker.ClassificationMutable ||
		binding.Grant.Capabilities[0].MaxCalls != 1 || binding.Grant.Budget.MaxToolCalls != 1 ||
		binding.Grant.Egress.Mode != "none" || len(binding.Grant.Secrets) != 0 {
		t.Fatalf("binding widened = %#v", binding)
	}
	authority := binding.MutationAuthority()
	if authority.Resource != binding.Plan.Resource || authority.Path != binding.Plan.Project.Path ||
		authority.ContentFingerprint != binding.ContentFingerprint || authority.MutationFingerprint != binding.MutationFingerprint {
		t.Fatalf("mutation authority = %#v", authority)
	}
	for name, mutate := range map[string]func(*Plan){
		"tool":               func(value *Plan) { value.Action.ToolIdentity = "mcp.write" },
		"automatic approval": func(value *Plan) { value.Action.Approval = agentbroker.ApprovalAutomatic },
		"idempotent":         func(value *Plan) { value.Action.Idempotent = true },
		"network":            func(value *Plan) { value.Sandbox.NetworkMode = "bridge" },
		"path traversal":     func(value *Plan) { value.Action.Project.Path = "../escape" },
		"arguments drift":    func(value *Plan) { value.Action.Arguments = json.RawMessage(`{}`) },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := plan
			candidate.Action.Arguments = append(json.RawMessage(nil), plan.Action.Arguments...)
			mutate(&candidate)
			if _, err := NewBindings(candidate, testProjectRunner, now); err == nil {
				t.Fatal("widened plan accepted")
			}
		})
	}
}

func TestSignedApprovalBindsReleaseActivationPlanAndAction(t *testing.T) {
	now := time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC)
	plan := validProjectPlan(t, now)
	bindings, err := NewBindings(plan, testProjectRunner, now)
	if err != nil {
		t.Fatal(err)
	}
	binding, _ := bindings.Action()
	activationFingerprint := repeatedFingerprint("c")
	planFingerprint := repeatedFingerprint("d")
	release := strings.Repeat("a", 40)
	payload := approvalPayloadFor(binding, plan, release, activationFingerprint, planFingerprint, now)
	directory := t.TempDir()
	documentPath, publicKeyPath := writeSignedApproval(t, directory, payload)
	expected := ApprovalBinding{ReleaseCommit: release, TargetFingerprint: plan.TargetFingerprint,
		RunnerID: testProjectRunner, ActivationFingerprint: activationFingerprint,
		PlanFingerprint: planFingerprint, CallerIdentity: ProjectCanaryIdentity,
		RequestIdentity: binding.Plan.RequestIdentity, IdempotencyKey: binding.Plan.IdempotencyKey,
		Action: binding}
	if err := validateApprovalPayload(payload, expected, now); err != nil {
		t.Fatalf("payload validation = %v, payload=%#v", err, payload)
	}
	if raw, err := readApprovalFile(documentPath, true); err != nil {
		t.Fatalf("document read = %v", err)
	} else {
		var document signedApprovalDocument
		if err := json.Unmarshal(raw, &document); err != nil {
			t.Fatalf("document decode = %v", err)
		}
		decoded, _ := base64.RawURLEncoding.DecodeString(document.Signature)
		keyRaw, _ := os.ReadFile(publicKeyPath)
		key, _ := base64.RawURLEncoding.DecodeString(string(keyRaw))
		if !ed25519.Verify(key, append([]byte("neo-agent-project-mutation-approval-v1\x00"), document.Payload...), decoded) {
			t.Fatal("direct signature verification failed")
		}
	}
	approval, err := LoadApproval(documentPath, publicKeyPath, expected, now)
	if err != nil || approval.Payload.ApprovalID != payload.ApprovalID || approval.DocumentFingerprint == "" {
		t.Fatalf("LoadApproval() = %#v, %v", approval, err)
	}
	prepared := agentrunner.PrepareResult{Prepared: true, IntentID: "intent_0123456789abcdef",
		IntentFingerprint: repeatedFingerprint("e"), IdempotencyKey: "commit_0123456789abcdefghijklmn",
		Approval: agentbroker.ApprovalPerCommit, ExpiresAt: timePointer(now.Add(5 * time.Minute))}
	if err := approval.VerifyPrepareResult(prepared, binding, now); err != nil {
		t.Fatal(err)
	}
	decision := approval.DecisionInputForResult(plan.UserID, prepared)
	if decision.ApprovalID != payload.ApprovalID || decision.IntentID != prepared.IntentID ||
		decision.ActorType != "operator" || decision.ExpectedRevision != 1 {
		t.Fatalf("decision = %#v", decision)
	}

	drifted := expected
	drifted.IdempotencyKey += "-other"
	if _, err := LoadApproval(documentPath, publicKeyPath, drifted, now); err == nil {
		t.Fatal("approval binding drift accepted")
	}
	link := filepath.Join(directory, "approval-link.json")
	if err := os.Symlink(documentPath, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadApproval(link, publicKeyPath, expected, now); err == nil {
		t.Fatal("approval symlink accepted")
	}
	tampered, err := os.ReadFile(documentPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered = []byte(strings.Replace(string(tampered), `"decision":"approved"`, `"decision":"denied"`, 1))
	if err := os.WriteFile(documentPath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadApproval(documentPath, publicKeyPath, expected, now); err == nil {
		t.Fatal("tampered approval accepted")
	}
}

func TestActivationBindingFingerprintHasNoApprovalRecordHashCycle(t *testing.T) {
	value := ActivationBindingFingerprint(strings.Repeat("a", 40), repeatedFingerprint("4"),
		testProjectRunner, repeatedFingerprint("d"), ProjectCanaryIdentity,
		"https://172.31.254.10:9445/internal/agent-broker/v1/relay")
	if !planFingerprint.MatchString(value) || value == ActivationBindingFingerprint(strings.Repeat("a", 40),
		repeatedFingerprint("4"), testProjectRunner, repeatedFingerprint("d"), ProjectCanaryIdentity,
		"https://172.31.254.11:9445/internal/agent-broker/v1/relay") {
		t.Fatalf("activation binding fingerprint = %q", value)
	}
}

func validProjectPlan(t *testing.T, now time.Time) Plan {
	t.Helper()
	packageFingerprint := repeatedFingerprint("1")
	runtimeFingerprint := repeatedFingerprint("2")
	content := "synthetic mutation\n"
	contentFingerprint := domainFingerprint("neo-project-file-v1", []byte(content))
	baseRevision := repeatedFingerprint("3")
	resource := "project-canary/g21-3"
	pathValue := "g21-3-canary.txt"
	mutationFingerprint := agentbroker.ProjectMutationFingerprint(baseRevision, resource, pathValue, contentFingerprint)
	arguments, err := json.Marshal(map[string]any{"contentFingerprint": contentFingerprint,
		"mutationFingerprint": mutationFingerprint, "path": pathValue, "sizeBytes": len(content)})
	if err != nil {
		t.Fatal(err)
	}
	return Plan{SchemaVersion: PlanSchemaVersion, Synthetic: true,
		UserID: "31313131-3131-4313-8313-313131313131", ProjectID: "project_31313131",
		AssistantID: "assistant_31313131", PackageFingerprint: packageFingerprint,
		RuntimeBundleFingerprint: runtimeFingerprint, TargetFingerprint: repeatedFingerprint("4"),
		IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), LeaseSeconds: 30,
		Sandbox: agentrunner.SandboxSpec{RuntimeBundleFingerprint: runtimeFingerprint,
			PackageFingerprint: packageFingerprint,
			Image:              "registry.invalid/neo/project-canary@" + repeatedFingerprint("5"), UID: 10001, GID: 10001,
			RootfsReadOnly: true, NoNewPrivileges: true, Capabilities: []string{},
			SeccompProfileFingerprint: repeatedFingerprint("6"), NetworkMode: "none",
			WorkspaceSnapshotID:  "workspace_snapshot_3131313131313131",
			WorkspaceFingerprint: repeatedFingerprint("7"), Resources: agentrunner.ResourceLimits{
				CPUMillis: 250, MemoryMiB: 128, PIDs: 16, WallSeconds: 30,
				OutputBytes: 4096, ScratchBytes: 1048576}},
		Argv: []string{"/opt/neo/bin/project-canary", "--wait-for-cancel"},
		Action: ActionPlan{ID: ActionProjectMutation, RequestIdentity: "g21.3-request-project-mutation",
			IdempotencyKey: "g21.3-project-mutation-reviewed", ToolIdentity: "project.patch",
			Capability: "project.write", Action: "apply_patch", Resource: resource,
			Classification: agentbroker.ClassificationMutable, Idempotent: false,
			Approval: agentbroker.ApprovalPerCommit, Arguments: arguments, BaseRevision: baseRevision,
			TTLSeconds: 300, Project: ProjectPolicy{Resource: resource, Path: pathValue,
				BaseRevision: baseRevision, Content: content, MaxBytes: 1024}}}
}

func approvalPayloadFor(binding Binding, plan Plan, release, activationFingerprint,
	planFingerprint string, now time.Time,
) ApprovalPayload {
	return ApprovalPayload{SchemaVersion: ApprovalSchemaVersion,
		ApprovalID: "approval_3131313131313131", Decision: "approved",
		Release: ApprovalRelease{GitCommit: release, MigrationHead: 95},
		Target:  ApprovalTarget{DeploymentFingerprint: plan.TargetFingerprint, RunnerID: testProjectRunner},
		Activation: ApprovalActivation{Stage: ApprovalStage, ActivationFingerprint: activationFingerprint,
			PlanFingerprint: planFingerprint},
		Request: ApprovalRequest{CallerIdentity: ProjectCanaryIdentity,
			RequestIdentity: binding.Plan.RequestIdentity, IdempotencyKey: binding.Plan.IdempotencyKey},
		Action: ApprovalAction{ToolIdentity: binding.Plan.ToolIdentity, Capability: binding.Plan.Capability,
			Action: binding.Plan.Action, Resource: binding.Plan.Resource, BaseRevision: binding.Plan.BaseRevision,
			Path: binding.Plan.Project.Path, ContentFingerprint: binding.ContentFingerprint,
			MutationFingerprint: binding.MutationFingerprint},
		Actor: ApprovalActor{Type: "operator", ID: "release-operator", ReasonCode: "PROJECT_CANARY_APPROVED"},
		Window: ApprovalWindow{IssuedAt: now.Add(-time.Minute), NotBefore: now.Add(-30 * time.Second),
			ExpiresAt: now.Add(5 * time.Minute)}}
}

func writeSignedApproval(t *testing.T, directory string, payload ApprovalPayload) (string, string) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payloadRaw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	signingInput := append([]byte("neo-agent-project-mutation-approval-v1\x00"), payloadRaw...)
	signature := ed25519.Sign(privateKey, signingInput)
	document, err := json.Marshal(signedApprovalDocument{Payload: payloadRaw,
		Signature: base64.RawURLEncoding.EncodeToString(signature)})
	if err != nil {
		t.Fatal(err)
	}
	documentPath := filepath.Join(directory, "approval.json")
	publicKeyPath := filepath.Join(directory, "approval-public-key")
	if err := os.WriteFile(documentPath, document, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(publicKeyPath, []byte(base64.RawURLEncoding.EncodeToString(publicKey)), 0o600); err != nil {
		t.Fatal(err)
	}
	return documentPath, publicKeyPath
}

func repeatedFingerprint(character string) string { return "sha256:" + strings.Repeat(character, 64) }
func timePointer(value time.Time) *time.Time      { return &value }
