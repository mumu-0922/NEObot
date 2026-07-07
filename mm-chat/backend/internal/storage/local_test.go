package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestLocalStorePutGetDelete(t *testing.T) {
	store, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStore() error = %v", err)
	}

	ctx := context.Background()
	key := "users/00000000-0000-0000-0000-000000000001/files/object-1"
	body := "hello local object store"
	if err := store.Put(ctx, key, strings.NewReader(body), int64(len(body)), "text/plain"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	reader, info, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	payload, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if string(payload) != body {
		t.Fatalf("payload = %q, want %q", string(payload), body)
	}
	if info.Key != key || info.Size != int64(len(body)) || info.ContentType != "text/plain" {
		t.Fatalf("info = %#v", info)
	}
	if info.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt is zero")
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, _, err := store.Get(ctx, key); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrObjectNotFound", err)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete() missing object error = %v", err)
	}
}

func TestLocalStoreRejectsUnsafeKeys(t *testing.T) {
	store, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStore() error = %v", err)
	}

	badKeys := []string{
		"",
		" ",
		" leading-space",
		"trailing-space ",
		"/absolute",
		"../escape",
		"safe/../escape",
		"safe//double",
		"safe/./dot",
		`safe\windows`,
		"C:/windows-drive",
	}
	for _, key := range badKeys {
		t.Run(key, func(t *testing.T) {
			err := store.Put(context.Background(), key, strings.NewReader("x"), 1, "text/plain")
			if !errors.Is(err, ErrInvalidObjectKey) {
				t.Fatalf("Put(%q) error = %v, want ErrInvalidObjectKey", key, err)
			}
		})
	}
}

func TestLocalStoreRejectsSizeMismatch(t *testing.T) {
	store, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStore() error = %v", err)
	}

	err = store.Put(context.Background(), "objects/too-large", strings.NewReader("abcd"), 3, "text/plain")
	if err == nil || !strings.Contains(err.Error(), "object size mismatch") {
		t.Fatalf("Put() error = %v, want size mismatch", err)
	}
	if _, _, err := store.Get(context.Background(), "objects/too-large"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("Get() after failed Put error = %v, want ErrObjectNotFound", err)
	}
}

func TestLocalStoreHonorsCancelledContext(t *testing.T) {
	store, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStore() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = store.Put(ctx, "objects/cancelled", strings.NewReader("x"), 1, "text/plain")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Put() error = %v, want context.Canceled", err)
	}
	if _, _, err := store.Get(context.Background(), "objects/cancelled"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("Get() after cancelled Put error = %v, want ErrObjectNotFound", err)
	}
}
