package agentbroker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	PatchWrite    = "write"
	PatchDelete   = "delete"
	maxPatchFiles = 128
	maxPatchBytes = int64(8 << 20)
)

type ProjectPatchEntry struct {
	Path               string `json:"path"`
	Operation          string `json:"operation"`
	Content            []byte `json:"-"`
	ContentFingerprint string `json:"contentFingerprint,omitempty"`
}

type ProjectPatch struct {
	BaseRevision string              `json:"baseRevision"`
	Entries      []ProjectPatchEntry `json:"entries"`
	ByteSize     int64               `json:"byteSize"`
	Fingerprint  string              `json:"patchFingerprint"`
}

func CanonicalProjectPatch(baseRevision string, entries []ProjectPatchEntry) (ProjectPatch, error) {
	if !revisionPattern.MatchString(baseRevision) || len(entries) < 1 || len(entries) > maxPatchFiles {
		return ProjectPatch{}, ErrInvalidInput
	}
	result := ProjectPatch{BaseRevision: baseRevision, Entries: make([]ProjectPatchEntry, len(entries))}
	seen := map[string]struct{}{}
	for index, entry := range entries {
		if !validProjectPath(entry.Path) || !member(entry.Operation, PatchWrite, PatchDelete) ||
			(entry.Operation == PatchDelete && len(entry.Content) != 0) || !utf8.Valid(entry.Content) {
			return ProjectPatch{}, ErrInvalidInput
		}
		collision := strings.ToLower(norm.NFC.String(entry.Path))
		if _, duplicate := seen[collision]; duplicate {
			return ProjectPatch{}, ErrInvalidInput
		}
		seen[collision] = struct{}{}
		result.ByteSize += int64(len(entry.Content))
		if result.ByteSize > maxPatchBytes {
			return ProjectPatch{}, ErrBudgetExhausted
		}
		entry.Content = append([]byte(nil), entry.Content...)
		if entry.Operation == PatchWrite {
			entry.ContentFingerprint = fingerprint("neo-project-file-v1", entry.Content)
		} else {
			entry.ContentFingerprint = ""
		}
		result.Entries[index] = entry
	}
	sort.Slice(result.Entries, func(i, j int) bool { return result.Entries[i].Path < result.Entries[j].Path })
	encoded, _ := json.Marshal(struct {
		BaseRevision string              `json:"baseRevision"`
		Entries      []ProjectPatchEntry `json:"entries"`
		ByteSize     int64               `json:"byteSize"`
	}{result.BaseRevision, result.Entries, result.ByteSize})
	result.Fingerprint = fingerprint("neo-project-patch-v1", encoded)
	return result, nil
}

func validProjectPath(value string) bool {
	if value == "" || len(value) > 512 || strings.ContainsRune(value, '\x00') || strings.Contains(value, "\\") ||
		strings.HasPrefix(value, "/") || path.Clean(value) != value || value == "." ||
		strings.HasPrefix(value, "../") || strings.Contains(value, "/../") || !norm.NFC.IsNormalString(value) {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" || component == "." || component == ".." {
			return false
		}
	}
	return true
}

type ProjectCASExecutor interface {
	CommitPatch(context.Context, string, ProjectPatch) (string, error)
	PatchStatus(context.Context, string, string) (string, error)
}

// DeterministicCASFake is non-production proof only. It models exact stable-key
// replay and revision conflicts without a real Project-file store.
type DeterministicCASFake struct {
	mu       sync.Mutex
	revision string
	replays  map[string]fakePatchReceipt
}

type fakePatchReceipt struct{ fingerprint, revision string }

func NewDeterministicCASFake(baseRevision string) (*DeterministicCASFake, error) {
	if !revisionPattern.MatchString(baseRevision) {
		return nil, ErrInvalidInput
	}
	return &DeterministicCASFake{revision: baseRevision, replays: map[string]fakePatchReceipt{}}, nil
}

func (executor *DeterministicCASFake) CommitPatch(_ context.Context, key string, patch ProjectPatch) (string, error) {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	if existing, ok := executor.replays[key]; ok {
		if existing.fingerprint != patch.Fingerprint {
			return "", ErrReplayDetected
		}
		return existing.revision, nil
	}
	if patch.BaseRevision != executor.revision {
		return "", ErrProjectConflict
	}
	digest := sha256.Sum256([]byte(executor.revision + "\x00" + patch.Fingerprint))
	executor.revision = "rev_" + hex.EncodeToString(digest[:16])
	executor.replays[key] = fakePatchReceipt{fingerprint: patch.Fingerprint, revision: executor.revision}
	return executor.revision, nil
}

func (executor *DeterministicCASFake) PatchStatus(_ context.Context, key, fingerprint string) (string, error) {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	receipt, ok := executor.replays[key]
	if !ok || receipt.fingerprint != fingerprint {
		return "", ErrNotFound
	}
	return receipt.revision, nil
}
