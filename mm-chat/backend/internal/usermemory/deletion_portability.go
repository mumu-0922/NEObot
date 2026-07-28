package usermemory

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	DeletionPackageFormat        = "mm-memory-deletions"
	DeletionPackageFormatVersion = 1
	DeletionPackageSchemaVersion = 1
	MaxDeletionPackageEntries    = 1000000
)

type DeletionPackageManifest struct {
	Kind            string `json:"kind"`
	Format          string `json:"format"`
	FormatVersion   int    `json:"formatVersion"`
	SchemaVersion   int    `json:"schemaVersion"`
	CreatedAt       string `json:"createdAt"`
	ExporterRelease string `json:"exporterRelease"`
	Count           int    `json:"count"`
	RecordsSHA256   string `json:"recordsSha256"`
}

type PortableDeletionEntry struct {
	Kind            string `json:"kind"`
	ManifestID      string `json:"manifestId"`
	EventID         string `json:"eventId"`
	UserID          string `json:"userId"`
	MemoryID        string `json:"memoryId"`
	TombstoneID     string `json:"tombstoneId"`
	ContentHash     string `json:"contentHash"`
	ScopeGeneration int64  `json:"scopeGeneration"`
	VisibilityEpoch int64  `json:"visibilityEpoch"`
	DeletedAt       string `json:"deletedAt"`
	PurgedAt        string `json:"purgedAt,omitempty"`
	ResultCode      string `json:"resultCode"`
}

type ParsedDeletionPackage struct {
	Manifest       DeletionPackageManifest
	ManifestHash   string
	DecryptedBytes int64
}

type DeletionExportSnapshot interface {
	ScanDeletionEntries(context.Context, func(PortableDeletionEntry) error) (int, error)
}

type DeletionPortabilityRepository interface {
	WithDeletionExportSnapshot(context.Context, func(DeletionExportSnapshot) error) error
	ReplayDeletionEntries(
		context.Context,
		func(func(PortableDeletionEntry) error) error,
	) (DeletionReplayResult, error)
}

type DeletionReplayResult struct {
	Entries           int            `json:"entries"`
	Replayed          int            `json:"replayed"`
	AlreadyApplied    int            `json:"alreadyApplied"`
	NotFound          int            `json:"notFound"`
	HashMismatch      int            `json:"hashMismatch"`
	ProjectionRebuilt int            `json:"projectionRebuilt"`
	ResultCounts      map[string]int `json:"resultCounts"`
}

func ExportDeletionPackage(
	ctx context.Context,
	repo DeletionPortabilityRepository,
	destination io.Writer,
	passphrase string,
	release string,
) (DeletionPackageManifest, error) {
	if repo == nil || destination == nil {
		return DeletionPackageManifest{}, errors.New("deletion export repository and destination are required")
	}
	if err := validatePortabilityPassphrase(passphrase); err != nil {
		return DeletionPackageManifest{}, err
	}
	release = strings.TrimSpace(release)
	if release == "" {
		release = "development"
	}
	manifest := DeletionPackageManifest{
		Kind: "manifest", Format: DeletionPackageFormat,
		FormatVersion:   DeletionPackageFormatVersion,
		SchemaVersion:   DeletionPackageSchemaVersion,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		ExporterRelease: release,
	}
	err := repo.WithDeletionExportSnapshot(ctx, func(snapshot DeletionExportSnapshot) error {
		firstHash := sha256.New()
		firstCount := 0
		lastKey := ""
		count, err := snapshot.ScanDeletionEntries(ctx, func(entry PortableDeletionEntry) error {
			if err := validatePortableDeletionEntry(entry); err != nil {
				return err
			}
			key := entry.DeletedAt + ":" + entry.ManifestID
			if lastKey != "" && key <= lastKey {
				return validation("MEMORY_DELETION_ENTRY_ORDER_INVALID", "deletion entries must be strictly ordered")
			}
			lastKey = key
			firstCount++
			return writeCanonicalRecord(firstHash, nil, entry)
		})
		if err != nil {
			return err
		}
		if count != firstCount || count < 0 || count > MaxDeletionPackageEntries {
			return validation("MEMORY_DELETION_PACKAGE_COUNT_LIMIT", "deletion package entry count exceeds the limit")
		}
		manifest.Count = count
		manifest.RecordsSHA256 = hex.EncodeToString(firstHash.Sum(nil))
		return EncryptPortabilityStream(destination, passphrase, func(plaintext io.Writer) error {
			line, err := json.Marshal(manifest)
			if err != nil {
				return err
			}
			if _, err := plaintext.Write(append(line, '\n')); err != nil {
				return err
			}
			secondHash := sha256.New()
			secondVisited := 0
			secondLastKey := ""
			secondCount, err := snapshot.ScanDeletionEntries(ctx, func(entry PortableDeletionEntry) error {
				if err := validatePortableDeletionEntry(entry); err != nil {
					return err
				}
				key := entry.DeletedAt + ":" + entry.ManifestID
				if secondLastKey != "" && key <= secondLastKey {
					return validation("MEMORY_DELETION_ENTRY_ORDER_INVALID", "deletion entries must be strictly ordered")
				}
				secondLastKey = key
				secondVisited++
				return writeCanonicalRecord(secondHash, plaintext, entry)
			})
			if err != nil {
				return err
			}
			if secondCount != secondVisited || secondCount != count ||
				hex.EncodeToString(secondHash.Sum(nil)) != manifest.RecordsSHA256 {
				return errors.New("deletion export snapshot changed between scans")
			}
			return nil
		})
	})
	if err != nil {
		return DeletionPackageManifest{}, err
	}
	return manifest, nil
}

func ParseEncryptedDeletionPackage(
	source io.Reader,
	passphrase string,
	visit func(PortableDeletionEntry) error,
) (ParsedDeletionPackage, error) {
	plaintext, err := DecryptPortabilityStream(source, passphrase)
	if err != nil {
		return ParsedDeletionPackage{}, err
	}
	return ParseDeletionPackage(plaintext, visit)
}

func ParseDeletionPackage(
	plaintext io.Reader,
	visit func(PortableDeletionEntry) error,
) (ParsedDeletionPackage, error) {
	if plaintext == nil {
		return ParsedDeletionPackage{}, errors.New("deletion package plaintext is required")
	}
	if visit == nil {
		visit = func(PortableDeletionEntry) error { return nil }
	}
	counter := &countingReader{reader: plaintext}
	scanner := bufio.NewScanner(counter)
	scanner.Buffer(make([]byte, 4096), maxMemoryPackageLineBytes)
	if !scanner.Scan() {
		if scanner.Err() != nil {
			return ParsedDeletionPackage{}, mapMemoryPackageReadError(scanner.Err())
		}
		return ParsedDeletionPackage{}, validation("MEMORY_DELETION_MANIFEST_REQUIRED", "deletion manifest is required")
	}
	manifestLine := append([]byte(nil), scanner.Bytes()...)
	var manifest DeletionPackageManifest
	if err := decodeCanonicalJSONLine(manifestLine, &manifest); err != nil {
		return ParsedDeletionPackage{}, err
	}
	if manifest.Kind != "manifest" || manifest.Format != DeletionPackageFormat ||
		manifest.FormatVersion != DeletionPackageFormatVersion ||
		manifest.SchemaVersion != DeletionPackageSchemaVersion ||
		manifest.Count < 0 || manifest.Count > MaxDeletionPackageEntries ||
		!validPortableTime(manifest.CreatedAt) ||
		strings.TrimSpace(manifest.ExporterRelease) == "" || len(manifest.ExporterRelease) > 128 ||
		!isLowerSHA256(manifest.RecordsSHA256) {
		return ParsedDeletionPackage{}, validation("MEMORY_DELETION_MANIFEST_INVALID", "deletion package manifest is invalid")
	}
	manifestSum := sha256.Sum256(append(append([]byte(nil), manifestLine...), '\n'))
	hasher := sha256.New()
	count := 0
	lastKey := ""
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		_, _ = hasher.Write(line)
		_, _ = hasher.Write([]byte{'\n'})
		var entry PortableDeletionEntry
		if err := decodeCanonicalJSONLine(line, &entry); err != nil {
			return ParsedDeletionPackage{}, err
		}
		if err := validatePortableDeletionEntry(entry); err != nil {
			return ParsedDeletionPackage{}, err
		}
		key := entry.DeletedAt + ":" + entry.ManifestID
		if lastKey != "" && key <= lastKey {
			return ParsedDeletionPackage{}, validation("MEMORY_DELETION_ENTRY_ORDER_INVALID", "deletion entries must be strictly ordered")
		}
		lastKey = key
		count++
		if count > MaxDeletionPackageEntries {
			return ParsedDeletionPackage{}, validation("MEMORY_DELETION_PACKAGE_COUNT_LIMIT", "deletion package entry count exceeds the limit")
		}
		if err := visit(entry); err != nil {
			return ParsedDeletionPackage{}, err
		}
	}
	if scanner.Err() != nil {
		return ParsedDeletionPackage{}, mapMemoryPackageReadError(scanner.Err())
	}
	if count != manifest.Count {
		return ParsedDeletionPackage{}, validation("MEMORY_DELETION_COUNT_MISMATCH", "deletion package count does not match")
	}
	if hex.EncodeToString(hasher.Sum(nil)) != manifest.RecordsSHA256 {
		return ParsedDeletionPackage{}, validation("MEMORY_DELETION_HASH_MISMATCH", "deletion package hash does not match")
	}
	return ParsedDeletionPackage{
		Manifest: manifest, ManifestHash: hex.EncodeToString(manifestSum[:]),
		DecryptedBytes: counter.count,
	}, nil
}

func ReplayEncryptedDeletionPackage(
	ctx context.Context,
	repo DeletionPortabilityRepository,
	source io.ReadSeeker,
	passphrase string,
) (DeletionReplayResult, error) {
	if repo == nil || source == nil {
		return DeletionReplayResult{}, errors.New("deletion replay repository and package are required")
	}
	return repo.ReplayDeletionEntries(ctx, func(apply func(PortableDeletionEntry) error) error {
		if _, err := source.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("rewind deletion package: %w", err)
		}
		_, err := ParseEncryptedDeletionPackage(source, passphrase, apply)
		return err
	})
}

func validatePortableDeletionEntry(entry PortableDeletionEntry) error {
	if entry.Kind != "deletion" || !uuidRE.MatchString(entry.ManifestID) ||
		!uuidRE.MatchString(entry.EventID) || !uuidRE.MatchString(entry.UserID) ||
		!uuidRE.MatchString(entry.MemoryID) || !uuidRE.MatchString(entry.TombstoneID) ||
		!isLowerSHA256(entry.ContentHash) || entry.ScopeGeneration < 1 ||
		entry.VisibilityEpoch < 1 || !validPortableTime(entry.DeletedAt) ||
		(entry.PurgedAt != "" && !validPortableTime(entry.PurgedAt)) ||
		(entry.ResultCode != "PENDING" && entry.ResultCode != "ONLINE_PURGED") ||
		(entry.ResultCode == "PENDING" && entry.PurgedAt != "") ||
		(entry.ResultCode == "ONLINE_PURGED" && entry.PurgedAt == "") {
		return validation("MEMORY_DELETION_ENTRY_INVALID", "deletion package entry is invalid")
	}
	if entry.PurgedAt != "" {
		deletedAt, _ := time.Parse(time.RFC3339Nano, entry.DeletedAt)
		purgedAt, _ := time.Parse(time.RFC3339Nano, entry.PurgedAt)
		if purgedAt.Before(deletedAt) {
			return validation("MEMORY_DELETION_ENTRY_INVALID", "deletion package entry is invalid")
		}
	}
	return nil
}

func WriteDeletionPackage(
	destination io.Writer,
	manifest DeletionPackageManifest,
	entries []PortableDeletionEntry,
) error {
	if destination == nil {
		return errors.New("deletion package destination is required")
	}
	if !validPortableTime(manifest.CreatedAt) ||
		strings.TrimSpace(manifest.ExporterRelease) == "" || len(manifest.ExporterRelease) > 128 {
		return validation("MEMORY_DELETION_MANIFEST_INVALID", "deletion package manifest is invalid")
	}
	hasher := sha256.New()
	records := make([][]byte, 0, len(entries))
	lastKey := ""
	for _, entry := range entries {
		if err := validatePortableDeletionEntry(entry); err != nil {
			return err
		}
		key := entry.DeletedAt + ":" + entry.ManifestID
		if lastKey != "" && key <= lastKey {
			return validation("MEMORY_DELETION_ENTRY_ORDER_INVALID", "deletion entries must be strictly ordered")
		}
		lastKey = key
		payload, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		records = append(records, payload)
		_, _ = hasher.Write(payload)
		_, _ = hasher.Write([]byte{'\n'})
	}
	manifest.Kind = "manifest"
	manifest.Format = DeletionPackageFormat
	manifest.FormatVersion = DeletionPackageFormatVersion
	manifest.SchemaVersion = DeletionPackageSchemaVersion
	manifest.Count = len(entries)
	manifest.RecordsSHA256 = hex.EncodeToString(hasher.Sum(nil))
	manifestLine, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if _, err := destination.Write(append(manifestLine, '\n')); err != nil {
		return err
	}
	for _, record := range records {
		if _, err := destination.Write(append(record, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func decodeDeletionJSON(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}
