package skillsupply

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxGitHubCatalogBytes    = int64(1 << 20)
	maxGitHubCatalogItems    = 256
	githubCatalogCacheTTL    = 5 * time.Minute
	defaultCatalogOwner      = "openai"
	defaultCatalogRepository = "skills"
	defaultCatalogRef        = "main"
	defaultCatalogPath       = "skills/.curated"
	defaultCatalogSource     = "openai/skills curated"
)

type GitHubCatalogSource struct {
	Client  SourceHTTPClient
	Timeout time.Duration

	mu       sync.Mutex
	cachedAt time.Time
	cached   []CatalogSkillSummary
	commitAt time.Time
	commit   string
}

func (source *GitHubCatalogSource) List(ctx context.Context) ([]CatalogSkillSummary, error) {
	if source == nil {
		return nil, ErrSourceUnavailable
	}
	source.mu.Lock()
	if len(source.cached) > 0 && time.Since(source.cachedAt) < githubCatalogCacheTTL {
		items := append([]CatalogSkillSummary(nil), source.cached...)
		source.mu.Unlock()
		return items, nil
	}
	source.mu.Unlock()

	client := source.Client
	if client == nil {
		client = directSkillHTTPClient(source.Timeout)
	}
	endpoint := "https://api.github.com/repos/" + defaultCatalogOwner + "/" +
		defaultCatalogRepository + "/contents/" + defaultCatalogPath + "?ref=" +
		url.QueryEscape(defaultCatalogRef)
	body, err := fetchDirectSourceBody(
		ctx, client, endpoint, "api.github.com", "application/json", maxGitHubCatalogBytes,
	)
	if err != nil {
		return nil, err
	}
	var response []struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if json.Unmarshal(body, &response) != nil || len(response) > maxGitHubCatalogItems {
		return nil, ErrSourceUnavailable
	}
	items := make([]CatalogSkillSummary, 0, len(response))
	seen := make(map[string]struct{}, len(response))
	for _, entry := range response {
		name := strings.TrimSpace(entry.Name)
		expectedPath := defaultCatalogPath + "/" + name
		if entry.Type != "dir" || !skillNamePattern.MatchString(name) ||
			entry.Path != expectedPath {
			continue
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, ErrSourceUnavailable
		}
		seen[name] = struct{}{}
		items = append(items, catalogSummary(name))
	}
	sort.Slice(items, func(left, right int) bool { return items[left].Name < items[right].Name })
	source.mu.Lock()
	source.cachedAt = time.Now()
	source.cached = append([]CatalogSkillSummary(nil), items...)
	source.mu.Unlock()
	return items, nil
}

func (source *GitHubCatalogSource) Detail(
	ctx context.Context,
	name string,
) (CatalogSkill, ArchiveSource, error) {
	name = strings.TrimSpace(name)
	if source == nil || !skillNamePattern.MatchString(name) {
		return CatalogSkill{}, ArchiveSource{}, ErrInvalidSource
	}
	client := source.Client
	if client == nil {
		client = directSkillHTTPClient(source.Timeout)
	}
	commit, err := source.resolveCommit(ctx, client)
	if err != nil {
		return CatalogSkill{}, ArchiveSource{}, err
	}
	archive, err := (GitHubSource{Client: client, Timeout: source.Timeout}).Fetch(
		ctx,
		"https://github.com/"+defaultCatalogOwner+"/"+defaultCatalogRepository,
		commit,
		defaultCatalogPath+"/"+name,
	)
	if err != nil {
		return CatalogSkill{}, ArchiveSource{}, err
	}
	validated, err := ValidateArchive(archive)
	if err != nil {
		return CatalogSkill{}, ArchiveSource{}, err
	}
	return catalogDetail(catalogSummary(name), commit, validated.Package), archive, nil
}

func (source *GitHubCatalogSource) resolveCommit(
	ctx context.Context,
	client SourceHTTPClient,
) (string, error) {
	source.mu.Lock()
	if gitCommitPattern.MatchString(source.commit) &&
		time.Since(source.commitAt) < githubCatalogCacheTTL {
		commit := source.commit
		source.mu.Unlock()
		return commit, nil
	}
	source.mu.Unlock()
	commit, err := resolveGitHubRef(
		ctx, client, defaultCatalogOwner, defaultCatalogRepository, defaultCatalogRef,
	)
	if err != nil {
		return "", err
	}
	source.mu.Lock()
	source.commit, source.commitAt = commit, time.Now()
	source.mu.Unlock()
	return commit, nil
}

func catalogSummary(name string) CatalogSkillSummary {
	pathValue := defaultCatalogPath + "/" + name
	return CatalogSkillSummary{
		ID: name, Name: name,
		Repository: defaultCatalogOwner + "/" + defaultCatalogRepository,
		Ref:        defaultCatalogRef, Path: pathValue,
		SourceURL: "https://github.com/" + defaultCatalogOwner + "/" +
			defaultCatalogRepository + "/tree/" + defaultCatalogRef + "/" + pathValue,
		CatalogSource: defaultCatalogSource,
	}
}

func catalogDetail(summary CatalogSkillSummary, commit string, pkg PackageVersion) CatalogSkill {
	return CatalogSkill{
		CatalogSkillSummary: summary, ResolvedCommit: commit,
		PackageFingerprint: pkg.PackageFingerprint, Version: pkg.Version,
		Description: pkg.Description, License: pkg.License,
		Compatibility: pkg.Compatibility,
		AllowedTools:  nonNilStrings(pkg.AllowedTools), HasRuntime: pkg.HasRuntime,
	}
}
