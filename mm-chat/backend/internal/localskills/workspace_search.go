package localskills

import (
	"bufio"
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxWorkspaceSearchFiles     = 2_000
	MaxWorkspaceSearchBytes     = 32 << 20
	MaxWorkspaceSearchResults   = 200
	maxWorkspaceSearchLineBytes = 256 << 10
	maxWorkspaceSearchPreview   = 512
)

type FileSearchRequest struct {
	Path       string
	Query      string
	Glob       string
	MaxResults int
}

type FileSearchMatch struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Preview string `json:"preview"`
}

type FileSearchResult struct {
	Matches      []FileSearchMatch `json:"matches"`
	FilesScanned int               `json:"filesScanned"`
	BytesScanned int64             `json:"bytesScanned"`
	Truncated    bool              `json:"truncated"`
}

func (executor *Executor) SearchWorkspaceFiles(
	ctx context.Context,
	request FileSearchRequest,
) (FileSearchResult, error) {
	result := FileSearchResult{Matches: []FileSearchMatch{}}
	if !executor.Enabled() {
		return result, ErrRuntimeFailed
	}
	rootPath, err := executor.cleanWorkspaceInputPath(request.Path, true)
	if err != nil || request.Query == "" || len(request.Query) > 4<<10 ||
		!utf8.ValidString(request.Query) || strings.ContainsRune(request.Query, '\x00') ||
		len(request.Glob) > 512 || request.MaxResults < 0 ||
		request.MaxResults > MaxWorkspaceSearchResults {
		return result, ErrWorkspaceInvalidInput
	}
	if request.MaxResults == 0 {
		request.MaxResults = 50
	}
	if request.Glob == "" {
		request.Glob = "*"
	}
	if _, err := path.Match(request.Glob, "probe"); err != nil {
		return result, ErrWorkspaceInvalidInput
	}
	root, err := os.OpenRoot(executor.config.WorkspaceRoot)
	if err != nil {
		return result, ErrRuntimeFailed
	}
	defer root.Close()
	if rootPath != "." {
		info, statErr := workspacePathInfo(root, rootPath)
		if statErr != nil || !info.IsDir() {
			return result, ErrWorkspaceInvalidPath
		}
	}

	filesVisited := 0
	err = fs.WalkDir(root.FS(), rootPath, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if name != rootPath && hiddenWorkspaceDirectory(entry.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		if filesVisited >= MaxWorkspaceSearchFiles || result.BytesScanned >= MaxWorkspaceSearchBytes {
			result.Truncated = true
			return fs.SkipAll
		}
		filesVisited++
		matched, matchErr := path.Match(request.Glob, name)
		if matchErr != nil || !matched {
			matched, _ = path.Match(request.Glob, path.Base(name))
		}
		if !matched {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil || info.Size() > MaxWorkspaceFileBytes ||
			result.BytesScanned+info.Size() > MaxWorkspaceSearchBytes {
			if infoErr == nil && result.BytesScanned+info.Size() > MaxWorkspaceSearchBytes {
				result.Truncated = true
				return fs.SkipAll
			}
			return nil
		}
		result.FilesScanned++
		result.BytesScanned += info.Size()
		file, openErr := root.Open(name)
		if openErr != nil {
			return nil
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 4<<10), maxWorkspaceSearchLineBytes)
		for line := 1; scanner.Scan(); line++ {
			text := scanner.Text()
			if !utf8.ValidString(text) || !strings.Contains(text, request.Query) {
				continue
			}
			result.Matches = append(result.Matches, FileSearchMatch{
				Path: name, Line: line, Preview: boundedWorkspacePreview(text),
			})
			if len(result.Matches) >= request.MaxResults {
				result.Truncated = true
				_ = file.Close()
				return fs.SkipAll
			}
		}
		_ = file.Close()
		return nil
	})
	if err != nil && !errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded) {
		return result, ErrRuntimeFailed
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func hiddenWorkspaceDirectory(name string) bool {
	switch name {
	case ".git", ".next", "node_modules", "vendor", "__pycache__":
		return true
	default:
		return false
	}
}

func boundedWorkspacePreview(value string) string {
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) && character != '\t' {
			return ' '
		}
		return character
	}, strings.TrimSpace(value))
	if len(value) <= maxWorkspaceSearchPreview {
		return value
	}
	value = value[:maxWorkspaceSearchPreview]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
