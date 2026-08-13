package agentrunner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/strictjson"
)

const maxRPCBytes = 256 << 10

var (
	prefixedID        = regexp.MustCompile(`^(rpc|run|step|attempt|grant|intent|approval|sandbox|workspace_snapshot)_[a-z0-9]{16,64}$`)
	fingerprint       = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	noncePattern      = regexp.MustCompile(`^[A-Za-z0-9_-]{32,128}$`)
	leasePattern      = regexp.MustCompile(`^lease_[A-Za-z0-9_-]{24,128}$`)
	identityPattern   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:@/-]{0,127}$`)
	identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)
	imagePattern      = regexp.MustCompile(`^[^@\s]+@sha256:[0-9a-f]{64}$`)
	uuidPattern       = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	revisionPattern   = regexp.MustCompile(`^(?:rev_[A-Za-z0-9_-]{8,128}|sha256:[a-f0-9]{64})$`)
	commitKeyPattern  = regexp.MustCompile(`^commit_[A-Za-z0-9_-]{24,128}$`)
)

var allowedFeatures = stringSet(
	"rootless_userns", "cgroup_v2", "seccomp", "readonly_rootfs",
	"snapshot_workspace", "idmapped_workspace", "network_broker", "pidfd_kill", "cgroup_reap",
	"rootless_runtime", "network_none", "subordinate_ids",
)

func DecodeRequest(body []byte, now time.Time, maxSkew time.Duration) (Request, error) {
	var envelope Envelope
	if err := strictjson.Decode(body, maxRPCBytes, &envelope); err != nil {
		return Request{}, ErrInvalidInput
	}
	if envelope.SchemaVersion != ProtocolVersion {
		return Request{}, ErrVersionUnsupported
	}
	if !validID(envelope.RequestID, "rpc") || !noncePattern.MatchString(envelope.Nonce) ||
		envelope.SentAt.IsZero() || envelope.SentAt.Before(now.Add(-maxSkew)) ||
		envelope.SentAt.After(now.Add(maxSkew)) {
		return Request{}, ErrInvalidInput
	}
	request := Request{Envelope: envelope}
	var typed any
	switch envelope.Method {
	case MethodProbe:
		request.Probe = &ProbeRequest{}
		typed = request.Probe
	case MethodLaunch:
		request.Launch = &LaunchRequest{}
		typed = request.Launch
	case MethodHeartbeat:
		request.Heartbeat = &HeartbeatRequest{}
		typed = request.Heartbeat
	case MethodCancel:
		request.Cancel = &CancelRequest{}
		typed = request.Cancel
	case MethodPrepare:
		request.Prepare = &PrepareRequest{}
		typed = request.Prepare
	case MethodCommit:
		request.Commit = &CommitRequest{}
		typed = request.Commit
	case MethodList:
		request.List = &ListRequest{}
		typed = request.List
	case MethodReconcile:
		request.Reconcile = &ReconcileRequest{}
		typed = request.Reconcile
	default:
		return Request{}, ErrVersionUnsupported
	}
	if err := strictjson.Decode(envelope.Body, maxRPCBytes, typed); err != nil {
		return Request{}, ErrInvalidInput
	}
	if err := validateRequest(&request); err != nil {
		return Request{}, err
	}
	canonical, err := json.Marshal(struct {
		SchemaVersion string    `json:"schemaVersion"`
		Method        string    `json:"method"`
		RequestID     string    `json:"requestId"`
		SentAt        time.Time `json:"sentAt"`
		Nonce         string    `json:"nonce"`
		Body          any       `json:"body"`
	}{envelope.SchemaVersion, envelope.Method, envelope.RequestID, envelope.SentAt,
		envelope.Nonce, typed})
	if err != nil {
		return Request{}, ErrInvalidInput
	}
	request.Canonical = canonical
	return request, nil
}

func validateRequest(request *Request) error {
	switch request.Method {
	case MethodProbe:
		if len(request.Probe.RequiredFeatures) < 1 || len(request.Probe.RequiredFeatures) > len(allowedFeatures) ||
			!uniqueAllowed(request.Probe.RequiredFeatures, allowedFeatures) {
			return ErrInvalidInput
		}
	case MethodLaunch:
		body := request.Launch
		if !validAttempt(body.Attempt) || !validID(body.GrantID, "grant") ||
			!validFingerprint(body.GrantFingerprint) || !validFingerprint(body.SnapshotFingerprint) ||
			validateSandbox(body.Sandbox) != nil || validateRegistry(body.ToolRegistry) != nil ||
			len(body.Argv) < 1 || len(body.Argv) > 32 {
			return ErrInvalidInput
		}
		for _, value := range body.Argv {
			if value == "" || len(value) > 512 || strings.ContainsRune(value, '\x00') {
				return ErrInvalidInput
			}
		}
	case MethodHeartbeat:
		body := request.Heartbeat
		if !validAttempt(body.Attempt) || body.LastEventSequence < 0 ||
			!member(body.ObservedState, "starting", "running", "prepared", "committing") {
			return ErrInvalidInput
		}
	case MethodCancel:
		body := request.Cancel
		if !validAttempt(body.Attempt) || !member(body.Mode, "cancel", "kill") ||
			!identifierPattern.MatchString(body.ReasonCode) || len(body.ReasonCode) > 64 {
			return ErrInvalidInput
		}
	case MethodPrepare:
		body := request.Prepare
		if !validAttempt(body.Attempt) || !validFingerprint(body.SnapshotFingerprint) ||
			!validID(body.GrantID, "grant") || !validFingerprint(body.GrantFingerprint) ||
			!validFingerprint(body.RegistryFingerprint) || !identifierPattern.MatchString(body.ToolIdentity) ||
			!identifierPattern.MatchString(body.Capability) || !identifierPattern.MatchString(body.Action) ||
			body.Resource == "" || len(body.Resource) > 256 || strings.ContainsAny(body.Resource, "\x00\r\n") ||
			!validFingerprint(body.ArgumentsFingerprint) || body.TTLSeconds < 60 || body.TTLSeconds > 3600 ||
			len(body.Arguments) == 0 || len(body.Arguments) > maxRPCBytes ||
			(body.BaseRevision != "" && !revisionPattern.MatchString(body.BaseRevision)) {
			return ErrInvalidInput
		}
		canonical, err := canonicalArguments(body.Arguments)
		if err != nil || fingerprintBytes("neo-effect-arguments-v1", canonical) != body.ArgumentsFingerprint {
			return ErrInvalidInput
		}
		body.Arguments = canonical
	case MethodCommit:
		body := request.Commit
		if !validAttempt(body.Attempt) || !validFingerprint(body.SnapshotFingerprint) ||
			!validFingerprint(body.GrantFingerprint) || !validFingerprint(body.RegistryFingerprint) ||
			!validID(body.IntentID, "intent") || !validFingerprint(body.IntentFingerprint) ||
			(body.ApprovalID != "" && !validID(body.ApprovalID, "approval")) ||
			!commitKeyPattern.MatchString(body.IdempotencyKey) {
			return ErrInvalidInput
		}
	case MethodList:
		if !identityPattern.MatchString(request.List.RunnerID) {
			return ErrInvalidInput
		}
	case MethodReconcile:
		if !identityPattern.MatchString(request.Reconcile.RunnerID) || len(request.Reconcile.Expected) > 1000 {
			return ErrInvalidInput
		}
		seen := map[string]struct{}{}
		for _, descriptor := range request.Reconcile.Expected {
			if !validSandboxDescriptor(descriptor) {
				return ErrInvalidInput
			}
			if _, duplicate := seen[descriptor.Attempt.AttemptID]; duplicate {
				return ErrInvalidInput
			}
			seen[descriptor.Attempt.AttemptID] = struct{}{}
		}
	default:
		return ErrVersionUnsupported
	}
	return nil
}

func validateSandbox(sandbox SandboxSpec) error {
	if !validFingerprint(sandbox.RuntimeBundleFingerprint) || !validFingerprint(sandbox.PackageFingerprint) ||
		!imagePattern.MatchString(sandbox.Image) || sandbox.UID < 1 || sandbox.GID < 1 ||
		!sandbox.RootfsReadOnly || !sandbox.NoNewPrivileges || len(sandbox.Capabilities) != 0 ||
		!validFingerprint(sandbox.SeccompProfileFingerprint) || sandbox.NetworkMode != "none" ||
		!validID(sandbox.WorkspaceSnapshotID, "workspace_snapshot") ||
		!validFingerprint(sandbox.WorkspaceFingerprint) {
		return ErrInvalidInput
	}
	limits := sandbox.Resources
	if limits.CPUMillis < 100 || limits.CPUMillis > 8000 ||
		limits.MemoryMiB < 64 || limits.MemoryMiB > 16384 ||
		limits.PIDs < 8 || limits.PIDs > 1024 ||
		limits.WallSeconds < 1 || limits.WallSeconds > 86400 ||
		limits.OutputBytes < 1 || limits.OutputBytes > 32<<20 ||
		limits.ScratchBytes < 1<<20 || limits.ScratchBytes > 8<<30 {
		return ErrInvalidInput
	}
	return nil
}

func validateRegistry(registry ToolRegistry) error {
	if registry.Depth < 0 || registry.Depth > 1 || len(registry.Tools) > 128 ||
		!validFingerprint(registry.RegistryFingerprint) {
		return ErrInvalidInput
	}
	seen := make(map[string]struct{}, len(registry.Tools))
	for _, tool := range registry.Tools {
		if !identifierPattern.MatchString(tool) || len(tool) > 64 {
			return ErrInvalidInput
		}
		if _, duplicate := seen[tool]; duplicate {
			return ErrInvalidInput
		}
		seen[tool] = struct{}{}
	}
	if registry.Depth == 1 {
		for _, denied := range []string{"delegate_task", "cron_manage", "grant_manage", "secret_manage", "runtime_manage"} {
			if _, ok := seen[denied]; ok {
				return ErrInvalidInput
			}
		}
	}
	return nil
}

func validateReleaseManifest(manifest ReleaseManifest) error {
	if manifest.SchemaVersion != ReleaseVersion || !identityPattern.MatchString(manifest.RunnerID) ||
		manifest.RunnerVersion == "" || len(manifest.RunnerVersion) > 128 ||
		manifest.ProtocolVersion != ProtocolVersion || manifest.StorageDriver != "overlay" ||
		manifest.NetworkMode != "none" || manifest.UserNamespaceSize < 65536 ||
		!validFingerprint(manifest.ProbeSuiteFingerprint) || len(manifest.Binaries) < 4 ||
		!uniqueAllowed(manifest.RequiredControllers, stringSet("cpu", "memory", "pids")) ||
		len(manifest.RequiredControllers) != 3 || !validReleaseFile(manifest.SeccompProfile) ||
		!validReleaseFile(manifest.IsolationAcceptance) {
		return ErrInvalidInput
	}
	seen := map[string]struct{}{}
	for _, binary := range manifest.Binaries {
		if !member(binary.Name, "podman", "crun", "conmon", "newuidmap", "newgidmap") ||
			binary.Version == "" || len(binary.Version) > 128 ||
			!validReleaseFile(ReleaseFile{Path: binary.Path, SHA256: binary.SHA256}) {
			return ErrInvalidInput
		}
		if _, duplicate := seen[binary.Name]; duplicate {
			return ErrInvalidInput
		}
		seen[binary.Name] = struct{}{}
	}
	for _, required := range []string{"podman", "crun", "conmon", "newuidmap", "newgidmap"} {
		if _, ok := seen[required]; !ok {
			return ErrInvalidInput
		}
	}
	return nil
}

func validReleaseFile(file ReleaseFile) bool {
	return strings.HasPrefix(file.Path, "/") && len(file.Path) <= 512 &&
		!strings.Contains(file.Path, "..") && !strings.ContainsRune(file.Path, '\x00') &&
		validFingerprint(file.SHA256)
}

func requestFingerprint(request Request) string {
	digest := sha256.Sum256(append([]byte("neo-runner-rpc-request-v1\x00"), request.Canonical...))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func canonicalArguments(raw []byte) ([]byte, error) {
	var value any
	if err := strictjson.Decode(raw, maxRPCBytes, &value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func fingerprintBytes(domain string, value []byte) string {
	digest := sha256.Sum256(append(append([]byte(nil), []byte(domain)...), append([]byte{0}, value...)...))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func AuthorityRequestFingerprint(method string, body any) string {
	var unsigned any
	switch method {
	case MethodLaunch:
		value, ok := body.(LaunchRequest)
		if !ok {
			return ""
		}
		value.Authority = AuthorityTicket{}
		unsigned = value
	case MethodHeartbeat:
		value, ok := body.(HeartbeatRequest)
		if !ok {
			return ""
		}
		value.Authority = AuthorityTicket{}
		unsigned = value
	case MethodCancel:
		value, ok := body.(CancelRequest)
		if !ok {
			return ""
		}
		value.Authority = AuthorityTicket{}
		unsigned = value
	case MethodPrepare:
		value, ok := body.(PrepareRequest)
		if !ok {
			return ""
		}
		value.Authority = AuthorityTicket{}
		unsigned = value
	case MethodCommit:
		value, ok := body.(CommitRequest)
		if !ok {
			return ""
		}
		value.Authority = AuthorityTicket{}
		unsigned = value
	default:
		return ""
	}
	encoded, err := json.Marshal(unsigned)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(append([]byte("neo-runner-authority-request-v1\x00"), encoded...))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func authorityFingerprintForRequest(request Request) string {
	switch request.Method {
	case MethodLaunch:
		return AuthorityRequestFingerprint(request.Method, *request.Launch)
	case MethodHeartbeat:
		return AuthorityRequestFingerprint(request.Method, *request.Heartbeat)
	case MethodCancel:
		return AuthorityRequestFingerprint(request.Method, *request.Cancel)
	case MethodPrepare:
		return AuthorityRequestFingerprint(request.Method, *request.Prepare)
	case MethodCommit:
		return AuthorityRequestFingerprint(request.Method, *request.Commit)
	default:
		return ""
	}
}

func validSandboxDescriptor(value SandboxDescriptor) bool {
	return validID(value.SandboxID, "sandbox") && validID(value.Attempt.RunID, "run") &&
		validID(value.Attempt.StepID, "step") && validID(value.Attempt.AttemptID, "attempt") &&
		value.Attempt.LeaseGeneration >= 1 && validFingerprint(value.SnapshotFingerprint) &&
		validFingerprint(value.SpecFingerprint) && validFingerprint(value.ProbeFingerprint) &&
		member(value.State, "expected", "created", "starting", "running", "stopping") && !value.UpdatedAt.IsZero()
}

func sandboxFingerprint(request LaunchRequest) string {
	body, _ := json.Marshal(struct {
		Snapshot string       `json:"snapshot"`
		Grant    string       `json:"grant"`
		Sandbox  SandboxSpec  `json:"sandbox"`
		Registry ToolRegistry `json:"registry"`
		Argv     []string     `json:"argv"`
	}{request.SnapshotFingerprint, request.GrantFingerprint, request.Sandbox,
		request.ToolRegistry, request.Argv})
	digest := sha256.Sum256(append([]byte("neo-runner-sandbox-spec-v1\x00"), body...))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func validAttempt(attempt AttemptRef) bool {
	return validID(attempt.RunID, "run") && validID(attempt.StepID, "step") &&
		validID(attempt.AttemptID, "attempt") && attempt.LeaseGeneration >= 1 &&
		identityPattern.MatchString(attempt.LeaseOwner) && leasePattern.MatchString(attempt.LeaseToken)
}

func validID(value, prefix string) bool {
	return strings.HasPrefix(value, prefix+"_") && prefixedID.MatchString(value)
}

func validFingerprint(value string) bool { return fingerprint.MatchString(value) }

func uniqueAllowed(values []string, allowed map[string]struct{}) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := allowed[value]; !ok {
			return false
		}
		if _, duplicate := seen[value]; duplicate {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func stringSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func member(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func ErrorCode(err error) string {
	switch {
	case errorsIs(err, ErrAuthFailed):
		return ErrorAuthFailed
	case errorsIs(err, ErrReplayDetected):
		return ErrorReplayDetected
	case errorsIs(err, ErrVersionUnsupported):
		return ErrorVersionUnsupported
	case errorsIs(err, ErrIsolationUnavailable):
		return ErrorIsolationUnavailable
	case errorsIs(err, ErrRuntimeUnavailable):
		return ErrorRuntimeUnavailable
	case errorsIs(err, ErrSnapshotMismatch):
		return ErrorSnapshotMismatch
	case errorsIs(err, ErrGrantDenied):
		return ErrorGrantDenied
	case errorsIs(err, ErrLeaseStale):
		return ErrorLeaseStale
	case errorsIs(err, ErrKillSwitchActive):
		return ErrorKillSwitchActive
	case errorsIs(err, ErrBudgetExhausted):
		return ErrorBudgetExhausted
	case errorsIs(err, ErrApprovalRequired):
		return ErrorApprovalRequired
	case errorsIs(err, ErrApprovalDenied):
		return ErrorApprovalDenied
	case errorsIs(err, ErrIntentExpired):
		return ErrorIntentExpired
	case errorsIs(err, ErrEgressDenied):
		return ErrorEgressDenied
	case errorsIs(err, ErrSecretDenied):
		return ErrorSecretDenied
	case errorsIs(err, ErrProjectConflict):
		return ErrorProjectConflict
	case errorsIs(err, ErrArtifactDenied):
		return ErrorArtifactDenied
	case errorsIs(err, ErrExecutorUnavailable):
		return ErrorExecutorUnavailable
	case errorsIs(err, ErrInvalidTransition):
		return ErrorInvalidTransition
	case errorsIs(err, ErrOutcomeUnknown):
		return ErrorOutcomeUnknown
	default:
		return ErrorInternal
	}
}

func errorsIs(err, target error) bool {
	return err == target || strings.Contains(fmt.Sprint(err), target.Error())
}
