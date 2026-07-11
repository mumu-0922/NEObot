package knowledge

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	defaultConsentExpiryBatch = 100
	defaultConsentExpiryPoll  = time.Second
)

type ConsentExpiryWorker struct {
	repo         *PostgresRepository
	batchSize    int
	pollInterval time.Duration
}

func NewConsentExpiryWorker(repo *PostgresRepository, batchSize int, pollInterval time.Duration) (*ConsentExpiryWorker, error) {
	if repo == nil || repo.db == nil {
		return nil, ErrDatabaseRequired
	}
	if batchSize <= 0 {
		batchSize = defaultConsentExpiryBatch
	}
	if batchSize > 1000 {
		return nil, fmt.Errorf("consent expiry batch size exceeds 1000")
	}
	if pollInterval <= 0 {
		pollInterval = defaultConsentExpiryPoll
	}
	return &ConsentExpiryWorker{repo: repo, batchSize: batchSize, pollInterval: pollInterval}, nil
}

func (worker *ConsentExpiryWorker) Run(ctx context.Context) error {
	if worker == nil || worker.repo == nil {
		return ErrDatabaseRequired
	}
	for {
		processed, err := worker.RunOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if processed > 0 {
			continue
		}
		timer := time.NewTimer(worker.pollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (worker *ConsentExpiryWorker) RunOnce(ctx context.Context) (int, error) {
	if worker == nil || worker.repo == nil || worker.repo.db == nil {
		return 0, ErrDatabaseRequired
	}
	rows, err := worker.repo.db.QueryContext(ctx, `SELECT id FROM processing_consents
WHERE superseded_at IS NULL AND decision='granted' AND expires_at IS NOT NULL
  AND expires_at<=clock_timestamp() AND expiry_materialized_at IS NULL
ORDER BY expires_at,id LIMIT $1`, worker.batchSize)
	if err != nil {
		return 0, fmt.Errorf("list due consent expiries: %w", err)
	}
	ids := make([]string, 0, worker.batchSize)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scan due consent expiry: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, fmt.Errorf("iterate due consent expiries: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close due consent expiries: %w", err)
	}
	processed := 0
	for _, id := range ids {
		changed, err := worker.repo.MaterializeConsentExpiry(ctx, id)
		if err != nil {
			return processed, err
		}
		if changed {
			processed++
		}
	}
	return processed, nil
}

func (r *PostgresRepository) MaterializeConsentExpiry(ctx context.Context, consentID string) (bool, error) {
	if err := r.requireDB(); err != nil {
		return false, err
	}
	var scope, processor string
	var collectionID, userID sql.NullString
	if err := r.db.QueryRowContext(ctx, `SELECT scope,collection_id,user_id,processor FROM processing_consents WHERE id=$1`, consentID).Scan(&scope, &collectionID, &userID, &processor); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("resolve consent expiry subject: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin consent expiry: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var collection collectionRow
	if scope == "collection" {
		collection, err = lockSystemConsentCollection(ctx, tx, collectionID.String)
	} else {
		err = lockSystemConsentUser(ctx, tx, userID.String)
	}
	if err != nil {
		return false, err
	}
	current, found, err := lockDueConsentByID(ctx, tx, consentID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return false, fmt.Errorf("read consent expiry time: %w", err)
	}
	now = now.UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE processing_consents SET expiry_materialized_at=$2,updated_at=$2 WHERE id=$1`, consentID, now); err != nil {
		return false, fmt.Errorf("mark consent expiry: %w", err)
	}
	expiry := current.ExpiresAt.Time.UTC()
	value := ProcessingConsent{Processor: processor, Decision: "granted", EffectiveStatus: "expired",
		Purposes: current.Purposes, DataTypes: current.DataTypes, PolicyVersion: current.PolicyVersion,
		DecidedAt: current.DecidedAt.UTC(), ExpiresAt: &expiry, MaterializedAt: &now}
	authority := consentAuthority{EndpointID: current.EndpointID, ProfileID: current.ProfileID,
		GovernanceRevision: current.GovernanceRevision, HeadRevision: current.HeadRevision}
	if scope == "collection" {
		revision := collection.ProcessingRevision + 1
		if _, err := tx.ExecContext(ctx, `UPDATE knowledge_collections SET collection_processing_revision=$2,updated_at=$3 WHERE id=$1`, collectionID.String, revision, now); err != nil {
			return false, fmt.Errorf("advance expired collection revision: %w", err)
		}
		if err := r.insertCollectionConsentEvent(ctx, tx, collectionID.String, value, current.ConsentRevision, revision, authority); err != nil {
			return false, err
		}
	} else {
		revision, err := advanceQueryConsentRevision(ctx, tx, userID.String, now)
		if err != nil {
			return false, err
		}
		if err := r.insertQueryConsentEvent(ctx, tx, userID.String, value, current.ConsentRevision, revision, authority); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit consent expiry: %w", err)
	}
	return true, nil
}

func lockDueConsentByID(ctx context.Context, tx *sql.Tx, consentID string) (currentConsentRow, bool, error) {
	var row currentConsentRow
	var purposes, dataTypes string
	err := tx.QueryRowContext(ctx, `SELECT id,endpoint_id,governance_profile_id,governance_revision,
governance_head_revision,decision,array_to_string(purposes,E'\x1f'),array_to_string(data_types,E'\x1f'),
policy_version,consent_revision,decided_at,expires_at,expiry_materialized_at
FROM processing_consents WHERE id=$1 AND superseded_at IS NULL AND decision='granted'
AND expires_at IS NOT NULL AND expires_at<=clock_timestamp() AND expiry_materialized_at IS NULL FOR UPDATE`, consentID).Scan(
		&row.ID, &row.EndpointID, &row.ProfileID, &row.GovernanceRevision, &row.HeadRevision, &row.Decision,
		&purposes, &dataTypes, &row.PolicyVersion, &row.ConsentRevision, &row.DecidedAt,
		&row.ExpiresAt, &row.ExpiryMaterializedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return row, false, nil
	}
	if err != nil {
		return row, false, fmt.Errorf("lock due consent expiry: %w", err)
	}
	row.Purposes, row.DataTypes = splitSQLList(purposes), splitSQLList(dataTypes)
	return row, true, nil
}

func lockSystemConsentCollection(ctx context.Context, tx *sql.Tx, collectionID string) (collectionRow, error) {
	var scope string
	var teamID sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT scope,team_id FROM knowledge_collections WHERE id=$1`, collectionID).Scan(&scope, &teamID); err != nil {
		return collectionRow{}, fmt.Errorf("resolve expiry collection: %w", err)
	}
	if scope == ScopeTeam {
		var id string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM teams WHERE id=$1 FOR UPDATE`, teamID.String).Scan(&id); err != nil {
			return collectionRow{}, fmt.Errorf("lock expiry team: %w", err)
		}
	}
	var row collectionRow
	if err := tx.QueryRowContext(ctx, `SELECT id,name,description,icon,color,scope,owner_user_id,team_id,
acl_revision,visibility_epoch,collection_processing_revision,create_request_hash,created_at,updated_at,deleted_at
FROM knowledge_collections WHERE id=$1 FOR UPDATE`, collectionID).Scan(row.scanTargets()...); err != nil {
		return collectionRow{}, fmt.Errorf("lock expiry collection: %w", err)
	}
	return row, nil
}

func lockSystemConsentUser(ctx context.Context, tx *sql.Tx, userID string) error {
	var id string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&id); err != nil {
		return fmt.Errorf("lock expiry user: %w", err)
	}
	return nil
}

func (r *PostgresRepository) materializeLockedCollectionExpiry(
	ctx context.Context, tx *sql.Tx, collectionID, processor string,
	current currentConsentRow, processingRevision int64, now time.Time,
) (int64, error) {
	if !consentExpiryDue(current, now) {
		return processingRevision, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE processing_consents SET expiry_materialized_at=$2,updated_at=$2 WHERE id=$1 AND expiry_materialized_at IS NULL`, current.ID, now); err != nil {
		return processingRevision, fmt.Errorf("materialize current collection expiry: %w", err)
	}
	processingRevision++
	if _, err := tx.ExecContext(ctx, `UPDATE knowledge_collections SET collection_processing_revision=$2,updated_at=$3 WHERE id=$1`, collectionID, processingRevision, now); err != nil {
		return processingRevision, fmt.Errorf("advance current collection expiry revision: %w", err)
	}
	value, authority := expiredConsentEventValues(processor, current, now)
	if err := r.insertCollectionConsentEvent(ctx, tx, collectionID, value, current.ConsentRevision, processingRevision, authority); err != nil {
		return processingRevision, err
	}
	return processingRevision, nil
}

func (r *PostgresRepository) materializeLockedQueryExpiry(
	ctx context.Context, tx *sql.Tx, userID, processor string,
	current currentConsentRow, now time.Time,
) (int64, error) {
	if !consentExpiryDue(current, now) {
		return 0, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE processing_consents SET expiry_materialized_at=$2,updated_at=$2 WHERE id=$1 AND expiry_materialized_at IS NULL`, current.ID, now); err != nil {
		return 0, fmt.Errorf("materialize current query expiry: %w", err)
	}
	revision, err := advanceQueryConsentRevision(ctx, tx, userID, now)
	if err != nil {
		return 0, err
	}
	value, authority := expiredConsentEventValues(processor, current, now)
	if err := r.insertQueryConsentEvent(ctx, tx, userID, value, current.ConsentRevision, revision, authority); err != nil {
		return 0, err
	}
	return revision, nil
}

func consentExpiryDue(current currentConsentRow, now time.Time) bool {
	return current.Decision == "granted" && current.ExpiresAt.Valid &&
		!current.ExpiresAt.Time.After(now) && !current.ExpiryMaterializedAt.Valid
}

func expiredConsentEventValues(processor string, current currentConsentRow, now time.Time) (ProcessingConsent, consentAuthority) {
	expiry := current.ExpiresAt.Time.UTC()
	value := ProcessingConsent{Processor: processor, Decision: "granted", EffectiveStatus: "expired",
		Purposes: current.Purposes, DataTypes: current.DataTypes, PolicyVersion: current.PolicyVersion,
		DecidedAt: current.DecidedAt.UTC(), ExpiresAt: &expiry, MaterializedAt: &now}
	authority := consentAuthority{EndpointID: current.EndpointID, ProfileID: current.ProfileID,
		GovernanceRevision: current.GovernanceRevision, HeadRevision: current.HeadRevision}
	return value, authority
}
