package memoryauthor

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

func ValidateFormalRoot(root, repositoryRoot string) (string, error) {
	root = strings.TrimSpace(root)
	repositoryRoot = strings.TrimSpace(repositoryRoot)
	if root == "" || repositoryRoot == "" {
		return "", errors.New("authoring root and repository root are required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", errors.New("resolve authoring root")
	}
	absoluteRepository, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return "", errors.New("resolve repository root")
	}
	if hasForbiddenPathComponent(absoluteRoot) {
		return "", errors.New("authoring root uses a forbidden secrets or backup path")
	}
	if err := rejectSymlinkComponents(absoluteRoot); err != nil {
		return "", err
	}
	if pathWithin(absoluteRoot, absoluteRepository) {
		allowed := filepath.Join(absoluteRepository, "mm-chat", "data", "memory-benchmark")
		if !pathWithin(absoluteRoot, allowed) || absoluteRoot == allowed {
			return "", errors.New("repository-local authoring root must be a versioned directory under mm-chat/data/memory-benchmark")
		}
	} else if containingRepository := findContainingRepository(absoluteRoot); containingRepository != "" {
		return "", errors.New("authoring root cannot be inside another Git repository")
	}
	return absoluteRoot, nil
}

func PublishPool(root string, pool GeneratedPool) error {
	if err := ValidatePool(pool); err != nil {
		return err
	}
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "." || root == "" {
		return errors.New("candidate output root is invalid")
	}
	if _, err := os.Lstat(root); err == nil {
		return errors.New("candidate output root already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect candidate output root: %w", err)
	}
	parent := filepath.Dir(root)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return errors.New("create candidate output parent")
	}
	if err := rejectSymlinkComponents(parent); err != nil {
		return err
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		return fmt.Errorf("create candidate output root: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.WriteFile(filepath.Join(root, ".generation-incomplete"), []byte("incomplete\n"), 0o600)
		}
	}()
	if err := writeExclusiveBytes(filepath.Join(root, CandidateFixtureFile), pool.FixtureJSON); err != nil {
		return err
	}
	if err := writeExclusiveBytes(filepath.Join(root, CandidateGoldenFile), pool.GoldenJSON); err != nil {
		return err
	}
	if err := os.Mkdir(filepath.Join(root, ReviewDirectory), 0o700); err != nil {
		return errors.New("create review directory")
	}
	if err := os.Mkdir(filepath.Join(root, ReviewDirectory, ReviewEventsDirectory), 0o700); err != nil {
		return errors.New("create review events directory")
	}
	if err := writeExclusiveBytes(filepath.Join(root, CandidateManifestFile), pool.ManifestJSON); err != nil {
		return err
	}
	if err := syncDirectory(root); err != nil {
		return err
	}
	complete = true
	return nil
}

func LoadPool(root string) (GeneratedPool, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if err := validateSecureDirectory(root); err != nil {
		return GeneratedPool{}, fmt.Errorf("validate candidate root: %w", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".generation-incomplete")); err == nil {
		return GeneratedPool{}, errors.New("candidate generation is incomplete and cannot be resumed in place")
	} else if !errors.Is(err, os.ErrNotExist) {
		return GeneratedPool{}, errors.New("inspect candidate generation marker")
	}
	fixtureBody, err := readSecureArtifact(filepath.Join(root, CandidateFixtureFile))
	if err != nil {
		return GeneratedPool{}, fmt.Errorf("read candidate fixtures: %w", err)
	}
	goldenBody, err := readSecureArtifact(filepath.Join(root, CandidateGoldenFile))
	if err != nil {
		return GeneratedPool{}, fmt.Errorf("read candidate Golden: %w", err)
	}
	manifestBody, err := readSecureArtifact(filepath.Join(root, CandidateManifestFile))
	if err != nil {
		return GeneratedPool{}, fmt.Errorf("read candidate manifest: %w", err)
	}
	fixtures, err := DecodeFixtureManifest(bytes.NewReader(fixtureBody))
	if err != nil {
		return GeneratedPool{}, err
	}
	golden, err := memoryeval.DecodeGoldenSet(bytes.NewReader(goldenBody))
	if err != nil {
		return GeneratedPool{}, fmt.Errorf("decode candidate Golden: %w", err)
	}
	manifest, err := DecodeCandidateManifest(bytes.NewReader(manifestBody))
	if err != nil {
		return GeneratedPool{}, err
	}
	pool := GeneratedPool{
		FixtureManifest: fixtures,
		Golden:          golden,
		Manifest:        manifest,
		FixtureJSON:     fixtureBody,
		GoldenJSON:      goldenBody,
		ManifestJSON:    manifestBody,
	}
	if err := ValidatePool(pool); err != nil {
		return GeneratedPool{}, err
	}
	return pool, nil
}

func readSecureArtifact(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("artifact must be a private regular non-symlink file")
	}
	return readRegularFile(path)
}

func writeExclusiveBytes(path string, body []byte) error {
	return writeExclusiveBytesStaged(path, body, filepath.Dir(path))
}

func writeExclusiveBytesStaged(path string, body []byte, stagingDirectory string) error {
	if len(body) == 0 || len(body) > maximumArtifactBytes {
		return errors.New("exclusive artifact body is invalid")
	}
	directory := filepath.Dir(path)
	if err := validateSecureDirectory(directory); err != nil {
		return err
	}
	if err := validateSecureDirectory(stagingDirectory); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(stagingDirectory, ".memory-author-*.tmp")
	if err != nil {
		return errors.New("create artifact temporary file")
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return errors.New("secure artifact temporary file")
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return errors.New("write artifact temporary file")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return errors.New("sync artifact temporary file")
	}
	if err := temporary.Close(); err != nil {
		return errors.New("close artifact temporary file")
	}
	if err := os.Link(temporaryPath, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return errors.New("artifact already exists")
		}
		return errors.New("publish artifact exclusively")
	}
	return syncDirectory(directory)
}

func replacePrivateBytes(path string, body []byte) error {
	if len(body) == 0 || len(body) > maximumArtifactBytes {
		return errors.New("replacement artifact body is invalid")
	}
	directory := filepath.Dir(path)
	if err := validateSecureDirectory(directory); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("replacement target is not a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("inspect replacement target")
	}
	temporary, err := os.CreateTemp(directory, ".memory-author-replace-*.tmp")
	if err != nil {
		return errors.New("create replacement temporary file")
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return errors.New("secure replacement temporary file")
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return errors.New("write replacement temporary file")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return errors.New("sync replacement temporary file")
	}
	if err := temporary.Close(); err != nil {
		return errors.New("close replacement temporary file")
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return errors.New("publish replacement artifact")
	}
	return syncDirectory(directory)
}

func validateSecureDirectory(path string) error {
	if err := rejectSymlinkComponents(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("authoring directory must be private and must not be a symlink")
	}
	return nil
}

func rejectSymlinkComponents(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return errors.New("resolve authoring path")
	}
	volume := filepath.VolumeName(absolute)
	current := volume + string(filepath.Separator)
	relative := strings.TrimPrefix(absolute, current)
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect authoring path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("authoring path contains a symlink")
		}
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return errors.New("open artifact directory for sync")
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return errors.New("sync artifact directory")
	}
	return nil
}

func pathWithin(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func hasForbiddenPathComponent(path string) bool {
	for _, component := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		switch strings.ToLower(component) {
		case "secrets", "backup":
			return true
		}
	}
	return false
}

func findContainingRepository(path string) string {
	current := filepath.Clean(path)
	for {
		if _, err := os.Lstat(current); errors.Is(err, os.ErrNotExist) {
			current = filepath.Dir(current)
			continue
		}
		break
	}
	for {
		if _, err := os.Lstat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}
