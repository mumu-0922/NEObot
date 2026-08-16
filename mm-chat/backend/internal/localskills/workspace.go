package localskills

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	WorkspaceVersionAbsent = "absent"

	MaxWorkspaceFileBytes       = 2 << 20
	MaxWorkspaceReadWindowBytes = 24 << 10
	MaxWorkspaceWriteBytes      = 24 << 10
)

var (
	ErrWorkspaceInvalidPath     = errors.New("workspace path is invalid")
	ErrWorkspaceInvalidInput    = errors.New("workspace input is invalid")
	ErrWorkspaceFileNotFound    = errors.New("workspace file was not found")
	ErrWorkspaceFileTooLarge    = errors.New("workspace file is too large")
	ErrWorkspaceInvalidUTF8     = errors.New("workspace file is not valid UTF-8")
	ErrWorkspaceVersionConflict = errors.New("workspace file version conflicts with expected version")
	ErrWorkspaceEditConflict    = errors.New("workspace edit target is ambiguous or missing")
)

type FileReadRequest struct {
	Path   string
	Offset int
	Limit  int
}

type FileReadResult struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	Version    string `json:"version"`
	Size       int    `json:"size"`
	Offset     int    `json:"offset"`
	NextOffset int    `json:"nextOffset"`
	Truncated  bool   `json:"truncated"`
}

type FileWriteRequest struct {
	Path            string
	Content         string
	ExpectedVersion string
}

type FileWriteResult struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	Size    int    `json:"size"`
}

type FileEditRequest struct {
	Path            string
	OldText         string
	NewText         string
	ReplaceAll      bool
	ExpectedVersion string
}

// ReadWorkspaceFile reads a bounded UTF-8 window while returning a version for
// the complete file. Paths are always relative to the configured workspace.
func (executor *Executor) ReadWorkspaceFile(
	ctx context.Context,
	request FileReadRequest,
) (FileReadResult, error) {
	if !executor.Enabled() {
		return FileReadResult{}, ErrRuntimeFailed
	}
	name, err := cleanWorkspacePath(request.Path, false)
	if err != nil || request.Offset < 0 || request.Offset > MaxWorkspaceFileBytes ||
		request.Limit < 0 || request.Limit > MaxWorkspaceReadWindowBytes {
		return FileReadResult{}, ErrWorkspaceInvalidInput
	}
	if request.Limit == 0 {
		request.Limit = MaxWorkspaceReadWindowBytes
	}
	root, err := os.OpenRoot(executor.config.WorkspaceRoot)
	if err != nil {
		return FileReadResult{}, ErrRuntimeFailed
	}
	defer root.Close()
	body, err := readWorkspaceRegularFile(ctx, root, name)
	if err != nil {
		return FileReadResult{}, err
	}
	if request.Offset > len(body) {
		return FileReadResult{}, ErrWorkspaceInvalidInput
	}
	start := request.Offset
	for start < len(body) && !utf8.RuneStart(body[start]) {
		start++
	}
	end := min(start+request.Limit, len(body))
	for end > start && end < len(body) && !utf8.RuneStart(body[end]) {
		end--
	}
	return FileReadResult{
		Path: name, Content: string(body[start:end]), Version: workspaceVersion(body),
		Size: len(body), Offset: start, NextOffset: end, Truncated: end < len(body),
	}, nil
}

// WriteWorkspaceFile atomically creates or replaces one UTF-8 file. The
// expected version is mandatory: use "absent" only when creating a new file.
func (executor *Executor) WriteWorkspaceFile(
	ctx context.Context,
	request FileWriteRequest,
) (FileWriteResult, error) {
	if !executor.Enabled() {
		return FileWriteResult{}, ErrRuntimeFailed
	}
	name, err := cleanWorkspacePath(request.Path, false)
	if err != nil || !validWorkspaceVersion(request.ExpectedVersion) ||
		len(request.Content) > MaxWorkspaceWriteBytes || !utf8.ValidString(request.Content) ||
		strings.ContainsRune(request.Content, '\x00') {
		return FileWriteResult{}, ErrWorkspaceInvalidInput
	}
	return executor.writeWorkspaceFileCAS(
		ctx, name, []byte(request.Content), request.ExpectedVersion,
	)
}

// EditWorkspaceFile performs an exact text replacement over a version-pinned
// file and commits it through the same atomic compare-and-swap path as write.
func (executor *Executor) EditWorkspaceFile(
	ctx context.Context,
	request FileEditRequest,
) (FileWriteResult, error) {
	if !executor.Enabled() {
		return FileWriteResult{}, ErrRuntimeFailed
	}
	name, err := cleanWorkspacePath(request.Path, false)
	if err != nil || request.OldText == "" || !utf8.ValidString(request.OldText) ||
		!utf8.ValidString(request.NewText) || strings.ContainsRune(request.OldText, '\x00') ||
		strings.ContainsRune(request.NewText, '\x00') ||
		request.ExpectedVersion == WorkspaceVersionAbsent ||
		!validWorkspaceVersion(request.ExpectedVersion) {
		return FileWriteResult{}, ErrWorkspaceInvalidInput
	}

	executor.workspaceWriteMu.Lock()
	defer executor.workspaceWriteMu.Unlock()
	root, err := os.OpenRoot(executor.config.WorkspaceRoot)
	if err != nil {
		return FileWriteResult{}, ErrRuntimeFailed
	}
	defer root.Close()
	body, err := readWorkspaceRegularFile(ctx, root, name)
	if err != nil {
		return FileWriteResult{}, err
	}
	if workspaceVersion(body) != request.ExpectedVersion {
		return FileWriteResult{}, ErrWorkspaceVersionConflict
	}
	count := strings.Count(string(body), request.OldText)
	if count == 0 || (!request.ReplaceAll && count != 1) {
		return FileWriteResult{}, ErrWorkspaceEditConflict
	}
	limit := 1
	if request.ReplaceAll {
		limit = -1
	}
	updated := strings.Replace(string(body), request.OldText, request.NewText, limit)
	if len(updated) > MaxWorkspaceWriteBytes {
		return FileWriteResult{}, ErrWorkspaceFileTooLarge
	}
	return writeWorkspaceFileCASLocked(ctx, root, name, []byte(updated), request.ExpectedVersion)
}

func (executor *Executor) writeWorkspaceFileCAS(
	ctx context.Context,
	name string,
	body []byte,
	expectedVersion string,
) (FileWriteResult, error) {
	executor.workspaceWriteMu.Lock()
	defer executor.workspaceWriteMu.Unlock()
	root, err := os.OpenRoot(executor.config.WorkspaceRoot)
	if err != nil {
		return FileWriteResult{}, ErrRuntimeFailed
	}
	defer root.Close()
	return writeWorkspaceFileCASLocked(ctx, root, name, body, expectedVersion)
}

func writeWorkspaceFileCASLocked(
	ctx context.Context,
	root *os.Root,
	name string,
	body []byte,
	expectedVersion string,
) (FileWriteResult, error) {
	if err := ctx.Err(); err != nil {
		return FileWriteResult{}, err
	}
	actualVersion, err := workspaceFileVersion(ctx, root, name)
	if err != nil {
		return FileWriteResult{}, err
	}
	if actualVersion != expectedVersion {
		return FileWriteResult{}, ErrWorkspaceVersionConflict
	}
	parent := path.Dir(name)
	if parent != "." {
		if err := root.MkdirAll(parent, 0o750); err != nil {
			return FileWriteResult{}, ErrWorkspaceInvalidPath
		}
	}
	temporary, err := workspaceTemporaryName(parent)
	if err != nil {
		return FileWriteResult{}, ErrRuntimeFailed
	}
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return FileWriteResult{}, ErrRuntimeFailed
	}
	keepTemporary := true
	defer func() {
		_ = file.Close()
		if keepTemporary {
			_ = root.Remove(temporary)
		}
	}()
	if _, err := file.Write(body); err != nil {
		return FileWriteResult{}, ErrRuntimeFailed
	}
	if err := file.Sync(); err != nil {
		return FileWriteResult{}, ErrRuntimeFailed
	}
	if err := file.Close(); err != nil {
		return FileWriteResult{}, ErrRuntimeFailed
	}
	// Recheck after the temporary bytes are durable. This closes the practical
	// read/write race for external editors before the atomic rename boundary.
	actualVersion, err = workspaceFileVersion(ctx, root, name)
	if err != nil {
		return FileWriteResult{}, err
	}
	if actualVersion != expectedVersion {
		return FileWriteResult{}, ErrWorkspaceVersionConflict
	}
	if err := root.Rename(temporary, name); err != nil {
		return FileWriteResult{}, ErrRuntimeFailed
	}
	keepTemporary = false
	if directory, err := root.Open(parent); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return FileWriteResult{Path: name, Version: workspaceVersion(body), Size: len(body)}, nil
}

func readWorkspaceRegularFile(ctx context.Context, root *os.Root, name string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := workspacePathInfo(root, name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrWorkspaceInvalidPath
	}
	if info.Size() > MaxWorkspaceFileBytes {
		return nil, ErrWorkspaceFileTooLarge
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, ErrWorkspaceInvalidPath
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, MaxWorkspaceFileBytes+1))
	if err != nil {
		return nil, ErrRuntimeFailed
	}
	if len(body) > MaxWorkspaceFileBytes {
		return nil, ErrWorkspaceFileTooLarge
	}
	if !utf8.Valid(body) || strings.IndexByte(string(body), 0) >= 0 {
		return nil, ErrWorkspaceInvalidUTF8
	}
	return body, nil
}

// workspacePathInfo rejects a symlink in every existing path component. An
// os.Root prevents escape but deliberately follows in-root symlinks, while the
// File Tool contract rejects symlink aliases altogether.
func workspacePathInfo(root *os.Root, name string) (os.FileInfo, error) {
	components := strings.Split(name, "/")
	current := ""
	var info os.FileInfo
	for index, component := range components {
		current = path.Join(current, component)
		candidate, err := root.Lstat(current)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, ErrWorkspaceFileNotFound
			}
			return nil, ErrWorkspaceInvalidPath
		}
		if candidate.Mode()&os.ModeSymlink != 0 ||
			(index < len(components)-1 && !candidate.IsDir()) {
			return nil, ErrWorkspaceInvalidPath
		}
		info = candidate
	}
	return info, nil
}

func workspaceFileVersion(ctx context.Context, root *os.Root, name string) (string, error) {
	body, err := readWorkspaceRegularFile(ctx, root, name)
	if errors.Is(err, ErrWorkspaceFileNotFound) {
		return WorkspaceVersionAbsent, nil
	}
	if err != nil {
		return "", err
	}
	return workspaceVersion(body), nil
}

func workspaceVersion(body []byte) string {
	digest := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func validWorkspaceVersion(value string) bool {
	if value == WorkspaceVersionAbsent {
		return true
	}
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func cleanWorkspacePath(value string, allowRoot bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" && allowRoot {
		return ".", nil
	}
	if value == "" || len(value) > 4<<10 || strings.ContainsRune(value, '\x00') ||
		filepath.IsAbs(value) || strings.Contains(value, "\\") {
		return "", ErrWorkspaceInvalidPath
	}
	cleaned := path.Clean(value)
	if cleaned != value || cleaned == ".." || strings.HasPrefix(cleaned, "../") ||
		(!allowRoot && cleaned == ".") {
		return "", ErrWorkspaceInvalidPath
	}
	return cleaned, nil
}

func workspaceTemporaryName(parent string) (string, error) {
	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	name := ".neo-chat-" + hex.EncodeToString(suffix[:]) + ".tmp"
	if parent == "." {
		return name, nil
	}
	return path.Join(parent, name), nil
}
