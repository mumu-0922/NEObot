package agentrunner

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	testRunID       = "run_0123456789abcdef"
	testStepID      = "step_0123456789abcdef"
	testAttemptID   = "attempt_0123456789abcdef"
	testRequestID   = "rpc_0123456789abcdef"
	testWorkspaceID = "workspace_snapshot_0123456789abcdef"
	testCaller      = "spiffe://neo-chat/backend"
	testRunner      = "neo-runner-primary"
)

func TestDecodeRequestRejectsAmbiguityVersionAndDrift(t *testing.T) {
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	body := mustEnvelope(t, MethodProbe, testRequestID, now, strings.Repeat("n", 32), ProbeRequest{RequiredFeatures: []string{"rootless_userns"}})
	if _, err := DecodeRequest(body, now, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]byte{
		bytes.Replace(body, []byte(`"method":"probe"`), []byte(`"method":"probe","method":"launch"`), 1),
		bytes.Replace(body, []byte(ProtocolVersion), []byte("neo.runner-rpc/v2"), 1),
		bytes.Replace(body, []byte(`"requiredFeatures":["rootless_userns"]`), []byte(`"requiredFeatures":["rootless_userns","rootless_userns"]`), 1),
		mustEnvelope(t, MethodProbe, testRequestID, now.Add(-time.Minute), strings.Repeat("n", 32), ProbeRequest{RequiredFeatures: []string{"rootless_userns"}}),
	} {
		if _, err := DecodeRequest(invalid, now, 15*time.Second); err == nil {
			t.Fatalf("invalid request accepted: %s", invalid)
		}
	}
}

func TestAuthorityBindsCallerAttemptTokenAndExpiry(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	verifier, _ := NewSignedAuthorityVerifier(publicKey)
	now := time.Now().UTC()
	attempt := testAttempt()
	claims := NewAuthorityClaims(testCaller, testRunner, MethodLaunch, testRequestID, strings.Repeat("n", 32), testFingerprint('a'), testFingerprint('6'), attempt, 4, now, now.Add(10*time.Second))
	ticket, err := SignAuthority(privateKey, claims)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifier.Verify(testCaller, testRunner, MethodLaunch, testRequestID, strings.Repeat("n", 32), testFingerprint('a'), testFingerprint('6'), attempt, ticket, now); err != nil {
		t.Fatal(err)
	}
	tampered := attempt
	tampered.LeaseToken = "lease_aaaaaaaaaaaaaaaaaaaaaaaa"
	if err := verifier.Verify(testCaller, testRunner, MethodLaunch, testRequestID, strings.Repeat("n", 32), testFingerprint('a'), testFingerprint('6'), tampered, ticket, now); !errors.Is(err, ErrLeaseStale) {
		t.Fatalf("token tamper=%v", err)
	}
	if err := verifier.Verify("spiffe://neo-chat/other", testRunner, MethodLaunch, testRequestID, strings.Repeat("n", 32), testFingerprint('a'), testFingerprint('6'), attempt, ticket, now); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("caller tamper=%v", err)
	}
	if err := verifier.Verify(testCaller, testRunner, MethodLaunch, testRequestID, strings.Repeat("n", 32), testFingerprint('a'), testFingerprint('6'), attempt, ticket, now.Add(11*time.Second)); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("expired=%v", err)
	}
}

func TestFileReplayLedgerSurvivesRestartAndRejectsMismatch(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "requests.jsonl")
	ledger, _ := NewFileReplayLedger(path)
	claim := ReplayClaim{CallerIdentity: testCaller, RequestID: testRequestID, Nonce: strings.Repeat("n", 32), Method: MethodProbe, RequestFingerprint: testFingerprint('1'), ExpiresAt: time.Now().Add(time.Hour)}
	if result, err := ledger.Claim(context.Background(), claim); err != nil || result.Replay {
		t.Fatalf("claim=%#v/%v", result, err)
	}
	if _, err := ledger.Claim(context.Background(), claim); !errors.Is(err, ErrReplayDetected) {
		t.Fatalf("inflight=%v", err)
	}
	response := []byte(`{"schemaVersion":"neo.runner-rpc/v1","method":"probe.result","requestId":"rpc_0123456789abcdef","sentAt":"2026-08-13T00:00:00Z","nonce":"nnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnn","body":{"ready":false}}`)
	if err := ledger.Complete(context.Background(), claim.CallerIdentity, claim.RequestID, claim.Nonce, response); err != nil {
		t.Fatal(err)
	}
	restarted, _ := NewFileReplayLedger(path)
	result, err := restarted.Claim(context.Background(), claim)
	if err != nil || !result.Replay || !bytes.Equal(result.Response, response) {
		t.Fatalf("restart replay=%#v/%v", result, err)
	}
	mismatch := claim
	mismatch.RequestFingerprint = testFingerprint('2')
	if _, err := restarted.Claim(context.Background(), mismatch); !errors.Is(err, ErrReplayDetected) {
		t.Fatalf("mismatch=%v", err)
	}
	restarted.now = func() time.Time { return claim.ExpiresAt.Add(time.Second) }
	if deleted, err := restarted.Compact(context.Background()); err != nil || deleted != 1 {
		t.Fatalf("compact=%d/%v", deleted, err)
	}
}

func TestWorkspaceMaterializerRejectsTraversalLinksSpecialAndCollision(t *testing.T) {
	for name, entries := range map[string][]tarFixture{
		"traversal": {{Name: "../escape", Body: []byte("x"), Type: tar.TypeReg}},
		"absolute":  {{Name: "/escape", Body: []byte("x"), Type: tar.TypeReg}},
		"symlink":   {{Name: "link", Link: "outside", Type: tar.TypeSymlink}},
		"hardlink":  {{Name: "hard", Link: "target", Type: tar.TypeLink}},
		"device":    {{Name: "dev", Type: tar.TypeChar}},
		"collision": {{Name: "Readme", Body: []byte("a"), Type: tar.TypeReg}, {Name: "README", Body: []byte("b"), Type: tar.TypeReg}},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			os.Chmod(root, 0o700)
			catalog, _ := NewWorkspaceCatalog(root)
			_, err := catalog.Register(context.Background(), testWorkspaceID, testFingerprint('1'), bytes.NewReader(makeTar(t, entries)))
			if err == nil {
				t.Fatal("unsafe archive accepted")
			}
			if names, _ := scanTreeNames(root); len(names) != 0 {
				t.Fatalf("residue=%v", names)
			}
		})
	}
}

func TestWorkspaceRegisterIsFingerprintBoundAndReadOnly(t *testing.T) {
	files := map[string][]byte{"README.md": []byte("hello"), "src/main.go": []byte("package main")}
	entries := []tarFixture{{Name: "README.md", Body: files["README.md"], Type: tar.TypeReg}, {Name: "src", Type: tar.TypeDir}, {Name: "src/main.go", Body: files["src/main.go"], Type: tar.TypeReg}}
	root := t.TempDir()
	os.Chmod(root, 0o700)
	catalog, _ := NewWorkspaceCatalog(root)
	t.Cleanup(func() { _ = catalog.Remove(testWorkspaceID, fingerprintWorkspaceFiles(files)) })
	workspace, err := catalog.Register(context.Background(), testWorkspaceID, fingerprintWorkspaceFiles(files), bytes.NewReader(makeTar(t, entries)))
	if err != nil {
		t.Fatal(err)
	}
	if workspace.FileCount != 2 || workspace.ByteCount != int64(len(files["README.md"])+len(files["src/main.go"])) {
		t.Fatalf("workspace=%#v", workspace)
	}
	if info, _ := os.Stat(filepath.Join(workspace.Path, "README.md")); info.Mode().Perm() != 0o400 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	if _, err := catalog.Resolve(testWorkspaceID, workspace.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(workspace.Path, "README.md"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Resolve(testWorkspaceID, workspace.Fingerprint); !errors.Is(err, ErrSnapshotMismatch) {
		t.Fatalf("mutated workspace=%v", err)
	}
	if err := os.Chmod(filepath.Join(workspace.Path, "README.md"), 0o400); err != nil {
		t.Fatal(err)
	}
	otherRoot := t.TempDir()
	os.Chmod(otherRoot, 0o700)
	other, _ := NewWorkspaceCatalog(otherRoot)
	if _, err := other.Register(context.Background(), testWorkspaceID, testFingerprint('2'), bytes.NewReader(makeTar(t, entries))); !errors.Is(err, ErrSnapshotMismatch) {
		t.Fatalf("mismatch=%v", err)
	}
}

func TestArtifactBrokerFencesAttemptAndCleansPartial(t *testing.T) {
	root := t.TempDir()
	os.Chmod(root, 0o700)
	broker, _ := NewArtifactBroker(root)
	attempt := testAttempt().Identity()
	_ = broker.AuthorizeAttempt(attempt, 4)
	upload, err := broker.Begin(attempt, ArtifactMetadata{Name: "report.json", MediaType: "application/json", Size: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := upload.Write([]byte("{}")); err != nil {
		t.Fatal(err)
	}
	stale := attempt
	stale.LeaseGeneration++
	if _, err := upload.Finalize(stale); !errors.Is(err, ErrLeaseStale) {
		t.Fatalf("stale finalize=%v", err)
	}
	if names, _ := scanTreeNames(root); len(names) > 1 {
		t.Fatalf("partial residue=%v", names)
	}
	upload, _ = broker.Begin(attempt, ArtifactMetadata{Name: "report.json", MediaType: "application/json", Size: 2})
	_, _ = upload.Write([]byte("{}"))
	receipt, err := upload.Finalize(attempt)
	if err != nil || receipt.Fingerprint != testFingerprintBytes([]byte("{}")) {
		t.Fatalf("receipt=%#v/%v", receipt, err)
	}
	if _, err := broker.Begin(attempt, ArtifactMetadata{Name: "overflow.txt", MediaType: "text/plain", Size: 3}); !errors.Is(err, ErrLeaseStale) {
		t.Fatalf("aggregate output overflow=%v", err)
	}
}

func TestArtifactUnixIntakeIsFramedBoundedAndFenced(t *testing.T) {
	root := t.TempDir()
	os.Chmod(root, 0o700)
	quarantine := filepath.Join(root, "quarantine")
	socketDirectory := filepath.Join(root, "broker")
	os.Mkdir(quarantine, 0o700)
	os.Mkdir(socketDirectory, 0o711)
	broker, _ := NewArtifactBroker(quarantine)
	attempt := testAttempt().Identity()
	_ = broker.AuthorizeAttempt(attempt, 4)
	current := true
	intake, err := StartArtifactIntake(filepath.Join(socketDirectory, "artifact.sock"), broker, attempt,
		func(candidate AttemptIdentity) bool { return current && candidate == attempt })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = intake.Close() })
	receipt, err := writeArtifactFrame(context.Background(), filepath.Join(socketDirectory, "artifact.sock"),
		artifactFrame{Name: "report.json", MediaType: "application/json", Size: 2}, []byte("{}"))
	if err != nil || receipt.Attempt != attempt || receipt.Fingerprint != testFingerprintBytes([]byte("{}")) {
		t.Fatalf("receipt=%#v/%v", receipt, err)
	}
	current = false
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := writeArtifactFrame(ctx, filepath.Join(socketDirectory, "artifact.sock"),
		artifactFrame{Name: "stale.txt", MediaType: "text/plain", Size: 1}, []byte("x")); err == nil {
		t.Fatal("stale artifact intake accepted")
	}
}

func TestServiceLaunchReplayCancelAndProbeFence(t *testing.T) {
	fixture := newServiceFixture(t, true)
	request := fixture.launchRequest(t)
	response, err := fixture.service.Handle(context.Background(), testCaller, request)
	if err != nil {
		t.Fatal(err)
	}
	launch := response.Body.(LaunchResult)
	if !launch.Accepted || len(fixture.driver.Plans) != 1 {
		t.Fatalf("launch=%#v plans=%d", launch, len(fixture.driver.Plans))
	}
	replayed, err := fixture.service.Handle(context.Background(), testCaller, request)
	if err != nil || replayed.Body.(LaunchResult).SandboxID != launch.SandboxID || len(fixture.driver.Plans) != 1 {
		t.Fatalf("replay=%#v/%v", replayed, err)
	}
	cancel := fixture.cancelRequest(t, request.Launch, MethodCancel, "kill")
	cancelResponse, err := fixture.service.Handle(context.Background(), testCaller, cancel)
	if err != nil || cancelResponse.Body.(CancelResult).ObservedTerminal != "killed" {
		t.Fatalf("cancel=%#v/%v", cancelResponse, err)
	}
	if _, ok := fixture.driver.Sandboxes[strings.Repeat("a", 64)]; ok {
		t.Fatal("sandbox not removed")
	}
	unready := newServiceFixture(t, false)
	request = unready.launchRequest(t)
	if _, err := unready.service.Handle(context.Background(), testCaller, request); !errors.Is(err, ErrIsolationUnavailable) || len(unready.driver.Plans) != 0 {
		t.Fatalf("unready=%v plans=%d", err, len(unready.driver.Plans))
	}
}

func TestServiceReconcileRPCReapsUnexpectedSandboxAndReplays(t *testing.T) {
	fixture := newServiceFixture(t, true)
	launch := fixture.launchRequest(t)
	if _, err := fixture.service.Handle(context.Background(), testCaller, launch); err != nil {
		t.Fatal(err)
	}
	now := fixture.service.now()
	raw := mustEnvelope(t, MethodReconcile, "rpc_aaaaaaaaaaaaaaaa", now, strings.Repeat("r", 32),
		ReconcileRequest{RunnerID: testRunner, Expected: []SandboxDescriptor{}})
	request, err := DecodeRequest(raw, now, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	response, err := fixture.service.Handle(context.Background(), testCaller, request)
	if err != nil || response.Body.(ReconcileResult).Cleaned != 1 || len(fixture.driver.Sandboxes) != 0 {
		t.Fatalf("reconcile=%#v/%v", response, err)
	}
	replayed, err := fixture.service.Handle(context.Background(), testCaller, request)
	if err != nil || replayed.Body.(ReconcileResult).Cleaned != 1 {
		t.Fatalf("reconcile replay=%#v/%v", replayed, err)
	}
}

func TestPrepareCommitStrictRelayVerifiesAuthorityAndReplaysLocally(t *testing.T) {
	fixture := newServiceFixture(t, true)
	relay := &recordingBrokerRelay{}
	fixture.service.WithBrokerRelay(relay)
	now := time.Date(2026, 8, 14, 2, 0, 0, 0, time.UTC)
	fixture.service.now = func() time.Time { return now }
	attempt := testAttempt()
	arguments := json.RawMessage(`{"path":"project/a"}`)
	prepareBody := PrepareRequest{Attempt: attempt, SnapshotFingerprint: testFingerprint('6'),
		GrantID: "grant_0123456789abcdef", GrantFingerprint: testFingerprint('5'),
		RegistryFingerprint: testFingerprint('8'), ToolIdentity: "workspace_read",
		Capability: "workspace.read", Action: "read", Resource: "project/a",
		Arguments: arguments, ArgumentsFingerprint: fingerprintBytes("neo-effect-arguments-v1", arguments),
		BaseRevision: "rev_01234567", TTLSeconds: 600}
	prepareBody.Authority = signedPrepareAuthority(t, fixture.private, "rpc_aaaaaaaaaaaaaaaa",
		strings.Repeat("p", 32), now, prepareBody)
	prepareRaw := mustEnvelope(t, MethodPrepare, "rpc_aaaaaaaaaaaaaaaa", now, strings.Repeat("p", 32), prepareBody)
	prepare, err := DecodeRequest(prepareRaw, now, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	response, err := fixture.service.Handle(context.Background(), testCaller, prepare)
	if err != nil || !response.Body.(PrepareResult).Prepared || relay.prepares != 1 {
		t.Fatalf("Prepare = %#v, %v, relays=%d", response, err, relay.prepares)
	}
	if _, err := fixture.service.Handle(context.Background(), testCaller, prepare); err != nil || relay.prepares != 1 {
		t.Fatalf("Prepare replay = %v, relays=%d", err, relay.prepares)
	}

	commitBody := CommitRequest{Attempt: attempt, SnapshotFingerprint: testFingerprint('6'),
		GrantFingerprint: testFingerprint('5'), RegistryFingerprint: testFingerprint('8'),
		IntentID: "intent_0123456789abcdef", IntentFingerprint: testFingerprint('a'),
		ApprovalID: "approval_0123456789abcdef", IdempotencyKey: "commit_0123456789abcdefghijklmn"}
	commitBody.Authority = signedCommitAuthority(t, fixture.private, "rpc_bbbbbbbbbbbbbbbb",
		strings.Repeat("m", 32), now, commitBody)
	commitRaw := mustEnvelope(t, MethodCommit, "rpc_bbbbbbbbbbbbbbbb", now, strings.Repeat("m", 32), commitBody)
	commit, err := DecodeRequest(commitRaw, now, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	response, err = fixture.service.Handle(context.Background(), testCaller, commit)
	if err != nil || response.Body.(CommitResult).Outcome != "committed" || relay.commits != 1 {
		t.Fatalf("Commit = %#v, %v, relays=%d", response, err, relay.commits)
	}

	tampered := bytes.Replace(prepareRaw, []byte(`"resource":"project/a"`), []byte(`"resource":"project/b"`), 1)
	request, err := DecodeRequest(tampered, now, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.Handle(context.Background(), testCaller, request); !errors.Is(err, ErrReplayDetected) || relay.prepares != 1 {
		t.Fatalf("tampered Prepare = %v, relays=%d", err, relay.prepares)
	}
}

func TestPrepareRejectsDuplicateMalformedAndFingerprintMismatchBeforeRelay(t *testing.T) {
	fixture := newServiceFixture(t, true)
	relay := &recordingBrokerRelay{}
	fixture.service.WithBrokerRelay(relay)
	now := time.Date(2026, 8, 14, 2, 0, 0, 0, time.UTC)
	arguments := json.RawMessage(`{"path":"project/a"}`)
	body := PrepareRequest{Attempt: testAttempt(), SnapshotFingerprint: testFingerprint('6'),
		GrantID: "grant_0123456789abcdef", GrantFingerprint: testFingerprint('5'), RegistryFingerprint: testFingerprint('8'),
		ToolIdentity: "workspace_read", Capability: "workspace.read", Action: "read", Resource: "project/a",
		Arguments: arguments, ArgumentsFingerprint: testFingerprint('f'), TTLSeconds: 600}
	raw := mustEnvelope(t, MethodPrepare, "rpc_aaaaaaaaaaaaaaaa", now, strings.Repeat("p", 32), body)
	for _, invalid := range [][]byte{
		raw,
		bytes.Replace(raw, []byte(`"ttlSeconds":600`), []byte(`"ttlSeconds":600,"ttlSeconds":601`), 1),
		bytes.Replace(raw, []byte(`"resource":"project/a"`), []byte(`"resource":"project/a","unknown":true`), 1),
	} {
		if _, err := DecodeRequest(invalid, now, 15*time.Second); err == nil {
			t.Fatalf("invalid Prepare accepted: %s", invalid)
		}
	}
	if relay.prepares != 0 {
		t.Fatalf("invalid requests reached relay %d times", relay.prepares)
	}
}

func signedRequestAuthority(t *testing.T, private ed25519.PrivateKey, method, requestID, nonce, snapshot string,
	attempt AttemptRef, now time.Time, body any) AuthorityTicket {
	t.Helper()
	claims := NewAuthorityClaims(testCaller, testRunner, method, requestID, nonce,
		AuthorityRequestFingerprint(method, body), snapshot, attempt, 0, now, now.Add(10*time.Second))
	ticket, err := SignAuthority(private, claims)
	if err != nil {
		t.Fatal(err)
	}
	return ticket
}

func signedPrepareAuthority(t *testing.T, private ed25519.PrivateKey, requestID, nonce string, now time.Time, body PrepareRequest) AuthorityTicket {
	t.Helper()
	unsigned := body
	unsigned.Authority = AuthorityTicket{}
	return signedRequestAuthority(t, private, MethodPrepare, requestID, nonce, body.SnapshotFingerprint, body.Attempt, now, unsigned)
}

func signedCommitAuthority(t *testing.T, private ed25519.PrivateKey, requestID, nonce string, now time.Time, body CommitRequest) AuthorityTicket {
	t.Helper()
	unsigned := body
	unsigned.Authority = AuthorityTicket{}
	return signedRequestAuthority(t, private, MethodCommit, requestID, nonce, body.SnapshotFingerprint, body.Attempt, now, unsigned)
}

type recordingBrokerRelay struct{ prepares, commits int }

func (relay *recordingBrokerRelay) Prepare(context.Context, PrepareRequest) (PrepareResult, error) {
	relay.prepares++
	expiresAt := time.Now().Add(time.Minute)
	return PrepareResult{Prepared: true, IntentID: "intent_0123456789abcdef", IntentFingerprint: testFingerprint('a'), IdempotencyKey: "commit_0123456789abcdefghijklmn", Approval: "once", ExpiresAt: &expiresAt}, nil
}
func (relay *recordingBrokerRelay) Commit(_ context.Context, input CommitRequest) (CommitResult, error) {
	relay.commits++
	return CommitResult{Outcome: "committed", IdempotencyKey: input.IdempotencyKey, ReceiptFingerprint: testFingerprint('b')}, nil
}

func TestPrepareCommitRelayFailuresUseSchemaShapedBodies(t *testing.T) {
	fixture := newServiceFixture(t, true)
	fixture.service.WithBrokerRelay(failingBrokerRelay{})
	now := time.Date(2026, 8, 14, 2, 0, 0, 0, time.UTC)
	fixture.service.now = func() time.Time { return now }
	arguments := json.RawMessage(`{"path":"project/a"}`)
	prepareBody := PrepareRequest{Attempt: testAttempt(), SnapshotFingerprint: testFingerprint('6'),
		GrantID: "grant_0123456789abcdef", GrantFingerprint: testFingerprint('5'),
		RegistryFingerprint: testFingerprint('8'), ToolIdentity: "workspace_read",
		Capability: "workspace.read", Action: "read", Resource: "project/a", Arguments: arguments,
		ArgumentsFingerprint: fingerprintBytes("neo-effect-arguments-v1", arguments), TTLSeconds: 600}
	prepareBody.Authority = signedPrepareAuthority(t, fixture.private, "rpc_cccccccccccccccc", strings.Repeat("q", 32), now, prepareBody)
	prepare, err := DecodeRequest(mustEnvelope(t, MethodPrepare, "rpc_cccccccccccccccc", now,
		strings.Repeat("q", 32), prepareBody), now, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	response, err := fixture.service.Handle(context.Background(), testCaller, prepare)
	prepareResult := response.Body.(PrepareResult)
	if !errors.Is(err, ErrRuntimeUnavailable) || prepareResult.Prepared ||
		prepareResult.Error == nil || prepareResult.Error.Code != ErrorRuntimeUnavailable {
		t.Fatalf("Prepare failure = %#v, %v", prepareResult, err)
	}
	encoded, _ := json.Marshal(response)
	if _, err := decodeReplayResponse(MethodPrepare, encoded); err != nil {
		t.Fatalf("Prepare response shape = %s, %v", encoded, err)
	}

	commitBody := CommitRequest{Attempt: testAttempt(), SnapshotFingerprint: testFingerprint('6'),
		GrantFingerprint: testFingerprint('5'), RegistryFingerprint: testFingerprint('8'),
		IntentID: "intent_0123456789abcdef", IntentFingerprint: testFingerprint('a'),
		ApprovalID: "approval_0123456789abcdef", IdempotencyKey: "commit_0123456789abcdefghijklmn"}
	commitBody.Authority = signedCommitAuthority(t, fixture.private, "rpc_dddddddddddddddd", strings.Repeat("r", 32), now, commitBody)
	commit, err := DecodeRequest(mustEnvelope(t, MethodCommit, "rpc_dddddddddddddddd", now,
		strings.Repeat("r", 32), commitBody), now, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	response, err = fixture.service.Handle(context.Background(), testCaller, commit)
	commitResult := response.Body.(CommitResult)
	if !errors.Is(err, ErrOutcomeUnknown) || commitResult.Outcome != "outcome_unknown" ||
		commitResult.IdempotencyKey != commitBody.IdempotencyKey || commitResult.Error == nil ||
		commitResult.Error.Code != ErrorOutcomeUnknown {
		t.Fatalf("Commit failure = %#v, %v", commitResult, err)
	}
}

type failingBrokerRelay struct{}

func (failingBrokerRelay) Prepare(context.Context, PrepareRequest) (PrepareResult, error) {
	return PrepareResult{}, ErrRuntimeUnavailable
}
func (failingBrokerRelay) Commit(context.Context, CommitRequest) (CommitResult, error) {
	return CommitResult{}, ErrOutcomeUnknown
}

func TestServiceFailureReplayPreservesOperationError(t *testing.T) {
	fixture := newServiceFixture(t, false)
	request := fixture.launchRequest(t)
	if _, err := fixture.service.Handle(context.Background(), testCaller, request); !errors.Is(err, ErrIsolationUnavailable) {
		t.Fatalf("first failure=%v", err)
	}
	if _, err := fixture.service.Handle(context.Background(), testCaller, request); !errors.Is(err, ErrIsolationUnavailable) {
		t.Fatalf("replayed failure=%v", err)
	}
}

func TestHTTPHandlerRequiresVerifiedClientAndStrictJSON(t *testing.T) {
	fixture := newServiceFixture(t, true)
	handler, _ := NewHTTPHandler(fixture.service, 15*time.Second, testCaller)
	now := fixture.service.now()
	body := mustEnvelope(t, MethodProbe, testRequestID, now, strings.Repeat("n", 32), ProbeRequest{RequiredFeatures: []string{"rootless_userns"}})
	request := httptest.NewRequest(http.MethodPost, RPCPath(), bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("plain=%d", recorder.Code)
	}
	request = httptest.NewRequest(http.MethodPost, RPCPath(), bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.TLS = &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{{Subject: pkix.Name{CommonName: testCaller}}}}}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("mtls=%d %s", recorder.Code, recorder.Body.String())
	}
}

func TestRPCClientUsesTLS13MutualAuthAndBindsResponse(t *testing.T) {
	fixture := newServiceFixture(t, true)
	serverCertificate, clientCertificate, caPool, files := rpcTLSFixture(t)
	handler, _ := NewHTTPHandler(fixture.service, 15*time.Second, testCaller)
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13,
		Certificates: []tls.Certificate{serverCertificate}, ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs: caPool}
	server.StartTLS()
	defer server.Close()
	files.CertificateFile = clientCertificate.certFile
	files.KeyFile = clientCertificate.keyFile
	endpoint := server.URL + RPCPath()
	client, err := NewRPCClient(endpoint, files, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	now := fixture.service.now()
	raw := mustEnvelope(t, MethodProbe, testRequestID, now, strings.Repeat("p", 32), ProbeRequest{RequiredFeatures: []string{"rootless_userns"}})
	request, _ := DecodeRequest(raw, now, 15*time.Second)
	response, err := client.Call(context.Background(), request)
	if err != nil || !response.Body.(ProbeResult).Ready {
		t.Fatalf("response=%#v/%v", response, err)
	}
	files.ServerName = "wrong-runner"
	wrong, _ := NewRPCClient(endpoint, files, 10*time.Second)
	if _, err := wrong.Call(context.Background(), request); !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("server identity mismatch=%v", err)
	}
}

func TestRPCClientRejectsMissingOrInvalidPort(t *testing.T) {
	_, _, _, files := rpcTLSFixture(t)
	for _, endpoint := range []string{
		"https://10.0.0.8/internal/neo-runner/v1/rpc",
		"https://10.0.0.8:0/internal/neo-runner/v1/rpc",
		"https://10.0.0.8:https/internal/neo-runner/v1/rpc",
	} {
		if _, err := NewRPCClient(endpoint, files, 10*time.Second); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("NewRPCClient(%q) error = %v", endpoint, err)
		}
	}
}

func TestPodmanDriverCreateIntentContainsHardIsolationAndNoShell(t *testing.T) {
	runner := &recordingCommands{outputs: [][]byte{[]byte(strings.Repeat("a", 64) + "\n")}}
	driver, _ := NewPodmanDriver("/usr/bin/podman", runner)
	plan := testLaunchPlan(t)
	item, err := driver.Create(context.Background(), plan)
	if err != nil || item.ContainerID != strings.Repeat("a", 64) {
		t.Fatal(err)
	}
	args := strings.Join(runner.args[0], " ")
	for _, required := range []string{"create", "--runtime crun", "--userns auto:size=65536", "--read-only", "--cap-drop=all", "no-new-privileges", "seccomp=", "--network=none", "--pid=private", "--ipc=private", "--pids-limit", "dst=/workspace,ro=true", "dst=/run/neo-broker,rw=true", "--tmpfs /scratch:rw,noexec,nosuid,nodev,size=67108864,mode=0700"} {
		if !strings.Contains(args, required) {
			t.Fatalf("missing %q in %s", required, args)
		}
	}
	for _, forbidden := range []string{" run ", "--privileged", "--network=host", "/var/run/docker.sock", "sh -c"} {
		if strings.Contains(" "+args+" ", forbidden) {
			t.Fatalf("forbidden %q in %s", forbidden, args)
		}
	}
}

func TestPodmanInspectRequiresExactFrozenIsolation(t *testing.T) {
	plan := testLaunchPlan(t)
	containerID := strings.Repeat("a", 64)
	base := podmanInspect{ID: containerID, Name: plan.ContainerName, ImageName: plan.Sandbox.Image,
		OCIRuntime: "crun", Mounts: testableIsolationForPlan(plan).Mounts}
	base.State.Status = "created"
	base.Config.User = "10001:10001"
	base.Config.Labels = map[string]string{"neo.runner.managed": "true", "neo.runner.sandbox": plan.SandboxID,
		"neo.runner.run": plan.Attempt.RunID, "neo.runner.step": plan.Attempt.StepID,
		"neo.runner.attempt": plan.Attempt.AttemptID, "neo.runner.generation": "1",
		"neo.runner.snapshot": plan.SnapshotFingerprint, "neo.runner.spec": plan.SpecFingerprint,
		"neo.runner.probe": plan.ProbeFingerprint}
	isolation := testableIsolationForPlan(plan)
	base.HostConfig.ReadonlyRootfs = isolation.ReadonlyRootfs
	base.HostConfig.NetworkMode = isolation.NetworkMode
	base.HostConfig.PidMode = isolation.PIDMode
	base.HostConfig.IpcMode = isolation.IPCMode
	base.HostConfig.UTSMode = isolation.UTSMode
	base.HostConfig.SecurityOpt = isolation.SecurityOpt
	base.HostConfig.PidsLimit = isolation.PIDsLimit
	base.HostConfig.Memory = isolation.Memory
	base.HostConfig.MemorySwap = isolation.MemorySwap
	base.HostConfig.NanoCpus = isolation.NanoCPUs
	base.HostConfig.Cgroups = isolation.Cgroups
	base.HostConfig.CgroupsMode = isolation.CgroupsMode
	base.HostConfig.CgroupManager = isolation.CgroupManager
	base.HostConfig.CgroupParent = isolation.CgroupParent
	base.HostConfig.UsernsMode = isolation.UsernsMode
	base.HostConfig.Tmpfs = isolation.Tmpfs
	base.HostConfig.LogConfig.Type = isolation.LogDriver
	base.IDMappings.UIDMap = isolation.UIDMap
	base.IDMappings.GIDMap = isolation.GIDMap
	encode := func(value podmanInspect) []byte {
		body, _ := json.Marshal([]podmanInspect{value})
		return body
	}
	runner := &recordingCommands{outputs: [][]byte{encode(base)}}
	driver, _ := NewPodmanDriver("/usr/bin/podman", runner)
	item, err := driver.Inspect(context.Background(), containerID)
	if err != nil || !matchesLaunchPlan(item, plan) {
		t.Fatalf("valid inspect=%#v/%v", item, err)
	}
	for name, mutate := range map[string]func(*podmanInspect){
		"cpu":        func(value *podmanInspect) { value.HostConfig.NanoCpus++ },
		"userns":     func(value *podmanInspect) { value.HostConfig.UsernsMode = "host" },
		"seccomp":    func(value *podmanInspect) { value.HostConfig.SecurityOpt[1] = "seccomp=/tmp/drift" },
		"scratch":    func(value *podmanInspect) { value.HostConfig.Tmpfs["/scratch"] = "rw,size=1" },
		"mount":      func(value *podmanInspect) { value.Mounts[0].Source = "/tmp/drift" },
		"capability": func(value *podmanInspect) { value.EffectiveCaps = []string{"CAP_SYS_ADMIN"} },
	} {
		t.Run(name, func(t *testing.T) {
			copyValue := base
			copyValue.HostConfig.SecurityOpt = append([]string(nil), base.HostConfig.SecurityOpt...)
			copyValue.HostConfig.Tmpfs = map[string]string{"/scratch": base.HostConfig.Tmpfs["/scratch"]}
			copyValue.Mounts = append([]DriverMount(nil), base.Mounts...)
			mutate(&copyValue)
			runner := &recordingCommands{outputs: [][]byte{encode(copyValue)}}
			driver, _ := NewPodmanDriver("/usr/bin/podman", runner)
			item, inspectErr := driver.Inspect(context.Background(), containerID)
			if inspectErr == nil && matchesLaunchPlan(item, plan) {
				t.Fatal("drifted inspect accepted")
			}
		})
	}
}

type serviceFixture struct {
	service   *Service
	driver    *MemoryDriver
	private   ed25519.PrivateKey
	workspace Workspace
}

func newServiceFixture(t *testing.T, ready bool) serviceFixture {
	t.Helper()
	root, err := os.MkdirTemp("", "neo-runner-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	for _, name := range []string{"sandboxes", "scratch", "workspaces", "artifacts"} {
		path := filepath.Join(root, name)
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string][]byte{"README.md": []byte("hello")}
	catalog, _ := NewWorkspaceCatalog(filepath.Join(root, "workspaces"))
	workspace, err := catalog.Register(context.Background(), testWorkspaceID, fingerprintWorkspaceFiles(files), bytes.NewReader(makeTar(t, []tarFixture{{Name: "README.md", Body: files["README.md"], Type: tar.TypeReg}})))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = catalog.Remove(workspace.ID, workspace.Fingerprint) })
	broker, _ := NewArtifactBroker(filepath.Join(root, "artifacts"))
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	authority, _ := NewSignedAuthorityVerifier(public)
	driver := NewMemoryDriver()
	evidence := ProbeEvidence{Ready: ready, RunnerID: testRunner, RunnerVersion: "test", Runtime: "podman", RuntimeVersion: "6.1.0", Features: []string{"rootless_userns"}, ProbeFingerprint: testFingerprint('9')}
	probeErr := error(nil)
	if !ready {
		probeErr = ErrIsolationUnavailable
	}
	service, err := NewService(ServiceConfig{RunnerID: testRunner, StateRoot: root, SeccompPath: "/etc/neo-runner/seccomp.json", SeccompFingerprint: testFingerprint('7'), ReplayTTL: time.Hour, HeartbeatExtension: 30 * time.Second}, StaticProbe{Evidence: evidence, Err: probeErr}, authority, NewMemoryReplayLedger(), driver, catalog, broker)
	if err != nil {
		t.Fatal(err)
	}
	return serviceFixture{service: service, driver: driver, private: private, workspace: workspace}
}
func (f serviceFixture) launchRequest(t *testing.T) Request {
	t.Helper()
	now := time.Now().UTC()
	f.service.now = func() time.Time { return now }
	attempt := testAttempt()
	body := LaunchRequest{Attempt: attempt, Lineage: RunLineage{RootRunID: attempt.RunID, Depth: 0}, GrantID: "grant_0123456789abcdef", GrantFingerprint: testFingerprint('5'), SnapshotFingerprint: testFingerprint('6'), Sandbox: SandboxSpec{RuntimeBundleFingerprint: testFingerprint('2'), PackageFingerprint: testFingerprint('1'), Image: "registry.example/neo/audit@" + testFingerprint('4'), UID: 10001, GID: 10001, RootfsReadOnly: true, NoNewPrivileges: true, Capabilities: []string{}, SeccompProfileFingerprint: testFingerprint('7'), NetworkMode: "none", WorkspaceSnapshotID: f.workspace.ID, WorkspaceFingerprint: f.workspace.Fingerprint, Resources: ResourceLimits{CPUMillis: 1000, MemoryMiB: 512, PIDs: 64, WallSeconds: 300, OutputBytes: 2 << 20, ScratchBytes: 64 << 20}}, ToolRegistry: ToolRegistry{Depth: 0, Tools: []string{"workspace.read"}, RegistryFingerprint: testFingerprint('8')}, Argv: []string{"/opt/neo/bin/audit"}}
	claims := NewAuthorityClaims(testCaller, testRunner, MethodLaunch, testRequestID, strings.Repeat("n", 32), AuthorityRequestFingerprint(MethodLaunch, body), testFingerprint('6'), attempt, 0, now, now.Add(10*time.Second))
	body.Authority, _ = SignAuthority(f.private, claims)
	raw := mustEnvelope(t, MethodLaunch, testRequestID, now, strings.Repeat("n", 32), body)
	request, err := DecodeRequest(raw, now, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return request
}
func (f serviceFixture) cancelRequest(t *testing.T, launch *LaunchRequest, method, mode string) Request {
	t.Helper()
	now := f.service.now()
	requestID := "rpc_fedcba9876543210"
	claims := NewAuthorityClaims(testCaller, testRunner, method, requestID, strings.Repeat("c", 32), AuthorityRequestFingerprint(method, CancelRequest{Attempt: launch.Attempt, Mode: mode, ReasonCode: "operator_kill"}), launch.SnapshotFingerprint, launch.Attempt, 0, now, now.Add(10*time.Second))
	ticket, _ := SignAuthority(f.private, claims)
	body := CancelRequest{Attempt: launch.Attempt, Authority: ticket, Mode: mode, ReasonCode: "operator_kill"}
	raw := mustEnvelope(t, method, requestID, now, strings.Repeat("c", 32), body)
	request, err := DecodeRequest(raw, now, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestLaunchRejectsForgedChildLineageBeforeDriverCreate(t *testing.T) {
	fixture := newServiceFixture(t, true)
	for _, mutate := range []func(*LaunchRequest){
		func(value *LaunchRequest) { value.Lineage = RunLineage{RootRunID: value.Attempt.RunID, Depth: 1} },
		func(value *LaunchRequest) {
			value.Lineage = RunLineage{RootRunID: value.Attempt.RunID, Depth: 0}
			value.ToolRegistry.Depth = 1
		},
		func(value *LaunchRequest) {
			value.Lineage = RunLineage{RootRunID: "run_fedcba9876543210", ParentRunID: "run_fedcba9876543210", Depth: 2}
			value.ToolRegistry.Depth = 1
		},
	} {
		request := fixture.launchRequest(t)
		mutate(request.Launch)
		body, _ := json.Marshal(request.Launch)
		raw := mustEnvelope(t, MethodLaunch, request.RequestID, request.SentAt, request.Nonce, json.RawMessage(body))
		if _, err := DecodeRequest(raw, request.SentAt, 15*time.Second); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("forged lineage error = %v", err)
		}
	}
	if len(fixture.driver.Plans) != 0 {
		t.Fatalf("driver received %d forged launches", len(fixture.driver.Plans))
	}
}

type tarFixture struct {
	Name string
	Body []byte
	Link string
	Type byte
}

func makeTar(t *testing.T, entries []tarFixture) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for _, entry := range entries {
		size := int64(len(entry.Body))
		if entry.Type == tar.TypeDir || entry.Type == tar.TypeSymlink || entry.Type == tar.TypeLink || entry.Type == tar.TypeChar {
			size = 0
		}
		header := &tar.Header{Name: entry.Name, Mode: 0o600, Typeflag: entry.Type, Size: size, Linkname: entry.Link}
		if entry.Type == tar.TypeDir {
			header.Mode = 0o700
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if size > 0 {
			if _, err := writer.Write(entry.Body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
func mustEnvelope(t *testing.T, method, requestID string, sent time.Time, nonce string, body any) []byte {
	t.Helper()
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	value := Envelope{SchemaVersion: ProtocolVersion, Method: method, RequestID: requestID, SentAt: sent, Nonce: nonce, Body: bodyJSON}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
func testAttempt() AttemptRef {
	return AttemptRef{RunID: testRunID, StepID: testStepID, AttemptID: testAttemptID, LeaseGeneration: 1, LeaseOwner: testRunner, LeaseToken: "lease_0123456789abcdef01234567"}
}
func testFingerprint(character byte) string { return "sha256:" + strings.Repeat(string(character), 64) }
func testFingerprintBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func testLaunchPlan(t *testing.T) LaunchPlan {
	t.Helper()
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	scratch := filepath.Join(root, "scratch")
	broker := filepath.Join(scratch, "broker")
	os.Mkdir(workspace, 0o500)
	os.Mkdir(scratch, 0o700)
	os.Mkdir(broker, 0o711)
	return LaunchPlan{SandboxID: "sandbox_0123456789abcdef", ContainerName: "neo-test", Attempt: testAttempt().Identity(), SnapshotFingerprint: testFingerprint('6'), SpecFingerprint: testFingerprint('3'), ProbeFingerprint: testFingerprint('9'), Sandbox: SandboxSpec{RuntimeBundleFingerprint: testFingerprint('2'), PackageFingerprint: testFingerprint('1'), Image: "registry.example/neo/audit@" + testFingerprint('4'), UID: 10001, GID: 10001, RootfsReadOnly: true, NoNewPrivileges: true, Capabilities: []string{}, SeccompProfileFingerprint: testFingerprint('7'), NetworkMode: "none", WorkspaceSnapshotID: testWorkspaceID, WorkspaceFingerprint: testFingerprint('3'), Resources: ResourceLimits{CPUMillis: 1000, MemoryMiB: 512, PIDs: 64, WallSeconds: 300, OutputBytes: 2 << 20, ScratchBytes: 64 << 20}}, WorkspacePath: workspace, ScratchPath: scratch, BrokerPath: broker, SeccompPath: "/etc/neo-runner/seccomp.json", Argv: []string{"/opt/neo/bin/audit"}}
}

type recordingCommands struct {
	args    [][]string
	outputs [][]byte
}

type tlsKeyPairFiles struct {
	certFile string
	keyFile  string
}

func rpcTLSFixture(t *testing.T) (tls.Certificate, tlsKeyPairFiles, *x509.CertPool, ClientTLSFiles) {
	t.Helper()
	now := time.Now()
	caPublic, caPrivate, _ := ed25519.GenerateKey(rand.Reader)
	caTemplate := &x509.Certificate{SerialNumber: bigOne(), Subject: pkix.Name{CommonName: "neo-runner-test-ca"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caPublic, caPrivate)
	caCertificate, _ := x509.ParseCertificate(caDER)
	root := t.TempDir()
	caFile := filepath.Join(root, "ca.pem")
	os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o644)
	serverFiles, serverCertificate := signedTLSIdentity(t, root, "server", "neo-runner-primary", true, caCertificate, caPrivate)
	clientFiles, _ := signedTLSIdentity(t, root, "client", testCaller, false, caCertificate, caPrivate)
	pool := x509.NewCertPool()
	pool.AddCert(caCertificate)
	return serverCertificate, clientFiles, pool, ClientTLSFiles{CertificateFile: serverFiles.certFile,
		KeyFile: serverFiles.keyFile, ServerCAFile: caFile, ServerName: "neo-runner-primary",
		ClientIdentity: testCaller}
}

func signedTLSIdentity(t *testing.T, root, prefix, identity string, server bool,
	ca *x509.Certificate, caPrivate ed25519.PrivateKey,
) (tlsKeyPairFiles, tls.Certificate) {
	t.Helper()
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	template := &x509.Certificate{SerialNumber: bigOne(), Subject: pkix.Name{CommonName: identity},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature}
	if server {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		template.DNSNames = []string{identity}
	} else {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	der, _ := x509.CreateCertificate(rand.Reader, template, ca, public, caPrivate)
	certFile := filepath.Join(root, prefix+".crt")
	keyFile := filepath.Join(root, prefix+".key")
	privateDER, _ := x509.MarshalPKCS8PrivateKey(private)
	os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
	os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}), 0o600)
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	return tlsKeyPairFiles{certFile: certFile, keyFile: keyFile}, certificate
}

func bigOne() *big.Int { return big.NewInt(time.Now().UnixNano()) }

func TestBrokerErrorCodesRemainProtocolVisible(t *testing.T) {
	for err, want := range map[error]string{
		ErrGrantDenied:         ErrorGrantDenied,
		ErrBudgetExhausted:     ErrorBudgetExhausted,
		ErrApprovalRequired:    ErrorApprovalRequired,
		ErrApprovalDenied:      ErrorApprovalDenied,
		ErrIntentExpired:       ErrorIntentExpired,
		ErrEgressDenied:        ErrorEgressDenied,
		ErrSecretDenied:        ErrorSecretDenied,
		ErrProjectConflict:     ErrorProjectConflict,
		ErrArtifactDenied:      ErrorArtifactDenied,
		ErrExecutorUnavailable: ErrorExecutorUnavailable,
	} {
		if got := ErrorCode(err); got != want {
			t.Fatalf("ErrorCode(%v) = %q, want %q", err, got, want)
		}
	}
}

func (r *recordingCommands) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	r.args = append(r.args, append([]string(nil), args...))
	if len(r.outputs) == 0 {
		return nil, ErrRuntimeUnavailable
	}
	output := r.outputs[0]
	r.outputs = r.outputs[1:]
	return output, nil
}
