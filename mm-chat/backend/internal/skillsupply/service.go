package skillsupply

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
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
	direct              DirectSkillLinkSource
	catalog             *GitHubCatalogSource
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
	return func(service *Service) {
		service.git.Client = client
		service.direct.Client = client
		service.catalog.Client = client
	}
}

func NewService(options ...ServiceOption) *Service {
	service := &Service{
		official: OfficialSource{}, catalog: &GitHubCatalogSource{},
		newID: uuid.NewString, now: time.Now,
	}
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
	return service.ingest(ctx, "", source)
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
	return service.ingest(ctx, "", source)
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
	return service.ingest(ctx, "", source)
}

func (service *Service) IngestZIP(ctx context.Context, userID string, data []byte) (Candidate, error) {
	if err := service.requireAdministrator(userID); err != nil {
		return Candidate{}, err
	}
	source, err := ZIPSource(data)
	if err != nil {
		return Candidate{}, err
	}
	return service.ingest(ctx, "", source)
}

// InstallDirectSkillLink resolves one allowlisted exact GitHub Skill path or
// legacy AIHero discovery page, pins its GitHub source to an exact commit,
// validates the package without executing source instructions, and installs it
// into only the requesting owner's private library. It deliberately bypasses
// Store review/publication.
func (service *Service) InstallDirectSkillLink(
	ctx context.Context,
	userID, rawURL, expectedName string,
) (Installation, error) {
	if service == nil || service.repository == nil || service.objects == nil {
		return Installation{}, ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	expectedName = strings.TrimSpace(expectedName)
	if !validUserID(userID) || !skillNamePattern.MatchString(expectedName) {
		return Installation{}, validationError("INVALID_DIRECT_SKILL_INSTALL", "direct Skill install is invalid")
	}
	source, err := service.direct.Fetch(ctx, rawURL, expectedName)
	if err != nil {
		return Installation{}, err
	}
	candidate, err := service.ingest(ctx, userID, source)
	if err != nil {
		return Installation{}, err
	}
	return service.installPrivateCandidate(ctx, userID, candidate)
}

func (service *Service) ListCatalog(ctx context.Context) (CatalogResult, error) {
	if service == nil || service.catalog == nil {
		return CatalogResult{}, ErrUnavailable
	}
	items, err := service.catalog.List(ctx)
	if err != nil {
		return CatalogResult{}, err
	}
	return CatalogResult{
		Items: items, TotalCount: len(items), Source: defaultCatalogSource,
	}, nil
}

func (service *Service) GetCatalogSkill(ctx context.Context, name string) (CatalogSkill, error) {
	if service == nil || service.catalog == nil {
		return CatalogSkill{}, ErrUnavailable
	}
	detail, _, err := service.catalog.Detail(ctx, name)
	return detail, err
}

func (service *Service) InstallCatalogSkill(
	ctx context.Context,
	userID, name string,
	input CatalogInstallInput,
) (Installation, error) {
	if service == nil || service.repository == nil || service.objects == nil || service.catalog == nil {
		return Installation{}, ErrUnavailable
	}
	userID, name = strings.TrimSpace(userID), strings.TrimSpace(name)
	input.ResolvedCommit = strings.TrimSpace(input.ResolvedCommit)
	input.PackageFingerprint = strings.TrimSpace(input.PackageFingerprint)
	if !validUserID(userID) || !skillNamePattern.MatchString(name) ||
		!gitCommitPattern.MatchString(input.ResolvedCommit) ||
		!digestPattern.MatchString(input.PackageFingerprint) {
		return Installation{}, validationError("INVALID_SKILL_CATALOG_INSTALL", "catalog Skill install is invalid")
	}
	client := service.catalog.Client
	if client == nil {
		client = directSkillHTTPClient(service.catalog.Timeout)
	}
	source, err := (GitHubSource{Client: client, Timeout: service.catalog.Timeout}).Fetch(
		ctx,
		"https://github.com/"+defaultCatalogOwner+"/"+defaultCatalogRepository,
		input.ResolvedCommit,
		defaultCatalogPath+"/"+name,
	)
	if err != nil {
		return Installation{}, err
	}
	validated, err := ValidateArchive(source)
	if err != nil {
		return Installation{}, err
	}
	if validated.Package.PackageFingerprint != input.PackageFingerprint {
		return Installation{}, ErrPackageChanged
	}
	candidate, err := service.ingest(ctx, userID, source)
	if err != nil {
		return Installation{}, err
	}
	return service.installPrivateCandidate(ctx, userID, candidate)
}

func (service *Service) installPrivateCandidate(
	ctx context.Context,
	userID string,
	candidate Candidate,
) (Installation, error) {
	installed, err := service.repository.Install(
		ctx, userID, candidate.ID, candidate.Package.PackageFingerprint,
	)
	if err == nil {
		return installed, nil
	}
	reconcileCtx, cancelReconcile := context.WithTimeout(
		context.WithoutCancel(ctx),
		directInstallReconcileTimeout,
	)
	defer cancelReconcile()
	library, listErr := service.repository.ListLibrary(reconcileCtx, userID)
	if listErr != nil {
		if errors.Is(err, ErrInstallationConflict) {
			return Installation{}, listErr
		}
		return Installation{}, err
	}
	for _, existing := range library {
		if existing.AdmissionID == candidate.ID &&
			existing.PackageFingerprint == candidate.Package.PackageFingerprint {
			return existing, nil
		}
	}
	return Installation{}, err
}

func (service *Service) ingest(
	ctx context.Context,
	ownerUserID string,
	source ArchiveSource,
) (Candidate, error) {
	if service == nil || service.repository == nil || service.objects == nil {
		return Candidate{}, ErrUnavailable
	}
	validated, err := ValidateArchive(source)
	if err != nil {
		return Candidate{}, err
	}
	if previous, findErr := service.repository.GetCandidateBySource(
		ctx, source.Type, source.Ref, strings.TrimSpace(ownerUserID),
	); findErr == nil {
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
		OwnerUserID:          strings.TrimSpace(ownerUserID),
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
	if current.OwnerUserID != "" {
		return Candidate{}, ErrAdmissionDenied
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

func (service *Service) GetConversationSelection(
	ctx context.Context,
	userID string,
	conversationID string,
) (ConversationSelection, error) {
	if service == nil || service.repository == nil {
		return ConversationSelection{}, ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	conversationID = strings.TrimSpace(conversationID)
	if userID == "" || !validUUID(conversationID) {
		return ConversationSelection{}, ErrSelectionInvalid
	}
	selection, found, err := service.repository.GetConversationSelection(ctx, userID, conversationID)
	if err != nil {
		return ConversationSelection{}, err
	}
	if found {
		return selection, nil
	}
	if err := service.repository.AuthorizeConversation(ctx, userID, conversationID); err != nil {
		return ConversationSelection{}, err
	}
	return ConversationSelection{
		ConversationID: conversationID,
		Revision:       0,
		Skills:         []Installation{},
	}, nil
}

func (service *Service) ReplaceConversationSelection(
	ctx context.Context,
	userID string,
	conversationID string,
	revision int64,
	installationIDs []string,
) (ConversationSelection, error) {
	if service == nil || service.repository == nil {
		return ConversationSelection{}, ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	conversationID = strings.TrimSpace(conversationID)
	if userID == "" || !validUUID(conversationID) || revision < 0 || len(installationIDs) > 32 {
		return ConversationSelection{}, ErrSelectionInvalid
	}
	library, err := service.repository.ListLibrary(ctx, userID)
	if err != nil {
		return ConversationSelection{}, err
	}
	byID := make(map[string]Installation, len(library))
	for _, installation := range library {
		byID[installation.ID] = installation
	}
	selected := make([]Installation, 0, len(installationIDs))
	seen := make(map[string]struct{}, len(installationIDs))
	for _, rawID := range installationIDs {
		installationID := strings.TrimSpace(rawID)
		if !validUUID(installationID) {
			return ConversationSelection{}, ErrSelectionInvalid
		}
		if _, duplicate := seen[installationID]; duplicate {
			return ConversationSelection{}, ErrSelectionInvalid
		}
		installation, exists := byID[installationID]
		if !exists {
			return ConversationSelection{}, ErrSelectionInvalid
		}
		seen[installationID] = struct{}{}
		selected = append(selected, installation)
	}
	sort.Slice(selected, func(left, right int) bool {
		if selected[left].Name == selected[right].Name {
			return selected[left].ID < selected[right].ID
		}
		return selected[left].Name < selected[right].Name
	})
	return service.repository.ReplaceConversationSelection(ctx, userID, ConversationSelection{
		ConversationID: conversationID,
		Revision:       revision,
		Skills:         selected,
	})
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

func validUserID(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}
