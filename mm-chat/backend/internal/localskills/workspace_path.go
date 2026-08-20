package localskills

import (
	"path"
	"path/filepath"
	"strings"
)

// cleanWorkspaceInputPath resolves a configured container/host workspace alias
// into the single relative path accepted by the existing os.Root boundary.
// It never introduces a second filesystem authority.
func (executor *Executor) cleanWorkspaceInputPath(
	value string,
	allowRoot bool,
) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > 4<<10 || strings.ContainsRune(value, '\x00') {
		return "", ErrWorkspaceInvalidPath
	}

	if absolute, ok, err := normalizeWSLUNCPath(value); err != nil {
		return "", err
	} else if ok {
		if executor.config.WorkspaceHostRoot == "" {
			return "", ErrWorkspaceInvalidPath
		}
		return cleanWorkspaceAbsoluteAlias(
			executor.config.WorkspaceHostRoot,
			absolute,
			allowRoot,
		)
	}

	if filepath.IsAbs(value) {
		for _, root := range []string{
			executor.config.WorkspaceRoot,
			executor.config.WorkspaceHostRoot,
		} {
			if root == "" {
				continue
			}
			if name, err := cleanWorkspaceAbsoluteAlias(root, value, allowRoot); err == nil {
				return name, nil
			}
		}
		return "", ErrWorkspaceInvalidPath
	}

	return cleanWorkspacePath(value, allowRoot)
}

func cleanWorkspaceAbsoluteAlias(root, target string, allowRoot bool) (string, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root ||
		!filepath.IsAbs(target) || filepath.Clean(target) != target ||
		!pathWithin(root, target) {
		return "", ErrWorkspaceInvalidPath
	}
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return "", ErrWorkspaceInvalidPath
	}
	return cleanWorkspacePath(filepath.ToSlash(relative), allowRoot)
}

func normalizeWSLUNCPath(value string) (string, bool, error) {
	if !strings.Contains(value, "\\") {
		return "", false, nil
	}
	slashed := strings.ReplaceAll(value, "\\", "/")
	lower := strings.ToLower(slashed)
	prefixes := []string{
		"//wsl.localhost/",
		"//wsl$/",
	}
	for _, prefix := range prefixes {
		if !strings.HasPrefix(lower, prefix) {
			continue
		}
		remainder := slashed[len(prefix):]
		separator := strings.IndexByte(remainder, '/')
		if separator <= 0 || separator == len(remainder)-1 {
			return "", true, ErrWorkspaceInvalidPath
		}
		distro := remainder[:separator]
		if distro == "." || distro == ".." {
			return "", true, ErrWorkspaceInvalidPath
		}
		absolute := "/" + remainder[separator+1:]
		if path.Clean(absolute) != absolute {
			return "", true, ErrWorkspaceInvalidPath
		}
		return filepath.FromSlash(absolute), true, nil
	}
	return "", false, nil
}
