package skillsupply

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

const (
	officialSyntheticIdentifier   = "neo-readonly-inspector"
	officialSyntheticVersion      = "1.0.0"
	maxDirectSkillPageBytes       = int64(1 << 20)
	maxGitHubCommitBytes          = int64(128 << 10)
	maxDirectSkillMatches         = 16
	directSkillTotalTimeout       = 45 * time.Second
	directInstallReconcileTimeout = 5 * time.Second
)

var (
	lobeIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,255}$`)
	gitCommitPattern      = regexp.MustCompile(`^[a-f0-9]{40}$`)
	githubRepositoryPath  = regexp.MustCompile(`^/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+(?:\.git)?$`)
	githubCoordinatePart  = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)
	aiHeroSkillPath       = regexp.MustCompile(`^/skills-([a-z0-9][a-z0-9-]{0,63})/?$`)
	aiHeroInstallCommand  = regexp.MustCompile(`\bnpx\s+skills@latest\s+add\s+([A-Za-z0-9_.-]{1,100})/([A-Za-z0-9_.-]{1,100})\s+--skill(?:=|\s+)([a-z0-9][a-z0-9-]{0,63})\b`)
)

type OfficialSource struct{}

func (OfficialSource) Fetch(_ context.Context, identifier, version string) (ArchiveSource, error) {
	identifier = strings.TrimSpace(identifier)
	version = strings.TrimSpace(version)
	if identifier != officialSyntheticIdentifier || version != officialSyntheticVersion {
		return ArchiveSource{}, ErrInvalidSource
	}
	archive, err := officialSyntheticArchive()
	if err != nil {
		return ArchiveSource{}, ErrSourceUnavailable
	}
	return ArchiveSource{Type: SourceOfficial, Ref: "official:" + identifier + "@" + version,
		Identifier: identifier, Version: version, ExpectedName: identifier,
		Data: archive, ExecutableAllowed: true}, nil
}

type LobeHubSource struct{ Fetcher LobeHubFetcher }

func (source LobeHubSource) Fetch(
	ctx context.Context,
	identifier, version string,
) (ArchiveSource, error) {
	identifier, version = strings.TrimSpace(identifier), strings.TrimSpace(version)
	if source.Fetcher == nil || !lobeIdentifierPattern.MatchString(identifier) ||
		!semverPattern.MatchString(version) {
		return ArchiveSource{}, ErrInvalidSource
	}
	data, err := source.Fetcher.FetchSkillPackage(ctx, identifier, version, MaxSourceArchiveBytes)
	if err != nil {
		return ArchiveSource{}, ErrSourceUnavailable
	}
	return ArchiveSource{Type: SourceLobeHub, Ref: "lobehub:" + identifier + "@" + version,
		Identifier: identifier, Version: version, ExpectedName: identifier, Data: data}, nil
}

type GitHubSource struct {
	Client  SourceHTTPClient
	Timeout time.Duration
}

type GitHubSkillCoordinate struct {
	Owner         string
	Repository    string
	Ref           string
	Subdirectory  string
	ExpectedName  string
	RepositoryURL string
}

// DirectSkillLinkSource understands a deliberately small install surface.
// It accepts one exact GitHub Skill directory or the legacy AIHero discovery
// page, pins a mutable ref to a full commit, and delegates package validation
// to the ordinary immutable ZIP pipeline. It never executes repository or page
// commands.
type DirectSkillLinkSource struct {
	Client  SourceHTTPClient
	Timeout time.Duration
}

func (source DirectSkillLinkSource) Fetch(
	ctx context.Context,
	rawURL, expectedName string,
) (ArchiveSource, error) {
	ctx, cancel := context.WithTimeout(ctx, directSkillTotalTimeout)
	defer cancel()
	expectedName = strings.TrimSpace(expectedName)
	if expectedName == "" {
		return ArchiveSource{}, ErrInvalidSource
	}
	client := source.Client
	if client == nil {
		client = directSkillHTTPClient(source.Timeout)
	}
	if coordinate, err := ParseGitHubSkillURL(rawURL); err == nil {
		if coordinate.ExpectedName != expectedName {
			return ArchiveSource{}, ErrInvalidSource
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
		return (GitHubSource{Client: client, Timeout: source.Timeout}).Fetch(
			ctx, coordinate.RepositoryURL, commit, coordinate.Subdirectory,
		)
	}
	pageURL, linkName, err := parseAIHeroSkillURL(rawURL)
	if err != nil || expectedName != linkName {
		return ArchiveSource{}, ErrInvalidSource
	}
	page, err := fetchDirectSourceBody(
		ctx, client, pageURL, "www.aihero.dev", "text/html", maxDirectSkillPageBytes,
	)
	if err != nil {
		return ArchiveSource{}, err
	}
	owner, repository, ok := parseUniqueAIHeroInstallCommand(page, expectedName)
	if !ok {
		return ArchiveSource{}, ErrInvalidSource
	}
	commit, err := resolveGitHubHEAD(ctx, client, owner, repository)
	if err != nil {
		return ArchiveSource{}, err
	}
	root, err := (GitHubSource{Client: client, Timeout: source.Timeout}).Fetch(
		ctx, "https://github.com/"+owner+"/"+repository, commit, "",
	)
	if err != nil {
		return ArchiveSource{}, err
	}
	return selectGitHubSkill(root, expectedName)
}

func parseAIHeroSkillURL(raw string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Port() != "" && parsed.Port() != "443") {
		return "", "", ErrInvalidSource
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host != "aihero.dev" && host != "www.aihero.dev" {
		return "", "", ErrInvalidSource
	}
	matches := aiHeroSkillPath.FindStringSubmatch(parsed.EscapedPath())
	if len(matches) != 2 || !skillNamePattern.MatchString(matches[1]) {
		return "", "", ErrInvalidSource
	}
	return "https://www.aihero.dev" + parsed.EscapedPath(), matches[1], nil
}

func parseUniqueAIHeroInstallCommand(page []byte, expectedName string) (string, string, bool) {
	matches := aiHeroInstallCommand.FindAllSubmatch(page, -1)
	coordinates := map[string][2]string{}
	for _, match := range matches {
		if len(match) != 4 || string(match[3]) != expectedName {
			continue
		}
		owner, repository := string(match[1]), strings.TrimSuffix(string(match[2]), ".git")
		if owner == "" || repository == "" {
			continue
		}
		coordinates[strings.ToLower(owner+"/"+repository)] = [2]string{owner, repository}
	}
	if len(coordinates) != 1 {
		return "", "", false
	}
	for _, coordinate := range coordinates {
		return coordinate[0], coordinate[1], true
	}
	return "", "", false
}

func resolveGitHubHEAD(
	ctx context.Context,
	client SourceHTTPClient,
	owner, repository string,
) (string, error) {
	return resolveGitHubRef(ctx, client, owner, repository, "HEAD")
}

func resolveGitHubRef(
	ctx context.Context,
	client SourceHTTPClient,
	owner, repository, ref string,
) (string, error) {
	if !githubCoordinatePart.MatchString(owner) || !githubCoordinatePart.MatchString(repository) ||
		!githubCoordinatePart.MatchString(ref) {
		return "", ErrInvalidSource
	}
	endpoint := "https://api.github.com/repos/" + url.PathEscape(owner) + "/" +
		url.PathEscape(repository) + "/commits/" + url.PathEscape(ref)
	body, err := fetchDirectSourceBody(
		ctx, client, endpoint, "api.github.com", "application/json", maxGitHubCommitBytes,
	)
	if err != nil {
		return "", err
	}
	var response struct {
		SHA string `json:"sha"`
	}
	if json.Unmarshal(body, &response) != nil || !gitCommitPattern.MatchString(response.SHA) {
		return "", ErrSourceUnavailable
	}
	return response.SHA, nil
}

// ParseGitHubSkillURL accepts only one unambiguous GitHub Skill directory.
// Repository roots and encoded path segments are deliberately rejected.
func ParseGitHubSkillURL(raw string) (GitHubSkillCoordinate, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) == 0 || len(raw) > 2048 {
		return GitHubSkillCoordinate{}, ErrInvalidSource
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Port() != "" && parsed.Port() != "443") ||
		!strings.EqualFold(strings.TrimSuffix(parsed.Hostname(), "."), "github.com") ||
		strings.Contains(parsed.EscapedPath(), "%") {
		return GitHubSkillCoordinate{}, ErrInvalidSource
	}
	segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(segments) < 5 || !githubCoordinatePart.MatchString(segments[0]) ||
		!githubCoordinatePart.MatchString(strings.TrimSuffix(segments[1], ".git")) ||
		(segments[2] != "tree" && segments[2] != "blob") ||
		!githubCoordinatePart.MatchString(segments[3]) {
		return GitHubSkillCoordinate{}, ErrInvalidSource
	}
	owner := segments[0]
	repository := strings.TrimSuffix(segments[1], ".git")
	subdirectoryParts := append([]string(nil), segments[4:]...)
	if segments[2] == "blob" {
		if len(subdirectoryParts) < 2 || subdirectoryParts[len(subdirectoryParts)-1] != "SKILL.md" {
			return GitHubSkillCoordinate{}, ErrInvalidSource
		}
		subdirectoryParts = subdirectoryParts[:len(subdirectoryParts)-1]
	}
	subdirectory := strings.Join(subdirectoryParts, "/")
	expectedName := path.Base(subdirectory)
	if !validArchivePath(subdirectory) || !skillNamePattern.MatchString(expectedName) {
		return GitHubSkillCoordinate{}, ErrInvalidSource
	}
	return GitHubSkillCoordinate{
		Owner: owner, Repository: repository, Ref: segments[3],
		Subdirectory: subdirectory, ExpectedName: expectedName,
		RepositoryURL: "https://github.com/" + owner + "/" + repository,
	}, nil
}

func selectGitHubSkill(root ArchiveSource, expectedName string) (ArchiveSource, error) {
	reader, err := zip.NewReader(bytes.NewReader(root.Data), int64(len(root.Data)))
	if err != nil {
		return ArchiveSource{}, ErrArchiveInvalid
	}
	prefix := cleanPrefix(root.StripPrefix)
	if prefix == "" {
		return ArchiveSource{}, ErrArchiveInvalid
	}
	directories := map[string]struct{}{}
	for _, entry := range reader.File {
		if entry == nil || entry.FileInfo().IsDir() || !strings.HasPrefix(entry.Name, prefix) ||
			!strings.HasSuffix(entry.Name, "/SKILL.md") {
			continue
		}
		relative := strings.TrimSuffix(strings.TrimPrefix(entry.Name, prefix), "/SKILL.md")
		if relative == "" || path.Base(relative) != expectedName || !validArchivePath(relative) {
			continue
		}
		directories[relative] = struct{}{}
		if len(directories) > maxDirectSkillMatches {
			return ArchiveSource{}, ErrInvalidSource
		}
	}
	valid := make([]ArchiveSource, 0, 1)
	for directory := range directories {
		candidate := root
		candidate.Identifier = expectedName
		candidate.ExpectedName = expectedName
		candidate.StripPrefix = prefix + directory + "/"
		candidate.Ref = root.Ref + ":" + directory
		if _, validateErr := ValidateArchive(candidate); validateErr == nil {
			valid = append(valid, candidate)
		}
	}
	if len(valid) != 1 {
		return ArchiveSource{}, ErrInvalidSource
	}
	return valid[0], nil
}

func fetchDirectSourceBody(
	ctx context.Context,
	client SourceHTTPClient,
	requestURL, expectedHost, expectedMediaType string,
	maximum int64,
) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, ErrInvalidSource
	}
	request.Header.Set("Accept", expectedMediaType)
	request.Header.Set("User-Agent", "Neo-Chat-Direct-Skill/1")
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
			!strings.EqualFold(response.Request.URL.Hostname(), expectedHost)) {
		return nil, ErrSourceUnavailable
	}
	mediaType, _, parseErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if parseErr != nil || !strings.EqualFold(mediaType, expectedMediaType) {
		return nil, ErrSourceUnavailable
	}
	return readBounded(response.Body, maximum)
}

func directSkillHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: sourceTimeout(timeout),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (source GitHubSource) Fetch(
	ctx context.Context,
	repositoryURL, commit, subdirectory string,
) (ArchiveSource, error) {
	owner, repository, err := parseGitHubRepository(repositoryURL)
	if err != nil || !gitCommitPattern.MatchString(strings.TrimSpace(commit)) {
		return ArchiveSource{}, ErrInvalidSource
	}
	commit = strings.TrimSpace(commit)
	subdirectory = strings.Trim(strings.TrimSpace(subdirectory), "/")
	if subdirectory != "" && !validArchivePath(subdirectory) {
		return ArchiveSource{}, ErrInvalidSource
	}
	client := source.Client
	if client == nil {
		client = &http.Client{Timeout: sourceTimeout(source.Timeout), CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 3 || request.URL.Scheme != "https" || request.URL.Host != "codeload.github.com" {
				return ErrSourceUnavailable
			}
			return nil
		}}
	}
	requestURL := fmt.Sprintf("https://codeload.github.com/%s/%s/zip/%s", url.PathEscape(owner), url.PathEscape(repository), commit)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return ArchiveSource{}, ErrInvalidSource
	}
	request.Header.Set("Accept", "application/zip")
	request.Header.Set("User-Agent", "Neo-Chat-Skill-Supply/1")
	response, err := client.Do(request)
	if err != nil {
		return ArchiveSource{}, ErrSourceUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
		return ArchiveSource{}, ErrSourceUnavailable
	}
	data, err := readBounded(response.Body, MaxSourceArchiveBytes)
	if err != nil {
		return ArchiveSource{}, ErrSourceUnavailable
	}
	prefix := repository + "-" + commit + "/"
	if subdirectory != "" {
		prefix += subdirectory + "/"
		if err := requireSelectedSkillRoot(data, prefix); err != nil {
			return ArchiveSource{}, err
		}
	}
	expectedName := repository
	if subdirectory != "" {
		expectedName = path.Base(subdirectory)
	}
	canonicalRepository := "https://github.com/" + owner + "/" + repository + ".git"
	ref := canonicalRepository + "#" + commit
	if subdirectory != "" {
		ref += ":" + subdirectory
	}
	return ArchiveSource{Type: SourceGit, Ref: ref, Identifier: repository,
		Version: "", StripPrefix: prefix, ExpectedName: expectedName, Data: data}, nil
}

func requireSelectedSkillRoot(data []byte, prefix string) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(reader.File) == 0 {
		return ErrArchiveInvalid
	}
	expected := prefix + "SKILL.md"
	for _, entry := range reader.File {
		if entry != nil && !entry.FileInfo().IsDir() && entry.Name == expected {
			return nil
		}
	}
	return ErrInvalidSource
}

func ZIPSource(data []byte) (ArchiveSource, error) {
	if len(data) == 0 || int64(len(data)) > MaxSourceArchiveBytes {
		return ArchiveSource{}, ErrInvalidSource
	}
	fingerprint := sha256Fingerprint(data)
	return ArchiveSource{Type: SourceZIP, Ref: "zip:" + fingerprint,
		Identifier: "upload", Data: append([]byte(nil), data...)}, nil
}

func parseGitHubRepository(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) == 0 || len(raw) > 512 {
		return "", "", ErrInvalidSource
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || !githubRepositoryPath.MatchString(parsed.Path) {
		return "", "", ErrInvalidSource
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	repository := strings.TrimSuffix(parts[1], ".git")
	if repository == "" || len(parts[0]) > 100 || len(repository) > 100 {
		return "", "", ErrInvalidSource
	}
	return parts[0], repository, nil
}

func sourceTimeout(value time.Duration) time.Duration {
	if value <= 0 || value > 30*time.Second {
		return 20 * time.Second
	}
	return value
}

func readBounded(reader io.Reader, maximum int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil || int64(len(data)) > maximum {
		return nil, ErrSourceUnavailable
	}
	return data, nil
}

func officialSyntheticArchive() ([]byte, error) {
	manifest := `{"schemaVersion":"neo.skill-runtime/v1","package":{"name":"neo-readonly-inspector","version":"1.0.0"},"runtime":{"kind":"rootless_oci","image":"registry.neo.invalid/runtime/read-only@sha256:1111111111111111111111111111111111111111111111111111111111111111","platform":"linux/amd64","user":{"uid":10001,"gid":10001}},"entrypoints":[{"name":"inspect","argv":["/opt/neo/bin/readonly-inspector"],"workingDirectory":"/workspace"}],"dependencies":[],"capabilityRequests":[{"capability":"workspace.read","actions":["list","read"],"reason":"Inspect the admitted synthetic read-only workspace."}],"egressRequests":[],"secretSlots":[],"resources":{"cpuMillis":500,"memoryMiB":128,"pids":32,"wallSeconds":30},"limits":{"maxPackageFiles":16,"maxPackageBytes":1048576,"maxExpandedBytes":1048576,"maxStdoutBytes":65536,"maxStderrBytes":65536,"maxArtifactBytes":1}}`
	files := []packageFile{
		{path: "SKILL.md", data: []byte("---\nname: neo-readonly-inspector\ndescription: Inspect a synthetic workspace using read-only list and read operations.\nlicense: MIT\nmetadata:\n  version: \"1.0.0\"\nallowed-tools: Read\n---\n\n# Neo Read-only Inspector\n\nThis is the server-owned synthetic supply-chain fixture.\n")},
		{path: "neo.runtime.json", data: []byte(manifest)},
		{path: "scripts/readonly-inspector", data: []byte("fixture bytes only; G20.1 never executes this file\n")},
	}
	return writeCanonicalArchive(files)
}
