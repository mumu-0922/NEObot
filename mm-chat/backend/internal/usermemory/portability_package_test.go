package usermemory

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestMemoryPackageAgeRoundTripAndTamper(t *testing.T) {
	plaintext := validMemoryPackage(t, false)
	var encrypted bytes.Buffer
	if err := EncryptPortabilityStream(&encrypted, "fixture-passphrase", func(writer io.Writer) error {
		_, err := writer.Write(plaintext)
		return err
	}); err != nil {
		t.Fatalf("EncryptPortabilityStream() error = %v", err)
	}

	parsed, err := ParseEncryptedMemoryPackage(
		bytes.NewReader(encrypted.Bytes()),
		"fixture-passphrase",
		MemoryPackageVisitorFuncs{},
	)
	if err != nil {
		t.Fatalf("ParseEncryptedMemoryPackage() error = %v", err)
	}
	if parsed.Manifest.Counts.Memories != 1 || parsed.DecryptedBytes != int64(len(plaintext)) {
		t.Fatalf("parsed package = %#v", parsed)
	}

	if _, err := ParseEncryptedMemoryPackage(
		bytes.NewReader(encrypted.Bytes()),
		"wrong-passphrase",
		MemoryPackageVisitorFuncs{},
	); errorCode(err) != "MEMORY_PORTABILITY_DECRYPT_FAILED" {
		t.Fatalf("wrong passphrase error = %v", err)
	}

	tampered := append([]byte(nil), encrypted.Bytes()...)
	tampered[len(tampered)-1] ^= 1
	if _, err := ParseEncryptedMemoryPackage(
		bytes.NewReader(tampered),
		"fixture-passphrase",
		MemoryPackageVisitorFuncs{},
	); errorCode(err) != "MEMORY_PACKAGE_AUTHENTICATION_FAILED" {
		t.Fatalf("tampered stream error = %v", err)
	}

	truncated := encrypted.Bytes()[:len(encrypted.Bytes())-16]
	if _, err := ParseEncryptedMemoryPackage(
		bytes.NewReader(truncated),
		"fixture-passphrase",
		MemoryPackageVisitorFuncs{},
	); errorCode(err) != "MEMORY_PACKAGE_AUTHENTICATION_FAILED" {
		t.Fatalf("truncated stream error = %v", err)
	}
}

func TestMemoryPackageStrictJSONAndHash(t *testing.T) {
	for name, mutate := range map[string]func([]byte) []byte{
		"duplicate": func(payload []byte) []byte {
			return bytes.Replace(payload, []byte(`{"kind":"memory",`), []byte(`{"kind":"memory","kind":"memory",`), 1)
		},
		"unknown": func(payload []byte) []byte {
			return bytes.Replace(payload, []byte(`{"kind":"memory",`), []byte(`{"kind":"memory","unknown":true,`), 1)
		},
		"non canonical": func(payload []byte) []byte {
			return bytes.Replace(payload, []byte(`{"kind":"memory",`), []byte(`{ "kind":"memory",`), 1)
		},
		"hash": func(payload []byte) []byte {
			return bytes.Replace(payload, []byte(`"content":"Keep answers concise"`), []byte(`"content":"Keep answers precise"`), 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseMemoryPackage(bytes.NewReader(mutate(validMemoryPackage(t, false))), MemoryPackageVisitorFuncs{})
			if err == nil {
				t.Fatal("ParseMemoryPackage() error = nil")
			}
		})
	}
}

func TestMemoryPackageRevisionChain(t *testing.T) {
	payload := validMemoryPackage(t, true)
	visited := 0
	parsed, err := ParseMemoryPackage(bytes.NewReader(payload), MemoryPackageVisitorFuncs{
		Revision: func(_ int, record PortableRevisionRecord) error {
			visited++
			if record.Revision != 2 || record.Prior == nil {
				t.Fatalf("revision = %#v", record)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("ParseMemoryPackage() error = %v", err)
	}
	if !parsed.Manifest.IncludeHistory || visited != 1 {
		t.Fatalf("parsed=%#v visited=%d", parsed, visited)
	}

	broken := bytes.Replace(payload, []byte(contentSHA256("Prefer concise answers")), []byte(strings.Repeat("a", 64)), 1)
	if _, err := ParseMemoryPackage(bytes.NewReader(broken), MemoryPackageVisitorFuncs{}); err == nil {
		t.Fatal("broken revision chain error = nil")
	}
}

func TestValidatePortableMemoryCandidateRejectsSecretAndLimits(t *testing.T) {
	record := validPortableMemory(1)
	if err := ValidatePortableMemoryCandidate(record); err != nil {
		t.Fatalf("ValidatePortableMemoryCandidate() error = %v", err)
	}
	record.Content = strings.Repeat("界", MaxContentChars+1)
	if err := ValidatePortableMemoryCandidate(record); err == nil {
		t.Fatal("oversized content error = nil")
	}
	record = validPortableMemory(1)
	record.Content = "api_key=fixture-secret-value"
	if sensitivity := ClassifyMemorySensitivity(record.Content); sensitivity != SensitivitySecret {
		t.Fatalf("sensitivity = %q", sensitivity)
	}
}

func validMemoryPackage(t *testing.T, history bool) []byte {
	t.Helper()
	record := validPortableMemory(1)
	records := []any{
		PortableSettingsRecord{Kind: "settings", Settings: DefaultSettings()},
		PortableProjectRecord{
			Kind: "project", Ref: "project-000001", Name: "Neo Chat",
			Description: "Memory portability", LifecycleStatus: "active",
		},
	}
	if history {
		record.Revision = 2
		record.Content = "Keep answers concise"
		record.ContentHash = contentSHA256(record.Content)
	}
	records = append(records, record)
	if history {
		prior := PortableMemorySnapshot{
			Type: "preference", Content: "Prefer concise answers",
			ContentHash: contentSHA256("Prefer concise answers"), Importance: 4,
			Tags: []string{"style"}, Enabled: true,
			Scope: PortableMemoryScope{Type: "global"}, LifecycleStatus: "active",
			Sensitivity: "normal", Confidence: 1,
			ObservedAt: "2026-07-28T00:00:00Z",
		}
		records = append(records, PortableRevisionRecord{
			Kind: "revision", MemoryRef: "memory-000001", Revision: 2,
			Operation: "update", OldContentHash: prior.ContentHash,
			NewContentHash: record.ContentHash, Prior: &prior,
			CreatedAt: "2026-07-28T01:00:00Z",
		})
	}
	manifest := MemoryPackageManifest{
		CreatedAt: "2026-07-28T02:00:00Z", ExporterRelease: "test",
		IncludeHistory: history,
		Counts: PortableRecordCounts{
			Settings: 1, Projects: 1, Memories: 1,
			Revisions: map[bool]int{false: 0, true: 1}[history],
		},
	}
	var plaintext bytes.Buffer
	if err := WriteMemoryPackage(&plaintext, manifest, records); err != nil {
		t.Fatalf("WriteMemoryPackage() error = %v", err)
	}
	return plaintext.Bytes()
}

func validPortableMemory(revision int64) PortableMemoryRecord {
	now := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	content := "Keep answers concise"
	return PortableMemoryRecord{
		Kind: "memory", Ref: "memory-000001", Revision: revision,
		Type: "preference", Content: content, ContentHash: contentSHA256(content),
		Importance: 4, Tags: []string{"style"}, OriginalAuthority: "manual",
		Enabled: true, Scope: PortableMemoryScope{Type: "global"},
		LifecycleStatus: "active", Sensitivity: "normal", Confidence: 1,
		ObservedAt: now, CreatedAt: now, UpdatedAt: now,
	}
}

func errorCode(err error) string {
	var target ValidationError
	if errors.As(err, &target) {
		return target.Code
	}
	return ""
}

func TestWriteMemoryPackageProducesCanonicalManifest(t *testing.T) {
	payload := validMemoryPackage(t, false)
	first, _, ok := bytes.Cut(payload, []byte{'\n'})
	if !ok {
		t.Fatal("manifest line missing")
	}
	var manifest MemoryPackageManifest
	if err := json.Unmarshal(first, &manifest); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if manifest.Kind != "manifest" || manifest.Format != "mm-memory" ||
		!isLowerSHA256(manifest.RecordsSHA256) {
		t.Fatalf("manifest = %#v", manifest)
	}
}
