package usermemory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"neo-chat/mm-chat/backend/internal/auth"
)

type postgresPortabilityExportSnapshot struct {
	tx             *sql.Tx
	userID         string
	includeHistory bool
}

type postgresDeletionExportSnapshot struct {
	tx *sql.Tx
}

func (r *PostgresRepository) WithPortabilityExportSnapshot(
	ctx context.Context,
	includeHistory bool,
	use func(PortabilityExportSnapshot) error,
) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	if use == nil {
		return errors.New("memory portability export callback is required")
	}
	user := auth.UserOrDevelopment(ctx)
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
	if err != nil {
		return fmt.Errorf("begin memory portability export snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	snapshot := postgresPortabilityExportSnapshot{
		tx: tx, userID: user.ID, includeHistory: includeHistory,
	}
	if err := use(snapshot); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit memory portability export snapshot: %w", err)
	}
	return nil
}

func (s postgresPortabilityExportSnapshot) ScanRecords(
	ctx context.Context,
	visit func(any) error,
) (PortableRecordCounts, error) {
	if s.tx == nil || visit == nil {
		return PortableRecordCounts{}, errors.New("memory portability export snapshot is invalid")
	}
	rows, err := s.tx.QueryContext(ctx, `
SELECT record_kind, payload
FROM memory_portability_export_records($1::uuid, $2)
ORDER BY sort_group, sort_key
`, s.userID, s.includeHistory)
	if err != nil {
		return PortableRecordCounts{}, mapPortabilityPostgresError(err)
	}
	defer rows.Close()
	var counts PortableRecordCounts
	for rows.Next() {
		var kind string
		var payload []byte
		if err := rows.Scan(&kind, &payload); err != nil {
			return PortableRecordCounts{}, fmt.Errorf("scan memory portability export record: %w", err)
		}
		record, err := decodePortabilityExportRecord(kind, payload)
		if err != nil {
			return PortableRecordCounts{}, err
		}
		switch record.(type) {
		case PortableSettingsRecord:
			counts.Settings++
		case PortableProjectRecord:
			counts.Projects++
		case PortableMemoryRecord:
			counts.Memories++
		case PortableRevisionRecord:
			counts.Revisions++
		}
		if err := visit(record); err != nil {
			return PortableRecordCounts{}, err
		}
	}
	if err := rows.Err(); err != nil {
		return PortableRecordCounts{}, fmt.Errorf("iterate memory portability export records: %w", err)
	}
	return counts, nil
}

func (r *PostgresRepository) WithDeletionExportSnapshot(
	ctx context.Context,
	use func(DeletionExportSnapshot) error,
) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	if use == nil {
		return errors.New("memory deletion export callback is required")
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
	if err != nil {
		return fmt.Errorf("begin memory deletion export snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := use(postgresDeletionExportSnapshot{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit memory deletion export snapshot: %w", err)
	}
	return nil
}

func (s postgresDeletionExportSnapshot) ScanDeletionEntries(
	ctx context.Context,
	visit func(PortableDeletionEntry) error,
) (int, error) {
	if s.tx == nil || visit == nil {
		return 0, errors.New("memory deletion export snapshot is invalid")
	}
	rows, err := s.tx.QueryContext(ctx, `
SELECT payload
FROM memory_portability_export_deletions()
ORDER BY sort_key
`)
	if err != nil {
		return 0, mapPortabilityPostgresError(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return 0, fmt.Errorf("scan memory deletion export record: %w", err)
		}
		var entry PortableDeletionEntry
		if err := json.Unmarshal(payload, &entry); err != nil {
			return 0, fmt.Errorf("decode memory deletion export record: %w", err)
		}
		if err := validatePortableDeletionEntry(entry); err != nil {
			return 0, err
		}
		count++
		if count > MaxDeletionPackageEntries {
			return 0, validation(
				"MEMORY_DELETION_PACKAGE_COUNT_LIMIT",
				"deletion package entry count exceeds the limit",
			)
		}
		if err := visit(entry); err != nil {
			return 0, err
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate memory deletion export records: %w", err)
	}
	return count, nil
}

func decodePortabilityExportRecord(kind string, payload []byte) (any, error) {
	var target any
	switch kind {
	case "settings":
		target = &PortableSettingsRecord{}
	case "project":
		target = &PortableProjectRecord{}
	case "memory":
		target = &PortableMemoryRecord{}
	case "revision":
		target = &PortableRevisionRecord{}
	default:
		return nil, errors.New("database returned an unsupported memory portability record kind")
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return nil, fmt.Errorf("decode memory portability export record: %w", err)
	}
	switch value := target.(type) {
	case *PortableSettingsRecord:
		return *value, nil
	case *PortableProjectRecord:
		return *value, nil
	case *PortableMemoryRecord:
		return *value, nil
	case *PortableRevisionRecord:
		return *value, nil
	default:
		return nil, errors.New("memory portability export record decode failed")
	}
}

func (r *PostgresRepository) PortabilityAuthorityState(ctx context.Context) (string, error) {
	if err := r.requireDB(); err != nil {
		return "", err
	}
	user := auth.UserOrDevelopment(ctx)
	var state string
	if err := r.db.QueryRowContext(ctx, `
SELECT memory_portability_authority_state($1::uuid)
`, user.ID).Scan(&state); err != nil {
		return "", mapPortabilityPostgresError(err)
	}
	return state, nil
}

func (r *PostgresRepository) ResolveImportProject(
	ctx context.Context,
	projectID string,
) (MemoryProject, error) {
	user := auth.UserOrDevelopment(ctx)
	return queryPortabilityJSON[MemoryProject](ctx, r.db, `
SELECT memory_portability_resolve_project($1::uuid, $2::uuid)
`, user.ID, projectID)
}

func (r *PostgresRepository) ResolveImportConversation(
	ctx context.Context,
	conversationID string,
) (ConversationMemoryPolicy, error) {
	user := auth.UserOrDevelopment(ctx)
	return queryPortabilityJSON[ConversationMemoryPolicy](ctx, r.db, `
SELECT memory_portability_resolve_conversation($1::uuid, $2::uuid)
`, user.ID, conversationID)
}

func (r *PostgresRepository) ResolveImportMemory(
	ctx context.Context,
	input ImportMemoryResolutionInput,
) (ImportMemoryResolution, error) {
	user := auth.UserOrDevelopment(ctx)
	return queryPortabilityJSON[ImportMemoryResolution](ctx, r.db, `
SELECT memory_portability_resolve_memory(
  $1::uuid, $2, $3, $4, $5,
  NULLIF($6, '')::uuid, NULLIF($7, '')::uuid, $8
)
`, user.ID, input.NormalizedContent, input.SubjectKey, input.FactKey,
		input.Scope.Type, input.Scope.ProjectID, input.Scope.ConversationID,
		input.Scope.ScopeGeneration)
}

func (r *PostgresRepository) CompletedPortabilityImport(
	ctx context.Context,
	metadata PortabilityApplyMetadata,
) (PortabilityApplyResult, bool, error) {
	if err := r.requireDB(); err != nil {
		return PortabilityApplyResult{}, false, err
	}
	user := auth.UserOrDevelopment(ctx)
	var payload []byte
	if err := r.db.QueryRowContext(ctx, `
SELECT memory_portability_completed_import(
  $1::uuid, $2::uuid, $3, $4, $5, $6, $7
)
`, user.ID, metadata.ImportID, metadata.PackageSHA256, metadata.ManifestSHA256,
		metadata.MappingsSHA256, metadata.PlanSHA256,
		metadata.AuthorityStateHash).Scan(&payload); err != nil {
		return PortabilityApplyResult{}, false, mapPortabilityPostgresError(err)
	}
	if len(payload) == 0 || string(payload) == "null" {
		return PortabilityApplyResult{}, false, nil
	}
	var result PortabilityApplyResult
	if err := json.Unmarshal(payload, &result); err != nil {
		return PortabilityApplyResult{}, false,
			fmt.Errorf("decode completed memory portability import: %w", err)
	}
	return result, true, nil
}

func (r *PostgresRepository) ApplyPortabilityImport(
	ctx context.Context,
	metadata PortabilityApplyMetadata,
	apply func(PortabilityApplySink) error,
) (PortabilityApplyResult, error) {
	if err := r.requireDB(); err != nil {
		return PortabilityApplyResult{}, err
	}
	if apply == nil {
		return PortabilityApplyResult{}, errors.New("memory portability apply callback is required")
	}
	user := auth.UserOrDevelopment(ctx)
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return PortabilityApplyResult{}, fmt.Errorf("begin memory portability import: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	begin, err := queryPortabilityJSON[PortabilityApplyResult](ctx, tx, `
SELECT memory_portability_begin_import(
  $1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10
)
`, user.ID, metadata.ImportID, metadata.PackageSHA256, metadata.ManifestSHA256,
		metadata.MappingsSHA256, metadata.PlanSHA256, metadata.AuthorityStateHash,
		metadata.ProjectCount, metadata.MemoryCount, metadata.RevisionCount)
	if err != nil {
		return PortabilityApplyResult{}, err
	}
	if begin.Status == "completed" {
		if err := tx.Commit(); err != nil {
			return PortabilityApplyResult{}, fmt.Errorf("commit replayed memory import: %w", err)
		}
		return begin, nil
	}
	sink := &postgresPortabilityApplySink{
		ctx: ctx, tx: tx, userID: user.ID, importID: metadata.ImportID,
	}
	if err := apply(sink); err != nil {
		return PortabilityApplyResult{}, err
	}
	result, err := queryPortabilityJSON[PortabilityApplyResult](ctx, tx, `
SELECT memory_portability_complete_import($1::uuid, $2::uuid, $3, $4)
`, user.ID, metadata.ImportID, sink.addedProjects, sink.addedMemories)
	if err != nil {
		return PortabilityApplyResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return PortabilityApplyResult{}, mapPortabilityPostgresError(err)
	}
	return result, nil
}

func (r *PostgresRepository) ReplayDeletionEntries(
	ctx context.Context,
	stream func(func(PortableDeletionEntry) error) error,
) (DeletionReplayResult, error) {
	if err := r.requireDB(); err != nil {
		return DeletionReplayResult{}, err
	}
	if stream == nil {
		return DeletionReplayResult{}, errors.New("memory deletion replay stream is required")
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return DeletionReplayResult{}, fmt.Errorf("begin memory deletion replay: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result := DeletionReplayResult{ResultCounts: make(map[string]int, 4)}
	if err := stream(func(entry PortableDeletionEntry) error {
		payload, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("marshal memory deletion replay entry: %w", err)
		}
		var status string
		if err := tx.QueryRowContext(ctx, `
SELECT memory_portability_replay_deletion($1::jsonb)
`, string(payload)).Scan(&status); err != nil {
			return mapPortabilityPostgresError(err)
		}
		switch status {
		case "REPLAYED":
			result.Replayed++
		case "ALREADY_APPLIED":
			result.AlreadyApplied++
		case "NOT_FOUND":
			result.NotFound++
		case "HASH_MISMATCH":
			result.HashMismatch++
		default:
			return errors.New("database returned an unsupported memory deletion replay result")
		}
		result.Entries++
		result.ResultCounts[status]++
		return nil
	}); err != nil {
		return DeletionReplayResult{}, err
	}
	if err := tx.QueryRowContext(ctx, `
SELECT memory_portability_rebuild_projections()
`).Scan(&result.ProjectionRebuilt); err != nil {
		return DeletionReplayResult{}, mapPortabilityPostgresError(err)
	}
	if err := tx.Commit(); err != nil {
		return DeletionReplayResult{}, mapPortabilityPostgresError(err)
	}
	return result, nil
}

type postgresPortabilityApplySink struct {
	ctx           context.Context
	tx            *sql.Tx
	userID        string
	importID      string
	addedProjects int
	addedMemories int
}

func (s *postgresPortabilityApplySink) CreateProject(input PortabilityApplyProject) error {
	_, err := s.tx.ExecContext(s.ctx, `
SELECT memory_portability_create_project(
  $1::uuid, $2::uuid, $3::uuid, $4, $5, $6
)
`, s.userID, s.importID, input.ID, input.Name, input.Description, input.LifecycleStatus)
	if err != nil {
		return mapPortabilityPostgresError(err)
	}
	s.addedProjects++
	return nil
}

func (s *postgresPortabilityApplySink) AddMemory(input PortabilityApplyMemory) error {
	payload, err := json.Marshal(input.Record)
	if err != nil {
		return fmt.Errorf("marshal imported memory: %w", err)
	}
	_, err = s.tx.ExecContext(s.ctx, `
SELECT memory_portability_add_memory(
  $1::uuid, $2::uuid, $3::uuid, $4::jsonb, $5, $6,
  NULLIF($7, '')::uuid, NULLIF($8, '')::uuid, $9
)
`, s.userID, s.importID, input.ID, string(payload),
		normalizeSearchText(input.Record.Content), input.Scope.Type,
		input.Scope.ProjectID, input.Scope.ConversationID, input.Scope.ScopeGeneration)
	if err != nil {
		return mapPortabilityPostgresError(err)
	}
	s.addedMemories++
	return nil
}

func (s *postgresPortabilityApplySink) AddRevision(input PortabilityApplyRevision) error {
	payload, err := json.Marshal(input.Record)
	if err != nil {
		return fmt.Errorf("marshal imported memory revision: %w", err)
	}
	_, err = s.tx.ExecContext(s.ctx, `
SELECT memory_portability_add_revision(
	$1::uuid, $2::uuid, $3::uuid, $4::jsonb, $5,
	NULLIF($6, '')::uuid, NULLIF($7, '')::uuid, $8,
	NULLIF($9, '')::uuid
)
`, s.userID, s.importID, input.MemoryID, string(payload), input.Scope.Type,
		input.Scope.ProjectID, input.Scope.ConversationID, input.Scope.ScopeGeneration,
		input.SupersededByMemoryID)
	if err != nil {
		return mapPortabilityPostgresError(err)
	}
	return nil
}

func (s *postgresPortabilityApplySink) FinalizeMemory(input PortabilityApplyFinalState) error {
	_, err := s.tx.ExecContext(s.ctx, `
SELECT memory_portability_finalize_memory(
  $1::uuid, $2::uuid, $3::uuid, $4, NULLIF($5, '')::uuid
)
`, s.userID, s.importID, input.MemoryID, input.LifecycleStatus,
		input.SupersededByMemoryID)
	if err != nil {
		return mapPortabilityPostgresError(err)
	}
	return nil
}

type portabilityQueryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func queryPortabilityJSON[T any](
	ctx context.Context,
	db portabilityQueryRower,
	query string,
	args ...any,
) (T, error) {
	var zero T
	if db == nil {
		return zero, ErrDatabaseRequired
	}
	var payload []byte
	if err := db.QueryRowContext(ctx, query, args...).Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return zero, ErrMemoryNotFound
		}
		return zero, mapPortabilityPostgresError(err)
	}
	if len(payload) == 0 || string(payload) == "null" {
		return zero, ErrMemoryNotFound
	}
	var result T
	if err := json.Unmarshal(payload, &result); err != nil {
		return zero, fmt.Errorf("decode memory portability response: %w", err)
	}
	return result, nil
}

func mapPortabilityPostgresError(err error) error {
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		return fmt.Errorf("memory portability query: %w", err)
	}
	switch strings.TrimSpace(postgresError.Message) {
	case "MEMORY_IMPORT_STATE_STALE", "MEMORY_IMPORT_SCOPE_STALE",
		"MEMORY_IMPORT_REPLAY_CONFLICT":
		return importPlanStaleError()
	case "MEMORY_IMPORT_PROJECT_LIMIT":
		return validation("MEMORY_IMPORT_PROJECT_LIMIT", "memory import exceeds the local Project limit")
	case "MEMORY_IMPORT_PROJECT_INVALID", "MEMORY_IMPORT_MEMORY_INVALID",
		"MEMORY_IMPORT_REVISION_INVALID", "MEMORY_IMPORT_FINAL_STATE_INVALID",
		"MEMORY_IMPORT_BATCH_INVALID":
		return validation(postgresError.Message, "memory import payload is invalid")
	case "MEMORY_DELETION_REPLAY_ENTRY_INVALID":
		return validation(postgresError.Message, "memory deletion replay entry is invalid")
	case "MEMORY_DELETION_REPLAY_CONFLICT":
		return validation(postgresError.Message, "memory deletion replay authority conflicts")
	default:
		if postgresError.Code == "23505" || postgresError.Code == "40001" {
			return importPlanStaleError()
		}
		return fmt.Errorf("memory portability query: %w", err)
	}
}

var _ PortabilityRepository = (*PostgresRepository)(nil)
var _ PortabilityExportRepository = (*PostgresRepository)(nil)
var _ PortabilityExportSnapshot = postgresPortabilityExportSnapshot{}
var _ PortabilityApplySink = (*postgresPortabilityApplySink)(nil)
var _ DeletionPortabilityRepository = (*PostgresRepository)(nil)
var _ DeletionExportSnapshot = postgresDeletionExportSnapshot{}
