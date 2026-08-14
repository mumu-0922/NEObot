package agentbroker

import (
	"context"
	"database/sql"
)

// PostgresProjectMutationRepository is the function-only G21.3 CAS/status/
// cleanup boundary. It never performs direct canary-table DML.
type PostgresProjectMutationRepository struct{ db *sql.DB }

func NewPostgresProjectMutationRepository(db *sql.DB) *PostgresProjectMutationRepository {
	return &PostgresProjectMutationRepository{db: db}
}

func (repository *PostgresProjectMutationRepository) CommitProjectMutation(ctx context.Context,
	authority ProjectMutationAuthority,
) (ProjectMutationReceipt, error) {
	if repository == nil || repository.db == nil || validateProjectMutationAuthority(authority, true) != nil {
		return ProjectMutationReceipt{}, ErrProjectMutationDenied
	}
	var receipt ProjectMutationReceipt
	err := repository.db.QueryRowContext(ctx, `
SELECT 'committed',receipt_fingerprint,committed_revision
FROM agent_project_mutation_commit(
  $1,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15
)`, authority.IntentID, authority.UserID, authority.RunID, authority.AttemptID,
		authority.Generation, authority.SnapshotFingerprint, authority.GrantFingerprint,
		authority.RegistryFingerprint, authority.IdempotencyKey, authority.Resource,
		authority.BaseRevision, authority.Path, authority.Content, authority.ContentFingerprint,
		authority.MutationFingerprint).Scan(&receipt.Outcome, &receipt.ReceiptFingerprint, &receipt.CommittedRevision)
	if err != nil {
		return ProjectMutationReceipt{}, mapPostgresError("commit agent Project mutation", err)
	}
	return receipt, nil
}

func (repository *PostgresProjectMutationRepository) ProjectMutationStatus(ctx context.Context,
	authority ProjectMutationAuthority,
) (ProjectMutationReceipt, error) {
	if repository == nil || repository.db == nil || validateProjectMutationAuthority(authority, true) != nil {
		return ProjectMutationReceipt{}, ErrProjectMutationDenied
	}
	var receipt ProjectMutationReceipt
	err := repository.db.QueryRowContext(ctx, `
SELECT outcome,receipt_fingerprint,current_revision
FROM agent_project_mutation_status($1,$2::uuid,$3,$4,$5,$6)
`, authority.IntentID, authority.UserID, authority.IdempotencyKey, authority.Resource,
		authority.BaseRevision, authority.MutationFingerprint).Scan(&receipt.Outcome,
		&receipt.ReceiptFingerprint, &receipt.CommittedRevision)
	if err != nil {
		return ProjectMutationReceipt{}, mapPostgresError("query agent Project mutation status", err)
	}
	return receipt, nil
}

func (repository *PostgresProjectMutationRepository) CleanupProjectMutation(ctx context.Context,
	authority ProjectMutationAuthority, receiptFingerprint string,
) (string, error) {
	if repository == nil || repository.db == nil || validateProjectMutationAuthority(authority, true) != nil ||
		!validFingerprint(receiptFingerprint) {
		return "", ErrProjectMutationDenied
	}
	var revision string
	err := repository.db.QueryRowContext(ctx, `
SELECT restored_revision FROM agent_project_mutation_cleanup($1,$2::uuid,$3,$4,$5)
`, authority.IntentID, authority.UserID, authority.IdempotencyKey, authority.Resource,
		receiptFingerprint).Scan(&revision)
	if err != nil {
		return "", mapPostgresError("cleanup agent Project mutation", err)
	}
	return revision, nil
}

func (repository *PostgresProjectMutationRepository) PendingProjectMutationCleanup(ctx context.Context,
	userID, runID, resource string,
) (ProjectMutationCleanupCandidate, bool, error) {
	if repository == nil || repository.db == nil || !uuidPattern.MatchString(userID) ||
		!validID(runID, "run") || !validProjectResource(resource) {
		return ProjectMutationCleanupCandidate{}, false, ErrProjectMutationDenied
	}
	var candidate ProjectMutationCleanupCandidate
	err := repository.db.QueryRowContext(ctx, `
SELECT receipt.intent_id,receipt.idempotency_key,receipt.attempt_id,receipt.generation,
       intent.snapshot_fingerprint,intent.grant_fingerprint,intent.registry_fingerprint,
       receipt.receipt_fingerprint
FROM agent_project_mutation_receipts receipt
JOIN agent_effect_intents intent ON intent.id=receipt.intent_id
WHERE receipt.user_id=$1::uuid AND receipt.run_id=$2 AND receipt.resource_id=$3
  AND receipt.cleaned_at IS NULL AND intent.state='committed'
`, userID, runID, resource).Scan(&candidate.IntentID, &candidate.IdempotencyKey,
		&candidate.AttemptID, &candidate.Generation, &candidate.SnapshotFingerprint,
		&candidate.GrantFingerprint, &candidate.RegistryFingerprint, &candidate.ReceiptFingerprint)
	if err == sql.ErrNoRows {
		return ProjectMutationCleanupCandidate{}, false, nil
	}
	if err != nil {
		return ProjectMutationCleanupCandidate{}, false, mapPostgresError("find pending agent Project mutation cleanup", err)
	}
	return candidate, true, nil
}
