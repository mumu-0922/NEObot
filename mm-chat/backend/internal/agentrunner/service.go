package agentrunner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"neo-chat/mm-chat/backend/internal/strictjson"
)

type ServiceConfig struct {
	RunnerID           string
	StateRoot          string
	SeccompPath        string
	SeccompFingerprint string
	ReplayTTL          time.Duration
	HeartbeatExtension time.Duration
}

type sandboxRecord struct {
	Descriptor       SandboxDescriptor `json:"descriptor"`
	ContainerID      string            `json:"containerId"`
	LeaseTokenDigest string            `json:"leaseTokenDigest"`
	WallDeadline     time.Time         `json:"wallDeadline"`
	Plan             LaunchPlan        `json:"plan"`
}

type Service struct {
	config     ServiceConfig
	probe      HostProbe
	authority  AuthorityVerifier
	replay     ReplayLedger
	driver     SandboxDriver
	workspaces *WorkspaceCatalog
	artifacts  *ArtifactBroker
	brokers    map[string]BrokerRelay
	now        func() time.Time
	mu         sync.Mutex
	records    map[string]sandboxRecord
	intakes    map[string]*ArtifactIntakeListener
}

// BrokerRelay is injected by the private control plane. The Runner receives no
// database, vault, object-store or MCP credentials and only forwards strict,
// authority-verified Prepare/Commit messages.
type BrokerRelay interface {
	Prepare(context.Context, PrepareRequest) (PrepareResult, error)
	Commit(context.Context, CommitRequest) (CommitResult, error)
}

type unavailableBrokerRelay struct{}

func (unavailableBrokerRelay) Prepare(context.Context, PrepareRequest) (PrepareResult, error) {
	return PrepareResult{}, ErrRuntimeUnavailable
}
func (unavailableBrokerRelay) Commit(context.Context, CommitRequest) (CommitResult, error) {
	return CommitResult{}, ErrRuntimeUnavailable
}

func (service *Service) CompactLocalReplay(ctx context.Context) (int, error) {
	if service == nil {
		return 0, ErrInvalidInput
	}
	return service.replay.Compact(ctx)
}

func NewService(config ServiceConfig, probe HostProbe, authority AuthorityVerifier,
	replay ReplayLedger, driver SandboxDriver, workspaces *WorkspaceCatalog,
	artifacts *ArtifactBroker) (*Service, error) {
	if !identityPattern.MatchString(config.RunnerID) || !filepath.IsAbs(config.StateRoot) ||
		!filepath.IsAbs(config.SeccompPath) || !validFingerprint(config.SeccompFingerprint) ||
		config.ReplayTTL < time.Minute || config.ReplayTTL > 24*time.Hour ||
		config.HeartbeatExtension < 5*time.Second || config.HeartbeatExtension > time.Hour ||
		probe == nil || authority == nil || replay == nil || driver == nil || workspaces == nil || artifacts == nil {
		return nil, ErrInvalidInput
	}
	for _, directory := range []string{filepath.Join(config.StateRoot, "sandboxes"), filepath.Join(config.StateRoot, "scratch")} {
		info, err := os.Stat(directory)
		if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
			return nil, ErrInvalidInput
		}
	}
	service := &Service{config: config, probe: probe, authority: authority, replay: replay, driver: driver,
		workspaces: workspaces, artifacts: artifacts, now: time.Now, records: map[string]sandboxRecord{},
		intakes: map[string]*ArtifactIntakeListener{}, brokers: map[string]BrokerRelay{}}
	if err := service.loadRecords(); err != nil {
		return nil, err
	}
	return service, nil
}

func (service *Service) WithBrokerRelayForCaller(caller string, relay BrokerRelay) error {
	if service == nil || !identityPattern.MatchString(caller) || relay == nil || service.brokers[caller] != nil {
		return ErrInvalidInput
	}
	service.brokers[caller] = relay
	return nil
}

func (service *Service) Handle(ctx context.Context, caller string, request Request) (Response, error) {
	if service == nil || !identityPattern.MatchString(caller) || len(request.Canonical) == 0 {
		return Response{}, ErrInvalidInput
	}
	fingerprint := requestFingerprint(request)
	if authorized := authorityFingerprintForRequest(request); authorized != "" {
		fingerprint = authorized
	}
	claim := ReplayClaim{CallerIdentity: caller, RequestID: request.RequestID, Nonce: request.Nonce,
		Method: request.Method, RequestFingerprint: fingerprint, ExpiresAt: service.now().Add(service.config.ReplayTTL)}
	replay, err := service.replay.Claim(ctx, claim)
	if err != nil {
		return service.errorResponse(request, err), err
	}
	if replay.Replay {
		response, decodeErr := decodeReplayResponse(request.Method, replay.Response)
		if decodeErr != nil {
			return Response{}, ErrRuntimeUnavailable
		}
		return response, responseOperationError(response)
	}
	response, operationErr := service.execute(ctx, caller, request)
	encoded, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		return Response{}, ErrRuntimeUnavailable
	}
	if completeErr := service.replay.Complete(ctx, caller, request.RequestID, request.Nonce, encoded); completeErr != nil {
		return Response{}, completeErr
	}
	return response, operationErr
}

func decodeReplayResponse(method string, body []byte) (Response, error) {
	var wire struct {
		SchemaVersion string          `json:"schemaVersion"`
		Method        string          `json:"method"`
		RequestID     string          `json:"requestId"`
		SentAt        time.Time       `json:"sentAt"`
		Nonce         string          `json:"nonce"`
		Body          json.RawMessage `json:"body"`
	}
	if strictjson.Decode(body, maxRPCBytes, &wire) != nil || wire.SchemaVersion != ProtocolVersion ||
		wire.Method != method+".result" || !validID(wire.RequestID, "rpc") ||
		!noncePattern.MatchString(wire.Nonce) || wire.SentAt.IsZero() {
		return Response{}, ErrRuntimeUnavailable
	}
	response := Response{SchemaVersion: wire.SchemaVersion, Method: wire.Method,
		RequestID: wire.RequestID, SentAt: wire.SentAt, Nonce: wire.Nonce}
	var generic ErrorResult
	if strictjson.Decode(wire.Body, maxRPCBytes, &generic) == nil && !generic.Accepted && generic.Error.Code != "" {
		response.Body = generic
		return response, nil
	}
	var target any
	switch method {
	case MethodProbe:
		target = &ProbeResult{}
	case MethodLaunch:
		target = &LaunchResult{}
	case MethodHeartbeat:
		target = &HeartbeatResult{}
	case MethodCancel:
		target = &CancelResult{}
	case MethodPrepare:
		target = &PrepareResult{}
	case MethodCommit:
		target = &CommitResult{}
	case MethodList:
		target = &ListResult{}
	case MethodReconcile:
		target = &ReconcileResult{}
	default:
		return Response{}, ErrVersionUnsupported
	}
	if strictjson.Decode(wire.Body, maxRPCBytes, target) != nil {
		return Response{}, ErrRuntimeUnavailable
	}
	if err := validateRelayResult(target); err != nil {
		return Response{}, err
	}
	switch typed := target.(type) {
	case *ProbeResult:
		response.Body = *typed
	case *LaunchResult:
		response.Body = *typed
	case *HeartbeatResult:
		response.Body = *typed
	case *CancelResult:
		response.Body = *typed
	case *PrepareResult:
		response.Body = *typed
	case *CommitResult:
		response.Body = *typed
	case *ListResult:
		response.Body = *typed
	case *ReconcileResult:
		response.Body = *typed
	}
	return response, nil
}

func (service *Service) execute(ctx context.Context, caller string, request Request) (Response, error) {
	switch request.Method {
	case MethodProbe:
		evidence, err := service.probe.Probe(ctx)
		body := ProbeResult{Ready: evidence.Ready, Runtime: evidence.Runtime,
			RuntimeVersion: evidence.RuntimeVersion, Features: evidence.Features, ProbeFingerprint: evidence.ProbeFingerprint}
		if err != nil {
			body.Error = &RPCError{Code: ErrorIsolationUnavailable, Retryable: false}
		}
		return service.response(request, MethodProbe+".result", body), err
	case MethodLaunch:
		return service.launch(ctx, caller, request)
	case MethodHeartbeat:
		return service.heartbeat(ctx, caller, request)
	case MethodCancel:
		return service.cancel(ctx, caller, request)
	case MethodPrepare:
		return service.prepare(ctx, caller, request)
	case MethodCommit:
		return service.commit(ctx, caller, request)
	case MethodList:
		return service.list(ctx, request)
	case MethodReconcile:
		return service.reconcileRPC(ctx, request)
	default:
		return service.errorResponse(request, ErrVersionUnsupported), ErrVersionUnsupported
	}
}

func (service *Service) prepare(ctx context.Context, caller string, request Request) (Response, error) {
	body := request.Prepare
	if err := service.authority.Verify(caller, service.config.RunnerID, MethodPrepare, request.RequestID,
		request.Nonce, authorityFingerprintForRequest(request), body.SnapshotFingerprint,
		body.Attempt, body.Authority, service.now()); err != nil {
		return service.response(request, MethodPrepare+".result", PrepareResult{Error: rpcError(authorityError(err))}), authorityError(err)
	}
	relay := service.brokers[caller]
	if relay == nil {
		relay = unavailableBrokerRelay{}
	}
	result, err := relay.Prepare(ctx, *body)
	if err != nil {
		result = PrepareResult{Error: rpcError(err)}
	} else if validateRelayResult(&result) != nil {
		err = ErrRuntimeUnavailable
		result = PrepareResult{Error: rpcError(err)}
	}
	return service.response(request, MethodPrepare+".result", result), err
}

func (service *Service) commit(ctx context.Context, caller string, request Request) (Response, error) {
	body := request.Commit
	if err := service.authority.Verify(caller, service.config.RunnerID, MethodCommit, request.RequestID,
		request.Nonce, authorityFingerprintForRequest(request), body.SnapshotFingerprint,
		body.Attempt, body.Authority, service.now()); err != nil {
		return service.response(request, MethodCommit+".result", CommitResult{IdempotencyKey: body.IdempotencyKey, Error: rpcError(authorityError(err))}), authorityError(err)
	}
	relay := service.brokers[caller]
	if relay == nil {
		relay = unavailableBrokerRelay{}
	}
	result, err := relay.Commit(ctx, *body)
	if err != nil {
		outcome := "rejected"
		if errors.Is(err, ErrOutcomeUnknown) {
			outcome = "outcome_unknown"
		}
		result = CommitResult{Outcome: outcome, IdempotencyKey: body.IdempotencyKey, Error: rpcError(err)}
	} else if validateRelayResult(&result) != nil || result.IdempotencyKey != body.IdempotencyKey {
		err = ErrRuntimeUnavailable
		result = CommitResult{Outcome: "rejected", IdempotencyKey: body.IdempotencyKey, Error: rpcError(err)}
	}
	return service.response(request, MethodCommit+".result", result), err
}

func rpcError(err error) *RPCError {
	if err == nil {
		return nil
	}
	return &RPCError{Code: ErrorCode(err), Retryable: false}
}

func validateRelayResult(value any) error {
	switch result := value.(type) {
	case *PrepareResult:
		if result.Prepared {
			if !validID(result.IntentID, "intent") || !validFingerprint(result.IntentFingerprint) ||
				!commitKeyPattern.MatchString(result.IdempotencyKey) ||
				!member(result.Approval, "automatic", "once", "per_commit") ||
				result.ExpiresAt == nil || result.ExpiresAt.IsZero() || result.Error != nil {
				return ErrRuntimeUnavailable
			}
			return nil
		}
		if result.IntentID != "" || result.IntentFingerprint != "" || result.IdempotencyKey != "" ||
			result.Approval != "" || result.ExpiresAt != nil || result.Replay || !validRPCError(result.Error) {
			return ErrRuntimeUnavailable
		}
		return nil
	case *CommitResult:
		if !commitKeyPattern.MatchString(result.IdempotencyKey) {
			return ErrRuntimeUnavailable
		}
		switch result.Outcome {
		case "committed", "replayed":
			if !validFingerprint(result.ReceiptFingerprint) || result.Error != nil ||
				(result.Outcome == "replayed") != result.Replay {
				return ErrRuntimeUnavailable
			}
		case "rejected", "outcome_unknown":
			if result.ReceiptFingerprint != "" || result.Replay || !validRPCError(result.Error) {
				return ErrRuntimeUnavailable
			}
		default:
			return ErrRuntimeUnavailable
		}
		return nil
	}
	return nil
}

func validRPCError(value *RPCError) bool {
	if value == nil || value.Retryable {
		return false
	}
	return member(value.Code, ErrorAuthFailed, ErrorReplayDetected, ErrorVersionUnsupported,
		ErrorRuntimeUnavailable, ErrorIsolationUnavailable, ErrorSnapshotMismatch, ErrorGrantDenied,
		ErrorLeaseStale, ErrorKillSwitchActive, ErrorBudgetExhausted, ErrorApprovalRequired,
		ErrorApprovalDenied, ErrorIntentExpired, ErrorEgressDenied, ErrorSecretDenied,
		ErrorProjectConflict, ErrorProjectMutationDenied, ErrorArtifactDenied, ErrorExecutorUnavailable,
		ErrorInvalidTransition, ErrorOutcomeUnknown, ErrorInternal)
}

func (service *Service) launch(ctx context.Context, caller string, request Request) (Response, error) {
	body := request.Launch
	if err := service.authority.Verify(caller, service.config.RunnerID, MethodLaunch, request.RequestID,
		request.Nonce, authorityFingerprintForRequest(request),
		body.SnapshotFingerprint, body.Attempt, body.Authority, service.now()); err != nil {
		return service.launchError(request, body.Attempt, authorityError(err))
	}
	if body.Authority.KillSwitchEpoch < 0 {
		return service.launchError(request, body.Attempt, ErrKillSwitchActive)
	}
	evidence, err := service.probe.Probe(ctx)
	if err != nil || !evidence.Ready {
		return service.launchError(request, body.Attempt, ErrIsolationUnavailable)
	}
	if body.Sandbox.SeccompProfileFingerprint != service.config.SeccompFingerprint {
		return service.launchError(request, body.Attempt, ErrSnapshotMismatch)
	}
	workspace, err := service.workspaces.Resolve(body.Sandbox.WorkspaceSnapshotID, body.Sandbox.WorkspaceFingerprint)
	if err != nil {
		return service.launchError(request, body.Attempt, ErrSnapshotMismatch)
	}
	specFingerprint := sandboxFingerprint(*body)
	sandboxID := deterministicSandboxID(body.Attempt)
	service.mu.Lock()
	defer service.mu.Unlock()
	if existing, ok := service.records[body.Attempt.AttemptID]; ok {
		if existing.Descriptor.SpecFingerprint != specFingerprint || existing.Descriptor.Attempt.LeaseGeneration != body.Attempt.LeaseGeneration {
			return service.launchError(request, body.Attempt, ErrReplayDetected)
		}
		started := existing.Descriptor.UpdatedAt
		return service.response(request, "launch.result", LaunchResult{Accepted: true, Attempt: body.Attempt.Identity(), SandboxID: existing.Descriptor.SandboxID, StartedAt: &started}), nil
	}
	scratchPath := filepath.Join(service.config.StateRoot, "scratch", body.Attempt.AttemptID)
	if err := os.Mkdir(scratchPath, 0o700); err != nil {
		return service.launchError(request, body.Attempt, ErrRuntimeUnavailable)
	}
	brokerPath := filepath.Join(scratchPath, "broker")
	// The parent state tree is mode 0700. This bind root is execute-only to the
	// remapped Sandbox user; the socket itself is write-only from the Sandbox.
	if err := os.Mkdir(brokerPath, 0o711); err != nil {
		_ = safeRemoveTree(filepath.Join(service.config.StateRoot, "scratch"), scratchPath)
		return service.launchError(request, body.Attempt, ErrRuntimeUnavailable)
	}
	if err := service.artifacts.AuthorizeAttempt(body.Attempt.Identity(), body.Sandbox.Resources.OutputBytes); err != nil {
		_ = safeRemoveTree(filepath.Join(service.config.StateRoot, "scratch"), scratchPath)
		return service.launchError(request, body.Attempt, ErrRuntimeUnavailable)
	}
	plan := LaunchPlan{SandboxID: sandboxID, ContainerName: "neo-" + strings.TrimPrefix(body.Attempt.AttemptID, "attempt_"),
		Attempt: body.Attempt.Identity(), SnapshotFingerprint: body.SnapshotFingerprint, SpecFingerprint: specFingerprint,
		ProbeFingerprint: evidence.ProbeFingerprint, Sandbox: body.Sandbox, WorkspacePath: workspace.Path,
		ScratchPath: scratchPath, BrokerPath: brokerPath, SeccompPath: service.config.SeccompPath,
		Argv: append([]string(nil), body.Argv...)}
	intake, err := StartArtifactIntake(filepath.Join(brokerPath, "artifact.sock"), service.artifacts,
		body.Attempt.Identity(), service.isCurrentAttempt)
	if err != nil {
		_ = safeRemoveTree(filepath.Join(service.config.StateRoot, "scratch"), scratchPath)
		return service.launchError(request, body.Attempt, ErrRuntimeUnavailable)
	}
	service.intakes[body.Attempt.AttemptID] = intake
	driverSandbox, err := service.driver.Create(ctx, plan)
	if err != nil {
		_ = intake.Close()
		delete(service.intakes, body.Attempt.AttemptID)
		_ = safeRemoveTree(filepath.Join(service.config.StateRoot, "scratch"), scratchPath)
		return service.launchError(request, body.Attempt, err)
	}
	inspected, err := service.driver.Inspect(ctx, driverSandbox.ContainerID)
	if err != nil || !sameSandboxIdentity(inspected, driverSandbox) || !matchesLaunchPlan(inspected, plan) {
		_ = service.driver.Reap(ctx, driverSandbox.ContainerID, true)
		_ = intake.Close()
		delete(service.intakes, body.Attempt.AttemptID)
		_ = safeRemoveTree(filepath.Join(service.config.StateRoot, "scratch"), scratchPath)
		return service.launchError(request, body.Attempt, ErrIsolationUnavailable)
	}
	if err := service.driver.Start(ctx, driverSandbox.ContainerID); err != nil {
		_ = service.driver.Reap(ctx, driverSandbox.ContainerID, true)
		_ = intake.Close()
		delete(service.intakes, body.Attempt.AttemptID)
		_ = safeRemoveTree(filepath.Join(service.config.StateRoot, "scratch"), scratchPath)
		return service.launchError(request, body.Attempt, err)
	}
	running, err := service.driver.Inspect(ctx, driverSandbox.ContainerID)
	if err != nil || running.State != "running" || !matchesLaunchPlan(running, plan) {
		_ = service.driver.Reap(ctx, driverSandbox.ContainerID, true)
		_ = intake.Close()
		delete(service.intakes, body.Attempt.AttemptID)
		_ = safeRemoveTree(filepath.Join(service.config.StateRoot, "scratch"), scratchPath)
		return service.launchError(request, body.Attempt, ErrIsolationUnavailable)
	}
	now := service.now().UTC()
	record := sandboxRecord{Descriptor: SandboxDescriptor{SandboxID: sandboxID, Attempt: body.Attempt.Identity(),
		SnapshotFingerprint: body.SnapshotFingerprint, SpecFingerprint: specFingerprint, ProbeFingerprint: evidence.ProbeFingerprint,
		State: "running", UpdatedAt: now}, ContainerID: driverSandbox.ContainerID,
		LeaseTokenDigest: body.Authority.LeaseTokenDigest,
		WallDeadline:     now.Add(time.Duration(body.Sandbox.Resources.WallSeconds) * time.Second)}
	record.Plan = plan
	if err := service.persistRecord(record); err != nil {
		_ = service.driver.Reap(ctx, driverSandbox.ContainerID, true)
		_ = intake.Close()
		delete(service.intakes, body.Attempt.AttemptID)
		_ = safeRemoveTree(filepath.Join(service.config.StateRoot, "scratch"), scratchPath)
		return service.launchError(request, body.Attempt, ErrRuntimeUnavailable)
	}
	service.records[body.Attempt.AttemptID] = record
	service.scheduleWallDeadline(record)
	return service.response(request, "launch.result", LaunchResult{Accepted: true, Attempt: body.Attempt.Identity(), SandboxID: sandboxID, StartedAt: &now}), nil
}

func (service *Service) heartbeat(ctx context.Context, caller string, request Request) (Response, error) {
	body := request.Heartbeat
	if err := service.authority.Verify(caller, service.config.RunnerID, MethodHeartbeat, request.RequestID,
		request.Nonce, authorityFingerprintForRequest(request),
		body.Authority.SnapshotFingerprint, body.Attempt, body.Authority, service.now()); err != nil {
		return service.heartbeatError(request, authorityError(err))
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	record, ok := service.records[body.Attempt.AttemptID]
	if !ok || record.Descriptor.Attempt != body.Attempt.Identity() || record.Descriptor.SnapshotFingerprint != body.Authority.SnapshotFingerprint {
		return service.heartbeatError(request, ErrLeaseStale)
	}
	inspected, err := service.driver.Inspect(ctx, record.ContainerID)
	if err != nil || inspected.State != "running" || !matchesLaunchPlan(inspected, record.Plan) {
		return service.heartbeatError(request, ErrRuntimeUnavailable)
	}
	expires := service.now().Add(service.config.HeartbeatExtension).UTC()
	record.Descriptor.UpdatedAt = service.now().UTC()
	if service.persistRecord(record) != nil {
		return service.heartbeatError(request, ErrRuntimeUnavailable)
	}
	service.records[body.Attempt.AttemptID] = record
	return service.response(request, "heartbeat.result", HeartbeatResult{Accepted: true, LeaseExpiresAt: &expires, KillRequested: false}), nil
}

func (service *Service) cancel(ctx context.Context, caller string, request Request) (Response, error) {
	body := request.Cancel
	if err := service.authority.Verify(caller, service.config.RunnerID, MethodCancel, request.RequestID,
		request.Nonce, authorityFingerprintForRequest(request),
		body.Authority.SnapshotFingerprint, body.Attempt, body.Authority, service.now()); err != nil {
		return service.cancelError(request, authorityError(err))
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	record, ok := service.records[body.Attempt.AttemptID]
	if !ok {
		return service.response(request, "cancel.result", CancelResult{Accepted: true, ObservedTerminal: "none"}), nil
	}
	if record.Descriptor.Attempt != body.Attempt.Identity() {
		return service.cancelError(request, ErrLeaseStale)
	}
	if err := service.cleanupRecord(ctx, record, body.Mode == "kill"); err != nil {
		return service.cancelError(request, ErrRuntimeUnavailable)
	}
	delete(service.records, body.Attempt.AttemptID)
	terminal := "canceled"
	if body.Mode == "kill" {
		terminal = "killed"
	}
	return service.response(request, "cancel.result", CancelResult{Accepted: true, ObservedTerminal: terminal}), nil
}

func (service *Service) list(ctx context.Context, request Request) (Response, error) {
	if request.List.RunnerID != service.config.RunnerID {
		return service.errorResponse(request, ErrAuthFailed), ErrAuthFailed
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	result := ListResult{RunnerID: service.config.RunnerID, Sandboxes: []SandboxDescriptor{}}
	for _, record := range service.records {
		if inspected, err := service.driver.Inspect(ctx, record.ContainerID); err == nil && matchesLaunchPlan(inspected, record.Plan) {
			record.Descriptor.State = inspected.State
		} else {
			return service.errorResponse(request, ErrIsolationUnavailable), ErrIsolationUnavailable
		}
		result.Sandboxes = append(result.Sandboxes, record.Descriptor)
	}
	return service.response(request, "list.result", result), nil
}

func (service *Service) reconcileRPC(ctx context.Context, request Request) (Response, error) {
	if request.Reconcile.RunnerID != service.config.RunnerID {
		return service.errorResponse(request, ErrAuthFailed), ErrAuthFailed
	}
	expected := make(map[string]SandboxDescriptor, len(request.Reconcile.Expected))
	for _, descriptor := range request.Reconcile.Expected {
		expected[descriptor.Attempt.AttemptID] = descriptor
	}
	cleaned, err := service.Reconcile(ctx, expected)
	if err != nil {
		return service.errorResponse(request, err), err
	}
	return service.response(request, MethodReconcile+".result", ReconcileResult{RunnerID: service.config.RunnerID, Cleaned: cleaned}), nil
}

func (service *Service) Reconcile(ctx context.Context, expected map[string]SandboxDescriptor) (int, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	driverItems, err := service.driver.List(ctx)
	if err != nil {
		return 0, err
	}
	cleaned := 0
	seen := map[string]struct{}{}
	for _, item := range driverItems {
		seen[item.Attempt.AttemptID] = struct{}{}
		wanted, ok := expected[item.Attempt.AttemptID]
		record, recorded := service.records[item.Attempt.AttemptID]
		if !ok || !recorded || wanted.Attempt != item.Attempt || wanted.SnapshotFingerprint != item.SnapshotFingerprint || wanted.SpecFingerprint != item.SpecFingerprint || record.ContainerID != item.ContainerID || !matchesLaunchPlan(item, record.Plan) {
			if reapErr := service.driver.Reap(ctx, item.ContainerID, true); reapErr != nil {
				return cleaned, reapErr
			}
			if recorded {
				if err := service.cleanupFiles(record); err != nil {
					return cleaned, err
				}
				delete(service.records, item.Attempt.AttemptID)
			}
			cleaned++
		}
	}
	for attemptID, record := range service.records {
		_, expectedOK := expected[attemptID]
		_, seenOK := seen[attemptID]
		if expectedOK && seenOK {
			continue
		}
		if _, inspectErr := service.driver.Inspect(ctx, record.ContainerID); inspectErr == nil {
			if reapErr := service.driver.Reap(ctx, record.ContainerID, true); reapErr != nil {
				return cleaned, reapErr
			}
		}
		if err := service.cleanupFiles(record); err != nil {
			return cleaned, err
		}
		delete(service.records, attemptID)
		cleaned++
	}
	orphans, err := service.cleanupOrphanState()
	return cleaned + orphans, err
}

func (service *Service) cleanupRecord(ctx context.Context, record sandboxRecord, force bool) error {
	reapErr := service.driver.Reap(ctx, record.ContainerID, force)
	if reapErr != nil {
		return ErrRuntimeUnavailable
	}
	return service.cleanupFiles(record)
}
func (service *Service) cleanupFiles(record sandboxRecord) error {
	if intake, ok := service.intakes[record.Descriptor.Attempt.AttemptID]; ok {
		_ = intake.Close()
		delete(service.intakes, record.Descriptor.Attempt.AttemptID)
	}
	artifactErr := service.artifacts.CleanupAttempt(record.Descriptor.Attempt.AttemptID)
	scratchErr := safeRemoveTree(filepath.Join(service.config.StateRoot, "scratch"),
		filepath.Join(service.config.StateRoot, "scratch", record.Descriptor.Attempt.AttemptID))
	path := service.recordPath(record.Descriptor.Attempt.AttemptID)
	if artifactErr != nil || scratchErr != nil {
		return ErrRuntimeUnavailable
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func (service *Service) isCurrentAttempt(attempt AttemptIdentity) bool {
	service.mu.Lock()
	defer service.mu.Unlock()
	record, ok := service.records[attempt.AttemptID]
	return ok && record.Descriptor.Attempt == attempt && service.now().Before(record.WallDeadline)
}
func (service *Service) persistRecord(record sandboxRecord) error {
	body, err := json.Marshal(record)
	if err != nil {
		return err
	}
	path := service.recordPath(record.Descriptor.Attempt.AttemptID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	file, err := os.OpenFile(tmp, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	file.Close()
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}
func (service *Service) recordPath(attemptID string) string {
	return filepath.Join(service.config.StateRoot, "sandboxes", attemptID+".json")
}
func (service *Service) loadRecords() error {
	directory := filepath.Join(service.config.StateRoot, "sandboxes")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return err
		}
		var record sandboxRecord
		if strictjson.Decode(body, 64<<10, &record) != nil || !validID(record.Descriptor.Attempt.AttemptID, "attempt") ||
			record.WallDeadline.IsZero() || validateLaunchPlan(record.Plan) != nil ||
			!recordMatchesPlan(record) ||
			!validDriverSandbox(DriverSandbox{ContainerID: record.ContainerID, SandboxID: record.Descriptor.SandboxID, Attempt: record.Descriptor.Attempt, SnapshotFingerprint: record.Descriptor.SnapshotFingerprint, SpecFingerprint: record.Descriptor.SpecFingerprint, ProbeFingerprint: record.Descriptor.ProbeFingerprint}) {
			return ErrRuntimeUnavailable
		}
		service.records[record.Descriptor.Attempt.AttemptID] = record
		if err := service.artifacts.AuthorizeAttempt(record.Descriptor.Attempt,
			record.Plan.Sandbox.Resources.OutputBytes); err != nil {
			return ErrRuntimeUnavailable
		}
		brokerPath := filepath.Join(service.config.StateRoot, "scratch", record.Descriptor.Attempt.AttemptID, "broker")
		if err := os.Chmod(brokerPath, 0o711); err != nil {
			return ErrRuntimeUnavailable
		}
		intake, intakeErr := StartArtifactIntake(filepath.Join(brokerPath, "artifact.sock"),
			service.artifacts, record.Descriptor.Attempt, service.isCurrentAttempt)
		if intakeErr != nil {
			return ErrRuntimeUnavailable
		}
		service.intakes[record.Descriptor.Attempt.AttemptID] = intake
		service.scheduleWallDeadline(record)
	}
	return nil
}

func recordMatchesPlan(record sandboxRecord) bool {
	return record.ContainerID != "" && record.Descriptor.SandboxID == record.Plan.SandboxID &&
		record.Descriptor.Attempt == record.Plan.Attempt &&
		record.Descriptor.SnapshotFingerprint == record.Plan.SnapshotFingerprint &&
		record.Descriptor.SpecFingerprint == record.Plan.SpecFingerprint &&
		record.Descriptor.ProbeFingerprint == record.Plan.ProbeFingerprint
}

func (service *Service) scheduleWallDeadline(record sandboxRecord) {
	delay := time.Until(record.WallDeadline)
	if delay < 0 {
		delay = 0
	}
	time.AfterFunc(delay, func() {
		service.mu.Lock()
		defer service.mu.Unlock()
		current, ok := service.records[record.Descriptor.Attempt.AttemptID]
		if !ok || current.ContainerID != record.ContainerID || current.WallDeadline != record.WallDeadline {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if service.cleanupRecord(ctx, current, true) == nil {
			delete(service.records, record.Descriptor.Attempt.AttemptID)
		}
	})
}

func (service *Service) cleanupOrphanState() (int, error) {
	active := make(map[string]struct{}, len(service.records))
	for attemptID := range service.records {
		active[attemptID] = struct{}{}
	}
	cleaned := 0
	for _, root := range []string{filepath.Join(service.config.StateRoot, "scratch"), service.artifacts.root} {
		entries, err := os.ReadDir(root)
		if err != nil {
			return cleaned, err
		}
		for _, entry := range entries {
			path := filepath.Join(root, entry.Name())
			_, retained := active[entry.Name()]
			if retained && entry.IsDir() {
				continue
			}
			if entry.IsDir() {
				if err := safeRemoveTree(root, path); err != nil {
					return cleaned, err
				}
			} else if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return cleaned, err
			}
			cleaned++
		}
	}
	return cleaned, nil
}

func (service *Service) response(request Request, method string, body any) Response {
	return Response{SchemaVersion: ProtocolVersion, Method: method, RequestID: request.RequestID, SentAt: service.now().UTC(), Nonce: request.Nonce, Body: body}
}
func (service *Service) errorResponse(request Request, err error) Response {
	if request.Method == MethodPrepare {
		return service.response(request, MethodPrepare+".result", PrepareResult{Error: rpcError(err)})
	}
	if request.Method == MethodCommit && request.Commit != nil {
		return service.response(request, MethodCommit+".result", CommitResult{Outcome: "rejected",
			IdempotencyKey: request.Commit.IdempotencyKey, Error: rpcError(err)})
	}
	return service.response(request, request.Method+".result", ErrorResult{Accepted: false,
		Error: RPCError{Code: ErrorCode(err), Retryable: false}})
}

func responseOperationError(response Response) error {
	var rpcError *RPCError
	switch body := response.Body.(type) {
	case ErrorResult:
		rpcError = &body.Error
	case ProbeResult:
		rpcError = body.Error
	case LaunchResult:
		rpcError = body.Error
	case HeartbeatResult:
		rpcError = body.Error
	case CancelResult:
		rpcError = body.Error
	case PrepareResult:
		rpcError = body.Error
	case CommitResult:
		rpcError = body.Error
	}
	if rpcError == nil {
		return nil
	}
	return rpcCodeError(rpcError.Code)
}
func (service *Service) launchError(request Request, attempt AttemptRef, err error) (Response, error) {
	return service.response(request, "launch.result", LaunchResult{Accepted: false, Attempt: attempt.Identity(), Error: &RPCError{Code: ErrorCode(err), Retryable: false}}), err
}
func (service *Service) heartbeatError(request Request, err error) (Response, error) {
	return service.response(request, "heartbeat.result", HeartbeatResult{Accepted: false, KillRequested: errors.Is(err, ErrKillSwitchActive), Error: &RPCError{Code: ErrorCode(err), Retryable: false}}), err
}
func (service *Service) cancelError(request Request, err error) (Response, error) {
	return service.response(request, "cancel.result", CancelResult{Accepted: false, ObservedTerminal: "none", Error: &RPCError{Code: ErrorCode(err), Retryable: false}}), err
}
func deterministicSandboxID(attempt AttemptRef) string {
	digest := sha256Bytes("neo-runner-sandbox-id-v1\x00" + attempt.AttemptID + "\x00" + fmt.Sprint(attempt.LeaseGeneration))
	return "sandbox_" + hex.EncodeToString(digest[:16])
}
func sha256Bytes(value string) [32]byte { return sha256.Sum256([]byte(value)) }
func sameSandboxIdentity(left, right DriverSandbox) bool {
	return left.ContainerID == right.ContainerID && left.SandboxID == right.SandboxID && left.Attempt == right.Attempt && left.SnapshotFingerprint == right.SnapshotFingerprint && left.SpecFingerprint == right.SpecFingerprint && left.ProbeFingerprint == right.ProbeFingerprint
}
