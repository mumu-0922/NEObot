package skillsupply

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

type fakeSkillMarketplace struct {
	archive []byte
	paths   []string
}

func (fake *fakeSkillMarketplace) FetchSkillPackage(
	_ context.Context, identifier, version string, _ int64,
) ([]byte, error) {
	if identifier != "owner-demo" || version != "1.2.3" {
		return nil, fmt.Errorf("unexpected package %s@%s", identifier, version)
	}
	return append([]byte(nil), fake.archive...), nil
}

func (fake *fakeSkillMarketplace) FetchSkillMarketJSON(
	_ context.Context, path string, _ int64,
) ([]byte, error) {
	fake.paths = append(fake.paths, path)
	switch {
	case strings.HasPrefix(path, "/api/v1/skills?"):
		return []byte(`{"items":[{"identifier":"owner-demo","name":"Demo Skill","description":"Marketplace demo","version":"1.2.3","category":"productivity","author":"Owner","installCount":7,"ratingAvg":4.5,"isOfficial":false,"isValidated":true,"isFeatured":true,"resourcesCount":1,"github":{"url":"https://github.com/owner/demo"}}],"currentPage":1,"pageSize":20,"totalCount":1,"totalPages":1}`), nil
	case strings.HasPrefix(path, "/api/v1/skills/categories?"):
		return []byte(`[{"category":"productivity","count":1}]`), nil
	default:
		return nil, fmt.Errorf("unexpected path %s", path)
	}
}

func (fake *fakeSkillMarketplace) FetchPublicSkillDetailJSON(
	_ context.Context, path string, _ int64,
) ([]byte, error) {
	fake.paths = append(fake.paths, path)
	if !strings.HasPrefix(path, "/api/v1/skills/owner-demo?") {
		return nil, fmt.Errorf("unexpected detail path %s", path)
	}
	return []byte(`{"identifier":"owner-demo","name":"Demo Skill","description":"Marketplace demo","version":"1.2.3","category":"productivity","installCount":7,"ratingAverage":4.5,"isOfficial":false,"isValidated":true,"isFeatured":true,"author":{"name":"Owner"},"github":{"url":"https://github.com/owner/demo"},"license":{"name":"MIT"},"manifest":{"name":"demo-skill","description":"Marketplace demo","permissions":["Read"]},"overview":{"summary":"Demo summary"},"resources":{"references/demo.txt":{"fileHash":"sha256:abc","size":3}},"versions":[{"version":"1.2.3","isLatest":true,"isValidated":true,"createdAt":"2026-08-28T00:00:00Z","versionNumber":1}]}`), nil
}

func TestMarketplaceSearchDetailAndExactOwnerPrivateInstall(t *testing.T) {
	fetcher := &fakeSkillMarketplace{archive: mustTestArchive(t, []packageFile{{
		path: "SKILL.md", data: []byte(validSkillMarkdown("demo-skill")),
	}}, 0, time.Time{})}
	repository := newMemoryRepository()
	service := NewService(
		WithRepository(repository), WithObjectStore(newMemoryObjectStore()), WithLobeHubFetcher(fetcher),
	)
	service.newID = sequenceIDs("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	result, err := service.SearchMarketplace(context.Background(), MarketplaceSearchInput{
		Query: "demo", Locale: "en-US", Sort: "installCount",
	})
	if err != nil || len(result.Items) != 1 || len(result.Categories) != 1 ||
		result.Items[0].Identifier != "owner-demo" || result.Source != SourceLobeHub {
		t.Fatalf("SearchMarketplace() = %#v, %v", result, err)
	}
	if len(fetcher.paths) < 2 || !strings.Contains(fetcher.paths[0], "locale=en-US") ||
		!strings.Contains(fetcher.paths[0], "sort=installCount") ||
		!strings.Contains(fetcher.paths[1], "locale=en-US") {
		t.Fatalf("localized sorted marketplace paths = %#v", fetcher.paths)
	}
	detail, err := service.GetMarketplaceSkillLocalized(
		context.Background(), testSkillUser, "owner-demo", "", "ja-JP",
	)
	if err != nil || detail.ManifestName != "demo-skill" || detail.Version != "1.2.3" ||
		len(detail.Resources) != 1 || detail.Installed {
		t.Fatalf("GetMarketplaceSkill() = %#v, %v", detail, err)
	}
	if !strings.Contains(fetcher.paths[len(fetcher.paths)-1], "locale=ja-JP") {
		t.Fatalf("localized detail path = %#v", fetcher.paths)
	}
	installed, err := service.InstallMarketplaceSkill(
		context.Background(), testSkillUser, "owner-demo", detail.Version,
	)
	if err != nil || installed.Name != "demo-skill" || installed.Version != "1.2.3" {
		t.Fatalf("InstallMarketplaceSkill() = %#v, %v", installed, err)
	}
	candidate := repository.candidates[installed.AdmissionID]
	if candidate.OwnerUserID != testSkillUser || candidate.SourceRef != "lobehub:owner-demo@1.2.3" ||
		candidate.Status != StatusValidated {
		t.Fatalf("private candidate = %#v", candidate)
	}
	detail, err = service.GetMarketplaceSkill(context.Background(), testSkillUser, "owner-demo", "1.2.3")
	if err != nil || !detail.Installed {
		t.Fatalf("installed detail = %#v, %v", detail, err)
	}
	linked, err := service.InstallSkillLink(
		context.Background(), testSkillUser, "https://lobehub.com/skills/owner-demo",
	)
	if err != nil || linked.ID != installed.ID {
		t.Fatalf("InstallSkillLink() = %#v, %v", linked, err)
	}
}

func TestMarketplaceExactLinkParsingAndInstall(t *testing.T) {
	for _, raw := range []string{
		"http://lobehub.com/skills/owner-demo", "https://lobehub.com/skills/owner-demo?version=1",
		"https://evil.example/skills/owner-demo", "https://lobehub.com/skills/owner%2Fdemo",
	} {
		if _, err := ParseLobeHubSkillURL(raw); err == nil {
			t.Fatalf("ParseLobeHubSkillURL(%q) succeeded", raw)
		}
	}
	if identifier, err := ParseLobeHubSkillURL("https://market.lobehub.com/skills/owner-demo"); err != nil || identifier != "owner-demo" {
		t.Fatalf("ParseLobeHubSkillURL() = %q, %v", identifier, err)
	}
}

func TestMarketplaceHandlerSearchDetailInstallAndDirectLink(t *testing.T) {
	fetcher := &fakeSkillMarketplace{archive: mustTestArchive(t, []packageFile{{
		path: "SKILL.md", data: []byte(validSkillMarkdown("demo-skill")),
	}}, 0, time.Time{})}
	service := NewService(
		WithRepository(newMemoryRepository()), WithObjectStore(newMemoryObjectStore()),
		WithLobeHubFetcher(fetcher),
	)
	service.newID = sequenceIDs("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	handler := NewHandler(service)

	search := skillRequest(handler, http.MethodGet,
		"/v1/skills/marketplace?q=demo&locale=en-US&sort=ratingAverage&page=1&pageSize=20", nil, "", testSkillUser)
	assertSkillHTTP(t, search, http.StatusOK, `"identifier":"owner-demo"`)
	detail := skillRequest(handler, http.MethodGet,
		"/v1/skills/marketplace/items/owner-demo?version=1.2.3&locale=ja-JP", nil, "", testSkillUser)
	assertSkillHTTP(t, detail, http.StatusOK, `"manifestName":"demo-skill"`)
	install := skillRequest(handler, http.MethodPost,
		"/v1/skills/marketplace/items/owner-demo/install",
		strings.NewReader(`{"version":"1.2.3"}`), "application/json", testSkillUser)
	assertSkillHTTP(t, install, http.StatusCreated, `"name":"demo-skill"`)
	direct := skillRequest(handler, http.MethodPost, "/v1/skills/direct/install",
		strings.NewReader(`{"url":"https://lobehub.com/skills/owner-demo"}`),
		"application/json", testSkillUser)
	assertSkillHTTP(t, direct, http.StatusCreated, `"name":"demo-skill"`)
	invalid := skillRequest(handler, http.MethodGet,
		"/v1/skills/marketplace?unknown=1", nil, "", testSkillUser)
	assertSkillHTTP(t, invalid, http.StatusBadRequest, "INVALID_SKILL_MARKETPLACE_QUERY")
	pathCount := len(fetcher.paths)
	invalid = skillRequest(handler, http.MethodGet,
		"/v1/skills/marketplace?locale=ar&sort=name", nil, "", testSkillUser)
	assertSkillHTTP(t, invalid, http.StatusBadRequest, "INVALID_SKILL_MARKETPLACE_QUERY")
	if len(fetcher.paths) != pathCount {
		t.Fatalf("invalid locale/sort reached Marketplace: %#v", fetcher.paths[pathCount:])
	}
}
