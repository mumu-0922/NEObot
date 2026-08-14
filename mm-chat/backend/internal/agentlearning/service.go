package agentlearning

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"neo-chat/mm-chat/backend/internal/skillsupply"
	"neo-chat/mm-chat/backend/internal/storage"
)

var (
	uuidPattern   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	idPattern     = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}_[a-z0-9]{16,64}$`)
	reasonPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	metricPattern = regexp.MustCompile(`^[a-z][A-Za-z0-9]{0,63}$`)
)

type Service struct {
	repository      Repository
	objects         storage.ObjectStore
	administratorID string
	static          Checker
	isolation       Checker
	evaluation      Checker
	enabled         bool
	now             func() time.Time
	newID           func(string) string
	newUUID         func() string
}

type ServiceOption func(*Service)

func WithRepository(repository Repository) ServiceOption {
	return func(service *Service) { service.repository = repository }
}

func WithObjectStore(objects storage.ObjectStore) ServiceOption {
	return func(service *Service) { service.objects = objects }
}

func WithAdministratorUserID(userID string) ServiceOption {
	return func(service *Service) { service.administratorID = strings.TrimSpace(userID) }
}

func WithCheckers(static, isolation, evaluation Checker) ServiceOption {
	return func(service *Service) {
		if static != nil {
			service.static = static
		}
		if isolation != nil {
			service.isolation = isolation
		}
		if evaluation != nil {
			service.evaluation = evaluation
		}
	}
}

func WithLearningEnabled(enabled bool) ServiceOption {
	return func(service *Service) { service.enabled = enabled }
}

func NewService(options ...ServiceOption) *Service {
	service := &Service{
		static: StaticChecker{}, isolation: unavailableChecker{kind: CheckIsolation},
		evaluation: unavailableChecker{kind: CheckEvaluation}, now: time.Now,
		newID: func(prefix string) string {
			return prefix + "_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		},
		newUUID: uuid.NewString,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (service *Service) Propose(ctx context.Context, input ProposeInput) (Draft, bool, error) {
	if err := service.requireEnabled(); err != nil {
		return Draft{}, false, err
	}
	input.UserID = strings.ToLower(strings.TrimSpace(input.UserID))
	input.SourceRunID = strings.TrimSpace(input.SourceRunID)
	input.BasePackageFingerprint = strings.TrimSpace(input.BasePackageFingerprint)
	if !uuidPattern.MatchString(input.UserID) || !validID(input.SourceRunID, "run") ||
		!digestPattern.MatchString(input.BasePackageFingerprint) || len(input.Archive) == 0 ||
		int64(len(input.Archive)) > skillsupply.MaxSourceArchiveBytes {
		return Draft{}, false, ErrInvalidInput
	}
	source, err := service.repository.GetSourceRun(ctx, input.UserID, input.SourceRunID,
		input.BasePackageFingerprint)
	if err != nil {
		return Draft{}, false, err
	}
	if source.State != "succeeded" || source.Depth != 0 {
		return Draft{}, false, ErrSourceInvalid
	}
	base, err := service.repository.GetBasePackage(ctx, input.BasePackageFingerprint)
	if err != nil {
		return Draft{}, false, err
	}
	baseArchive, err := service.readObject(ctx, base.Package.PackageObjectKey,
		base.Package.PackageBytes, "")
	if err != nil {
		return Draft{}, false, err
	}
	baseValidated, err := skillsupply.ValidateArchive(skillsupply.ArchiveSource{
		Type: skillsupply.SourceLearning, Ref: "base:" + input.BasePackageFingerprint,
		Data: baseArchive,
	})
	if err != nil || baseValidated.Package.PackageFingerprint != input.BasePackageFingerprint {
		return Draft{}, false, ErrSourceDrift
	}
	proposed, err := skillsupply.ValidateArchive(skillsupply.ArchiveSource{
		Type: skillsupply.SourceLearning, Ref: "draft:" + input.SourceRunID, Data: input.Archive,
	})
	if err != nil {
		return Draft{}, false, err
	}
	if proposed.Package.PackageFingerprint == input.BasePackageFingerprint {
		return Draft{}, false, ErrInvalidInput
	}
	if !authorityUnchanged(baseValidated, proposed) {
		return Draft{}, false, ErrAuthorityWidened
	}
	tests, testsJSON, testsFingerprint, err := deriveTests(proposed.Inventory)
	if err != nil {
		return Draft{}, false, err
	}
	changed, err := changedPaths(baseValidated.CanonicalArchive, proposed.CanonicalArchive)
	if err != nil || len(changed) == 0 || len(changed) > 2048 {
		return Draft{}, false, ErrInvalidInput
	}
	evidence, evidenceJSON, evidenceFingerprint, err := canonicalEvidence(input.Evidence)
	if err != nil || validateEvidenceCoverage(evidence, input.BasePackageFingerprint, changed) != nil {
		return Draft{}, false, ErrInvalidInput
	}
	archiveFingerprint := contentFingerprint(proposed.CanonicalArchive)
	spec := DraftSpec{
		SchemaVersion: SchemaVersion, SourceRunID: source.RunID,
		SourceSnapshotID: source.SnapshotID, SourceSnapshotFingerprint: source.SnapshotFingerprint,
		BasePackageFingerprint:     input.BasePackageFingerprint,
		ProposedPackageFingerprint: proposed.Package.PackageFingerprint,
		RuntimeBundleFingerprint:   proposed.Package.RuntimeBundleFingerprint,
		SBOMFingerprint:            proposed.Package.SBOMFingerprint,
		ArchiveFingerprint:         archiveFingerprint, EvidenceFingerprint: evidenceFingerprint,
		TestFingerprint: testsFingerprint, Name: proposed.Package.Name,
		Version: proposed.Package.Version, ArchiveBytes: int64(len(proposed.CanonicalArchive)),
		Evidence: evidence, Tests: tests, ChangedPaths: changed,
	}
	draftFP, _, err := draftFingerprint(spec)
	if err != nil {
		return Draft{}, false, err
	}
	objectKey := digestObjectKey("skill-drafts", archiveFingerprint, ".zip")
	if err := service.storeImmutableObject(ctx, objectKey, proposed.CanonicalArchive,
		"application/zip", archiveFingerprint); err != nil {
		return Draft{}, false, fmt.Errorf("store Agent Draft quarantine: %w", err)
	}
	now := service.now().UTC()
	draft := Draft{ID: service.newID("draft"), UserID: input.UserID,
		DraftFingerprint: draftFP, Spec: spec, State: StateQuarantined, Revision: 1,
		DraftObjectKey: objectKey, BasePackageObjectKey: base.Package.PackageObjectKey,
		CreatedAt: now, UpdatedAt: now}
	return service.repository.CreateDraft(ctx, preparedDraft{Draft: draft,
		EvidenceJSON: evidenceJSON, TestsJSON: testsJSON})
}

func (service *Service) RunChecks(ctx context.Context, request ClaimRequest) ([]Draft, error) {
	if err := service.requireEnabled(); err != nil {
		return nil, err
	}
	if err := validateClaimRequest(request); err != nil {
		return nil, err
	}
	if _, err := service.repository.Reconcile(ctx, request.Now.UTC(), request.Limit); err != nil {
		return nil, err
	}
	claims, err := service.repository.ClaimChecks(ctx, request)
	if err != nil {
		return nil, err
	}
	completed := make([]Draft, 0, len(claims))
	for _, claim := range claims {
		archive, readErr := service.readObject(ctx, claim.DraftObjectKey,
			claim.Spec.ArchiveBytes, claim.Spec.ArchiveFingerprint)
		if readErr != nil {
			if releaseErr := service.releaseCheck(ctx, claim, "DRAFT_OBJECT_DRIFT"); releaseErr != nil {
				return completed, releaseErr
			}
			return completed, readErr
		}
		receipts, checkErr := service.runCheckBundle(ctx, claim, archive)
		if checkErr != nil {
			if releaseErr := service.releaseCheck(ctx, claim, "CHECK_UNAVAILABLE"); releaseErr != nil {
				return completed, releaseErr
			}
			return completed, checkErr
		}
		updated, completeErr := service.repository.CompleteChecks(ctx, claim, receipts)
		if completeErr != nil {
			return completed, completeErr
		}
		completed = append(completed, updated)
	}
	return completed, nil
}

func (service *Service) GetDiff(
	ctx context.Context,
	administratorID, draftID string,
) ([]FileDiff, error) {
	if err := service.requireAdministrator(administratorID); err != nil {
		return nil, err
	}
	draft, err := service.repository.GetDraft(ctx, "", strings.TrimSpace(draftID))
	if err != nil {
		return nil, err
	}
	base, err := service.repository.GetBasePackage(ctx, draft.Spec.BasePackageFingerprint)
	if err != nil {
		return nil, err
	}
	baseArchive, err := service.readObject(ctx, base.Package.PackageObjectKey,
		base.Package.PackageBytes, "")
	if err != nil {
		return nil, err
	}
	baseValidated, err := skillsupply.ValidateArchive(skillsupply.ArchiveSource{
		Type: skillsupply.SourceLearning, Ref: "base:" + draft.Spec.BasePackageFingerprint,
		Data: baseArchive,
	})
	if err != nil || baseValidated.Package.PackageFingerprint != draft.Spec.BasePackageFingerprint {
		return nil, ErrObjectDrift
	}
	draftArchive, err := service.readObject(ctx, draft.DraftObjectKey,
		draft.Spec.ArchiveBytes, draft.Spec.ArchiveFingerprint)
	if err != nil {
		return nil, err
	}
	return buildDiff(baseArchive, draftArchive)
}

func (service *Service) Reject(
	ctx context.Context,
	administratorID string,
	input ReviewInput,
) (Draft, bool, error) {
	if err := service.requireAdministrator(administratorID); err != nil {
		return Draft{}, false, err
	}
	if err := validateReviewInput(input); err != nil {
		return Draft{}, false, err
	}
	return service.repository.Reject(ctx, strings.ToLower(strings.TrimSpace(administratorID)),
		input, service.newID("draft_decision"), input.ReasonCode)
}

func (service *Service) Promote(
	ctx context.Context,
	administratorID string,
	input ReviewInput,
) (Promotion, error) {
	if err := service.requireEnabled(); err != nil {
		return Promotion{}, err
	}
	if err := service.requireAdministrator(administratorID); err != nil {
		return Promotion{}, err
	}
	if err := validateReviewInput(input); err != nil {
		return Promotion{}, err
	}
	draft, err := service.repository.GetDraft(ctx, "", input.DraftID)
	if err != nil {
		return Promotion{}, err
	}
	if draft.DraftFingerprint != input.DraftFingerprint ||
		draft.Spec.ProposedPackageFingerprint != input.ProposedPackageFingerprint ||
		(draft.State != StatePromoted && draft.Revision != input.ExpectedRevision) {
		return Promotion{}, ErrRevisionConflict
	}
	if draft.State == StatePromoted {
		return service.repository.ReplayPromotion(ctx,
			strings.ToLower(strings.TrimSpace(administratorID)), input)
	}
	archive, err := service.readObject(ctx, draft.DraftObjectKey,
		draft.Spec.ArchiveBytes, draft.Spec.ArchiveFingerprint)
	if err != nil {
		return Promotion{}, err
	}
	validated, err := skillsupply.ValidateArchive(skillsupply.ArchiveSource{
		Type: skillsupply.SourceLearning, Ref: "draft:" + draft.ID, Data: archive,
	})
	if err != nil || validated.Package.PackageFingerprint != draft.Spec.ProposedPackageFingerprint ||
		validated.Package.RuntimeBundleFingerprint != draft.Spec.RuntimeBundleFingerprint ||
		validated.Package.SBOMFingerprint != draft.Spec.SBOMFingerprint {
		return Promotion{}, ErrObjectDrift
	}
	base, err := service.repository.GetBasePackage(ctx, draft.Spec.BasePackageFingerprint)
	if err != nil {
		return Promotion{}, err
	}
	baseArchive, err := service.readObject(ctx, base.Package.PackageObjectKey,
		base.Package.PackageBytes, "")
	if err != nil {
		return Promotion{}, err
	}
	baseValidated, err := skillsupply.ValidateArchive(skillsupply.ArchiveSource{
		Type: skillsupply.SourceLearning, Ref: "base:" + draft.Spec.BasePackageFingerprint,
		Data: baseArchive,
	})
	if err != nil || baseValidated.Package.PackageFingerprint != draft.Spec.BasePackageFingerprint {
		return Promotion{}, ErrObjectDrift
	}
	if !authorityUnchanged(baseValidated, validated) {
		return Promotion{}, ErrAuthorityWidened
	}
	packageKey := digestObjectKey("skill-packages", validated.Package.PackageFingerprint, ".zip")
	sbomKey := digestObjectKey("skill-sboms", validated.Package.SBOMFingerprint, ".cdx.json")
	sourceKey := digestObjectKey("skill-quarantine", draft.Spec.ArchiveFingerprint, ".zip")
	if err := service.storeImmutableObject(ctx, sourceKey, archive,
		"application/zip", draft.Spec.ArchiveFingerprint); err != nil {
		return Promotion{}, fmt.Errorf("store learning promotion source: %w", err)
	}
	if err := service.storeImmutableObject(ctx, packageKey, validated.CanonicalArchive,
		"application/zip", contentFingerprint(validated.CanonicalArchive)); err != nil {
		return Promotion{}, fmt.Errorf("store learning canonical package: %w", err)
	}
	if err := service.storeImmutableObject(ctx, sbomKey, validated.SBOM,
		"application/vnd.cyclonedx+json", validated.Package.SBOMFingerprint); err != nil {
		return Promotion{}, fmt.Errorf("store learning SBOM: %w", err)
	}
	validated.Package.PackageObjectKey = packageKey
	validated.Package.SBOMObjectKey = sbomKey
	allowed, _ := json.Marshal(nonNilStrings(validated.Package.AllowedTools))
	capabilities, _ := json.Marshal(nonNilCapabilities(validated.Package.CapabilityRequests))
	return service.repository.Promote(ctx, preparedPromotion{
		ReviewInput: input, AdministratorID: strings.ToLower(strings.TrimSpace(administratorID)),
		CandidateID: service.newUUID(), DecisionID: service.newID("draft_decision"),
		SourceRef:       "draft:" + draft.ID + ":" + draft.DraftFingerprint,
		SourceObjectKey: sourceKey, Package: validated.Package,
		AllowedToolsJSON: allowed, CapabilitiesJSON: capabilities,
	})
}

func (service *Service) Cleanup(ctx context.Context, request ClaimRequest) (CleanupResult, error) {
	if service == nil || service.repository == nil {
		return CleanupResult{}, ErrDatabaseRequired
	}
	if service.objects == nil {
		return CleanupResult{}, ErrObjectStoreRequired
	}
	if err := validateClaimRequest(request); err != nil {
		return CleanupResult{}, err
	}
	if _, err := service.repository.Reconcile(ctx, request.Now.UTC(), request.Limit); err != nil {
		return CleanupResult{}, err
	}
	claims, err := service.repository.ClaimCleanup(ctx, request)
	if err != nil {
		return CleanupResult{}, err
	}
	result := CleanupResult{}
	for _, claim := range claims {
		draft, draftErr := service.repository.GetDraft(ctx, "", claim.DraftID)
		if draftErr != nil {
			return result, draftErr
		}
		if draft.DraftObjectKey != claim.ObjectKey || draft.Spec.ArchiveFingerprint != claim.ObjectFingerprint {
			result.Failed++
			if _, releaseErr := service.repository.ReleaseCleanup(ctx, claim,
				"DRAFT_OBJECT_DRIFT", service.cleanupRetryAt(claim)); releaseErr != nil {
				return result, releaseErr
			}
			continue
		}
		_, readErr := service.readObject(ctx, claim.ObjectKey,
			draft.Spec.ArchiveBytes, claim.ObjectFingerprint)
		missing := errors.Is(readErr, storage.ErrObjectNotFound)
		if readErr != nil && !missing {
			result.Failed++
			reason := "OBJECT_READ_FAILED"
			if errors.Is(readErr, ErrObjectDrift) {
				reason = "DRAFT_OBJECT_DRIFT"
			}
			if _, releaseErr := service.repository.ReleaseCleanup(ctx, claim,
				reason, service.cleanupRetryAt(claim)); releaseErr != nil {
				return result, releaseErr
			}
			continue
		}
		if !missing {
			if deleteErr := service.objects.Delete(ctx, claim.ObjectKey); deleteErr != nil &&
				!errors.Is(deleteErr, storage.ErrObjectNotFound) {
				result.Failed++
				if _, releaseErr := service.repository.ReleaseCleanup(ctx, claim,
					"OBJECT_DELETE_FAILED", service.cleanupRetryAt(claim)); releaseErr != nil {
					return result, releaseErr
				}
				continue
			}
		}
		if err := service.repository.CompleteCleanup(ctx, claim); err != nil {
			return result, err
		}
		result.Completed++
	}
	return result, nil
}

func (service *Service) cleanupRetryAt(claim CleanupClaim) time.Time {
	return service.now().UTC().Add(time.Duration(min(claim.Attempts+1, 10)) * time.Minute)
}

func (service *Service) Reconcile(ctx context.Context, now time.Time, limit int) (ReconcileResult, error) {
	if service == nil || service.repository == nil {
		return ReconcileResult{}, ErrDatabaseRequired
	}
	if now.IsZero() || limit < 1 || limit > 1000 {
		return ReconcileResult{}, ErrInvalidInput
	}
	return service.repository.Reconcile(ctx, now.UTC(), limit)
}

func (service *Service) Prune(ctx context.Context, cutoff time.Time, limit int) (PruneResult, error) {
	if service == nil || service.repository == nil {
		return PruneResult{}, ErrDatabaseRequired
	}
	if cutoff.IsZero() || cutoff.After(service.now().UTC()) || limit < 1 || limit > 1000 {
		return PruneResult{}, ErrInvalidInput
	}
	return service.repository.Prune(ctx, cutoff.UTC(), limit)
}

func (service *Service) runCheckBundle(
	ctx context.Context,
	claim CheckClaim,
	archive []byte,
) ([]CheckReceipt, error) {
	input := CheckInput{Draft: claim.Draft, Archive: archive}
	static, err := service.runCheck(ctx, CheckStatic, service.static, input)
	if err != nil {
		return nil, err
	}
	var isolation, evaluation CheckReceipt
	if static.Status == CheckPassed {
		isolation, err = service.runCheck(ctx, CheckIsolation, service.isolation, input)
		if err != nil {
			return nil, err
		}
	} else {
		isolation = service.blockedReceipt(CheckIsolation, "BLOCKED_BY_STATIC", claim)
	}
	if static.Status == CheckPassed && isolation.Status == CheckPassed {
		evaluation, err = service.runCheck(ctx, CheckEvaluation, service.evaluation, input)
		if err != nil {
			return nil, err
		}
	} else {
		reason := "BLOCKED_BY_STATIC"
		if static.Status == CheckPassed {
			reason = "BLOCKED_BY_ISOLATION"
		}
		evaluation = service.blockedReceipt(CheckEvaluation, reason, claim)
	}
	return []CheckReceipt{static, isolation, evaluation}, nil
}

func (service *Service) runCheck(
	ctx context.Context,
	kind string,
	checker Checker,
	input CheckInput,
) (CheckReceipt, error) {
	if checker == nil {
		return CheckReceipt{}, ErrCheckUnavailable
	}
	result, err := checker.Check(ctx, input)
	if err != nil {
		return CheckReceipt{}, err
	}
	if !validCheckResult(result) {
		return CheckReceipt{}, ErrInvalidInput
	}
	metrics := nonNilMetrics(result.Metrics)
	encoded, _ := json.Marshal(metrics)
	evidence := []byte(strings.Join([]string{input.Draft.DraftFingerprint, kind,
		result.Status, result.ReasonCode, result.SuiteFingerprint, string(encoded)}, "\x00"))
	return CheckReceipt{ID: service.newID("draft_check"), Kind: kind,
		Status: result.Status, ReasonCode: result.ReasonCode,
		SuiteFingerprint:    result.SuiteFingerprint,
		EvidenceFingerprint: fingerprint("neo-agent-learning-check-receipt-v1", evidence),
		DurationMillis:      result.DurationMillis, Metrics: metrics}, nil
}

func (service *Service) blockedReceipt(kind, reason string, claim CheckClaim) CheckReceipt {
	suite := fingerprint("neo-agent-learning-blocked-suite-v1", []byte(kind+"\x00"+reason))
	evidence := fingerprint("neo-agent-learning-check-receipt-v1",
		[]byte(claim.DraftFingerprint+"\x00"+kind+"\x00"+CheckFailed+"\x00"+reason+"\x00"+suite+"\x00{}"))
	return CheckReceipt{ID: service.newID("draft_check"), Kind: kind, Status: CheckFailed,
		ReasonCode: reason, SuiteFingerprint: suite, EvidenceFingerprint: evidence,
		Metrics: map[string]int64{}}
}

func (service *Service) releaseCheck(ctx context.Context, claim CheckClaim, reason string) error {
	retryAt := service.now().UTC().Add(time.Duration(min(claim.CheckAttempts+1, 10)) * time.Minute)
	_, err := service.repository.ReleaseCheck(ctx, claim, reason, retryAt)
	return err
}

func (service *Service) readObject(
	ctx context.Context,
	key string,
	expectedSize int64,
	expectedFingerprint string,
) ([]byte, error) {
	if service == nil || service.objects == nil {
		return nil, ErrObjectStoreRequired
	}
	if expectedSize < 1 || expectedSize > 128<<20 {
		return nil, ErrObjectDrift
	}
	reader, info, err := service.objects.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("read Agent learning object: %w", err)
	}
	defer reader.Close()
	if info.Size != expectedSize {
		return nil, ErrObjectDrift
	}
	data, err := io.ReadAll(io.LimitReader(reader, expectedSize+1))
	if err != nil || int64(len(data)) != expectedSize {
		return nil, ErrObjectDrift
	}
	if expectedFingerprint != "" && contentFingerprint(data) != expectedFingerprint {
		return nil, ErrObjectDrift
	}
	return data, nil
}

func (service *Service) storeImmutableObject(
	ctx context.Context,
	key string,
	data []byte,
	contentType, expectedFingerprint string,
) error {
	if service == nil || service.objects == nil || len(data) == 0 ||
		!digestPattern.MatchString(expectedFingerprint) || contentFingerprint(data) != expectedFingerprint {
		return ErrObjectDrift
	}
	existing, info, err := service.objects.Get(ctx, key)
	if err == nil {
		defer existing.Close()
		if info.Size != int64(len(data)) {
			return ErrObjectDrift
		}
		value, readErr := io.ReadAll(io.LimitReader(existing, int64(len(data))+1))
		if readErr != nil || !bytes.Equal(value, data) {
			return ErrObjectDrift
		}
		return nil
	}
	if !errors.Is(err, storage.ErrObjectNotFound) {
		return err
	}
	if err := service.objects.Put(ctx, key, bytes.NewReader(data), int64(len(data)), contentType); err != nil {
		return err
	}
	stored, readErr := service.readObject(ctx, key, int64(len(data)), expectedFingerprint)
	if readErr != nil || !bytes.Equal(stored, data) {
		return ErrObjectDrift
	}
	return nil
}

func (service *Service) requireEnabled() error {
	if service == nil || service.repository == nil {
		return ErrDatabaseRequired
	}
	if service.objects == nil {
		return ErrObjectStoreRequired
	}
	if !service.enabled {
		return ErrLearningDisabled
	}
	return nil
}

func (service *Service) requireAdministrator(userID string) error {
	if service == nil || service.repository == nil {
		return ErrDatabaseRequired
	}
	userID = strings.ToLower(strings.TrimSpace(userID))
	if !uuidPattern.MatchString(userID) || userID != strings.ToLower(service.administratorID) {
		return ErrAdministratorNeeded
	}
	return nil
}

func validateClaimRequest(request ClaimRequest) error {
	if request.Owner == "" || strings.TrimSpace(request.Owner) != request.Owner || len(request.Owner) > 128 ||
		request.Now.IsZero() || request.LeaseDuration < 5*time.Second || request.LeaseDuration > 5*time.Minute ||
		request.Limit < 1 || request.Limit > 1000 {
		return ErrInvalidInput
	}
	return nil
}

func validateReviewInput(input ReviewInput) error {
	if !validID(input.DraftID, "draft") || input.ExpectedRevision < 1 ||
		!digestPattern.MatchString(input.DraftFingerprint) ||
		!digestPattern.MatchString(input.ProposedPackageFingerprint) ||
		!reasonPattern.MatchString(input.ReasonCode) {
		return ErrInvalidInput
	}
	return nil
}

func validCheckResult(result CheckResult) bool {
	if (result.Status != CheckPassed && result.Status != CheckFailed) ||
		!reasonPattern.MatchString(result.ReasonCode) ||
		!digestPattern.MatchString(result.SuiteFingerprint) || result.DurationMillis < 0 ||
		result.DurationMillis > 86_400_000 || len(result.Metrics) > 16 {
		return false
	}
	for key, value := range result.Metrics {
		if !metricPattern.MatchString(key) || value < 0 || value > 1<<53 {
			return false
		}
	}
	return true
}

func validID(value, prefix string) bool {
	return idPattern.MatchString(value) && strings.HasPrefix(value, prefix+"_")
}

func digestObjectKey(prefix, value, suffix string) string {
	return prefix + "/sha256/" + strings.TrimPrefix(value, "sha256:") + suffix
}

func nonNilStrings(value []string) []string {
	if value == nil {
		return []string{}
	}
	return value
}

func nonNilCapabilities(value []skillsupply.CapabilityRequest) []skillsupply.CapabilityRequest {
	if value == nil {
		return []skillsupply.CapabilityRequest{}
	}
	return value
}
