package agentcontrol

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentcron"
	"neo-chat/mm-chat/backend/internal/agentlearning"
	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/storage"
)

const (
	testUserID  = "11111111-1111-4111-8111-111111111111"
	testAdminID = "22222222-2222-4222-8222-222222222222"
	testRunID   = "run_1234567890abcdef"
	testFP      = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testRuntime = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

type fakeRepository struct {
	shadow          ShadowSnapshot
	runsByUser      map[string][]RunSummary
	detailByUser    map[string]RunDetail
	cancellation    CancelRunInput
	optIn           SetShadowOptInInput
	policy          UpdateShadowPolicyInput
	observation     ShadowMeasurement
	observationReq  ShadowRequest
	observationBoot string
	boot            string
	artifact        ArtifactSource
	draft           DraftSummary
	canaryRequest   EnqueueProductCanaryInput
}

type fakeBrokerService struct {
	decision agentbroker.ApprovalInput
	cancel   agentbroker.CancelInput
}

func (service *fakeBrokerService) DecideApproval(
	_ context.Context,
	input agentbroker.ApprovalInput,
) (agentbroker.PreparedIntent, error) {
	service.decision = input
	return agentbroker.PreparedIntent{
		IntentID: input.IntentID, IntentFingerprint: input.IntentFingerprint,
		State: input.Decision, ApprovalRevision: input.ExpectedRevision + 1,
	}, nil
}

func (service *fakeBrokerService) Cancel(
	_ context.Context,
	input agentbroker.CancelInput,
) (agentbroker.PreparedIntent, error) {
	service.cancel = input
	return agentbroker.PreparedIntent{
		IntentID: input.IntentID, IntentFingerprint: input.IntentFingerprint,
		State: "canceled",
	}, nil
}

func (repository *fakeRepository) ListRuns(_ context.Context, userID string, _ int) ([]RunSummary, error) {
	return repository.runsByUser[userID], nil
}
func (repository *fakeRepository) GetRun(_ context.Context, userID, runID string) (RunDetail, error) {
	result, ok := repository.detailByUser[userID+"/"+runID]
	if !ok {
		return RunDetail{}, ErrNotFound
	}
	return result, nil
}
func (repository *fakeRepository) GetArtifact(_ context.Context, userID, runID, artifactID string) (ArtifactSource, error) {
	if repository.artifact.ID != artifactID || repository.artifact.ObjectKey == "" ||
		userID != testUserID || runID != testRunID {
		return ArtifactSource{}, ErrNotFound
	}
	return repository.artifact, nil
}
func (repository *fakeRepository) CancelRun(_ context.Context, input CancelRunInput) (RunCancellation, error) {
	repository.cancellation = input
	return RunCancellation{ID: input.CancellationID, RunID: input.RunID, Mode: input.Mode, State: "completed"}, nil
}
func (*fakeRepository) ListSchedules(context.Context, string, int) ([]ScheduleSummary, error) {
	return []ScheduleSummary{}, nil
}
func (*fakeRepository) ListDrafts(context.Context, int) ([]DraftSummary, error) {
	return []DraftSummary{}, nil
}
func (repository *fakeRepository) GetDraft(_ context.Context, draftID string) (DraftSummary, error) {
	if repository.draft.ID != draftID {
		return DraftSummary{}, ErrNotFound
	}
	return repository.draft, nil
}
func (repository *fakeRepository) GetShadow(context.Context, string) (ShadowSnapshot, error) {
	return repository.shadow, nil
}
func (repository *fakeRepository) EnqueueProductCanary(
	_ context.Context,
	input EnqueueProductCanaryInput,
) (ProductCanaryRequest, error) {
	if !repository.shadow.Effective {
		return ProductCanaryRequest{}, ErrIsolationUnavailable
	}
	repository.canaryRequest = input
	return ProductCanaryRequest{ID: input.RequestID, ActivationID: "activation_1234567890abcdef",
		State: "queued", PolicyRevision: input.ExpectedPolicyRevision,
		OptGeneration: input.ExpectedGeneration, RequestFingerprint: input.RequestFingerprint}, nil
}
func (repository *fakeRepository) UpdateShadowPolicy(_ context.Context, input UpdateShadowPolicyInput) (ShadowPolicy, error) {
	repository.policy = input
	return ShadowPolicy{Revision: input.ExpectedRevision + 1, Enabled: input.Enabled}, nil
}
func (repository *fakeRepository) SetShadowOptIn(_ context.Context, input SetShadowOptInInput) (ShadowOptIn, error) {
	repository.optIn = input
	repository.shadow.OptIn = ShadowOptIn{OptedIn: input.OptedIn, Generation: input.ExpectedGeneration + 1, PolicyRevision: input.PolicyRevision}
	return repository.shadow.OptIn, nil
}
func (repository *fakeRepository) RegisterShadowBoot(_ context.Context, bootID string) (int64, error) {
	repository.boot = bootID
	return 1, nil
}
func (repository *fakeRepository) AppendShadowObservation(_ context.Context, boot string, request ShadowRequest, measurement ShadowMeasurement) (ShadowObservation, error) {
	repository.observationBoot = boot
	repository.observationReq = request
	repository.observation = measurement
	return ShadowObservation{ID: "shadow_observation_0000000000000001", PolicyRevision: request.PolicyRevision, Generation: request.Generation, ReasonCode: measurement.ReasonCode}, nil
}

type shadowAdapterFunc func(context.Context, ShadowRequest) (ShadowMeasurement, error)

type fakeArtifactStore struct{ payload []byte }

func (store fakeArtifactStore) Get(context.Context, string) (io.ReadCloser, storage.ObjectInfo, error) {
	return io.NopCloser(bytes.NewReader(store.payload)), storage.ObjectInfo{Size: int64(len(store.payload))}, nil
}

func (adapter shadowAdapterFunc) Observe(ctx context.Context, request ShadowRequest) (ShadowMeasurement, error) {
	return adapter(ctx, request)
}

func TestServiceStatusAndHeldEnqueueRemainHonest(t *testing.T) {
	repository := &fakeRepository{shadow: ShadowSnapshot{HeldReasonCode: "SHADOW_DISABLED"}}
	service := NewService(WithRepository(repository), WithAdministratorUserID(testAdminID))
	if err := service.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repository.boot == "" {
		t.Fatal("Initialize did not register a restart fence")
	}
	status, err := service.Status(context.Background(), testAdminID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.IsAdministrator || status.Runtime.Executable || status.Runtime.ReasonCode != RuntimeHeldReason {
		t.Fatalf("status = %#v", status)
	}
	if _, err := service.EnqueueRootRun(context.Background(), testAdminID, 1, 1); !errors.Is(err, ErrIsolationUnavailable) {
		t.Fatal("held enqueue did not return ISOLATION_UNAVAILABLE")
	}
}

func TestServiceEnqueuesOnlyBoundProductCanaryRequest(t *testing.T) {
	repository := &fakeRepository{shadow: ShadowSnapshot{Effective: true,
		HeldReasonCode: "PRODUCT_CANARY_READY"}}
	service := NewService(WithRepository(repository))
	request, err := service.EnqueueRootRun(context.Background(), testUserID, 7, 3)
	if err != nil {
		t.Fatal(err)
	}
	if request.State != "queued" || repository.canaryRequest.UserID != testUserID ||
		repository.canaryRequest.ExpectedPolicyRevision != 7 ||
		repository.canaryRequest.ExpectedGeneration != 3 ||
		!fingerprintPattern.MatchString(repository.canaryRequest.RequestFingerprint) ||
		!validID(repository.canaryRequest.RequestID, "product_request") {
		t.Fatalf("request=%#v captured=%#v", request, repository.canaryRequest)
	}
	status, err := service.Status(context.Background(), testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Runtime.Executable || !status.Runtime.ProductCanary ||
		status.Runtime.State != "canary_ready" {
		t.Fatalf("status=%#v", status)
	}
}

func TestServiceStatusReportsLocalDirectExecution(t *testing.T) {
	repository := &fakeRepository{shadow: ShadowSnapshot{HeldReasonCode: "SHADOW_DISABLED"}}
	service := NewService(
		WithRepository(repository),
		WithLocalDirectExecution(true),
	)
	if err := service.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, err := service.Status(context.Background(), testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Runtime.Executable || status.Runtime.ProductCanary ||
		status.Runtime.State != "local_ready" ||
		status.Runtime.ReasonCode != RuntimeLocalDirectReason {
		t.Fatalf("status=%#v", status)
	}
}

func TestServiceAcceptsCanonicalDevelopmentOwnerUUID(t *testing.T) {
	repository := &fakeRepository{
		shadow:     ShadowSnapshot{HeldReasonCode: "SHADOW_DISABLED"},
		runsByUser: map[string][]RunSummary{},
	}
	service := NewService(
		WithRepository(repository),
		WithAdministratorUserID(auth.DevelopmentUserID),
		WithLocalDirectExecution(true),
	)
	status, err := service.Status(context.Background(), auth.DevelopmentUserID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.IsAdministrator || status.Runtime.State != "local_ready" {
		t.Fatalf("status=%#v", status)
	}
	if _, err := service.ListRuns(context.Background(), auth.DevelopmentUserID, 50); err != nil {
		t.Fatalf("ListRuns() error=%v", err)
	}
	if _, err := service.ListSchedules(context.Background(), auth.DevelopmentUserID, 50); err != nil {
		t.Fatalf("ListSchedules() error=%v", err)
	}
	for _, invalid := range []string{
		"", "not-a-uuid", "00000000000000000000000000000001",
		"00000000-0000-0000-0000-000000000001-extra",
	} {
		if validUUID(invalid) {
			t.Fatalf("validUUID(%q)=true", invalid)
		}
	}
}

func TestServiceRunOwnershipAndCancellationBinding(t *testing.T) {
	repository := &fakeRepository{detailByUser: map[string]RunDetail{
		testUserID + "/" + testRunID: {Run: RunSummary{ID: testRunID}},
	}}
	service := NewService(WithRepository(repository))
	if _, err := service.GetRun(context.Background(), testAdminID, testRunID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user GetRun err = %v", err)
	}
	result, err := service.CancelRun(context.Background(), testUserID, CancelRunInput{
		RunID: testRunID, ExpectedState: "queued", SnapshotFingerprint: testFP,
		Mode: "cancel", ReasonCode: "USER_REQUESTED",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "completed" || repository.cancellation.UserID != testUserID ||
		repository.cancellation.CancellationID == "" || repository.cancellation.SnapshotFingerprint != testFP {
		t.Fatalf("cancellation = %#v, captured = %#v", result, repository.cancellation)
	}
}

func TestServiceApprovalDecisionAndCancelBindUserAuthority(t *testing.T) {
	broker := &fakeBrokerService{}
	service := NewService(WithBroker(broker))
	intentID := "intent_1234567890abcdef"

	decision, err := service.DecideApproval(context.Background(), testUserID, agentbroker.ApprovalInput{
		IntentID: intentID, IntentFingerprint: testFP, Decision: "approved",
		ExpectedRevision: 2, ReasonCode: "USER_APPROVED",
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != "approved" || broker.decision.UserID != testUserID ||
		broker.decision.ActorType != "user" || broker.decision.ActorID != testUserID ||
		broker.decision.ApprovalID == "" || broker.decision.ExpectedRevision != 2 {
		t.Fatalf("decision=%#v captured=%#v", decision, broker.decision)
	}

	canceled, err := service.CancelApproval(context.Background(), testUserID, agentbroker.CancelInput{
		IntentID: intentID, IntentFingerprint: testFP, ReasonCode: "USER_CANCELED",
	})
	if err != nil {
		t.Fatal(err)
	}
	if canceled.State != "canceled" || broker.cancel.UserID != testUserID ||
		broker.cancel.ActorType != "user" || broker.cancel.ActorID != testUserID ||
		broker.cancel.CancellationID == "" {
		t.Fatalf("cancel=%#v captured=%#v", canceled, broker.cancel)
	}
}

func TestServiceArtifactDownloadKeepsObjectKeyServerSide(t *testing.T) {
	repository := &fakeRepository{artifact: ArtifactSource{
		Artifact: Artifact{
			ID: "artifact_1234567890abcdef", AttemptID: "attempt_1234567890abcdef",
			Generation: 1, Name: "report.txt", MediaType: "text/plain", Size: 6,
			Fingerprint: testFP,
		},
		ObjectKey: "agent-artifacts/run/attempt/1/hash",
	}}
	service := NewService(
		WithRepository(repository),
		WithArtifactStore(fakeArtifactStore{payload: []byte("report")}),
	)
	artifact, reader, err := service.GetArtifactContent(context.Background(), testUserID,
		testRunID, repository.artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	payload, _ := io.ReadAll(reader)
	if artifact.Name != "report.txt" || string(payload) != "report" || artifact.DownloadURL != "" {
		t.Fatalf("artifact = %#v payload=%q", artifact, payload)
	}
	if _, _, err := service.GetArtifactContent(context.Background(), testAdminID,
		testRunID, repository.artifact.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user artifact err = %v", err)
	}
}

func TestServiceShadowRequiresEligibilityAndPersistsOnlyMeasurement(t *testing.T) {
	repository := &fakeRepository{shadow: ShadowSnapshot{
		Eligible: true, HeldReasonCode: RuntimeHeldReason,
		Policy: ShadowPolicy{Revision: 3, AdmissionID: "33333333-3333-4333-8333-333333333333", PackageFingerprint: testFP, RuntimeBundleFingerprint: testRuntime, Mode: ShadowModeSynthetic},
		OptIn:  ShadowOptIn{OptedIn: true, Generation: 4, PolicyRevision: 3},
	}}
	service := NewService(
		WithRepository(repository),
		WithShadowAdapter(shadowAdapterFunc(func(_ context.Context, request ShadowRequest) (ShadowMeasurement, error) {
			if request.UserID != testUserID || request.PackageFingerprint != testFP {
				t.Fatalf("request = %#v", request)
			}
			return ShadowMeasurement{Outcome: "succeeded", ReasonCode: "SYNTHETIC_OK", LatencyBucket: "lt_100ms", RunCount: 1, StepCount: 2, AttemptCount: 2}, nil
		})),
	)
	if err := service.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	request := ShadowRequest{
		UserID: testUserID, PolicyRevision: 3, Generation: 4,
		AdmissionID:        repository.shadow.Policy.AdmissionID,
		PackageFingerprint: testFP, RuntimeBundleFingerprint: testRuntime,
		Mode: ShadowModeSynthetic,
	}
	result, err := service.ObserveShadow(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.ReasonCode != "SYNTHETIC_OK" || repository.observationBoot == "" || repository.observation.RunCount != 1 {
		t.Fatalf("observation = %#v, measurement = %#v", result, repository.observation)
	}
	repository.shadow.Eligible = false
	repository.shadow.HeldReasonCode = "BUDGET_EXCEEDED"
	if _, err := service.ObserveShadow(context.Background(), request); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("budget err = %v", err)
	}
	repository.shadow.HeldReasonCode = "KILL_SWITCH_ACTIVE"
	if _, err := service.ObserveShadow(context.Background(), request); !errors.Is(err, ErrKillSwitchActive) {
		t.Fatalf("Kill Switch err = %v", err)
	}
}

func TestMapPostgresErrorPreservesKillSwitchAuthority(t *testing.T) {
	err := mapPostgresError("append Agent Shadow observation", &pgconn.PgError{
		Code: "P0001", Message: "KILL_SWITCH_ACTIVE",
	})
	if !errors.Is(err, ErrKillSwitchActive) {
		t.Fatalf("mapped err = %v", err)
	}
}

func TestServiceShadowPolicyIsAdministratorOnlyAndFingerprintBound(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(WithRepository(repository), WithAdministratorUserID(testAdminID))
	starts := time.Now().UTC().Add(time.Minute)
	expires := starts.Add(time.Hour)
	input := UpdateShadowPolicyInput{
		ExpectedRevision: 0, Enabled: true, Mode: ShadowModeReadOnly,
		AdmissionID:        "33333333-3333-4333-8333-333333333333",
		PackageFingerprint: testFP, RuntimeBundleFingerprint: testRuntime,
		CohortBasisPoints: 500, MaxObservations: 100, MaxErrors: 5,
		StartsAt: &starts, ExpiresAt: &expires, ReasonCode: "ADMIN_ENABLE",
	}
	if _, err := service.UpdateShadowPolicy(context.Background(), testUserID, input); !errors.Is(err, ErrAdministratorNeeded) {
		t.Fatalf("ordinary user policy err = %v", err)
	}
	policy, err := service.UpdateShadowPolicy(context.Background(), testAdminID, input)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Revision != 1 || repository.policy.AdministratorID != testAdminID || repository.policy.PackageFingerprint != testFP {
		t.Fatalf("policy = %#v, captured = %#v", policy, repository.policy)
	}
	repository.shadow.Policy = ShadowPolicy{Revision: policy.Revision}
	shadow, err := service.SetShadowOptIn(context.Background(), testUserID, SetShadowOptInInput{
		ExpectedGeneration: 0, PolicyRevision: policy.Revision,
		OptedIn: true, ReasonCode: "USER_OPT_IN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !shadow.OptIn.OptedIn || repository.optIn.UserID != testUserID ||
		repository.optIn.PolicyRevision != policy.Revision {
		t.Fatalf("shadow=%#v captured=%#v", shadow, repository.optIn)
	}
}

func TestServiceScheduleAndDraftMutationsKeepOwningAuthority(t *testing.T) {
	cron := &fakeCronService{template: agentcron.Template{
		ID: "cron_1234567890abcdef", UserID: testUserID,
		CurrentRevision: 3, State: agentcron.TemplatePaused,
	}}
	learning := &fakeLearningService{}
	repository := &fakeRepository{}
	service := NewService(
		WithRepository(repository), WithCron(cron), WithLearning(learning),
		WithAdministratorUserID(testAdminID),
	)

	_, _, err := service.CreateSchedule(context.Background(), testUserID, agentcron.CreateInput{
		TemplateID: cron.template.ID, ExpectedRevision: 0,
		Approval: agentcron.Approval{ReasonCode: "USER_CREATED"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cron.create.Spec.Owner.UserID != testUserID || cron.create.Approval.ActorID != testUserID ||
		cron.create.Approval.ActorType != "user" {
		t.Fatalf("create=%#v", cron.create)
	}
	if _, err := service.ChangeScheduleLifecycle(context.Background(), testUserID,
		cron.template.ID, agentcron.TemplateActive, 2, "USER_ACTIVE"); !errors.Is(err, agentcron.ErrRevisionConflict) {
		t.Fatalf("stale resume err=%v", err)
	}
	if _, err := service.ChangeScheduleLifecycle(context.Background(), testUserID,
		cron.template.ID, agentcron.TemplateActive, 3, "USER_ACTIVE"); err != nil {
		t.Fatal(err)
	}
	if cron.resumeApproval.ActorID != testUserID || cron.resumeApproval.ReasonCode != "USER_ACTIVE" {
		t.Fatalf("resume approval=%#v", cron.resumeApproval)
	}
	if _, err := service.ChangeScheduleLifecycle(context.Background(), testUserID,
		cron.template.ID, agentcron.TemplatePaused, 3, "USER_PAUSED"); err != nil {
		t.Fatal(err)
	}
	if cron.lifecycle.UserID != testUserID || cron.lifecycle.ExpectedRevision != 3 {
		t.Fatalf("lifecycle=%#v", cron.lifecycle)
	}

	review := agentlearning.ReviewInput{
		DraftID: "draft_1234567890abcdef", ExpectedRevision: 4,
		DraftFingerprint: testFP, ProposedPackageFingerprint: testRuntime,
		ReasonCode: "ADMIN_REJECTED",
	}
	if _, err := service.ReviewDraft(context.Background(), testUserID, "reject", review); !errors.Is(err, ErrAdministratorNeeded) {
		t.Fatalf("non-admin review err=%v", err)
	}
	repositoryDraft := DraftSummary{ID: review.DraftID, Revision: 5}
	repository.draft = repositoryDraft
	result, err := service.ReviewDraft(context.Background(), testAdminID, "reject", review)
	if err != nil {
		t.Fatal(err)
	}
	if result.Draft == nil || result.Draft.ID != review.DraftID ||
		learning.administratorID != testAdminID || learning.review != review {
		t.Fatalf("review result=%#v admin=%q input=%#v", result, learning.administratorID, learning.review)
	}
}

var _ Repository = (*fakeRepository)(nil)
var _ BrokerService = (*fakeBrokerService)(nil)
var _ CronService = (*fakeCronService)(nil)
var _ LearningService = (*fakeLearningService)(nil)

type fakeCronService struct {
	template       agentcron.Template
	create         agentcron.CreateInput
	lifecycle      agentcron.LifecycleInput
	resumeApproval agentcron.Approval
}

func (service *fakeCronService) GetTemplate(context.Context, string, string) (agentcron.Template, error) {
	return service.template, nil
}
func (service *fakeCronService) CreateRevision(_ context.Context, input agentcron.CreateInput) (agentcron.Template, bool, error) {
	service.create = input
	return service.template, true, nil
}
func (service *fakeCronService) SetLifecycle(_ context.Context, input agentcron.LifecycleInput) (agentcron.Template, error) {
	service.lifecycle = input
	return service.template, nil
}
func (service *fakeCronService) Resume(_ context.Context, template agentcron.Template, approval agentcron.Approval) (agentcron.Template, error) {
	service.template = template
	service.resumeApproval = approval
	return service.template, nil
}

type fakeLearningService struct {
	administratorID string
	review          agentlearning.ReviewInput
}

func (*fakeLearningService) GetDiff(context.Context, string, string) ([]agentlearning.FileDiff, error) {
	return nil, nil
}
func (service *fakeLearningService) Reject(_ context.Context, administratorID string, input agentlearning.ReviewInput) (agentlearning.Draft, bool, error) {
	service.administratorID = administratorID
	service.review = input
	return agentlearning.Draft{}, false, nil
}
func (service *fakeLearningService) Promote(_ context.Context, administratorID string, input agentlearning.ReviewInput) (agentlearning.Promotion, error) {
	service.administratorID = administratorID
	service.review = input
	return agentlearning.Promotion{}, nil
}
