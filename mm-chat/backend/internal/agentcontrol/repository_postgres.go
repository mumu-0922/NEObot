package agentcontrol

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"neo-chat/mm-chat/backend/internal/agentlearning"
)

type PostgresRepository struct{ db *sql.DB }

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (repository *PostgresRepository) requireDB() error {
	if repository == nil || repository.db == nil {
		return ErrDatabaseRequired
	}
	return nil
}

func (repository *PostgresRepository) ListRuns(
	ctx context.Context,
	userID string,
	limit int,
) ([]RunSummary, error) {
	if err := repository.requireDB(); err != nil {
		return nil, err
	}
	rows, err := repository.db.QueryContext(ctx, `
SELECT id,state,snapshot_fingerprint,request_fingerprint,created_at,updated_at,terminal_at,
       COALESCE(cancellation_state,''),COALESCE(cancellation_mode,''),COALESCE(cancellation_reason,'')
FROM agent_product_runs WHERE user_id=$1::uuid
ORDER BY created_at DESC,id DESC LIMIT $2
`, userID, limit)
	if err != nil {
		return nil, mapPostgresError("list Agent product Runs", err)
	}
	defer rows.Close()
	result := make([]RunSummary, 0)
	for rows.Next() {
		item, scanErr := scanRunSummary(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan Agent product Run: %w", scanErr)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *PostgresRepository) GetRun(
	ctx context.Context,
	userID, runID string,
) (RunDetail, error) {
	if err := repository.requireDB(); err != nil {
		return RunDetail{}, err
	}
	result := RunDetail{
		Steps: []RunStep{}, Attempts: []RunAttempt{}, Events: []RunEvent{},
		Approvals: []Approval{}, Children: []ChildRun{}, Artifacts: []Artifact{},
	}
	run, err := scanRunSummary(repository.db.QueryRowContext(ctx, `
SELECT id,state,snapshot_fingerprint,request_fingerprint,created_at,updated_at,terminal_at,
       COALESCE(cancellation_state,''),COALESCE(cancellation_mode,''),COALESCE(cancellation_reason,'')
FROM agent_product_runs WHERE user_id=$1::uuid AND id=$2
`, userID, runID))
	if errors.Is(err, sql.ErrNoRows) {
		return RunDetail{}, ErrNotFound
	}
	if err != nil {
		return RunDetail{}, mapPostgresError("get Agent product Run", err)
	}
	result.Run = run

	steps, err := repository.db.QueryContext(ctx, `
SELECT id,ordinal,kind,state,current_generation,created_at,updated_at,terminal_at
FROM agent_product_steps WHERE user_id=$1::uuid AND run_id=$2 ORDER BY ordinal
`, userID, runID)
	if err != nil {
		return RunDetail{}, mapPostgresError("list Agent product Steps", err)
	}
	for steps.Next() {
		var item RunStep
		if err := steps.Scan(&item.ID, &item.Ordinal, &item.Kind, &item.State,
			&item.CurrentGeneration, &item.CreatedAt, &item.UpdatedAt, &item.TerminalAt); err != nil {
			_ = steps.Close()
			return RunDetail{}, fmt.Errorf("scan Agent product Step: %w", err)
		}
		result.Steps = append(result.Steps, item)
	}
	if err := steps.Close(); err != nil {
		return RunDetail{}, err
	}

	attempts, err := repository.db.QueryContext(ctx, `
SELECT id,step_id,generation,state,lease_expires_at,created_at,updated_at,terminal_at
FROM agent_product_attempts WHERE user_id=$1::uuid AND run_id=$2 ORDER BY step_id,generation
`, userID, runID)
	if err != nil {
		return RunDetail{}, mapPostgresError("list Agent product Attempts", err)
	}
	for attempts.Next() {
		var item RunAttempt
		if err := attempts.Scan(&item.ID, &item.StepID, &item.Generation, &item.State,
			&item.LeaseExpiresAt, &item.CreatedAt, &item.UpdatedAt, &item.TerminalAt); err != nil {
			_ = attempts.Close()
			return RunDetail{}, fmt.Errorf("scan Agent product Attempt: %w", err)
		}
		result.Attempts = append(result.Attempts, item)
	}
	if err := attempts.Close(); err != nil {
		return RunDetail{}, err
	}

	events, err := repository.db.QueryContext(ctx, `
SELECT id,COALESCE(step_id,''),COALESCE(attempt_id,''),sequence,occurred_at,kind,entity,
       COALESCE(from_state,''),to_state,actor_type,reason_code,COALESCE(generation,0),lease_expires_at
FROM agent_product_events WHERE user_id=$1::uuid AND run_id=$2 ORDER BY sequence
`, userID, runID)
	if err != nil {
		return RunDetail{}, mapPostgresError("list Agent product events", err)
	}
	for events.Next() {
		var item RunEvent
		if err := events.Scan(&item.ID, &item.StepID, &item.AttemptID, &item.Sequence,
			&item.OccurredAt, &item.Kind, &item.Entity, &item.From, &item.To,
			&item.ActorType, &item.ReasonCode, &item.Generation, &item.LeaseExpiresAt); err != nil {
			_ = events.Close()
			return RunDetail{}, fmt.Errorf("scan Agent product event: %w", err)
		}
		result.Events = append(result.Events, item)
	}
	if err := events.Close(); err != nil {
		return RunDetail{}, err
	}

	approvals, err := repository.db.QueryContext(ctx, `
SELECT intent_id,intent_fingerprint,tool_identity,capability,action,arguments_fingerprint,
       approval_class,approval_revision,state,expires_at,approved_at,terminal_at,COALESCE(error_code,'')
FROM agent_product_approvals WHERE user_id=$1::uuid AND run_id=$2 ORDER BY created_at,intent_id
`, userID, runID)
	if err != nil {
		return RunDetail{}, mapPostgresError("list Agent product approvals", err)
	}
	for approvals.Next() {
		var item Approval
		if err := approvals.Scan(&item.IntentID, &item.IntentFingerprint,
			&item.ToolIdentity, &item.Capability, &item.Action, &item.ArgumentsFingerprint,
			&item.ApprovalClass, &item.ApprovalRevision, &item.State, &item.ExpiresAt,
			&item.ApprovedAt, &item.TerminalAt, &item.ErrorCode); err != nil {
			_ = approvals.Close()
			return RunDetail{}, fmt.Errorf("scan Agent product approval: %w", err)
		}
		result.Approvals = append(result.Approvals, item)
	}
	if err := approvals.Close(); err != nil {
		return RunDetail{}, err
	}

	children, err := repository.db.QueryContext(ctx, `
SELECT run_id,COALESCE(parent_run_id,''),depth,state,package_fingerprint,
       runtime_bundle_fingerprint,grant_fingerprint,registry_fingerprint,expires_at,created_at
FROM agent_product_children WHERE user_id=$1::uuid AND (run_id=$2 OR parent_run_id=$2)
ORDER BY depth,created_at,run_id
`, userID, runID)
	if err != nil {
		return RunDetail{}, mapPostgresError("list Agent product children", err)
	}
	for children.Next() {
		var item ChildRun
		if err := children.Scan(&item.RunID, &item.ParentRunID, &item.Depth, &item.State,
			&item.PackageFingerprint, &item.RuntimeBundleFingerprint, &item.GrantFingerprint,
			&item.RegistryFingerprint, &item.ExpiresAt, &item.CreatedAt); err != nil {
			_ = children.Close()
			return RunDetail{}, fmt.Errorf("scan Agent product Child: %w", err)
		}
		result.Children = append(result.Children, item)
	}
	if err := children.Close(); err != nil {
		return RunDetail{}, err
	}

	artifacts, err := repository.db.QueryContext(ctx, `
SELECT id,attempt_id,generation,name,media_type,size_bytes,fingerprint,created_at
FROM agent_product_artifacts WHERE user_id=$1::uuid AND run_id=$2 ORDER BY created_at,id
`, userID, runID)
	if err != nil {
		return RunDetail{}, mapPostgresError("list Agent product Artifacts", err)
	}
	for artifacts.Next() {
		var item Artifact
		if err := artifacts.Scan(&item.ID, &item.AttemptID, &item.Generation,
			&item.Name, &item.MediaType, &item.Size, &item.Fingerprint, &item.CreatedAt); err != nil {
			_ = artifacts.Close()
			return RunDetail{}, fmt.Errorf("scan Agent product Artifact: %w", err)
		}
		item.DownloadURL = "/v1/agent-center/runs/" + runID + "/artifacts/" + item.ID + "/content"
		result.Artifacts = append(result.Artifacts, item)
	}
	if err := artifacts.Close(); err != nil {
		return RunDetail{}, err
	}
	return result, nil
}

func (repository *PostgresRepository) GetArtifact(
	ctx context.Context,
	userID, runID, artifactID string,
) (ArtifactSource, error) {
	if err := repository.requireDB(); err != nil {
		return ArtifactSource{}, err
	}
	var source ArtifactSource
	err := repository.db.QueryRowContext(ctx, `
SELECT id,attempt_id,generation,name,media_type,size_bytes,fingerprint,created_at,object_key
FROM agent_product_get_artifact($1::uuid,$2,$3)
`, userID, runID, artifactID).Scan(&source.ID, &source.AttemptID,
		&source.Generation, &source.Name, &source.MediaType, &source.Size,
		&source.Fingerprint, &source.CreatedAt, &source.ObjectKey)
	if errors.Is(err, sql.ErrNoRows) {
		return ArtifactSource{}, ErrNotFound
	}
	if err != nil {
		return ArtifactSource{}, mapPostgresError("get Agent artifact", err)
	}
	source.DownloadURL = "/v1/agent-center/runs/" + runID + "/artifacts/" + artifactID + "/content"
	return source, nil
}

func scanRunSummary(row interface{ Scan(...any) error }) (RunSummary, error) {
	var result RunSummary
	err := row.Scan(&result.ID, &result.State, &result.SnapshotFingerprint,
		&result.RequestFingerprint, &result.CreatedAt, &result.UpdatedAt,
		&result.TerminalAt, &result.CancellationState, &result.CancellationMode,
		&result.CancellationReason)
	return result, err
}

func (repository *PostgresRepository) CancelRun(
	ctx context.Context,
	input CancelRunInput,
) (RunCancellation, error) {
	if err := repository.requireDB(); err != nil {
		return RunCancellation{}, err
	}
	var result RunCancellation
	err := repository.db.QueryRowContext(ctx, `
SELECT cancellation_id,run_id,mode,state,expected_run_state,snapshot_fingerprint,reason_code,created_at
FROM agent_product_cancel_run($1,$2::uuid,$3,$4,$5,$6,$7)
`, input.CancellationID, input.UserID, input.RunID, input.ExpectedState,
		input.SnapshotFingerprint, input.Mode, input.ReasonCode,
	).Scan(&result.ID, &result.RunID, &result.Mode, &result.State,
		&result.ExpectedRunState, &result.SnapshotFingerprint, &result.ReasonCode,
		&result.CreatedAt)
	if err != nil {
		return RunCancellation{}, mapPostgresError("cancel Agent product Run", err)
	}
	return result, nil
}

func (repository *PostgresRepository) ListSchedules(
	ctx context.Context,
	userID string,
	limit int,
) ([]ScheduleSummary, error) {
	if err := repository.requireDB(); err != nil {
		return nil, err
	}
	rows, err := repository.db.QueryContext(ctx, `
SELECT id,state,current_revision,revision_fingerprint,next_trigger_at,schedule_expression,
       timezone,calculator,automation_class,approval_id,package_fingerprint,
       runtime_bundle_fingerprint,missed_policy,overlap_policy,created_at,updated_at
FROM agent_product_schedules WHERE user_id=$1::uuid
ORDER BY updated_at DESC,id DESC LIMIT $2
`, userID, limit)
	if err != nil {
		return nil, mapPostgresError("list Agent product schedules", err)
	}
	defer rows.Close()
	result := make([]ScheduleSummary, 0)
	for rows.Next() {
		var item ScheduleSummary
		if err := rows.Scan(&item.ID, &item.State, &item.CurrentRevision,
			&item.RevisionFingerprint, &item.NextTriggerAt, &item.ScheduleExpression,
			&item.Timezone, &item.Calculator, &item.AutomationClass, &item.ApprovalID,
			&item.PackageFingerprint, &item.RuntimeBundleFingerprint, &item.MissedPolicy,
			&item.OverlapPolicy, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan Agent product schedule: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *PostgresRepository) ListDrafts(
	ctx context.Context,
	limit int,
) ([]DraftSummary, error) {
	if err := repository.requireDB(); err != nil {
		return nil, err
	}
	rows, err := repository.db.QueryContext(ctx, `
SELECT id,user_id::text,state,revision,draft_fingerprint,base_package_fingerprint,
       proposed_package_fingerprint,runtime_bundle_fingerprint,name,version,
       check_generation,check_attempts,COALESCE(admission_id::text,''),created_at,updated_at,object_deleted_at
FROM agent_product_drafts ORDER BY updated_at DESC,id DESC LIMIT $1
`, limit)
	if err != nil {
		return nil, mapPostgresError("list Agent product Drafts", err)
	}
	defer rows.Close()
	result := make([]DraftSummary, 0)
	for rows.Next() {
		item, scanErr := scanDraftSummary(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan Agent product Draft: %w", scanErr)
		}
		item.Checks = []agentlearning.CheckReceipt{}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *PostgresRepository) GetDraft(
	ctx context.Context,
	draftID string,
) (DraftSummary, error) {
	if err := repository.requireDB(); err != nil {
		return DraftSummary{}, err
	}
	result, err := scanDraftSummary(repository.db.QueryRowContext(ctx, `
SELECT id,user_id::text,state,revision,draft_fingerprint,base_package_fingerprint,
       proposed_package_fingerprint,runtime_bundle_fingerprint,name,version,
       check_generation,check_attempts,COALESCE(admission_id::text,''),created_at,updated_at,object_deleted_at
FROM agent_product_drafts WHERE id=$1
`, draftID))
	if errors.Is(err, sql.ErrNoRows) {
		return DraftSummary{}, ErrNotFound
	}
	if err != nil {
		return DraftSummary{}, mapPostgresError("get Agent product Draft", err)
	}
	result.Checks = []agentlearning.CheckReceipt{}
	rows, err := repository.db.QueryContext(ctx, `
SELECT id,kind,status,reason_code,suite_fingerprint,evidence_fingerprint,duration_millis,metrics
FROM agent_product_draft_checks WHERE draft_id=$1 AND generation=$2 ORDER BY kind
`, draftID, result.CheckGeneration)
	if err != nil {
		return DraftSummary{}, mapPostgresError("list Agent product Draft checks", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item agentlearning.CheckReceipt
		var metrics []byte
		if err := rows.Scan(&item.ID, &item.Kind, &item.Status, &item.ReasonCode,
			&item.SuiteFingerprint, &item.EvidenceFingerprint, &item.DurationMillis,
			&metrics); err != nil {
			return DraftSummary{}, fmt.Errorf("scan Agent product Draft check: %w", err)
		}
		if err := json.Unmarshal(metrics, &item.Metrics); err != nil {
			return DraftSummary{}, fmt.Errorf("decode Agent product Draft check metrics: %w", err)
		}
		result.Checks = append(result.Checks, item)
	}
	return result, rows.Err()
}

func scanDraftSummary(row interface{ Scan(...any) error }) (DraftSummary, error) {
	var result DraftSummary
	err := row.Scan(&result.ID, &result.OwnerUserID, &result.State, &result.Revision,
		&result.DraftFingerprint, &result.BasePackageFingerprint,
		&result.ProposedPackageFingerprint, &result.RuntimeBundleFingerprint,
		&result.Name, &result.Version, &result.CheckGeneration, &result.CheckAttempts,
		&result.AdmissionID, &result.CreatedAt, &result.UpdatedAt, &result.ObjectDeletedAt)
	return result, err
}

func (repository *PostgresRepository) GetShadow(
	ctx context.Context,
	userID string,
) (ShadowSnapshot, error) {
	if err := repository.requireDB(); err != nil {
		return ShadowSnapshot{}, err
	}
	var result ShadowSnapshot
	var startsAt, expiresAt sql.NullTime
	err := repository.db.QueryRowContext(ctx, `
SELECT policy_revision,enabled,mode,COALESCE(admission_id::text,''),package_fingerprint,
       runtime_bundle_fingerprint,cohort_basis_points,max_observations,max_errors,
       starts_at,expires_at,policy_updated_at,opted_in,opt_generation,opt_policy_revision,
	       opt_updated_at,cohort_selected,eligible,effective,held_reason_code,observation_count,error_count
FROM agent_product_shadow_snapshot($1::uuid)
`, userID).Scan(&result.Policy.Revision, &result.Policy.Enabled, &result.Policy.Mode,
		&result.Policy.AdmissionID, &result.Policy.PackageFingerprint,
		&result.Policy.RuntimeBundleFingerprint, &result.Policy.CohortBasisPoints,
		&result.Policy.MaxObservations, &result.Policy.MaxErrors, &startsAt, &expiresAt,
		&result.Policy.UpdatedAt, &result.OptIn.OptedIn, &result.OptIn.Generation,
		&result.OptIn.PolicyRevision, &result.OptIn.UpdatedAt, &result.CohortSelected,
		&result.Eligible, &result.Effective, &result.HeldReasonCode, &result.ObservationCount,
		&result.ErrorCount)
	if err != nil {
		return ShadowSnapshot{}, mapPostgresError("get Agent Shadow state", err)
	}
	if startsAt.Valid {
		value := startsAt.Time.UTC()
		result.Policy.StartsAt = &value
	}
	if expiresAt.Valid {
		value := expiresAt.Time.UTC()
		result.Policy.ExpiresAt = &value
	}
	return result, nil
}

func (repository *PostgresRepository) UpdateShadowPolicy(
	ctx context.Context,
	input UpdateShadowPolicyInput,
) (ShadowPolicy, error) {
	if err := repository.requireDB(); err != nil {
		return ShadowPolicy{}, err
	}
	var result ShadowPolicy
	var startsAt, expiresAt sql.NullTime
	err := repository.db.QueryRowContext(ctx, `
SELECT revision,enabled,mode,COALESCE(admission_id::text,''),package_fingerprint,
       runtime_bundle_fingerprint,cohort_basis_points,max_observations,max_errors,
       starts_at,expires_at,updated_at
FROM agent_product_update_shadow_policy(
  $1::uuid,$2,$3,$4,NULLIF($5,'')::uuid,$6,$7,$8,$9,$10,$11,$12,$13
)
`, input.AdministratorID, input.ExpectedRevision, input.Enabled, input.Mode,
		input.AdmissionID, input.PackageFingerprint, input.RuntimeBundleFingerprint,
		input.CohortBasisPoints, input.MaxObservations, input.MaxErrors, input.StartsAt,
		input.ExpiresAt, input.ReasonCode,
	).Scan(&result.Revision, &result.Enabled, &result.Mode, &result.AdmissionID,
		&result.PackageFingerprint, &result.RuntimeBundleFingerprint,
		&result.CohortBasisPoints, &result.MaxObservations, &result.MaxErrors,
		&startsAt, &expiresAt, &result.UpdatedAt)
	if err != nil {
		return ShadowPolicy{}, mapPostgresError("update Agent Shadow policy", err)
	}
	if startsAt.Valid {
		value := startsAt.Time.UTC()
		result.StartsAt = &value
	}
	if expiresAt.Valid {
		value := expiresAt.Time.UTC()
		result.ExpiresAt = &value
	}
	return result, nil
}

func (repository *PostgresRepository) SetShadowOptIn(
	ctx context.Context,
	input SetShadowOptInInput,
) (ShadowOptIn, error) {
	if err := repository.requireDB(); err != nil {
		return ShadowOptIn{}, err
	}
	var result ShadowOptIn
	err := repository.db.QueryRowContext(ctx, `
SELECT opted_in,generation,policy_revision,updated_at
FROM agent_product_set_shadow_opt_in($1::uuid,$2,$3,$4,$5)
`, input.UserID, input.ExpectedGeneration, input.PolicyRevision,
		input.OptedIn, input.ReasonCode,
	).Scan(&result.OptedIn, &result.Generation, &result.PolicyRevision,
		&result.UpdatedAt)
	if err != nil {
		return ShadowOptIn{}, mapPostgresError("set Agent Shadow opt-in", err)
	}
	return result, nil
}

func (repository *PostgresRepository) RegisterShadowBoot(
	ctx context.Context,
	bootID string,
) (int64, error) {
	if err := repository.requireDB(); err != nil {
		return 0, err
	}
	var epoch int64
	if err := repository.db.QueryRowContext(ctx,
		`SELECT agent_product_register_shadow_boot($1)`, bootID).Scan(&epoch); err != nil {
		return 0, mapPostgresError("register Agent Shadow boot", err)
	}
	return epoch, nil
}

func (repository *PostgresRepository) AppendShadowObservation(
	ctx context.Context,
	bootID string,
	request ShadowRequest,
	measurement ShadowMeasurement,
) (ShadowObservation, error) {
	if err := repository.requireDB(); err != nil {
		return ShadowObservation{}, err
	}
	var result ShadowObservation
	err := repository.db.QueryRowContext(ctx, `
SELECT observation_id,policy_revision,generation,admission_id::text,package_fingerprint,
       runtime_bundle_fingerprint,mode,outcome,reason_code,latency_bucket,
       run_count,step_count,attempt_count,error_count,created_at
FROM agent_product_append_shadow_observation(
  $1,$2::uuid,$3,$4,$5::uuid,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15
)
`, bootID, request.UserID, request.PolicyRevision, request.Generation,
		request.AdmissionID, request.PackageFingerprint, request.RuntimeBundleFingerprint,
		request.Mode, measurement.Outcome, measurement.ReasonCode,
		measurement.LatencyBucket, measurement.RunCount, measurement.StepCount,
		measurement.AttemptCount, measurement.ErrorCount,
	).Scan(&result.ID, &result.PolicyRevision, &result.Generation,
		&result.AdmissionID, &result.PackageFingerprint,
		&result.RuntimeBundleFingerprint, &result.Mode, &result.Outcome,
		&result.ReasonCode, &result.LatencyBucket, &result.RunCount,
		&result.StepCount, &result.AttemptCount, &result.ErrorCount,
		&result.CreatedAt)
	if err != nil {
		return ShadowObservation{}, mapPostgresError("append Agent Shadow observation", err)
	}
	return result, nil
}

func mapPostgresError(operation string, err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		message := strings.ToUpper(postgresError.Message)
		switch {
		case strings.Contains(message, "REVISION_CONFLICT"):
			return ErrRevisionConflict
		case strings.Contains(message, "GENERATION_STALE"):
			return ErrGenerationStale
		case strings.Contains(message, "FINGERPRINT_DRIFT"):
			return ErrFingerprintDrift
		case strings.Contains(message, "BUDGET_EXCEEDED"):
			return ErrBudgetExceeded
		case strings.Contains(message, "KILL_SWITCH_ACTIVE"):
			return ErrKillSwitchActive
		case strings.Contains(message, "RUN_CANCEL_BLOCKED"):
			return ErrRunCancelBlocked
		case strings.Contains(message, "NOT_FOUND"):
			return ErrNotFound
		}
		if postgresError.Code == "22023" || postgresError.Code == "23514" {
			return fmt.Errorf("%w: %s", ErrInvalidInput, postgresError.Message)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

var _ Repository = (*PostgresRepository)(nil)
