package localskills

import (
	"context"
	"io"
	"os"
)

// WorkspaceArtifactSnapshot is an immutable, bounded copy of one regular
// workspace file. It deliberately supports binary content, unlike file_read.
type WorkspaceArtifactSnapshot struct {
	Path    string
	Body    []byte
	Version string
}

// ReadWorkspaceArtifact snapshots one workspace-relative regular file for
// publication. The same path and symlink rules as the interactive File Tools
// apply, while maxBytes is supplied by the server upload policy.
func (executor *Executor) ReadWorkspaceArtifact(
	ctx context.Context,
	value string,
	maxBytes int64,
) (WorkspaceArtifactSnapshot, error) {
	if !executor.Enabled() {
		return WorkspaceArtifactSnapshot{}, ErrRuntimeFailed
	}
	name, err := cleanWorkspacePath(value, false)
	if err != nil || maxBytes < 1 {
		return WorkspaceArtifactSnapshot{}, ErrWorkspaceInvalidInput
	}
	root, err := os.OpenRoot(executor.config.WorkspaceRoot)
	if err != nil {
		return WorkspaceArtifactSnapshot{}, ErrRuntimeFailed
	}
	defer root.Close()

	info, err := workspacePathInfo(root, name)
	if err != nil {
		return WorkspaceArtifactSnapshot{}, err
	}
	if !info.Mode().IsRegular() {
		return WorkspaceArtifactSnapshot{}, ErrWorkspaceInvalidPath
	}
	if info.Size() > maxBytes {
		return WorkspaceArtifactSnapshot{}, ErrWorkspaceFileTooLarge
	}
	if err := ctx.Err(); err != nil {
		return WorkspaceArtifactSnapshot{}, err
	}
	file, err := root.Open(name)
	if err != nil {
		return WorkspaceArtifactSnapshot{}, ErrWorkspaceInvalidPath
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() {
		return WorkspaceArtifactSnapshot{}, ErrWorkspaceInvalidPath
	}
	if openedInfo.Size() > maxBytes {
		return WorkspaceArtifactSnapshot{}, ErrWorkspaceFileTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return WorkspaceArtifactSnapshot{}, ErrRuntimeFailed
	}
	if int64(len(body)) > maxBytes {
		return WorkspaceArtifactSnapshot{}, ErrWorkspaceFileTooLarge
	}
	return WorkspaceArtifactSnapshot{
		Path: name, Body: body, Version: workspaceVersion(body),
	}, nil
}
