package memorycapture

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maximumPublishedArtifactBytes = 128 << 20

type Artifact struct {
	Name string
	Body []byte
}

// PublishArtifactsExclusive publishes a complete private bundle without
// overwriting existing evidence. The run manifest is linked last as the
// commit marker; a failure removes only links created by this call.
func PublishArtifactsExclusive(directory string, artifacts []Artifact) (map[string]string, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" || len(artifacts) == 0 {
		return nil, ErrCaptureInvalid
	}
	ordered := append([]Artifact(nil), artifacts...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Name == "run-manifest.json" {
			return false
		}
		if ordered[j].Name == "run-manifest.json" {
			return true
		}
		return ordered[i].Name < ordered[j].Name
	})
	seen := make(map[string]struct{}, len(ordered))
	names := make([]string, len(ordered))
	for _, artifact := range ordered {
		if !validArtifactName(artifact.Name) || len(artifact.Body) == 0 ||
			len(artifact.Body) > maximumPublishedArtifactBytes {
			return nil, ErrCaptureInvalid
		}
		if _, duplicate := seen[artifact.Name]; duplicate {
			return nil, ErrCaptureInvalid
		}
		seen[artifact.Name] = struct{}{}
	}
	for index, artifact := range ordered {
		names[index] = artifact.Name
	}
	if err := PreflightArtifactOutputs(directory, names); err != nil {
		return nil, err
	}

	temporaryPaths := make([]string, 0, len(ordered))
	publishedPaths := make([]string, 0, len(ordered))
	cleanup := func() {
		for _, path := range temporaryPaths {
			_ = os.Remove(path)
		}
		for _, path := range publishedPaths {
			_ = os.Remove(path)
		}
	}
	digests := make(map[string]string, len(ordered))
	for _, artifact := range ordered {
		temporary, err := os.CreateTemp(directory, ".memory-regression-*.tmp")
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("%w: create artifact temporary file", ErrCaptureUnavailable)
		}
		temporaryPath := temporary.Name()
		temporaryPaths = append(temporaryPaths, temporaryPath)
		if err := temporary.Chmod(0o600); err != nil {
			_ = temporary.Close()
			cleanup()
			return nil, fmt.Errorf("%w: secure artifact temporary file", ErrCaptureUnavailable)
		}
		if _, err := temporary.Write(artifact.Body); err != nil {
			_ = temporary.Close()
			cleanup()
			return nil, fmt.Errorf("%w: write artifact temporary file", ErrCaptureUnavailable)
		}
		if err := temporary.Sync(); err != nil {
			_ = temporary.Close()
			cleanup()
			return nil, fmt.Errorf("%w: sync artifact temporary file", ErrCaptureUnavailable)
		}
		if err := temporary.Close(); err != nil {
			cleanup()
			return nil, fmt.Errorf("%w: close artifact temporary file", ErrCaptureUnavailable)
		}
		target := filepath.Join(directory, artifact.Name)
		if err := os.Link(temporaryPath, target); err != nil {
			cleanup()
			if errors.Is(err, os.ErrExist) {
				return nil, fmt.Errorf("%w: artifact output already exists", ErrCaptureStateConflict)
			}
			return nil, fmt.Errorf("%w: publish artifact", ErrCaptureUnavailable)
		}
		publishedPaths = append(publishedPaths, target)
		digest := sha256.Sum256(artifact.Body)
		digests[artifact.Name] = hex.EncodeToString(digest[:])
	}
	for _, path := range temporaryPaths {
		if err := os.Remove(path); err != nil {
			cleanup()
			return nil, fmt.Errorf("%w: remove artifact temporary file", ErrCaptureUnavailable)
		}
	}
	directoryHandle, err := os.Open(directory)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("%w: open artifact directory", ErrCaptureUnavailable)
	}
	if err := directoryHandle.Sync(); err != nil {
		_ = directoryHandle.Close()
		cleanup()
		return nil, fmt.Errorf("%w: sync artifact directory", ErrCaptureUnavailable)
	}
	if err := directoryHandle.Close(); err != nil {
		cleanup()
		return nil, fmt.Errorf("%w: close artifact directory", ErrCaptureUnavailable)
	}
	return digests, nil
}

// PreflightArtifactOutputs validates the private destination and refuses any
// existing target before a caller performs live Provider work. Publication
// still repeats the exclusive link check to close concurrent races.
func PreflightArtifactOutputs(directory string, names []string) error {
	directory = strings.TrimSpace(directory)
	if directory == "" || len(names) == 0 {
		return ErrCaptureInvalid
	}
	if err := ensurePrivateArtifactDirectory(directory); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if !validArtifactName(name) {
			return ErrCaptureInvalid
		}
		if _, duplicate := seen[name]; duplicate {
			return ErrCaptureInvalid
		}
		seen[name] = struct{}{}
		if _, err := os.Lstat(filepath.Join(directory, name)); err == nil {
			return fmt.Errorf("%w: artifact output already exists", ErrCaptureStateConflict)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: inspect artifact output", ErrCaptureInvalid)
		}
	}
	return nil
}

func ensurePrivateArtifactDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("%w: create artifact directory", ErrCaptureUnavailable)
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 ||
		info.Mode().Perm()&0o077 != 0 || info.Mode().Perm()&0o300 != 0o300 {
		return fmt.Errorf("%w: artifact directory is not private", ErrCaptureInvalid)
	}
	return nil
}

func validArtifactName(value string) bool {
	if value == "" || filepath.Base(value) != value || value == "." ||
		strings.TrimSpace(value) != value || len(value) > 128 {
		return false
	}
	for _, current := range value {
		if current >= 'a' && current <= 'z' || current >= '0' && current <= '9' ||
			current == '-' || current == '_' || current == '.' {
			continue
		}
		return false
	}
	return true
}
