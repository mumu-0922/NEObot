package skillsupply

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

const (
	officialSyntheticIdentifier = "neo-readonly-inspector"
	officialSyntheticVersion    = "1.0.0"
)

var (
	lobeIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,255}$`)
	gitCommitPattern      = regexp.MustCompile(`^[a-f0-9]{40}$`)
	githubRepositoryPath  = regexp.MustCompile(`^/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+(?:\.git)?$`)
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
