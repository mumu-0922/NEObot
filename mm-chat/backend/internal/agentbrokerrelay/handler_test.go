package agentbrokerrelay

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentrunner"
)

const (
	testRunnerRelayIdentity = "spiffe://neo-chat/neo-runner-broker-relay"
	testCanaryIdentity      = "spiffe://neo-chat/agent-runtime-broker-canary"
	testRunnerID            = "neo-runner-primary"
)

func TestHandlerVerifiesTransportAndOriginalAuthorityBeforePrepare(t *testing.T) {
	target := &fakeTarget{prepare: agentrunner.PrepareResult{Prepared: true,
		IntentID: "intent_0123456789abcdef", IntentFingerprint: relayFingerprint('7'),
		IdempotencyKey: "commit_0123456789abcdef01234567", Approval: "automatic",
		ExpiresAt: timePointer(time.Now().UTC().Add(time.Minute))}}
	verifier := &fakeAuthorityVerifier{}
	handler, err := NewHandler(target, verifier, testRunnerRelayIdentity, testCanaryIdentity, testRunnerID)
	if err != nil {
		t.Fatal(err)
	}
	body := relayPrepareRequest()
	recorder := callHandler(t, handler, agentrunner.MethodPrepare, body, testRunnerRelayIdentity)
	if recorder.Code != http.StatusOK || target.prepares != 1 || verifier.calls != 1 {
		t.Fatalf("status=%d prepares=%d verifies=%d body=%s", recorder.Code, target.prepares,
			verifier.calls, recorder.Body.String())
	}
	var result response
	if json.Unmarshal(recorder.Body.Bytes(), &result) != nil || result.Prepare == nil || !result.Prepare.Prepared {
		t.Fatalf("response = %s", recorder.Body.String())
	}
}

func TestHandlerRejectsWrongRelayIdentityBeforeAuthorityOrTarget(t *testing.T) {
	target := &fakeTarget{}
	verifier := &fakeAuthorityVerifier{}
	handler, _ := NewHandler(target, verifier, testRunnerRelayIdentity, testCanaryIdentity, testRunnerID)
	recorder := callHandler(t, handler, agentrunner.MethodPrepare, relayPrepareRequest(),
		"spiffe://neo-chat/not-the-runner")
	if recorder.Code != http.StatusUnauthorized || target.prepares != 0 || verifier.calls != 0 {
		t.Fatalf("status=%d prepares=%d verifies=%d", recorder.Code, target.prepares, verifier.calls)
	}
}

func TestHandlerRejectsOriginalAuthorityDriftBeforeTarget(t *testing.T) {
	target := &fakeTarget{}
	verifier := &fakeAuthorityVerifier{err: agentrunner.ErrLeaseStale}
	handler, _ := NewHandler(target, verifier, testRunnerRelayIdentity, testCanaryIdentity, testRunnerID)
	recorder := callHandler(t, handler, agentrunner.MethodPrepare, relayPrepareRequest(), testRunnerRelayIdentity)
	if recorder.Code != http.StatusUnauthorized || target.prepares != 0 || verifier.calls != 1 {
		t.Fatalf("status=%d prepares=%d verifies=%d", recorder.Code, target.prepares, verifier.calls)
	}
}

func TestHandlerMapsOutcomeUnknownWithoutRetryHint(t *testing.T) {
	target := &fakeTarget{commitErr: agentrunner.ErrOutcomeUnknown}
	handler, _ := NewHandler(target, &fakeAuthorityVerifier{}, testRunnerRelayIdentity,
		testCanaryIdentity, testRunnerID)
	recorder := callHandler(t, handler, agentrunner.MethodCommit, relayCommitRequest(), testRunnerRelayIdentity)
	if recorder.Code != http.StatusConflict || target.commits != 1 ||
		!strings.Contains(recorder.Body.String(), agentrunner.ErrorOutcomeUnknown) ||
		strings.Contains(recorder.Body.String(), `"retryable":true`) {
		t.Fatalf("status=%d commits=%d body=%s", recorder.Code, target.commits, recorder.Body.String())
	}
}

func callHandler(t *testing.T, handler http.Handler, method string, body any, identity string) *httptest.ResponseRecorder {
	t.Helper()
	encodedBody, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(envelope{SchemaVersion: ProtocolVersion, Method: method, Body: encodedBody})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "https://10.0.0.8"+Path, strings.NewReader(string(payload)))
	request.Header.Set("Content-Type", "application/json")
	request.TLS = &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{{
		Subject: pkix.Name{CommonName: identity},
	}}}}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func relayPrepareRequest() agentrunner.PrepareRequest {
	arguments := json.RawMessage(`{}`)
	return agentrunner.PrepareRequest{Attempt: relayAttempt(), Authority: relayTicket(agentrunner.MethodPrepare),
		SnapshotFingerprint: relayFingerprint('6'), GrantID: "grant_0123456789abcdef",
		GrantFingerprint: relayFingerprint('4'), RegistryFingerprint: relayFingerprint('5'),
		ToolIdentity: "workspace_read", Capability: "workspace.read", Action: "read",
		Resource: "project/a", Arguments: arguments,
		ArgumentsFingerprint: domainFingerprint("neo-effect-arguments-v1", arguments), TTLSeconds: 60}
}

func relayCommitRequest() agentrunner.CommitRequest {
	return agentrunner.CommitRequest{Attempt: relayAttempt(), Authority: relayTicket(agentrunner.MethodCommit),
		SnapshotFingerprint: relayFingerprint('6'), GrantFingerprint: relayFingerprint('4'),
		RegistryFingerprint: relayFingerprint('5'), IntentID: "intent_0123456789abcdef",
		IntentFingerprint: relayFingerprint('7'), IdempotencyKey: "commit_0123456789abcdef01234567"}
}

func relayAttempt() agentrunner.AttemptRef {
	return agentrunner.AttemptRef{RunID: "run_0123456789abcdef", StepID: "step_0123456789abcdef",
		AttemptID: "attempt_0123456789abcdef", LeaseGeneration: 1, LeaseOwner: testRunnerID,
		LeaseToken: "lease_0123456789abcdef01234567"}
}

func relayTicket(method string) agentrunner.AuthorityTicket {
	return agentrunner.AuthorityTicket{AuthorityClaims: agentrunner.AuthorityClaims{
		CallerIdentity: testCanaryIdentity, Method: method, RequestID: "rpc_0123456789abcdef",
		Nonce: strings.Repeat("n", 32),
	}}
}

func relayFingerprint(character byte) string {
	return "sha256:" + strings.Repeat(string(character), 64)
}

func domainFingerprint(domain string, body []byte) string {
	digest := sha256.Sum256(append(append([]byte(nil), domain...), append([]byte{0}, body...)...))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func timePointer(value time.Time) *time.Time { return &value }

type fakeTarget struct {
	prepare    agentrunner.PrepareResult
	prepareErr error
	commit     agentrunner.CommitResult
	commitErr  error
	prepares   int
	commits    int
}

func (target *fakeTarget) Prepare(context.Context, agentrunner.PrepareRequest) (agentrunner.PrepareResult, error) {
	target.prepares++
	return target.prepare, target.prepareErr
}

func (target *fakeTarget) Commit(context.Context, agentrunner.CommitRequest) (agentrunner.CommitResult, error) {
	target.commits++
	return target.commit, target.commitErr
}

type fakeAuthorityVerifier struct {
	calls int
	err   error
}

func (verifier *fakeAuthorityVerifier) Verify(string, string, string, string, string, string, string,
	agentrunner.AttemptRef, agentrunner.AuthorityTicket, time.Time,
) error {
	verifier.calls++
	return verifier.err
}
