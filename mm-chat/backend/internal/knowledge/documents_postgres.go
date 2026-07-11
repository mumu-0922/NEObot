package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

func (r *PostgresRepository) ListDocuments(ctx context.Context, input ListDocumentsRepositoryInput) (DocumentPageResult, error) {
	if err := r.requireDB(); err != nil {
		return DocumentPageResult{}, err
	}
	if _, _, err := queryVisibleCollection(ctx, r.db, input.CollectionID, input.ActorUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DocumentPageResult{}, ErrCollectionNotFound
		}
		return DocumentPageResult{}, fmt.Errorf("authorize document list: %w", err)
	}
	limit := input.Limit
	if limit < 1 || limit > maximumPageLimit {
		limit = defaultPageLimit
	}
	query := documentReadSelect + `
WHERE d.collection_id = $1 AND d.deleted_at IS NULL
  AND d.status IN ('processing','active')
  AND ` + visibleDocumentCollectionACL + "\n"
	args := []any{input.CollectionID, input.ActorUserID}
	if input.After != nil {
		query += `  AND (d.created_at < $3 OR (d.created_at = $3 AND d.id < $4))
`
		args = append(args, input.After.CreatedAt.UTC(), input.After.ID)
	}
	query += fmt.Sprintf("ORDER BY d.created_at DESC, d.id DESC\nLIMIT $%d", len(args)+1)
	args = append(args, limit+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return DocumentPageResult{}, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()
	items := make([]Document, 0, limit)
	for rows.Next() {
		document, scanErr := scanDocument(rows)
		if scanErr != nil {
			return DocumentPageResult{}, fmt.Errorf("scan document: %w", scanErr)
		}
		if len(items) == limit {
			return DocumentPageResult{Items: items, HasMore: true}, nil
		}
		items = append(items, document)
	}
	if err := rows.Err(); err != nil {
		return DocumentPageResult{}, fmt.Errorf("iterate documents: %w", err)
	}
	return DocumentPageResult{Items: items}, nil
}

func (r *PostgresRepository) GetDocument(ctx context.Context, input DocumentLookupInput) (Document, error) {
	if err := r.requireDB(); err != nil {
		return Document{}, err
	}
	row := r.db.QueryRowContext(ctx, documentReadSelect+`
WHERE d.id = $1 AND d.deleted_at IS NULL AND d.status IN ('processing','active')
  AND `+visibleDocumentCollectionACL, input.DocumentID, input.ActorUserID)
	document, err := scanDocument(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, ErrDocumentNotFound
	}
	if err != nil {
		return Document{}, fmt.Errorf("get document: %w", err)
	}
	return document, nil
}

func (r *PostgresRepository) GetActiveDocumentContentMetadata(ctx context.Context, input DocumentLookupInput) (DocumentContentMetadata, error) {
	if err := r.requireDB(); err != nil {
		return DocumentContentMetadata{}, err
	}
	var metadata DocumentContentMetadata
	err := r.db.QueryRowContext(ctx, `
SELECT d.id, v.id, f.id, f.original_filename, f.mime_type, f.byte_size,
  f.sha256, f.object_key
FROM knowledge_documents d
JOIN knowledge_document_versions v
  ON v.id = d.current_version_id AND v.document_id = d.id AND v.status = 'active'
JOIN files f ON f.id = v.file_id AND f.upload_status = 'available' AND f.deleted_at IS NULL
WHERE d.id = $1 AND d.status = 'active' AND d.deleted_at IS NULL
  AND `+visibleDocumentCollectionACL, input.DocumentID, input.ActorUserID).Scan(
		&metadata.DocumentID, &metadata.VersionID, &metadata.FileID, &metadata.FileName,
		&metadata.MIMEType, &metadata.ByteSize, &metadata.SHA256, &metadata.ObjectKey,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return DocumentContentMetadata{}, ErrDocumentNotFound
	}
	if err != nil {
		return DocumentContentMetadata{}, fmt.Errorf("authorize active document content: %w", err)
	}
	return metadata, nil
}

const documentReadSelect = `
SELECT d.id, d.collection_id, d.status, d.created_at, d.updated_at,
  cv.id, cv.source_version, cv.status, cv.error_code,
  cf.id, cf.original_filename, cf.mime_type, cf.byte_size, cv.created_at, cv.updated_at,
  pv.id, pv.source_version, pv.status, pv.error_code,
  pf.id, pf.original_filename, pf.mime_type, pf.byte_size, pv.created_at, pv.updated_at
FROM knowledge_documents d
LEFT JOIN knowledge_document_versions cv
  ON cv.id = d.current_version_id AND cv.document_id = d.id
LEFT JOIN files cf ON cf.id = cv.file_id
LEFT JOIN LATERAL (
  SELECT candidate.* FROM knowledge_document_versions candidate
  WHERE candidate.document_id = d.id
    AND candidate.status IN ('uploaded','processing','failed')
    AND (cv.source_version IS NULL OR candidate.source_version > cv.source_version)
  ORDER BY candidate.source_version DESC LIMIT 1
) pv ON true
LEFT JOIN files pf ON pf.id = pv.file_id
`

const visibleDocumentCollectionACL = `EXISTS (
  SELECT 1 FROM knowledge_collections c
  JOIN users actor
    ON actor.id = $2
   AND actor.account_status = 'active'
   AND actor.deleted_at IS NULL
  LEFT JOIN team_memberships m
    ON m.team_id = c.team_id AND m.user_id = $2 AND m.status = 'active'
  LEFT JOIN teams t ON t.id = c.team_id AND t.deleted_at IS NULL
  WHERE c.id = d.collection_id AND c.deleted_at IS NULL
    AND ((c.scope = 'personal' AND c.owner_user_id = $2)
      OR (c.scope = 'team' AND m.user_id IS NOT NULL AND t.id IS NOT NULL))
)`

type rowScanner interface{ Scan(...any) error }

type nullableDocumentVersion struct {
	ID, Status, ErrorCode, FileID, FileName, MIMEType sql.NullString
	SourceVersion, ByteSize                           sql.NullInt64
	CreatedAt, UpdatedAt                              sql.NullTime
}

func (version *nullableDocumentVersion) scanTargets() []any {
	return []any{&version.ID, &version.SourceVersion, &version.Status, &version.ErrorCode,
		&version.FileID, &version.FileName, &version.MIMEType, &version.ByteSize,
		&version.CreatedAt, &version.UpdatedAt}
}

func (version nullableDocumentVersion) value() *DocumentVersion {
	if !version.ID.Valid {
		return nil
	}
	return &DocumentVersion{ID: version.ID.String, SourceVersion: version.SourceVersion.Int64,
		Status: version.Status.String, ErrorCode: version.ErrorCode.String,
		File: DocumentFile{ID: version.FileID.String, Name: version.FileName.String,
			MIMEType: version.MIMEType.String, ByteSize: version.ByteSize.Int64},
		CreatedAt: version.CreatedAt.Time.UTC(), UpdatedAt: version.UpdatedAt.Time.UTC()}
}

func scanDocument(scanner rowScanner) (Document, error) {
	var document Document
	var current, pending nullableDocumentVersion
	targets := []any{&document.ID, &document.CollectionID, &document.Status,
		&document.CreatedAt, &document.UpdatedAt}
	targets = append(targets, current.scanTargets()...)
	targets = append(targets, pending.scanTargets()...)
	if err := scanner.Scan(targets...); err != nil {
		return Document{}, err
	}
	document.CreatedAt = document.CreatedAt.UTC()
	document.UpdatedAt = document.UpdatedAt.UTC()
	document.CurrentVersion = current.value()
	document.PendingVersion = pending.value()
	return document, nil
}

func (r *PostgresRepository) CreateDocument(ctx context.Context, input CreateDocumentRepositoryInput) (Document, error) {
	if err := r.requireDB(); err != nil {
		return Document{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Document{}, fmt.Errorf("begin bind document: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	collection, _, err := lockCollectionForManage(ctx, tx, input.CollectionID, input.ActorUserID)
	if err != nil {
		return Document{}, err
	}
	if existing, found, err := findDocumentReplay(ctx, tx, input); err != nil {
		return Document{}, err
	} else if found {
		return existing, nil
	}

	var file DocumentFile
	var hash string
	var purpose sql.NullString
	err = tx.QueryRowContext(ctx, `
SELECT id, original_filename, mime_type, byte_size, sha256, metadata->>'purpose'
FROM files
WHERE id = $1 AND user_id = $2 AND upload_status = 'available' AND deleted_at IS NULL
FOR UPDATE
`, input.FileID, input.ActorUserID).Scan(&file.ID, &file.Name, &file.MIMEType, &file.ByteSize, &hash, &purpose)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, ErrFileNotFound
	}
	if err != nil {
		return Document{}, fmt.Errorf("lock knowledge file: %w", err)
	}
	if !purpose.Valid || purpose.String != "knowledge" {
		return Document{}, ErrFileNotFound
	}

	authority, err := resolveParseAuthority(ctx, tx, input.CollectionID, input.ParseProcessor, file.MIMEType)
	if err != nil {
		return Document{}, err
	}

	var document Document
	err = tx.QueryRowContext(ctx, `
INSERT INTO knowledge_documents (
  id, collection_id, created_by_user_id, idempotency_key, create_request_hash
) VALUES ($1, $2, $3, $4, $5)
RETURNING id, collection_id, status, created_at, updated_at
`, input.DocumentID, input.CollectionID, input.ActorUserID, input.IdempotencyKey, input.RequestHash).
		Scan(&document.ID, &document.CollectionID, &document.Status, &document.CreatedAt, &document.UpdatedAt)
	if err != nil {
		return Document{}, mapDocumentInsertError(err)
	}
	version := DocumentVersion{File: file}
	err = tx.QueryRowContext(ctx, `
INSERT INTO knowledge_document_versions (
  id, document_id, file_id, source_version, content_hash,
  created_by_user_id, idempotency_key, request_hash
) VALUES ($1, $2, $3, 1, $4, $5, $6, $7)
RETURNING id, source_version, status, created_at, updated_at
`, input.VersionID, input.DocumentID, input.FileID, hash, input.ActorUserID,
		input.IdempotencyKey, input.RequestHash).Scan(
		&version.ID, &version.SourceVersion, &version.Status, &version.CreatedAt, &version.UpdatedAt)
	if err != nil {
		return Document{}, fmt.Errorf("insert document version: %w", err)
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
  $1,$2,$3,$4,$5,'parse','initial',$6,$7,$8,$9,$10,$11,$12,
  $13,$14,$15,1,$16,$17,$18,$19
)
`, input.JobID, input.CollectionID, input.DocumentID, input.VersionID, input.FileID,
		input.ParseProcessor, authority.EndpointID, authority.ProfileID,
		authority.GovernanceRevision, authority.HeadRevision,
		authority.ConsentID, authority.ConsentRevision, collection.ACLRevision, collection.VisibilityEpoch,
		collection.ProcessingRevision, input.ActorUserID,
		documentOperationIdempotencyScope(input.DocumentID, "initial", input.ActorUserID),
		input.IdempotencyKey, input.RequestHash)
	if err != nil {
		return Document{}, fmt.Errorf("insert parse job: %w", err)
	}
	if err := r.insertDocumentOutbox(
		ctx, tx, document, version, input.JobID, hash, "initial",
		authority, collection, 1,
	); err != nil {
		return Document{}, err
	}
	if err := tx.Commit(); err != nil {
		return Document{}, fmt.Errorf("commit bind document: %w", err)
	}
	return document, nil
}

func findDocumentReplay(ctx context.Context, tx *sql.Tx, input CreateDocumentRepositoryInput) (Document, bool, error) {
	var document Document
	var storedHash string
	err := tx.QueryRowContext(ctx, `
SELECT id, collection_id, status, create_request_hash, created_at, updated_at
FROM knowledge_documents
WHERE collection_id = $1 AND created_by_user_id = $2 AND idempotency_key = $3
FOR UPDATE
`, input.CollectionID, input.ActorUserID, input.IdempotencyKey).Scan(
		&document.ID, &document.CollectionID, &document.Status, &storedHash,
		&document.CreatedAt, &document.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return document, false, nil
	}
	if err != nil {
		return document, false, fmt.Errorf("check document replay: %w", err)
	}
	if storedHash != input.RequestHash {
		return Document{}, false, ErrIdempotencyConflict
	}
	version, err := queryPendingVersion(ctx, tx, document.ID)
	if err != nil {
		return Document{}, false, err
	}
	document.PendingVersion = &version
	return document, true, nil
}

func queryPendingVersion(ctx context.Context, tx *sql.Tx, documentID string) (DocumentVersion, error) {
	var version DocumentVersion
	err := tx.QueryRowContext(ctx, `
SELECT v.id, v.source_version, v.status, COALESCE(v.error_code,''),
  f.id, f.original_filename, f.mime_type, f.byte_size, v.created_at, v.updated_at
FROM knowledge_document_versions v JOIN files f ON f.id = v.file_id
WHERE v.document_id = $1 AND v.status IN ('uploaded','processing','failed')
ORDER BY v.source_version DESC LIMIT 1
`, documentID).Scan(&version.ID, &version.SourceVersion, &version.Status, &version.ErrorCode,
		&version.File.ID, &version.File.Name, &version.File.MIMEType, &version.File.ByteSize,
		&version.CreatedAt, &version.UpdatedAt)
	if err != nil {
		return version, fmt.Errorf("query pending version: %w", err)
	}
	return version, nil
}

func queryVersionByID(ctx context.Context, tx *sql.Tx, documentID, versionID string) (*DocumentVersion, error) {
	var version DocumentVersion
	err := tx.QueryRowContext(ctx, `
SELECT v.id, v.source_version, v.status, COALESCE(v.error_code,''),
  f.id, f.original_filename, f.mime_type, f.byte_size, v.created_at, v.updated_at
FROM knowledge_document_versions v JOIN files f ON f.id = v.file_id
WHERE v.document_id = $1 AND v.id = $2
`, documentID, versionID).Scan(&version.ID, &version.SourceVersion, &version.Status,
		&version.ErrorCode, &version.File.ID, &version.File.Name, &version.File.MIMEType,
		&version.File.ByteSize, &version.CreatedAt, &version.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("query document version: %w", err)
	}
	return &version, nil
}

func (r *PostgresRepository) insertDocumentOutbox(
	ctx context.Context,
	tx *sql.Tx,
	document Document,
	version DocumentVersion,
	jobID, hash, operation string,
	authority parseAuthority,
	collection collectionRow,
	documentVisibilityEpoch int64,
) error {
	eventID, err := r.newEventID()
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{
		"schemaVersion": 1, "collectionId": document.CollectionID,
		"documentId": document.ID, "documentVersionId": version.ID,
		"sourceVersion": version.SourceVersion, "fileId": version.File.ID,
		"contentHash": hash, "jobId": jobID, "operation": operation,
		"processor": authority.Processor, "endpointId": authority.EndpointID,
		"governanceProfileId":          authority.ProfileID,
		"governanceRevision":           authority.GovernanceRevision,
		"governanceHeadRevision":       authority.HeadRevision,
		"collectionConsentId":          authority.ConsentID,
		"collectionConsentRevision":    authority.ConsentRevision,
		"collectionAclRevision":        collection.ACLRevision,
		"collectionVisibilityEpoch":    collection.VisibilityEpoch,
		"collectionProcessingRevision": collection.ProcessingRevision,
		"documentVisibilityEpoch":      documentVisibilityEpoch,
	})
	_, err = tx.ExecContext(ctx, `INSERT INTO knowledge_outbox
(event_id,aggregate_type,aggregate_key,event_type,payload)
VALUES ($1,'knowledge_document',$2,'knowledge.document.version.requested',$3::jsonb)`, eventID, document.ID, string(payload))
	if err != nil {
		return fmt.Errorf("insert document outbox: %w", err)
	}
	return nil
}

func mapDocumentInsertError(err error) error {
	if isConstraint(err, "idx_knowledge_documents_collection_creator_idempotency") {
		return ErrIdempotencyConflict
	}
	return fmt.Errorf("insert document: %w", err)
}
