package skillsupply

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

type fakeSkillMarketplace struct {
	categoriesJSON []byte
	detailJSON     []byte
	packageCalls   int
	paths          []string
}

func (fake *fakeSkillMarketplace) FetchSkillPackage(
	_ context.Context, identifier, version string, _ int64,
) ([]byte, error) {
	fake.packageCalls++
	return nil, fmt.Errorf("legacy package download must not run for %s@%s", identifier, version)
}

func (fake *fakeSkillMarketplace) FetchSkillMarketJSON(
	_ context.Context, path string, _ int64,
) ([]byte, error) {
	fake.paths = append(fake.paths, path)
	switch {
	case strings.HasPrefix(path, "/api/v1/skills?"):
		return []byte(`{"items":[{"identifier":"owner-demo","name":"Demo Skill","description":"Marketplace demo","version":"1.2.3","category":"productivity-tasks","author":"Owner","installCount":7,"ratingAvg":4.5,"isOfficial":false,"isValidated":true,"isFeatured":true,"resourcesCount":1,"github":{"url":"https://github.com/owner/demo"}}],"currentPage":1,"pageSize":20,"totalCount":1,"totalPages":1}`), nil
	case strings.HasPrefix(path, "/api/v1/skills/categories?"):
		if fake.categoriesJSON != nil {
			return append([]byte(nil), fake.categoriesJSON...), nil
		}
		return []byte(`[{"category":"productivity-tasks","count":1}]`), nil
	default:
		return nil, fmt.Errorf("unexpected path %s", path)
	}
}

func TestMarketplaceCategoriesProjectCuratedTaxonomyAndKeepBounded(t *testing.T) {
	categories := make([]MarketplaceCategory, 296)
	for index := range categories {
		category := fmt.Sprintf("community-%03d", index)
		if index < len(curatedMarketplaceCategories) {
			category = curatedMarketplaceCategories[len(curatedMarketplaceCategories)-index-1]
		}
		categories[index] = MarketplaceCategory{
			Category: category,
			Count:    index,
		}
	}
	categoriesJSON, err := json.Marshal(categories)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := &fakeSkillMarketplace{categoriesJSON: categoriesJSON}
	service := NewService(WithLobeHubFetcher(fetcher))
	result, err := service.SearchMarketplace(context.Background(), MarketplaceSearchInput{})
	if err != nil || len(result.Categories) != len(curatedMarketplaceCategories) {
		t.Fatalf("SearchMarketplace() category count = %d, %v", len(result.Categories), err)
	}
	for index, category := range result.Categories {
		if category.Category != curatedMarketplaceCategories[index] {
			t.Fatalf("category order[%d] = %q", index, category.Category)
		}
	}

	bounded := make([]MarketplaceCategory, maxMarketplaceCategories)
	for index := range bounded {
		category := fmt.Sprintf("bounded-%03d", index)
		if index < len(curatedMarketplaceCategories) {
			category = curatedMarketplaceCategories[index]
		}
		bounded[index] = MarketplaceCategory{
			Category: category,
			Count:    index,
		}
	}
	fetcher.categoriesJSON, err = json.Marshal(bounded)
	if err != nil {
		t.Fatal(err)
	}
	result, err = service.SearchMarketplace(context.Background(), MarketplaceSearchInput{})
	if err != nil || len(result.Categories) != len(curatedMarketplaceCategories) {
		t.Fatalf("bounded category count = %d, %v", len(result.Categories), err)
	}

	oversized := make([]MarketplaceCategory, maxMarketplaceCategories+1)
	for index := range oversized {
		oversized[index] = MarketplaceCategory{
			Category: fmt.Sprintf("oversized-%03d", index),
			Count:    index,
		}
	}
	fetcher.categoriesJSON, err = json.Marshal(oversized)
	if err != nil {
		t.Fatal(err)
	}
	result, err = service.SearchMarketplace(context.Background(), MarketplaceSearchInput{})
	if err != nil || len(result.Categories) != 0 {
		t.Fatalf("oversized categories should fail open with item list: %#v, %v", result.Categories, err)
	}
}

func (fake *fakeSkillMarketplace) FetchPublicSkillDetailJSON(
	_ context.Context, path string, _ int64,
) ([]byte, error) {
	fake.paths = append(fake.paths, path)
	if !strings.HasPrefix(path, "/api/v1/skills/owner-demo?") {
		return nil, fmt.Errorf("unexpected detail path %s", path)
	}
	if fake.detailJSON != nil {
		return append([]byte(nil), fake.detailJSON...), nil
	}
	return []byte(`{
		"identifier":"owner-demo","name":"Demo Skill","description":"Marketplace demo",
		"version":"1.2.3","category":"productivity-tasks","installCount":7,
		"ratingAverage":4.5,"isOfficial":false,"isValidated":true,"isFeatured":true,
		"author":{"name":"Owner"},"github":{"url":"https://github.com/owner/demo"},
		"license":{"name":"MIT"},
		"manifest":{"name":"demo-skill",
			"sourceUrl":"https://github.com/owner/demo/tree/main/skills/demo-skill",
			"description":"Marketplace demo","permissions":["Read"]},
		"overview":{"summary":"Demo summary"},"resources":{},
		"versions":[{"version":"1.2.3","isLatest":true,"isValidated":true,
			"createdAt":"2026-08-28T00:00:00Z","versionNumber":1}]
	}`), nil
}

func marketplaceGitHubFixture(t *testing.T) (SourceHTTPClient, *[]string) {
	t.Helper()
	commit := strings.Repeat("c", 40)
	skill := []byte(validSkillMarkdown("demo-skill"))
	blobSHA := gitBlobSHA(skill)
	calls := []string{}
	client := sourceRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls = append(calls, request.URL.String())
		switch request.URL.String() {
		case "https://api.github.com/repos/owner/demo/commits/main":
			return sourceResponse(request, "application/json", `{"sha":"`+commit+`"}`), nil
		case "https://api.github.com/repos/owner/demo/contents/skills/demo-skill?ref=" + commit:
			return sourceResponse(request, "application/json", fmt.Sprintf(
				`[{"name":"SKILL.md","path":"skills/demo-skill/SKILL.md","sha":"%s","size":%d,"type":"file"}]`,
				blobSHA, len(skill),
			)), nil
		case "https://raw.githubusercontent.com/owner/demo/" + commit + "/skills/demo-skill/SKILL.md":
			return sourceResponse(request, "application/octet-stream", string(skill)), nil
		default:
			return nil, fmt.Errorf("unexpected GitHub request %s", request.URL)
		}
	})
	return client, &calls
}

func TestMarketplaceSearchDetailAndExactOwnerPrivateInstall(t *testing.T) {
	fetcher := &fakeSkillMarketplace{}
	githubClient, githubCalls := marketplaceGitHubFixture(t)
	repository := newMemoryRepository()
	service := NewService(
		WithRepository(repository), WithObjectStore(newMemoryObjectStore()), WithLobeHubFetcher(fetcher),
		WithGitHubClient(githubClient),
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
		len(detail.Resources) != 0 || detail.Installed {
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
	if fetcher.packageCalls != 0 || len(*githubCalls) != 3 ||
		!strings.Contains((*githubCalls)[0], "/commits/main") ||
		!strings.Contains((*githubCalls)[1], "/contents/skills/demo-skill?ref="+strings.Repeat("c", 40)) ||
		!strings.Contains((*githubCalls)[2], "raw.githubusercontent.com/owner/demo/"+strings.Repeat("c", 40)) {
		t.Fatalf("legacy/GitHub package calls = %d/%#v", fetcher.packageCalls, *githubCalls)
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

func TestMarketplaceInstallRejectsMismatchedGitHubSourceBeforeIO(t *testing.T) {
	fetcher := &fakeSkillMarketplace{detailJSON: []byte(`{
		"identifier":"owner-demo","name":"Demo Skill","description":"Marketplace demo",
		"version":"1.2.3","category":"productivity-tasks","installCount":7,
		"ratingAverage":4.5,"author":{"name":"Owner"},
		"manifest":{"name":"demo-skill","sourceUrl":"https://github.com/owner/demo/tree/main/skills/other-skill"},
		"resources":{},"versions":[]
	}`)}
	githubCalls := 0
	repository := newMemoryRepository()
	objects := newMemoryObjectStore()
	service := NewService(
		WithRepository(repository), WithObjectStore(objects), WithLobeHubFetcher(fetcher),
		WithGitHubClient(sourceRoundTripFunc(func(*http.Request) (*http.Response, error) {
			githubCalls++
			return nil, fmt.Errorf("GitHub must not be called")
		})),
	)

	_, err := service.InstallMarketplaceSkill(
		context.Background(), testSkillUser, "owner-demo", "1.2.3",
	)
	if !errors.Is(err, ErrInvalidSource) || githubCalls != 0 || fetcher.packageCalls != 0 ||
		len(repository.candidates) != 0 || len(objects.objects) != 0 {
		t.Fatalf("mismatched source error=%v github=%d legacy=%d candidates=%d objects=%d",
			err, githubCalls, fetcher.packageCalls, len(repository.candidates), len(objects.objects))
	}
}

func TestMarketplaceInstallRejectsGitHubBlobDriftWithoutMutation(t *testing.T) {
	fetcher := &fakeSkillMarketplace{}
	commit := strings.Repeat("d", 40)
	skill := []byte(validSkillMarkdown("demo-skill"))
	blobSHA := gitBlobSHA(skill)
	rawCalls := 0
	client := sourceRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.String() {
		case "https://api.github.com/repos/owner/demo/commits/main":
			return sourceResponse(request, "application/json", `{"sha":"`+commit+`"}`), nil
		case "https://api.github.com/repos/owner/demo/contents/skills/demo-skill?ref=" + commit:
			return sourceResponse(request, "application/json", fmt.Sprintf(
				`[{"name":"SKILL.md","path":"skills/demo-skill/SKILL.md","sha":"%s","size":%d,"type":"file"}]`,
				blobSHA, len(skill),
			)), nil
		case "https://raw.githubusercontent.com/owner/demo/" + commit + "/skills/demo-skill/SKILL.md":
			rawCalls++
			return sourceResponse(request, "application/octet-stream", strings.Repeat("x", len(skill))), nil
		default:
			return nil, fmt.Errorf("unexpected GitHub request %s", request.URL)
		}
	})
	repository := newMemoryRepository()
	objects := newMemoryObjectStore()
	service := NewService(
		WithRepository(repository), WithObjectStore(objects), WithLobeHubFetcher(fetcher),
		WithGitHubClient(client),
	)

	_, err := service.InstallMarketplaceSkill(
		context.Background(), testSkillUser, "owner-demo", "1.2.3",
	)
	if !errors.Is(err, ErrSourceUnavailable) || rawCalls != 1 || fetcher.packageCalls != 0 ||
		len(repository.candidates) != 0 || len(objects.objects) != 0 {
		t.Fatalf("blob drift error=%v raw=%d legacy=%d candidates=%d objects=%d",
			err, rawCalls, fetcher.packageCalls, len(repository.candidates), len(objects.objects))
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
	fetcher := &fakeSkillMarketplace{}
	githubClient, _ := marketplaceGitHubFixture(t)
	service := NewService(
		WithRepository(newMemoryRepository()), WithObjectStore(newMemoryObjectStore()),
		WithLobeHubFetcher(fetcher), WithGitHubClient(githubClient),
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
	invalid = skillRequest(handler, http.MethodGet,
		"/v1/skills/marketplace?category=_meta", nil, "", testSkillUser)
	assertSkillHTTP(t, invalid, http.StatusBadRequest, "INVALID_SKILL_MARKETPLACE_QUERY")
	if len(fetcher.paths) != pathCount {
		t.Fatalf("non-curated category reached Marketplace: %#v", fetcher.paths[pathCount:])
	}
}
