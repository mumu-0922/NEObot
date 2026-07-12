package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

type consentAuthority struct {
	EndpointID, ModelID, ProfileID, ProfileContractHash string
	GovernanceRevision, HeadRevision                    int64
	AllowedPurposes, AllowedDataTypes                   []string
}

type currentConsentRow struct {
	ID, EndpointID, ModelID, ProfileID, ProfileContractHash string
	Decision, PolicyVersion                                 string
	GovernanceRevision, HeadRevision, ConsentRevision       int64
	Purposes, DataTypes                                     []string
	DecidedAt                                               time.Time
	ExpiresAt                                               sql.NullTime
	ExpiryMaterializedAt                                    sql.NullTime
}

func (r *PostgresRepository) ListCollectionConsents(ctx context.Context, input CollectionConsentLookupInput) ([]ProcessingConsent, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin collection consent list: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockCollectionForConsentRead(ctx, tx, input.CollectionID, input.ActorUserID); err != nil {
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
FROM processing_consents pc WHERE pc.scope='collection' AND pc.collection_id=$1
AND pc.superseded_at IS NULL ORDER BY pc.processor,pc.endpoint_id`
	if capabilities.exactModelIdentity {
		query = `SELECT pc.processor,pc.endpoint_id,pc.model_id,p.profile_contract_hash,pc.decision,
CASE WHEN pc.decision='granted' AND pc.expires_at<=clock_timestamp() THEN 'expired' ELSE pc.decision END,
array_to_string(pc.purposes,E'\x1f'),array_to_string(pc.data_types,E'\x1f'),
pc.policy_version,pc.decided_at,pc.expires_at
FROM processing_consents pc JOIN processor_governance_profiles p ON p.id=pc.governance_profile_id
WHERE pc.scope='collection' AND pc.collection_id=$1 AND pc.superseded_at IS NULL
ORDER BY pc.processor,pc.endpoint_id,pc.model_id`
	}
	rows, err := tx.QueryContext(ctx, query, input.CollectionID)
	if err != nil {
		return nil, fmt.Errorf("list collection consents: %w", err)
	}
	result, err := scanConsentRows(rows, "collection")
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit collection consent list: %w", err)
	}
	return result, nil
}

func scanConsentRows(rows *sql.Rows, scope string) ([]ProcessingConsent, error) {
	defer rows.Close()
	result := make([]ProcessingConsent, 0)
	for rows.Next() {
		var value ProcessingConsent
		var expires sql.NullTime
		var purposes, dataTypes string
		if err := rows.Scan(&value.Processor, &value.EndpointID, &value.ModelID,
			&value.ProfileContractHash, &value.Decision, &value.EffectiveStatus,
			&purposes, &dataTypes, &value.PolicyVersion, &value.DecidedAt, &expires); err != nil {
			return nil, fmt.Errorf("scan %s consent: %w", scope, err)
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
		return nil, fmt.Errorf("iterate %s consents: %w", scope, err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close %s consents: %w", scope, err)
	}
	return result, nil
}

func lockCollectionForConsentRead(ctx context.Context, tx *sql.Tx, collectionID, actorID string) error {
	if err := lockActiveUser(ctx, tx, actorID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCollectionNotFound
		}
		return fmt.Errorf("lock consent actor: %w", err)
	}
	var scope string
	var ownerID, teamID sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT scope,owner_user_id,team_id FROM knowledge_collections WHERE id=$1`, collectionID).Scan(&scope, &ownerID, &teamID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCollectionNotFound
		}
		return fmt.Errorf("resolve consent collection: %w", err)
	}
	if scope == ScopePersonal {
		if !ownerID.Valid || ownerID.String != actorID {
			return ErrCollectionNotFound
		}
	} else if _, err := lockVisibleTeam(ctx, tx, teamID.String, actorID); err != nil {
		return err
	}
	var deletedAt sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT deleted_at FROM knowledge_collections WHERE id=$1 FOR UPDATE`, collectionID).Scan(&deletedAt); err != nil {
		return fmt.Errorf("lock consent collection: %w", err)
	}
	if deletedAt.Valid {
		return ErrCollectionNotFound
	}
	return nil
}

func (r *PostgresRepository) PutCollectionConsent(ctx context.Context, input PutCollectionConsentRepositoryInput) (ProcessingConsent, error) {
	if err := r.requireDB(); err != nil {
		return ProcessingConsent{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ProcessingConsent{}, fmt.Errorf("begin collection consent grant: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	collection, _, err := lockCollectionForManage(ctx, tx, input.CollectionID, input.ActorUserID)
	if err != nil {
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
	current, found, err := lockCurrentCollectionConsent(ctx, tx, capabilities, input.CollectionID,
		ProcessorModelIdentity{Processor: input.Processor, EndpointID: authority.EndpointID, ModelID: authority.ModelID})
	if err != nil {
		return ProcessingConsent{}, err
	}
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return ProcessingConsent{}, fmt.Errorf("read consent decision time: %w", err)
	}
	now = now.UTC()
	processingRevision := collection.ProcessingRevision
	if found {
		processingRevision, err = r.materializeLockedCollectionExpiry(ctx, tx, input.CollectionID,
			input.Processor, current, processingRevision, now)
		if err != nil {
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
				return ProcessingConsent{}, fmt.Errorf("commit elapsed collection consent replay: %w", err)
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
			return ProcessingConsent{}, fmt.Errorf("supersede collection consent: %w", err)
		}
	}
	consentID, err := r.newEventID()
	if err != nil {
		return ProcessingConsent{}, fmt.Errorf("generate collection consent id: %w", err)
	}
	if capabilities.exactModelIdentity {
		_, err = tx.ExecContext(ctx, `INSERT INTO processing_consents (
id,scope,collection_id,processor,endpoint_id,model_id,governance_profile_id,governance_revision,
governance_head_revision,purposes,data_types,policy_version,decision,consent_revision,
granted_by_user_id,decided_at,expires_at,created_at,updated_at
) VALUES ($1,'collection',$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'granted',$12,$13,$14,$15,$14,$14)`,
			consentID, input.CollectionID, input.Processor, authority.EndpointID, authority.ModelID,
			authority.ProfileID, authority.GovernanceRevision, authority.HeadRevision, input.Purposes,
			input.DataTypes, input.PolicyVersion, consentRevision, input.ActorUserID, now, input.ExpiresAt)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO processing_consents (
id,scope,collection_id,processor,endpoint_id,governance_profile_id,governance_revision,
governance_head_revision,purposes,data_types,policy_version,decision,consent_revision,
granted_by_user_id,decided_at,expires_at,created_at,updated_at
) VALUES ($1,'collection',$2,$3,$4,$5,$6,$7,$8,$9,$10,'granted',$11,$12,$13,$14,$13,$13)`,
			consentID, input.CollectionID, input.Processor, authority.EndpointID, authority.ProfileID,
			authority.GovernanceRevision, authority.HeadRevision, input.Purposes, input.DataTypes,
			input.PolicyVersion, consentRevision, input.ActorUserID, now, input.ExpiresAt)
	}
	if err != nil {
		return ProcessingConsent{}, fmt.Errorf("insert collection consent: %w", err)
	}
	processingRevision++
	if _, err := tx.ExecContext(ctx, `UPDATE knowledge_collections SET collection_processing_revision=$2,updated_at=$3 WHERE id=$1`, input.CollectionID, processingRevision, now); err != nil {
		return ProcessingConsent{}, fmt.Errorf("advance collection processing revision: %w", err)
	}
	value := ProcessingConsent{Processor: input.Processor, EndpointID: authority.EndpointID,
		ModelID: authority.ModelID, ProfileContractHash: authority.ProfileContractHash,
		Decision: "granted", EffectiveStatus: "granted", Purposes: input.Purposes,
		DataTypes: input.DataTypes, PolicyVersion: input.PolicyVersion, DecidedAt: now, ExpiresAt: input.ExpiresAt}
	if err := r.insertCollectionConsentEvent(ctx, tx, input.CollectionID, value, consentRevision, processingRevision, authority); err != nil {
		return ProcessingConsent{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProcessingConsent{}, fmt.Errorf("commit collection consent grant: %w", err)
	}
	return value, nil
}

func (r *PostgresRepository) RevokeCollectionConsent(ctx context.Context, input CollectionConsentLookupInput) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin collection consent revoke: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	collection, _, err := lockCollectionForManage(ctx, tx, input.CollectionID, input.ActorUserID)
	if err != nil {
		return err
	}
	capabilities, err := loadRuntimeSchemaCapabilities(ctx, tx)
	if err != nil {
		return err
	}
	current, found, err := lockCurrentCollectionConsent(ctx, tx, capabilities, input.CollectionID,
		ProcessorModelIdentity{Processor: input.Processor, EndpointID: input.EndpointID, ModelID: input.ModelID})
	if err != nil {
		return err
	}
	if !found || current.Decision == "revoked" {
		return nil
	}
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return fmt.Errorf("read consent revocation time: %w", err)
	}
	now = now.UTC()
	processingRevision, err := r.materializeLockedCollectionExpiry(ctx, tx, input.CollectionID,
		input.Processor, current, collection.ProcessingRevision, now)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE processing_consents SET superseded_at=$2,updated_at=$2 WHERE id=$1`, current.ID, now); err != nil {
		return fmt.Errorf("supersede granted consent: %w", err)
	}
	consentID, err := r.newEventID()
	if err != nil {
		return fmt.Errorf("generate revoked consent id: %w", err)
	}
	consentRevision := current.ConsentRevision + 1
	if capabilities.exactModelIdentity {
		_, err = tx.ExecContext(ctx, `INSERT INTO processing_consents (
id,scope,collection_id,processor,endpoint_id,model_id,governance_profile_id,governance_revision,
governance_head_revision,purposes,data_types,policy_version,decision,consent_revision,
granted_by_user_id,decided_at,created_at,updated_at
) VALUES ($1,'collection',$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'revoked',$12,$13,$14,$14,$14)`,
			consentID, input.CollectionID, input.Processor, current.EndpointID, current.ModelID,
			current.ProfileID, current.GovernanceRevision, current.HeadRevision, current.Purposes,
			current.DataTypes, current.PolicyVersion, consentRevision, input.ActorUserID, now)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO processing_consents (
id,scope,collection_id,processor,endpoint_id,governance_profile_id,governance_revision,
governance_head_revision,purposes,data_types,policy_version,decision,consent_revision,
granted_by_user_id,decided_at,created_at,updated_at
) VALUES ($1,'collection',$2,$3,$4,$5,$6,$7,$8,$9,$10,'revoked',$11,$12,$13,$13,$13)`, consentID,
			input.CollectionID, input.Processor, current.EndpointID, current.ProfileID, current.GovernanceRevision,
			current.HeadRevision, current.Purposes, current.DataTypes, current.PolicyVersion, consentRevision,
			input.ActorUserID, now)
	}
	if err != nil {
		return fmt.Errorf("insert revoked collection consent: %w", err)
	}
	processingRevision++
	if _, err := tx.ExecContext(ctx, `UPDATE knowledge_collections SET collection_processing_revision=$2,updated_at=$3 WHERE id=$1`, input.CollectionID, processingRevision, now); err != nil {
		return fmt.Errorf("advance collection processing revision: %w", err)
	}
	authority := authorityFromCurrent(current)
	value := consentFromRow(input.Processor, current)
	value.Decision, value.EffectiveStatus, value.DecidedAt, value.ExpiresAt = "revoked", "revoked", now, nil
	if err := r.insertCollectionConsentEvent(ctx, tx, input.CollectionID, value, consentRevision, processingRevision, authority); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit collection consent revoke: %w", err)
	}
	return nil
}

func lockConsentAuthority(ctx context.Context, tx *sql.Tx, capabilities runtimeSchemaCapabilities, identity ProcessorModelIdentity) (consentAuthority, error) {
	if err := lockGovernanceProcessor(ctx, tx, identity.Processor); err != nil {
		return consentAuthority{}, err
	}
	if !capabilities.exactModelIdentity && identity.EndpointID != "" {
		return consentAuthority{}, ErrKnowledgeProcessorUnavailable
	}
	query := `SELECT h.endpoint_id,''::text,h.active_profile_id,''::text,
h.active_governance_revision,h.head_revision,array_to_string(p.allowed_purposes,E'\x1f'),
array_to_string(p.allowed_data_types,E'\x1f')
FROM processor_governance_heads h JOIN processor_governance_profiles p
ON p.processor=h.processor AND p.endpoint_id=h.endpoint_id AND p.id=h.active_profile_id
AND p.governance_revision=h.active_governance_revision WHERE h.processor=$1 AND h.status='active'
AND p.status='approved' ORDER BY h.endpoint_id LIMIT 2 FOR UPDATE OF h,p`
	args := []any{identity.Processor}
	if capabilities.exactModelIdentity {
		query = `SELECT h.endpoint_id,h.model_id,h.active_profile_id,p.profile_contract_hash,
h.active_governance_revision,h.head_revision,array_to_string(p.allowed_purposes,E'\x1f'),
array_to_string(p.allowed_data_types,E'\x1f')
FROM processor_governance_heads h JOIN processor_governance_profiles p
ON p.processor=h.processor AND p.endpoint_id=h.endpoint_id AND p.model_id=h.model_id
AND p.id=h.active_profile_id AND p.governance_revision=h.active_governance_revision
WHERE h.processor=$1 AND h.status='active' AND p.status='approved'`
		if identity.EndpointID != "" {
			query += ` AND h.endpoint_id=$2 AND h.model_id=$3`
			args = append(args, identity.EndpointID, identity.ModelID)
		}
		query += ` ORDER BY h.endpoint_id,h.model_id LIMIT 2 FOR UPDATE OF h,p`
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return consentAuthority{}, fmt.Errorf("lock consent processor identity: %w", err)
	}
	defer rows.Close()
	values := make([]consentAuthority, 0, 2)
	for rows.Next() {
		var value consentAuthority
		var purposes, dataTypes string
		if err := rows.Scan(&value.EndpointID, &value.ModelID, &value.ProfileID,
			&value.ProfileContractHash, &value.GovernanceRevision, &value.HeadRevision,
			&purposes, &dataTypes); err != nil {
			return consentAuthority{}, fmt.Errorf("scan consent processor identity: %w", err)
		}
		value.AllowedPurposes, value.AllowedDataTypes = splitSQLList(purposes), splitSQLList(dataTypes)
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return consentAuthority{}, fmt.Errorf("iterate consent processor identities: %w", err)
	}
	if len(values) == 0 {
		return consentAuthority{}, ErrKnowledgeProcessorUnavailable
	}
	if len(values) != 1 {
		return consentAuthority{}, ErrGovernanceIdentityAmbiguous
	}
	return values[0], nil
}

func lockCurrentCollectionConsent(ctx context.Context, tx *sql.Tx, capabilities runtimeSchemaCapabilities,
	collectionID string, identity ProcessorModelIdentity) (currentConsentRow, bool, error) {
	query := `SELECT pc.id,pc.endpoint_id,''::text,pc.governance_profile_id,''::text,
pc.governance_revision,pc.governance_head_revision,pc.decision,array_to_string(pc.purposes,E'\x1f'),
array_to_string(pc.data_types,E'\x1f'),pc.policy_version,pc.consent_revision,pc.decided_at,
pc.expires_at,pc.expiry_materialized_at FROM processing_consents pc
WHERE pc.scope='collection' AND pc.collection_id=$1 AND pc.processor=$2
AND pc.superseded_at IS NULL ORDER BY pc.endpoint_id LIMIT 2 FOR UPDATE OF pc`
	args := []any{collectionID, identity.Processor}
	if capabilities.exactModelIdentity {
		query = `SELECT pc.id,pc.endpoint_id,pc.model_id,pc.governance_profile_id,p.profile_contract_hash,
pc.governance_revision,pc.governance_head_revision,pc.decision,array_to_string(pc.purposes,E'\x1f'),
array_to_string(pc.data_types,E'\x1f'),pc.policy_version,pc.consent_revision,pc.decided_at,
pc.expires_at,pc.expiry_materialized_at FROM processing_consents pc
JOIN processor_governance_profiles p ON p.id=pc.governance_profile_id
WHERE pc.scope='collection' AND pc.collection_id=$1 AND pc.processor=$2 AND pc.superseded_at IS NULL`
		if identity.EndpointID != "" {
			query += ` AND pc.endpoint_id=$3 AND pc.model_id=$4`
			args = append(args, identity.EndpointID, identity.ModelID)
		}
		query += ` ORDER BY pc.endpoint_id,pc.model_id LIMIT 2 FOR UPDATE OF pc`
	}
	return lockCurrentConsentRows(ctx, tx, "collection", query, args...)
}

func lockCurrentConsentRows(ctx context.Context, tx *sql.Tx, label, query string, args ...any) (currentConsentRow, bool, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return currentConsentRow{}, false, fmt.Errorf("lock current %s consent: %w", label, err)
	}
	defer rows.Close()
	values := make([]currentConsentRow, 0, 2)
	for rows.Next() {
		var row currentConsentRow
		var purposes, dataTypes string
		if err := rows.Scan(&row.ID, &row.EndpointID, &row.ModelID, &row.ProfileID,
			&row.ProfileContractHash, &row.GovernanceRevision, &row.HeadRevision, &row.Decision,
			&purposes, &dataTypes, &row.PolicyVersion, &row.ConsentRevision, &row.DecidedAt,
			&row.ExpiresAt, &row.ExpiryMaterializedAt); err != nil {
			return currentConsentRow{}, false, fmt.Errorf("scan current %s consent: %w", label, err)
		}
		row.Purposes, row.DataTypes = splitSQLList(purposes), splitSQLList(dataTypes)
		values = append(values, row)
	}
	if err := rows.Err(); err != nil {
		return currentConsentRow{}, false, fmt.Errorf("iterate current %s consents: %w", label, err)
	}
	if len(values) == 0 {
		return currentConsentRow{}, false, nil
	}
	if len(values) != 1 {
		return currentConsentRow{}, false, ErrConsentIdentityAmbiguous
	}
	return values[0], true, nil
}

func (r *PostgresRepository) insertCollectionConsentEvent(ctx context.Context, tx *sql.Tx, collectionID string,
	value ProcessingConsent, consentRevision, processingRevision int64, authority consentAuthority) error {
	eventID, err := r.newEventID()
	if err != nil {
		return fmt.Errorf("generate collection consent event id: %w", err)
	}
	payloadObject := map[string]any{"schemaVersion": 1, "scope": "collection", "collectionId": collectionID,
		"processor": value.Processor, "endpointId": authority.EndpointID, "modelId": authority.ModelID,
		"profileContractHash": authority.ProfileContractHash, "decision": value.Decision,
		"effectiveStatus": value.EffectiveStatus, "consentRevision": consentRevision,
		"collectionProcessingRevision": processingRevision, "governanceProfileId": authority.ProfileID,
		"governanceRevision": authority.GovernanceRevision, "governanceHeadRevision": authority.HeadRevision}
	addConsentExpiryEventFields(payloadObject, value)
	payload, err := json.Marshal(payloadObject)
	if err != nil {
		return fmt.Errorf("marshal collection consent event: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO knowledge_outbox(event_id,aggregate_type,aggregate_key,event_type,payload)
VALUES ($1,'knowledge_collection',$2,'knowledge.collection.consent.changed',$3::jsonb)`, eventID, collectionID, string(payload)); err != nil {
		return fmt.Errorf("insert collection consent event: %w", err)
	}
	return nil
}

func addConsentExpiryEventFields(payload map[string]any, value ProcessingConsent) {
	if value.ExpiresAt != nil {
		key := "expiresAt"
		if value.MaterializedAt != nil {
			key = "expiredAt"
		}
		payload[key] = value.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	if value.MaterializedAt != nil {
		payload["materializedAt"] = value.MaterializedAt.UTC().Format(time.RFC3339Nano)
		payload["reason"] = "expired"
	}
}

func authorityFromCurrent(row currentConsentRow) consentAuthority {
	return consentAuthority{EndpointID: row.EndpointID, ModelID: row.ModelID, ProfileID: row.ProfileID,
		ProfileContractHash: row.ProfileContractHash, GovernanceRevision: row.GovernanceRevision,
		HeadRevision: row.HeadRevision}
}

func consentFromRow(processor string, row currentConsentRow) ProcessingConsent {
	effective := row.Decision
	if row.Decision == "granted" && row.ExpiresAt.Valid && !row.ExpiresAt.Time.After(time.Now().UTC()) {
		effective = "expired"
	}
	value := ProcessingConsent{Processor: processor, EndpointID: row.EndpointID, ModelID: row.ModelID,
		ProfileContractHash: row.ProfileContractHash, Decision: row.Decision, EffectiveStatus: effective,
		Purposes: row.Purposes, DataTypes: row.DataTypes, PolicyVersion: row.PolicyVersion,
		DecidedAt: row.DecidedAt.UTC()}
	if row.ExpiresAt.Valid {
		expiry := row.ExpiresAt.Time.UTC()
		value.ExpiresAt = &expiry
	}
	return value
}

func nullTimeEqual(value sql.NullTime, expected *time.Time) bool {
	if !value.Valid || expected == nil {
		return !value.Valid && expected == nil
	}
	return value.Time.Equal(*expected)
}

func isStringSubset(values, allowed []string) bool {
	for _, value := range values {
		if !slices.Contains(allowed, value) {
			return false
		}
	}
	return true
}

func isDataTypeSubset(values, allowed []string) bool {
	for _, value := range values {
		if value == "*" && !slices.Contains(allowed, "*") {
			return false
		}
		if value != "*" && !slices.Contains(allowed, value) && !slices.Contains(allowed, "*") {
			return false
		}
	}
	return true
}

func splitSQLList(value string) []string {
	if value == "" {
		return []string{}
	}
	return strings.Split(value, "\x1f")
}
