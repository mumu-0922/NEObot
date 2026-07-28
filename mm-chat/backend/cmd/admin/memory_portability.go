package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/database"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const memoryPortabilityCommandTimeout = 30 * time.Minute

func runMemoryDeletionsExport(args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("memory-deletions-export", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var output string
	var passphraseStdin bool
	flags.StringVar(&output, "output", "", "encrypted deletion package output")
	flags.BoolVar(&passphraseStdin, "passphrase-stdin", false, "read passphrase from standard input")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 ||
		strings.TrimSpace(output) == "" || !passphraseStdin {
		return usageError()
	}
	passphrase, err := readPasswordLine(stdin)
	if err != nil {
		return err
	}
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), memoryPortabilityCommandTimeout)
	defer cancel()
	repo, closeDatabase, err := openMemoryPortabilityRepository(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeDatabase()
	manifest, err := exportDeletionPackageFile(
		ctx, repo, strings.TrimSpace(output), passphrase, cfg.Version,
	)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		stdout,
		"memory deletions exported entries=%d records_sha256=%s output=%s\n",
		manifest.Count, manifest.RecordsSHA256, strings.TrimSpace(output),
	)
	return err
}

func runMemoryDeletionsReplay(args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("memory-deletions-replay", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var input string
	var passphraseStdin, backendStopped bool
	flags.StringVar(&input, "input", "", "encrypted deletion package input")
	flags.BoolVar(&passphraseStdin, "passphrase-stdin", false, "read passphrase from standard input")
	flags.BoolVar(&backendStopped, "backend-stopped", false, "confirm the backend is stopped or unopened")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 ||
		strings.TrimSpace(input) == "" || !passphraseStdin || !backendStopped {
		return usageError()
	}
	passphrase, err := readPasswordLine(stdin)
	if err != nil {
		return err
	}
	packageFile, err := openRegularNoSymlink(strings.TrimSpace(input))
	if err != nil {
		return err
	}
	defer func() { _ = packageFile.Close() }()
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), memoryPortabilityCommandTimeout)
	defer cancel()
	repo, closeDatabase, err := openMemoryPortabilityRepository(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeDatabase()
	result, err := usermemory.ReplayEncryptedDeletionPackage(
		ctx, repo, packageFile, passphrase,
	)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		stdout,
		"memory deletions replayed entries=%d replayed=%d already_applied=%d not_found=%d hash_mismatch=%d projections_rebuilt=%d\n",
		result.Entries, result.Replayed, result.AlreadyApplied, result.NotFound,
		result.HashMismatch, result.ProjectionRebuilt,
	)
	return err
}

func openMemoryPortabilityRepository(
	ctx context.Context,
	cfg config.Config,
) (*usermemory.PostgresRepository, func(), error) {
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return nil, func() {}, errors.New("DATABASE_URL is required")
	}
	db, err := database.Open(ctx, cfg)
	if err != nil {
		return nil, func() {}, err
	}
	if db == nil || db.SQL() == nil {
		if db != nil {
			_ = db.Close()
		}
		return nil, func() {}, usermemory.ErrDatabaseRequired
	}
	return usermemory.NewPostgresRepository(db.SQL()), func() { _ = db.Close() }, nil
}

func exportDeletionPackageFile(
	ctx context.Context,
	repo usermemory.DeletionPortabilityRepository,
	output, passphrase, release string,
) (usermemory.DeletionPackageManifest, error) {
	if strings.TrimSpace(output) == "" {
		return usermemory.DeletionPackageManifest{}, errors.New("deletion package output is required")
	}
	if _, err := os.Lstat(output); err == nil {
		return usermemory.DeletionPackageManifest{}, errors.New("deletion package output already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return usermemory.DeletionPackageManifest{}, fmt.Errorf("inspect deletion package output: %w", err)
	}
	directory := filepath.Dir(output)
	temporary, err := os.CreateTemp(directory, ".mm-memory-deletions-*.tmp")
	if err != nil {
		return usermemory.DeletionPackageManifest{}, fmt.Errorf("create encrypted deletion package: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return usermemory.DeletionPackageManifest{}, fmt.Errorf("secure encrypted deletion package: %w", err)
	}
	manifest, err := usermemory.ExportDeletionPackage(
		ctx, repo, temporary, passphrase, release,
	)
	if err != nil {
		return usermemory.DeletionPackageManifest{}, err
	}
	if err := temporary.Sync(); err != nil {
		return usermemory.DeletionPackageManifest{}, fmt.Errorf("sync encrypted deletion package: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return usermemory.DeletionPackageManifest{}, fmt.Errorf("close encrypted deletion package: %w", err)
	}
	if err := os.Link(temporaryPath, output); err != nil {
		return usermemory.DeletionPackageManifest{}, fmt.Errorf("publish encrypted deletion package: %w", err)
	}
	removeTemporary = true
	if err := os.Remove(temporaryPath); err != nil {
		_ = os.Remove(output)
		return usermemory.DeletionPackageManifest{}, fmt.Errorf("remove encrypted deletion package temporary file: %w", err)
	}
	removeTemporary = false
	return manifest, nil
}

func openRegularNoSymlink(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect encrypted deletion package: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("encrypted deletion package must be a regular non-symlink file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open encrypted deletion package: %w", err)
	}
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return nil, errors.New("encrypted deletion package changed while opening")
	}
	return file, nil
}
