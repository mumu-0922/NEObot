package agentbroker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

const maximumProjectCanaryBytes = int64(4096)

// ProjectMutationAuthority is the exact live Broker binding repeated at the
// independent Project CAS authority. It deliberately excludes lease plaintext.
type ProjectMutationAuthority struct {
	UserID              string
	RunID               string
	AttemptID           string
	Generation          int64
	SnapshotFingerprint string
	GrantFingerprint    string
	RegistryFingerprint string
	IntentID            string
	IdempotencyKey      string
	Resource            string
	BaseRevision        string
	Path                string
	Content             []byte
	ContentFingerprint  string
	MutationFingerprint string
}

type ProjectMutationReceipt struct {
	Outcome            string
	ReceiptFingerprint string
	CommittedRevision  string
}

type ProjectMutationRepository interface {
	CommitProjectMutation(context.Context, ProjectMutationAuthority) (ProjectMutationReceipt, error)
	ProjectMutationStatus(context.Context, ProjectMutationAuthority) (ProjectMutationReceipt, error)
	CleanupProjectMutation(context.Context, ProjectMutationAuthority, string) (string, error)
}

type ProjectMutationCleanupCandidate struct {
	IntentID            string
	IdempotencyKey      string
	AttemptID           string
	Generation          int64
	SnapshotFingerprint string
	GrantFingerprint    string
	RegistryFingerprint string
	ReceiptFingerprint  string
}

type ProjectMutationCleanupRepository interface {
	PendingProjectMutationCleanup(context.Context, string, string, string) (ProjectMutationCleanupCandidate, bool, error)
}

// ProjectEffectExecutor adapts one pre-reviewed Project patch to the general
// Broker effect seam. It cannot accept package-selected paths or content.
type ProjectEffectExecutor struct {
	repository           ProjectMutationRepository
	authority            ProjectMutationAuthority
	arguments            json.RawMessage
	argumentsFingerprint string
}

func NewProjectEffectExecutor(repository ProjectMutationRepository, authority ProjectMutationAuthority) (*ProjectEffectExecutor, error) {
	if repository == nil || validateProjectMutationAuthority(authority, false) != nil {
		return nil, ErrInvalidInput
	}
	encodedArguments, err := json.Marshal(map[string]any{
		"contentFingerprint":  authority.ContentFingerprint,
		"mutationFingerprint": authority.MutationFingerprint,
		"path":                authority.Path,
		"sizeBytes":           len(authority.Content),
	})
	if err != nil {
		return nil, ErrInvalidInput
	}
	arguments, err := canonicalJSON(encodedArguments, maxArgumentsBytes)
	if err != nil {
		return nil, ErrInvalidInput
	}
	copyAuthority := authority
	copyAuthority.Content = append([]byte(nil), authority.Content...)
	return &ProjectEffectExecutor{repository: repository, authority: copyAuthority, arguments: arguments,
		argumentsFingerprint: fingerprint("neo-effect-arguments-v1", arguments)}, nil
}

func (executor *ProjectEffectExecutor) Commit(ctx context.Context, request ExecutionRequest) (ExecutorReceipt, error) {
	if executor == nil || executor.repository == nil || !executor.matches(request) {
		return ExecutorReceipt{}, &DispatchError{Cause: ErrGrantDenied, PossibleSend: false}
	}
	authority := executor.bind(request)
	receipt, err := executor.repository.CommitProjectMutation(ctx, authority)
	clear(authority.Content)
	if err != nil {
		return ExecutorReceipt{}, &DispatchError{Cause: err, PossibleSend: projectMutationMayHaveSent(err)}
	}
	if receipt.Outcome != OutcomeCommitted || !validFingerprint(receipt.ReceiptFingerprint) ||
		!revisionPattern.MatchString(receipt.CommittedRevision) {
		return ExecutorReceipt{}, &DispatchError{Cause: ErrExecutorUnavailable, PossibleSend: true}
	}
	return ExecutorReceipt{ReceiptFingerprint: receipt.ReceiptFingerprint, StatusToken: receipt.CommittedRevision}, nil
}

func (executor *ProjectEffectExecutor) Status(ctx context.Context, request ExecutionRequest) (ExecutorStatus, error) {
	if executor == nil || executor.repository == nil || !executor.matches(request) {
		return ExecutorStatus{}, ErrGrantDenied
	}
	authority := executor.bind(request)
	receipt, err := executor.repository.ProjectMutationStatus(ctx, authority)
	clear(authority.Content)
	if err != nil {
		return ExecutorStatus{}, err
	}
	switch receipt.Outcome {
	case OutcomeCommitted:
		if !validFingerprint(receipt.ReceiptFingerprint) || !revisionPattern.MatchString(receipt.CommittedRevision) {
			return ExecutorStatus{}, ErrExecutorUnavailable
		}
		return ExecutorStatus{Outcome: OutcomeCommitted, ReceiptFingerprint: receipt.ReceiptFingerprint,
			StatusToken: receipt.CommittedRevision}, nil
	case OutcomeRejected:
		return ExecutorStatus{Outcome: OutcomeRejected, StatusToken: receipt.CommittedRevision}, nil
	case OutcomeUnknown:
		return ExecutorStatus{Outcome: OutcomeUnknown, StatusToken: receipt.CommittedRevision}, nil
	default:
		return ExecutorStatus{}, ErrExecutorUnavailable
	}
}

func (executor *ProjectEffectExecutor) Cleanup(ctx context.Context, request ExecutionRequest, receiptFingerprint string) (string, error) {
	if executor == nil || executor.repository == nil || !executor.matches(request) || !validFingerprint(receiptFingerprint) {
		return "", ErrGrantDenied
	}
	authority := executor.bind(request)
	revision, err := executor.repository.CleanupProjectMutation(ctx, authority, receiptFingerprint)
	clear(authority.Content)
	if err != nil || !revisionPattern.MatchString(revision) {
		if err != nil {
			return "", err
		}
		return "", ErrExecutorUnavailable
	}
	return revision, nil
}

// ReconcileCleanup restores a committed synthetic mutation after a controller
// crash between the Broker terminal fact and the cleanup acknowledgement.
func (executor *ProjectEffectExecutor) ReconcileCleanup(ctx context.Context, userID, runID string) (bool, string, error) {
	if executor == nil || executor.repository == nil || !uuidPattern.MatchString(userID) || !validID(runID, "run") {
		return false, "", ErrProjectMutationDenied
	}
	repository, ok := executor.repository.(ProjectMutationCleanupRepository)
	if !ok {
		return false, "", ErrProjectMutationDenied
	}
	candidate, found, err := repository.PendingProjectMutationCleanup(ctx, userID, runID, executor.authority.Resource)
	if err != nil || !found {
		return false, "", err
	}
	authority := executor.authority
	authority.UserID = userID
	authority.RunID = runID
	authority.AttemptID = candidate.AttemptID
	authority.Generation = candidate.Generation
	authority.SnapshotFingerprint = candidate.SnapshotFingerprint
	authority.GrantFingerprint = candidate.GrantFingerprint
	authority.RegistryFingerprint = candidate.RegistryFingerprint
	authority.IntentID = candidate.IntentID
	authority.IdempotencyKey = candidate.IdempotencyKey
	authority.Content = append([]byte(nil), executor.authority.Content...)
	revision, cleanupErr := executor.repository.CleanupProjectMutation(ctx, authority, candidate.ReceiptFingerprint)
	clear(authority.Content)
	if cleanupErr != nil || !revisionPattern.MatchString(revision) {
		if cleanupErr != nil {
			return true, "", cleanupErr
		}
		return true, "", ErrExecutorUnavailable
	}
	return true, revision, nil
}

func (executor *ProjectEffectExecutor) matches(request ExecutionRequest) bool {
	actualArguments, err := ArgumentsFingerprint(request.Arguments)
	return request.ToolIdentity == "project.patch" && request.Capability == "project.write" &&
		request.Action == "apply_patch" && request.Resource == executor.authority.Resource &&
		request.BaseRevision == executor.authority.BaseRevision && err == nil &&
		actualArguments == executor.argumentsFingerprint &&
		(request.ArgumentsFingerprint == "" || request.ArgumentsFingerprint == executor.argumentsFingerprint)
}

func (executor *ProjectEffectExecutor) bind(request ExecutionRequest) ProjectMutationAuthority {
	authority := executor.authority
	authority.UserID = request.UserID
	authority.RunID = request.RunID
	authority.AttemptID = request.AttemptID
	authority.Generation = request.Generation
	authority.SnapshotFingerprint = request.SnapshotFingerprint
	authority.GrantFingerprint = request.GrantFingerprint
	authority.RegistryFingerprint = request.RegistryFingerprint
	authority.IntentID = request.IntentID
	authority.IdempotencyKey = request.IdempotencyKey
	authority.Content = append([]byte(nil), executor.authority.Content...)
	return authority
}

func validateProjectMutationAuthority(authority ProjectMutationAuthority, requireRuntime bool) error {
	if !validProjectResource(authority.Resource) || !validProjectCanaryPath(authority.Path) ||
		!revisionPattern.MatchString(authority.BaseRevision) || len(authority.Content) < 1 ||
		int64(len(authority.Content)) > maximumProjectCanaryBytes {
		return ErrInvalidInput
	}
	contentFingerprint := fingerprint("neo-project-file-v1", authority.Content)
	mutationFingerprint := ProjectMutationFingerprint(authority.BaseRevision, authority.Resource, authority.Path, contentFingerprint)
	if authority.ContentFingerprint != contentFingerprint || authority.MutationFingerprint != mutationFingerprint {
		return ErrSnapshotMismatch
	}
	if requireRuntime && (!uuidPattern.MatchString(authority.UserID) || !validID(authority.RunID, "run") ||
		!validID(authority.AttemptID, "attempt") || authority.Generation < 1 ||
		!validFingerprint(authority.SnapshotFingerprint) || !validFingerprint(authority.GrantFingerprint) ||
		!validFingerprint(authority.RegistryFingerprint) || !validID(authority.IntentID, "intent") ||
		!stringsHasCommitPrefix(authority.IdempotencyKey)) {
		return ErrInvalidInput
	}
	return nil
}

func ProjectMutationFingerprint(baseRevision, resource, pathValue, contentFingerprint string) string {
	return fingerprint("neo-project-canary-mutation-v1", []byte(fmt.Sprintf("%s\n%s\n%s\n%s",
		baseRevision, resource, pathValue, contentFingerprint)))
}

func validProjectResource(value string) bool {
	return len(value) >= len("project-canary/a") && len(value) <= 80 &&
		value[:len("project-canary/")] == "project-canary/" && validIdentifier(value[len("project-canary/"):])
}

func validProjectCanaryPath(value string) bool {
	return validProjectPath(value) && len(value) <= 128 && !bytes.ContainsRune([]byte(value), '/')
}

func stringsHasCommitPrefix(value string) bool {
	return len(value) >= 31 && len(value) <= 160 && len(value) >= len("commit_") && value[:len("commit_")] == "commit_"
}

func projectMutationMayHaveSent(err error) bool {
	return !errors.Is(err, ErrInvalidInput) && !errors.Is(err, ErrGrantDenied) &&
		!errors.Is(err, ErrLeaseStale) && !errors.Is(err, ErrSnapshotMismatch) &&
		!errors.Is(err, ErrKillSwitchActive) && !errors.Is(err, ErrApprovalRequired) &&
		!errors.Is(err, ErrApprovalDenied) && !errors.Is(err, ErrReplayDetected) &&
		!errors.Is(err, ErrProjectConflict) && !errors.Is(err, ErrProjectMutationDenied)
}
