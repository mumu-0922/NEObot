package mcpclient

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestLobeHubMarketplaceM2MSearchCacheAndDetailCompatibility(t *testing.T) {
	t.Parallel()
	fixedNow := time.Date(2026, 8, 11, 8, 0, 0, 0, time.UTC)
	const clientID = "neo-chat-test"
	const clientSecret = "0123456789abcdef0123456789abcdef"
	var tokenCalls, searchCalls, detailCalls atomic.Int32

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/oauth/token":
			tokenCalls.Add(1)
			if request.Method != http.MethodPost {
				t.Fatalf("token method = %s", request.Method)
			}
			body, _ := io.ReadAll(request.Body)
			form, _ := url.ParseQuery(string(body))
			assertion := form.Get("client_assertion")
			assertMarketplaceAssertion(t, assertion, clientID, clientSecret, "https://"+request.Host+"/oauth/token")
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"access_token":"market-token","expires_in":3600,"token_type":"Bearer"}`)
		case "/api/v1/plugins":
			searchCalls.Add(1)
			if request.Header.Get("Authorization") != "Bearer market-token" ||
				request.URL.Query().Get("q") != "deep" || request.URL.Query().Get("locale") != "zh-CN" ||
				request.URL.Query().Get("category") != "developer" {
				t.Fatalf("search auth/query = %q %#v", request.Header.Get("Authorization"), request.URL.Query())
			}
			_, _ = io.WriteString(writer, `{
                  "currentPage":1,"pageSize":20,"totalCount":1,"totalPages":1,
                  "items":[{"identifier":"deepwiki","name":"DeepWiki","description":"Repository docs",
                    "icon":"https://github.com/example.png",
                    "author":"LobeHub","connectionType":"remote","installationMethods":"none",
                    "toolsCount":3,"installCount":42,"isOfficial":true,"isValidated":true,
                  "ratingAverage":4.8,"github":{"stars":99,"url":"https://github.com/example/deepwiki"}}]
                }`)
		case "/api/v1/plugins/categories":
			if request.URL.Query().Get("q") != "deep" || request.URL.Query().Get("locale") != "zh-CN" {
				t.Fatalf("category query = %#v", request.URL.Query())
			}
			_, _ = io.WriteString(writer, `[{"category":"developer","count":42}]`)
		case "/api/v1/plugins/deepwiki":
			detailCalls.Add(1)
			if request.URL.Query().Get("version") != "" || request.URL.Query().Get("locale") != "zh-CN" {
				t.Fatalf("detail query = %#v", request.URL.Query())
			}
			_, _ = io.WriteString(writer, `{
                  "identifier":"deepwiki","name":"DeepWiki","description":"Repository docs","version":"1.2.3",
                  "author":{"name":"LobeHub"},"connectionType":"remote","toolsCount":3,
                  "homepage":"https://example.com/deepwiki","github":{"stars":99,"url":"https://github.com/example/deepwiki"},
                  "overview":{"summary":"Read repository documentation"},
                  "tools":[{"name":"read_wiki","description":"Read docs"}],
                  "deploymentOptions":[
                    {"connection":{"type":"http","url":"https://mcp.deepwiki.com/mcp"},"installationMethod":"none","isRecommended":true},
                    {"connection":{"type":"http","url":"https://api.example.com/mcp","configSchema":{"required":["apiKey"]}},"installationMethod":"none"},
                    {"connection":{"type":"stdio","command":"npx","args":["evil"]},"installationMethod":"npm"},
                    {"connection":{"type":"sse","url":"https://example.com/sse"},"installationMethod":"none"}
                  ]
                }`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	marketplace, err := NewLobeHubMarketplace(LobeHubMarketplaceConfig{
		BaseURL: server.URL, ClientID: clientID, ClientSecret: clientSecret,
		Timeout: 5 * time.Second, CacheTTL: time.Minute, HTTPClient: server.Client(),
		Now: func() time.Time { return fixedNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	input := MarketplaceSearchInput{Query: "deep", Category: "developer", Page: 1, PageSize: 20}
	first, err := marketplace.Search(context.Background(), input)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	second, err := marketplace.Search(context.Background(), input)
	if err != nil || len(second.Items) != 1 || first.Items[0].Identifier != "deepwiki" ||
		first.Items[0].Icon != "https://github.com/example.png" ||
		len(first.Categories) != 1 || first.Categories[0].Count != 42 {
		t.Fatalf("cached Search() first=%#v second=%#v error=%v", first, second, err)
	}
	if tokenCalls.Load() != 1 || searchCalls.Load() != 1 {
		t.Fatalf("calls token=%d search=%d, want 1/1", tokenCalls.Load(), searchCalls.Load())
	}

	detail, err := marketplace.GetItem(context.Background(), "deepwiki", "1.2.3")
	if err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}
	if detail.Version != "1.2.3" || len(detail.Tools) != 1 || len(detail.Deployments) != 4 || detailCalls.Load() != 1 {
		t.Fatalf("detail = %#v calls=%d", detail, detailCalls.Load())
	}
	if _, err := marketplace.GetItem(context.Background(), "deepwiki", "1.2.4"); !errors.Is(err, ErrMarketplaceChanged) {
		t.Fatalf("GetItem() drift error = %v, want ErrMarketplaceChanged", err)
	}
	compatibilities := []string{
		detail.Deployments[0].Compatibility,
		detail.Deployments[1].Compatibility,
		detail.Deployments[2].Compatibility,
		detail.Deployments[3].Compatibility,
	}
	want := []string{
		MarketplaceCompatibilityInstallable,
		MarketplaceCompatibilityNeedsConfig,
		MarketplaceCompatibilityRequiresRunner,
		MarketplaceCompatibilityIncompatible,
	}
	if strings.Join(compatibilities, ",") != strings.Join(want, ",") || len(detail.Deployments[0].Hash) != 64 {
		t.Fatalf("compatibilities=%v hash=%q", compatibilities, detail.Deployments[0].Hash)
	}
}

func TestLobeHubMarketplaceTokenFetchIsSingleflight(t *testing.T) {
	t.Parallel()
	var tokenCalls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/oauth/token" {
			tokenCalls.Add(1)
			time.Sleep(20 * time.Millisecond)
			_, _ = io.WriteString(writer, `{"access_token":"one-token","expires_in":3600}`)
			return
		}
		_, _ = io.WriteString(writer, `{"currentPage":1,"pageSize":1,"totalCount":0,"totalPages":0,"items":[]}`)
	}))
	defer server.Close()
	marketplace, err := NewLobeHubMarketplace(LobeHubMarketplaceConfig{
		BaseURL: server.URL, ClientID: "client", ClientSecret: strings.Repeat("s", 32),
		Timeout: 5 * time.Second, CacheTTL: time.Minute, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for index := 0; index < 8; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, callErr := marketplace.Search(context.Background(), MarketplaceSearchInput{
				Query: "query-" + string(rune('a'+index)), Page: 1, PageSize: 1,
			})
			if callErr != nil {
				t.Errorf("Search(%d) error = %v", index, callErr)
			}
		}(index)
	}
	wait.Wait()
	if tokenCalls.Load() != 1 {
		t.Fatalf("token calls = %d, want 1", tokenCalls.Load())
	}
}

func TestInstallMarketplaceItemUsesPrivateValidationAndSelection(t *testing.T) {
	t.Parallel()
	userID, conversationID := uuid.NewString(), uuid.NewString()
	repo := newFakeRepository()
	repo.scopes[userID+":"+conversationID] = ConversationScope{ConversationID: conversationID, UserID: userID}
	repo.selections[userID+":"+conversationID] = Selection{
		ConversationID: conversationID, Mode: SelectionModeCustom, Revision: 3, Servers: []SelectionServer{},
	}
	deployment := MarketplaceDeployment{
		ConnectionType: "http", InstallationMethod: "none", Recommended: true,
		Compatibility: MarketplaceCompatibilityInstallable, EndpointURL: "https://1.1.1.1/mcp",
	}
	deployment.Hash, _ = marketplaceDeploymentHash("deepwiki", "1.2.3", deployment)
	marketplace := &fakeMarketplace{detail: MarketplaceItemDetail{
		MarketplaceItem: MarketplaceItem{Identifier: "deepwiki", Name: "DeepWiki"},
		Version:         "1.2.3", Deployments: []MarketplaceDeployment{deployment},
	}}
	config := testMCPConfig()
	config.MarketplaceEnabled = true
	service, err := NewService(
		config, repo, &fakeConnector{sessions: []Session{fakeSession{tools: []Tool{}}}},
		nil, nil, Catalog{}, nil, WithMarketplace(marketplace),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.InstallMarketplaceItem(context.Background(), userID, MarketplaceInstallInput{
		Identifier: "deepwiki", Version: "1.2.3", ConversationID: conversationID,
		SelectionRevision: 3, EnableForConversation: true,
	})
	if err != nil {
		t.Fatalf("InstallMarketplaceItem() error = %v", err)
	}
	if result.Server.Status != ServerStatusReady || !result.Enabled || result.Selection == nil ||
		len(result.Selection.Servers) != 1 || result.Selection.Servers[0].Ref != result.Server.Ref {
		t.Fatalf("install result = %#v", result)
	}
	provenance, _ := result.Server.Metadata["marketplace"].(map[string]any)
	if provenance["identifier"] != "deepwiki" || provenance["deploymentHash"] != deployment.Hash {
		t.Fatalf("provenance = %#v", provenance)
	}
}

func TestInstallMarketplaceItemUsesApprovedSharedRunnerArtifact(t *testing.T) {
	t.Parallel()
	userID := uuid.NewString()
	repo := newFakeRepository()
	deployment := MarketplaceDeployment{
		ConnectionType: "stdio", InstallationMethod: "npm", Recommended: true,
		Compatibility: MarketplaceCompatibilityRequiresRunner,
		Command:       "npx", Args: []string{"ctx7"}, PackageName: "@upstash/context7-mcp",
	}
	deployment.Hash, _ = marketplaceDeploymentHash("upstash-context7", "2.2.0", deployment)
	artifact := Server{
		Ref:  ServerRef{Source: SourceManifest, ID: "marketplace-upstash-context7-2.2.0"},
		Name: "Context7 artifact", Transport: TransportStdio, AuthType: AuthNone,
		Status: ServerStatusReady, Command: &Command{Argv: []string{"/opt/mcp/context7-mcp"}},
		Metadata: map[string]any{
			"marketplaceArtifact": MarketplaceArtifact{
				Provider: marketplaceProviderLobeHub, Identifier: "upstash-context7", Version: "2.2.0",
				ConnectionType: "stdio", InstallationMethod: "npm", DeploymentHash: deployment.Hash,
			},
			"toolPolicy": map[string]string{"query-docs": ClassificationRead},
		},
	}
	marketplace := &fakeMarketplace{detail: MarketplaceItemDetail{
		MarketplaceItem: MarketplaceItem{Identifier: "upstash-context7", Name: "Context7"},
		Version:         "2.2.0", Deployments: []MarketplaceDeployment{deployment},
	}}
	config := testMCPConfig()
	config.RemoteEnabled = false
	config.StdioEnabled = true
	config.MarketplaceEnabled = true
	service, err := NewService(
		config, repo, &fakeConnector{sessions: []Session{fakeSession{tools: []Tool{{Name: "query-docs", Supported: true}}}}},
		nil, nil, Catalog{}, []Server{artifact}, WithMarketplace(marketplace),
	)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := service.MarketplaceItem(context.Background(), "upstash-context7", "2.2.0")
	if err != nil || len(detail.Deployments) != 1 ||
		detail.Deployments[0].Compatibility != MarketplaceCompatibilityInstallable {
		t.Fatalf("MarketplaceItem() detail=%#v error=%v", detail, err)
	}
	result, err := service.InstallMarketplaceItem(context.Background(), userID, MarketplaceInstallInput{
		Identifier: "upstash-context7", Version: "2.2.0",
	})
	if err != nil {
		t.Fatalf("InstallMarketplaceItem() error = %v", err)
	}
	if result.Server.Transport != TransportStdio || result.Server.Status != ServerStatusReady ||
		result.Server.EndpointURL != "runner://"+artifact.Ref.ID ||
		result.Server.Metadata["runnerArtifactId"] != artifact.Ref.ID {
		t.Fatalf("installed Runner server = %#v", result.Server)
	}
	if len(result.Server.Tools) != 1 ||
		result.Server.Tools[0].Classification != ClassificationRead {
		t.Fatalf("installed Runner Tool policy = %#v", result.Server.Tools)
	}
	servers, err := service.ListServers(context.Background(), userID, "")
	if err != nil || len(servers) != 1 || servers[0].Ref != result.Server.Ref ||
		len(servers[0].Tools) != 1 || servers[0].Tools[0].Classification != ClassificationRead {
		t.Fatalf("ListServers() servers=%#v error=%v", servers, err)
	}
	repo.mu.Lock()
	tampered := repo.private[userID+":"+result.Server.Ref.ID]
	provenance, _ := tampered.Metadata["marketplace"].(map[string]any)
	provenance["version"] = "2.2.1"
	repo.private[userID+":"+result.Server.Ref.ID] = tampered
	repo.mu.Unlock()
	if _, err := service.serverForUser(
		context.Background(), userID, result.Server.Ref, ConversationScope{},
	); err != ErrServerUnavailable {
		t.Fatalf("serverForUser() tampered provenance error=%v, want ErrServerUnavailable", err)
	}
}

func TestMarketplaceStdioRequiresExactEnabledRunnerArtifact(t *testing.T) {
	t.Parallel()
	deployment := MarketplaceDeployment{
		ConnectionType: "stdio", InstallationMethod: "npm", Recommended: true,
		Compatibility: MarketplaceCompatibilityRequiresRunner,
		Command:       "npx", Args: []string{"ctx7"}, PackageName: "@upstash/context7-mcp",
	}
	deployment.Hash, _ = marketplaceDeploymentHash("upstash-context7", "2.2.0", deployment)
	detail := MarketplaceItemDetail{
		MarketplaceItem: MarketplaceItem{Identifier: "upstash-context7", Name: "Context7"},
		Version:         "2.2.0", Deployments: []MarketplaceDeployment{deployment},
	}
	artifact := Server{
		Ref: ServerRef{Source: SourceManifest, ID: "context7-artifact"}, Name: "Context7 artifact",
		Transport: TransportStdio, AuthType: AuthNone, Status: ServerStatusReady,
		Command: &Command{Argv: []string{"/opt/mcp/context7-mcp"}},
		Metadata: map[string]any{"marketplaceArtifact": MarketplaceArtifact{
			Provider: marketplaceProviderLobeHub, Identifier: "upstash-context7", Version: "2.2.0",
			ConnectionType: "stdio", InstallationMethod: "npm", DeploymentHash: deployment.Hash,
		}},
	}
	tests := []struct {
		name           string
		stdioEnabled   bool
		manifest       []Server
		wantReason     string
		wantInstallErr error
	}{
		{
			name: "no matching artifact", stdioEnabled: true,
			wantReason:     "No approved shared Runner artifact is available",
			wantInstallErr: ErrMarketplaceIncompatible,
		},
		{
			name: "matching artifact disabled", stdioEnabled: false, manifest: []Server{artifact},
			wantReason:     "The approved shared Runner artifact is disabled",
			wantInstallErr: ErrMarketplaceIncompatible,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			config := testMCPConfig()
			config.MarketplaceEnabled = true
			config.StdioEnabled = test.stdioEnabled
			service, err := NewService(
				config, newFakeRepository(), nil, nil, nil, Catalog{}, test.manifest,
				WithMarketplace(&fakeMarketplace{detail: detail}),
			)
			if err != nil {
				t.Fatal(err)
			}
			approved, err := service.MarketplaceItem(context.Background(), "upstash-context7", "2.2.0")
			if err != nil || len(approved.Deployments) != 1 ||
				approved.Deployments[0].Compatibility != MarketplaceCompatibilityRequiresRunner ||
				approved.Deployments[0].CompatibilityReason != test.wantReason {
				t.Fatalf("MarketplaceItem() detail=%#v error=%v", approved, err)
			}
			_, err = service.InstallMarketplaceItem(context.Background(), uuid.NewString(), MarketplaceInstallInput{
				Identifier: "upstash-context7", Version: "2.2.0",
			})
			if err != test.wantInstallErr {
				t.Fatalf("InstallMarketplaceItem() error=%v want=%v", err, test.wantInstallErr)
			}
		})
	}
}

func TestInstallMarketplaceItemRejectsStaleSelectionBeforeCreatingServer(t *testing.T) {
	t.Parallel()
	userID, conversationID := uuid.NewString(), uuid.NewString()
	repo := newFakeRepository()
	repo.scopes[userID+":"+conversationID] = ConversationScope{ConversationID: conversationID, UserID: userID}
	repo.selections[userID+":"+conversationID] = Selection{ConversationID: conversationID, Mode: SelectionModeCustom, Revision: 2}
	marketplace := &fakeMarketplace{}
	config := testMCPConfig()
	config.MarketplaceEnabled = true
	service, err := NewService(config, repo, nil, nil, nil, Catalog{}, nil, WithMarketplace(marketplace))
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.InstallMarketplaceItem(context.Background(), userID, MarketplaceInstallInput{
		Identifier: "deepwiki", Version: "1.2.3", ConversationID: conversationID,
		SelectionRevision: 1, EnableForConversation: true,
	})
	if err != ErrSelectionInvalid || marketplace.detailCalls.Load() != 0 || len(repo.private) != 0 {
		t.Fatalf("error=%v detailCalls=%d private=%d", err, marketplace.detailCalls.Load(), len(repo.private))
	}
}

type fakeMarketplace struct {
	detail      MarketplaceItemDetail
	detailErr   error
	detailCalls atomic.Int32
}

func (m *fakeMarketplace) Search(context.Context, MarketplaceSearchInput) (MarketplaceSearchResult, error) {
	return MarketplaceSearchResult{}, nil
}

func (m *fakeMarketplace) GetItem(context.Context, string, string) (MarketplaceItemDetail, error) {
	m.detailCalls.Add(1)
	return m.detail, m.detailErr
}

func assertMarketplaceAssertion(t *testing.T, assertion, clientID, secret, audience string) {
	t.Helper()
	parts := strings.Split(assertion, ".")
	if len(parts) != 3 {
		t.Fatalf("assertion segments = %d", len(parts))
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		t.Fatal("assertion signature invalid")
	}
	claimsJSON, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims struct {
		Issuer   string `json:"iss"`
		Subject  string `json:"sub"`
		Audience string `json:"aud"`
	}
	if json.Unmarshal(claimsJSON, &claims) != nil || claims.Issuer != clientID ||
		claims.Subject != clientID || claims.Audience != audience {
		t.Fatalf("assertion claims = %#v", claims)
	}
}
