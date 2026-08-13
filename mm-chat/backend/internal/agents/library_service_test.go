package agents

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestInstallRequiresAdmittedMatchingFingerprint(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(WithRepository(repo))
	snapshot, err := normalizeCustomSnapshot(CreateCustomInput{
		Avatar: "🤖", Title: "Writer", Description: "Writes", Category: "writing",
		Tags: []string{"writing"}, SystemPrompt: "You are a careful writer.",
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SourceIdentifier = "writer"
	snapshot.ContentFingerprint = fingerprintSnapshot(snapshot)
	repo.admissions["writer"] = Admission{Snapshot: snapshot, Status: AdmissionAdmitted}

	if _, err := service.Install(context.Background(), testUserID, "writer", "0"+snapshot.ContentFingerprint[1:]); !errors.Is(err, ErrMarketChanged) {
		t.Fatalf("mismatched fingerprint error = %v, want ErrMarketChanged", err)
	}
	if len(repo.library) != 0 {
		t.Fatal("mismatched fingerprint created partial library entry")
	}
	entry, err := service.Install(context.Background(), testUserID, "writer", snapshot.ContentFingerprint)
	if err != nil || entry.SourceIdentifier != "writer" || entry.SystemPrompt == "" {
		t.Fatalf("install = %#v, %v", entry, err)
	}
}

func TestUpdateCustomRejectsStaleRevisionAndPreservesCurrent(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(WithRepository(repo))
	created, err := service.CreateCustom(context.Background(), testUserID, CreateCustomInput{
		Avatar: "🤖", Title: "One", Category: "general", SystemPrompt: "Prompt one",
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.UpdateCustom(context.Background(), testUserID, created.ID, UpdateCustomInput{
		ExpectedRevision:  created.Revision,
		CreateCustomInput: CreateCustomInput{Avatar: "🤖", Title: "Two", Category: "general", SystemPrompt: "Prompt two"},
	})
	if err != nil || updated.Revision != 2 {
		t.Fatalf("first update = %#v, %v", updated, err)
	}
	_, err = service.UpdateCustom(context.Background(), testUserID, created.ID, UpdateCustomInput{
		ExpectedRevision:  created.Revision,
		CreateCustomInput: CreateCustomInput{Avatar: "🤖", Title: "Stale", Category: "general", SystemPrompt: "stale"},
	})
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale update error = %v", err)
	}
	if repo.library[created.ID].Title != "Two" {
		t.Fatalf("stale update overwrote current = %#v", repo.library[created.ID])
	}
}

func TestCopyToCustomRejectsStaleSourceRevision(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(WithRepository(repo))
	snapshot, err := normalizeCustomSnapshot(CreateCustomInput{
		Title: "Writer", SystemPrompt: "Write carefully.",
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SourceIdentifier = "writer"
	snapshot.RequiredTools = []string{"search"}
	snapshot.ContentFingerprint = fingerprintSnapshot(snapshot)
	installed := repo.create(SourceLobeHub, snapshot)

	if _, err := service.CopyToCustom(context.Background(), testUserID, installed.ID, installed.Revision+1); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale copy error = %v, want ErrRevisionConflict", err)
	}
	if len(repo.library) != 1 {
		t.Fatalf("stale copy created entry; library=%#v", repo.library)
	}
	copy, err := service.CopyToCustom(context.Background(), testUserID, installed.ID, installed.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if copy.Source != SourceCustom || copy.RequiredTools[0] != "search" || copy.ContentFingerprint == installed.ContentFingerprint {
		t.Fatalf("copy = %#v", copy)
	}
}

func TestRequiredToolsAreCanonicalDisplayMetadata(t *testing.T) {
	agent := Agent{
		Identifier:    "writer",
		Meta:          AgentMeta{Title: "Writer", SystemRole: "Write carefully."},
		RequiredTools: []string{" search ", "Search", "files"},
	}
	snapshot, err := snapshotFromAgent(agent)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.RequiredTools) != 2 || snapshot.RequiredTools[0] != "search" || snapshot.RequiredTools[1] != "files" {
		t.Fatalf("required tools = %#v", snapshot.RequiredTools)
	}
	withoutTools := snapshot
	withoutTools.RequiredTools = nil
	withoutTools.ContentFingerprint = fingerprintSnapshot(withoutTools)
	if snapshot.ContentFingerprint == withoutTools.ContentFingerprint {
		t.Fatal("required Tool declarations were excluded from canonical fingerprint")
	}
	if got := agentFromSnapshot(snapshot, true).RequiredTools; len(got) != 2 {
		t.Fatalf("market agent required tools = %#v", got)
	}
}

func TestOrdinarySearchReturnsOnlyAdmissions(t *testing.T) {
	repo := newFakeRepository()
	approved, _ := normalizeCustomSnapshot(CreateCustomInput{Title: "Approved", SystemPrompt: "safe"})
	approved.SourceIdentifier = "approved"
	approved.ContentFingerprint = fingerprintSnapshot(approved)
	repo.admissions["approved"] = Admission{Snapshot: approved, Status: AdmissionAdmitted}
	rejected := approved
	rejected.SourceIdentifier = "rejected"
	rejected.ContentFingerprint = fingerprintSnapshot(rejected)
	repo.admissions["rejected"] = Admission{Snapshot: rejected, Status: AdmissionRejected}
	service := NewService(WithRepository(repo), WithAdministratorUserID(adminUserID))

	result, err := service.SearchMarket(context.Background(), testUserID, MarketSearchInput{Page: 1, PageSize: 20})
	if err != nil || len(result.Agents) != 1 || result.Agents[0].Identifier != "approved" {
		t.Fatalf("ordinary search = %#v, %v", result, err)
	}
}

func TestAdministratorCheckAcceptsConfiguredBootstrapIdentity(t *testing.T) {
	bootstrapUserID := "00000000-0000-0000-0000-000000000001"
	service := NewService(WithAdministratorUserID(bootstrapUserID))

	if !service.IsAdministrator(bootstrapUserID) {
		t.Fatal("configured bootstrap identity was not recognized as administrator")
	}
	if service.IsAdministrator(testUserID) {
		t.Fatal("ordinary user was recognized as administrator")
	}
	if NewService().IsAdministrator("") {
		t.Fatal("empty administrator identity was accepted")
	}
}

func TestLibraryAllowsMultipleCustomAssistants(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(WithRepository(repo))
	for _, title := range []string{"Writer", "Reviewer"} {
		if _, err := service.CreateCustom(context.Background(), testUserID, CreateCustomInput{
			Title: title, SystemPrompt: "Act as " + title + ".",
		}); err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
	}
	if len(repo.library) != 2 {
		t.Fatalf("custom assistant count = %d, want 2", len(repo.library))
	}
}

func TestMarketLibraryRequiresRepository(t *testing.T) {
	service := NewService(WithAdministratorUserID(adminUserID))
	if _, err := service.MarketDetail(context.Background(), adminUserID, "writer", LocaleEnglish); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatalf("market detail error = %v, want ErrRepositoryUnavailable", err)
	}
	if err := service.ReviewMarket(context.Background(), adminUserID, "writer", AdmissionAdmitted,
		strings.Repeat("a", 64), LocaleEnglish); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatalf("market review error = %v, want ErrRepositoryUnavailable", err)
	}
}

func TestLiveMarketPathsOmitEnglishLocaleSegment(t *testing.T) {
	tests := []struct {
		name     string
		locale   Locale
		resource string
		want     string
	}{
		{name: "english list", locale: LocaleEnglish, resource: "agent.data", want: "/agent.data"},
		{name: "english detail", locale: LocaleEnglish, resource: "/agent/writer.data", want: "/agent/writer.data"},
		{name: "chinese list", locale: LocaleChinese, resource: "agent.data", want: "/zh/agent.data"},
		{name: "japanese detail", locale: LocaleJapanese, resource: "agent/writer.data", want: "/ja/agent/writer.data"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := liveMarketPath(test.locale, test.resource); got != test.want {
				t.Fatalf("liveMarketPath() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestReviewMarketUsesLiveDetailFingerprint(t *testing.T) {
	requestedPaths := []string{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestedPaths = append(requestedPaths, request.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(liveDetailFixture("writer", "Live Writer", "You are a live writer."))
	}))
	defer upstream.Close()

	repo := newFakeRepository()
	service := NewService(WithRepository(repo), WithAdministratorUserID(adminUserID),
		WithLiveMarketBaseURL(upstream.URL))
	detail, err := service.MarketDetail(context.Background(), adminUserID, "writer", LocaleEnglish)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Homepage != upstream.URL+"/agent/writer" || !validFingerprint(detail.Fingerprint) {
		t.Fatalf("live detail = %#v", detail)
	}
	if err := service.ReviewMarket(context.Background(), adminUserID, "writer", AdmissionAdmitted,
		detail.Fingerprint, LocaleEnglish); err != nil {
		t.Fatalf("review live detail: %v", err)
	}
	admission := repo.admissions["writer"]
	if admission.ContentFingerprint != detail.Fingerprint || admission.SystemPrompt != "You are a live writer." {
		t.Fatalf("admission = %#v", admission)
	}
	for _, path := range requestedPaths {
		if path != "/agent/writer.data" {
			t.Fatalf("live detail path = %q, want /agent/writer.data", path)
		}
	}
}

func TestLiveListAndDetailUseSameCanonicalFingerprint(t *testing.T) {
	rawAgent := map[string]any{
		"identifier":  "writer",
		"name":        "Live Writer",
		"description": "Writes",
		"category":    "writing",
		"avatar":      "🤖",
		"config": map[string]any{
			"systemRole": "You are a live writer.",
			"plugins":    []any{},
		},
		"versionNumber": 1,
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/agent.data":
			_ = json.NewEncoder(w).Encode(singleFetchFixture(map[string]any{
				"items": []any{rawAgent}, "categories": []any{},
				"totalCount": 1, "totalMarketCount": 1,
			}))
		case "/agent/writer.data":
			_ = json.NewEncoder(w).Encode(singleFetchFixture(map[string]any{"detail": rawAgent}))
		default:
			http.NotFound(w, request)
		}
	}))
	defer upstream.Close()

	repo := newFakeRepository()
	service := NewService(WithRepository(repo), WithAdministratorUserID(adminUserID),
		WithLiveMarketBaseURL(upstream.URL))
	market, err := service.SearchMarket(context.Background(), adminUserID, MarketSearchInput{
		Locale: LocaleEnglish, Page: 1, PageSize: 20,
	})
	if err != nil || len(market.Agents) != 1 {
		t.Fatalf("live market = %#v, %v", market, err)
	}
	detail, err := service.MarketDetail(context.Background(), adminUserID, "writer", LocaleEnglish)
	if err != nil {
		t.Fatal(err)
	}
	if market.Agents[0].Homepage != detail.Homepage ||
		market.Agents[0].Fingerprint != detail.Fingerprint ||
		!validFingerprint(detail.Fingerprint) {
		t.Fatalf("list/detail mismatch: list=%#v detail=%#v", market.Agents[0], detail)
	}
}

func TestOfficialMarketListAndDetailUseAuthenticatedAdapter(t *testing.T) {
	rawAgent := map[string]any{
		"identifier":  "writer",
		"name":        "Official Writer",
		"description": "Writes",
		"category":    "copywriting",
		"avatar":      "📝",
		"author":      map[string]any{"name": "Market Author"},
		"config": map[string]any{
			"systemRole": "You are an official writer.",
			"plugins":    []any{},
		},
		"versionNumber": 3,
	}
	market := &fakeOfficialMarketClient{responses: map[string]any{
		"/api/v1/agents": map[string]any{
			"items":       []any{rawAgent},
			"currentPage": 1,
			"pageSize":    20,
			"totalCount":  840,
			"totalPages":  42,
		},
		"/api/v1/agents/detail/writer": rawAgent,
	}}
	repo := newFakeRepository()
	service := NewService(
		WithRepository(repo),
		WithAdministratorUserID(adminUserID),
		WithOfficialMarket(market),
	)
	result, err := service.SearchMarket(context.Background(), adminUserID, MarketSearchInput{
		Locale: LocaleChinese, Page: 1, PageSize: 20,
	})
	if err != nil || result.Source != "lobehub-market" || result.TotalCount != 840 ||
		result.TotalPages != 42 || len(result.Agents) != 1 {
		t.Fatalf("official market = %#v, %v", result, err)
	}
	if len(result.Categories) != 14 {
		t.Fatalf("official categories = %#v", result.Categories)
	}
	for _, category := range result.Categories {
		if category.ID == "engineering" || category.ID == "uncategorized" {
			t.Fatalf("unsupported category leaked: %#v", category)
		}
	}
	detail, err := service.MarketDetail(context.Background(), adminUserID, "writer", LocaleChinese)
	if err != nil {
		t.Fatal(err)
	}
	if result.Agents[0].Fingerprint != detail.Fingerprint ||
		!validFingerprint(detail.Fingerprint) || detail.Meta.SystemRole != "You are an official writer." {
		t.Fatalf("official list/detail mismatch: list=%#v detail=%#v", result.Agents[0], detail)
	}
	if !market.requested("/api/v1/agents", "locale=zh-CN") ||
		!market.requested("/api/v1/agents/detail/writer", "locale=zh-CN") {
		t.Fatalf("official requests = %#v", market.paths)
	}
}

func TestReviewMarketFallsBackToLegacyDetail(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/agent/legacy-writer.data":
			http.Error(w, "live market unavailable", http.StatusBadGateway)
		case "/legacy-writer.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"meta":{"avatar":"📝","title":"Legacy Writer","description":"Writes","category":"writing"},
				"config":{"systemRole":"You are a legacy writer."},
				"author":"Registry"
			}`))
		default:
			http.NotFound(w, request)
		}
	}))
	defer upstream.Close()

	repo := newFakeRepository()
	service := NewService(WithRepository(repo), WithAdministratorUserID(adminUserID),
		WithLiveMarketBaseURL(upstream.URL), WithRegistryBaseURL(upstream.URL))
	detail, err := service.MarketDetail(context.Background(), adminUserID, "legacy-writer", LocaleEnglish)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Version != "legacy" || !validFingerprint(detail.Fingerprint) {
		t.Fatalf("legacy detail = %#v", detail)
	}
	if err := service.ReviewMarket(context.Background(), adminUserID, "legacy-writer", AdmissionAdmitted,
		detail.Fingerprint, LocaleEnglish); err != nil {
		t.Fatalf("review legacy detail: %v", err)
	}
	admission := repo.admissions["legacy-writer"]
	if admission.ContentFingerprint != detail.Fingerprint || admission.SourceVersion != "legacy" {
		t.Fatalf("legacy admission = %#v", admission)
	}
}

func liveDetailFixture(identifier, title, systemPrompt string) []any {
	return []any{
		"detail",
		map[string]any{"_0": 2},
		map[string]any{"_3": 4, "_5": 6, "_7": 8, "_9": 10, "_11": 12, "_13": 14, "_17": 18},
		"identifier", identifier,
		"name", title,
		"description", "Writes",
		"category", "writing",
		"avatar", "🤖",
		"config", map[string]any{"_15": 16},
		"systemRole", systemPrompt,
		"versionNumber", 1,
	}
}

func singleFetchFixture(value any) []any {
	values := []any{}
	var encode func(any) int
	encode = func(current any) int {
		index := len(values)
		values = append(values, nil)
		switch typed := current.(type) {
		case map[string]any:
			encoded := make(map[string]any, len(typed))
			values[index] = encoded
			for key, item := range typed {
				keyIndex := encode(key)
				encoded["_"+strconv.Itoa(keyIndex)] = encode(item)
			}
		case []any:
			encoded := make([]any, 0, len(typed))
			values[index] = encoded
			for _, item := range typed {
				encoded = append(encoded, encode(item))
			}
			values[index] = encoded
		default:
			values[index] = current
		}
		return index
	}
	encode(value)
	return values
}

type fakeOfficialMarketClient struct {
	paths     []string
	responses map[string]any
}

func (client *fakeOfficialMarketClient) FetchAgentMarketJSON(
	_ context.Context,
	path string,
	_ int64,
) ([]byte, error) {
	client.paths = append(client.paths, path)
	base := strings.SplitN(path, "?", 2)[0]
	response, ok := client.responses[base]
	if !ok {
		return nil, ErrMarketUnavailable
	}
	return json.Marshal(response)
}

func (client *fakeOfficialMarketClient) requested(base, queryPart string) bool {
	for _, path := range client.paths {
		if strings.HasPrefix(path, base+"?") && strings.Contains(path, queryPart) {
			return true
		}
	}
	return false
}

const (
	testUserID  = "00000000-0000-4000-8000-000000000002"
	adminUserID = "00000000-0000-4000-8000-000000000001"
)

type fakeRepository struct {
	library    map[string]LibraryEntry
	admissions map[string]Admission
	next       int
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{library: map[string]LibraryEntry{}, admissions: map[string]Admission{}}
}

func (r *fakeRepository) ListLibrary(_ context.Context, _ string) ([]LibraryEntry, error) {
	items := []LibraryEntry{}
	for _, item := range r.library {
		items = append(items, item)
	}
	return items, nil
}
func (r *fakeRepository) GetLibrary(_ context.Context, _ string, id string) (LibraryEntry, error) {
	entry, ok := r.library[id]
	if !ok {
		return LibraryEntry{}, ErrLibraryNotFound
	}
	return entry, nil
}
func (r *fakeRepository) CreateCustom(_ context.Context, _ string, snapshot Snapshot) (LibraryEntry, error) {
	return r.create(SourceCustom, snapshot), nil
}
func (r *fakeRepository) UpdateCustom(_ context.Context, _ string, id string, revision int64, snapshot Snapshot) (LibraryEntry, error) {
	return r.update(id, SourceCustom, revision, snapshot)
}
func (r *fakeRepository) DeleteCustom(_ context.Context, _ string, id string, revision int64) error {
	return r.delete(id, SourceCustom, revision)
}
func (r *fakeRepository) Install(_ context.Context, _ string, snapshot Snapshot) (LibraryEntry, error) {
	for _, entry := range r.library {
		if entry.Source == SourceLobeHub && entry.SourceIdentifier == snapshot.SourceIdentifier {
			return LibraryEntry{}, ErrLibraryConflict
		}
	}
	return r.create(SourceLobeHub, snapshot), nil
}
func (r *fakeRepository) UpdateInstalled(_ context.Context, _ string, id string, revision int64, snapshot Snapshot) (LibraryEntry, error) {
	return r.update(id, SourceLobeHub, revision, snapshot)
}
func (r *fakeRepository) Uninstall(_ context.Context, _ string, id string, revision int64) error {
	return r.delete(id, SourceLobeHub, revision)
}
func (r *fakeRepository) GetAdmission(_ context.Context, id string) (Admission, error) {
	admission, ok := r.admissions[id]
	if !ok {
		return Admission{}, ErrAdmissionNotFound
	}
	return admission, nil
}
func (r *fakeRepository) ListAdmissions(_ context.Context, input MarketSearchInput) (MarketSearchResult, error) {
	agents := []Agent{}
	for _, admission := range r.admissions {
		if admission.Status == AdmissionAdmitted {
			agents = append(agents, agentFromSnapshot(admission.Snapshot, true))
		}
	}
	return MarketSearchResult{Agents: agents, Categories: []MarketCategory{}, Page: input.Page, PageSize: input.PageSize, TotalCount: len(agents), TotalPages: pageCount(len(agents), input.PageSize), Source: "neo-chat-admitted"}, nil
}
func (r *fakeRepository) UpsertAdmission(_ context.Context, _ string, admission Admission) error {
	r.admissions[admission.SourceIdentifier] = admission
	return nil
}
func (r *fakeRepository) create(source string, snapshot Snapshot) LibraryEntry {
	r.next++
	id := "00000000-0000-4000-8000-" + leftPad(r.next)
	entry := LibraryEntry{ID: id, Source: source, SourceIdentifier: snapshot.SourceIdentifier,
		Avatar: snapshot.Avatar, Title: snapshot.Title, Description: snapshot.Description,
		Category: snapshot.Category, Tags: snapshot.Tags, SystemPrompt: snapshot.SystemPrompt,
		Author: snapshot.Author, Homepage: snapshot.Homepage, SourceVersion: snapshot.SourceVersion,
		SourceUpdatedAt: snapshot.SourceUpdatedAt, RequiredTools: snapshot.RequiredTools,
		ContentFingerprint: snapshot.ContentFingerprint, Revision: 1}
	r.library[id] = entry
	return entry
}
func (r *fakeRepository) update(id, source string, revision int64, snapshot Snapshot) (LibraryEntry, error) {
	entry, ok := r.library[id]
	if !ok || entry.Source != source {
		return LibraryEntry{}, ErrLibraryNotFound
	}
	if entry.Revision != revision {
		return LibraryEntry{}, ErrRevisionConflict
	}
	entry.Avatar, entry.Title, entry.Description, entry.Category = snapshot.Avatar, snapshot.Title, snapshot.Description, snapshot.Category
	entry.Tags, entry.SystemPrompt, entry.ContentFingerprint = snapshot.Tags, snapshot.SystemPrompt, snapshot.ContentFingerprint
	entry.SourceIdentifier, entry.SourceVersion, entry.SourceUpdatedAt = snapshot.SourceIdentifier, snapshot.SourceVersion, snapshot.SourceUpdatedAt
	entry.RequiredTools = snapshot.RequiredTools
	entry.Revision++
	r.library[id] = entry
	return entry, nil
}
func (r *fakeRepository) delete(id, source string, revision int64) error {
	entry, ok := r.library[id]
	if !ok || entry.Source != source {
		return ErrLibraryNotFound
	}
	if entry.Revision != revision {
		return ErrRevisionConflict
	}
	delete(r.library, id)
	return nil
}
func leftPad(value int) string {
	const zeros = "000000000000"
	text := string(rune('0' + value))
	return zeros[:12-len(text)] + text
}
