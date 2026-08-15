package agentlearningworker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"neo-chat/mm-chat/backend/internal/agentlearning"
	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

var ErrUnavailable = errors.New("DRAFT_LEARNING_RUNNER_UNAVAILABLE")

var requiredFeatures = []string{
	"rootless_userns", "cgroup_v2", "seccomp", "readonly_rootfs",
	"snapshot_workspace", "network_none", "pidfd_kill", "cgroup_reap",
}

type AttemptRepository interface {
	Begin(context.Context, BeginInput) (BeginResult, error)
	RecordLaunch(context.Context, agentrunner.AttemptRef, string, string, string) error
	RecordResult(context.Context, agentrunner.AttemptRef, RunnerResult) error
	MarkCancelPending(context.Context, agentrunner.AttemptRef, string) error
	CompleteCleanup(context.Context, agentrunner.AttemptRef, bool, string) error
	RunnerInventory(context.Context, int) ([]InventoryAttempt, error)
	ReconcileRunner(context.Context, time.Time, int) (int, error)
	PruneRunner(context.Context, time.Time, int) (int, int, int, error)
}

type RecoveryResult struct {
	Inventory int
	Marked    int
	Reaped    int
	Completed int
}

type AuthorityService interface {
	IssueAuthority(context.Context, agentrunner.IssueAuthorityInput) (agentrunner.AuthorityTicket, json.RawMessage, error)
	CompleteRequest(context.Context, string, string, string, string, []byte) error
}

type RunnerClient interface {
	Call(context.Context, agentrunner.Request) (agentrunner.Response, error)
}

type Runtime struct {
	plan       Plan
	repository AttemptRepository
	authority  AuthorityService
	client     RunnerClient
	now        func() time.Time
	newID      func(string) string
	newToken   func() (string, error)
}

type kindChecker struct {
	runtime *Runtime
	kind    string
}

func NewRuntime(
	plan Plan,
	repository AttemptRepository,
	authority AuthorityService,
	client RunnerClient,
) (*Runtime, error) {
	if ValidatePlan(plan) != nil || repository == nil || authority == nil || client == nil {
		return nil, ErrInvalidPlan
	}
	return &Runtime{
		plan: plan, repository: repository, authority: authority, client: client,
		now: time.Now,
		newID: func(prefix string) string {
			return prefix + "_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		},
		newToken: randomLeaseToken,
	}, nil
}

func (runtime *Runtime) IsolationChecker() agentlearning.Checker {
	return kindChecker{runtime: runtime, kind: agentlearning.CheckIsolation}
}

func (runtime *Runtime) EvaluationChecker() agentlearning.Checker {
	return kindChecker{runtime: runtime, kind: agentlearning.CheckEvaluation}
}

// Reconcile removes only exact expired Draft-check Sandboxes. The Runner
// reconcile request preserves every unrelated descriptor returned by list, so
// this lifecycle-only caller cannot accidentally reap Root/Child/Broker work.
// Lost lease tokens are never reconstructed: cleanup waits for both the
// durable attempt lease and the latest signed authority to expire.
func (runtime *Runtime) Reconcile(
	ctx context.Context,
	now time.Time,
	limit int,
) (RecoveryResult, error) {
	if runtime == nil || runtime.repository == nil || now.IsZero() || limit < 1 || limit > 1000 {
		return RecoveryResult{}, ErrUnavailable
	}
	now = now.UTC()
	marked, err := runtime.repository.ReconcileRunner(ctx, now, limit)
	if err != nil {
		return RecoveryResult{}, ErrUnavailable
	}
	inventory, err := runtime.repository.RunnerInventory(ctx, limit)
	if err != nil {
		return RecoveryResult{}, ErrUnavailable
	}
	result := RecoveryResult{Inventory: len(inventory), Marked: marked}
	listed, err := runtime.list(ctx, now)
	if err != nil {
		return result, err
	}
	byAttempt := make(map[string]agentrunner.SandboxDescriptor, len(listed))
	for _, descriptor := range listed {
		byAttempt[descriptor.Attempt.AttemptID] = descriptor
	}
	stale := make(map[string]InventoryAttempt)
	for _, item := range inventory {
		descriptor, exists := byAttempt[item.Attempt.AttemptID]
		if exists && !runtime.matchesInventory(item, descriptor) {
			// An exact Attempt ID with drifted immutable bindings is residue, but
			// it is still reaped only after all old authority is dead.
			exists = true
		}
		if now.Before(item.LeaseExpiresAt) ||
			(item.AuthorityExpiresAt != nil && now.Before(*item.AuthorityExpiresAt)) {
			continue
		}
		if item.State == "leased" && exists {
			if !runtime.matchesRecoverableLaunch(item, descriptor) {
				return result, ErrUnavailable
			}
			attempt := runtime.inventoryAttempt(item)
			if err := runtime.repository.RecordLaunch(ctx, attempt, descriptor.SandboxID,
				descriptor.SpecFingerprint, descriptor.ProbeFingerprint); err != nil {
				return result, ErrUnavailable
			}
			if err := runtime.repository.MarkCancelPending(ctx, attempt,
				"RUNNER_RESTART_RECOVERY"); err != nil {
				return result, ErrUnavailable
			}
			item.State, item.CleanupState = "cancel_pending", "pending"
		}
		if item.State == "leased" && !exists {
			attempt := runtime.inventoryAttempt(item)
			if err := runtime.repository.MarkCancelPending(ctx, attempt,
				"RUNNER_RESTART_NO_SANDBOX"); err != nil {
				return result, ErrUnavailable
			}
			result.Completed++
			continue
		}
		if item.State == "launched" || item.State == "result_ready" ||
			item.State == "cancel_pending" {
			stale[item.Attempt.AttemptID] = item
		}
	}
	if len(stale) == 0 {
		return result, nil
	}
	expected := make([]agentrunner.SandboxDescriptor, 0, len(listed))
	for _, descriptor := range listed {
		if _, remove := stale[descriptor.Attempt.AttemptID]; !remove {
			expected = append(expected, descriptor)
		}
	}
	cleaned, err := runtime.reconcileRunner(ctx, now, expected)
	if err != nil {
		return result, err
	}
	result.Reaped = cleaned
	after, err := runtime.list(ctx, runtime.now().UTC())
	if err != nil {
		return result, err
	}
	remaining := make(map[string]struct{}, len(after))
	for _, descriptor := range after {
		remaining[descriptor.Attempt.AttemptID] = struct{}{}
	}
	for attemptID, item := range stale {
		if _, exists := remaining[attemptID]; exists {
			continue
		}
		attempt := runtime.inventoryAttempt(item)
		if item.State != "cancel_pending" {
			if err := runtime.repository.MarkCancelPending(ctx, attempt,
				"RUNNER_RESTART_RECOVERY"); err != nil {
				return result, ErrUnavailable
			}
		}
		if err := runtime.repository.CompleteCleanup(ctx, attempt, true, ""); err != nil {
			return result, ErrUnavailable
		}
		result.Completed++
	}
	return result, nil
}

func (runtime *Runtime) Prune(ctx context.Context, cutoff time.Time, limit int) error {
	if runtime == nil || cutoff.IsZero() || limit < 1 || limit > 1000 {
		return ErrUnavailable
	}
	_, _, _, err := runtime.repository.PruneRunner(ctx, cutoff.UTC(), limit)
	if err != nil {
		return ErrUnavailable
	}
	return nil
}

func (runtime *Runtime) Residue(ctx context.Context, limit int) (int, error) {
	if runtime == nil || limit < 1 || limit > 1000 {
		return 0, ErrUnavailable
	}
	inventory, err := runtime.repository.RunnerInventory(ctx, limit)
	if err != nil {
		return 0, ErrUnavailable
	}
	return len(inventory), nil
}

func (runtime *Runtime) list(ctx context.Context, now time.Time) ([]agentrunner.SandboxDescriptor, error) {
	request, err := agentrunner.NewRequest(agentrunner.MethodList,
		agentrunner.ListRequest{RunnerID: runtime.plan.RunnerID}, now)
	if err != nil {
		return nil, ErrUnavailable
	}
	response, err := runtime.client.Call(ctx, request)
	value, ok := response.Body.(agentrunner.ListResult)
	if err != nil || !ok || value.RunnerID != runtime.plan.RunnerID || value.Sandboxes == nil {
		return nil, ErrUnavailable
	}
	return value.Sandboxes, nil
}

func (runtime *Runtime) reconcileRunner(ctx context.Context, now time.Time,
	expected []agentrunner.SandboxDescriptor,
) (int, error) {
	request, err := agentrunner.NewRequest(agentrunner.MethodReconcile,
		agentrunner.ReconcileRequest{RunnerID: runtime.plan.RunnerID, Expected: expected}, now)
	if err != nil {
		return 0, ErrUnavailable
	}
	response, err := runtime.client.Call(ctx, request)
	value, ok := response.Body.(agentrunner.ReconcileResult)
	if err != nil || !ok || value.RunnerID != runtime.plan.RunnerID || value.Cleaned < 0 {
		return 0, ErrUnavailable
	}
	return value.Cleaned, nil
}

func (runtime *Runtime) inventoryAttempt(item InventoryAttempt) agentrunner.AttemptRef {
	return agentrunner.AttemptRef{RunID: item.Attempt.RunID, StepID: item.Attempt.StepID,
		AttemptID: item.Attempt.AttemptID, LeaseGeneration: item.Attempt.LeaseGeneration,
		LeaseOwner: item.LeaseOwner, LeaseToken: "lost_after_restart"}
}

func (runtime *Runtime) matchesInventory(item InventoryAttempt,
	descriptor agentrunner.SandboxDescriptor,
) bool {
	return descriptor.Attempt == item.Attempt && descriptor.SnapshotFingerprint == item.SnapshotFingerprint &&
		(item.SandboxID == "" || descriptor.SandboxID == item.SandboxID) &&
		(item.SpecFingerprint == "" || descriptor.SpecFingerprint == item.SpecFingerprint) &&
		(item.ProbeFingerprint == "" || descriptor.ProbeFingerprint == item.ProbeFingerprint)
}

func (runtime *Runtime) matchesRecoverableLaunch(item InventoryAttempt,
	descriptor agentrunner.SandboxDescriptor,
) bool {
	check, ok := runtime.plan.Check(item.Kind)
	if !ok || descriptor.Attempt != item.Attempt ||
		descriptor.SnapshotFingerprint != runtime.plan.RunnerSnapshotFingerprint {
		return false
	}
	launch := agentrunner.LaunchRequest{Attempt: runtime.inventoryAttempt(item),
		Lineage: agentrunner.RunLineage{RootRunID: item.Attempt.RunID, Depth: 0},
		GrantID: runtime.plan.GrantID, GrantFingerprint: runtime.plan.GrantFingerprint,
		SnapshotFingerprint: runtime.plan.RunnerSnapshotFingerprint,
		Sandbox:             runtime.plan.Sandbox, ToolRegistry: runtime.plan.ToolRegistry,
		Argv: append([]string(nil), check.Argv...)}
	return descriptor.SpecFingerprint == agentrunner.SandboxFingerprint(launch) &&
		fingerprintRE.MatchString(descriptor.ProbeFingerprint)
}

func (checker kindChecker) Check(ctx context.Context, input agentlearning.CheckInput) (agentlearning.CheckResult, error) {
	if checker.runtime == nil {
		return agentlearning.CheckResult{}, ErrUnavailable
	}
	return checker.runtime.check(ctx, checker.kind, input)
}

func (runtime *Runtime) check(
	ctx context.Context,
	kind string,
	input agentlearning.CheckInput,
) (result agentlearning.CheckResult, resultErr error) {
	check, ok := runtime.plan.Check(kind)
	archiveDigest := sha256.Sum256(input.Archive)
	if !ok || input.Draft.ID != runtime.plan.DraftID || input.Draft.UserID != runtime.plan.UserID ||
		input.Draft.DraftFingerprint != runtime.plan.DraftFingerprint ||
		input.Draft.Spec.ProposedPackageFingerprint != runtime.plan.ProposedPackageFingerprint ||
		input.Draft.Spec.RuntimeBundleFingerprint != runtime.plan.RuntimeBundleFingerprint ||
		input.Draft.Spec.ArchiveFingerprint != runtime.plan.ArchiveFingerprint ||
		"sha256:"+hex.EncodeToString(archiveDigest[:]) != runtime.plan.ArchiveFingerprint ||
		input.Draft.CheckGeneration < 1 || input.Draft.CheckOwner == "" || input.Draft.CheckExpiresAt == nil {
		return agentlearning.CheckResult{}, ErrUnavailable
	}

	now := runtime.now().UTC()
	probe, err := runtime.probe(ctx, now)
	if err != nil {
		return agentlearning.CheckResult{}, err
	}
	token, err := runtime.newToken()
	if err != nil {
		return agentlearning.CheckResult{}, ErrUnavailable
	}
	attempt := agentrunner.AttemptRef{
		RunID: runtime.newID("run"), StepID: runtime.newID("step"),
		AttemptID: runtime.newID("attempt"), LeaseGeneration: 1,
		LeaseOwner: runtime.plan.RunnerID, LeaseToken: token,
	}
	begin, err := runtime.repository.Begin(ctx, BeginInput{
		DraftID: input.Draft.ID, ClaimOwner: input.Draft.CheckOwner,
		CheckGeneration: input.Draft.CheckGeneration, Kind: kind, Attempt: attempt,
		SnapshotFingerprint: runtime.plan.RunnerSnapshotFingerprint, Now: now,
		LeaseDuration: time.Duration(runtime.plan.LeaseSeconds) * time.Second,
	})
	if err != nil || !begin.Created || begin.AttemptID != attempt.AttemptID || begin.State != "leased" {
		return agentlearning.CheckResult{}, ErrUnavailable
	}
	launched := false
	cleanupComplete := false
	defer func() {
		if resultErr == nil || cleanupComplete {
			return
		}
		reason := "RUNNER_CHECK_FAILED"
		if launched {
			cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
			cancelErr := runtime.cancel(cancelCtx, attempt, reason)
			cancel()
			_ = runtime.repository.MarkCancelPending(context.WithoutCancel(ctx), attempt, reason)
			if cancelErr == nil {
				_ = runtime.repository.CompleteCleanup(context.WithoutCancel(ctx), attempt, true, "")
				return
			}
			_ = runtime.repository.CompleteCleanup(context.WithoutCancel(ctx), attempt, false, "RUNNER_CANCEL_FAILED")
			return
		}
		_ = runtime.repository.MarkCancelPending(context.WithoutCancel(ctx), attempt, reason)
	}()

	launch := agentrunner.LaunchRequest{
		Attempt: attempt,
		Lineage: agentrunner.RunLineage{RootRunID: attempt.RunID, Depth: 0},
		GrantID: runtime.plan.GrantID, GrantFingerprint: runtime.plan.GrantFingerprint,
		SnapshotFingerprint: runtime.plan.RunnerSnapshotFingerprint,
		Sandbox:             runtime.plan.Sandbox, ToolRegistry: runtime.plan.ToolRegistry,
		Argv: append([]string(nil), check.Argv...),
	}
	response, _, err := runtime.authorizedCall(ctx, agentrunner.MethodLaunch, launch, attempt, now)
	launchResult, validLaunch := response.Body.(agentrunner.LaunchResult)
	if err != nil || !validLaunch || !launchResult.Accepted || launchResult.SandboxID == "" ||
		launchResult.Attempt != attempt.Identity() {
		return agentlearning.CheckResult{}, ErrUnavailable
	}
	launched = true
	if err := runtime.repository.RecordLaunch(ctx, attempt, launchResult.SandboxID,
		agentrunner.SandboxFingerprint(launch), probe.ProbeFingerprint); err != nil {
		return agentlearning.CheckResult{}, ErrUnavailable
	}

	resultResponse, err := runtime.waitResult(ctx, attempt, now)
	if err != nil {
		return agentlearning.CheckResult{}, err
	}
	payload, err := runtime.validateResult(kind, check, input.Draft, attempt, resultResponse)
	if err != nil {
		return agentlearning.CheckResult{}, err
	}
	if err := runtime.repository.RecordResult(ctx, attempt, RunnerResult{
		ArtifactName: resultResponse.Receipt.Name, ArtifactBytes: resultResponse.Receipt.Size,
		ArtifactFingerprint: resultResponse.Receipt.Fingerprint,
		Payload:             append(json.RawMessage(nil), resultResponse.Payload...),
	}); err != nil {
		return agentlearning.CheckResult{}, ErrUnavailable
	}
	if err := runtime.cancel(ctx, attempt, "RUNNER_CHECK_COMPLETE"); err != nil {
		return agentlearning.CheckResult{}, ErrUnavailable
	}
	if err := runtime.repository.CompleteCleanup(ctx, attempt, true, ""); err != nil {
		return agentlearning.CheckResult{}, ErrUnavailable
	}
	cleanupComplete = true
	return agentlearning.CheckResult{
		Status: payload.Status, ReasonCode: payload.ReasonCode,
		SuiteFingerprint:    payload.SuiteFingerprint,
		EvidenceFingerprint: resultResponse.Receipt.Fingerprint,
		DurationMillis:      payload.DurationMillis, Metrics: payload.Metrics,
	}, nil
}

func (runtime *Runtime) probe(ctx context.Context, now time.Time) (agentrunner.ProbeResult, error) {
	request, err := agentrunner.NewRequest(agentrunner.MethodProbe,
		agentrunner.ProbeRequest{RequiredFeatures: append([]string(nil), requiredFeatures...)}, now)
	if err != nil {
		return agentrunner.ProbeResult{}, ErrUnavailable
	}
	response, err := runtime.client.Call(ctx, request)
	result, ok := response.Body.(agentrunner.ProbeResult)
	if err != nil || !ok || !result.Ready || !fingerprintRE.MatchString(result.ProbeFingerprint) {
		return agentrunner.ProbeResult{}, ErrUnavailable
	}
	return result, nil
}

func (runtime *Runtime) waitResult(
	ctx context.Context,
	attempt agentrunner.AttemptRef,
	started time.Time,
) (agentrunner.ResultResult, error) {
	deadline := started.Add(time.Duration(runtime.plan.ResultTimeoutSeconds) * time.Second)
	for {
		now := runtime.now().UTC()
		if !now.Before(deadline) {
			return agentrunner.ResultResult{}, ErrUnavailable
		}
		body := agentrunner.ResultRequest{Attempt: attempt,
			SnapshotFingerprint: runtime.plan.RunnerSnapshotFingerprint, Name: ResultArtifact}
		response, _, err := runtime.authorizedCall(ctx, agentrunner.MethodResult, body, attempt, now)
		value, ok := response.Body.(agentrunner.ResultResult)
		if err != nil || !ok || value.Error != nil {
			return agentrunner.ResultResult{}, ErrUnavailable
		}
		if value.Ready {
			if value.Receipt == nil || len(value.Payload) < 2 {
				return agentrunner.ResultResult{}, ErrUnavailable
			}
			return value, nil
		}
		timer := time.NewTimer(time.Duration(runtime.plan.ResultPollMillis) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return agentrunner.ResultResult{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func (runtime *Runtime) cancel(
	ctx context.Context,
	attempt agentrunner.AttemptRef,
	reason string,
) error {
	body := agentrunner.CancelRequest{Attempt: attempt, Mode: "cancel", ReasonCode: strings.ToLower(reason)}
	response, _, err := runtime.authorizedCall(ctx, agentrunner.MethodCancel, body, attempt, runtime.now().UTC())
	result, ok := response.Body.(agentrunner.CancelResult)
	if err != nil || !ok || !result.Accepted || result.ObservedTerminal == "" {
		return ErrUnavailable
	}
	return nil
}

func (runtime *Runtime) authorizedCall(
	ctx context.Context,
	method string,
	body any,
	attempt agentrunner.AttemptRef,
	now time.Time,
) (agentrunner.Response, agentrunner.Request, error) {
	unsigned, err := agentrunner.NewRequest(method, body, now)
	if err != nil {
		return agentrunner.Response{}, agentrunner.Request{}, ErrUnavailable
	}
	fingerprint := agentrunner.AuthorityRequestFingerprint(method, body)
	ticket, replay, err := runtime.authority.IssueAuthority(ctx, agentrunner.IssueAuthorityInput{
		CallerIdentity: runtime.plan.CallerIdentity, RunnerID: runtime.plan.RunnerID,
		RequestID: unsigned.RequestID, Nonce: unsigned.Nonce, Method: method,
		RequestFingerprint: fingerprint, UserID: runtime.plan.UserID, Attempt: attempt,
		SnapshotFingerprint: runtime.plan.RunnerSnapshotFingerprint, TTL: 15 * time.Second,
	})
	if err != nil || len(replay) != 0 {
		return agentrunner.Response{}, agentrunner.Request{}, ErrUnavailable
	}
	signed, err := agentrunner.BindAuthority(unsigned, ticket)
	if err != nil {
		return agentrunner.Response{}, agentrunner.Request{}, ErrUnavailable
	}
	response, operationErr := runtime.client.Call(ctx, signed)
	if response.SchemaVersion != "" {
		encoded, marshalErr := json.Marshal(response)
		if marshalErr != nil || runtime.authority.CompleteRequest(ctx,
			runtime.plan.CallerIdentity, signed.RequestID, signed.Nonce,
			fingerprint, encoded) != nil {
			return agentrunner.Response{}, signed, ErrUnavailable
		}
	}
	if operationErr != nil {
		return response, signed, fmt.Errorf("%w: %s", ErrUnavailable, agentrunner.ErrorCode(operationErr))
	}
	return response, signed, nil
}

type resultPayload struct {
	SchemaVersion              string           `json:"schemaVersion"`
	ActivationID               string           `json:"activationId"`
	DraftID                    string           `json:"draftId"`
	DraftFingerprint           string           `json:"draftFingerprint"`
	CheckGeneration            int64            `json:"checkGeneration"`
	Kind                       string           `json:"kind"`
	ProposedPackageFingerprint string           `json:"proposedPackageFingerprint"`
	RuntimeBundleFingerprint   string           `json:"runtimeBundleFingerprint"`
	ArchiveFingerprint         string           `json:"archiveFingerprint"`
	WorkspaceSnapshotID        string           `json:"workspaceSnapshotId"`
	WorkspaceFingerprint       string           `json:"workspaceFingerprint"`
	SuiteFingerprint           string           `json:"suiteFingerprint"`
	Status                     string           `json:"status"`
	ReasonCode                 string           `json:"reasonCode"`
	DurationMillis             int64            `json:"durationMillis"`
	Metrics                    map[string]int64 `json:"metrics"`
}

func (runtime *Runtime) validateResult(
	kind string,
	check CheckPlan,
	draft agentlearning.Draft,
	attempt agentrunner.AttemptRef,
	result agentrunner.ResultResult,
) (resultPayload, error) {
	payloadDigest := sha256.Sum256(result.Payload)
	if result.Receipt == nil || result.Receipt.Attempt != attempt.Identity() ||
		result.Receipt.Name != ResultArtifact || result.Receipt.MediaType != "application/json" ||
		result.Receipt.Size < 2 || result.Receipt.Size > 65536 ||
		result.Receipt.Size != int64(len(result.Payload)) ||
		result.Receipt.Fingerprint != "sha256:"+hex.EncodeToString(payloadDigest[:]) {
		return resultPayload{}, ErrUnavailable
	}
	var payload resultPayload
	if strictjson.Decode(result.Payload, 65536, &payload) != nil ||
		payload.SchemaVersion != ResultVersion || payload.ActivationID != runtime.plan.ActivationID ||
		payload.DraftID != draft.ID || payload.DraftFingerprint != draft.DraftFingerprint ||
		payload.CheckGeneration != draft.CheckGeneration || payload.Kind != kind ||
		payload.ProposedPackageFingerprint != runtime.plan.ProposedPackageFingerprint ||
		payload.RuntimeBundleFingerprint != runtime.plan.RuntimeBundleFingerprint ||
		payload.ArchiveFingerprint != runtime.plan.ArchiveFingerprint ||
		payload.WorkspaceSnapshotID != runtime.plan.Sandbox.WorkspaceSnapshotID ||
		payload.WorkspaceFingerprint != runtime.plan.Sandbox.WorkspaceFingerprint ||
		payload.SuiteFingerprint != check.SuiteFingerprint ||
		(payload.Status != agentlearning.CheckPassed && payload.Status != agentlearning.CheckFailed) ||
		!regexpReason(payload.ReasonCode) || payload.DurationMillis < 0 ||
		payload.DurationMillis > 86_400_000 || len(payload.Metrics) > 16 {
		return resultPayload{}, ErrUnavailable
	}
	for key, value := range payload.Metrics {
		if len(key) < 1 || len(key) > 64 || value < 0 || value > 1<<53 {
			return resultPayload{}, ErrUnavailable
		}
		for index, character := range key {
			if !(character >= 'a' && character <= 'z') &&
				!(index > 0 && character >= 'A' && character <= 'Z') &&
				!(index > 0 && character >= '0' && character <= '9') {
				return resultPayload{}, ErrUnavailable
			}
		}
	}
	return payload, nil
}

func regexpReason(value string) bool {
	if len(value) < 1 || len(value) > 64 || value[0] < 'A' || value[0] > 'Z' {
		return false
	}
	for _, character := range value[1:] {
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func randomLeaseToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "lease_" + base64.RawURLEncoding.EncodeToString(raw), nil
}
