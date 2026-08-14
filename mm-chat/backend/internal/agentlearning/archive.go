package agentlearning

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"neo-chat/mm-chat/backend/internal/skillsupply"
)

const (
	maxDiffTextBytes = 512 << 10
	maxDiffFileBytes = 64 << 10
)

func fingerprint(domain string, value []byte) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte(domain))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(value)
	return "sha256:" + hex.EncodeToString(digest.Sum(nil))
}

func contentFingerprint(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func canonicalEvidence(input []EvidenceRef) ([]EvidenceRef, []byte, string, error) {
	if len(input) < 2 || len(input) > 32 {
		return nil, nil, "", ErrInvalidInput
	}
	items := append([]EvidenceRef(nil), input...)
	for index := range items {
		items[index].Kind = strings.TrimSpace(items[index].Kind)
		items[index].Ref = strings.TrimSpace(items[index].Ref)
		items[index].Fingerprint = strings.TrimSpace(items[index].Fingerprint)
		items[index].Paths = append([]string(nil), items[index].Paths...)
		sort.Strings(items[index].Paths)
		if (items[index].Kind != EvidenceSourcePackage && items[index].Kind != EvidenceRunEvent) ||
			items[index].Ref == "" || !digestPattern.MatchString(items[index].Fingerprint) ||
			len(items[index].Paths) < 1 || len(items[index].Paths) > 256 ||
			!uniqueBoundedStrings(items[index].Paths, 512) ||
			!validEvidenceReference(items[index]) {
			return nil, nil, "", ErrInvalidInput
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].Ref < items[j].Ref
	})
	for index := 1; index < len(items); index++ {
		if items[index-1].Kind == items[index].Kind && items[index-1].Ref == items[index].Ref {
			return nil, nil, "", ErrInvalidInput
		}
	}
	encoded, err := json.Marshal(items)
	if err != nil || len(encoded) > 32<<10 {
		return nil, nil, "", ErrInvalidInput
	}
	return items, encoded, fingerprint("neo-agent-learning-evidence-v1", encoded), nil
}

func deriveTests(inventory []skillsupply.FileInventory) ([]TestFile, []byte, string, error) {
	tests := make([]TestFile, 0)
	for _, file := range inventory {
		if strings.HasPrefix(file.Path, "tests/") {
			tests = append(tests, TestFile{Path: file.Path, Size: file.Size, Fingerprint: file.SHA256})
		}
	}
	if len(tests) < 1 || len(tests) > 256 {
		return nil, nil, "", ErrInvalidInput
	}
	sort.Slice(tests, func(i, j int) bool { return tests[i].Path < tests[j].Path })
	encoded, err := json.Marshal(tests)
	if err != nil || len(encoded) > 64<<10 {
		return nil, nil, "", ErrInvalidInput
	}
	return tests, encoded, fingerprint("neo-agent-learning-tests-v1", encoded), nil
}

func validateEvidenceCoverage(
	evidence []EvidenceRef,
	baseFingerprint string,
	changed []string,
) error {
	hasBase := false
	runPaths := map[string]struct{}{}
	for _, item := range evidence {
		if item.Kind == EvidenceSourcePackage && item.Ref == baseFingerprint &&
			item.Fingerprint == baseFingerprint {
			hasBase = true
		}
		if item.Kind == EvidenceRunEvent {
			for _, path := range item.Paths {
				runPaths[path] = struct{}{}
			}
		}
	}
	if !hasBase || len(runPaths) == 0 {
		return ErrInvalidInput
	}
	for _, path := range changed {
		if _, ok := runPaths[path]; !ok {
			return ErrInvalidInput
		}
	}
	return nil
}

func authorityUnchanged(base, proposed skillsupply.ValidatedPackage) bool {
	if !base.Package.HasRuntime || !proposed.Package.HasRuntime || base.Package.Manifest == nil ||
		proposed.Package.Manifest == nil || base.Package.Name != proposed.Package.Name ||
		base.Package.Version == proposed.Package.Version ||
		!reflect.DeepEqual(base.Package.AllowedTools, proposed.Package.AllowedTools) ||
		!reflect.DeepEqual(base.Package.CapabilityRequests, proposed.Package.CapabilityRequests) {
		return false
	}
	baseManifest := *base.Package.Manifest
	proposedManifest := *proposed.Package.Manifest
	baseManifest.Package.Version = proposedManifest.Package.Version
	return reflect.DeepEqual(baseManifest, proposedManifest)
}

func extractArchive(data []byte) (map[string][]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(reader.File) == 0 || len(reader.File) > 2048 {
		return nil, ErrObjectDrift
	}
	files := make(map[string][]byte, len(reader.File))
	var total int64
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() || !entry.Mode().IsRegular() || entry.UncompressedSize64 > 16<<20 {
			return nil, ErrObjectDrift
		}
		body, openErr := entry.Open()
		if openErr != nil {
			return nil, ErrObjectDrift
		}
		value, readErr := io.ReadAll(io.LimitReader(body, 16<<20+1))
		_ = body.Close()
		if readErr != nil || len(value) > 16<<20 || uint64(len(value)) != entry.UncompressedSize64 {
			return nil, ErrObjectDrift
		}
		total += int64(len(value))
		if total > 128<<20 {
			return nil, ErrObjectDrift
		}
		files[entry.Name] = value
	}
	return files, nil
}

func changedPaths(baseArchive, proposedArchive []byte) ([]string, error) {
	base, err := extractArchive(baseArchive)
	if err != nil {
		return nil, err
	}
	proposed, err := extractArchive(proposedArchive)
	if err != nil {
		return nil, err
	}
	paths := make(map[string]struct{}, len(base)+len(proposed))
	for path := range base {
		paths[path] = struct{}{}
	}
	for path := range proposed {
		paths[path] = struct{}{}
	}
	changed := make([]string, 0)
	for path := range paths {
		if !bytes.Equal(base[path], proposed[path]) {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	return changed, nil
}

func buildDiff(baseArchive, proposedArchive []byte) ([]FileDiff, error) {
	base, err := extractArchive(baseArchive)
	if err != nil {
		return nil, err
	}
	proposed, err := extractArchive(proposedArchive)
	if err != nil {
		return nil, err
	}
	paths := make(map[string]struct{}, len(base)+len(proposed))
	for path := range base {
		paths[path] = struct{}{}
	}
	for path := range proposed {
		paths[path] = struct{}{}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	result := make([]FileDiff, 0)
	textBudget := 0
	for _, path := range ordered {
		before, beforeOK := base[path]
		after, afterOK := proposed[path]
		if beforeOK && afterOK && bytes.Equal(before, after) {
			continue
		}
		item := FileDiff{Path: path, BeforeSize: int64(len(before)), AfterSize: int64(len(after))}
		switch {
		case !beforeOK:
			item.Change = "added"
		case !afterOK:
			item.Change = "deleted"
		default:
			item.Change = "modified"
		}
		if beforeOK {
			item.BeforeFingerprint = contentFingerprint(before)
		}
		if afterOK {
			item.AfterFingerprint = contentFingerprint(after)
		}
		if textFile(before) && textFile(after) && len(before) <= maxDiffFileBytes &&
			len(after) <= maxDiffFileBytes && textBudget+len(before)+len(after) <= maxDiffTextBytes {
			item.BeforeText, item.AfterText = string(before), string(after)
			textBudget += len(before) + len(after)
		} else {
			item.Binary = true
		}
		result = append(result, item)
	}
	return result, nil
}

func textFile(value []byte) bool {
	if len(value) == 0 {
		return true
	}
	if !utf8.Valid(value) || bytes.IndexByte(value, 0) >= 0 {
		return false
	}
	for _, character := range string(value) {
		if unicode.IsControl(character) && character != '\n' && character != '\r' && character != '\t' {
			return false
		}
	}
	return true
}

func uniqueBoundedStrings(values []string, maximum int) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" || len(value) > maximum || strings.TrimSpace(value) != value {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validEvidenceReference(item EvidenceRef) bool {
	switch item.Kind {
	case EvidenceSourcePackage:
		return digestPattern.MatchString(item.Ref) && item.Ref == item.Fingerprint &&
			len(item.Paths) == 1 && item.Paths[0] == "*"
	case EvidenceRunEvent:
		if !validID(item.Ref, "event") {
			return false
		}
		for _, path := range item.Paths {
			if !validDraftPath(path) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func validDraftPath(value string) bool {
	if value == "" || len(value) > 512 || strings.TrimSpace(value) != value ||
		strings.HasPrefix(value, "/") || strings.Contains(value, `\`) {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == ".." {
			return false
		}
	}
	return true
}

func draftFingerprint(spec DraftSpec) (string, []byte, error) {
	encoded, err := json.Marshal(spec)
	if err != nil || len(encoded) > 256<<10 {
		return "", nil, fmt.Errorf("encode Draft spec: %w", ErrInvalidInput)
	}
	return fingerprint("neo-agent-learning-draft-v1", encoded), encoded, nil
}
