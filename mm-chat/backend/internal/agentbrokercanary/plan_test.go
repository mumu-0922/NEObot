package agentbrokercanary

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/mcpclient"
)

const testCanaryRunner = "neo-runner-primary"

func TestBindingsDeriveStableAuthorityAndRejectPrepareCommitDrift(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	plan := validCanaryPlan(t, now, t.TempDir())
	first, err := NewBindings(plan, testCanaryRunner, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewBindings(plan, testCanaryRunner, now)
	if err != nil {
		t.Fatal(err)
	}
	binding, _ := first.Action(ActionProjectRead)
	replay, _ := second.Action(ActionProjectRead)
	if binding.RunID != replay.RunID || binding.StepID != replay.StepID ||
		binding.Registry.Fingerprint != replay.Registry.Fingerprint || binding.SnapshotFingerprint != replay.SnapshotFingerprint {
		t.Fatalf("binding drift = %#v / %#v", binding, replay)
	}
	grantFingerprint, _ := agentbroker.GrantFingerprint(binding.Grant)
	attempt := agentrunner.AttemptRef{RunID: binding.RunID, StepID: binding.StepID,
		AttemptID: "attempt_0123456789abcdef", LeaseGeneration: 1,
		LeaseOwner: testCanaryRunner, LeaseToken: "lease_0123456789abcdefghijklmnopqrstuv"}
	prepare := agentrunner.PrepareRequest{Attempt: attempt, Authority: agentrunner.AuthorityTicket{
		AuthorityClaims: agentrunner.AuthorityClaims{CallerIdentity: BrokerCanaryIdentity,
			Method: agentrunner.MethodPrepare}}, SnapshotFingerprint: binding.SnapshotFingerprint,
		GrantID: binding.Grant.GrantID, GrantFingerprint: grantFingerprint,
		RegistryFingerprint: binding.Registry.Fingerprint, ToolIdentity: binding.Plan.ToolIdentity,
		Capability: binding.Plan.Capability, Action: binding.Plan.Action, Resource: binding.Plan.Resource,
		Arguments: binding.Plan.Arguments, ArgumentsFingerprint: binding.ArgumentsFingerprint,
		BaseRevision: binding.Plan.BaseRevision, TTLSeconds: binding.Plan.TTLSeconds}
	resolved, err := first.ResolvePrepare(prepare, now)
	if err != nil || resolved.UserID != plan.UserID || resolved.Grant.GrantID != binding.Grant.GrantID {
		t.Fatalf("ResolvePrepare = %#v, %v", resolved, err)
	}
	changed := prepare
	changed.Capability = "workspace.read"
	if _, err := first.ResolvePrepare(changed, now); !errors.Is(err, agentbroker.ErrSnapshotMismatch) {
		t.Fatalf("Prepare drift = %v", err)
	}
	commit := agentrunner.CommitRequest{Attempt: attempt, Authority: agentrunner.AuthorityTicket{
		AuthorityClaims: agentrunner.AuthorityClaims{CallerIdentity: BrokerCanaryIdentity,
			Method: agentrunner.MethodCommit}}, SnapshotFingerprint: binding.SnapshotFingerprint,
		GrantFingerprint: grantFingerprint, RegistryFingerprint: binding.Registry.Fingerprint,
		IntentID: "intent_0123456789abcdef", IntentFingerprint: testCanaryFingerprint("8"),
		IdempotencyKey: "commit_0123456789abcdefghijklmn"}
	if userID, err := first.ResolveCommit(commit, now); err != nil || userID != plan.UserID {
		t.Fatalf("ResolveCommit = %q, %v", userID, err)
	}
	commit.RegistryFingerprint = testCanaryFingerprint("9")
	if _, err := first.ResolveCommit(commit, now); !errors.Is(err, agentbroker.ErrSnapshotMismatch) {
		t.Fatalf("Commit drift = %v", err)
	}
}

func TestExactFileReaderUsesOpenat2AndReturnsContentFreeReceipt(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	root := t.TempDir()
	payload := []byte("reviewed project bytes")
	if err := os.WriteFile(filepath.Join(root, "fixture.txt"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	plan := validCanaryPlan(t, now, root)
	binding, _ := mustBindings(t, plan, now).Action(ActionProjectRead)
	reader, err := NewExactFileReader(binding.Plan)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reader.Read(context.Background(), executionFor(binding))
	if err != nil || !bytes.Contains(result, []byte(`"fingerprint":"sha256:`)) ||
		bytes.Contains(result, payload) {
		t.Fatalf("Read = %s, %v", result, err)
	}
	if err := os.Symlink("fixture.txt", filepath.Join(root, "linked.txt")); err != nil {
		t.Fatal(err)
	}
	linked := binding.Plan
	linked.File = &FileReadPlan{Root: root, RelativePath: "linked.txt", ExpectedBytes: int64(len(payload)),
		ExpectedFingerprint: contentFingerprint(payload), MaxBytes: 1024}
	linked.Arguments = json.RawMessage(`{"path":"linked.txt"}`)
	linkedReader, _ := NewExactFileReader(linked)
	if _, err := linkedReader.Read(context.Background(), executionForPlan(linked)); !errors.Is(err, agentbroker.ErrExecutorUnavailable) {
		t.Fatalf("symlink read = %v", err)
	}
}

func TestMCPReadExecutorPinsManifestAuthNoneReadToolAndSanitizesResult(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	plan := validCanaryPlan(t, now, t.TempDir())
	binding, _ := mustBindings(t, plan, now).Action(ActionMCPRead)
	session := &canaryMCPSession{tools: []mcpclient.Tool{{ServerRef: mcpclient.ServerRef{
		Source: mcpclient.SourceManifest, ID: binding.Plan.MCP.ServerID}, Name: binding.Plan.MCP.ToolName,
		InputSchema: map[string]any{"type": "object"}, Classification: mcpclient.ClassificationRead,
		Supported: true}}, result: mcpclient.CallResult{Content: []mcpclient.Content{{
		Type: "text", Text: "raw MCP result must not escape", ByteSize: 30}}}}
	connector := &canaryMCPConnector{session: session}
	executor, err := NewMCPReadExecutor(binding.Plan, connector)
	if err != nil {
		t.Fatal(err)
	}
	result, err := executor.Read(context.Background(), executionFor(binding))
	if err != nil || connector.credential != "" || connector.server.AuthType != mcpclient.AuthNone ||
		connector.server.Transport != mcpclient.TransportStdio || session.calls != 1 ||
		bytes.Contains(result, []byte("raw MCP")) {
		t.Fatalf("MCP Read = %s, %v, connector=%#v calls=%d", result, err, connector.server, session.calls)
	}
	session.callErr = errors.New("acknowledgement lost")
	if _, err := executor.Read(context.Background(), executionFor(binding)); err == nil {
		t.Fatal("possible-send MCP failure was accepted")
	} else {
		var dispatch *agentbroker.DispatchError
		if !errors.As(err, &dispatch) || !dispatch.PossibleSend {
			t.Fatalf("MCP dispatch error = %v", err)
		}
	}
}

func TestArtifactExecutorUsesQuarantineAuthorizationAndObjectBeforeRow(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	plan := validCanaryPlan(t, now, t.TempDir())
	binding, _ := mustBindings(t, plan, now).Action(ActionArtifact)
	request := executionFor(binding)
	request.UserID, request.RunID, request.StepID = plan.UserID, binding.RunID, binding.StepID
	request.AttemptID, request.Generation = "attempt_0123456789abcdef", 1
	request.SnapshotFingerprint = binding.SnapshotFingerprint
	request.GrantFingerprint, _ = agentbroker.GrantFingerprint(binding.Grant)
	request.RegistryFingerprint = binding.Registry.Fingerprint
	root := t.TempDir()
	attemptRoot := filepath.Join(root, request.AttemptID)
	if err := os.Mkdir(attemptRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := []byte("artifact")
	if err := os.WriteFile(filepath.Join(attemptRoot, binding.Plan.Artifact.Name), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	quarantine, err := NewDirectoryQuarantine(root)
	if err != nil {
		t.Fatal(err)
	}
	order := []string{}
	repository := &canaryArtifactRepository{order: &order}
	objects := &canaryArtifactObjects{order: &order}
	publisher, err := agentbroker.NewArtifactPublisher(quarantine, objects, repository, CanaryArtifactScanner{})
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewArtifactExecutor(binding.Plan, plan.ArtifactPolicy, publisher)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := executor.Commit(context.Background(), request)
	if err != nil || !strings.HasPrefix(receipt.ReceiptFingerprint, "sha256:") ||
		strings.Join(order, ",") != "authorize,put,attach" || repository.candidate.IntentID != request.IntentID {
		t.Fatalf("Artifact Commit = %#v, %v, order=%v candidate=%#v", receipt, err, order, repository.candidate)
	}
	if _, err := os.Stat(filepath.Join(attemptRoot, binding.Plan.Artifact.Name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("quarantine cleanup = %v", err)
	}
}

func TestPossibleSendExecutorReturnsAmbiguousDispatch(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	binding, _ := mustBindings(t, validCanaryPlan(t, now, t.TempDir()), now).Action(ActionPossibleSend)
	executor, err := NewPossibleSendExecutor(binding.Plan)
	if err != nil {
		t.Fatal(err)
	}
	_, err = executor.Read(context.Background(), executionFor(binding))
	var dispatch *agentbroker.DispatchError
	if !errors.As(err, &dispatch) || !dispatch.PossibleSend {
		t.Fatalf("possible-send error = %v", err)
	}
}

func validCanaryPlan(t *testing.T, now time.Time, root string) Plan {
	t.Helper()
	payload := []byte("reviewed project bytes")
	return Plan{SchemaVersion: PlanSchemaVersion, Synthetic: true,
		UserID: "21212121-2121-4212-8212-212121212121", ProjectID: "project_21212121",
		AssistantID: "assistant_21212121", PackageFingerprint: testCanaryFingerprint("1"),
		RuntimeBundleFingerprint: testCanaryFingerprint("2"), IssuedAt: now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour), ArtifactPolicy: ArtifactPolicy{MaxBytes: 1024,
			AllowedMediaTypes: []string{"application/json", "text/plain"}},
		Sandbox: agentrunner.SandboxSpec{RuntimeBundleFingerprint: testCanaryFingerprint("2"),
			PackageFingerprint: testCanaryFingerprint("1"), Image: "localhost/neo-agent-canary@sha256:" + strings.Repeat("3", 64),
			UID: 1000, GID: 1000, RootfsReadOnly: true, NoNewPrivileges: true, Capabilities: []string{},
			SeccompProfileFingerprint: testCanaryFingerprint("4"), NetworkMode: "none",
			WorkspaceSnapshotID: "workspace_snapshot_2121212121212121", WorkspaceFingerprint: testCanaryFingerprint("5"),
			Resources: agentrunner.ResourceLimits{CPUMillis: 100, MemoryMiB: 64, PIDs: 8,
				WallSeconds: 30, OutputBytes: 1024, ScratchBytes: 1 << 20}},
		Argv: []string{"/opt/neo/bin/broker-canary"}, LeaseSeconds: 30,
		Actions: []ActionPlan{
			{ID: ActionProjectRead, IdempotencyKey: "g21.2-broker-canary-project", ToolIdentity: "project.read",
				Capability: "project.read", Action: "read_file", Resource: "fixture.txt",
				Classification: agentbroker.ClassificationRead, Idempotent: true, Approval: agentbroker.ApprovalAutomatic,
				Arguments: json.RawMessage(`{"path":"fixture.txt"}`), TTLSeconds: 600,
				File: &FileReadPlan{Root: root, RelativePath: "fixture.txt", ExpectedBytes: int64(len(payload)),
					ExpectedFingerprint: contentFingerprint(payload), MaxBytes: 1024}},
			{ID: ActionWorkspaceRead, IdempotencyKey: "g21.2-broker-canary-workspace", ToolIdentity: "workspace.read",
				Capability: "workspace.read", Action: "read_file", Resource: "fixture.txt",
				Classification: agentbroker.ClassificationRead, Idempotent: true, Approval: agentbroker.ApprovalAutomatic,
				Arguments: json.RawMessage(`{"path":"fixture.txt"}`), TTLSeconds: 600,
				File: &FileReadPlan{Root: root, RelativePath: "fixture.txt", ExpectedBytes: int64(len(payload)),
					ExpectedFingerprint: contentFingerprint(payload), MaxBytes: 1024}},
			{ID: ActionMCPRead, IdempotencyKey: "g21.2-broker-canary-mcp", ToolIdentity: "mcp.read",
				Capability: "mcp.read", Action: "call", Resource: "manifest:fixture/lookup",
				Classification: agentbroker.ClassificationRead, Idempotent: true, Approval: agentbroker.ApprovalAutomatic,
				Arguments: json.RawMessage(`{"topic":"canary"}`), TTLSeconds: 600,
				MCP: &MCPReadPlan{ServerID: "fixture", ServerName: "Fixture", ToolName: "lookup",
					CommandArgv: []string{"/opt/mcp/fixture"}, ToolPolicy: map[string]string{"lookup": "read"}, MaxResultBytes: 4096}},
			{ID: ActionArtifact, IdempotencyKey: "g21.2-broker-canary-artifact", ToolIdentity: "artifact.publish",
				Capability: "artifact.publish", Action: "publish", Resource: "result.txt",
				Classification: agentbroker.ClassificationMutable, Approval: agentbroker.ApprovalAutomatic,
				Arguments: json.RawMessage(`{"mediaType":"text/plain","name":"result.txt"}`), TTLSeconds: 600,
				Artifact: &ArtifactPlan{Name: "result.txt", MediaType: "text/plain", SizeBytes: 8,
					Fingerprint: contentFingerprint([]byte("artifact"))}},
			{ID: ActionPossibleSend, IdempotencyKey: "g21.2-broker-canary-possible-send", ToolIdentity: "canary.possible_send",
				Capability: "mcp.read", Action: "call", Resource: "manifest:fixture/ack-loss",
				Classification: agentbroker.ClassificationRead, Idempotent: true, Approval: agentbroker.ApprovalAutomatic,
				Arguments: json.RawMessage(`{"probe":"ack-loss"}`), TTLSeconds: 600},
		}}
}

func mustBindings(t *testing.T, plan Plan, now time.Time) *Bindings {
	t.Helper()
	bindings, err := NewBindings(plan, testCanaryRunner, now)
	if err != nil {
		t.Fatal(err)
	}
	return bindings
}

func executionFor(binding ActionBinding) agentbroker.ExecutionRequest {
	return executionForPlan(binding.Plan)
}

func executionForPlan(action ActionPlan) agentbroker.ExecutionRequest {
	fingerprint, _ := agentbroker.ArgumentsFingerprint(action.Arguments)
	return agentbroker.ExecutionRequest{IntentID: "intent_0123456789abcdef",
		ToolIdentity: action.ToolIdentity, Capability: action.Capability, Action: action.Action,
		Resource: action.Resource, Arguments: action.Arguments, ArgumentsFingerprint: fingerprint,
		BaseRevision: action.BaseRevision}
}

func testCanaryFingerprint(character string) string { return "sha256:" + strings.Repeat(character, 64) }

type canaryMCPConnector struct {
	server     mcpclient.Server
	credential string
	session    mcpclient.Session
}

func (connector *canaryMCPConnector) Connect(_ context.Context, server mcpclient.Server, credential string) (mcpclient.Session, error) {
	connector.server, connector.credential = server, credential
	return connector.session, nil
}

type canaryMCPSession struct {
	tools   []mcpclient.Tool
	result  mcpclient.CallResult
	callErr error
	calls   int
}

func (session *canaryMCPSession) ListTools(context.Context) ([]mcpclient.Tool, error) {
	return append([]mcpclient.Tool(nil), session.tools...), nil
}
func (session *canaryMCPSession) CallTool(context.Context, string, map[string]any) (mcpclient.CallResult, error) {
	session.calls++
	return session.result, session.callErr
}
func (*canaryMCPSession) Close() error { return nil }

type canaryArtifactRepository struct {
	order     *[]string
	candidate agentbroker.ArtifactCandidate
}

func (repository *canaryArtifactRepository) AuthorizeArtifact(_ context.Context, candidate agentbroker.ArtifactCandidate) error {
	*repository.order = append(*repository.order, "authorize")
	repository.candidate = candidate
	return nil
}
func (repository *canaryArtifactRepository) AttachArtifact(_ context.Context, candidate agentbroker.ArtifactCandidate, _ string) error {
	*repository.order = append(*repository.order, "attach")
	repository.candidate = candidate
	return nil
}

type canaryArtifactObjects struct {
	order *[]string
	body  []byte
}

func (objects *canaryArtifactObjects) Put(_ context.Context, _ string, body io.Reader, _ int64, _ string) error {
	*objects.order = append(*objects.order, "put")
	objects.body, _ = io.ReadAll(body)
	return nil
}
func (*canaryArtifactObjects) Delete(context.Context, string) error { return nil }
