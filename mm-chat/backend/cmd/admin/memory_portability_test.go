package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"neo-chat/mm-chat/backend/internal/usermemory"
)

type adminDeletionRepository struct{}

func (adminDeletionRepository) WithDeletionExportSnapshot(
	_ context.Context,
	use func(usermemory.DeletionExportSnapshot) error,
) error {
	return use(adminDeletionSnapshot{})
}

func (adminDeletionRepository) ReplayDeletionEntries(
	_ context.Context,
	stream func(func(usermemory.PortableDeletionEntry) error) error,
) (usermemory.DeletionReplayResult, error) {
	if err := stream(func(usermemory.PortableDeletionEntry) error { return nil }); err != nil {
		return usermemory.DeletionReplayResult{}, err
	}
	return usermemory.DeletionReplayResult{}, nil
}

type adminDeletionSnapshot struct{}

func (adminDeletionSnapshot) ScanDeletionEntries(
	context.Context,
	func(usermemory.PortableDeletionEntry) error,
) (int, error) {
	return 0, nil
}

func TestExportDeletionPackageFileIsOwnerOnlyAuthenticatedAndNoClobber(t *testing.T) {
	output := filepath.Join(t.TempDir(), "deletions.mm-memory-deletions")
	manifest, err := exportDeletionPackageFile(
		context.Background(), adminDeletionRepository{}, output,
		"fixture-passphrase", "test-release",
	)
	if err != nil {
		t.Fatalf("exportDeletionPackageFile() error = %v", err)
	}
	info, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 || manifest.Count != 0 {
		t.Fatalf("mode=%o manifest=%#v", info.Mode().Perm(), manifest)
	}
	payload, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := usermemory.ParseEncryptedDeletionPackage(
		bytes.NewReader(payload), "fixture-passphrase", nil,
	); err != nil {
		t.Fatalf("parse exported package: %v", err)
	}
	if _, err := exportDeletionPackageFile(
		context.Background(), adminDeletionRepository{}, output,
		"fixture-passphrase", "test-release",
	); err == nil {
		t.Fatal("existing output was overwritten")
	}
}

func TestOpenRegularNoSymlinkRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	link := filepath.Join(directory, "link")
	if err := os.WriteFile(target, []byte("ciphertext"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := openRegularNoSymlink(link); err == nil {
		t.Fatal("symlink input was accepted")
	}
}

var _ usermemory.DeletionPortabilityRepository = adminDeletionRepository{}
var _ usermemory.DeletionExportSnapshot = adminDeletionSnapshot{}
