package agentbroker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Service struct {
	repository   Repository
	executors    map[string]EffectExecutor
	readers      map[string]ReadOnlyExecutor
	secretBroker *SecretBroker
	now          func() time.Time
}

func NewService(repository Repository, executors map[string]EffectExecutor, secretBrokers ...*SecretBroker) (*Service, error) {
	return NewServiceWithReadOnly(repository, executors, nil, secretBrokers...)
}

func NewServiceWithReadOnly(repository Repository, executors map[string]EffectExecutor,
	readers map[string]ReadOnlyExecutor, secretBrokers ...*SecretBroker,
) (*Service, error) {
	if repository == nil {
		return nil, ErrDatabaseRequired
	}
	if len(secretBrokers) > 1 || len(secretBrokers) == 1 && secretBrokers[0] == nil {
		return nil, ErrInvalidInput
	}
	copyExecutors := make(map[string]EffectExecutor, len(executors))
	for identity, executor := range executors {
		if !validIdentifier(identity) || executor == nil {
			return nil, ErrInvalidInput
		}
		copyExecutors[identity] = executor
	}
	copyReaders := make(map[string]ReadOnlyExecutor, len(readers))
	for identity, reader := range readers {
		if !validIdentifier(identity) || reader == nil || copyExecutors[identity] != nil {
			return nil, ErrInvalidInput
		}
		copyReaders[identity] = reader
	}
	service := &Service{repository: repository, executors: copyExecutors, readers: copyReaders, now: time.Now}
	if len(secretBrokers) == 1 {
		service.secretBroker = secretBrokers[0]
	}
	return service, nil
}

func (service *Service) Prepare(ctx context.Context, input PrepareInput) (PreparedIntent, error) {
	if service == nil || service.repository == nil || !validID(input.RequestID, "request") ||
		validateAttempt(input.Attempt) != nil || input.TTL < time.Minute || input.TTL > time.Hour ||
		input.Registry.RunID != input.Attempt.RunID ||
		input.Registry.Fingerprint != input.Attempt.RegistryFingerprint ||
		input.Grant.Run.RunID != input.Attempt.RunID ||
		input.Grant.Subject.UserID != input.Attempt.UserID ||
		input.Registry.Subject != input.Grant.Subject || input.Registry.GrantID != input.Grant.GrantID ||
		input.Registry.Depth != input.Grant.Run.Depth ||
		input.Registry.PackageFingerprint != input.Grant.PackageFingerprint ||
		input.Registry.RuntimeBundleFingerprint != input.Grant.RuntimeBundleFingerprint ||
		!input.Registry.ExpiresAt.Equal(input.Grant.ExpiresAt) {
		return PreparedIntent{}, ErrInvalidInput
	}
	now := service.now().UTC()
	if err := validateGrant(input.Grant, now); err != nil {
		return PreparedIntent{}, err
	}
	grantFingerprint, err := canonicalGrantFingerprint(input.Grant)
	if err != nil || grantFingerprint != input.Attempt.GrantFingerprint {
		return PreparedIntent{}, ErrSnapshotMismatch
	}
	tool, err := RegistryToolFor(input.Registry, input.ToolIdentity, input.Action, input.Resource)
	if err != nil {
		return PreparedIntent{}, err
	}
	if service.readers[input.ToolIdentity] != nil &&
		(tool.Classification != ClassificationRead || !tool.Idempotent || tool.Approval != ApprovalAutomatic) {
		return PreparedIntent{}, ErrGrantDenied
	}
	if tool.Classification != ClassificationRead && tool.Approval == ApprovalAutomatic {
		// Automatic authorization is allowed by the frozen Grant, but it still
		// produces an approval fact in PostgreSQL. It is not a direct write.
	} else if tool.Approval == ApprovalDenied {
		return PreparedIntent{}, ErrGrantDenied
	}
	arguments, err := canonicalJSON(input.Arguments, maxArgumentsBytes)
	if err != nil {
		return PreparedIntent{}, err
	}
	if input.BaseRevision != "" && !revisionPattern.MatchString(input.BaseRevision) {
		return PreparedIntent{}, ErrInvalidInput
	}
	intentID, err := randomID("intent", 16)
	if err != nil {
		return PreparedIntent{}, err
	}
	idempotencyKey, err := randomCommitKey()
	if err != nil {
		return PreparedIntent{}, err
	}
	argumentsFingerprint := fingerprint("neo-effect-arguments-v1", arguments)
	requestFingerprint := prepareRequestFingerprint(input, argumentsFingerprint)
	state := IntentAwaitingApproval
	if tool.Approval == ApprovalAutomatic {
		state = IntentApproved
	}
	expiresAt := now.Add(input.TTL)
	if input.Grant.ExpiresAt.Before(expiresAt) {
		expiresAt = input.Grant.ExpiresAt
	}
	prepared := PreparedIntent{
		SchemaVersion: IntentVersion, IntentID: intentID, RequestID: input.RequestID,
		RequestFingerprint: requestFingerprint, IdempotencyKey: idempotencyKey,
		Subject: input.Grant.Subject, RunID: input.Attempt.RunID, StepID: input.Attempt.StepID,
		AttemptID: input.Attempt.AttemptID, Generation: input.Attempt.Generation,
		LeaseOwner: input.Attempt.LeaseOwner, LeaseTokenDigest: tokenDigest(input.Attempt.LeaseToken),
		SnapshotFingerprint: input.Attempt.SnapshotFingerprint, GrantID: input.Grant.GrantID,
		GrantFingerprint: grantFingerprint, RegistryFingerprint: input.Registry.Fingerprint,
		ToolIdentity: tool.Identity, Capability: tool.Capability, Action: input.Action,
		Resource: input.Resource, ArgumentsFingerprint: argumentsFingerprint,
		CanonicalArguments: arguments, BaseRevision: input.BaseRevision,
		ApprovalClass: tool.Approval, State: state, KillSwitchEpoch: input.Attempt.KillSwitchEpoch,
		CapabilityMaxCalls: tool.MaxCalls, GrantMaxToolCalls: input.Grant.Budget.MaxToolCalls,
		CreatedAt: now, ExpiresAt: expiresAt,
	}
	prepared.IntentFingerprint = intentFingerprint(prepared)
	return service.repository.Prepare(ctx, prepared)
}

func prepareRequestFingerprint(input PrepareInput, argumentsFingerprint string) string {
	body, _ := json.Marshal(struct {
		UserID               string `json:"userId"`
		RunID                string `json:"runId"`
		StepID               string `json:"stepId"`
		AttemptID            string `json:"attemptId"`
		Generation           int64  `json:"generation"`
		LeaseOwner           string `json:"leaseOwner"`
		LeaseTokenDigest     string `json:"leaseTokenDigest"`
		SnapshotFingerprint  string `json:"snapshotFingerprint"`
		GrantFingerprint     string `json:"grantFingerprint"`
		RegistryFingerprint  string `json:"registryFingerprint"`
		ToolIdentity         string `json:"toolIdentity"`
		Action               string `json:"action"`
		Resource             string `json:"resource"`
		ArgumentsFingerprint string `json:"argumentsFingerprint"`
		BaseRevision         string `json:"baseRevision,omitempty"`
		TTLNanoseconds       int64  `json:"ttlNanoseconds"`
		KillSwitchEpoch      int64  `json:"killSwitchEpoch"`
	}{input.Attempt.UserID, input.Attempt.RunID, input.Attempt.StepID, input.Attempt.AttemptID,
		input.Attempt.Generation, input.Attempt.LeaseOwner, tokenDigest(input.Attempt.LeaseToken),
		input.Attempt.SnapshotFingerprint, input.Attempt.GrantFingerprint,
		input.Attempt.RegistryFingerprint, input.ToolIdentity, input.Action, input.Resource,
		argumentsFingerprint, input.BaseRevision, input.TTL.Nanoseconds(), input.Attempt.KillSwitchEpoch})
	return fingerprint("neo-effect-prepare-request-v1", body)
}

func intentFingerprint(intent PreparedIntent) string {
	body, _ := json.Marshal(struct {
		Subject              Subject `json:"subject"`
		RunID                string  `json:"runId"`
		StepID               string  `json:"stepId"`
		AttemptID            string  `json:"attemptId"`
		Generation           int64   `json:"generation"`
		LeaseOwner           string  `json:"leaseOwner"`
		LeaseTokenDigest     string  `json:"leaseTokenDigest"`
		SnapshotFingerprint  string  `json:"snapshotFingerprint"`
		GrantFingerprint     string  `json:"grantFingerprint"`
		RegistryFingerprint  string  `json:"registryFingerprint"`
		ToolIdentity         string  `json:"toolIdentity"`
		Capability           string  `json:"capability"`
		Action               string  `json:"action"`
		Resource             string  `json:"resource"`
		ArgumentsFingerprint string  `json:"argumentsFingerprint"`
		BaseRevision         string  `json:"baseRevision,omitempty"`
		ApprovalClass        string  `json:"approvalClass"`
		CapabilityMaxCalls   int     `json:"capabilityMaxCalls"`
		GrantMaxToolCalls    int     `json:"grantMaxToolCalls"`
		KillSwitchEpoch      int64   `json:"killSwitchEpoch"`
		ExpiresAt            string  `json:"expiresAt"`
	}{intent.Subject, intent.RunID, intent.StepID, intent.AttemptID, intent.Generation,
		intent.LeaseOwner, intent.LeaseTokenDigest,
		intent.SnapshotFingerprint, intent.GrantFingerprint, intent.RegistryFingerprint,
		intent.ToolIdentity, intent.Capability, intent.Action, intent.Resource,
		intent.ArgumentsFingerprint, intent.BaseRevision, intent.ApprovalClass,
		intent.CapabilityMaxCalls, intent.GrantMaxToolCalls,
		intent.KillSwitchEpoch, intent.ExpiresAt.UTC().Format(time.RFC3339Nano)})
	return fingerprint("neo-effect-intent-v1", body)
}

func (service *Service) DecideApproval(ctx context.Context, input ApprovalInput) (PreparedIntent, error) {
	if service == nil || service.repository == nil || !validID(input.ApprovalID, "approval") ||
		!validID(input.IntentID, "intent") || !validFingerprint(input.IntentFingerprint) ||
		!member(input.Decision, "approved", "denied") ||
		!member(input.ActorType, "user", "operator") || input.ActorID == "" || len(input.ActorID) > 128 ||
		(input.ActorType == "user" && input.ActorID != input.UserID) ||
		input.ReasonCode == "" || len(input.ReasonCode) > 64 || input.ExpectedRevision < 1 {
		return PreparedIntent{}, ErrInvalidInput
	}
	return service.repository.DecideApproval(ctx, input)
}

func (service *Service) RevokeGrant(ctx context.Context, input GrantRevocationInput) (bool, error) {
	if service == nil || service.repository == nil || !validID(input.GrantID, "grant") ||
		!validFingerprint(input.GrantFingerprint) || !member(input.ActorType, "operator", "system") ||
		input.ActorID == "" || len(input.ActorID) > 128 || strings.TrimSpace(input.ActorID) != input.ActorID ||
		!reasonPattern.MatchString(input.ReasonCode) {
		return false, ErrInvalidInput
	}
	created, err := service.repository.RevokeGrant(ctx, input)
	if err != nil {
		return false, err
	}
	if service.secretBroker != nil {
		service.secretBroker.revokeGrant(input.GrantID)
	}
	return created, nil
}

func (service *Service) Cancel(ctx context.Context, input CancelInput) (PreparedIntent, error) {
	if service == nil || service.repository == nil || !validID(input.CancellationID, "cancellation") ||
		!uuidPattern.MatchString(input.UserID) || !validID(input.IntentID, "intent") ||
		!validFingerprint(input.IntentFingerprint) || !member(input.ActorType, "user", "operator") ||
		input.ActorID == "" || len(input.ActorID) > 128 || strings.TrimSpace(input.ActorID) != input.ActorID ||
		(input.ActorType == "user" && input.ActorID != input.UserID) || !reasonPattern.MatchString(input.ReasonCode) {
		return PreparedIntent{}, ErrInvalidInput
	}
	intent, err := service.repository.Cancel(ctx, input)
	if err != nil {
		return PreparedIntent{}, err
	}
	if service.secretBroker != nil {
		service.secretBroker.clearIntent(intent.IntentID)
	}
	return intent, nil
}

func (service *Service) Commit(ctx context.Context, input CommitInput) (CommitResult, error) {
	if service == nil || service.repository == nil || validateAttempt(input.Attempt) != nil ||
		!validID(input.IntentID, "intent") || !validFingerprint(input.IntentFingerprint) ||
		(input.ApprovalID != "" && !validID(input.ApprovalID, "approval")) ||
		!strings.HasPrefix(input.IdempotencyKey, "commit_") || len(input.IdempotencyKey) > 160 {
		return CommitResult{}, ErrInvalidInput
	}
	claim, err := service.repository.ClaimCommit(ctx, input, tokenDigest(input.Attempt.LeaseToken))
	if err != nil {
		return CommitResult{}, err
	}
	if claim.Replay {
		return resultFromIntent(claim.Intent, true)
	}
	if reader := service.readers[claim.Intent.ToolIdentity]; reader != nil {
		return service.commitRead(ctx, claim.Intent, reader)
	}
	executor := service.executors[claim.Intent.ToolIdentity]
	if executor == nil {
		completed, completeErr := service.completeCommit(ctx, claim.Intent.IntentID,
			IntentFailed, "", "", "EXECUTOR_UNAVAILABLE")
		if completeErr != nil {
			return CommitResult{}, completeErr
		}
		return resultFromIntent(completed, false)
	}
	request := executionRequest(claim.Intent)
	receipt, dispatchErr := executor.Commit(ctx, request)
	if dispatchErr == nil {
		if !validFingerprint(receipt.ReceiptFingerprint) {
			dispatchErr = &DispatchError{Cause: ErrExecutorUnavailable, PossibleSend: true}
		} else {
			completed, completeErr := service.completeCommit(ctx, claim.Intent.IntentID,
				IntentCommitted, receipt.ReceiptFingerprint, digestOptional(receipt.StatusToken), "")
			if completeErr != nil {
				return CommitResult{}, completeErr
			}
			return resultFromIntent(completed, false)
		}
	}
	possibleSend := false
	var typed *DispatchError
	if errors.As(dispatchErr, &typed) {
		possibleSend = typed.PossibleSend
	}
	if !possibleSend {
		completed, completeErr := service.completeCommit(ctx, claim.Intent.IntentID,
			IntentFailed, "", "", "EXECUTOR_FAILED_BEFORE_SEND")
		if completeErr != nil {
			return CommitResult{}, completeErr
		}
		return resultFromIntent(completed, false)
	}
	status, statusErr := executor.Status(context.WithoutCancel(ctx), request)
	if statusErr == nil && status.Outcome == OutcomeCommitted && validFingerprint(status.ReceiptFingerprint) {
		completed, completeErr := service.completeCommit(ctx, claim.Intent.IntentID,
			IntentCommitted, status.ReceiptFingerprint, digestOptional(status.StatusToken), "")
		if completeErr != nil {
			return CommitResult{}, completeErr
		}
		return resultFromIntent(completed, false)
	}
	if statusErr == nil && status.Outcome == OutcomeRejected {
		completed, completeErr := service.completeCommit(ctx, claim.Intent.IntentID,
			IntentFailed, "", digestOptional(status.StatusToken), "EXECUTOR_REJECTED")
		if completeErr != nil {
			return CommitResult{}, completeErr
		}
		return resultFromIntent(completed, false)
	}
	completed, completeErr := service.completeCommit(ctx, claim.Intent.IntentID,
		IntentOutcomeUnknown, "", digestOptional(status.StatusToken), "OUTCOME_UNKNOWN")
	if completeErr != nil {
		return CommitResult{}, completeErr
	}
	result, _ := resultFromIntent(completed, false)
	return result, ErrOutcomeUnknown
}

func (service *Service) commitRead(ctx context.Context, intent PreparedIntent, reader ReadOnlyExecutor) (CommitResult, error) {
	result, readErr := reader.Read(ctx, executionRequest(intent))
	if readErr == nil {
		canonical, canonicalErr := canonicalJSON(result, maxArgumentsBytes)
		clear(result)
		if canonicalErr == nil {
			completed, completeErr := service.completeCommit(ctx, intent.IntentID, IntentCommitted,
				fingerprint("neo-read-executor-receipt-v1", canonical), "", "")
			clear(canonical)
			if completeErr != nil {
				return CommitResult{}, completeErr
			}
			return resultFromIntent(completed, false)
		}
		readErr = &DispatchError{Cause: ErrExecutorUnavailable, PossibleSend: true}
	}
	possibleSend := false
	var typed *DispatchError
	if errors.As(readErr, &typed) {
		possibleSend = typed.PossibleSend
	}
	if !possibleSend {
		completed, completeErr := service.completeCommit(ctx, intent.IntentID,
			IntentFailed, "", "", "EXECUTOR_FAILED_BEFORE_SEND")
		if completeErr != nil {
			return CommitResult{}, completeErr
		}
		return resultFromIntent(completed, false)
	}
	completed, completeErr := service.completeCommit(ctx, intent.IntentID,
		IntentOutcomeUnknown, "", "", "OUTCOME_UNKNOWN")
	if completeErr != nil {
		return CommitResult{}, completeErr
	}
	commitResult, _ := resultFromIntent(completed, false)
	return commitResult, ErrOutcomeUnknown
}

func (service *Service) completeCommit(ctx context.Context, intentID, state, receipt, statusDigest, code string) (PreparedIntent, error) {
	intent, err := service.repository.CompleteCommit(ctx, intentID, state, receipt, statusDigest, code)
	if err == nil && service.secretBroker != nil {
		service.secretBroker.clearIntent(intentID)
	}
	return intent, err
}

func executionRequest(intent PreparedIntent) ExecutionRequest {
	return ExecutionRequest{UserID: intent.Subject.UserID, RunID: intent.RunID, StepID: intent.StepID,
		AttemptID: intent.AttemptID, Generation: intent.Generation,
		SnapshotFingerprint: intent.SnapshotFingerprint, GrantFingerprint: intent.GrantFingerprint,
		RegistryFingerprint: intent.RegistryFingerprint,
		IntentID:            intent.IntentID, IdempotencyKey: intent.IdempotencyKey,
		ToolIdentity: intent.ToolIdentity, Capability: intent.Capability, Action: intent.Action,
		Resource: intent.Resource, Arguments: append(json.RawMessage(nil), intent.CanonicalArguments...),
		ArgumentsFingerprint: intent.ArgumentsFingerprint, BaseRevision: intent.BaseRevision}
}

func digestOptional(value string) string {
	if value == "" {
		return ""
	}
	return tokenDigest(value)
}

func resultFromIntent(intent PreparedIntent, replay bool) (CommitResult, error) {
	result := CommitResult{IdempotencyKey: intent.IdempotencyKey,
		ReceiptFingerprint: intent.ReceiptFingerprint, ErrorCode: intent.ErrorCode, Replay: replay}
	switch intent.State {
	case IntentCommitted:
		result.Outcome = OutcomeCommitted
		if replay {
			result.Outcome = OutcomeReplayed
		}
		return result, nil
	case IntentOutcomeUnknown:
		result.Outcome = OutcomeUnknown
		return result, ErrOutcomeUnknown
	case IntentFailed, IntentRejected, IntentCanceled, IntentExpired:
		result.Outcome = OutcomeRejected
		return result, nil
	default:
		return CommitResult{}, fmt.Errorf("%w: nonterminal Commit replay", ErrInvalidTransition)
	}
}

func (service *Service) ReconcileCommitting(ctx context.Context, limit int) (int, error) {
	if service == nil || service.repository == nil || limit < 1 || limit > 1000 {
		return 0, ErrInvalidInput
	}
	intents, err := service.repository.ListCommitting(ctx, limit)
	if err != nil {
		return 0, err
	}
	completed := 0
	for _, intent := range intents {
		executor := service.executors[intent.ToolIdentity]
		state, receipt, statusDigest, code := IntentOutcomeUnknown, "", "", "OUTCOME_UNKNOWN"
		if service.readers[intent.ToolIdentity] != nil {
			// A committing read may already have returned a result to this process.
			// The raw result is intentionally not durable, so restart recovery must
			// not execute it again merely because the Tool is idempotent.
		} else if executor != nil {
			status, statusErr := executor.Status(ctx, executionRequest(intent))
			if statusErr == nil {
				statusDigest = digestOptional(status.StatusToken)
				if status.Outcome == OutcomeCommitted && validFingerprint(status.ReceiptFingerprint) {
					state, receipt, code = IntentCommitted, status.ReceiptFingerprint, ""
				} else if status.Outcome == OutcomeRejected {
					state, code = IntentFailed, "EXECUTOR_REJECTED"
				}
			}
		}
		if _, err := service.completeCommit(ctx, intent.IntentID, state, receipt, statusDigest, code); err != nil {
			return completed, err
		}
		completed++
	}
	return completed, nil
}

func (service *Service) Expire(ctx context.Context, cutoff time.Time, limit int) (int, error) {
	if service == nil || service.repository == nil || cutoff.IsZero() || limit < 1 || limit > 1000 {
		return 0, ErrInvalidInput
	}
	changed, err := service.repository.Expire(ctx, cutoff.UTC(), limit)
	if err == nil && service.secretBroker != nil {
		service.secretBroker.clearExpired(cutoff.UTC())
	}
	return changed, err
}
