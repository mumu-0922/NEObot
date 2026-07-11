package knowledge

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
)

type parseAuthority struct {
	Processor, ConsentID, EndpointID, ProfileID       string
	GovernanceRevision, HeadRevision, ConsentRevision int64
}

func documentOperationIdempotencyScope(documentID, operation, actorUserID string) string {
	return "document:" + documentID + ":" + operation + ":actor:" + actorUserID
}

func (r *PostgresRepository) CreateDocumentVersion(ctx context.Context, input CreateDocumentVersionRepositoryInput) (Document, error) {
	if err := r.requireDB(); err != nil {
		return Document{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Document{}, fmt.Errorf("begin replace document version: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var collectionID string
	err = tx.QueryRowContext(ctx, `
SELECT collection_id FROM knowledge_documents WHERE id = $1
`, input.DocumentID).Scan(&collectionID)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, ErrDocumentNotFound
	}
	if err != nil {
		return Document{}, fmt.Errorf("resolve replacement collection: %w", err)
	}
	collection, _, err := lockCollectionForManage(ctx, tx, collectionID, input.ActorUserID)
	if errors.Is(err, ErrCollectionNotFound) {
		return Document{}, ErrDocumentNotFound
	}
	if err != nil {
		return Document{}, err
	}

	var document Document
	var currentVersionID string
	var visibilityEpoch int64
	err = tx.QueryRowContext(ctx, `
SELECT id, collection_id, status, current_version_id, visibility_epoch, created_at, updated_at
FROM knowledge_documents
WHERE id = $1 AND collection_id = $2 AND status = 'active' AND deleted_at IS NULL
FOR UPDATE
`, input.DocumentID, collectionID).Scan(
		&document.ID, &document.CollectionID, &document.Status, &currentVersionID,
		&visibilityEpoch, &document.CreatedAt, &document.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, ErrDocumentNotFound
	}
	if err != nil {
		return Document{}, fmt.Errorf("lock active document for replacement: %w", err)
	}
	if replay, found, replayErr := findVersionReplay(ctx, tx, input, document, currentVersionID); replayErr != nil {
		return Document{}, replayErr
	} else if found {
		return replay, nil
	}

	if processing, checkErr := hasDocumentProcessingWork(ctx, tx, input.DocumentID); checkErr != nil {
		return Document{}, checkErr
	} else if processing {
		return Document{}, ErrDocumentProcessing
	}

	var currentFileID string
	if err := tx.QueryRowContext(ctx, `
SELECT file_id FROM knowledge_document_versions
WHERE document_id = $1 AND id = $2 AND status = 'active'
`, input.DocumentID, currentVersionID).Scan(&currentFileID); err != nil {
		return Document{}, fmt.Errorf("resolve current document file: %w", err)
	}
	file, hash, err := lockReplacementFiles(ctx, tx, currentFileID, input.FileID, input.ActorUserID)
	if err != nil {
		return Document{}, err
	}
	authority, err := resolveParseAuthority(ctx, tx, collectionID, input.ParseProcessor, file.MIMEType)
	if err != nil {
		return Document{}, err
	}

	var sourceVersion int64
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(max(source_version), 0) + 1
FROM knowledge_document_versions WHERE document_id = $1
`, input.DocumentID).Scan(&sourceVersion); err != nil {
		return Document{}, fmt.Errorf("allocate replacement source version: %w", err)
	}
	version := DocumentVersion{File: file}
	err = tx.QueryRowContext(ctx, `
INSERT INTO knowledge_document_versions (
  id, document_id, file_id, source_version, content_hash,
  created_by_user_id, idempotency_key, request_hash
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING id, source_version, status, created_at, updated_at
`, input.VersionID, input.DocumentID, input.FileID, sourceVersion, hash,
		input.ActorUserID, input.IdempotencyKey, input.RequestHash).Scan(
		&version.ID, &version.SourceVersion, &version.Status, &version.CreatedAt, &version.UpdatedAt)
	if err != nil {
		if isConstraint(err, "idx_knowledge_document_versions_one_nonterminal") {
			return Document{}, ErrDocumentProcessing
		}
		if isConstraint(err, "idx_knowledge_document_versions_document_creator_idempotency") {
			return Document{}, ErrIdempotencyConflict
		}
		return Document{}, fmt.Errorf("insert replacement version: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
UPDATE knowledge_documents SET updated_at = now() WHERE id = $1 RETURNING updated_at
`, input.DocumentID).Scan(&document.UpdatedAt); err != nil {
		return Document{}, fmt.Errorf("touch document replacement time: %w", err)
	}
	document.CurrentVersion, err = queryVersionByID(ctx, tx, input.DocumentID, currentVersionID)
	if err != nil {
		return Document{}, err
	}
	document.PendingVersion = &version

	_, err = tx.ExecContext(ctx, `
INSERT INTO knowledge_processing_jobs (
  id, collection_id, document_id, document_version_id, file_id, stage, operation,
  processor, endpoint_id, governance_profile_id, governance_revision,
  governance_head_revision, collection_consent_id, collection_consent_revision,
  collection_acl_revision, collection_visibility_epoch,
  collection_processing_revision, document_visibility_epoch,
  requested_by_user_id, idempotency_scope, idempotency_key, request_hash
) VALUES (
  $1,$2,$3,$4,$5,'parse','replace',$6,$7,$8,$9,$10,$11,$12,
  $13,$14,$15,$16,$17,$18,$19,$20
)
`, input.JobID, collectionID, input.DocumentID, input.VersionID, input.FileID,
		input.ParseProcessor, authority.EndpointID, authority.ProfileID,
		authority.GovernanceRevision, authority.HeadRevision, authority.ConsentID,
		authority.ConsentRevision, collection.ACLRevision, collection.VisibilityEpoch,
		collection.ProcessingRevision, visibilityEpoch, input.ActorUserID,
		documentOperationIdempotencyScope(input.DocumentID, "replace", input.ActorUserID),
		input.IdempotencyKey, input.RequestHash)
	if err != nil {
		return Document{}, fmt.Errorf("insert replacement parse job: %w", err)
	}
	if err := r.insertDocumentOutbox(
		ctx, tx, document, version, input.JobID, hash, "replace",
		authority, collection, visibilityEpoch,
	); err != nil {
		return Document{}, err
	}
	if err := tx.Commit(); err != nil {
		return Document{}, fmt.Errorf("commit replacement version: %w", err)
	}
	return document, nil
}

func findVersionReplay(ctx context.Context, tx *sql.Tx, input CreateDocumentVersionRepositoryInput, document Document, currentVersionID string) (Document, bool, error) {
	var versionID, storedHash string
	err := tx.QueryRowContext(ctx, `
SELECT id, request_hash FROM knowledge_document_versions
WHERE document_id = $1 AND created_by_user_id = $2 AND idempotency_key = $3
`, input.DocumentID, input.ActorUserID, input.IdempotencyKey).Scan(&versionID, &storedHash)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, false, nil
	}
	if err != nil {
		return Document{}, false, fmt.Errorf("check replacement replay: %w", err)
	}
	if storedHash != input.RequestHash {
		return Document{}, false, ErrIdempotencyConflict
	}
	document.CurrentVersion, err = queryVersionByID(ctx, tx, input.DocumentID, currentVersionID)
	if err != nil {
		return Document{}, false, err
	}
	if versionID == currentVersionID {
		return document, true, nil
	}
	document.PendingVersion, err = queryVersionByID(ctx, tx, input.DocumentID, versionID)
	if err != nil {
		return Document{}, false, err
	}
	return document, true, nil
}

func lockReplacementFiles(ctx context.Context, tx *sql.Tx, currentFileID, newFileID, actorID string) (DocumentFile, string, error) {
	ids := []string{currentFileID}
	if newFileID != currentFileID {
		ids = append(ids, newFileID)
	}
	sort.Strings(ids)
	var result DocumentFile
	var hash string
	foundNew := false
	for _, id := range ids {
		var ownerID, status string
		var purpose sql.NullString
		var deletedAt sql.NullTime
		var file DocumentFile
		var fileHash string
		err := tx.QueryRowContext(ctx, `
SELECT id,user_id,original_filename,mime_type,byte_size,sha256,
  upload_status,metadata->>'purpose',deleted_at
FROM files WHERE id = $1 FOR UPDATE
`, id).Scan(&file.ID, &ownerID, &file.Name, &file.MIMEType, &file.ByteSize,
			&fileHash, &status, &purpose, &deletedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return DocumentFile{}, "", ErrFileNotFound
		}
		if err != nil {
			return DocumentFile{}, "", fmt.Errorf("lock replacement file: %w", err)
		}
		if id == newFileID {
			if ownerID != actorID || status != "available" || deletedAt.Valid ||
				!purpose.Valid || purpose.String != "knowledge" {
				return DocumentFile{}, "", ErrFileNotFound
			}
			result, hash, foundNew = file, fileHash, true
		}
	}
	if !foundNew {
		return DocumentFile{}, "", ErrFileNotFound
	}
	return result, hash, nil
}

func resolveParseAuthority(ctx context.Context, tx *sql.Tx, collectionID, processor, mimeType string) (parseAuthority, error) {
	authority := parseAuthority{Processor: processor}
	err := tx.QueryRowContext(ctx, `
SELECT h.endpoint_id,h.active_profile_id,h.active_governance_revision,h.head_revision
FROM processor_governance_heads h
JOIN processor_governance_profiles p
  ON p.processor=h.processor AND p.endpoint_id=h.endpoint_id
 AND p.id=h.active_profile_id AND p.governance_revision=h.active_governance_revision
WHERE h.processor=$1 AND h.status='active' AND p.status='approved'
  AND 'parse'=ANY(p.allowed_purposes)
  AND ($2=ANY(p.allowed_data_types) OR '*'=ANY(p.allowed_data_types))
ORDER BY EXISTS (
  SELECT 1 FROM processing_consents pc
  WHERE pc.scope='collection' AND pc.collection_id=$3
    AND pc.processor=h.processor AND pc.endpoint_id=h.endpoint_id
    AND pc.governance_profile_id=h.active_profile_id
    AND pc.governance_revision=h.active_governance_revision
    AND pc.governance_head_revision=h.head_revision
    AND pc.superseded_at IS NULL AND pc.decision='granted'
    AND (pc.expires_at IS NULL OR pc.expires_at>now())
    AND 'parse'=ANY(pc.purposes)
    AND ($2=ANY(pc.data_types) OR '*'=ANY(pc.data_types))
) DESC,h.endpoint_id
LIMIT 1 FOR UPDATE OF h,p
`, processor, mimeType, collectionID).Scan(
		&authority.EndpointID, &authority.ProfileID,
		&authority.GovernanceRevision, &authority.HeadRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return parseAuthority{}, ErrKnowledgeProcessorUnavailable
	}
	if err != nil {
		return parseAuthority{}, fmt.Errorf("resolve parse processor: %w", err)
	}
	err = tx.QueryRowContext(ctx, `
SELECT id,consent_revision FROM processing_consents
WHERE scope='collection' AND collection_id=$1 AND processor=$2 AND endpoint_id=$3
  AND governance_profile_id=$4 AND governance_revision=$5
  AND governance_head_revision=$6 AND superseded_at IS NULL AND decision='granted'
  AND (expires_at IS NULL OR expires_at>now()) AND 'parse'=ANY(purposes)
  AND ($7=ANY(data_types) OR '*'=ANY(data_types))
FOR UPDATE
`, collectionID, processor, authority.EndpointID, authority.ProfileID,
		authority.GovernanceRevision, authority.HeadRevision, mimeType).Scan(
		&authority.ConsentID, &authority.ConsentRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return parseAuthority{}, ErrProcessingConsent
	}
	if err != nil {
		return parseAuthority{}, fmt.Errorf("resolve parse consent: %w", err)
	}
	return authority, nil
}
