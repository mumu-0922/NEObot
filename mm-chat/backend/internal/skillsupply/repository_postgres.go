package skillsupply

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct {
	db    *sql.DB
	newID func() string
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db, newID: uuid.NewString}
}

func (repository *PostgresRepository) CreateCandidate(
	ctx context.Context,
	candidate Candidate,
) (Candidate, error) {
	if err := repository.requireDB(); err != nil {
		return Candidate{}, err
	}
	allowedTools, err := json.Marshal(nonNilStrings(candidate.Package.AllowedTools))
	if err != nil {
		return Candidate{}, fmt.Errorf("marshal Skill allowed Tools: %w", err)
	}
	capabilities, err := json.Marshal(nonNilCapabilities(candidate.Package.CapabilityRequests))
	if err != nil {
		return Candidate{}, fmt.Errorf("marshal Skill capability requests: %w", err)
	}
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return Candidate{}, fmt.Errorf("begin Skill candidate transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var runtimeFingerprint any
	if candidate.Package.RuntimeBundleFingerprint != "" {
		runtimeFingerprint = candidate.Package.RuntimeBundleFingerprint
	}
	result, err := tx.ExecContext(ctx, `
INSERT INTO skill_package_versions (
  package_fingerprint, runtime_bundle_fingerprint, sbom_fingerprint, name,
  version, description, license, compatibility, allowed_tools,
  capability_requests, has_runtime, file_count, package_bytes, expanded_bytes,
  package_object_key, sbom_object_key, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10::jsonb, $11, $12,
          $13, $14, $15, $16, $17)
ON CONFLICT (package_fingerprint) DO NOTHING
`, candidate.Package.PackageFingerprint, runtimeFingerprint,
		candidate.Package.SBOMFingerprint, candidate.Package.Name, candidate.Package.Version,
		candidate.Package.Description, candidate.Package.License, candidate.Package.Compatibility,
		string(allowedTools), string(capabilities), candidate.Package.HasRuntime,
		candidate.Package.FileCount, candidate.Package.PackageBytes, candidate.Package.ExpandedBytes,
		candidate.Package.PackageObjectKey, candidate.Package.SBOMObjectKey, candidate.CreatedAt)
	if err != nil {
		return Candidate{}, fmt.Errorf("insert Skill package version: %w", err)
	}
	if count, countErr := result.RowsAffected(); countErr != nil {
		return Candidate{}, fmt.Errorf("read Skill package insert result: %w", countErr)
	} else if count == 0 {
		match, compareErr := repository.packageMatches(ctx, tx, candidate.Package)
		if compareErr != nil {
			return Candidate{}, compareErr
		}
		if !match {
			return Candidate{}, ErrPackageCollision
		}
	}
	result, err = tx.ExecContext(ctx, `
INSERT INTO skill_package_candidates (
  id, source_type, source_ref, source_artifact_sha256, source_object_key,
  package_fingerprint, status, admission_eligible, validation_summary,
  revision, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, 'validated', $7, $8, 1, $9, $9)
ON CONFLICT (source_type, source_ref) DO NOTHING
`, candidate.ID, candidate.SourceType, candidate.SourceRef,
		candidate.SourceArtifactSHA256, candidate.SourceObjectKey,
		candidate.Package.PackageFingerprint, candidate.AdmissionEligible,
		candidate.ValidationSummary, candidate.CreatedAt)
	if err != nil {
		return Candidate{}, fmt.Errorf("insert Skill candidate: %w", err)
	}
	if count, countErr := result.RowsAffected(); countErr != nil {
		return Candidate{}, fmt.Errorf("read Skill candidate insert result: %w", countErr)
	} else if count == 0 {
		var existingID, sourceHash, packageFingerprint string
		readErr := tx.QueryRowContext(ctx, `
SELECT id, source_artifact_sha256, package_fingerprint
FROM skill_package_candidates
WHERE source_type = $1 AND source_ref = $2
`, candidate.SourceType, candidate.SourceRef).Scan(&existingID, &sourceHash, &packageFingerprint)
		if readErr != nil {
			return Candidate{}, fmt.Errorf("read existing Skill source: %w", readErr)
		}
		if sourceHash != candidate.SourceArtifactSHA256 {
			return Candidate{}, ErrSourceDrift
		}
		if packageFingerprint != candidate.Package.PackageFingerprint {
			return Candidate{}, ErrPackageCollision
		}
		candidate.ID = existingID
	}
	if err := tx.Commit(); err != nil {
		return Candidate{}, fmt.Errorf("commit Skill candidate: %w", err)
	}
	return repository.GetCandidate(ctx, candidate.ID)
}

func (repository *PostgresRepository) packageMatches(
	ctx context.Context,
	tx *sql.Tx,
	version PackageVersion,
) (bool, error) {
	var runtime sql.NullString
	var sbom, name, semanticVersion, description, license, compatibility, packageKey, sbomKey string
	var allowedTools, capabilities []byte
	var hasRuntime bool
	var fileCount int
	var packageBytes, expandedBytes int64
	err := tx.QueryRowContext(ctx, `
SELECT runtime_bundle_fingerprint, sbom_fingerprint, name, version, description,
       license, compatibility, allowed_tools, capability_requests, has_runtime,
       file_count, package_bytes, expanded_bytes, package_object_key, sbom_object_key
FROM skill_package_versions WHERE package_fingerprint = $1
`, version.PackageFingerprint).Scan(&runtime, &sbom, &name, &semanticVersion, &description,
		&license, &compatibility, &allowedTools, &capabilities, &hasRuntime,
		&fileCount, &packageBytes, &expandedBytes, &packageKey, &sbomKey)
	if err != nil {
		return false, fmt.Errorf("read existing Skill package: %w", err)
	}
	var storedAllowedTools []string
	if err := json.Unmarshal(allowedTools, &storedAllowedTools); err != nil {
		return false, fmt.Errorf("decode existing Skill allowed Tools: %w", err)
	}
	var storedCapabilities []CapabilityRequest
	if err := json.Unmarshal(capabilities, &storedCapabilities); err != nil {
		return false, fmt.Errorf("decode existing Skill capability requests: %w", err)
	}
	return runtime.String == version.RuntimeBundleFingerprint && sbom == version.SBOMFingerprint &&
		name == version.Name && semanticVersion == version.Version && description == version.Description &&
		license == version.License && compatibility == version.Compatibility &&
		reflect.DeepEqual(nonNilStrings(storedAllowedTools), nonNilStrings(version.AllowedTools)) &&
		reflect.DeepEqual(nonNilCapabilities(storedCapabilities), nonNilCapabilities(version.CapabilityRequests)) &&
		hasRuntime == version.HasRuntime &&
		fileCount == version.FileCount && packageBytes == version.PackageBytes &&
		expandedBytes == version.ExpandedBytes && packageKey == version.PackageObjectKey &&
		sbomKey == version.SBOMObjectKey, nil
}

func (repository *PostgresRepository) GetCandidate(ctx context.Context, candidateID string) (Candidate, error) {
	if err := repository.requireDB(); err != nil {
		return Candidate{}, err
	}
	candidate, err := scanCandidate(repository.db.QueryRowContext(ctx, candidateSelect+`
WHERE candidate.id = $1
`, candidateID))
	if errors.Is(err, sql.ErrNoRows) {
		return Candidate{}, ErrCandidateNotFound
	}
	if err != nil {
		return Candidate{}, fmt.Errorf("get Skill candidate: %w", err)
	}
	return candidate, nil
}

func (repository *PostgresRepository) GetCandidateBySource(
	ctx context.Context,
	sourceType, sourceRef string,
) (Candidate, error) {
	if err := repository.requireDB(); err != nil {
		return Candidate{}, err
	}
	candidate, err := scanCandidate(repository.db.QueryRowContext(ctx, candidateSelect+`
WHERE candidate.source_type = $1 AND candidate.source_ref = $2
`, sourceType, sourceRef))
	if errors.Is(err, sql.ErrNoRows) {
		return Candidate{}, ErrCandidateNotFound
	}
	if err != nil {
		return Candidate{}, fmt.Errorf("get Skill candidate by source: %w", err)
	}
	return candidate, nil
}

func (repository *PostgresRepository) ReviewCandidate(
	ctx context.Context,
	candidateID, reviewerID string,
	input ReviewInput,
) (Candidate, error) {
	if err := repository.requireDB(); err != nil {
		return Candidate{}, err
	}
	candidate, err := scanCandidate(repository.db.QueryRowContext(ctx, `
WITH candidate AS (
UPDATE skill_package_candidates
SET status = $2, reviewed_by_user_id = $3, review_reason = $4,
    revision = revision + 1, updated_at = now()
WHERE id = $1 AND revision = $5 AND package_fingerprint = $6
  AND ($2 <> 'admitted' OR admission_eligible)
RETURNING *
)
SELECT `+candidateProjection+`
FROM candidate
JOIN skill_package_versions package
  ON package.package_fingerprint = candidate.package_fingerprint
`, candidateID, input.Status, reviewerID, input.Reason, input.ExpectedRevision,
		input.PackageFingerprint))
	if errors.Is(err, sql.ErrNoRows) {
		if _, findErr := repository.GetCandidate(ctx, candidateID); findErr != nil {
			return Candidate{}, findErr
		}
		return Candidate{}, ErrRevisionConflict
	}
	if err != nil {
		return Candidate{}, fmt.Errorf("review Skill candidate: %w", err)
	}
	return candidate, nil
}

func (repository *PostgresRepository) ListStore(
	ctx context.Context,
	page, pageSize int,
) (StoreResult, error) {
	if err := repository.requireDB(); err != nil {
		return StoreResult{}, err
	}
	var total int
	if err := repository.db.QueryRowContext(ctx, `
SELECT COUNT(*)::int FROM skill_package_candidates WHERE status = 'admitted'
`).Scan(&total); err != nil {
		return StoreResult{}, fmt.Errorf("count Skill Store: %w", err)
	}
	rows, err := repository.db.QueryContext(ctx, candidateSelect+`
WHERE candidate.status = 'admitted'
ORDER BY candidate.updated_at DESC, candidate.id DESC
LIMIT $1 OFFSET $2
`, pageSize, (page-1)*pageSize)
	if err != nil {
		return StoreResult{}, fmt.Errorf("list Skill Store: %w", err)
	}
	defer rows.Close()
	items := []Candidate{}
	for rows.Next() {
		candidate, scanErr := scanCandidate(rows)
		if scanErr != nil {
			return StoreResult{}, fmt.Errorf("scan Skill Store: %w", scanErr)
		}
		items = append(items, candidate)
	}
	if err := rows.Err(); err != nil {
		return StoreResult{}, fmt.Errorf("iterate Skill Store: %w", err)
	}
	return StoreResult{Items: items, Page: page, PageSize: pageSize,
		TotalCount: total, TotalPages: pageCount(total, pageSize)}, nil
}

func (repository *PostgresRepository) GetStoreItem(ctx context.Context, admissionID string) (Candidate, error) {
	if err := repository.requireDB(); err != nil {
		return Candidate{}, err
	}
	candidate, err := scanCandidate(repository.db.QueryRowContext(ctx, candidateSelect+`
WHERE candidate.id = $1 AND candidate.status = 'admitted'
`, admissionID))
	if errors.Is(err, sql.ErrNoRows) {
		return Candidate{}, ErrAdmissionDenied
	}
	if err != nil {
		return Candidate{}, fmt.Errorf("get Skill Store item: %w", err)
	}
	return candidate, nil
}

func (repository *PostgresRepository) Install(
	ctx context.Context,
	userID, admissionID, packageFingerprint string,
) (Installation, error) {
	if err := repository.requireDB(); err != nil {
		return Installation{}, err
	}
	installation, err := scanInstallation(repository.db.QueryRowContext(ctx, `
INSERT INTO skill_installations (
  id, user_id, admission_id, package_fingerprint, skill_name
)
SELECT $1, $2, candidate.id, candidate.package_fingerprint, package.name
FROM skill_package_candidates candidate
JOIN skill_package_versions package
  ON package.package_fingerprint = candidate.package_fingerprint
WHERE candidate.id = $3 AND candidate.status = 'admitted'
  AND candidate.package_fingerprint = $4
RETURNING id, user_id, admission_id, package_fingerprint, skill_name,
  revision, created_at, updated_at
`, repository.newID(), userID, admissionID, packageFingerprint))
	if errors.Is(err, sql.ErrNoRows) {
		return Installation{}, ErrAdmissionDenied
	}
	if err != nil {
		if isUniqueViolation(err) {
			return Installation{}, ErrInstallationConflict
		}
		return Installation{}, fmt.Errorf("install Skill: %w", err)
	}
	return repository.hydrateInstallation(ctx, installation)
}

func (repository *PostgresRepository) ListLibrary(
	ctx context.Context,
	userID string,
) ([]Installation, error) {
	if err := repository.requireDB(); err != nil {
		return nil, err
	}
	rows, err := repository.db.QueryContext(ctx, installationSelect+`
WHERE installation.user_id = $1
ORDER BY installation.updated_at DESC, installation.id DESC
`, userID)
	if err != nil {
		return nil, fmt.Errorf("list Skill library: %w", err)
	}
	defer rows.Close()
	installations := []Installation{}
	for rows.Next() {
		installation, scanErr := scanInstallationView(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan Skill library: %w", scanErr)
		}
		installations = append(installations, installation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Skill library: %w", err)
	}
	return installations, nil
}

func (repository *PostgresRepository) Uninstall(
	ctx context.Context,
	userID, installationID string,
	expectedRevision int64,
) error {
	if err := repository.requireDB(); err != nil {
		return err
	}
	result, err := repository.db.ExecContext(ctx, `
DELETE FROM skill_installations
WHERE id = $1 AND user_id = $2 AND revision = $3
`, installationID, userID, expectedRevision)
	if err != nil {
		return fmt.Errorf("uninstall Skill: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read Skill uninstall result: %w", err)
	}
	if count > 0 {
		return nil
	}
	var revision int64
	err = repository.db.QueryRowContext(ctx, `
SELECT revision FROM skill_installations WHERE id = $1 AND user_id = $2
`, installationID, userID).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInstallationNotFound
	}
	if err != nil {
		return fmt.Errorf("classify Skill uninstall: %w", err)
	}
	return ErrRevisionConflict
}

func (repository *PostgresRepository) hydrateInstallation(
	ctx context.Context,
	installation Installation,
) (Installation, error) {
	result, err := scanInstallationView(repository.db.QueryRowContext(ctx, installationSelect+`
WHERE installation.id = $1 AND installation.user_id = $2
`, installation.ID, installation.UserID))
	if err != nil {
		return Installation{}, fmt.Errorf("hydrate Skill installation: %w", err)
	}
	return result, nil
}

func (repository *PostgresRepository) requireDB() error {
	if repository == nil || repository.db == nil {
		return ErrUnavailable
	}
	return nil
}

const candidateProjection = `candidate.id, candidate.source_type, candidate.source_ref,
  candidate.source_artifact_sha256, candidate.source_object_key,
  candidate.package_fingerprint, candidate.status, candidate.admission_eligible,
  candidate.validation_summary, COALESCE(candidate.reviewed_by_user_id::text, ''),
  candidate.review_reason, candidate.revision, candidate.created_at,
  candidate.updated_at, package.runtime_bundle_fingerprint,
  package.sbom_fingerprint, package.name, package.version, package.description,
  package.license, package.compatibility, package.allowed_tools,
  package.capability_requests, package.has_runtime, package.file_count,
  package.package_bytes, package.expanded_bytes, package.package_object_key,
  package.sbom_object_key, package.created_at`

const candidateSelect = `
SELECT ` + candidateProjection + `
FROM skill_package_candidates candidate
JOIN skill_package_versions package
  ON package.package_fingerprint = candidate.package_fingerprint
`

const installationSelect = `
SELECT installation.id, installation.user_id, installation.admission_id,
  installation.package_fingerprint, installation.skill_name,
  package.version, package.description, package.allowed_tools,
  installation.revision, installation.created_at, installation.updated_at
FROM skill_installations installation
JOIN skill_package_versions package
  ON package.package_fingerprint = installation.package_fingerprint
`

type scanner interface{ Scan(...any) error }

func scanCandidate(row scanner) (Candidate, error) {
	var candidate Candidate
	var runtimeFingerprint sql.NullString
	var allowedTools, capabilities []byte
	err := row.Scan(&candidate.ID, &candidate.SourceType, &candidate.SourceRef,
		&candidate.SourceArtifactSHA256, &candidate.SourceObjectKey,
		&candidate.Package.PackageFingerprint, &candidate.Status,
		&candidate.AdmissionEligible, &candidate.ValidationSummary,
		&candidate.ReviewedByUserID, &candidate.ReviewReason, &candidate.Revision,
		&candidate.CreatedAt, &candidate.UpdatedAt, &runtimeFingerprint,
		&candidate.Package.SBOMFingerprint, &candidate.Package.Name,
		&candidate.Package.Version, &candidate.Package.Description,
		&candidate.Package.License, &candidate.Package.Compatibility,
		&allowedTools, &capabilities, &candidate.Package.HasRuntime,
		&candidate.Package.FileCount, &candidate.Package.PackageBytes,
		&candidate.Package.ExpandedBytes, &candidate.Package.PackageObjectKey,
		&candidate.Package.SBOMObjectKey, &candidate.Package.CreatedAt)
	if err != nil {
		return Candidate{}, err
	}
	candidate.Package.RuntimeBundleFingerprint = runtimeFingerprint.String
	if err := json.Unmarshal(allowedTools, &candidate.Package.AllowedTools); err != nil {
		return Candidate{}, err
	}
	if err := json.Unmarshal(capabilities, &candidate.Package.CapabilityRequests); err != nil {
		return Candidate{}, err
	}
	candidate.Package.AllowedTools = nonNilStrings(candidate.Package.AllowedTools)
	candidate.Package.CapabilityRequests = nonNilCapabilities(candidate.Package.CapabilityRequests)
	return candidate, nil
}

func scanInstallation(row scanner) (Installation, error) {
	var installation Installation
	err := row.Scan(&installation.ID, &installation.UserID, &installation.AdmissionID,
		&installation.PackageFingerprint, &installation.Name, &installation.Revision,
		&installation.CreatedAt, &installation.UpdatedAt)
	return installation, err
}

func scanInstallationView(row scanner) (Installation, error) {
	var installation Installation
	var allowedTools []byte
	err := row.Scan(&installation.ID, &installation.UserID, &installation.AdmissionID,
		&installation.PackageFingerprint, &installation.Name, &installation.Version,
		&installation.Description, &allowedTools, &installation.Revision,
		&installation.CreatedAt, &installation.UpdatedAt)
	if err != nil {
		return Installation{}, err
	}
	if err := json.Unmarshal(allowedTools, &installation.AllowedTools); err != nil {
		return Installation{}, err
	}
	installation.AllowedTools = nonNilStrings(installation.AllowedTools)
	return installation, nil
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func nonNilCapabilities(values []CapabilityRequest) []CapabilityRequest {
	if values == nil {
		return []CapabilityRequest{}
	}
	return values
}

func isUniqueViolation(err error) bool {
	var pgError *pgconn.PgError
	return errors.As(err, &pgError) && pgError.Code == "23505"
}

var _ Repository = (*PostgresRepository)(nil)
