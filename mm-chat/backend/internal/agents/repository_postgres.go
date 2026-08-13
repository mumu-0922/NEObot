package agents

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct {
	db    *sql.DB
	newID func() string
}

var _ Repository = (*PostgresRepository)(nil)

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db, newID: uuid.NewString}
}

func (r *PostgresRepository) ListLibrary(ctx context.Context, userID string) ([]LibraryEntry, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, librarySelect+`
WHERE entry.user_id = $1
ORDER BY entry.updated_at DESC, entry.id DESC
`, userID)
	if err != nil {
		return nil, fmt.Errorf("list assistant library: %w", err)
	}
	defer rows.Close()
	entries := []LibraryEntry{}
	for rows.Next() {
		entry, scanErr := scanLibraryEntry(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan assistant library: %w", scanErr)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate assistant library: %w", err)
	}
	return entries, nil
}

func (r *PostgresRepository) GetLibrary(ctx context.Context, userID, entryID string) (LibraryEntry, error) {
	if err := r.requireDB(); err != nil {
		return LibraryEntry{}, err
	}
	entry, err := scanLibraryEntry(r.db.QueryRowContext(ctx, librarySelect+`
WHERE entry.id = $1 AND entry.user_id = $2
`, entryID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return LibraryEntry{}, ErrLibraryNotFound
	}
	if err != nil {
		return LibraryEntry{}, fmt.Errorf("get assistant library entry: %w", err)
	}
	return entry, nil
}

func (r *PostgresRepository) CreateCustom(ctx context.Context, userID string, snapshot Snapshot) (LibraryEntry, error) {
	return r.insert(ctx, r.newID(), userID, SourceCustom, snapshot)
}

func (r *PostgresRepository) Install(ctx context.Context, userID string, snapshot Snapshot) (LibraryEntry, error) {
	return r.insert(ctx, r.newID(), userID, SourceLobeHub, snapshot)
}

func (r *PostgresRepository) insert(
	ctx context.Context,
	id, userID, source string,
	snapshot Snapshot,
) (LibraryEntry, error) {
	if err := r.requireDB(); err != nil {
		return LibraryEntry{}, err
	}
	tags, err := json.Marshal(snapshot.Tags)
	if err != nil {
		return LibraryEntry{}, fmt.Errorf("marshal assistant tags: %w", err)
	}
	requiredTools, err := json.Marshal(snapshot.RequiredTools)
	if err != nil {
		return LibraryEntry{}, fmt.Errorf("marshal assistant required tools: %w", err)
	}
	var sourceIdentifier any
	if source == SourceLobeHub {
		sourceIdentifier = snapshot.SourceIdentifier
	}
	entry, err := scanLibraryEntry(r.db.QueryRowContext(ctx, libraryInsertReturning, id, userID,
		source, sourceIdentifier, snapshot.Avatar, snapshot.Title, snapshot.Description,
		snapshot.Category, string(tags), snapshot.SystemPrompt, snapshot.Author,
		snapshot.Homepage, snapshot.SourceVersion, snapshot.SourceUpdatedAt,
		string(requiredTools), snapshot.ContentFingerprint))
	if err != nil {
		if isLibraryConflict(err) {
			return LibraryEntry{}, ErrLibraryConflict
		}
		return LibraryEntry{}, fmt.Errorf("create assistant library entry: %w", err)
	}
	return entry, nil
}

func (r *PostgresRepository) UpdateCustom(
	ctx context.Context,
	userID, entryID string,
	expectedRevision int64,
	snapshot Snapshot,
) (LibraryEntry, error) {
	return r.update(ctx, userID, entryID, SourceCustom, expectedRevision, snapshot)
}

func (r *PostgresRepository) UpdateInstalled(
	ctx context.Context,
	userID, entryID string,
	expectedRevision int64,
	snapshot Snapshot,
) (LibraryEntry, error) {
	return r.update(ctx, userID, entryID, SourceLobeHub, expectedRevision, snapshot)
}

func (r *PostgresRepository) update(
	ctx context.Context,
	userID, entryID, source string,
	expectedRevision int64,
	snapshot Snapshot,
) (LibraryEntry, error) {
	if err := r.requireDB(); err != nil {
		return LibraryEntry{}, err
	}
	tags, err := json.Marshal(snapshot.Tags)
	if err != nil {
		return LibraryEntry{}, fmt.Errorf("marshal assistant tags: %w", err)
	}
	requiredTools, err := json.Marshal(snapshot.RequiredTools)
	if err != nil {
		return LibraryEntry{}, fmt.Errorf("marshal assistant required tools: %w", err)
	}
	entry, err := scanLibraryEntry(r.db.QueryRowContext(ctx, librarySelectUpdate+`
UPDATE assistant_library_entries
SET avatar = $5, title = $6, description = $7, category = $8,
	    tags = $9::jsonb, system_prompt = $10, author = $11, homepage = $12,
	    source_version = $13, source_updated_at = $14,
	    required_tools = $15::jsonb, content_fingerprint = $16,
	    revision = revision + 1, updated_at = now()
WHERE id = $1 AND user_id = $2 AND source = $3 AND revision = $4
RETURNING id, source, COALESCE(source_identifier, ''), avatar, title,
  description, category, tags, system_prompt, author, homepage,
	  source_version, source_updated_at, required_tools, content_fingerprint, revision,
  created_at, updated_at
`, entryID, userID, source, expectedRevision, snapshot.Avatar, snapshot.Title,
		snapshot.Description, snapshot.Category, string(tags), snapshot.SystemPrompt,
		snapshot.Author, snapshot.Homepage, snapshot.SourceVersion,
		snapshot.SourceUpdatedAt, string(requiredTools), snapshot.ContentFingerprint))
	if errors.Is(err, sql.ErrNoRows) {
		return LibraryEntry{}, r.classifyMissingOrConflict(ctx, userID, entryID, source)
	}
	if err != nil {
		return LibraryEntry{}, fmt.Errorf("update assistant library entry: %w", err)
	}
	return entry, nil
}

func (r *PostgresRepository) DeleteCustom(ctx context.Context, userID, entryID string, expectedRevision int64) error {
	return r.delete(ctx, userID, entryID, SourceCustom, expectedRevision)
}

func (r *PostgresRepository) Uninstall(ctx context.Context, userID, entryID string, expectedRevision int64) error {
	return r.delete(ctx, userID, entryID, SourceLobeHub, expectedRevision)
}

func (r *PostgresRepository) delete(ctx context.Context, userID, entryID, source string, expectedRevision int64) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `
DELETE FROM assistant_library_entries
WHERE id = $1 AND user_id = $2 AND source = $3 AND revision = $4
`, entryID, userID, source, expectedRevision)
	if err != nil {
		return fmt.Errorf("delete assistant library entry: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted assistant count: %w", err)
	}
	if count == 0 {
		return r.classifyMissingOrConflict(ctx, userID, entryID, source)
	}
	return nil
}

func (r *PostgresRepository) GetAdmission(ctx context.Context, identifier string) (Admission, error) {
	if err := r.requireDB(); err != nil {
		return Admission{}, err
	}
	admission, err := scanAdmission(r.db.QueryRowContext(ctx, admissionSelect+`
WHERE source = 'lobehub' AND source_identifier = $1
`, identifier))
	if errors.Is(err, sql.ErrNoRows) {
		return Admission{}, ErrAdmissionNotFound
	}
	if err != nil {
		return Admission{}, fmt.Errorf("get assistant admission: %w", err)
	}
	return admission, nil
}

func (r *PostgresRepository) ListAdmissions(ctx context.Context, input MarketSearchInput) (MarketSearchResult, error) {
	if err := r.requireDB(); err != nil {
		return MarketSearchResult{}, err
	}
	offset := (input.Page - 1) * input.PageSize
	query := strings.TrimSpace(input.Query)
	category := strings.TrimSpace(input.Category)
	var total int
	if err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*)::int
FROM assistant_market_admissions
WHERE status = 'admitted'
  AND ($1 = '' OR category = $1)
  AND ($2 = '' OR title ILIKE '%' || $2 || '%' OR description ILIKE '%' || $2 || '%'
       OR tags::text ILIKE '%' || $2 || '%')
`, category, query).Scan(&total); err != nil {
		return MarketSearchResult{}, fmt.Errorf("count assistant admissions: %w", err)
	}
	rows, err := r.db.QueryContext(ctx, admissionSelect+`
WHERE status = 'admitted'
  AND ($1 = '' OR category = $1)
  AND ($2 = '' OR title ILIKE '%' || $2 || '%' OR description ILIKE '%' || $2 || '%'
       OR tags::text ILIKE '%' || $2 || '%')
ORDER BY updated_at DESC, source_identifier
LIMIT $3 OFFSET $4
`, category, query, input.PageSize, offset)
	if err != nil {
		return MarketSearchResult{}, fmt.Errorf("list assistant admissions: %w", err)
	}
	defer rows.Close()
	agents := []Agent{}
	for rows.Next() {
		admission, scanErr := scanAdmission(rows)
		if scanErr != nil {
			return MarketSearchResult{}, fmt.Errorf("scan assistant admission: %w", scanErr)
		}
		agents = append(agents, agentFromSnapshot(admission.Snapshot, true))
	}
	if err := rows.Err(); err != nil {
		return MarketSearchResult{}, fmt.Errorf("iterate assistant admissions: %w", err)
	}
	categories, err := r.listAdmissionCategories(ctx)
	if err != nil {
		return MarketSearchResult{}, err
	}
	return MarketSearchResult{Agents: agents, Categories: categories, Page: input.Page,
		PageSize: input.PageSize, TotalCount: total, TotalPages: pageCount(total, input.PageSize),
		Source: "neo-chat-admitted"}, nil
}

func (r *PostgresRepository) listAdmissionCategories(ctx context.Context) ([]MarketCategory, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT category, COUNT(*)::int
FROM assistant_market_admissions
WHERE status = 'admitted'
GROUP BY category ORDER BY category
`)
	if err != nil {
		return nil, fmt.Errorf("list assistant admission categories: %w", err)
	}
	defer rows.Close()
	categories := []MarketCategory{}
	for rows.Next() {
		var category MarketCategory
		if err := rows.Scan(&category.ID, &category.Count); err != nil {
			return nil, fmt.Errorf("scan assistant admission category: %w", err)
		}
		categories = append(categories, category)
	}
	return categories, rows.Err()
}

func (r *PostgresRepository) UpsertAdmission(ctx context.Context, reviewerID string, admission Admission) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	tags, err := json.Marshal(admission.Tags)
	if err != nil {
		return fmt.Errorf("marshal assistant admission tags: %w", err)
	}
	requiredTools, err := json.Marshal(admission.RequiredTools)
	if err != nil {
		return fmt.Errorf("marshal assistant admission required tools: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO assistant_market_admissions (
	  source, source_identifier, status, avatar, title, description, category,
	  tags, system_prompt, author, homepage, source_version, source_updated_at,
	  required_tools, content_fingerprint, reviewed_by_user_id
	) VALUES ('lobehub', $1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10,
	          $11, $12, $13::jsonb, $14, $15)
ON CONFLICT (source, source_identifier) DO UPDATE SET
  status = EXCLUDED.status, avatar = EXCLUDED.avatar, title = EXCLUDED.title,
  description = EXCLUDED.description, category = EXCLUDED.category,
  tags = EXCLUDED.tags, system_prompt = EXCLUDED.system_prompt,
  author = EXCLUDED.author, homepage = EXCLUDED.homepage,
	  source_version = EXCLUDED.source_version,
	  source_updated_at = EXCLUDED.source_updated_at,
	  required_tools = EXCLUDED.required_tools,
	  content_fingerprint = EXCLUDED.content_fingerprint,
  reviewed_by_user_id = EXCLUDED.reviewed_by_user_id, updated_at = now()
`, admission.SourceIdentifier, admission.Status, admission.Avatar, admission.Title,
		admission.Description, admission.Category, string(tags), admission.SystemPrompt,
		admission.Author, admission.Homepage, admission.SourceVersion,
		admission.SourceUpdatedAt, string(requiredTools), admission.ContentFingerprint, reviewerID)
	if err != nil {
		return fmt.Errorf("upsert assistant admission: %w", err)
	}
	return nil
}

func (r *PostgresRepository) classifyMissingOrConflict(ctx context.Context, userID, entryID, source string) error {
	var revision int64
	err := r.db.QueryRowContext(ctx, `
SELECT revision FROM assistant_library_entries
WHERE id = $1 AND user_id = $2 AND source = $3
`, entryID, userID, source).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrLibraryNotFound
	}
	if err != nil {
		return fmt.Errorf("classify assistant write conflict: %w", err)
	}
	return ErrRevisionConflict
}

func (r *PostgresRepository) requireDB() error {
	if r == nil || r.db == nil {
		return ErrRepositoryUnavailable
	}
	return nil
}

func isLibraryConflict(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}

type scanner interface{ Scan(...any) error }

func scanLibraryEntry(row scanner) (LibraryEntry, error) {
	var entry LibraryEntry
	var tags []byte
	var requiredTools []byte
	err := row.Scan(&entry.ID, &entry.Source, &entry.SourceIdentifier, &entry.Avatar,
		&entry.Title, &entry.Description, &entry.Category, &tags, &entry.SystemPrompt,
		&entry.Author, &entry.Homepage, &entry.SourceVersion, &entry.SourceUpdatedAt,
		&requiredTools, &entry.ContentFingerprint, &entry.Revision, &entry.CreatedAt, &entry.UpdatedAt)
	if err != nil {
		return LibraryEntry{}, err
	}
	if err := json.Unmarshal(tags, &entry.Tags); err != nil {
		return LibraryEntry{}, err
	}
	if entry.Tags == nil {
		entry.Tags = []string{}
	}
	if err := json.Unmarshal(requiredTools, &entry.RequiredTools); err != nil {
		return LibraryEntry{}, err
	}
	if entry.RequiredTools == nil {
		entry.RequiredTools = []string{}
	}
	return entry, nil
}

func scanAdmission(row scanner) (Admission, error) {
	var admission Admission
	var tags []byte
	var requiredTools []byte
	err := row.Scan(&admission.SourceIdentifier, &admission.Status, &admission.Avatar,
		&admission.Title, &admission.Description, &admission.Category, &tags,
		&admission.SystemPrompt, &admission.Author, &admission.Homepage,
		&admission.SourceVersion, &admission.SourceUpdatedAt,
		&requiredTools, &admission.ContentFingerprint)
	if err != nil {
		return Admission{}, err
	}
	if err := json.Unmarshal(tags, &admission.Tags); err != nil {
		return Admission{}, err
	}
	if admission.Tags == nil {
		admission.Tags = []string{}
	}
	if err := json.Unmarshal(requiredTools, &admission.RequiredTools); err != nil {
		return Admission{}, err
	}
	if admission.RequiredTools == nil {
		admission.RequiredTools = []string{}
	}
	return admission, nil
}

const librarySelect = `
SELECT entry.id, entry.source, COALESCE(entry.source_identifier, ''), entry.avatar,
	  entry.title, entry.description, entry.category, entry.tags, entry.system_prompt,
	  entry.author, entry.homepage, entry.source_version, entry.source_updated_at,
	  entry.required_tools, entry.content_fingerprint, entry.revision,
	  entry.created_at, entry.updated_at
FROM assistant_library_entries entry
`

const libraryInsertReturning = `
INSERT INTO assistant_library_entries (
  id, user_id, source, source_identifier, avatar, title, description, category,
	  tags, system_prompt, author, homepage, source_version, source_updated_at,
	  required_tools, content_fingerprint
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11, $12,
	          $13, $14, $15::jsonb, $16)
RETURNING id, source, COALESCE(source_identifier, ''), avatar, title,
  description, category, tags, system_prompt, author, homepage,
	  source_version, source_updated_at, required_tools, content_fingerprint, revision,
  created_at, updated_at
`

const librarySelectUpdate = ``

const admissionSelect = `
SELECT source_identifier, status, avatar, title, description, category, tags,
	  system_prompt, author, homepage, source_version, source_updated_at,
	  required_tools, content_fingerprint
FROM assistant_market_admissions
`
