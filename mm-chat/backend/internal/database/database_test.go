package database

import (
	"context"
	"testing"

	"neo-chat/mm-chat/backend/internal/config"
)

func TestOpenReturnsNilWhenDatabaseDisabled(t *testing.T) {
	db, err := Open(context.Background(), config.Config{DatabaseURL: "  "})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if db != nil {
		t.Fatalf("Open() db = %#v, want nil", db)
	}
}

func TestNilDBIsReadyAndClosable(t *testing.T) {
	var db *DB
	if err := db.CheckReady(context.Background()); err != nil {
		t.Fatalf("CheckReady() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
