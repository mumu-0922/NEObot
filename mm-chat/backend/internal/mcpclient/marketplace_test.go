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
			if request.Header.Get("Authorization") != "" || request.URL.Query().Get("version") != "" ||
				request.URL.Query().Get("locale") != "zh-CN" {
				t.Fatalf("detail auth/query = %q %#v", request.Header.Get("Authorization"), request.URL.Query())
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
		case "/.well-known/oauth-protected-resource/mcp":
			http.NotFound(writer, request)
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
		MarketplaceCompatibilityIncompatible,
		MarketplaceCompatibilityRequiresRunner,
		MarketplaceCompatibilityIncompatible,
	}
	if strings.Join(compatibilities, ",") != strings.Join(want, ",") || len(detail.Deployments[0].Hash) != 64 {
		t.Fatalf("compatibilities=%v hash=%q", compatibilities, detail.Deployments[0].Hash)
	}
}

func TestNormalizeLobeDeploymentUsesApprovedHeaderWithoutPersistingQuerySecret(t *testing.T) {
	t.Parallel()
	var option lobeDeploymentOption
	if err := json.Unmarshal([]byte(`{
	  "connection":{"type":"http","url":"https://mcp.tavily.com/mcp/?tavilyApiKey={{config.TAVILY_API_KEY}}",
	    "configSchema":{"type":"object","required":["TAVILY_API_KEY"],"properties":{"TAVILY_API_KEY":{"type":"string"}}}},
	  "installationMethod":"none"
	}`), &option); err != nil {
		t.Fatal(err)
	}
	deployment := normalizeLobeDeployment("tavily-ai-tavily-mcp", "0.2.19", option)
	if deployment.EndpointURL != "https://mcp.tavily.com/mcp/" || deployment.InstallMode != "header" ||
		deployment.HeaderName != "Authorization" || len(deployment.SecretFields) != 1 ||
		strings.Contains(deployment.EndpointURL, "TAVILY_API_KEY") || strings.Contains(deployment.EndpointURL, "tavilyApiKey") {
		t.Fatalf("deployment=%#v", deployment)
	}
}

func TestLobeHubMarketplaceResolvesNPMTagToExactPackageVersion(t *testing.T) {
	t.Parallel()
	var detailCalls, npmCalls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/plugins/playwright":
			detailCalls.Add(1)
			_, _ = io.WriteString(writer, `{
			  "identifier":"playwright","name":"Playwright MCP",
			  "version":"1.62.0-alpha-1783623505000",
			  "deploymentOptions":[{
			    "connection":{"type":"stdio","command":"npx","args":["@playwright/mcp@latest"]},
			    "installationDetails":{"packageName":"@playwright/mcp"},
			    "installationMethod":"npm","isRecommended":true
			  }]
			}`)
		case "/registry/@playwright/mcp/latest":
			npmCalls.Add(1)
			_, _ = io.WriteString(writer, `{"name":"@playwright/mcp","version":"0.0.79"}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	marketplace, err := NewLobeHubMarketplace(LobeHubMarketplaceConfig{
		BaseURL: server.URL, NPMRegistryURL: server.URL + "/registry",
		ClientID: "client", ClientSecret: strings.Repeat("s", 32),
		Timeout: 5 * time.Second, CacheTTL: time.Minute, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := marketplace.GetItem(context.Background(), "playwright", "1.62.0-alpha-1783623505000")
	if err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}
	if len(detail.Deployments) != 1 ||
		detail.Deployments[0].PackageSpec != "@playwright/mcp@0.0.79" {
		t.Fatalf("deployment = %#v", detail.Deployments)
	}
	if detailCalls.Load() != 1 || npmCalls.Load() != 1 {
		t.Fatalf("detail/npm calls = %d/%d, want 1/1", detailCalls.Load(), npmCalls.Load())
	}
}

func TestLobeHubMarketplaceRejectsMissingNPMVersion(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/plugins/missing":
			_, _ = io.WriteString(writer, `{
			  "identifier":"missing","name":"Missing MCP","version":"9.9.9",
			  "deploymentOptions":[{
			    "connection":{"type":"stdio","command":"npx","args":["missing-mcp"]},
			    "installationDetails":{"packageName":"missing-mcp"},
			    "installationMethod":"npm","isRecommended":true
			  }]
			}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	marketplace, err := NewLobeHubMarketplace(LobeHubMarketplaceConfig{
		BaseURL: server.URL, NPMRegistryURL: server.URL,
		ClientID: "client", ClientSecret: strings.Repeat("s", 32),
		Timeout: 5 * time.Second, CacheTTL: time.Minute, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := marketplace.GetItem(context.Background(), "missing", "9.9.9")
	if err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}
	if len(detail.Deployments) != 1 ||
		detail.Deployments[0].Compatibility != MarketplaceCompatibilityIncompatible ||
		detail.Deployments[0].PackageSpec != "" {
		t.Fatalf("deployment = %#v", detail.Deployments)
	}
}

func TestNPMRegistryURLRequiresHTTPS(t *testing.T) {
	t.Parallel()
	_, err := NewLobeHubMarketplace(LobeHubMarketplaceConfig{
		BaseURL: "https://market.example.test", NPMRegistryURL: "http://registry.example.test",
		ClientID: "client", ClientSecret: strings.Repeat("s", 32),
		Timeout: 5 * time.Second, CacheTTL: time.Minute,
	})
	if !errors.Is(err, ErrMarketplaceUnavailable) {
		t.Fatalf("NewLobeHubMarketplace() error = %v, want ErrMarketplaceUnavailable", err)
	}
}

func TestDynamicMarketplaceArtifactUsesResolvedExactNPMVersion(t *testing.T) {
	t.Parallel()
	deployment := MarketplaceDeployment{
		ConnectionType: "stdio", InstallationMethod: "npm", Command: "npx",
		Args: []string{"@playwright/mcp@latest"}, PackageName: "@playwright/mcp",
		PackageSpec: "@playwright/mcp@0.0.79", Hash: strings.Repeat("a", 64),
	}
	artifact, ok := dynamicMarketplaceArtifact(MarketplaceItemDetail{
		MarketplaceItem: MarketplaceItem{Identifier: "playwright", Name: "Playwright MCP"},
		Version:         "1.62.0-alpha-1783623505000",
	}, deployment)
	if !ok {
		t.Fatal("dynamicMarketplaceArtifact() rejected registry-resolved package")
	}
	dynamic, _ := artifact.Metadata["dynamicRunnerArtifact"].(DynamicRunnerArtifact)
	if dynamic.PackageSpec != "@playwright/mcp@0.0.79" || len(dynamic.Args) != 0 {
		t.Fatalf("dynamic artifact = %#v", dynamic)
	}
}

func TestBindPrivateServerDisplayPreservesDynamicIconAndFailsClosed(t *testing.T) {
	t.Parallel()
	deployment := MarketplaceDeployment{
		ConnectionType: "stdio", InstallationMethod: "npm", Command: "npx",
		Args: []string{"@playwright/mcp@latest"}, PackageName: "@playwright/mcp",
		PackageSpec: "@playwright/mcp@0.0.79", Hash: strings.Repeat("a", 64),
	}
	detail := MarketplaceItemDetail{
		MarketplaceItem: MarketplaceItem{
			Identifier: "playwright", Name: "Playwright MCP", Icon: "https://github.com/microsoft.png",
		},
		Version: "1.62.0-alpha-1783623505000",
	}
	artifact, ok := dynamicMarketplaceArtifact(detail, deployment)
	if !ok {
		t.Fatal("dynamicMarketplaceArtifact() rejected registry-resolved package")
	}
	dynamic, _ := artifact.Metadata["dynamicRunnerArtifact"].(DynamicRunnerArtifact)
	installed := Server{
		Ref:         ServerRef{Source: SourcePrivate, ID: uuid.NewString()},
		Name:        detail.Name,
		Icon:        artifact.Icon,
		Transport:   TransportStdio,
		EndpointURL: "runner://" + artifact.Ref.ID,
		AuthType:    AuthNone,
		Metadata: map[string]any{
			"runnerArtifactId":      artifact.Ref.ID,
			"dynamicRunnerArtifact": dynamicRunnerArtifactMap(dynamic),
			"marketplace": map[string]any{
				"provider": marketplaceProviderLobeHub, "identifier": detail.Identifier,
				"version": detail.Version, "deploymentHash": deployment.Hash,
			},
		},
	}
	service, err := NewService(testMCPConfig(), newFakeRepository(), nil, nil, nil, Catalog{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	service.bindPrivateServerDisplay(&installed)
	if installed.Icon != detail.Icon {
		t.Fatalf("bound dynamic icon = %q, want %q", installed.Icon, detail.Icon)
	}

	installed.Metadata["runnerArtifactId"] = "tampered-artifact"
	service.bindPrivateServerDisplay(&installed)
	if installed.Icon != "" {
		t.Fatalf("tampered dynamic icon = %q, want fail-closed empty icon", installed.Icon)
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
		MarketplaceItem: MarketplaceItem{
			Identifier: "deepwiki", Name: "DeepWiki", Icon: "https://github.com/deepwiki.png",
		},
		Version: "1.2.3", Deployments: []MarketplaceDeployment{deployment},
	}}
	config := testMCPAdminConfig(userID)
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
	if result.Server.Icon != "https://github.com/deepwiki.png" {
		t.Fatalf("installed remote icon = %q", result.Server.Icon)
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
		Icon:   "https://github.com/upstash.png",
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
		MarketplaceItem: MarketplaceItem{
			Identifier: "upstash-context7", Name: "Context7 platform - documentation for every prompt", Icon: "https://untrusted.example/context7.png",
		},
		Version: "2.2.0", Deployments: []MarketplaceDeployment{deployment},
	}}
	config := testMCPAdminConfig(userID)
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
	detail, err := service.MarketplaceItem(context.Background(), uuid.NewString(), "upstash-context7", "2.2.0")
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
		result.Server.Name != artifact.Name ||
		result.Server.EndpointURL != "runner://"+artifact.Ref.ID ||
		result.Server.Metadata["runnerArtifactId"] != artifact.Ref.ID {
		t.Fatalf("installed Runner server = %#v", result.Server)
	}
	if len(result.Server.Tools) != 1 ||
		result.Server.Tools[0].Classification != ClassificationRead {
		t.Fatalf("installed Runner Tool policy = %#v", result.Server.Tools)
	}
	if result.Server.Icon != artifact.Icon {
		t.Fatalf("installed Runner icon = %q, want current artifact %q", result.Server.Icon, artifact.Icon)
	}
	servers, err := service.ListServers(context.Background(), userID, "")
	if err != nil || len(servers) != 1 || servers[0].Ref != result.Server.Ref ||
		servers[0].Name != artifact.Name || servers[0].Icon != artifact.Icon || len(servers[0].Tools) != 1 ||
		servers[0].Tools[0].Classification != ClassificationRead {
		t.Fatalf("ListServers() servers=%#v error=%v", servers, err)
	}
	resolved, err := service.serverForUser(
		context.Background(), userID, result.Server.Ref, ConversationScope{},
	)
	if err != nil || resolved.Name != artifact.Name {
		t.Fatalf("serverForUser() Server=%#v error=%v", resolved, err)
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

func TestMarketplaceItemMarksExactInstalledVersion(t *testing.T) {
	t.Parallel()
	userID := uuid.NewString()
	repo := newFakeRepository()
	repo.private[userID+":"+uuid.NewString()] = Server{
		Ref: ServerRef{Source: SourcePrivate, ID: uuid.NewString()},
		Metadata: map[string]any{"marketplace": map[string]any{
			"provider": marketplaceProviderLobeHub, "identifier": "context7", "version": "2.2.0",
		}},
	}
	config := testMCPAdminConfig(userID)
	config.MarketplaceEnabled = true
	service, err := NewService(
		config, repo, nil, nil, nil, Catalog{}, nil,
		WithMarketplace(&fakeMarketplace{detail: MarketplaceItemDetail{
			MarketplaceItem: MarketplaceItem{Identifier: "context7", Name: "Context7"}, Version: "2.2.0",
		}}),
	)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := service.MarketplaceItem(context.Background(), userID, "context7", "2.2.0")
	if err != nil || !detail.Installed {
		t.Fatalf("MarketplaceItem() detail=%#v error=%v", detail, err)
	}
}

func TestInstallMarketplaceRunnerCredentialProbeControlsReadyStatus(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		probeError error
		wantStatus string
		wantCode   string
	}{
		{name: "invalid credential stays recoverable", probeError: ErrCredentialInvalid, wantStatus: ServerStatusNeedsAuth, wantCode: "credential_invalid"},
		{name: "live credential becomes ready", wantStatus: ServerStatusReady},
		{name: "provider outage is unavailable", probeError: ErrServerUnavailable, wantStatus: ServerStatusUnavailable, wantCode: "credential_probe_failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			userID := uuid.NewString()
			deployment := MarketplaceDeployment{
				ConnectionType: "stdio", InstallationMethod: "npm", Recommended: true,
				Compatibility: MarketplaceCompatibilityNeedsConfig, InstallMode: "runner_env",
				Command: "npx", Args: []string{"-y", "probe-mcp@1.0.0"}, PackageName: "probe-mcp",
				SecretFields: []string{"API_KEY"},
			}
			deployment.Hash, _ = marketplaceDeploymentHash("probe-mcp", "1.0.0", deployment)
			artifact := Server{
				Ref: ServerRef{Source: SourceManifest, ID: "probe-mcp-1.0.0"}, Name: "Probe",
				Transport: TransportStdio, AuthType: AuthNone, Status: ServerStatusReady,
				Command:         &Command{Argv: []string{"/opt/mcp/probe"}, UserSecretEnv: []string{"API_KEY"}},
				CredentialProbe: &CredentialProbe{EndpointURL: "https://api.example.com/usage", Method: "GET", SecretField: "API_KEY", HeaderName: "Authorization"},
				Metadata: map[string]any{
					"marketplaceArtifact": MarketplaceArtifact{Provider: marketplaceProviderLobeHub, Identifier: "probe-mcp", Version: "1.0.0", ConnectionType: "stdio", InstallationMethod: "npm", DeploymentHash: deployment.Hash},
					"toolPolicy":          map[string]string{"lookup": ClassificationRead},
				},
			}
			config := testMCPAdminConfig(userID)
			config.StdioEnabled, config.MarketplaceEnabled = true, true
			service, err := NewService(
				config, newFakeRepository(), &fakeConnector{sessions: []Session{fakeSession{tools: []Tool{{Name: "lookup", Supported: true}}}}},
				testVault(t), nil, Catalog{}, []Server{artifact},
				WithMarketplace(&fakeMarketplace{detail: MarketplaceItemDetail{MarketplaceItem: MarketplaceItem{Identifier: "probe-mcp", Name: "Probe"}, Version: "1.0.0", Deployments: []MarketplaceDeployment{deployment}}}),
				withCredentialProbe(func(_ context.Context, _ Server, credential string) error {
					if strings.Contains(credential, "secret-value") {
						return test.probeError
					}
					return errors.New("credential missing from internal probe")
				}),
			)
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.InstallMarketplaceItem(context.Background(), userID, MarketplaceInstallInput{
				Identifier: "probe-mcp", Version: "1.0.0", DeploymentHash: deployment.Hash,
				Secrets: map[string]string{"API_KEY": "secret-value"},
			})
			if err != nil {
				t.Fatalf("InstallMarketplaceItem() error = %v", err)
			}
			if result.Server.Status != test.wantStatus || result.Server.LastErrorCode != test.wantCode ||
				(result.Server.Status == ServerStatusReady) != (result.ValidationError == "") {
				t.Fatalf("install result = %#v", result)
			}
		})
	}
}

func TestInstallMarketplaceItemReusesExactDraftAndRevalidatesNewCredential(t *testing.T) {
	t.Parallel()
	userID := uuid.NewString()
	repo := newFakeRepository()
	deployment := MarketplaceDeployment{
		ConnectionType: "stdio", InstallationMethod: "npm", Recommended: true,
		Compatibility: MarketplaceCompatibilityNeedsConfig, InstallMode: "runner_env",
		Command: "npx", Args: []string{"-y", "probe-mcp@1.0.0"}, PackageName: "probe-mcp",
		SecretFields: []string{"API_KEY"},
	}
	deployment.Hash, _ = marketplaceDeploymentHash("probe-mcp", "1.0.0", deployment)
	artifact := Server{
		Ref: ServerRef{Source: SourceManifest, ID: "probe-mcp-1.0.0"}, Name: "Probe",
		Transport: TransportStdio, AuthType: AuthNone, Status: ServerStatusReady,
		Command:         &Command{Argv: []string{"/opt/mcp/probe"}, UserSecretEnv: []string{"API_KEY"}},
		CredentialProbe: &CredentialProbe{EndpointURL: "https://api.example.com/usage", Method: "GET", SecretField: "API_KEY", HeaderName: "Authorization"},
		Metadata: map[string]any{
			"marketplaceArtifact": MarketplaceArtifact{Provider: marketplaceProviderLobeHub, Identifier: "probe-mcp", Version: "1.0.0", ConnectionType: "stdio", InstallationMethod: "npm", DeploymentHash: deployment.Hash},
		},
	}
	config := testMCPAdminConfig(userID)
	config.StdioEnabled, config.MarketplaceEnabled = true, true
	service, err := NewService(
		config, repo, &fakeConnector{sessions: []Session{fakeSession{tools: []Tool{}}, fakeSession{tools: []Tool{}}}},
		testVault(t), nil, Catalog{}, []Server{artifact},
		WithMarketplace(&fakeMarketplace{detail: MarketplaceItemDetail{MarketplaceItem: MarketplaceItem{Identifier: "probe-mcp", Name: "Probe"}, Version: "1.0.0", Deployments: []MarketplaceDeployment{deployment}}}),
		withCredentialProbe(func(_ context.Context, _ Server, credential string) error {
			if strings.Contains(credential, "invalid-key") {
				return ErrCredentialInvalid
			}
			if strings.Contains(credential, "valid-key") {
				return nil
			}
			return errors.New("unexpected credential")
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.InstallMarketplaceItem(context.Background(), userID, MarketplaceInstallInput{
		Identifier: "probe-mcp", Version: "1.0.0", DeploymentHash: deployment.Hash,
		Secrets: map[string]string{"API_KEY": "invalid-key"},
	})
	if err != nil || first.Server.Status != ServerStatusNeedsAuth {
		t.Fatalf("first install result=%#v error=%v", first, err)
	}
	second, err := service.InstallMarketplaceItem(context.Background(), userID, MarketplaceInstallInput{
		Identifier: "probe-mcp", Version: "1.0.0", DeploymentHash: deployment.Hash,
		Secrets: map[string]string{"API_KEY": "valid-key"},
	})
	if err != nil {
		t.Fatalf("second install error=%v", err)
	}
	if second.Server.Ref != first.Server.Ref || second.Server.Status != ServerStatusReady ||
		second.ValidationError != "" || len(repo.private) != 1 {
		t.Fatalf("reused install first=%#v second=%#v private=%d", first, second, len(repo.private))
	}
}

func TestInstallMarketplaceItemDoesNotReuseUnrelatedEndpoint(t *testing.T) {
	t.Parallel()
	userID := uuid.NewString()
	repo := newFakeRepository()
	endpoint := "https://1.1.1.1/mcp"
	if _, err := repo.CreatePrivateServer(context.Background(), userID, CreateServerInput{
		Name: "Custom", EndpointURL: endpoint, AuthType: AuthNone,
	}); err != nil {
		t.Fatal(err)
	}
	deployment := MarketplaceDeployment{
		ConnectionType: "http", InstallationMethod: "none", Recommended: true,
		Compatibility: MarketplaceCompatibilityInstallable, EndpointURL: endpoint,
	}
	deployment.Hash, _ = marketplaceDeploymentHash("deepwiki", "1.2.3", deployment)
	config := testMCPAdminConfig(userID)
	config.MarketplaceEnabled = true
	service, err := NewService(
		config, repo, nil, nil, nil, Catalog{}, nil,
		WithMarketplace(&fakeMarketplace{detail: MarketplaceItemDetail{MarketplaceItem: MarketplaceItem{Identifier: "deepwiki", Name: "DeepWiki"}, Version: "1.2.3", Deployments: []MarketplaceDeployment{deployment}}}),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.InstallMarketplaceItem(context.Background(), userID, MarketplaceInstallInput{
		Identifier: "deepwiki", Version: "1.2.3", DeploymentHash: deployment.Hash,
	})
	if err != ErrServerConflict || len(repo.private) != 1 {
		t.Fatalf("install error=%v private=%d", err, len(repo.private))
	}
}

func TestInstallMarketplaceItemSupportsCustomRelayURLAndHeaderCredential(t *testing.T) {
	t.Parallel()
	userID := uuid.NewString()
	repo := newFakeRepository()
	deployment := MarketplaceDeployment{
		ConnectionType: "stdio", InstallationMethod: "npm", Recommended: true,
		Compatibility: MarketplaceCompatibilityRequiresRunner,
		Command:       "npx", Args: []string{"tavily-mcp"}, PackageName: "tavily-mcp",
		SecretFields: []string{"TAVILY_API_KEY"},
	}
	deployment.Hash, _ = marketplaceDeploymentHash("tavily", "1.0.0", deployment)
	config := testMCPAdminConfig(userID)
	config.MarketplaceEnabled = true
	service, err := NewService(
		config, repo, &fakeConnector{sessions: []Session{fakeSession{tools: []Tool{}}}},
		testVault(t), nil, Catalog{}, nil,
		WithMarketplace(&fakeMarketplace{detail: MarketplaceItemDetail{
			MarketplaceItem: MarketplaceItem{Identifier: "tavily", Name: "Tavily", Icon: "https://github.com/tavily-ai.png"},
			Version:         "1.0.0", Deployments: []MarketplaceDeployment{deployment},
		}}),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.InstallMarketplaceItem(context.Background(), userID, MarketplaceInstallInput{
		Identifier: "tavily", Version: "1.0.0",
		CustomEndpointURL: "https://1.1.1.1/mcp", CustomAuthType: AuthHeader,
		CustomHeaderName: "X-API-Key", CustomCredential: "relay-key",
	})
	if err != nil {
		t.Fatalf("InstallMarketplaceItem() error=%v", err)
	}
	if result.Server.EndpointURL != "https://1.1.1.1/mcp" || result.Server.AuthType != AuthHeader ||
		result.Server.HeaderAuth == nil || result.Server.HeaderAuth.Name != "X-API-Key" ||
		result.Server.Status != ServerStatusReady || !result.Server.HasCredential || len(repo.private) != 1 {
		t.Fatalf("custom relay result=%#v private=%d", result, len(repo.private))
	}
	provenance, _ := result.Server.Metadata["marketplace"].(map[string]any)
	if provenance["connectionMode"] != "custom_remote" || provenance["identifier"] != "tavily" {
		t.Fatalf("custom relay provenance=%#v", provenance)
	}
	if strings.Contains(result.Server.EndpointURL, "relay-key") {
		t.Fatal("credential leaked into endpoint URL")
	}
}

func TestInstallMarketplaceItemRejectsUnsafeCustomRelayURL(t *testing.T) {
	t.Parallel()
	userID := uuid.NewString()
	config := testMCPAdminConfig(userID)
	config.MarketplaceEnabled = true
	service, err := NewService(
		config, newFakeRepository(), nil, nil, nil, Catalog{}, nil,
		WithMarketplace(&fakeMarketplace{detail: MarketplaceItemDetail{
			MarketplaceItem: MarketplaceItem{Identifier: "tavily", Name: "Tavily"}, Version: "1.0.0",
			Deployments: []MarketplaceDeployment{{ConnectionType: "http"}},
		}}),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.InstallMarketplaceItem(context.Background(), userID, MarketplaceInstallInput{
		Identifier: "tavily", Version: "1.0.0", CustomEndpointURL: "http://127.0.0.1:9000/mcp",
		CustomAuthType: AuthNone,
	})
	if err != ErrURLBlocked {
		t.Fatalf("unsafe relay error=%v, want ErrURLBlocked", err)
	}
}

func TestInstallMarketplaceItemRejectsCustomRelayForCredentialFreeStdio(t *testing.T) {
	t.Parallel()
	userID := uuid.NewString()
	config := testMCPAdminConfig(userID)
	config.MarketplaceEnabled = true
	service, err := NewService(
		config, newFakeRepository(), nil, nil, nil, Catalog{}, nil,
		WithMarketplace(&fakeMarketplace{detail: MarketplaceItemDetail{
			MarketplaceItem: MarketplaceItem{Identifier: "context7", Name: "Context7"}, Version: "1.0.0",
			Deployments: []MarketplaceDeployment{{
				ConnectionType: "stdio", InstallationMethod: "npm",
				Compatibility: MarketplaceCompatibilityInstallable, InstallMode: "direct",
			}},
		}}),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.InstallMarketplaceItem(context.Background(), userID, MarketplaceInstallInput{
		Identifier: "context7", Version: "1.0.0", CustomEndpointURL: "https://1.1.1.1/mcp",
		CustomAuthType: AuthNone,
	})
	if err != ErrMarketplaceIncompatible {
		t.Fatalf("credential-free stdio relay error=%v, want ErrMarketplaceIncompatible", err)
	}
}

func TestInstallMarketplaceItemRejectsMixedCustomAndOfficialConfiguration(t *testing.T) {
	t.Parallel()
	userID := uuid.NewString()
	config := testMCPAdminConfig(userID)
	config.MarketplaceEnabled = true
	service, err := NewService(
		config, newFakeRepository(), nil, nil, nil, Catalog{}, nil,
		WithMarketplace(&fakeMarketplace{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.InstallMarketplaceItem(context.Background(), userID, MarketplaceInstallInput{
		Identifier: "tavily", Version: "1.0.0", CustomEndpointURL: "https://1.1.1.1/mcp",
		CustomAuthType: AuthHeader, CustomHeaderName: "Authorization", CustomCredential: "relay-key",
		DeploymentHash: strings.Repeat("a", 64), Secrets: map[string]string{"API_KEY": "official-key"},
	})
	if err != ErrSelectionInvalid {
		t.Fatalf("mixed install error=%v, want ErrSelectionInvalid", err)
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
			name: "dynamic npm artifact", stdioEnabled: true,
			wantReason: "Approved shared Runner artifact",
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
			userID := uuid.NewString()
			config := testMCPAdminConfig(userID)
			config.MarketplaceEnabled = true
			config.StdioEnabled = test.stdioEnabled
			service, err := NewService(
				config, newFakeRepository(), nil, nil, nil, Catalog{}, test.manifest,
				WithMarketplace(&fakeMarketplace{detail: detail}),
			)
			if err != nil {
				t.Fatal(err)
			}
			approved, err := service.MarketplaceItem(context.Background(), uuid.NewString(), "upstash-context7", "2.2.0")
			wantCompatibility := MarketplaceCompatibilityRequiresRunner
			if test.stdioEnabled {
				wantCompatibility = MarketplaceCompatibilityInstallable
			}
			if err != nil || len(approved.Deployments) != 1 ||
				approved.Deployments[0].Compatibility != wantCompatibility ||
				approved.Deployments[0].CompatibilityReason != test.wantReason {
				t.Fatalf("MarketplaceItem() detail=%#v error=%v", approved, err)
			}
			_, err = service.InstallMarketplaceItem(context.Background(), userID, MarketplaceInstallInput{
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
	config := testMCPAdminConfig(userID)
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
