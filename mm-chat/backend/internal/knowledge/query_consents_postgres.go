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
	capabilities, err := loadRuntimeSchemaCapabilities(ctx, tx)
	if err != nil {
		return nil, err
	}
	query := `SELECT pc.processor,pc.endpoint_id,''::text,''::text,pc.decision,
CASE WHEN pc.decision='granted' AND pc.expires_at<=clock_timestamp() THEN 'expired' ELSE pc.decision END,
array_to_string(pc.purposes,E'\x1f'),array_to_string(pc.data_types,E'\x1f'),
pc.policy_version,pc.decided_at,pc.expires_at
FROM processing_consents pc WHERE pc.scope='query' AND pc.user_id=$1
AND pc.superseded_at IS NULL ORDER BY pc.processor,pc.endpoint_id`
	if capabilities.exactModelIdentity {
		query = `SELECT pc.processor,pc.endpoint_id,pc.model_id,p.profile_contract_hash,pc.decision,
CASE WHEN pc.decision='granted' AND pc.expires_at<=clock_timestamp() THEN 'expired' ELSE pc.decision END,
array_to_string(pc.purposes,E'\x1f'),array_to_string(pc.data_types,E'\x1f'),
pc.policy_version,pc.decided_at,pc.expires_at
FROM processing_consents pc JOIN processor_governance_profiles p ON p.id=pc.governance_profile_id
WHERE pc.scope='query' AND pc.user_id=$1 AND pc.superseded_at IS NULL
ORDER BY pc.processor,pc.endpoint_id,pc.model_id`
	}
	rows, err := tx.QueryContext(ctx, query, input.ActorUserID)
	if err != nil {
		return nil, fmt.Errorf("list query consents: %w", err)
	}
	result, err := scanConsentRows(rows, "query")
	if err != nil {
		return nil, err
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
	capabilities, err := loadRuntimeSchemaCapabilities(ctx, tx)
	if err != nil {
		return ProcessingConsent{}, err
	}
	authority, err := lockConsentAuthority(ctx, tx, capabilities, ProcessorModelIdentity{
		Processor: input.Processor, EndpointID: input.EndpointID, ModelID: input.ModelID,
	})
	if err != nil {
		return ProcessingConsent{}, err
	}
	if !isStringSubset(input.Purposes, authority.AllowedPurposes) || !isDataTypeSubset(input.DataTypes, authority.AllowedDataTypes) {
		return ProcessingConsent{}, ErrKnowledgeProcessorUnavailable
	}
	current, found, err := lockCurrentQueryConsent(ctx, tx, capabilities, input.ActorUserID,
		ProcessorModelIdentity{Processor: input.Processor, EndpointID: authority.EndpointID, ModelID: authority.ModelID})
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
		current.ModelID == authority.ModelID && current.ProfileID == authority.ProfileID &&
		current.ProfileContractHash == authority.ProfileContractHash &&
		current.GovernanceRevision == authority.GovernanceRevision && current.HeadRevision == authority.HeadRevision &&
		slices.Equal(current.Purposes, input.Purposes) && slices.Equal(current.DataTypes, input.DataTypes) &&
		current.PolicyVersion == input.PolicyVersion && nullTimeEqual(current.ExpiresAt, input.ExpiresAt)
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
	if capabilities.exactModelIdentity {
		_, err = tx.ExecContext(ctx, `INSERT INTO processing_consents (
id,scope,user_id,processor,endpoint_id,model_id,governance_profile_id,governance_revision,
governance_head_revision,purposes,data_types,policy_version,decision,consent_revision,
granted_by_user_id,decided_at,expires_at,created_at,updated_at
) VALUES ($1,'query',$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'granted',$12,$2,$13,$14,$13,$13)`,
			consentID, input.ActorUserID, input.Processor, authority.EndpointID, authority.ModelID,
			authority.ProfileID, authority.GovernanceRevision, authority.HeadRevision, input.Purposes,
			input.DataTypes, input.PolicyVersion, consentRevision, now, input.ExpiresAt)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO processing_consents (
id,scope,user_id,processor,endpoint_id,governance_profile_id,governance_revision,
governance_head_revision,purposes,data_types,policy_version,decision,consent_revision,
granted_by_user_id,decided_at,expires_at,created_at,updated_at
) VALUES ($1,'query',$2,$3,$4,$5,$6,$7,$8,$9,$10,'granted',$11,$2,$12,$13,$12,$12)`,
			consentID, input.ActorUserID, input.Processor, authority.EndpointID, authority.ProfileID,
			authority.GovernanceRevision, authority.HeadRevision, input.Purposes, input.DataTypes,
			input.PolicyVersion, consentRevision, now, input.ExpiresAt)
	}
	if err != nil {
		return ProcessingConsent{}, fmt.Errorf("insert query consent: %w", err)
	}
	value := ProcessingConsent{Processor: input.Processor, EndpointID: authority.EndpointID,
		ModelID: authority.ModelID, ProfileContractHash: authority.ProfileContractHash,
		Decision: "granted", EffectiveStatus: "granted", Purposes: input.Purposes,
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
	capabilities, err := loadRuntimeSchemaCapabilities(ctx, tx)
	if err != nil {
		return err
	}
	current, found, err := lockCurrentQueryConsent(ctx, tx, capabilities, input.ActorUserID,
		ProcessorModelIdentity{Processor: input.Processor, EndpointID: input.EndpointID, ModelID: input.ModelID})
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
	if capabilities.exactModelIdentity {
		_, err = tx.ExecContext(ctx, `INSERT INTO processing_consents (
id,scope,user_id,processor,endpoint_id,model_id,governance_profile_id,governance_revision,
governance_head_revision,purposes,data_types,policy_version,decision,consent_revision,
granted_by_user_id,decided_at,created_at,updated_at
) VALUES ($1,'query',$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'revoked',$12,$2,$13,$13,$13)`,
			consentID, input.ActorUserID, input.Processor, current.EndpointID, current.ModelID,
			current.ProfileID, current.GovernanceRevision, current.HeadRevision, current.Purposes,
			current.DataTypes, current.PolicyVersion, consentRevision, now)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO processing_consents (
id,scope,user_id,processor,endpoint_id,governance_profile_id,governance_revision,
governance_head_revision,purposes,data_types,policy_version,decision,consent_revision,
granted_by_user_id,decided_at,created_at,updated_at
) VALUES ($1,'query',$2,$3,$4,$5,$6,$7,$8,$9,$10,'revoked',$11,$2,$12,$12,$12)`, consentID,
			input.ActorUserID, input.Processor, current.EndpointID, current.ProfileID,
			current.GovernanceRevision, current.HeadRevision, current.Purposes, current.DataTypes,
			current.PolicyVersion, consentRevision, now)
	}
	if err != nil {
		return fmt.Errorf("insert revoked query consent: %w", err)
	}
	authority := authorityFromCurrent(current)
	value := consentFromRow(input.Processor, current)
	value.Decision, value.EffectiveStatus, value.DecidedAt, value.ExpiresAt = "revoked", "revoked", now, nil
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

func lockCurrentQueryConsent(ctx context.Context, tx *sql.Tx, capabilities runtimeSchemaCapabilities,
	userID string, identity ProcessorModelIdentity) (currentConsentRow, bool, error) {
	query := `SELECT pc.id,pc.endpoint_id,''::text,pc.governance_profile_id,''::text,
pc.governance_revision,pc.governance_head_revision,pc.decision,array_to_string(pc.purposes,E'\x1f'),
array_to_string(pc.data_types,E'\x1f'),pc.policy_version,pc.consent_revision,pc.decided_at,
pc.expires_at,pc.expiry_materialized_at FROM processing_consents pc
WHERE pc.scope='query' AND pc.user_id=$1 AND pc.processor=$2 AND pc.superseded_at IS NULL
ORDER BY pc.endpoint_id LIMIT 2 FOR UPDATE OF pc`
	args := []any{userID, identity.Processor}
	if capabilities.exactModelIdentity {
		query = `SELECT pc.id,pc.endpoint_id,pc.model_id,pc.governance_profile_id,p.profile_contract_hash,
pc.governance_revision,pc.governance_head_revision,pc.decision,array_to_string(pc.purposes,E'\x1f'),
array_to_string(pc.data_types,E'\x1f'),pc.policy_version,pc.consent_revision,pc.decided_at,
pc.expires_at,pc.expiry_materialized_at FROM processing_consents pc
JOIN processor_governance_profiles p ON p.id=pc.governance_profile_id
WHERE pc.scope='query' AND pc.user_id=$1 AND pc.processor=$2 AND pc.superseded_at IS NULL`
		if identity.EndpointID != "" {
			query += ` AND pc.endpoint_id=$3 AND pc.model_id=$4`
			args = append(args, identity.EndpointID, identity.ModelID)
		}
		query += ` ORDER BY pc.endpoint_id,pc.model_id LIMIT 2 FOR UPDATE OF pc`
	}
	return lockCurrentConsentRows(ctx, tx, "query", query, args...)
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
		if err := tx.QueryRowContext(ctx, `UPDATE user_query_consent_state SET query_consent_revision=2,updated_at=$2
WHERE user_id=$1 RETURNING query_consent_revision`, userID, now).Scan(&revision); err != nil {
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
		"processor": value.Processor, "endpointId": authority.EndpointID, "modelId": authority.ModelID,
		"profileContractHash": authority.ProfileContractHash, "decision": value.Decision,
		"effectiveStatus": value.EffectiveStatus, "consentRevision": consentRevision,
		"queryConsentRevision": queryRevision, "governanceProfileId": authority.ProfileID,
		"governanceRevision": authority.GovernanceRevision, "governanceHeadRevision": authority.HeadRevision}
	addConsentExpiryEventFields(payloadObject, value)
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
