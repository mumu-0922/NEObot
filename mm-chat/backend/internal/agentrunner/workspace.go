package agentrunner

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unicode/utf8"

	"golang.org/x/sys/unix"
	"golang.org/x/text/unicode/norm"
)

const (
	maxWorkspaceFiles     = 20_000
	maxWorkspaceBytes     = int64(512 << 20)
	maxWorkspaceFileBytes = int64(64 << 20)
)

type Workspace struct {
	ID          string
	Fingerprint string
	Path        string
	FileCount   int
	ByteCount   int64
}

type WorkspaceCatalog struct{ root string }

func NewWorkspaceCatalog(root string) (*WorkspaceCatalog, error) {
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) || root == string(filepath.Separator) {
		return nil, ErrInvalidInput
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return nil, ErrInvalidInput
	}
	return &WorkspaceCatalog{root: root}, nil
}

func (catalog *WorkspaceCatalog) Register(ctx context.Context, id, expectedFingerprint string, archive io.Reader) (Workspace, error) {
	if catalog == nil || !validID(id, "workspace_snapshot") || !validFingerprint(expectedFingerprint) || archive == nil {
		return Workspace{}, ErrInvalidInput
	}
	finalPath, err := catalog.snapshotPath(id, expectedFingerprint)
	if err != nil {
		return Workspace{}, err
	}
	if existing, err := inspectWorkspace(finalPath, id, expectedFingerprint); err == nil {
		return existing, nil
	}
	staging, err := os.MkdirTemp(catalog.root, ".workspace-stage-")
	if err != nil {
		return Workspace{}, ErrRuntimeUnavailable
	}
	if err := os.Chmod(staging, 0o700); err != nil {
		os.RemoveAll(staging)
		return Workspace{}, ErrRuntimeUnavailable
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = safeRemoveTree(catalog.root, staging)
		}
	}()
	workspace, err := materializeWorkspace(ctx, staging, id, archive)
	if err != nil {
		return Workspace{}, err
	}
	if workspace.Fingerprint != expectedFingerprint {
		return Workspace{}, ErrSnapshotMismatch
	}
	if err := makeTreeReadOnly(staging); err != nil {
		return Workspace{}, ErrRuntimeUnavailable
	}
	if err := os.Rename(staging, finalPath); err != nil {
		if existing, inspectErr := inspectWorkspace(finalPath, id, expectedFingerprint); inspectErr == nil {
			return existing, nil
		}
		return Workspace{}, ErrRuntimeUnavailable
	}
	cleanup = false
	if err := syncDirectory(catalog.root); err != nil {
		return Workspace{}, ErrRuntimeUnavailable
	}
	workspace.Path = finalPath
	return workspace, nil
}

func (catalog *WorkspaceCatalog) Resolve(id, expectedFingerprint string) (Workspace, error) {
	if catalog == nil || !validID(id, "workspace_snapshot") || !validFingerprint(expectedFingerprint) {
		return Workspace{}, ErrInvalidInput
	}
	path, err := catalog.snapshotPath(id, expectedFingerprint)
	if err != nil {
		return Workspace{}, err
	}
	return inspectWorkspace(path, id, expectedFingerprint)
}

func (catalog *WorkspaceCatalog) Remove(id, expectedFingerprint string) error {
	if catalog == nil || !validID(id, "workspace_snapshot") || !validFingerprint(expectedFingerprint) {
		return ErrInvalidInput
	}
	path, err := catalog.snapshotPath(id, expectedFingerprint)
	if err != nil {
		return err
	}
	if err := makeTreeOwnerWritable(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return safeRemoveTree(catalog.root, path)
}

func (catalog *WorkspaceCatalog) snapshotPath(id, fingerprint string) (string, error) {
	digest := strings.TrimPrefix(fingerprint, "sha256:")
	path := filepath.Join(catalog.root, id+"-"+digest)
	if !pathWithin(catalog.root, path) {
		return "", ErrInvalidInput
	}
	return path, nil
}

func materializeWorkspace(ctx context.Context, root, id string, source io.Reader) (Workspace, error) {
	reader := tar.NewReader(io.LimitReader(source, maxWorkspaceBytes+(64<<20)))
	type fileFact struct {
		path   string
		size   int64
		digest string
	}
	facts := make([]fileFact, 0)
	seen := map[string]bool{}
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return Workspace{}, err
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Workspace{}, ErrSnapshotMismatch
		}
		path, collisionKey, err := canonicalWorkspacePath(header.Name)
		if err != nil {
			return Workspace{}, err
		}
		isDirectory := header.Typeflag == tar.TypeDir
		if _, duplicate := seen[collisionKey]; duplicate {
			return Workspace{}, ErrSnapshotMismatch
		}
		if !isDirectory {
			for parent := filepath.ToSlash(filepath.Dir(path)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
				if directory, occupied := seen[strings.ToLower(parent)]; occupied && !directory {
					return Workspace{}, ErrSnapshotMismatch
				}
			}
		}
		seen[collisionKey] = isDirectory
		if len(seen) > maxWorkspaceFiles {
			return Workspace{}, ErrSnapshotMismatch
		}
		target := filepath.Join(root, filepath.FromSlash(path))
		if !pathWithin(root, target) {
			return Workspace{}, ErrSnapshotMismatch
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if header.Size != 0 || ensureSafeDirectory(root, target) != nil {
				return Workspace{}, ErrSnapshotMismatch
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > maxWorkspaceFileBytes || total+header.Size > maxWorkspaceBytes {
				return Workspace{}, ErrSnapshotMismatch
			}
			if err := ensureSafeDirectory(root, filepath.Dir(target)); err != nil {
				return Workspace{}, ErrSnapshotMismatch
			}
			digest, err := writeWorkspaceFile(target, reader, header.Size)
			if err != nil {
				return Workspace{}, err
			}
			total += header.Size
			facts = append(facts, fileFact{path: path, size: header.Size, digest: digest})
		default:
			return Workspace{}, ErrSnapshotMismatch
		}
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].path < facts[j].path })
	digest := sha256.New()
	_, _ = digest.Write([]byte("neo-workspace-tree-v1\x00"))
	for _, fact := range facts {
		_, _ = fmt.Fprintf(digest, "%s\x00%d\x00%s\x00", fact.path, fact.size, fact.digest)
	}
	return Workspace{ID: id, Fingerprint: "sha256:" + hex.EncodeToString(digest.Sum(nil)), Path: root,
		FileCount: len(facts), ByteCount: total}, nil
}

func canonicalWorkspacePath(value string) (string, string, error) {
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') || strings.Contains(value, "\\") {
		return "", "", ErrSnapshotMismatch
	}
	normalized := norm.NFC.String(value)
	clean := filepath.ToSlash(filepath.Clean(normalized))
	if filepath.IsAbs(normalized) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") ||
		strings.HasPrefix(clean, "/") || clean != strings.TrimSuffix(normalized, "/") {
		return "", "", ErrSnapshotMismatch
	}
	for _, part := range strings.Split(clean, "/") {
		if part == "" || part == "." || part == ".." || len(part) > 255 {
			return "", "", ErrSnapshotMismatch
		}
	}
	return clean, strings.ToLower(clean), nil
}

func ensureSafeDirectory(root, target string) error {
	if filepath.Clean(root) == filepath.Clean(target) {
		return nil
	}
	if !pathWithin(root, target) {
		return ErrSnapshotMismatch
	}
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if err := os.Mkdir(current, 0o700); err != nil {
				return err
			}
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return ErrSnapshotMismatch
		}
	}
	return nil
}

func writeWorkspaceFile(path string, source io.Reader, size int64) (string, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return "", ErrSnapshotMismatch
	}
	digest := sha256.New()
	written, copyErr := io.CopyN(io.MultiWriter(file, digest), source, size)
	syncErr := file.Sync()
	closeErr := file.Close()
	if copyErr != nil || written != size || syncErr != nil || closeErr != nil {
		return "", ErrSnapshotMismatch
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func inspectWorkspace(path, id, fingerprint string) (Workspace, error) {
	type fileFact struct {
		path   string
		size   int64
		digest string
	}
	facts := make([]fileFact, 0)
	seen := map[string]struct{}{}
	var total int64
	err := filepath.Walk(path, func(current string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.Mode()&os.ModeSymlink != 0 || !runnerOwned(info) {
			return ErrSnapshotMismatch
		}
		if current == path {
			if !info.IsDir() || info.Mode().Perm() != 0o500 {
				return ErrSnapshotMismatch
			}
			return nil
		}
		relative, relErr := filepath.Rel(path, current)
		if relErr != nil {
			return ErrSnapshotMismatch
		}
		canonical, collisionKey, canonicalErr := canonicalWorkspacePath(filepath.ToSlash(relative))
		if canonicalErr != nil || canonical != filepath.ToSlash(relative) {
			return ErrSnapshotMismatch
		}
		if _, duplicate := seen[collisionKey]; duplicate {
			return ErrSnapshotMismatch
		}
		seen[collisionKey] = struct{}{}
		if len(seen) > maxWorkspaceFiles {
			return ErrSnapshotMismatch
		}
		if info.IsDir() {
			if info.Mode().Perm() != 0o500 {
				return ErrSnapshotMismatch
			}
			return nil
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o400 ||
			info.Size() < 0 || info.Size() > maxWorkspaceFileBytes || total+info.Size() > maxWorkspaceBytes {
			return ErrSnapshotMismatch
		}
		digest, digestErr := fingerprintWorkspaceFile(current, info.Size())
		if digestErr != nil {
			return digestErr
		}
		total += info.Size()
		facts = append(facts, fileFact{path: canonical, size: info.Size(), digest: digest})
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return Workspace{}, ErrNotFound
		}
		return Workspace{}, ErrSnapshotMismatch
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].path < facts[j].path })
	digest := sha256.New()
	_, _ = digest.Write([]byte("neo-workspace-tree-v1\x00"))
	for _, fact := range facts {
		_, _ = fmt.Fprintf(digest, "%s\x00%d\x00%s\x00", fact.path, fact.size, fact.digest)
	}
	actual := "sha256:" + hex.EncodeToString(digest.Sum(nil))
	if actual != fingerprint {
		return Workspace{}, ErrSnapshotMismatch
	}
	return Workspace{ID: id, Fingerprint: actual, Path: path, FileCount: len(facts), ByteCount: total}, nil
}

func runnerOwned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid()) && stat.Gid == uint32(os.Getegid())
}

func fingerprintWorkspaceFile(path string, expectedSize int64) (string, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return "", ErrSnapshotMismatch
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return "", ErrSnapshotMismatch
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o400 ||
		info.Size() != expectedSize || !runnerOwned(info) {
		return "", ErrSnapshotMismatch
	}
	digest := sha256.New()
	written, err := io.Copy(digest, io.LimitReader(file, maxWorkspaceFileBytes+1))
	if err != nil || written != expectedSize {
		return "", ErrSnapshotMismatch
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func makeTreeReadOnly(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return ErrSnapshotMismatch
		}
		if info.IsDir() {
			return os.Chmod(path, 0o500)
		}
		if !info.Mode().IsRegular() {
			return ErrSnapshotMismatch
		}
		return os.Chmod(path, 0o400)
	})
}

func makeTreeOwnerWritable(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrSnapshotMismatch
		}
		if info.IsDir() {
			return os.Chmod(path, 0o700)
		}
		if !info.Mode().IsRegular() {
			return ErrSnapshotMismatch
		}
		return os.Chmod(path, 0o600)
	})
}

func safeRemoveTree(root, target string) error {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if root == target || !pathWithin(root, target) {
		return ErrInvalidInput
	}
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return ErrInvalidInput
	}
	return os.RemoveAll(target)
}

func pathWithin(root, target string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func fingerprintWorkspaceFiles(files map[string][]byte) string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	digest := sha256.New()
	_, _ = digest.Write([]byte("neo-workspace-tree-v1\x00"))
	for _, path := range paths {
		fileDigest := sha256.Sum256(files[path])
		_, _ = fmt.Fprintf(digest, "%s\x00%d\x00%s\x00", path, len(files[path]), hex.EncodeToString(fileDigest[:]))
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil))
}

func scanTreeNames(root string) ([]string, error) {
	names := []string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		relative, _ := filepath.Rel(root, path)
		names = append(names, filepath.ToSlash(relative))
		return nil
	})
	return names, err
}
