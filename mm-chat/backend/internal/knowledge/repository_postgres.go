package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	eventCollectionCreated    = "knowledge.collection.created"
	eventCollectionUpdated    = "knowledge.collection.updated"
	eventCollectionTombstoned = "knowledge.collection.tombstoned"
)

type PostgresRepository struct {
	db         *sql.DB
	newEventID func() (string, error)
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db, newEventID: newUUID}
}

func (r *PostgresRepository) CreateCollection(ctx context.Context, input CreateCollectionRepositoryInput) (Collection, error) {
	if err := r.requireDB(); err != nil {
		return Collection{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Collection{}, fmt.Errorf("begin create collection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	role := ""
	if input.Scope == ScopePersonal {
		if err := lockActiveUser(ctx, tx, input.ActorUserID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return Collection{}, ErrUnauthenticated
			}
			return Collection{}, fmt.Errorf("lock collection owner: %w", err)
		}
	} else {
		role, err = lockVisibleTeam(ctx, tx, input.TeamID, input.ActorUserID)
		if err != nil {
			return Collection{}, err
		}
		if role != "admin" {
			return Collection{}, ErrTeamAdminRequired
		}
	}

	existing, found, err := findCollectionByIdempotency(ctx, tx, input.ActorUserID, input.IdempotencyKey)
	if err != nil {
		return Collection{}, err
	}
	if found {
		if !existing.collectionRow.RequestHash.Valid ||
			existing.collectionRow.RequestHash.String != input.CreateRequestHash ||
			existing.DeletedAt.Valid {
			return Collection{}, ErrIdempotencyConflict
		}
		return existing.toCollection(role), nil
	}

	ownerID, teamID := any(nil), any(nil)
	if input.Scope == ScopePersonal {
		ownerID = input.ActorUserID
	} else {
		teamID = input.TeamID
	}
	row := collectionRow{}
	err = tx.QueryRowContext(ctx, `
INSERT INTO knowledge_collections (
  id, name, description, icon, color, scope, owner_user_id, team_id,
  created_by_user_id, idempotency_key, create_request_hash
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id, name, description, icon, color, scope, owner_user_id, team_id,
  acl_revision, visibility_epoch, collection_processing_revision,
  create_request_hash, created_at, updated_at, deleted_at
`, input.ID, input.Name, input.Description, input.Icon, input.Color, input.Scope,
		ownerID, teamID, input.ActorUserID, input.IdempotencyKey, input.CreateRequestHash,
	).Scan(row.scanTargets()...)
	if err != nil {
		if isConstraint(err, "idx_knowledge_collections_creator_idempotency") {
			return Collection{}, ErrIdempotencyConflict
		}
		return Collection{}, fmt.Errorf("insert collection: %w", err)
	}
	if err := r.insertCollectionOutbox(ctx, tx, row, eventCollectionCreated); err != nil {
		return Collection{}, err
	}
	if err := tx.Commit(); err != nil {
		if isConstraint(err, "idx_knowledge_collections_creator_idempotency") {
			return Collection{}, ErrIdempotencyConflict
		}
		return Collection{}, fmt.Errorf("commit create collection: %w", err)
	}
	return row.toCollection(role), nil
}

func (r *PostgresRepository) ListCollections(ctx context.Context, input ListCollectionsRepositoryInput) (CollectionPageResult, error) {
	if err := r.requireDB(); err != nil {
		return CollectionPageResult{}, err
	}
	limit := input.Limit
	if limit < 1 || limit > maximumPageLimit {
		limit = defaultPageLimit
	}
	query := `
SELECT c.id, c.name, c.description, c.icon, c.color, c.scope,
  c.owner_user_id, c.team_id, c.acl_revision, c.visibility_epoch,
  c.collection_processing_revision, c.create_request_hash,
  c.created_at, c.updated_at, c.deleted_at,
  CASE WHEN c.scope = 'personal' THEN 'owner' ELSE m.role END AS actor_role
FROM knowledge_collections c
LEFT JOIN team_memberships m
  ON c.scope = 'team'
 AND m.team_id = c.team_id
 AND m.user_id = $1
 AND m.status = 'active'
LEFT JOIN teams t ON t.id = c.team_id AND t.deleted_at IS NULL
WHERE c.deleted_at IS NULL
  AND (
    (c.scope = 'personal' AND c.owner_user_id = $1)
    OR (c.scope = 'team' AND m.user_id IS NOT NULL AND t.id IS NOT NULL)
  )
`
	args := []any{input.ActorUserID}
	if input.Scope != "" {
		query += fmt.Sprintf("  AND c.scope = $%d\n", len(args)+1)
		args = append(args, input.Scope)
	}
	if input.TeamID != "" {
		query += fmt.Sprintf("  AND c.team_id = $%d\n", len(args)+1)
		args = append(args, input.TeamID)
	}
	if input.After != nil {
		query += fmt.Sprintf(`  AND (c.created_at < $%d OR (c.created_at = $%d AND c.id < $%d))
`, len(args)+1, len(args)+1, len(args)+2)
		args = append(args, input.After.CreatedAt.UTC(), input.After.ID)
	}
	query += fmt.Sprintf("ORDER BY c.created_at DESC, c.id DESC\nLIMIT $%d", len(args)+1)
	args = append(args, limit+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return CollectionPageResult{}, fmt.Errorf("list collections: %w", err)
	}
	defer rows.Close()
	items := make([]Collection, 0, limit)
	for rows.Next() {
		row := collectionRow{}
		var role string
		targets := append(row.scanTargets(), &role)
		if err := rows.Scan(targets...); err != nil {
			return CollectionPageResult{}, fmt.Errorf("scan collection: %w", err)
		}
		if len(items) == limit {
			return CollectionPageResult{Items: items, HasMore: true}, nil
		}
		items = append(items, row.toCollection(role))
	}
	if err := rows.Err(); err != nil {
		return CollectionPageResult{}, fmt.Errorf("iterate collections: %w", err)
	}
	return CollectionPageResult{Items: items}, nil
}

func (r *PostgresRepository) GetCollection(ctx context.Context, input CollectionLookupInput) (Collection, error) {
	if err := r.requireDB(); err != nil {
		return Collection{}, err
	}
	row, role, err := queryVisibleCollection(ctx, r.db, input.CollectionID, input.ActorUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Collection{}, ErrCollectionNotFound
		}
		return Collection{}, fmt.Errorf("get collection: %w", err)
	}
	return row.toCollection(role), nil
}

func (r *PostgresRepository) UpdateCollection(ctx context.Context, input UpdateCollectionRepositoryInput) (Collection, error) {
	if err := r.requireDB(); err != nil {
		return Collection{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Collection{}, fmt.Errorf("begin update collection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	row, role, err := lockCollectionForManage(ctx, tx, input.CollectionID, input.ActorUserID)
	if err != nil {
		return Collection{}, err
	}
	name, description, icon, color := row.Name, row.Description, row.Icon, row.Color
	if input.Name != nil {
		name = *input.Name
	}
	if input.Description != nil {
		description = *input.Description
	}
	if input.Icon != nil {
		icon = *input.Icon
	}
	if input.Color != nil {
		color = *input.Color
	}
	if name == row.Name && description == row.Description && icon == row.Icon && color == row.Color {
		return row.toCollection(role), nil
	}
	updated := collectionRow{}
	err = tx.QueryRowContext(ctx, `
UPDATE knowledge_collections
SET name = $2, description = $3, icon = $4, color = $5, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, name, description, icon, color, scope, owner_user_id, team_id,
  acl_revision, visibility_epoch, collection_processing_revision,
  create_request_hash, created_at, updated_at, deleted_at
`, input.CollectionID, name, description, icon, color).Scan(updated.scanTargets()...)
	if err != nil {
		return Collection{}, fmt.Errorf("update collection: %w", err)
	}
	if err := r.insertCollectionOutbox(ctx, tx, updated, eventCollectionUpdated); err != nil {
		return Collection{}, err
	}
	if err := tx.Commit(); err != nil {
		return Collection{}, fmt.Errorf("commit update collection: %w", err)
	}
	return updated.toCollection(role), nil
}

func (r *PostgresRepository) DeleteCollection(ctx context.Context, input DeleteCollectionRepositoryInput) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete collection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	row, _, err := lockCollectionForManageIncludingDeleted(ctx, tx, input.CollectionID, input.ActorUserID)
	if err != nil {
		return err
	}
	if row.DeletedAt.Valid {
		return nil
	}
	for _, query := range []string{
		`SELECT id FROM knowledge_documents WHERE collection_id = $1 ORDER BY id FOR UPDATE`,
		`SELECT v.id FROM knowledge_document_versions v JOIN knowledge_documents d ON d.id = v.document_id WHERE d.collection_id = $1 ORDER BY v.id FOR UPDATE OF v`,
		`SELECT id FROM knowledge_processing_jobs WHERE collection_id = $1 ORDER BY id FOR UPDATE`,
	} {
		if err := lockIDs(ctx, tx, query, input.CollectionID); err != nil {
			return err
		}
	}
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT now()`).Scan(&now); err != nil {
		return fmt.Errorf("read collection deletion time: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE knowledge_processing_jobs
SET status = 'cancelled', lease_owner = NULL, lease_expires_at = NULL,
    completed_at = $2, updated_at = $2
WHERE collection_id = $1 AND status IN ('pending', 'processing')
`, input.CollectionID, now); err != nil {
		return fmt.Errorf("cancel collection jobs: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE knowledge_document_versions v
SET status = 'tombstoned', visibility_epoch = v.visibility_epoch + 1, updated_at = $2
FROM knowledge_documents d
WHERE v.document_id = d.id AND d.collection_id = $1
  AND v.status NOT IN ('tombstoned', 'deleted')
`, input.CollectionID, now); err != nil {
		return fmt.Errorf("tombstone collection versions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE knowledge_documents
SET status = 'tombstoned', visibility_epoch = visibility_epoch + 1,
    deleted_at = $2, updated_at = $2
WHERE collection_id = $1 AND deleted_at IS NULL
`, input.CollectionID, now); err != nil {
		return fmt.Errorf("tombstone collection documents: %w", err)
	}
	deleted := collectionRow{}
	err = tx.QueryRowContext(ctx, `
UPDATE knowledge_collections
SET acl_revision = acl_revision + 1,
    visibility_epoch = visibility_epoch + 1,
    deleted_at = $2, updated_at = $2
WHERE id = $1
RETURNING id, name, description, icon, color, scope, owner_user_id, team_id,
  acl_revision, visibility_epoch, collection_processing_revision,
  create_request_hash, created_at, updated_at, deleted_at
`, input.CollectionID, now).Scan(deleted.scanTargets()...)
	if err != nil {
		return fmt.Errorf("delete collection: %w", err)
	}
	if err := r.insertCollectionOutbox(ctx, tx, deleted, eventCollectionTombstoned); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete collection: %w", err)
	}
	return nil
}

type collectionRow struct {
	ID, Name, Description, Icon, Color, Scope        string
	OwnerID, TeamID                                  sql.NullString
	ACLRevision, VisibilityEpoch, ProcessingRevision int64
	RequestHash                                      sql.NullString
	CreatedAt, UpdatedAt                             time.Time
	DeletedAt                                        sql.NullTime
}

func (row *collectionRow) scanTargets() []any {
	return []any{&row.ID, &row.Name, &row.Description, &row.Icon, &row.Color, &row.Scope,
		&row.OwnerID, &row.TeamID, &row.ACLRevision, &row.VisibilityEpoch,
		&row.ProcessingRevision, &row.RequestHash, &row.CreatedAt, &row.UpdatedAt, &row.DeletedAt}
}

func (row collectionRow) toCollection(role string) Collection {
	manage := row.Scope == ScopePersonal || role == "admin"
	return Collection{ID: row.ID, Name: row.Name, Description: row.Description, Icon: row.Icon,
		Color: row.Color, Scope: row.Scope, TeamID: row.TeamID.String,
		Permissions: Permissions{Read: true, Manage: manage, ManageConsent: manage},
		ACLRevision: row.ACLRevision, VisibilityEpoch: row.VisibilityEpoch,
		CollectionProcessingRevision: row.ProcessingRevision,
		CreatedAt:                    row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC()}
}

type idempotencyRow struct{ collectionRow }

func findCollectionByIdempotency(ctx context.Context, tx *sql.Tx, actorID, key string) (idempotencyRow, bool, error) {
	row := idempotencyRow{}
	err := tx.QueryRowContext(ctx, `
SELECT id, name, description, icon, color, scope, owner_user_id, team_id,
  acl_revision, visibility_epoch, collection_processing_revision,
  create_request_hash, created_at, updated_at, deleted_at
FROM knowledge_collections
WHERE created_by_user_id = $1 AND idempotency_key = $2
FOR UPDATE
`, actorID, key).Scan(row.collectionRow.scanTargets()...)
	if errors.Is(err, sql.ErrNoRows) {
		return row, false, nil
	}
	if err != nil {
		return row, false, fmt.Errorf("check collection idempotency: %w", err)
	}
	return row, true, nil
}

func queryVisibleCollection(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, collectionID, actorID string) (collectionRow, string, error) {
	row := collectionRow{}
	var role string
	err := q.QueryRowContext(ctx, `
SELECT c.id, c.name, c.description, c.icon, c.color, c.scope,
  c.owner_user_id, c.team_id, c.acl_revision, c.visibility_epoch,
  c.collection_processing_revision, c.create_request_hash,
  c.created_at, c.updated_at, c.deleted_at,
  CASE WHEN c.scope = 'personal' THEN 'owner' ELSE m.role END
FROM knowledge_collections c
LEFT JOIN team_memberships m ON m.team_id = c.team_id AND m.user_id = $2 AND m.status = 'active'
LEFT JOIN teams t ON t.id = c.team_id AND t.deleted_at IS NULL
WHERE c.id = $1 AND c.deleted_at IS NULL
  AND ((c.scope = 'personal' AND c.owner_user_id = $2)
    OR (c.scope = 'team' AND m.user_id IS NOT NULL AND t.id IS NOT NULL))
`, collectionID, actorID).Scan(append(row.scanTargets(), &role)...)
	return row, role, err
}

func lockCollectionForManage(ctx context.Context, tx *sql.Tx, collectionID, actorID string) (collectionRow, string, error) {
	row, role, err := lockCollectionForManageIncludingDeleted(ctx, tx, collectionID, actorID)
	if err == nil && row.DeletedAt.Valid {
		return collectionRow{}, "", ErrCollectionNotFound
	}
	return row, role, err
}

func lockCollectionForManageIncludingDeleted(ctx context.Context, tx *sql.Tx, collectionID, actorID string) (collectionRow, string, error) {
	var scope string
	var ownerID, teamID sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT scope, owner_user_id, team_id FROM knowledge_collections WHERE id = $1`, collectionID).Scan(&scope, &ownerID, &teamID)
	if errors.Is(err, sql.ErrNoRows) {
		return collectionRow{}, "", ErrCollectionNotFound
	}
	if err != nil {
		return collectionRow{}, "", fmt.Errorf("resolve collection subject: %w", err)
	}
	role := "owner"
	if scope == ScopePersonal {
		if !ownerID.Valid || ownerID.String != actorID {
			return collectionRow{}, "", ErrCollectionNotFound
		}
	} else {
		role, err = lockVisibleTeam(ctx, tx, teamID.String, actorID)
		if err != nil {
			return collectionRow{}, "", err
		}
	}
	row := collectionRow{}
	err = tx.QueryRowContext(ctx, `
SELECT id, name, description, icon, color, scope, owner_user_id, team_id,
  acl_revision, visibility_epoch, collection_processing_revision,
  create_request_hash, created_at, updated_at, deleted_at
FROM knowledge_collections WHERE id = $1 FOR UPDATE
`, collectionID).Scan(row.scanTargets()...)
	if err != nil {
		return collectionRow{}, "", fmt.Errorf("lock collection: %w", err)
	}
	if scope == ScopeTeam && role != "admin" {
		return collectionRow{}, "", ErrTeamAdminRequired
	}
	return row, role, nil
}

func lockVisibleTeam(ctx context.Context, tx *sql.Tx, teamID, actorID string) (string, error) {
	var id string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM teams WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, teamID).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrCollectionNotFound
		}
		return "", fmt.Errorf("lock collection team: %w", err)
	}
	var role string
	err := tx.QueryRowContext(ctx, `
SELECT m.role FROM team_memberships m
JOIN users u ON u.id = m.user_id
WHERE m.team_id = $1 AND m.user_id = $2 AND m.status = 'active'
  AND u.account_status = 'active' AND u.deleted_at IS NULL
`, teamID, actorID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrCollectionNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolve collection membership: %w", err)
	}
	return role, nil
}

func lockActiveUser(ctx context.Context, tx *sql.Tx, userID string) error {
	var id string
	return tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id = $1 AND account_status = 'active' AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&id)
}

func lockIDs(ctx context.Context, tx *sql.Tx, query string, collectionID string) error {
	rows, err := tx.QueryContext(ctx, query, collectionID)
	if err != nil {
		return fmt.Errorf("lock collection dependents: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan locked dependent: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate locked dependents: %w", err)
	}
	return nil
}

func (r *PostgresRepository) insertCollectionOutbox(ctx context.Context, tx *sql.Tx, row collectionRow, eventType string) error {
	eventID, err := r.newEventID()
	if err != nil {
		return fmt.Errorf("generate collection outbox event id: %w", err)
	}
	payload := map[string]any{
		"schemaVersion": 1, "collectionId": row.ID, "scope": row.Scope,
		"aclRevision": row.ACLRevision, "visibilityEpoch": row.VisibilityEpoch,
		"collectionProcessingRevision": row.ProcessingRevision,
	}
	if row.OwnerID.Valid {
		payload["ownerUserId"] = row.OwnerID.String
	}
	if row.TeamID.Valid {
		payload["teamId"] = row.TeamID.String
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal collection outbox: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO knowledge_outbox (event_id, aggregate_type, aggregate_key, event_type, payload)
VALUES ($1, 'knowledge_collection', $2, $3, $4::jsonb)
`, eventID, row.ID, eventType, string(encoded))
	if err != nil {
		return fmt.Errorf("insert collection outbox: %w", err)
	}
	return nil
}

func (r *PostgresRepository) requireDB() error {
	if r == nil || r.db == nil {
		return ErrDatabaseRequired
	}
	return nil
}

func isConstraint(err error, name string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == name
}

var _ Repository = (*PostgresRepository)(nil)
