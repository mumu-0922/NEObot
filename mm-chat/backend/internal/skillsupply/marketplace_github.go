package skillsupply

import (
	"context"
	"crypto/sha1" // #nosec G505 -- Git blob object IDs are defined by SHA-1.
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
)

const (
	maxMarketplaceGitHubDirectoryBytes = int64(2 << 20)
	maxMarketplaceGitHubFiles          = 256
	maxMarketplaceGitHubDirectories    = 64
)

type marketplaceGitHubEntry struct {
	Name            string `json:"name"`
	Path            string `json:"path"`
	SHA             string `json:"sha"`
	Type            string `json:"type"`
	Size            int64  `json:"size"`
	SubmoduleGitURL string `json:"submodule_git_url"`
}

func validMarketplaceGitHubSourceURL(rawURL, expectedName string) string {
	rawURL = strings.TrimSpace(rawURL)
	coordinate, err := ParseGitHubSkillURL(rawURL)
	if err != nil || coordinate.ExpectedName != strings.TrimSpace(expectedName) {
		return ""
	}
	return rawURL
}

// fetchGitHubSubtree pins one exact GitHub Skill coordinate and materializes
// only that directory. Marketplace repositories can be much larger than a
// Skill, so downloading the enclosing codeload ZIP is deliberately avoided.
func (source DirectSkillLinkSource) fetchGitHubSubtree(
	ctx context.Context,
	rawURL, expectedName string,
) (ArchiveSource, error) {
	ctx, cancel := context.WithTimeout(ctx, directSkillTotalTimeout)
	defer cancel()

	coordinate, err := ParseGitHubSkillURL(rawURL)
	expectedName = strings.TrimSpace(expectedName)
	if err != nil || coordinate.ExpectedName != expectedName {
		return ArchiveSource{}, ErrInvalidSource
	}
	client := source.Client
	if client == nil {
		client = directSkillHTTPClient(source.Timeout)
	}
	commit := coordinate.Ref
	if !gitCommitPattern.MatchString(commit) {
		commit, err = resolveGitHubRef(
			ctx, client, coordinate.Owner, coordinate.Repository, coordinate.Ref,
		)
		if err != nil {
			return ArchiveSource{}, err
		}
	}

	entries, err := enumerateMarketplaceGitHubFiles(ctx, client, coordinate, commit)
	if err != nil {
		return ArchiveSource{}, err
	}
	files := make([]packageFile, 0, len(entries))
	var totalBytes int64
	for _, entry := range entries {
		body, fetchErr := fetchMarketplaceGitHubBlob(ctx, client, coordinate, commit, entry)
		if fetchErr != nil {
			return ArchiveSource{}, fetchErr
		}
		totalBytes += int64(len(body))
		if totalBytes > MaxSourceArchiveBytes {
			return ArchiveSource{}, ErrArchiveInvalid
		}
		relative := strings.TrimPrefix(entry.Path, coordinate.Subdirectory+"/")
		files = append(files, packageFile{path: relative, data: body})
	}
	archive, err := writeCanonicalArchive(files)
	if err != nil || len(archive) == 0 || int64(len(archive)) > MaxSourceArchiveBytes {
		return ArchiveSource{}, ErrArchiveInvalid
	}
	canonicalRepository := "https://github.com/" + coordinate.Owner + "/" +
		coordinate.Repository + ".git"
	return ArchiveSource{
		Type: SourceGit, Ref: canonicalRepository + "#" + commit + ":" + coordinate.Subdirectory,
		Identifier: coordinate.Repository, ExpectedName: expectedName, Data: archive,
	}, nil
}

func enumerateMarketplaceGitHubFiles(
	ctx context.Context,
	client SourceHTTPClient,
	coordinate GitHubSkillCoordinate,
	commit string,
) ([]marketplaceGitHubEntry, error) {
	directories := []string{coordinate.Subdirectory}
	directoryCount := 1
	files := make([]marketplaceGitHubEntry, 0, 8)
	seenPaths := make(map[string]struct{}, 8)
	var declaredBytes int64
	rootSkillFound := false

	for len(directories) > 0 {
		directory := directories[0]
		directories = directories[1:]
		requestURL := marketplaceGitHubContentsURL(coordinate, directory, commit)
		body, err := fetchDirectSourceBody(
			ctx, client, requestURL, "api.github.com", "application/json",
			maxMarketplaceGitHubDirectoryBytes,
		)
		if err != nil {
			return nil, err
		}
		var entries []marketplaceGitHubEntry
		if json.Unmarshal(body, &entries) != nil || len(entries) > maxPackageFiles {
			return nil, ErrSourceUnavailable
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
		for _, entry := range entries {
			if !validMarketplaceGitHubEntry(directory, entry) {
				return nil, ErrArchiveInvalid
			}
			relative := strings.TrimPrefix(entry.Path, coordinate.Subdirectory+"/")
			if !validArchivePath(relative) {
				return nil, ErrArchiveInvalid
			}
			if _, duplicate := seenPaths[relative]; duplicate {
				return nil, ErrArchiveInvalid
			}
			seenPaths[relative] = struct{}{}
			switch entry.Type {
			case "dir":
				directoryCount++
				if directoryCount > maxMarketplaceGitHubDirectories {
					return nil, ErrArchiveInvalid
				}
				directories = append(directories, entry.Path)
			case "file":
				if entry.SubmoduleGitURL != "" || !gitCommitPattern.MatchString(entry.SHA) ||
					entry.Size < 0 || entry.Size > maxFileBytes || len(files) >= maxMarketplaceGitHubFiles {
					return nil, ErrArchiveInvalid
				}
				declaredBytes += entry.Size
				if declaredBytes > MaxSourceArchiveBytes {
					return nil, ErrArchiveInvalid
				}
				if relative == "SKILL.md" {
					rootSkillFound = true
				}
				files = append(files, entry)
			default:
				return nil, ErrArchiveInvalid
			}
		}
	}
	if !rootSkillFound || len(files) == 0 {
		return nil, ErrInvalidSource
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func validMarketplaceGitHubEntry(directory string, entry marketplaceGitHubEntry) bool {
	if entry.Name == "" || strings.Contains(entry.Name, "/") ||
		!validArchivePath(entry.Name) || entry.Path != path.Join(directory, entry.Name) ||
		!validArchivePath(entry.Path) {
		return false
	}
	return entry.Type == "file" || entry.Type == "dir"
}

func marketplaceGitHubContentsURL(
	coordinate GitHubSkillCoordinate,
	directory, commit string,
) string {
	query := url.Values{"ref": []string{commit}}
	return "https://api.github.com/repos/" + url.PathEscape(coordinate.Owner) + "/" +
		url.PathEscape(coordinate.Repository) + "/contents/" + escapeGitHubPath(directory) +
		"?" + query.Encode()
}

func fetchMarketplaceGitHubBlob(
	ctx context.Context,
	client SourceHTTPClient,
	coordinate GitHubSkillCoordinate,
	commit string,
	entry marketplaceGitHubEntry,
) ([]byte, error) {
	requestURL := "https://raw.githubusercontent.com/" + url.PathEscape(coordinate.Owner) + "/" +
		url.PathEscape(coordinate.Repository) + "/" + url.PathEscape(commit) + "/" +
		escapeGitHubPath(entry.Path)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, ErrInvalidSource
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("User-Agent", "Neo-Chat-Skill-Supply/1")
	response, err := client.Do(request)
	if err != nil {
		return nil, ErrSourceUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
		return nil, ErrSourceUnavailable
	}
	if response.Request != nil && response.Request.URL != nil &&
		(response.Request.URL.Scheme != "https" ||
			!strings.EqualFold(response.Request.URL.Hostname(), "raw.githubusercontent.com")) {
		return nil, ErrSourceUnavailable
	}
	body, err := readBounded(response.Body, entry.Size)
	if err != nil || int64(len(body)) != entry.Size || gitBlobSHA(body) != entry.SHA {
		return nil, ErrSourceUnavailable
	}
	return body, nil
}

func escapeGitHubPath(value string) string {
	parts := strings.Split(value, "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return strings.Join(parts, "/")
}

func gitBlobSHA(data []byte) string {
	digest := sha1.New() // #nosec G401 -- Git object compatibility requires SHA-1.
	_, _ = fmt.Fprintf(digest, "blob %d\x00", len(data))
	_, _ = digest.Write(data)
	return hex.EncodeToString(digest.Sum(nil))
}
