package skillsupply

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"neo-chat/mm-chat/backend/internal/storage"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type Service struct {
	repository          Repository
	objects             storage.ObjectStore
	administratorUserID string
	official            OfficialSource
	lobehub             LobeHubSource
	git                 GitHubSource
	newID               func() string
	now                 func() time.Time
	runtimeMu           sync.Mutex
}

type ServiceOption func(*Service)

func WithRepository(repository Repository) ServiceOption {
	return func(service *Service) { service.repository = repository }
}

func WithObjectStore(objects storage.ObjectStore) ServiceOption {
	return func(service *Service) { service.objects = objects }
}

func WithAdministratorUserID(userID string) ServiceOption {
	return func(service *Service) { service.administratorUserID = strings.TrimSpace(userID) }
}

func WithLobeHubFetcher(fetcher LobeHubFetcher) ServiceOption {
	return func(service *Service) { service.lobehub = LobeHubSource{Fetcher: fetcher} }
}

func WithGitHubClient(client SourceHTTPClient) ServiceOption {
	return func(service *Service) { service.git.Client = client }
}

func NewService(options ...ServiceOption) *Service {
	service := &Service{official: OfficialSource{}, newID: uuid.NewString, now: time.Now}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (service *Service) IsAdministrator(userID string) bool {
	if service == nil {
		return false
	}
	return strings.TrimSpace(userID) != "" && strings.EqualFold(
		strings.TrimSpace(userID), strings.TrimSpace(service.administratorUserID),
	)
}

func (service *Service) IngestOfficial(
	ctx context.Context,
	userID, identifier, version string,
) (Candidate, error) {
	if err := service.requireAdministrator(userID); err != nil {
		return Candidate{}, err
	}
	source, err := service.official.Fetch(ctx, identifier, version)
	if err != nil {
		return Candidate{}, err
	}
	return service.ingest(ctx, source)
}

func (service *Service) IngestLobeHub(
	ctx context.Context,
	userID, identifier, version string,
) (Candidate, error) {
	if err := service.requireAdministrator(userID); err != nil {
		return Candidate{}, err
	}
	source, err := service.lobehub.Fetch(ctx, identifier, version)
	if err != nil {
		return Candidate{}, err
	}
	return service.ingest(ctx, source)
}

func (service *Service) IngestGit(
	ctx context.Context,
	userID, repositoryURL, commit, subdirectory string,
) (Candidate, error) {
	if err := service.requireAdministrator(userID); err != nil {
		return Candidate{}, err
	}
	source, err := service.git.Fetch(ctx, repositoryURL, commit, subdirectory)
	if err != nil {
		return Candidate{}, err
	}
	return service.ingest(ctx, source)
}

func (service *Service) IngestZIP(ctx context.Context, userID string, data []byte) (Candidate, error) {
	if err := service.requireAdministrator(userID); err != nil {
		return Candidate{}, err
	}
	source, err := ZIPSource(data)
	if err != nil {
		return Candidate{}, err
	}
	return service.ingest(ctx, source)
}

func (service *Service) ingest(ctx context.Context, source ArchiveSource) (Candidate, error) {
	if service == nil || service.repository == nil || service.objects == nil {
		return Candidate{}, ErrUnavailable
	}
	validated, err := ValidateArchive(source)
	if err != nil {
		return Candidate{}, err
	}
	if previous, findErr := service.repository.GetCandidateBySource(ctx, source.Type, source.Ref); findErr == nil {
		if previous.SourceArtifactSHA256 != validated.SourceArtifactSHA256 {
			return Candidate{}, ErrSourceDrift
		}
		if previous.Package.PackageFingerprint != validated.Package.PackageFingerprint ||
			previous.Package.SBOMFingerprint != validated.Package.SBOMFingerprint {
			return Candidate{}, ErrPackageCollision
		}
		return previous, nil
	} else if !errors.Is(findErr, ErrCandidateNotFound) {
		return Candidate{}, findErr
	}

	sourceObjectKey := digestObjectKey("skill-quarantine", validated.SourceArtifactSHA256, ".zip")
	packageObjectKey := digestObjectKey("skill-packages", validated.Package.PackageFingerprint, ".zip")
	sbomObjectKey := digestObjectKey("skill-sboms", validated.Package.SBOMFingerprint, ".cdx.json")
	if err := service.objects.Put(ctx, sourceObjectKey, bytes.NewReader(source.Data), int64(len(source.Data)), "application/zip"); err != nil {
		return Candidate{}, fmt.Errorf("store skill quarantine object: %w", err)
	}
	if err := service.objects.Put(ctx, packageObjectKey, bytes.NewReader(validated.CanonicalArchive), int64(len(validated.CanonicalArchive)), "application/zip"); err != nil {
		return Candidate{}, fmt.Errorf("store canonical skill package: %w", err)
	}
	if err := service.objects.Put(ctx, sbomObjectKey, bytes.NewReader(validated.SBOM), int64(len(validated.SBOM)), "application/vnd.cyclonedx+json"); err != nil {
		return Candidate{}, fmt.Errorf("store skill SBOM: %w", err)
	}
	validated.Package.PackageObjectKey = packageObjectKey
	validated.Package.SBOMObjectKey = sbomObjectKey
	now := service.now().UTC()
	candidate := Candidate{
		ID: service.newID(), SourceType: source.Type, SourceRef: source.Ref,
		SourceArtifactSHA256: validated.SourceArtifactSHA256, SourceObjectKey: sourceObjectKey,
		Package: validated.Package, Status: StatusValidated,
		AdmissionEligible: AdmissionEligible(source, validated),
		ValidationSummary: "validated_no_execute", Revision: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	return service.repository.CreateCandidate(ctx, candidate)
}

func (service *Service) GetCandidate(ctx context.Context, userID, candidateID string) (Candidate, error) {
	if err := service.requireAdministrator(userID); err != nil {
		return Candidate{}, err
	}
	if !validUUID(candidateID) {
		return Candidate{}, validationError("INVALID_SKILL_CANDIDATE", "skill candidate id is invalid")
	}
	return service.repository.GetCandidate(ctx, candidateID)
}

func (service *Service) ReviewCandidate(
	ctx context.Context,
	userID, candidateID string,
	input ReviewInput,
) (Candidate, error) {
	if err := service.requireAdministrator(userID); err != nil {
		return Candidate{}, err
	}
	input.Status, input.PackageFingerprint, input.Reason = strings.TrimSpace(input.Status),
		strings.TrimSpace(input.PackageFingerprint), strings.TrimSpace(input.Reason)
	if !validUUID(candidateID) || input.ExpectedRevision < 1 ||
		(input.Status != StatusAdmitted && input.Status != StatusRejected) ||
		!digestPattern.MatchString(input.PackageFingerprint) || len([]rune(input.Reason)) > 1000 ||
		!validPlainText(input.Reason) {
		return Candidate{}, validationError("INVALID_SKILL_REVIEW", "skill review is invalid")
	}
	current, err := service.repository.GetCandidate(ctx, candidateID)
	if err != nil {
		return Candidate{}, err
	}
	if current.Revision != input.ExpectedRevision || current.Package.PackageFingerprint != input.PackageFingerprint {
		return Candidate{}, ErrRevisionConflict
	}
	if input.Status == StatusAdmitted && !current.AdmissionEligible {
		return Candidate{}, ErrAdmissionIneligible
	}
	return service.repository.ReviewCandidate(ctx, candidateID, strings.TrimSpace(userID), input)
}

func (service *Service) ListStore(ctx context.Context, page, pageSize int) (StoreResult, error) {
	if service == nil || service.repository == nil {
		return StoreResult{}, ErrUnavailable
	}
	if page < 1 || page > 1000 || pageSize != 20 {
		return StoreResult{}, validationError("INVALID_SKILL_STORE_QUERY", "skill Store query is invalid")
	}
	return service.repository.ListStore(ctx, page, pageSize)
}

func (service *Service) GetStoreItem(ctx context.Context, admissionID string) (Candidate, error) {
	if service == nil || service.repository == nil {
		return Candidate{}, ErrUnavailable
	}
	if !validUUID(admissionID) {
		return Candidate{}, validationError("INVALID_SKILL_ADMISSION", "skill admission id is invalid")
	}
	return service.repository.GetStoreItem(ctx, admissionID)
}

func (service *Service) Install(
	ctx context.Context,
	userID, admissionID, packageFingerprint string,
) (Installation, error) {
	if service == nil || service.repository == nil {
		return Installation{}, ErrUnavailable
	}
	if strings.TrimSpace(userID) == "" || !validUUID(admissionID) || !digestPattern.MatchString(packageFingerprint) {
		return Installation{}, validationError("INVALID_SKILL_INSTALL", "skill install is invalid")
	}
	current, err := service.repository.GetStoreItem(ctx, admissionID)
	if err != nil {
		return Installation{}, err
	}
	if current.Package.PackageFingerprint != packageFingerprint {
		return Installation{}, ErrPackageChanged
	}
	return service.repository.Install(ctx, strings.TrimSpace(userID), admissionID, packageFingerprint)
}

func (service *Service) ListLibrary(ctx context.Context, userID string) ([]Installation, error) {
	if service == nil || service.repository == nil {
		return nil, ErrUnavailable
	}
	if strings.TrimSpace(userID) == "" {
		return nil, validationError("INVALID_SKILL_LIBRARY", "skill library owner is invalid")
	}
	return service.repository.ListLibrary(ctx, strings.TrimSpace(userID))
}

func (service *Service) Uninstall(
	ctx context.Context,
	userID, installationID string,
	expectedRevision int64,
) error {
	if service == nil || service.repository == nil {
		return ErrUnavailable
	}
	if strings.TrimSpace(userID) == "" || !validUUID(installationID) || expectedRevision < 1 {
		return validationError("INVALID_SKILL_UNINSTALL", "skill uninstall is invalid")
	}
	return service.repository.Uninstall(ctx, strings.TrimSpace(userID), installationID, expectedRevision)
}

func (service *Service) requireAdministrator(userID string) error {
	if service == nil || service.repository == nil || service.objects == nil {
		return ErrUnavailable
	}
	if !service.IsAdministrator(userID) {
		return ErrAdministratorNeeded
	}
	return nil
}

func digestObjectKey(prefix, fingerprint, suffix string) string {
	return prefix + "/sha256/" + strings.TrimPrefix(fingerprint, "sha256:") + suffix
}

func validUUID(value string) bool {
	return uuidPattern.MatchString(strings.ToLower(strings.TrimSpace(value)))
}
