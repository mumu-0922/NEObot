package skillsupply

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	MaxSourceArchiveBytes = int64(32 << 20)
	maxExpandedBytes      = int64(128 << 20)
	maxFileBytes          = int64(16 << 20)
	maxPackageFiles       = 2048
	maxExpansionRatio     = int64(200)
)

type packageFile struct {
	path string
	data []byte
}

func ValidateArchive(source ArchiveSource) (ValidatedPackage, error) {
	if len(source.Data) == 0 || int64(len(source.Data)) > MaxSourceArchiveBytes ||
		!validSourceType(source.Type) || strings.TrimSpace(source.Ref) == "" {
		return ValidatedPackage{}, ErrArchiveInvalid
	}
	files, expanded, err := readArchiveFiles(source)
	if err != nil {
		return ValidatedPackage{}, err
	}
	metadata, err := parseSkillMarkdown(files)
	if err != nil {
		return ValidatedPackage{}, err
	}
	expectedName := strings.TrimSpace(source.ExpectedName)
	if expectedName != "" && metadata.Name != expectedName {
		return ValidatedPackage{}, ErrManifestInvalid
	}
	manifest, err := parseRuntimeManifest(files, metadata.Name)
	if err != nil {
		return ValidatedPackage{}, err
	}
	version := source.Version
	if version == "" {
		version = metadata.Metadata["version"]
	}
	if version == "" && manifest != nil {
		version = manifest.Package.Version
	}
	if version != "" && !semverPattern.MatchString(version) {
		return ValidatedPackage{}, ErrManifestInvalid
	}
	if manifest != nil && version != "" && manifest.Package.Version != version {
		return ValidatedPackage{}, ErrManifestInvalid
	}

	inventory, fingerprint := inventoryFingerprint(files)
	if version == "" {
		version = "0.0.0+" + strings.TrimPrefix(fingerprint, "sha256:")[:12]
	}
	runtimeFingerprint := ""
	if manifest != nil {
		runtimeFingerprint, err = fingerprintRuntime(fingerprint, *manifest)
		if err != nil {
			return ValidatedPackage{}, ErrManifestInvalid
		}
	}
	canonicalArchive, err := writeCanonicalArchive(files)
	if err != nil {
		return ValidatedPackage{}, ErrArchiveInvalid
	}
	if manifest != nil && (int64(len(files)) > manifest.Limits.MaxPackageFiles ||
		int64(len(canonicalArchive)) > manifest.Limits.MaxPackageBytes ||
		expanded > manifest.Limits.MaxExpandedBytes) {
		return ValidatedPackage{}, ErrManifestInvalid
	}
	sbom, sbomFingerprint, err := buildSBOM(metadata.Name, version, fingerprint, inventory, manifest)
	if err != nil {
		return ValidatedPackage{}, ErrManifestInvalid
	}
	allowedTools := nonNilStrings(metadata.AllowedTools)
	capabilityRequests := []CapabilityRequest{}
	if manifest != nil {
		capabilityRequests = append(capabilityRequests, manifest.CapabilityRequests...)
	}
	return ValidatedPackage{
		SourceArtifactSHA256: sha256Fingerprint(source.Data),
		CanonicalArchive:     canonicalArchive,
		SBOM:                 sbom,
		Inventory:            inventory,
		Package: PackageVersion{
			PackageFingerprint: fingerprint, RuntimeBundleFingerprint: runtimeFingerprint,
			SBOMFingerprint: sbomFingerprint, Name: metadata.Name, Version: version,
			Description: metadata.Description, License: metadata.License,
			Compatibility: metadata.Compatibility, AllowedTools: allowedTools,
			CapabilityRequests: capabilityRequests, HasRuntime: manifest != nil,
			FileCount: len(files), PackageBytes: int64(len(canonicalArchive)),
			ExpandedBytes: expanded, Manifest: manifest,
		},
	}, nil
}

func readArchiveFiles(source ArchiveSource) ([]packageFile, int64, error) {
	reader, err := zip.NewReader(bytes.NewReader(source.Data), int64(len(source.Data)))
	if err != nil || len(reader.File) == 0 || len(reader.File) > maxPackageFiles*2 {
		return nil, 0, ErrArchiveInvalid
	}
	prefix := cleanPrefix(source.StripPrefix)
	if source.StripPrefix != "" && prefix == "" {
		return nil, 0, ErrInvalidSource
	}
	rawFiles := make([]packageFile, 0, min(len(reader.File), maxPackageFiles))
	var expanded int64
	for _, entry := range reader.File {
		if entry == nil {
			return nil, 0, ErrArchiveInvalid
		}
		if entry.NonUTF8 || entry.Flags&0x1 != 0 {
			return nil, 0, ErrArchiveInvalid
		}
		if entry.FileInfo().IsDir() {
			name := strings.TrimSuffix(entry.Name, "/")
			if name == entry.Name || !validArchivePath(name) {
				return nil, 0, ErrArchiveInvalid
			}
			continue
		}
		if !validArchivePath(entry.Name) {
			return nil, 0, ErrArchiveInvalid
		}
		name := entry.Name
		if prefix != "" {
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			if name == prefix {
				return nil, 0, ErrArchiveInvalid
			}
			name = strings.TrimPrefix(name, prefix)
		}
		if !entry.Mode().IsRegular() || entry.UncompressedSize64 > uint64(maxFileBytes) ||
			entry.CompressedSize64 == 0 && entry.UncompressedSize64 > 0 {
			return nil, 0, ErrArchiveInvalid
		}
		if entry.CompressedSize64 > 0 && entry.UncompressedSize64 > uint64(maxExpansionRatio)*entry.CompressedSize64 {
			return nil, 0, ErrArchiveInvalid
		}
		expanded += int64(entry.UncompressedSize64)
		if expanded > maxExpandedBytes || len(rawFiles) >= maxPackageFiles {
			return nil, 0, ErrArchiveInvalid
		}
		body, readErr := readZipEntry(entry)
		if readErr != nil || int64(len(body)) != int64(entry.UncompressedSize64) {
			return nil, 0, ErrArchiveInvalid
		}
		rawFiles = append(rawFiles, packageFile{path: name, data: body})
	}
	if len(rawFiles) == 0 {
		return nil, 0, ErrArchiveInvalid
	}
	if err := requireRootSkill(rawFiles); err != nil {
		return nil, 0, err
	}
	seen := map[string]struct{}{}
	fold := cases.Fold()
	for _, file := range rawFiles {
		collisionKey := fold.String(norm.NFC.String(file.path))
		if _, exists := seen[collisionKey]; exists {
			return nil, 0, ErrArchiveInvalid
		}
		seen[collisionKey] = struct{}{}
	}
	sort.Slice(rawFiles, func(i, j int) bool { return rawFiles[i].path < rawFiles[j].path })
	return rawFiles, expanded, nil
}

func requireRootSkill(files []packageFile) error {
	rootCount := 0
	for _, file := range files {
		if file.path == "SKILL.md" {
			rootCount++
		} else if strings.HasSuffix(file.path, "/SKILL.md") {
			return ErrArchiveInvalid
		}
	}
	if rootCount != 1 {
		return ErrArchiveInvalid
	}
	return nil
}

func inventoryFingerprint(files []packageFile) ([]FileInventory, string) {
	inventory := make([]FileInventory, 0, len(files))
	digest := sha256.New()
	_, _ = digest.Write([]byte("neo.skill-package/v1\x00"))
	for _, file := range files {
		sum := sha256.Sum256(file.data)
		hash := hex.EncodeToString(sum[:])
		inventory = append(inventory, FileInventory{Path: file.path, Size: int64(len(file.data)), SHA256: "sha256:" + hash})
		_, _ = fmt.Fprintf(digest, "%d:%s\x00%d\x00%s\n", len(file.path), file.path, len(file.data), hash)
	}
	return inventory, "sha256:" + hex.EncodeToString(digest.Sum(nil))
}

func fingerprintRuntime(packageFingerprint string, manifest RuntimeManifest) (string, error) {
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte("neo.skill-runtime-bundle/v1\x00" + packageFingerprint + "\x00"))
	_, _ = digest.Write(encoded)
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}

func writeCanonicalArchive(files []packageFile) ([]byte, error) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	fixed := time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
	for _, file := range files {
		header := &zip.FileHeader{Name: file.path, Method: zip.Store, Modified: fixed}
		header.SetMode(0o644)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write(file.data); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func readZipEntry(entry *zip.File) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxFileBytes+1))
	if err != nil || int64(len(data)) > maxFileBytes {
		return nil, ErrArchiveInvalid
	}
	return data, nil
}

func validArchivePath(value string) bool {
	if value == "" || len(value) > 512 || !utf8.ValidString(value) || value != norm.NFC.String(value) ||
		strings.ContainsAny(value, "\x00\\") || strings.HasPrefix(value, "/") || path.Clean(value) != value ||
		value == "." || strings.HasPrefix(value, "../") || strings.Contains(value, "/../") ||
		strings.Contains(value, "//") || strings.Contains(value, ":") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." || len(part) > 255 {
			return false
		}
	}
	return true
}

func cleanPrefix(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(value, "/"))
	if value == "" {
		return ""
	}
	if !strings.HasSuffix(value, "/") {
		value += "/"
	}
	if !validArchivePath(strings.TrimSuffix(value, "/")) {
		return ""
	}
	return value
}

func validSourceType(value string) bool {
	return value == SourceOfficial || value == SourceLobeHub || value == SourceGit ||
		value == SourceZIP || value == SourceLearning
}

func sha256Fingerprint(data []byte) string {
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func pageCount(total, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return (total + pageSize - 1) / pageSize
}
