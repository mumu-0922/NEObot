package skillsupply

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxRuntimeViewBytes = int64(512 << 10)

// RuntimeSkill is the immutable, owner-installed package projection available
// to the local direct Chat runtime. RootPath is server-local authority and must
// never be serialized to an API response or persisted in chat metadata.
type RuntimeSkill struct {
	Name               string
	Description        string
	Version            string
	PackageFingerprint string
	RootPath           string `json:"-"`
	Files              []string
}

type runtimeMarker struct {
	SchemaVersion      string   `json:"schemaVersion"`
	PackageFingerprint string   `json:"packageFingerprint"`
	Files              []string `json:"files"`
}

// PrepareRuntimeSkills resolves the current owner's installations, revalidates
// the exact canonical package objects, and atomically materializes immutable
// package directories for progressive Skill loading and local command use.
func (service *Service) PrepareRuntimeSkills(
	ctx context.Context,
	userID, runtimeRoot string,
) ([]RuntimeSkill, error) {
	if service == nil || service.repository == nil || service.objects == nil {
		return nil, ErrRuntimeUnavailable
	}
	userID = strings.TrimSpace(userID)
	runtimeRoot = filepath.Clean(strings.TrimSpace(runtimeRoot))
	if userID == "" || !filepath.IsAbs(runtimeRoot) || runtimeRoot == string(filepath.Separator) {
		return nil, ErrRuntimeUnavailable
	}
	installations, err := service.repository.ListLibrary(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(installations) == 0 {
		return []RuntimeSkill{}, nil
	}
	service.runtimeMu.Lock()
	defer service.runtimeMu.Unlock()
	if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		return nil, ErrRuntimeUnavailable
	}
	result := make([]RuntimeSkill, 0, len(installations))
	for _, installation := range installations {
		candidate, err := service.repository.GetStoreItem(ctx, installation.AdmissionID)
		if err != nil {
			return nil, err
		}
		if candidate.Status != StatusAdmitted ||
			candidate.Package.PackageFingerprint != installation.PackageFingerprint ||
			candidate.Package.Name != installation.Name {
			return nil, ErrPackageChanged
		}
		root, files, err := service.materializeRuntimePackage(ctx, runtimeRoot, candidate.Package)
		if err != nil {
			return nil, err
		}
		result = append(result, RuntimeSkill{
			Name: installation.Name, Description: installation.Description,
			Version: installation.Version, PackageFingerprint: installation.PackageFingerprint,
			RootPath: root, Files: files,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (service *Service) materializeRuntimePackage(
	ctx context.Context,
	runtimeRoot string,
	packageVersion PackageVersion,
) (string, []string, error) {
	digest := strings.TrimPrefix(packageVersion.PackageFingerprint, "sha256:")
	if len(digest) != 64 || strings.Trim(digest, "0123456789abcdef") != "" {
		return "", nil, ErrPackageChanged
	}
	root := filepath.Join(runtimeRoot, digest)
	if files, ok := validRuntimeMarker(root, packageVersion.PackageFingerprint); ok {
		return root, files, nil
	}
	reader, info, err := service.objects.Get(ctx, packageVersion.PackageObjectKey)
	if err != nil {
		return "", nil, ErrRuntimeUnavailable
	}
	defer reader.Close()
	if info.Size != packageVersion.PackageBytes || info.Size < 1 || info.Size > MaxSourceArchiveBytes {
		return "", nil, ErrPackageChanged
	}
	archive, err := io.ReadAll(io.LimitReader(reader, info.Size+1))
	if err != nil || int64(len(archive)) != info.Size {
		return "", nil, ErrRuntimeUnavailable
	}
	validated, err := ValidateArchive(ArchiveSource{
		Type: SourceZIP, Ref: "runtime:" + packageVersion.PackageFingerprint,
		ExpectedName: packageVersion.Name, Data: archive,
	})
	if err != nil || validated.Package.PackageFingerprint != packageVersion.PackageFingerprint {
		return "", nil, ErrPackageChanged
	}
	if err := removeRuntimeDirectory(runtimeRoot, root); err != nil {
		return "", nil, ErrRuntimeUnavailable
	}
	temporary, err := os.MkdirTemp(runtimeRoot, ".skill-"+digest[:12]+"-")
	if err != nil {
		return "", nil, ErrRuntimeUnavailable
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(temporary)
		}
	}()
	files, err := writeRuntimeArchive(temporary, validated.CanonicalArchive)
	if err != nil {
		return "", nil, err
	}
	marker := runtimeMarker{
		SchemaVersion:      "neo.local-skill-cache/v1",
		PackageFingerprint: packageVersion.PackageFingerprint,
		Files:              files,
	}
	encoded, err := json.Marshal(marker)
	if err != nil || os.WriteFile(filepath.Join(temporary, ".neo-skill.json"), encoded, 0o600) != nil {
		return "", nil, ErrRuntimeUnavailable
	}
	if err := os.Chmod(temporary, 0o700); err != nil || os.Rename(temporary, root) != nil {
		return "", nil, ErrRuntimeUnavailable
	}
	cleanup = false
	return root, files, nil
}

// ReadRuntimeSkillFile reads one exact file from a previously prepared package.
// It rechecks the immutable file inventory before touching disk and never
// accepts absolute paths or traversal.
func ReadRuntimeSkillFile(skill RuntimeSkill, relativePath string) ([]byte, error) {
	relativePath = strings.TrimSpace(relativePath)
	if !validArchivePath(relativePath) || !runtimeFileListed(skill.Files, relativePath) ||
		!filepath.IsAbs(skill.RootPath) {
		return nil, ErrRuntimeFileNotFound
	}
	if err := ValidateRuntimeSkill(skill); err != nil {
		return nil, err
	}
	root := filepath.Clean(skill.RootPath)
	target := filepath.Join(root, filepath.FromSlash(relativePath))
	if !pathWithin(root, target) {
		return nil, ErrRuntimeFileNotFound
	}
	info, err := os.Lstat(target)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxRuntimeViewBytes {
		return nil, ErrRuntimeFileNotFound
	}
	data, err := os.ReadFile(target)
	if err != nil || int64(len(data)) != info.Size() {
		return nil, ErrRuntimeUnavailable
	}
	return data, nil
}

// ValidateRuntimeSkill rehashes the exact inventory immediately before Chat
// exposes a prepared package to a terminal command.
func ValidateRuntimeSkill(skill RuntimeSkill) error {
	root := filepath.Clean(strings.TrimSpace(skill.RootPath))
	if !filepath.IsAbs(root) || root == string(filepath.Separator) ||
		strings.TrimSpace(skill.PackageFingerprint) == "" {
		return ErrPackageChanged
	}
	fingerprint, err := runtimeTreeFingerprint(root, skill.Files)
	if err != nil {
		return err
	}
	if fingerprint != skill.PackageFingerprint {
		return ErrPackageChanged
	}
	return nil
}

func writeRuntimeArchive(root string, archive []byte) ([]string, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, ErrPackageChanged
	}
	files := make([]string, 0, len(reader.File))
	for _, entry := range reader.File {
		if entry == nil || entry.Name == ".neo-skill.json" || entry.FileInfo().IsDir() ||
			!validArchivePath(entry.Name) ||
			!entry.Mode().IsRegular() || entry.UncompressedSize64 > uint64(maxFileBytes) {
			return nil, ErrPackageChanged
		}
		target := filepath.Join(root, filepath.FromSlash(entry.Name))
		if !pathWithin(root, target) || os.MkdirAll(filepath.Dir(target), 0o700) != nil {
			return nil, ErrRuntimeUnavailable
		}
		body, err := readZipEntry(entry)
		if err != nil {
			return nil, ErrPackageChanged
		}
		mode := os.FileMode(0o600)
		if strings.HasPrefix(entry.Name, "scripts/") {
			mode = 0o700
		}
		if err := os.WriteFile(target, body, mode); err != nil {
			return nil, ErrRuntimeUnavailable
		}
		files = append(files, entry.Name)
	}
	sort.Strings(files)
	if len(files) == 0 || !runtimeFileListed(files, "SKILL.md") {
		return nil, ErrPackageChanged
	}
	return files, nil
}

func validRuntimeMarker(root, fingerprint string) ([]string, bool) {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, false
	}
	markerPath := filepath.Join(root, ".neo-skill.json")
	markerInfo, err := os.Lstat(markerPath)
	if err != nil || !markerInfo.Mode().IsRegular() || markerInfo.Size() < 1 ||
		markerInfo.Size() > 1<<20 {
		return nil, false
	}
	encoded, err := os.ReadFile(markerPath)
	if err != nil || int64(len(encoded)) != markerInfo.Size() {
		return nil, false
	}
	var marker runtimeMarker
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&marker) != nil || marker.SchemaVersion != "neo.local-skill-cache/v1" ||
		marker.PackageFingerprint != fingerprint || len(marker.Files) == 0 ||
		!runtimeFileListed(marker.Files, "SKILL.md") {
		return nil, false
	}
	for index, name := range marker.Files {
		if !validArchivePath(name) || (index > 0 && marker.Files[index-1] >= name) {
			return nil, false
		}
		fileInfo, statErr := os.Lstat(filepath.Join(root, filepath.FromSlash(name)))
		if statErr != nil || !fileInfo.Mode().IsRegular() {
			return nil, false
		}
	}
	if current, fingerprintErr := runtimeTreeFingerprint(root, marker.Files); fingerprintErr != nil || current != fingerprint {
		return nil, false
	}
	return append([]string(nil), marker.Files...), true
}

func runtimeTreeFingerprint(root string, files []string) (string, error) {
	if err := validateRuntimeTreeInventory(root, files); err != nil {
		return "", err
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte("neo.skill-package/v1\x00"))
	for index, name := range files {
		if !validArchivePath(name) || name == ".neo-skill.json" ||
			(index > 0 && files[index-1] >= name) {
			return "", ErrPackageChanged
		}
		target := filepath.Join(root, filepath.FromSlash(name))
		if !pathWithin(root, target) {
			return "", ErrPackageChanged
		}
		info, err := os.Lstat(target)
		if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxFileBytes {
			return "", ErrPackageChanged
		}
		body, err := os.ReadFile(target)
		if err != nil || int64(len(body)) != info.Size() {
			return "", ErrRuntimeUnavailable
		}
		sum := sha256.Sum256(body)
		_, _ = fmt.Fprintf(digest, "%d:%s\x00%d\x00%s\n", len(name), name, len(body), hex.EncodeToString(sum[:]))
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}

func validateRuntimeTreeInventory(root string, files []string) error {
	expectedFiles := make(map[string]struct{}, len(files))
	expectedDirectories := map[string]struct{}{".": {}}
	for _, name := range files {
		if !validArchivePath(name) || name == ".neo-skill.json" {
			return ErrPackageChanged
		}
		expectedFiles[name] = struct{}{}
		for directory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(name))); directory != "."; directory = filepath.ToSlash(filepath.Dir(filepath.FromSlash(directory))) {
			expectedDirectories[directory] = struct{}{}
		}
	}
	seenFiles := make(map[string]struct{}, len(files))
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return ErrRuntimeUnavailable
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return ErrRuntimeUnavailable
		}
		relative = filepath.ToSlash(relative)
		if relative == ".neo-skill.json" {
			if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
				return ErrPackageChanged
			}
			return nil
		}
		if entry.IsDir() {
			if _, ok := expectedDirectories[relative]; !ok {
				return ErrPackageChanged
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return ErrPackageChanged
		}
		if _, ok := expectedFiles[relative]; !ok {
			return ErrPackageChanged
		}
		seenFiles[relative] = struct{}{}
		return nil
	})
	if err != nil {
		return err
	}
	if len(seenFiles) != len(expectedFiles) {
		return ErrPackageChanged
	}
	return nil
}

func removeRuntimeDirectory(runtimeRoot, target string) error {
	if !pathWithin(runtimeRoot, target) || filepath.Clean(runtimeRoot) == filepath.Clean(target) {
		return ErrRuntimeUnavailable
	}
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrRuntimeUnavailable
	}
	return os.RemoveAll(target)
}

func runtimeFileListed(files []string, name string) bool {
	index := sort.SearchStrings(files, name)
	return index < len(files) && files[index] == name
}

func pathWithin(root, target string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
