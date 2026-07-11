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
	EndpointID, ProfileID             string
	GovernanceRevision, HeadRevision  int64
	AllowedPurposes, AllowedDataTypes []string
}

type currentConsentRow struct {
	ID, EndpointID, ProfileID, Decision, PolicyVersion string
	GovernanceRevision, HeadRevision, ConsentRevision  int64
	Purposes, DataTypes                                []string
	DecidedAt                                          time.Time
	ExpiresAt                                          sql.NullTime
	ExpiryMaterializedAt                               sql.NullTime
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
	rows, err := tx.QueryContext(ctx, `SELECT processor,decision,
CASE WHEN decision='granted' AND expires_at<=clock_timestamp() THEN 'expired' ELSE decision END,
array_to_string(purposes,E'\x1f'),
array_to_string(data_types,E'\x1f'),policy_version,decided_at,expires_at
FROM processing_consents WHERE scope='collection' AND collection_id=$1 AND superseded_at IS NULL ORDER BY processor`, input.CollectionID)
	if err != nil {
		return nil, fmt.Errorf("list collection consents: %w", err)
	}
	defer rows.Close()
	result := make([]ProcessingConsent, 0)
	for rows.Next() {
		var value ProcessingConsent
		var expires sql.NullTime
		var purposes, dataTypes string
		if err := rows.Scan(&value.Processor, &value.Decision, &value.EffectiveStatus, &purposes, &dataTypes,
			&value.PolicyVersion, &value.DecidedAt, &expires); err != nil {
			return nil, fmt.Errorf("scan collection consent: %w", err)
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
		return nil, fmt.Errorf("iterate collection consents: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close collection consents: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit collection consent list: %w", err)
	}
	return result, nil
}

func lockCollectionForConsentRead(ctx context.Context, tx *sql.Tx, collectionID, actorID string) error {
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
	authority, err := lockUniqueConsentAuthority(ctx, tx, input.Processor)
	if err != nil {
		return ProcessingConsent{}, err
	}
	if !isStringSubset(input.Purposes, authority.AllowedPurposes) || !isDataTypeSubset(input.DataTypes, authority.AllowedDataTypes) {
		return ProcessingConsent{}, ErrKnowledgeProcessorUnavailable
	}
	current, found, err := lockCurrentCollectionConsent(ctx, tx, input.CollectionID, input.Processor)
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
		processingRevision, err = r.materializeLockedCollectionExpiry(
			ctx, tx, input.CollectionID, input.Processor, current, processingRevision, now,
		)
		if err != nil {
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
	_, err = tx.ExecContext(ctx, `INSERT INTO processing_consents (
id,scope,collection_id,processor,endpoint_id,governance_profile_id,governance_revision,
governance_head_revision,purposes,data_types,policy_version,decision,consent_revision,
granted_by_user_id,decided_at,expires_at,created_at,updated_at
) VALUES ($1,'collection',$2,$3,$4,$5,$6,$7,$8,$9,$10,'granted',$11,$12,$13,$14,$13,$13)`,
		consentID, input.CollectionID, input.Processor, authority.EndpointID, authority.ProfileID,
		authority.GovernanceRevision, authority.HeadRevision, input.Purposes, input.DataTypes,
		input.PolicyVersion, consentRevision, input.ActorUserID, now, input.ExpiresAt)
	if err != nil {
		return ProcessingConsent{}, fmt.Errorf("insert collection consent: %w", err)
	}
	processingRevision++
	if _, err := tx.ExecContext(ctx, `UPDATE knowledge_collections SET collection_processing_revision=$2,updated_at=$3 WHERE id=$1`, input.CollectionID, processingRevision, now); err != nil {
		return ProcessingConsent{}, fmt.Errorf("advance collection processing revision: %w", err)
	}
	value := ProcessingConsent{Processor: input.Processor, Decision: "granted", EffectiveStatus: "granted", Purposes: input.Purposes,
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
	current, found, err := lockCurrentCollectionConsent(ctx, tx, input.CollectionID, input.Processor)
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
	processingRevision := collection.ProcessingRevision
	processingRevision, err = r.materializeLockedCollectionExpiry(
		ctx, tx, input.CollectionID, input.Processor, current, processingRevision, now,
	)
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
	_, err = tx.ExecContext(ctx, `INSERT INTO processing_consents (
id,scope,collection_id,processor,endpoint_id,governance_profile_id,governance_revision,
governance_head_revision,purposes,data_types,policy_version,decision,consent_revision,
granted_by_user_id,decided_at,created_at,updated_at
) VALUES ($1,'collection',$2,$3,$4,$5,$6,$7,$8,$9,$10,'revoked',$11,$12,$13,$13,$13)`, consentID,
		input.CollectionID, input.Processor, current.EndpointID, current.ProfileID, current.GovernanceRevision,
		current.HeadRevision, current.Purposes, current.DataTypes, current.PolicyVersion, consentRevision, input.ActorUserID, now)
	if err != nil {
		return fmt.Errorf("insert revoked collection consent: %w", err)
	}
	processingRevision++
	if _, err := tx.ExecContext(ctx, `UPDATE knowledge_collections SET collection_processing_revision=$2,updated_at=$3 WHERE id=$1`, input.CollectionID, processingRevision, now); err != nil {
		return fmt.Errorf("advance collection processing revision: %w", err)
	}
	authority := consentAuthority{EndpointID: current.EndpointID, ProfileID: current.ProfileID,
		GovernanceRevision: current.GovernanceRevision, HeadRevision: current.HeadRevision}
	value := ProcessingConsent{Processor: input.Processor, Decision: "revoked", EffectiveStatus: "revoked", Purposes: current.Purposes,
		DataTypes: current.DataTypes, PolicyVersion: current.PolicyVersion, DecidedAt: now}
	if err := r.insertCollectionConsentEvent(ctx, tx, input.CollectionID, value, consentRevision, processingRevision, authority); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit collection consent revoke: %w", err)
	}
	return nil
}

func lockUniqueConsentAuthority(ctx context.Context, tx *sql.Tx, processor string) (consentAuthority, error) {
	if err := lockGovernanceProcessor(ctx, tx, processor); err != nil {
		return consentAuthority{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT h.endpoint_id,h.active_profile_id,h.active_governance_revision,h.head_revision,
array_to_string(p.allowed_purposes,E'\x1f'),array_to_string(p.allowed_data_types,E'\x1f')
FROM processor_governance_heads h JOIN processor_governance_profiles p
ON p.processor=h.processor AND p.endpoint_id=h.endpoint_id AND p.id=h.active_profile_id
AND p.governance_revision=h.active_governance_revision WHERE h.processor=$1 AND h.status='active'
AND p.status='approved' ORDER BY h.endpoint_id LIMIT 2 FOR UPDATE OF h,p`, processor)
	if err != nil {
		return consentAuthority{}, fmt.Errorf("lock consent processor: %w", err)
	}
	defer rows.Close()
	values := []consentAuthority{}
	for rows.Next() {
		var value consentAuthority
		var purposes, dataTypes string
		if err := rows.Scan(&value.EndpointID, &value.ProfileID, &value.GovernanceRevision,
			&value.HeadRevision, &purposes, &dataTypes); err != nil {
			return consentAuthority{}, fmt.Errorf("scan consent processor: %w", err)
		}
		value.AllowedPurposes, value.AllowedDataTypes = splitSQLList(purposes), splitSQLList(dataTypes)
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return consentAuthority{}, fmt.Errorf("iterate consent processors: %w", err)
	}
	if len(values) != 1 {
		return consentAuthority{}, ErrKnowledgeProcessorUnavailable
	}
	return values[0], nil
}

func lockCurrentCollectionConsent(ctx context.Context, tx *sql.Tx, collectionID, processor string) (currentConsentRow, bool, error) {
	var row currentConsentRow
	query := tx.QueryRowContext(ctx, `SELECT id,endpoint_id,governance_profile_id,governance_revision,
governance_head_revision,decision,array_to_string(purposes,E'\x1f'),
array_to_string(data_types,E'\x1f'),policy_version,consent_revision,decided_at,expires_at,expiry_materialized_at
FROM processing_consents WHERE scope='collection' AND collection_id=$1 AND processor=$2
AND superseded_at IS NULL FOR UPDATE`, collectionID, processor)
	var purposes, dataTypes string
	err := query.Scan(&row.ID, &row.EndpointID, &row.ProfileID,
		&row.GovernanceRevision, &row.HeadRevision, &row.Decision, &purposes, &dataTypes,
		&row.PolicyVersion, &row.ConsentRevision, &row.DecidedAt, &row.ExpiresAt, &row.ExpiryMaterializedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return row, false, nil
	}
	if err != nil {
		return row, false, fmt.Errorf("lock current collection consent: %w", err)
	}
	row.Purposes, row.DataTypes = splitSQLList(purposes), splitSQLList(dataTypes)
	return row, true, nil
}

func (r *PostgresRepository) insertCollectionConsentEvent(ctx context.Context, tx *sql.Tx, collectionID string,
	value ProcessingConsent, consentRevision, processingRevision int64, authority consentAuthority) error {
	eventID, err := r.newEventID()
	if err != nil {
		return fmt.Errorf("generate collection consent event id: %w", err)
	}
	payloadObject := map[string]any{"schemaVersion": 1, "scope": "collection", "collectionId": collectionID,
		"processor": value.Processor, "endpointId": authority.EndpointID,
		"decision": value.Decision, "effectiveStatus": value.EffectiveStatus, "consentRevision": consentRevision,
		"collectionProcessingRevision": processingRevision, "governanceProfileId": authority.ProfileID,
		"governanceRevision": authority.GovernanceRevision, "governanceHeadRevision": authority.HeadRevision}
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
		return fmt.Errorf("marshal collection consent event: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO knowledge_outbox(event_id,aggregate_type,aggregate_key,event_type,payload)
VALUES ($1,'knowledge_collection',$2,'knowledge.collection.consent.changed',$3::jsonb)`, eventID, collectionID, string(payload)); err != nil {
		return fmt.Errorf("insert collection consent event: %w", err)
	}
	return nil
}

func consentFromRow(processor string, row currentConsentRow) ProcessingConsent {
	effective := row.Decision
	if row.Decision == "granted" && row.ExpiresAt.Valid && !row.ExpiresAt.Time.After(time.Now().UTC()) {
		effective = "expired"
	}
	value := ProcessingConsent{Processor: processor, Decision: row.Decision, EffectiveStatus: effective, Purposes: row.Purposes,
		DataTypes: row.DataTypes, PolicyVersion: row.PolicyVersion, DecidedAt: row.DecidedAt.UTC()}
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
