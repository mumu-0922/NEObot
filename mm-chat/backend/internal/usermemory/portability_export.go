package usermemory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"strings"
	"time"
)

type PortabilityExportSnapshot interface {
	ScanRecords(context.Context, func(any) error) (PortableRecordCounts, error)
}

type PortabilityExportRepository interface {
	WithPortabilityExportSnapshot(
		context.Context,
		bool,
		func(PortabilityExportSnapshot) error,
	) error
}

type MemoryExportResult struct {
	Filename string
	Manifest MemoryPackageManifest
}

func (s *Service) ExportMemoryPackage(
	ctx context.Context,
	destination io.Writer,
	passphrase string,
	includeHistory bool,
) (MemoryExportResult, error) {
	if destination == nil {
		return MemoryExportResult{}, errors.New("memory export destination is required")
	}
	if err := validatePortabilityPassphrase(passphrase); err != nil {
		return MemoryExportResult{}, err
	}
	if err := s.requireRepository(); err != nil {
		return MemoryExportResult{}, err
	}
	repo, ok := s.repo.(PortabilityExportRepository)
	if !ok {
		return MemoryExportResult{}, ErrPortabilityRepositoryRequired
	}
	release := strings.TrimSpace(s.release)
	if release == "" {
		release = "development"
	}
	createdAt := time.Now().UTC()
	manifest := MemoryPackageManifest{
		Kind:            "manifest",
		Format:          MemoryPackageFormat,
		FormatVersion:   MemoryPackageFormatVersion,
		SchemaVersion:   MemoryPackageSchemaVersion,
		CreatedAt:       createdAt.Format(time.RFC3339Nano),
		ExporterRelease: release,
		IncludeHistory:  includeHistory,
	}
	err := repo.WithPortabilityExportSnapshot(ctx, includeHistory, func(snapshot PortabilityExportSnapshot) error {
		firstHash := sha256.New()
		counts, err := snapshot.ScanRecords(ctx, func(record any) error {
			return writeExportRecord(firstHash, nil, record)
		})
		if err != nil {
			return err
		}
		if err := validateExportCounts(counts, includeHistory); err != nil {
			return err
		}
		manifest.Counts = counts
		manifest.RecordsSHA256 = hex.EncodeToString(firstHash.Sum(nil))
		return EncryptPortabilityStream(destination, passphrase, func(plaintext io.Writer) error {
			manifestLine, err := json.Marshal(manifest)
			if err != nil {
				return fmt.Errorf("marshal memory export manifest: %w", err)
			}
			if _, err := plaintext.Write(append(manifestLine, '\n')); err != nil {
				return fmt.Errorf("write memory export manifest: %w", err)
			}
			secondHash := sha256.New()
			secondCounts, err := snapshot.ScanRecords(ctx, func(record any) error {
				return writeExportRecord(secondHash, plaintext, record)
			})
			if err != nil {
				return err
			}
			if secondCounts != counts ||
				hex.EncodeToString(secondHash.Sum(nil)) != manifest.RecordsSHA256 {
				return errors.New("memory export snapshot changed between deterministic scans")
			}
			return nil
		})
	})
	if err != nil {
		return MemoryExportResult{}, err
	}
	return MemoryExportResult{
		Filename: "memory-" + createdAt.Format("20060102T150405Z") + ".mm-memory",
		Manifest: manifest,
	}, nil
}

func writeExportRecord(hasher hash.Hash, destination io.Writer, record any) error {
	switch record.(type) {
	case PortableSettingsRecord, PortableProjectRecord, PortableMemoryRecord,
		PortableRevisionRecord:
	default:
		return errors.New("memory export snapshot returned an unsupported record")
	}
	return writeCanonicalRecord(hasher, destination, record)
}

func validateExportCounts(counts PortableRecordCounts, includeHistory bool) error {
	if counts.Settings < 0 || counts.Settings > 1 ||
		counts.Projects < 0 || counts.Projects > MaxMemoryPackageProjects ||
		counts.Memories < 0 || counts.Memories > MaxMemoryPackageMemories ||
		counts.Revisions < 0 || counts.Revisions > MaxMemoryPackageRevisions ||
		(!includeHistory && counts.Revisions != 0) {
		return packageCountError("export")
	}
	return nil
}
