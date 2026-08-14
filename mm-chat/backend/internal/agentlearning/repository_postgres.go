package agentlearning

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct {
	db    *sql.DB
	newID func(string) string
}

func NewPostgresRepository(database *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: database, newID: func(prefix string) string {
		return prefix + "_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}}
}

func (repository *PostgresRepository) GetSourceRun(
	ctx context.Context,
	userID, runID, baseFingerprint string,
) (SourceRun, error) {
	if err := repository.requireDB(); err != nil {
		return SourceRun{}, err
	}
	var result SourceRun
	err := repository.db.QueryRowContext(ctx, `
SELECT run.id,run.user_id::text,run.snapshot_id,run.snapshot_fingerprint,run.state,
  COALESCE((snapshot.canonical_snapshot->>'depth')::integer,0)
FROM agent_runs run JOIN agent_run_snapshots snapshot ON snapshot.id=run.snapshot_id
  AND snapshot.user_id=run.user_id AND snapshot.fingerprint=run.snapshot_fingerprint
WHERE run.id=$1 AND run.user_id=$2::uuid AND run.state='succeeded'
  AND snapshot.canonical_snapshot->>'packageFingerprint'=$3
`, runID, userID, baseFingerprint).Scan(&result.RunID, &result.UserID, &result.SnapshotID,
		&result.SnapshotFingerprint, &result.State, &result.Depth)
	if errors.Is(err, sql.ErrNoRows) {
		return SourceRun{}, ErrSourceInvalid
	}
	if err != nil {
		return SourceRun{}, fmt.Errorf("read Agent learning source Run: %w", err)
	}
	return result, nil
}

func (repository *PostgresRepository) GetBasePackage(
	ctx context.Context,
	fingerprint string,
) (BasePackage, error) {
	if err := repository.requireDB(); err != nil {
		return BasePackage{}, err
	}
	var result BasePackage
	var runtime sql.NullString
	var allowed, capabilities []byte
	err := repository.db.QueryRowContext(ctx, `
SELECT package_fingerprint,runtime_bundle_fingerprint,sbom_fingerprint,name,version,description,license,compatibility,
  allowed_tools,capability_requests,has_runtime,file_count,package_bytes,expanded_bytes,package_object_key,sbom_object_key,created_at
FROM skill_package_versions WHERE package_fingerprint=$1
`, fingerprint).Scan(&result.Package.PackageFingerprint, &runtime,
		&result.Package.SBOMFingerprint, &result.Package.Name, &result.Package.Version,
		&result.Package.Description, &result.Package.License, &result.Package.Compatibility,
		&allowed, &capabilities, &result.Package.HasRuntime, &result.Package.FileCount,
		&result.Package.PackageBytes, &result.Package.ExpandedBytes,
		&result.Package.PackageObjectKey, &result.Package.SBOMObjectKey, &result.Package.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BasePackage{}, ErrSourceInvalid
	}
	if err != nil {
		return BasePackage{}, fmt.Errorf("read Agent learning base package: %w", err)
	}
	result.Package.RuntimeBundleFingerprint = runtime.String
	if err := json.Unmarshal(allowed, &result.Package.AllowedTools); err != nil {
		return BasePackage{}, fmt.Errorf("decode base allowed Tools: %w", err)
	}
	if err := json.Unmarshal(capabilities, &result.Package.CapabilityRequests); err != nil {
		return BasePackage{}, fmt.Errorf("decode base capability requests: %w", err)
	}
	return result, nil
}

func (repository *PostgresRepository) CreateDraft(
	ctx context.Context,
	prepared preparedDraft,
) (Draft, bool, error) {
	if err := repository.requireDB(); err != nil {
		return Draft{}, false, err
	}
	spec, err := json.Marshal(prepared.Spec)
	if err != nil {
		return Draft{}, false, fmt.Errorf("marshal Agent Draft spec: %w", err)
	}
	var draftID, state string
	var revision int64
	var created bool
	err = repository.db.QueryRowContext(ctx, `
SELECT draft_id,state,revision,created FROM agent_learning_create_draft(
  $1,$2::uuid,$3,$4::jsonb,$5,$6,$7)
`, prepared.ID, prepared.UserID, prepared.DraftFingerprint, string(spec), prepared.DraftObjectKey,
		prepared.CreatedAt, repository.newID("draft_event")).Scan(&draftID, &state, &revision, &created)
	if err != nil {
		return Draft{}, false, repository.mapError("create Agent Draft", err)
	}
	result, err := repository.GetDraft(ctx, prepared.UserID, draftID)
	return result, created, err
}

func (repository *PostgresRepository) GetDraft(
	ctx context.Context,
	userID, draftID string,
) (Draft, error) {
	if err := repository.requireDB(); err != nil {
		return Draft{}, err
	}
	result, err := scanDraft(repository.db.QueryRowContext(ctx, draftSelect+`
WHERE draft.id=$1 AND ($2='' OR draft.user_id::text=$2)
`, draftID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return Draft{}, ErrNotFound
	}
	if err != nil {
		return Draft{}, fmt.Errorf("read Agent Draft: %w", err)
	}
	return result, nil
}

func (repository *PostgresRepository) ClaimChecks(
	ctx context.Context,
	request ClaimRequest,
) ([]CheckClaim, error) {
	if err := repository.requireDB(); err != nil {
		return nil, err
	}
	rows, err := repository.db.QueryContext(ctx, `
SELECT `+draftProjection+`
FROM agent_learning_claim_checks($1,$2,$3,$4) draft
JOIN skill_package_versions base ON base.package_fingerprint=draft.base_package_fingerprint
ORDER BY draft.next_check_at,draft.id
`, request.Owner, request.Now.UTC(), int(request.LeaseDuration/time.Second), request.Limit)
	if err != nil {
		return nil, repository.mapError("claim Agent Draft checks", err)
	}
	defer rows.Close()
	result := make([]CheckClaim, 0)
	for rows.Next() {
		draft, scanErr := scanDraft(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan Agent Draft check claim: %w", scanErr)
		}
		result = append(result, CheckClaim{Draft: draft, ClaimGeneration: draft.CheckGeneration,
			ClaimOwner: draft.CheckOwner, ClaimExpiresAt: *draft.CheckExpiresAt})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Agent Draft check claims: %w", err)
	}
	return result, nil
}

func (repository *PostgresRepository) CompleteChecks(
	ctx context.Context,
	claim CheckClaim,
	receipts []CheckReceipt,
) (Draft, error) {
	if err := repository.requireDB(); err != nil {
		return Draft{}, err
	}
	encoded, err := json.Marshal(receipts)
	if err != nil {
		return Draft{}, fmt.Errorf("marshal Agent Draft check receipts: %w", err)
	}
	if _, err := repository.db.ExecContext(ctx, `
SELECT agent_learning_complete_checks($1,$2,$3,$4::jsonb,$5)
`, claim.ID, claim.ClaimOwner, claim.ClaimGeneration, string(encoded),
		repository.newID("draft_event")); err != nil {
		return Draft{}, repository.mapError("complete Agent Draft checks", err)
	}
	return repository.GetDraft(ctx, claim.UserID, claim.ID)
}

func (repository *PostgresRepository) ReleaseCheck(
	ctx context.Context,
	claim CheckClaim,
	errorCode string,
	retryAt time.Time,
) (bool, error) {
	if err := repository.requireDB(); err != nil {
		return false, err
	}
	var terminal bool
	err := repository.db.QueryRowContext(ctx, `
SELECT agent_learning_release_check($1,$2,$3,$4,$5,$6)
`, claim.ID, claim.ClaimOwner, claim.ClaimGeneration, errorCode, retryAt.UTC(),
		repository.newID("draft_event")).Scan(&terminal)
	if err != nil {
		return false, repository.mapError("release Agent Draft check", err)
	}
	return terminal, nil
}

func (repository *PostgresRepository) Reject(
	ctx context.Context,
	administratorID string,
	input ReviewInput,
	decisionID, reasonCode string,
) (Draft, bool, error) {
	if err := repository.requireDB(); err != nil {
		return Draft{}, false, err
	}
	var draftID, state string
	var revision int64
	var created bool
	err := repository.db.QueryRowContext(ctx, `
SELECT draft_id,state,revision,created FROM agent_learning_reject(
  $1,$2::uuid,$3,$4,$5,$6,$7,$8)
`, input.DraftID, administratorID, input.ExpectedRevision, input.DraftFingerprint,
		input.ProposedPackageFingerprint, decisionID, reasonCode,
		repository.newID("draft_event")).Scan(&draftID, &state, &revision, &created)
	if err != nil {
		return Draft{}, false, repository.mapError("reject Agent Draft", err)
	}
	result, err := repository.GetDraft(ctx, "", draftID)
	return result, created, err
}

func (repository *PostgresRepository) Promote(
	ctx context.Context,
	prepared preparedPromotion,
) (Promotion, error) {
	if err := repository.requireDB(); err != nil {
		return Promotion{}, err
	}
	packageDocument := map[string]any{
		"runtimeBundleFingerprint": prepared.Package.RuntimeBundleFingerprint,
		"sbomFingerprint":          prepared.Package.SBOMFingerprint,
		"name":                     prepared.Package.Name, "version": prepared.Package.Version,
		"description": prepared.Package.Description, "license": prepared.Package.License,
		"compatibility": prepared.Package.Compatibility,
		"hasRuntime":    prepared.Package.HasRuntime, "fileCount": prepared.Package.FileCount,
		"packageBytes": prepared.Package.PackageBytes, "expandedBytes": prepared.Package.ExpandedBytes,
		"packageObjectKey": prepared.Package.PackageObjectKey,
		"sbomObjectKey":    prepared.Package.SBOMObjectKey,
	}
	var allowedTools, capabilityRequests any
	if err := json.Unmarshal(prepared.AllowedToolsJSON, &allowedTools); err != nil {
		return Promotion{}, fmt.Errorf("decode promotion allowed Tools: %w", err)
	}
	if err := json.Unmarshal(prepared.CapabilitiesJSON, &capabilityRequests); err != nil {
		return Promotion{}, fmt.Errorf("decode promotion capability requests: %w", err)
	}
	packageDocument["allowedTools"], packageDocument["capabilityRequests"] = allowedTools, capabilityRequests
	encoded, err := json.Marshal(packageDocument)
	if err != nil {
		return Promotion{}, fmt.Errorf("marshal promotion package: %w", err)
	}
	var result Promotion
	err = repository.db.QueryRowContext(ctx, `
SELECT draft_id,admission_id::text,package_fingerprint,created FROM agent_learning_promote(
  $1,$2::uuid,$3,$4,$5,$6::uuid,$7,$8,$9,$10,$11::jsonb,$12)
`, prepared.DraftID, prepared.AdministratorID, prepared.ExpectedRevision,
		prepared.DraftFingerprint, prepared.ProposedPackageFingerprint, prepared.CandidateID,
		prepared.DecisionID, prepared.ReasonCode, prepared.SourceRef,
		prepared.SourceObjectKey, string(encoded), repository.newID("draft_event")).Scan(
		&result.DraftID, &result.AdmissionID, &result.PackageFingerprint, &result.Created)
	if err != nil {
		return Promotion{}, repository.mapError("promote Agent Draft", err)
	}
	return result, nil
}

func (repository *PostgresRepository) ReplayPromotion(
	ctx context.Context,
	administratorID string,
	input ReviewInput,
) (Promotion, error) {
	if err := repository.requireDB(); err != nil {
		return Promotion{}, err
	}
	var result Promotion
	err := repository.db.QueryRowContext(ctx, `
SELECT draft.id,draft.admission_id::text,draft.promoted_package_fingerprint,false
FROM agent_learning_drafts draft
JOIN agent_learning_decisions decision ON decision.draft_id=draft.id
WHERE draft.id=$1 AND draft.state='promoted' AND decision.decision='promote'
  AND decision.actor_user_id=$2::uuid AND decision.expected_revision=$3
  AND decision.draft_fingerprint=$4 AND decision.proposed_package_fingerprint=$5
  AND decision.reason_code=$6
`, input.DraftID, administratorID, input.ExpectedRevision, input.DraftFingerprint,
		input.ProposedPackageFingerprint, input.ReasonCode).Scan(
		&result.DraftID, &result.AdmissionID, &result.PackageFingerprint, &result.Created)
	if errors.Is(err, sql.ErrNoRows) {
		return Promotion{}, ErrPromotionDenied
	}
	if err != nil {
		return Promotion{}, fmt.Errorf("replay Agent Draft promotion: %w", err)
	}
	return result, nil
}

func (repository *PostgresRepository) ClaimCleanup(
	ctx context.Context,
	request ClaimRequest,
) ([]CleanupClaim, error) {
	if err := repository.requireDB(); err != nil {
		return nil, err
	}
	rows, err := repository.db.QueryContext(ctx, `
SELECT draft_id,object_key,object_fingerprint,generation,claim_owner,claim_expires_at,attempts
FROM agent_learning_claim_cleanup($1,$2,$3,$4) ORDER BY next_attempt_at,draft_id
`, request.Owner, request.Now.UTC(), int(request.LeaseDuration/time.Second), request.Limit)
	if err != nil {
		return nil, repository.mapError("claim Agent Draft cleanup", err)
	}
	defer rows.Close()
	result := make([]CleanupClaim, 0)
	for rows.Next() {
		var claim CleanupClaim
		if err := rows.Scan(&claim.DraftID, &claim.ObjectKey, &claim.ObjectFingerprint,
			&claim.Generation, &claim.Owner, &claim.ExpiresAt, &claim.Attempts); err != nil {
			return nil, fmt.Errorf("scan Agent Draft cleanup claim: %w", err)
		}
		result = append(result, claim)
	}
	return result, rows.Err()
}

func (repository *PostgresRepository) CompleteCleanup(
	ctx context.Context,
	claim CleanupClaim,
) error {
	if err := repository.requireDB(); err != nil {
		return err
	}
	_, err := repository.db.ExecContext(ctx, `
SELECT agent_learning_complete_cleanup($1,$2,$3,$4)
`, claim.DraftID, claim.Owner, claim.Generation, claim.ObjectFingerprint)
	return repository.mapError("complete Agent Draft cleanup", err)
}

func (repository *PostgresRepository) ReleaseCleanup(
	ctx context.Context,
	claim CleanupClaim,
	errorCode string,
	retryAt time.Time,
) (bool, error) {
	if err := repository.requireDB(); err != nil {
		return false, err
	}
	var terminal bool
	err := repository.db.QueryRowContext(ctx, `
SELECT agent_learning_release_cleanup($1,$2,$3,$4,$5)
`, claim.DraftID, claim.Owner, claim.Generation, errorCode, retryAt.UTC()).Scan(&terminal)
	if err != nil {
		return false, repository.mapError("release Agent Draft cleanup", err)
	}
	return terminal, nil
}

func (repository *PostgresRepository) Reconcile(
	ctx context.Context,
	now time.Time,
	limit int,
) (ReconcileResult, error) {
	if err := repository.requireDB(); err != nil {
		return ReconcileResult{}, err
	}
	var result ReconcileResult
	err := repository.db.QueryRowContext(ctx, `
SELECT checks_reclaimed,cleanup_reclaimed FROM agent_learning_reconcile($1,$2)
`, now.UTC(), limit).Scan(&result.ChecksReclaimed, &result.CleanupReclaimed)
	if err != nil {
		return ReconcileResult{}, repository.mapError("reconcile Agent learning", err)
	}
	return result, nil
}

func (repository *PostgresRepository) Prune(
	ctx context.Context,
	cutoff time.Time,
	limit int,
) (PruneResult, error) {
	if err := repository.requireDB(); err != nil {
		return PruneResult{}, err
	}
	var result PruneResult
	err := repository.db.QueryRowContext(ctx, `
SELECT drafts_pruned,audits_pruned FROM agent_learning_prune($1,$2)
`, cutoff.UTC(), limit).Scan(&result.DraftsPruned, &result.AuditsPruned)
	if err != nil {
		return PruneResult{}, repository.mapError("prune Agent learning", err)
	}
	return result, nil
}

func (repository *PostgresRepository) requireDB() error {
	if repository == nil || repository.db == nil {
		return ErrDatabaseRequired
	}
	return nil
}

func (repository *PostgresRepository) mapError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch strings.ToUpper(postgresError.Message) {
		case "DRAFT_NOT_FOUND", "AGENT_LEARNING_NOT_FOUND":
			return ErrNotFound
		case "SOURCE_RUN_INVALID":
			return ErrSourceInvalid
		case "SOURCE_DRIFT":
			return ErrSourceDrift
		case "PACKAGE_ALREADY_EXISTS":
			return ErrPackageExists
		case "STALE_CLAIM":
			return ErrStaleClaim
		case "REVISION_CONFLICT":
			return ErrRevisionConflict
		case "CHECKS_INCOMPLETE":
			return ErrChecksIncomplete
		case "PROMOTION_DENIED":
			return ErrPromotionDenied
		case "KILL_SWITCH_ACTIVE":
			return ErrKillSwitchActive
		}
		if postgresError.Code == "22023" || postgresError.Code == "23514" {
			return fmt.Errorf("%w: %s", ErrInvalidInput, postgresError.Message)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

const draftProjection = `
draft.id,draft.user_id::text,draft.draft_fingerprint,draft.spec,draft.state,draft.revision,draft.check_attempts,
draft.check_generation,COALESCE(draft.check_owner,''),draft.check_expires_at,COALESCE(draft.admission_id::text,''),
COALESCE(draft.promoted_package_fingerprint,''),draft.draft_object_key,base.package_object_key,
draft.created_at,draft.updated_at,draft.object_deleted_at`

const draftSelect = `SELECT ` + draftProjection + `
FROM agent_learning_drafts draft
JOIN skill_package_versions base ON base.package_fingerprint=draft.base_package_fingerprint
`

type rowScanner interface {
	Scan(...any) error
}

func scanDraft(row rowScanner) (Draft, error) {
	var result Draft
	var spec []byte
	var checkExpires, objectDeleted sql.NullTime
	err := row.Scan(&result.ID, &result.UserID, &result.DraftFingerprint, &spec,
		&result.State, &result.Revision, &result.CheckAttempts, &result.CheckGeneration,
		&result.CheckOwner, &checkExpires, &result.AdmissionID, &result.PromotedFingerprint,
		&result.DraftObjectKey, &result.BasePackageObjectKey, &result.CreatedAt,
		&result.UpdatedAt, &objectDeleted)
	if err != nil {
		return Draft{}, err
	}
	if err := json.Unmarshal(spec, &result.Spec); err != nil {
		return Draft{}, fmt.Errorf("decode Agent Draft spec: %w", err)
	}
	if checkExpires.Valid {
		result.CheckExpiresAt = &checkExpires.Time
	}
	if objectDeleted.Valid {
		result.ObjectDeletedAt = &objectDeleted.Time
	}
	return result, nil
}
