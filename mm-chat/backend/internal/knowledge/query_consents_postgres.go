package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

func (r *PostgresRepository) ListQueryConsents(ctx context.Context, input QueryConsentLookupInput) ([]ProcessingConsent, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin query consent list: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireActiveConsentUser(ctx, tx, input.ActorUserID); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT processor,decision,
CASE WHEN decision='granted' AND expires_at<=clock_timestamp() THEN 'expired' ELSE decision END,
array_to_string(purposes,E'\x1f'),
array_to_string(data_types,E'\x1f'),policy_version,decided_at,expires_at
FROM processing_consents WHERE scope='query' AND user_id=$1 AND superseded_at IS NULL ORDER BY processor`, input.ActorUserID)
	if err != nil {
		return nil, fmt.Errorf("list query consents: %w", err)
	}
	result := make([]ProcessingConsent, 0)
	for rows.Next() {
		var value ProcessingConsent
		var purposes, dataTypes string
		var expires sql.NullTime
		if err := rows.Scan(&value.Processor, &value.Decision, &value.EffectiveStatus, &purposes, &dataTypes,
			&value.PolicyVersion, &value.DecidedAt, &expires); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan query consent: %w", err)
		}
		value.Purposes, value.DataTypes = splitSQLList(purposes), splitSQLList(dataTypes)
		value.DecidedAt = value.DecidedAt.UTC()
		if expires.Valid {
			expiry := expires.Time.UTC()
			value.ExpiresAt = &expiry
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate query consents: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close query consents: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit query consent list: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) PutQueryConsent(ctx context.Context, input PutQueryConsentRepositoryInput) (ProcessingConsent, error) {
	if err := r.requireDB(); err != nil {
		return ProcessingConsent{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ProcessingConsent{}, fmt.Errorf("begin query consent grant: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireActiveConsentUser(ctx, tx, input.ActorUserID); err != nil {
		return ProcessingConsent{}, err
	}
	authority, err := lockUniqueConsentAuthority(ctx, tx, input.Processor)
	if err != nil {
		return ProcessingConsent{}, err
	}
	if !isStringSubset(input.Purposes, authority.AllowedPurposes) || !isDataTypeSubset(input.DataTypes, authority.AllowedDataTypes) {
		return ProcessingConsent{}, ErrKnowledgeProcessorUnavailable
	}
	current, found, err := lockCurrentQueryConsent(ctx, tx, input.ActorUserID, input.Processor)
	if err != nil {
		return ProcessingConsent{}, err
	}
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return ProcessingConsent{}, fmt.Errorf("read query consent decision time: %w", err)
	}
	now = now.UTC()
	if found {
		if _, err := r.materializeLockedQueryExpiry(ctx, tx, input.ActorUserID, input.Processor, current, now); err != nil {
			return ProcessingConsent{}, err
		}
	}
	matchesCurrent := found && current.Decision == "granted" && current.EndpointID == authority.EndpointID &&
		current.ProfileID == authority.ProfileID && current.GovernanceRevision == authority.GovernanceRevision &&
		current.HeadRevision == authority.HeadRevision && slices.Equal(current.Purposes, input.Purposes) &&
		slices.Equal(current.DataTypes, input.DataTypes) && current.PolicyVersion == input.PolicyVersion &&
		nullTimeEqual(current.ExpiresAt, input.ExpiresAt)
	if matchesCurrent {
		value := consentFromRow(input.Processor, current)
		if current.ExpiresAt.Valid && !current.ExpiresAt.Time.After(now) {
			value.EffectiveStatus = "expired"
			if err := tx.Commit(); err != nil {
				return ProcessingConsent{}, fmt.Errorf("commit elapsed query consent replay: %w", err)
			}
		}
		return value, nil
	}
	if input.ExpiresAt != nil && !input.ExpiresAt.After(now) {
		return ProcessingConsent{}, invalidConsentPayload("expiry must be in the future")
	}
	consentRevision := int64(1)
	if found {
		consentRevision = current.ConsentRevision + 1
		if _, err := tx.ExecContext(ctx, `UPDATE processing_consents SET superseded_at=$2,updated_at=$2 WHERE id=$1`, current.ID, now); err != nil {
			return ProcessingConsent{}, fmt.Errorf("supersede query consent: %w", err)
		}
	}
	queryRevision, err := advanceQueryConsentRevision(ctx, tx, input.ActorUserID, now)
	if err != nil {
		return ProcessingConsent{}, err
	}
	consentID, err := r.newEventID()
	if err != nil {
		return ProcessingConsent{}, fmt.Errorf("generate query consent id: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO processing_consents (
id,scope,user_id,processor,endpoint_id,governance_profile_id,governance_revision,
governance_head_revision,purposes,data_types,policy_version,decision,consent_revision,
granted_by_user_id,decided_at,expires_at,created_at,updated_at
) VALUES ($1,'query',$2,$3,$4,$5,$6,$7,$8,$9,$10,'granted',$11,$2,$12,$13,$12,$12)`,
		consentID, input.ActorUserID, input.Processor, authority.EndpointID, authority.ProfileID,
		authority.GovernanceRevision, authority.HeadRevision, input.Purposes, input.DataTypes,
		input.PolicyVersion, consentRevision, now, input.ExpiresAt)
	if err != nil {
		return ProcessingConsent{}, fmt.Errorf("insert query consent: %w", err)
	}
	value := ProcessingConsent{Processor: input.Processor, Decision: "granted", EffectiveStatus: "granted", Purposes: input.Purposes,
		DataTypes: input.DataTypes, PolicyVersion: input.PolicyVersion, DecidedAt: now, ExpiresAt: input.ExpiresAt}
	if err := r.insertQueryConsentEvent(ctx, tx, input.ActorUserID, value, consentRevision, queryRevision, authority); err != nil {
		return ProcessingConsent{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProcessingConsent{}, fmt.Errorf("commit query consent grant: %w", err)
	}
	return value, nil
}

func (r *PostgresRepository) RevokeQueryConsent(ctx context.Context, input QueryConsentLookupInput) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin query consent revoke: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireActiveConsentUser(ctx, tx, input.ActorUserID); err != nil {
		return err
	}
	current, found, err := lockCurrentQueryConsent(ctx, tx, input.ActorUserID, input.Processor)
	if err != nil {
		return err
	}
	if !found || current.Decision == "revoked" {
		return nil
	}
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return fmt.Errorf("read query consent revocation time: %w", err)
	}
	now = now.UTC()
	if _, err := r.materializeLockedQueryExpiry(ctx, tx, input.ActorUserID, input.Processor, current, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE processing_consents SET superseded_at=$2,updated_at=$2 WHERE id=$1`, current.ID, now); err != nil {
		return fmt.Errorf("supersede granted query consent: %w", err)
	}
	queryRevision, err := advanceQueryConsentRevision(ctx, tx, input.ActorUserID, now)
	if err != nil {
		return err
	}
	consentID, err := r.newEventID()
	if err != nil {
		return fmt.Errorf("generate revoked query consent id: %w", err)
	}
	consentRevision := current.ConsentRevision + 1
	_, err = tx.ExecContext(ctx, `INSERT INTO processing_consents (
id,scope,user_id,processor,endpoint_id,governance_profile_id,governance_revision,
governance_head_revision,purposes,data_types,policy_version,decision,consent_revision,
granted_by_user_id,decided_at,created_at,updated_at
) VALUES ($1,'query',$2,$3,$4,$5,$6,$7,$8,$9,$10,'revoked',$11,$2,$12,$12,$12)`, consentID,
		input.ActorUserID, input.Processor, current.EndpointID, current.ProfileID,
		current.GovernanceRevision, current.HeadRevision, current.Purposes, current.DataTypes,
		current.PolicyVersion, consentRevision, now)
	if err != nil {
		return fmt.Errorf("insert revoked query consent: %w", err)
	}
	authority := consentAuthority{EndpointID: current.EndpointID, ProfileID: current.ProfileID,
		GovernanceRevision: current.GovernanceRevision, HeadRevision: current.HeadRevision}
	value := ProcessingConsent{Processor: input.Processor, Decision: "revoked", EffectiveStatus: "revoked", Purposes: current.Purposes,
		DataTypes: current.DataTypes, PolicyVersion: current.PolicyVersion, DecidedAt: now}
	if err := r.insertQueryConsentEvent(ctx, tx, input.ActorUserID, value, consentRevision, queryRevision, authority); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit query consent revoke: %w", err)
	}
	return nil
}

func requireActiveConsentUser(ctx context.Context, tx *sql.Tx, userID string) error {
	if err := lockActiveUser(ctx, tx, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUnauthenticated
		}
		return fmt.Errorf("lock query consent user: %w", err)
	}
	return nil
}

func lockCurrentQueryConsent(ctx context.Context, tx *sql.Tx, userID, processor string) (currentConsentRow, bool, error) {
	var row currentConsentRow
	var purposes, dataTypes string
	err := tx.QueryRowContext(ctx, `SELECT id,endpoint_id,governance_profile_id,governance_revision,
governance_head_revision,decision,array_to_string(purposes,E'\x1f'),array_to_string(data_types,E'\x1f'),
policy_version,consent_revision,decided_at,expires_at,expiry_materialized_at FROM processing_consents
WHERE scope='query' AND user_id=$1 AND processor=$2 AND superseded_at IS NULL FOR UPDATE`, userID, processor).Scan(
		&row.ID, &row.EndpointID, &row.ProfileID, &row.GovernanceRevision, &row.HeadRevision,
		&row.Decision, &purposes, &dataTypes, &row.PolicyVersion, &row.ConsentRevision,
		&row.DecidedAt, &row.ExpiresAt, &row.ExpiryMaterializedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return row, false, nil
	}
	if err != nil {
		return row, false, fmt.Errorf("lock current query consent: %w", err)
	}
	row.Purposes, row.DataTypes = splitSQLList(purposes), splitSQLList(dataTypes)
	return row, true, nil
}

func advanceQueryConsentRevision(ctx context.Context, tx *sql.Tx, userID string, now time.Time) (int64, error) {
	var revision int64
	err := tx.QueryRowContext(ctx, `INSERT INTO user_query_consent_state(user_id) VALUES ($1)
ON CONFLICT (user_id) DO UPDATE SET query_consent_revision=user_query_consent_state.query_consent_revision+1,
updated_at=$2 RETURNING query_consent_revision`, userID, now).Scan(&revision)
	if err != nil {
		return 0, fmt.Errorf("advance query consent revision: %w", err)
	}
	if revision == 1 {
		if err := tx.QueryRowContext(ctx, `UPDATE user_query_consent_state SET query_consent_revision=2,updated_at=$2 WHERE user_id=$1 RETURNING query_consent_revision`, userID, now).Scan(&revision); err != nil {
			return 0, fmt.Errorf("initialize query consent revision: %w", err)
		}
	}
	return revision, nil
}

func (r *PostgresRepository) insertQueryConsentEvent(ctx context.Context, tx *sql.Tx, userID string,
	value ProcessingConsent, consentRevision, queryRevision int64, authority consentAuthority) error {
	eventID, err := r.newEventID()
	if err != nil {
		return fmt.Errorf("generate query consent event id: %w", err)
	}
	payloadObject := map[string]any{"schemaVersion": 1, "scope": "query", "userId": userID,
		"processor": value.Processor, "endpointId": authority.EndpointID, "decision": value.Decision,
		"effectiveStatus": value.EffectiveStatus,
		"consentRevision": consentRevision, "queryConsentRevision": queryRevision,
		"governanceProfileId": authority.ProfileID, "governanceRevision": authority.GovernanceRevision,
		"governanceHeadRevision": authority.HeadRevision}
	if value.ExpiresAt != nil {
		key := "expiresAt"
		if value.MaterializedAt != nil {
			key = "expiredAt"
		}
		payloadObject[key] = value.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	if value.MaterializedAt != nil {
		payloadObject["materializedAt"] = value.MaterializedAt.UTC().Format(time.RFC3339Nano)
		payloadObject["reason"] = "expired"
	}
	payload, err := json.Marshal(payloadObject)
	if err != nil {
		return fmt.Errorf("marshal query consent event: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO knowledge_outbox(event_id,aggregate_type,aggregate_key,event_type,payload)
VALUES ($1,'knowledge_user',$2,'knowledge.user.query-consent.changed',$3::jsonb)`, eventID, userID, string(payload)); err != nil {
		return fmt.Errorf("insert query consent event: %w", err)
	}
	return nil
}
