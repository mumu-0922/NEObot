package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

func (r *PostgresRepository) ApplyGovernance(ctx context.Context, manifest GovernanceManifest, manifestHash string) (ProcessorGovernanceHead, error) {
	if err := r.requireDB(); err != nil {
		return ProcessorGovernanceHead{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ProcessorGovernanceHead{}, fmt.Errorf("begin governance apply: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockGovernanceBinding(ctx, tx, manifest.Processor, manifest.EndpointID); err != nil {
		return ProcessorGovernanceHead{}, err
	}

	head, found, err := queryGovernanceHead(ctx, tx, manifest.Processor, manifest.EndpointID)
	if err != nil {
		return ProcessorGovernanceHead{}, err
	}
	if found && head.Status == "active" && head.ManifestHash == manifestHash {
		return head, nil
	}
	var governanceRevision int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(max(governance_revision),0)+1 FROM processor_governance_profiles WHERE processor=$1 AND endpoint_id=$2`, manifest.Processor, manifest.EndpointID).Scan(&governanceRevision); err != nil {
		return ProcessorGovernanceHead{}, fmt.Errorf("allocate governance revision: %w", err)
	}
	profileID, err := r.newEventID()
	if err != nil {
		return ProcessorGovernanceHead{}, fmt.Errorf("generate governance profile id: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO processor_governance_profiles (
id,processor,endpoint_id,model_api_version,allowed_purposes,allowed_data_types,region,
retention_policy,deletion_contract,training_use,status,governance_revision,manifest_hash
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'approved',$11,$12)`, profileID, manifest.Processor,
		manifest.EndpointID, manifest.ModelAPIVersion, manifest.AllowedPurposes, manifest.AllowedDataTypes,
		manifest.Region, manifest.RetentionPolicy, manifest.DeletionContract, manifest.TrainingUse,
		governanceRevision, manifestHash); err != nil {
		return ProcessorGovernanceHead{}, fmt.Errorf("insert governance profile: %w", err)
	}
	headRevision := int64(1)
	if found {
		headRevision = head.HeadRevision + 1
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO processor_governance_heads (
processor,endpoint_id,status,active_profile_id,active_governance_revision,head_revision
) VALUES ($1,$2,'active',$3,$4,$5)
ON CONFLICT (processor,endpoint_id) DO UPDATE SET status='active',active_profile_id=EXCLUDED.active_profile_id,
active_governance_revision=EXCLUDED.active_governance_revision,head_revision=EXCLUDED.head_revision,updated_at=clock_timestamp()
RETURNING updated_at`, manifest.Processor, manifest.EndpointID, profileID, governanceRevision, headRevision).Scan(&head.UpdatedAt)
	if err != nil {
		return ProcessorGovernanceHead{}, fmt.Errorf("advance governance head: %w", err)
	}
	head = ProcessorGovernanceHead{Processor: manifest.Processor, EndpointID: manifest.EndpointID, Status: "active",
		ActiveProfileID: profileID, ActiveGovernanceRevision: governanceRevision, HeadRevision: headRevision,
		ManifestHash: manifestHash, UpdatedAt: head.UpdatedAt.UTC()}
	if err := r.insertGovernanceEvent(ctx, tx, head); err != nil {
		return ProcessorGovernanceHead{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProcessorGovernanceHead{}, fmt.Errorf("commit governance apply: %w", err)
	}
	return head, nil
}

func (r *PostgresRepository) DisableGovernance(ctx context.Context, processor, endpointID string) (ProcessorGovernanceHead, error) {
	if err := r.requireDB(); err != nil {
		return ProcessorGovernanceHead{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ProcessorGovernanceHead{}, fmt.Errorf("begin governance disable: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockGovernanceBinding(ctx, tx, processor, endpointID); err != nil {
		return ProcessorGovernanceHead{}, err
	}
	head, found, err := queryGovernanceHead(ctx, tx, processor, endpointID)
	if err != nil {
		return ProcessorGovernanceHead{}, err
	}
	if !found {
		return ProcessorGovernanceHead{}, ErrGovernanceHeadNotFound
	}
	if head.Status == "disabled" {
		return head, nil
	}
	head.Status, head.ActiveProfileID, head.ManifestHash = "disabled", "", ""
	head.ActiveGovernanceRevision, head.HeadRevision = 0, head.HeadRevision+1
	if err := tx.QueryRowContext(ctx, `UPDATE processor_governance_heads SET status='disabled',active_profile_id=NULL,
active_governance_revision=NULL,head_revision=$3,updated_at=clock_timestamp() WHERE processor=$1 AND endpoint_id=$2
RETURNING updated_at`, processor, endpointID, head.HeadRevision).Scan(&head.UpdatedAt); err != nil {
		return ProcessorGovernanceHead{}, fmt.Errorf("disable governance head: %w", err)
	}
	head.UpdatedAt = head.UpdatedAt.UTC()
	if err := r.insertGovernanceEvent(ctx, tx, head); err != nil {
		return ProcessorGovernanceHead{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProcessorGovernanceHead{}, fmt.Errorf("commit governance disable: %w", err)
	}
	return head, nil
}

func lockGovernanceBinding(ctx context.Context, tx *sql.Tx, processor, endpointID string) error {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, processor+"\n"+endpointID); err != nil {
		return fmt.Errorf("lock governance binding: %w", err)
	}
	return nil
}

func queryGovernanceHead(ctx context.Context, tx *sql.Tx, processor, endpointID string) (ProcessorGovernanceHead, bool, error) {
	var head ProcessorGovernanceHead
	var profileID sql.NullString
	var governanceRevision sql.NullInt64
	var manifestHash sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT h.status,h.active_profile_id,h.active_governance_revision,h.head_revision,
h.updated_at,p.manifest_hash FROM processor_governance_heads h LEFT JOIN processor_governance_profiles p
ON p.id=h.active_profile_id WHERE h.processor=$1 AND h.endpoint_id=$2 FOR UPDATE OF h`, processor, endpointID).Scan(
		&head.Status, &profileID, &governanceRevision, &head.HeadRevision, &head.UpdatedAt, &manifestHash)
	if errors.Is(err, sql.ErrNoRows) {
		return head, false, nil
	}
	if err != nil {
		return head, false, fmt.Errorf("lock governance head: %w", err)
	}
	head.Processor, head.EndpointID = processor, endpointID
	head.ActiveProfileID, head.ActiveGovernanceRevision, head.ManifestHash = profileID.String, governanceRevision.Int64, manifestHash.String
	head.UpdatedAt = head.UpdatedAt.UTC()
	return head, true, nil
}

func (r *PostgresRepository) insertGovernanceEvent(ctx context.Context, tx *sql.Tx, head ProcessorGovernanceHead) error {
	eventID, err := r.newEventID()
	if err != nil {
		return fmt.Errorf("generate governance event id: %w", err)
	}
	payload := map[string]any{"schemaVersion": 1, "processor": head.Processor, "endpointId": head.EndpointID,
		"status": head.Status, "headRevision": head.HeadRevision}
	if head.Status == "active" {
		payload["activeProfileId"] = head.ActiveProfileID
		payload["governanceRevision"] = head.ActiveGovernanceRevision
		payload["manifestHash"] = head.ManifestHash
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal governance event: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO knowledge_outbox(event_id,aggregate_type,aggregate_key,event_type,payload)
VALUES ($1,'processor_governance_head',$2,'knowledge.governance.head.changed',$3::jsonb)`, eventID,
		head.Processor+"/"+head.EndpointID, string(encoded)); err != nil {
		return fmt.Errorf("insert governance event: %w", err)
	}
	return nil
}
