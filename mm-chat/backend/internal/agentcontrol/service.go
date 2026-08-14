package agentcontrol

import (
	"context"
	"errors"
	"io"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentcron"
	"neo-chat/mm-chat/backend/internal/agentlearning"
)

var (
	uuidPattern        = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	prefixedIDPattern  = regexp.MustCompile(`^[a-z][a-z0-9_]*_[a-z0-9]{16,64}$`)
	fingerprintPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	reasonPattern      = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
)

type Service struct {
	repository      Repository
	broker          BrokerService
	cron            CronService
	learning        LearningService
	administratorID string
	shadowAdapter   ShadowAdapter
	artifactStore   ArtifactStore
	bootID          string
	newID           func(string) string
}

type ServiceOption func(*Service)

func WithRepository(repository Repository) ServiceOption {
	return func(service *Service) { service.repository = repository }
}

func WithBroker(service BrokerService) ServiceOption {
	return func(control *Service) { control.broker = service }
}

func WithCron(service CronService) ServiceOption {
	return func(control *Service) { control.cron = service }
}

func WithLearning(service LearningService) ServiceOption {
	return func(control *Service) { control.learning = service }
}

func WithAdministratorUserID(userID string) ServiceOption {
	return func(service *Service) {
		service.administratorID = strings.ToLower(strings.TrimSpace(userID))
	}
}

func WithShadowAdapter(adapter ShadowAdapter) ServiceOption {
	return func(service *Service) { service.shadowAdapter = adapter }
}

func WithArtifactStore(store ArtifactStore) ServiceOption {
	return func(service *Service) { service.artifactStore = store }
}

func NewService(options ...ServiceOption) *Service {
	service := &Service{
		bootID: "boot_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		newID: func(prefix string) string {
			return prefix + "_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		},
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (service *Service) Initialize(ctx context.Context) error {
	if service == nil || service.repository == nil {
		return ErrDatabaseRequired
	}
	_, err := service.repository.RegisterShadowBoot(ctx, service.bootID)
	return err
}

func (service *Service) IsAdministrator(userID string) bool {
	return service != nil && service.administratorID != "" &&
		strings.EqualFold(strings.TrimSpace(userID), service.administratorID)
}

func (service *Service) Status(ctx context.Context, userID string) (CenterStatus, error) {
	if service == nil || service.repository == nil {
		return CenterStatus{}, ErrDatabaseRequired
	}
	if !validUUID(userID) {
		return CenterStatus{}, ErrInvalidInput
	}
	shadow, err := service.repository.GetShadow(ctx, normalizedUserID(userID))
	if err != nil {
		return CenterStatus{}, err
	}
	return CenterStatus{
		IsAdministrator: service.IsAdministrator(userID),
		Runtime: RuntimeStatus{
			State: "held", ReasonCode: RuntimeHeldReason, Executable: false,
			Scheduler: false, LearningWorker: false,
		},
		Shadow: shadow,
	}, nil
}

func (service *Service) GetArtifactContent(
	ctx context.Context,
	userID, runID, artifactID string,
) (Artifact, io.ReadCloser, error) {
	if service == nil || service.repository == nil || service.artifactStore == nil {
		return Artifact{}, nil, ErrDatabaseRequired
	}
	userID = normalizedUserID(userID)
	if !validUUID(userID) || !validID(runID, "run") || !validID(artifactID, "artifact") {
		return Artifact{}, nil, ErrInvalidInput
	}
	source, err := service.repository.GetArtifact(ctx, userID, runID, artifactID)
	if err != nil {
		return Artifact{}, nil, err
	}
	reader, info, err := service.artifactStore.Get(ctx, source.ObjectKey)
	if err != nil {
		return Artifact{}, nil, err
	}
	if info.Size != source.Size {
		_ = reader.Close()
		return Artifact{}, nil, ErrFingerprintDrift
	}
	return source.Artifact, reader, nil
}

func (service *Service) ListRuns(ctx context.Context, userID string, limit int) ([]RunSummary, error) {
	if service == nil || service.repository == nil {
		return nil, ErrDatabaseRequired
	}
	if !validUUID(userID) || limit < 1 || limit > 100 {
		return nil, ErrInvalidInput
	}
	return service.repository.ListRuns(ctx, normalizedUserID(userID), limit)
}

func (service *Service) GetRun(ctx context.Context, userID, runID string) (RunDetail, error) {
	if service == nil || service.repository == nil {
		return RunDetail{}, ErrDatabaseRequired
	}
	if !validUUID(userID) || !validID(runID, "run") {
		return RunDetail{}, ErrInvalidInput
	}
	return service.repository.GetRun(ctx, normalizedUserID(userID), runID)
}

// EnqueueRootRun is an explicit product boundary. It deliberately returns the
// exact-host held result without persisting a Run or constructing authority in
// the API process.
func (service *Service) EnqueueRootRun(context.Context, string) error {
	return ErrIsolationUnavailable
}

func (service *Service) CancelRun(
	ctx context.Context,
	userID string,
	input CancelRunInput,
) (RunCancellation, error) {
	if service == nil || service.repository == nil {
		return RunCancellation{}, ErrDatabaseRequired
	}
	input.UserID = normalizedUserID(userID)
	input.ExpectedState = strings.TrimSpace(input.ExpectedState)
	input.SnapshotFingerprint = strings.TrimSpace(input.SnapshotFingerprint)
	input.Mode = strings.TrimSpace(input.Mode)
	input.ReasonCode = strings.TrimSpace(input.ReasonCode)
	if input.CancellationID == "" {
		input.CancellationID = service.newID("cancellation")
	}
	if !validUUID(input.UserID) || !validID(input.RunID, "run") ||
		!validID(input.CancellationID, "cancellation") ||
		!fingerprintPattern.MatchString(input.SnapshotFingerprint) ||
		(input.Mode != "cancel" && input.Mode != "kill") ||
		!reasonPattern.MatchString(input.ReasonCode) || !validRunState(input.ExpectedState) {
		return RunCancellation{}, ErrInvalidInput
	}
	return service.repository.CancelRun(ctx, input)
}

func (service *Service) DecideApproval(
	ctx context.Context,
	userID string,
	input agentbroker.ApprovalInput,
) (Approval, error) {
	if service == nil || service.broker == nil {
		return Approval{}, ErrDatabaseRequired
	}
	input.UserID = normalizedUserID(userID)
	input.ActorType = "user"
	input.ActorID = input.UserID
	if input.ApprovalID == "" {
		input.ApprovalID = service.newID("approval")
	}
	intent, err := service.broker.DecideApproval(ctx, input)
	if err != nil {
		return Approval{}, err
	}
	return approvalFromIntent(intent), nil
}

func (service *Service) CancelApproval(
	ctx context.Context,
	userID string,
	input agentbroker.CancelInput,
) (Approval, error) {
	if service == nil || service.broker == nil {
		return Approval{}, ErrDatabaseRequired
	}
	input.UserID = normalizedUserID(userID)
	input.ActorType = "user"
	input.ActorID = input.UserID
	if input.CancellationID == "" {
		input.CancellationID = service.newID("cancellation")
	}
	intent, err := service.broker.Cancel(ctx, input)
	if err != nil {
		return Approval{}, err
	}
	return approvalFromIntent(intent), nil
}

func approvalFromIntent(intent agentbroker.PreparedIntent) Approval {
	return Approval{
		IntentID: intent.IntentID, IntentFingerprint: intent.IntentFingerprint,
		ToolIdentity: intent.ToolIdentity, Capability: intent.Capability,
		Action: intent.Action, ArgumentsFingerprint: intent.ArgumentsFingerprint,
		ApprovalClass: intent.ApprovalClass, ApprovalRevision: intent.ApprovalRevision,
		State: intent.State, ExpiresAt: intent.ExpiresAt, ApprovedAt: intent.ApprovedAt,
		TerminalAt: intent.TerminalAt, ErrorCode: intent.ErrorCode,
	}
}

func (service *Service) ListSchedules(
	ctx context.Context,
	userID string,
	limit int,
) ([]ScheduleSummary, error) {
	if service == nil || service.repository == nil {
		return nil, ErrDatabaseRequired
	}
	if !validUUID(userID) || limit < 1 || limit > 100 {
		return nil, ErrInvalidInput
	}
	return service.repository.ListSchedules(ctx, normalizedUserID(userID), limit)
}

func (service *Service) GetSchedule(
	ctx context.Context,
	userID, templateID string,
) (agentcron.Template, error) {
	if service == nil || service.cron == nil {
		return agentcron.Template{}, ErrDatabaseRequired
	}
	return service.cron.GetTemplate(ctx, normalizedUserID(userID), templateID)
}

func (service *Service) CreateSchedule(
	ctx context.Context,
	userID string,
	input agentcron.CreateInput,
) (agentcron.Template, bool, error) {
	if service == nil || service.cron == nil {
		return agentcron.Template{}, false, ErrDatabaseRequired
	}
	userID = normalizedUserID(userID)
	if !validUUID(userID) {
		return agentcron.Template{}, false, ErrInvalidInput
	}
	input.Spec.Owner.UserID = userID
	input.Approval.ActorType = "user"
	input.Approval.ActorID = userID
	return service.cron.CreateRevision(ctx, input)
}

func (service *Service) ChangeScheduleLifecycle(
	ctx context.Context,
	userID, templateID, state string,
	expectedRevision int64,
	reasonCode string,
) (agentcron.Template, error) {
	if service == nil || service.cron == nil {
		return agentcron.Template{}, ErrDatabaseRequired
	}
	userID = normalizedUserID(userID)
	if !validUUID(userID) || !reasonPattern.MatchString(reasonCode) {
		return agentcron.Template{}, ErrInvalidInput
	}
	if state == agentcron.TemplateActive {
		template, err := service.cron.GetTemplate(ctx, userID, templateID)
		if err != nil {
			return agentcron.Template{}, err
		}
		if template.CurrentRevision != expectedRevision {
			return agentcron.Template{}, agentcron.ErrRevisionConflict
		}
		return service.cron.Resume(ctx, template, agentcron.Approval{
			ActorType: "user", ActorID: userID, ReasonCode: reasonCode,
		})
	}
	return service.cron.SetLifecycle(ctx, agentcron.LifecycleInput{
		TemplateID: templateID, UserID: userID, ExpectedRevision: expectedRevision,
		To: state, ActorType: "user", ActorID: userID, ReasonCode: reasonCode,
	})
}

func (service *Service) ListDrafts(
	ctx context.Context,
	administratorID string,
	limit int,
) ([]DraftSummary, error) {
	if err := service.requireAdministrator(administratorID); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 || service.repository == nil {
		return nil, ErrInvalidInput
	}
	return service.repository.ListDrafts(ctx, limit)
}

func (service *Service) GetDraft(
	ctx context.Context,
	administratorID, draftID string,
) (DraftSummary, error) {
	if err := service.requireAdministrator(administratorID); err != nil {
		return DraftSummary{}, err
	}
	if !validID(draftID, "draft") || service.repository == nil {
		return DraftSummary{}, ErrInvalidInput
	}
	return service.repository.GetDraft(ctx, draftID)
}

func (service *Service) GetDraftDiff(
	ctx context.Context,
	administratorID, draftID string,
) ([]agentlearning.FileDiff, error) {
	if err := service.requireAdministrator(administratorID); err != nil {
		return nil, err
	}
	if service.learning == nil {
		return nil, ErrDatabaseRequired
	}
	return service.learning.GetDiff(ctx, normalizedUserID(administratorID), draftID)
}

func (service *Service) ReviewDraft(
	ctx context.Context,
	administratorID, decision string,
	input agentlearning.ReviewInput,
) (DraftReviewResult, error) {
	if err := service.requireAdministrator(administratorID); err != nil {
		return DraftReviewResult{}, err
	}
	if service.learning == nil {
		return DraftReviewResult{}, ErrDatabaseRequired
	}
	switch decision {
	case "reject":
		_, created, err := service.learning.Reject(ctx, normalizedUserID(administratorID), input)
		if err != nil {
			return DraftReviewResult{}, err
		}
		draft, err := service.repository.GetDraft(ctx, input.DraftID)
		if err != nil {
			return DraftReviewResult{}, err
		}
		return DraftReviewResult{Decision: decision, Draft: &draft, Created: created}, nil
	case "promote":
		promotion, err := service.learning.Promote(ctx, normalizedUserID(administratorID), input)
		if err != nil {
			return DraftReviewResult{}, err
		}
		return DraftReviewResult{Decision: decision, Promotion: &promotion, Created: promotion.Created}, nil
	default:
		return DraftReviewResult{}, ErrInvalidInput
	}
}

func (service *Service) UpdateShadowPolicy(
	ctx context.Context,
	administratorID string,
	input UpdateShadowPolicyInput,
) (ShadowPolicy, error) {
	if err := service.requireAdministrator(administratorID); err != nil {
		return ShadowPolicy{}, err
	}
	if service.repository == nil {
		return ShadowPolicy{}, ErrDatabaseRequired
	}
	input.AdministratorID = normalizedUserID(administratorID)
	if input.ExpectedRevision < 0 || !validShadowMode(input.Mode) ||
		input.CohortBasisPoints < 0 || input.CohortBasisPoints > 10000 ||
		input.MaxObservations < 1 || input.MaxObservations > 100000 ||
		input.MaxErrors < 0 || input.MaxErrors > input.MaxObservations ||
		!reasonPattern.MatchString(input.ReasonCode) {
		return ShadowPolicy{}, ErrInvalidInput
	}
	if input.Enabled && (!validUUID(input.AdmissionID) ||
		!fingerprintPattern.MatchString(input.PackageFingerprint) ||
		!fingerprintPattern.MatchString(input.RuntimeBundleFingerprint) ||
		input.StartsAt == nil || input.ExpiresAt == nil ||
		!input.ExpiresAt.After(*input.StartsAt)) {
		return ShadowPolicy{}, ErrInvalidInput
	}
	return service.repository.UpdateShadowPolicy(ctx, input)
}

func (service *Service) SetShadowOptIn(
	ctx context.Context,
	userID string,
	input SetShadowOptInInput,
) (ShadowSnapshot, error) {
	if service == nil || service.repository == nil {
		return ShadowSnapshot{}, ErrDatabaseRequired
	}
	input.UserID = normalizedUserID(userID)
	if !validUUID(input.UserID) || input.ExpectedGeneration < 0 ||
		input.PolicyRevision < 0 || !reasonPattern.MatchString(input.ReasonCode) {
		return ShadowSnapshot{}, ErrInvalidInput
	}
	if _, err := service.repository.SetShadowOptIn(ctx, input); err != nil {
		return ShadowSnapshot{}, err
	}
	return service.repository.GetShadow(ctx, input.UserID)
}

func (service *Service) ObserveShadow(
	ctx context.Context,
	request ShadowRequest,
) (ShadowObservation, error) {
	if service == nil || service.repository == nil {
		return ShadowObservation{}, ErrDatabaseRequired
	}
	if service.shadowAdapter == nil {
		return ShadowObservation{}, ErrIsolationUnavailable
	}
	request.UserID = normalizedUserID(request.UserID)
	if !validUUID(request.UserID) || request.PolicyRevision < 1 ||
		request.Generation < 1 || !validUUID(request.AdmissionID) ||
		!fingerprintPattern.MatchString(request.PackageFingerprint) ||
		!fingerprintPattern.MatchString(request.RuntimeBundleFingerprint) ||
		!validShadowMode(request.Mode) {
		return ShadowObservation{}, ErrInvalidInput
	}
	snapshot, err := service.repository.GetShadow(ctx, request.UserID)
	if err != nil {
		return ShadowObservation{}, err
	}
	if !snapshot.Eligible {
		switch snapshot.HeldReasonCode {
		case "BUDGET_EXCEEDED":
			return ShadowObservation{}, ErrBudgetExceeded
		case "KILL_SWITCH_ACTIVE":
			return ShadowObservation{}, ErrKillSwitchActive
		}
		return ShadowObservation{}, ErrIsolationUnavailable
	}
	if snapshot.Policy.Revision != request.PolicyRevision ||
		snapshot.OptIn.Generation != request.Generation {
		return ShadowObservation{}, ErrGenerationStale
	}
	if snapshot.Policy.AdmissionID != request.AdmissionID ||
		snapshot.Policy.PackageFingerprint != request.PackageFingerprint ||
		snapshot.Policy.RuntimeBundleFingerprint != request.RuntimeBundleFingerprint ||
		snapshot.Policy.Mode != request.Mode {
		return ShadowObservation{}, ErrFingerprintDrift
	}
	measurement, err := service.shadowAdapter.Observe(ctx, request)
	if err != nil {
		return ShadowObservation{}, err
	}
	if !validMeasurement(measurement) {
		return ShadowObservation{}, ErrInvalidInput
	}
	return service.repository.AppendShadowObservation(ctx, service.bootID, request, measurement)
}

func (service *Service) requireAdministrator(userID string) error {
	if !service.IsAdministrator(userID) {
		return ErrAdministratorNeeded
	}
	return nil
}

func normalizedUserID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validUUID(value string) bool { return uuidPattern.MatchString(normalizedUserID(value)) }

func validID(value, prefix string) bool {
	return strings.HasPrefix(value, prefix+"_") && prefixedIDPattern.MatchString(value)
}

func validShadowMode(value string) bool {
	return value == ShadowModeSynthetic || value == ShadowModeReadOnly
}

func validRunState(value string) bool {
	switch value {
	case "pending", "admitted", "queued", "running":
		return true
	default:
		return false
	}
}

func validMeasurement(value ShadowMeasurement) bool {
	if !reasonPattern.MatchString(value.ReasonCode) ||
		(value.Outcome != "succeeded" && value.Outcome != "failed" && value.Outcome != "held") ||
		(value.LatencyBucket != "lt_100ms" && value.LatencyBucket != "lt_500ms" &&
			value.LatencyBucket != "lt_2s" && value.LatencyBucket != "gte_2s") {
		return false
	}
	for _, count := range []int{value.RunCount, value.StepCount, value.AttemptCount, value.ErrorCount} {
		if count < 0 || count > 100000 {
			return false
		}
	}
	return true
}

func errorIsConflict(err error) bool {
	return errors.Is(err, ErrRevisionConflict) || errors.Is(err, agentcron.ErrRevisionConflict) ||
		errors.Is(err, agentlearning.ErrRevisionConflict)
}
