package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const createVersionTableSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version BIGINT PRIMARY KEY,
  name TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

var migrationFileRE = regexp.MustCompile(`^([0-9]+)_([A-Za-z0-9][A-Za-z0-9_-]*)\.(up|down)\.sql$`)

// Migration represents one paired up/down SQL migration.
type Migration struct {
	Version     int64
	VersionText string
	Name        string
	UpPath      string
	DownPath    string
}

// ID returns the canonical versioned migration name without direction suffix.
func (m Migration) ID() string {
	return m.VersionText + "_" + m.Name
}

// Runner applies embedded SQL migrations against Postgres.
type Runner struct {
	db    *sql.DB
	files fs.FS
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// NewRunner creates a migration runner using the supplied DB pool and SQL file
// system.
func NewRunner(db *sql.DB, files fs.FS) *Runner {
	return &Runner{db: db, files: files}
}

// Up applies every unapplied migration in ascending version order.
func (r *Runner) Up(ctx context.Context) ([]Migration, error) {
	if err := r.requireReady(); err != nil {
		return nil, err
	}
	if err := r.ensureVersionTable(ctx); err != nil {
		return nil, err
	}

	migrations, err := Load(r.files)
	if err != nil {
		return nil, err
	}
	applied, err := r.appliedVersions(ctx)
	if err != nil {
		return nil, err
	}

	var changed []Migration
	for _, m := range migrations {
		if applied[m.Version] {
			continue
		}
		if err := r.applyUp(ctx, m); err != nil {
			return changed, err
		}
		changed = append(changed, m)
	}

	return changed, nil
}

// Down rolls back the latest applied migration, or all applied migrations when
// all is true. The schema_migrations table is retained; version rows are
// deleted by the runner after each successful rollback.
func (r *Runner) Down(ctx context.Context, all bool) ([]Migration, error) {
	if err := r.requireReady(); err != nil {
		return nil, err
	}
	if err := r.ensureVersionTable(ctx); err != nil {
		return nil, err
	}

	migrations, err := Load(r.files)
	if err != nil {
		return nil, err
	}
	applied, err := r.appliedVersions(ctx)
	if err != nil {
		return nil, err
	}

	var targets []Migration
	for i := len(migrations) - 1; i >= 0; i-- {
		m := migrations[i]
		if !applied[m.Version] {
			continue
		}
		targets = append(targets, m)
		if !all {
			break
		}
	}

	var changed []Migration
	for _, m := range targets {
		if err := r.applyDown(ctx, m); err != nil {
			return changed, err
		}
		changed = append(changed, m)
	}

	return changed, nil
}

// Load parses paired migration files from files and returns them ordered by
// numeric version.
func Load(files fs.FS) ([]Migration, error) {
	if files == nil {
		return nil, errors.New("migration files filesystem is required")
	}

	paths, err := fs.Glob(files, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("glob migration files: %w", err)
	}
	if len(paths) == 0 {
		return nil, errors.New("no migration files found")
	}

	byVersion := make(map[int64]Migration)
	for _, filePath := range paths {
		parsed, direction, err := parseMigrationFilename(filePath)
		if err != nil {
			return nil, err
		}

		existing, ok := byVersion[parsed.Version]
		if ok && (existing.VersionText != parsed.VersionText || existing.Name != parsed.Name) {
			return nil, fmt.Errorf(
				"migration version %d has conflicting names %q and %q",
				parsed.Version,
				existing.ID(),
				parsed.ID(),
			)
		}
		if !ok {
			existing = parsed
		}

		switch direction {
		case "up":
			if existing.UpPath != "" {
				return nil, fmt.Errorf("duplicate up migration for %s", parsed.ID())
			}
			existing.UpPath = filePath
		case "down":
			if existing.DownPath != "" {
				return nil, fmt.Errorf("duplicate down migration for %s", parsed.ID())
			}
			existing.DownPath = filePath
		default:
			return nil, fmt.Errorf("unsupported migration direction %q", direction)
		}

		byVersion[parsed.Version] = existing
	}

	migrations := make([]Migration, 0, len(byVersion))
	for _, m := range byVersion {
		if m.UpPath == "" || m.DownPath == "" {
			return nil, fmt.Errorf("migration %s must include both up and down files", m.ID())
		}
		migrations = append(migrations, m)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

func parseMigrationFilename(filePath string) (Migration, string, error) {
	filename := path.Base(filePath)
	matches := migrationFileRE.FindStringSubmatch(filename)
	if matches == nil {
		return Migration{}, "", fmt.Errorf("invalid migration filename %q", filePath)
	}

	version, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil || version <= 0 {
		return Migration{}, "", fmt.Errorf("invalid migration version in %q", filePath)
	}

	return Migration{
		Version:     version,
		VersionText: matches[1],
		Name:        matches[2],
	}, matches[3], nil
}

func (r *Runner) requireReady() error {
	if r == nil || r.db == nil {
		return errors.New("database is required")
	}
	if r.files == nil {
		return errors.New("migration files filesystem is required")
	}

	return nil
}

func (r *Runner) ensureVersionTable(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, createVersionTableSQL); err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}

	return nil
}

func (r *Runner) appliedVersions(ctx context.Context) (map[int64]bool, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("query applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int64]bool)
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migrations: %w", err)
	}

	return applied, nil
}

func (r *Runner) applyUp(ctx context.Context, m Migration) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", m.ID(), err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := r.execSQLFile(ctx, tx, m.UpPath); err != nil {
		return fmt.Errorf("apply migration %s: %w", m.ID(), err)
	}
	if err := r.recordApplied(ctx, tx, m); err != nil {
		return fmt.Errorf("record migration %s: %w", m.ID(), err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", m.ID(), err)
	}

	return nil
}

func (r *Runner) applyDown(ctx context.Context, m Migration) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin rollback %s: %w", m.ID(), err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := r.execSQLFile(ctx, tx, m.DownPath); err != nil {
		return fmt.Errorf("rollback migration %s: %w", m.ID(), err)
	}
	if err := r.deleteApplied(ctx, tx, m.Version); err != nil {
		return fmt.Errorf("delete migration record %s: %w", m.ID(), err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit rollback %s: %w", m.ID(), err)
	}

	return nil
}

func (r *Runner) execSQLFile(ctx context.Context, runner execer, filePath string) error {
	contents, err := fs.ReadFile(r.files, filePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", filePath, err)
	}

	script := strings.TrimSpace(string(contents))
	if script == "" {
		return fmt.Errorf("migration file %s is empty", filePath)
	}

	if _, err := runner.ExecContext(ctx, script); err != nil {
		return fmt.Errorf("execute %s: %w", filePath, err)
	}

	return nil
}

func (r *Runner) recordApplied(ctx context.Context, runner execer, m Migration) error {
	_, err := runner.ExecContext(
		ctx,
		`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`,
		m.Version,
		m.Name,
	)
	return err
}

func (r *Runner) deleteApplied(ctx context.Context, runner execer, version int64) error {
	_, err := runner.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = $1`, version)
	return err
}
