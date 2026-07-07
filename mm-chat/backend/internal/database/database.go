package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/config"
)

// DB wraps the process Postgres connection pool.
type DB struct {
	sqlDB *sql.DB
}

// Open returns a connected Postgres pool when DATABASE_URL is configured. A
// blank DATABASE_URL disables database runtime wiring and returns nil.
func Open(ctx context.Context, cfg config.Config) (*DB, error) {
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return nil, nil
	}

	pgxConfig, err := pgx.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	pgxConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	sqlDB := stdlib.OpenDB(*pgxConfig)
	sqlDB.SetMaxOpenConns(cfg.DBMaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.DBMaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.DBConnMaxLifetime)

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &DB{sqlDB: sqlDB}, nil
}

// SQL exposes the underlying database/sql pool for infrastructure code such as
// migration runners.
func (db *DB) SQL() *sql.DB {
	if db == nil {
		return nil
	}

	return db.sqlDB
}

// CheckReady verifies the pool can reach Postgres. A nil DB means database
// wiring is disabled and is considered ready.
func (db *DB) CheckReady(ctx context.Context) error {
	if db == nil || db.sqlDB == nil {
		return nil
	}

	return db.sqlDB.PingContext(ctx)
}

// Close releases the underlying database pool.
func (db *DB) Close() error {
	if db == nil || db.sqlDB == nil {
		return nil
	}

	return db.sqlDB.Close()
}
