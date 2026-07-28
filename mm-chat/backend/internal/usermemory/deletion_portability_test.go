package usermemory

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

type deletionPortabilityTestRepository struct {
	entries   []PortableDeletionEntry
	committed bool
}

func (r *deletionPortabilityTestRepository) WithDeletionExportSnapshot(
	_ context.Context,
	use func(DeletionExportSnapshot) error,
) error {
	return use(deletionExportTestSnapshot{entries: r.entries})
}

func (r *deletionPortabilityTestRepository) ReplayDeletionEntries(
	_ context.Context,
	stream func(func(PortableDeletionEntry) error) error,
) (DeletionReplayResult, error) {
	staged := 0
	if err := stream(func(PortableDeletionEntry) error {
		staged++
		return nil
	}); err != nil {
		return DeletionReplayResult{}, err
	}
	r.committed = true
	return DeletionReplayResult{Entries: staged, Replayed: staged}, nil
}

type deletionExportTestSnapshot struct {
	entries []PortableDeletionEntry
}

func (s deletionExportTestSnapshot) ScanDeletionEntries(
	_ context.Context,
	visit func(PortableDeletionEntry) error,
) (int, error) {
	for _, entry := range s.entries {
		if err := visit(entry); err != nil {
			return 0, err
		}
	}
	return len(s.entries), nil
}

func TestDeletionPackageExportParseAndReplay(t *testing.T) {
	repo := &deletionPortabilityTestRepository{entries: []PortableDeletionEntry{
		validPortableDeletionEntry(),
	}}
	var encrypted bytes.Buffer
	manifest, err := ExportDeletionPackage(
		context.Background(), repo, &encrypted, "fixture-passphrase", "test-release",
	)
	if err != nil {
		t.Fatalf("ExportDeletionPackage() error = %v", err)
	}
	if manifest.Count != 1 || manifest.ExporterRelease != "test-release" {
		t.Fatalf("manifest = %#v", manifest)
	}
	visited := 0
	parsed, err := ParseEncryptedDeletionPackage(
		bytes.NewReader(encrypted.Bytes()), "fixture-passphrase",
		func(entry PortableDeletionEntry) error {
			visited++
			if entry != repo.entries[0] {
				t.Fatalf("entry = %#v, want %#v", entry, repo.entries[0])
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ParseEncryptedDeletionPackage() error = %v", err)
	}
	if parsed.ManifestHash == "" || parsed.DecryptedBytes == 0 || visited != 1 {
		t.Fatalf("parsed=%#v visited=%d", parsed, visited)
	}

	result, err := ReplayEncryptedDeletionPackage(
		context.Background(), repo, bytes.NewReader(encrypted.Bytes()),
		"fixture-passphrase",
	)
	if err != nil || !repo.committed || result.Entries != 1 {
		t.Fatalf("replay=%#v committed=%t err=%v", result, repo.committed, err)
	}
}

func TestDeletionPackageRejectsStrictJSONAndResultShape(t *testing.T) {
	valid := validDeletionPackage(t)
	for name, payload := range map[string][]byte{
		"duplicate": bytes.Replace(
			valid, []byte(`{"kind":"deletion",`),
			[]byte(`{"kind":"deletion","kind":"deletion",`), 1,
		),
		"unknown": bytes.Replace(
			valid, []byte(`{"kind":"deletion",`),
			[]byte(`{"kind":"deletion","plaintext":"forbidden",`), 1,
		),
		"trailing": bytes.Replace(
			valid, []byte("\n"), []byte("{}\n"), 1,
		),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDeletionPackage(bytes.NewReader(payload), nil); err == nil {
				t.Fatal("ParseDeletionPackage() error = nil")
			}
		})
	}

	entry := validPortableDeletionEntry()
	entry.PurgedAt = ""
	if err := validatePortableDeletionEntry(entry); err == nil {
		t.Fatal("ONLINE_PURGED without purgedAt error = nil")
	}
	entry.ResultCode = "PENDING"
	entry.PurgedAt = "2026-07-28T00:00:01Z"
	if err := validatePortableDeletionEntry(entry); err == nil {
		t.Fatal("PENDING with purgedAt error = nil")
	}
}

func TestDeletionReplayDoesNotCommitUnauthenticatedCiphertext(t *testing.T) {
	plaintext := validDeletionPackage(t)
	var encrypted bytes.Buffer
	if err := EncryptPortabilityStream(
		&encrypted, "fixture-passphrase", func(writer io.Writer) error {
			_, err := writer.Write(plaintext)
			return err
		},
	); err != nil {
		t.Fatal(err)
	}
	tampered := append([]byte(nil), encrypted.Bytes()...)
	tampered[len(tampered)-1] ^= 1
	repo := &deletionPortabilityTestRepository{}
	if _, err := ReplayEncryptedDeletionPackage(
		context.Background(), repo, bytes.NewReader(tampered), "fixture-passphrase",
	); err == nil {
		t.Fatal("ReplayEncryptedDeletionPackage() error = nil")
	}
	if repo.committed {
		t.Fatal("tampered deletion package committed")
	}
}

func validDeletionPackage(t *testing.T) []byte {
	t.Helper()
	var plaintext bytes.Buffer
	err := WriteDeletionPackage(&plaintext, DeletionPackageManifest{
		CreatedAt: "2026-07-28T00:00:02Z", ExporterRelease: "test-release",
	}, []PortableDeletionEntry{validPortableDeletionEntry()})
	if err != nil {
		t.Fatal(err)
	}
	return plaintext.Bytes()
}

func validPortableDeletionEntry() PortableDeletionEntry {
	return PortableDeletionEntry{
		Kind:            "deletion",
		ManifestID:      "10000000-0000-4000-8000-000000000001",
		EventID:         "20000000-0000-4000-8000-000000000001",
		UserID:          "30000000-0000-4000-8000-000000000001",
		MemoryID:        "40000000-0000-4000-8000-000000000001",
		TombstoneID:     "50000000-0000-4000-8000-000000000001",
		ContentHash:     strings.Repeat("a", 64),
		ScopeGeneration: 1, VisibilityEpoch: 1,
		DeletedAt: "2026-07-28T00:00:00Z",
		PurgedAt:  "2026-07-28T00:00:01Z", ResultCode: "ONLINE_PURGED",
	}
}

var _ DeletionPortabilityRepository = (*deletionPortabilityTestRepository)(nil)
var _ DeletionExportSnapshot = deletionExportTestSnapshot{}
