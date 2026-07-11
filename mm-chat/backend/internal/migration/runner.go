package migration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const migrationAdvisoryLockID int64 = 0x4d4d43484154

const createVersionTableSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version BIGINT PRIMARY KEY,
  name TEXT NOT NULL,
  checksum TEXT,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE schema_migrations ADD COLUMN IF NOT EXISTS checksum TEXT;
`

var migrationFileRE = regexp.MustCompile(`^([0-9]+)_([A-Za-z0-9][A-Za-z0-9_-]*)\.(up|down)\.sql$`)

// Migration represents one paired up/down SQL migration.
type Migration struct {
	Version     int64
	VersionText string
	Name        string
	UpPath      string
	DownPath    string
	Checksum    string
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

type migrationStore interface {
	execer
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// NewRunner creates a migration runner using the supplied DB pool and SQL file
// system.
func NewRunner(db *sql.DB, files fs.FS) *Runner {
	return &Runner{db: db, files: files}
}

// Up applies every unapplied migration in ascending version order.
func (r *Runner) Up(ctx context.Context) (changed []Migration, err error) {
	if err := r.requireReady(); err != nil {
		return nil, err
	}
	conn, release, err := r.lockedConnection(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, release()) }()
	return r.up(ctx, conn)
}

func (r *Runner) up(ctx context.Context, store migrationStore) ([]Migration, error) {
	if err := r.ensureVersionTable(ctx, store); err != nil {
		return nil, err
	}

	migrations, err := Load(r.files)
	if err != nil {
		return nil, err
	}
	applied, legacy, err := r.appliedVersions(ctx, store, migrations)
	if err != nil {
		return nil, err
	}
	if len(legacy) != 0 {
		return nil, fmt.Errorf("legacy migration %s has no checksum; run migrate baseline after verifying the migration source", legacy[0].ID())
	}

	var changed []Migration
	for _, m := range migrations {
		if applied[m.Version] {
			continue
		}
		if err := r.applyUp(ctx, store, m); err != nil {
			return changed, err
		}
		changed = append(changed, m)
	}

	return changed, nil
}

// Down rolls back the latest applied migration, or all applied migrations when
// all is true. The schema_migrations table is retained; version rows are
// deleted by the runner after each successful rollback.
func (r *Runner) Down(ctx context.Context, all bool) (changed []Migration, err error) {
	if err := r.requireReady(); err != nil {
		return nil, err
	}
	conn, release, err := r.lockedConnection(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, release()) }()
	return r.down(ctx, conn, all)
}

func (r *Runner) down(ctx context.Context, store migrationStore, all bool) ([]Migration, error) {
	if err := r.ensureVersionTable(ctx, store); err != nil {
		return nil, err
	}

	migrations, err := Load(r.files)
	if err != nil {
		return nil, err
	}
	applied, legacy, err := r.appliedVersions(ctx, store, migrations)
	if err != nil {
		return nil, err
	}
	if len(legacy) != 0 {
		return nil, fmt.Errorf("legacy migration %s has no checksum; run migrate baseline after verifying the migration source", legacy[0].ID())
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
		if err := r.applyDown(ctx, store, m); err != nil {
			return changed, err
		}
		changed = append(changed, m)
	}

	return changed, nil
}

// BaselineLegacyChecksums explicitly accepts the currently embedded migration
// source for applied legacy rows that predate checksum tracking.
func (r *Runner) BaselineLegacyChecksums(ctx context.Context) (changed []Migration, err error) {
	if err := r.requireReady(); err != nil {
		return nil, err
	}
	conn, release, err := r.lockedConnection(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, release()) }()
	if err := r.ensureVersionTable(ctx, conn); err != nil {
		return nil, err
	}
	migrations, err := Load(r.files)
	if err != nil {
		return nil, err
	}
	_, legacy, err := r.appliedVersions(ctx, conn, migrations)
	if err != nil {
		return nil, err
	}
	for _, migration := range legacy {
		result, updateErr := conn.ExecContext(
			ctx,
			`UPDATE schema_migrations SET checksum = $2 WHERE version = $1 AND checksum IS NULL`,
			migration.Version,
			migration.Checksum,
		)
		if updateErr != nil {
			return changed, fmt.Errorf("baseline legacy migration %s checksum: %w", migration.ID(), updateErr)
		}
		rows, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return changed, fmt.Errorf("read baseline result for migration %s: %w", migration.ID(), rowsErr)
		}
		if rows != 1 {
			return changed, fmt.Errorf("baseline legacy migration %s changed %d rows, want 1", migration.ID(), rows)
		}
		changed = append(changed, migration)
	}
	if _, remaining, err := r.appliedVersions(ctx, conn, migrations); err != nil {
		return changed, err
	} else if len(remaining) != 0 {
		return changed, fmt.Errorf("legacy migration %s still has no checksum after baseline", remaining[0].ID())
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
		up, err := fs.ReadFile(files, m.UpPath)
		if err != nil {
			return nil, fmt.Errorf("read migration %s up checksum: %w", m.ID(), err)
		}
		down, err := fs.ReadFile(files, m.DownPath)
		if err != nil {
			return nil, fmt.Errorf("read migration %s down checksum: %w", m.ID(), err)
		}
		hash := sha256.New()
		_, _ = hash.Write([]byte(m.ID()))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(up)
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(down)
		m.Checksum = fmt.Sprintf("%x", hash.Sum(nil))
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

func (r *Runner) lockedConnection(ctx context.Context) (*sql.Conn, func() error, error) {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("open migration lock connection: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationAdvisoryLockID); err != nil {
		discardMigrationConnection(conn)
		return nil, nil, fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	release := func() error {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var unlocked bool
		unlockErr := conn.QueryRowContext(
			unlockCtx,
			`SELECT pg_advisory_unlock($1)`,
			migrationAdvisoryLockID,
		).Scan(&unlocked)
		if unlockErr != nil {
			discardMigrationConnection(conn)
			return fmt.Errorf("release migration advisory lock: %w", unlockErr)
		}
		if !unlocked {
			discardMigrationConnection(conn)
			return errors.New("release migration advisory lock: lock was not held")
		}
		closeErr := conn.Close()
		if closeErr != nil {
			return fmt.Errorf("close migration lock connection: %w", closeErr)
		}
		return nil
	}
	return conn, release, nil
}

func discardMigrationConnection(conn *sql.Conn) {
	if conn == nil {
		return
	}
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = conn.Close()
}

func (r *Runner) ensureVersionTable(ctx context.Context, store execer) error {
	if _, err := store.ExecContext(ctx, createVersionTableSQL); err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}

	return nil
}

func (r *Runner) appliedVersions(
	ctx context.Context,
	store migrationStore,
	migrations []Migration,
) (map[int64]bool, []Migration, error) {
	rows, err := store.QueryContext(ctx, `SELECT version, name, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, nil, fmt.Errorf("query applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int64]bool)
	known := make(map[int64]Migration, len(migrations))
	for _, migration := range migrations {
		known[migration.Version] = migration
	}
	var legacy []Migration
	for rows.Next() {
		var version int64
		var name string
		var checksum sql.NullString
		if err := rows.Scan(&version, &name, &checksum); err != nil {
			return nil, nil, fmt.Errorf("scan applied migration: %w", err)
		}
		migration, ok := known[version]
		if !ok {
			return nil, nil, fmt.Errorf("applied migration version %d is not embedded", version)
		}
		if name != migration.Name {
			return nil, nil, fmt.Errorf(
				"applied migration %d name mismatch: database=%q embedded=%q",
				version,
				name,
				migration.Name,
			)
		}
		if checksum.Valid && checksum.String != migration.Checksum {
			return nil, nil, fmt.Errorf("applied migration %s checksum mismatch", migration.ID())
		}
		if !checksum.Valid {
			legacy = append(legacy, migration)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate applied migrations: %w", err)
	}

	if err := rows.Close(); err != nil {
		return nil, nil, fmt.Errorf("close applied migrations: %w", err)
	}

	return applied, legacy, nil
}

func (r *Runner) applyUp(ctx context.Context, store migrationStore, m Migration) error {
	tx, err := store.BeginTx(ctx, nil)
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

func (r *Runner) applyDown(ctx context.Context, store migrationStore, m Migration) error {
	tx, err := store.BeginTx(ctx, nil)
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
		`INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)`,
		m.Version,
		m.Name,
		m.Checksum,
	)
	return err
}

func (r *Runner) deleteApplied(ctx context.Context, runner execer, version int64) error {
	_, err := runner.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = $1`, version)
	return err
}
