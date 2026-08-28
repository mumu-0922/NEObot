package mcpclient

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseManifestRejectsUnknownDuplicateAndUnsafeCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "unknown field",
			raw:  `{"version":1,"servers":[],"future":true}`,
		},
		{
			name: "duplicate id",
			raw: `{"version":1,"servers":[
                {"id":"same","name":"One","transport":"streamable_http","endpointUrl":"https://one.example/mcp"},
                {"id":"same","name":"Two","transport":"streamable_http","endpointUrl":"https://two.example/mcp"}
              ]}`,
		},
		{
			name: "relative executable",
			raw:  `{"version":1,"servers":[{"id":"local","name":"Local","transport":"stdio","command":{"argv":["node","server.js"]}}]}`,
		},
		{
			name: "shell executable",
			raw:  `{"version":1,"servers":[{"id":"local","name":"Local","transport":"stdio","command":{"argv":["/bin/sh","-c","server"]}}]}`,
		},
		{
			name: "remote user info",
			raw:  `{"version":1,"servers":[{"id":"remote","name":"Remote","transport":"streamable_http","endpointUrl":"https://user:pass@example.com/mcp"}]}`,
		},
		{
			name: "unsafe icon",
			raw:  `{"version":1,"servers":[{"id":"remote","name":"Remote","icon":"http://127.0.0.1/icon.png","transport":"streamable_http","endpointUrl":"https://example.com/mcp"}]}`,
		},
		{
			name: "empty Tool allowlist",
			raw:  `{"version":1,"servers":[{"id":"local","name":"Local","transport":"stdio","command":{"argv":["/opt/mcp/local"]},"allowedTools":[]}]}`,
		},
		{
			name: "duplicate Tool allowlist",
			raw:  `{"version":1,"servers":[{"id":"local","name":"Local","transport":"stdio","command":{"argv":["/opt/mcp/local"]},"allowedTools":["lookup","lookup"]}]}`,
		},
		{
			name: "Tool policy outside allowlist",
			raw:  `{"version":1,"servers":[{"id":"local","name":"Local","transport":"stdio","command":{"argv":["/opt/mcp/local"]},"allowedTools":["lookup"],"toolPolicy":{"mutate":"write"}}]}`,
		},
		{
			name: "run scope on remote",
			raw:  `{"version":1,"servers":[{"id":"remote","name":"Remote","transport":"streamable_http","endpointUrl":"https://example.com/mcp","instanceScope":"run"}]}`,
		},
		{
			name: "unknown instance scope",
			raw:  `{"version":1,"servers":[{"id":"local","name":"Local","transport":"stdio","command":{"argv":["/opt/mcp/local"]},"instanceScope":"conversation"}]}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseManifest([]byte(test.raw), nil)
			if !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("parseManifest() error = %v, want ErrManifestInvalid", err)
			}
		})
	}
}

func TestParseManifestNormalizesApprovedRemoteAndStdioServers(t *testing.T) {
	t.Parallel()
	raw := `{
      "version": 1,
      "servers": [
        {
          "id": "remote",
          "name": "Remote",
          "icon": "https://cdn.example/remote.png",
          "transport": "streamable_http",
          "endpointUrl": "https://mcp.example/api?tenant=one",
          "auth": {"type":"oauth","clientId":"neo-chat","scopes":["tools.read","tools.read"]},
          "grants": [{"scopeType":"global","defaultEnabled":true}],
          "toolPolicy": {"lookup":"read"}
        },
        {
          "id": "typescript-server",
          "name": "TypeScript",
          "transport": "stdio",
          "command": {
            "argv": ["/opt/mcp/typescript-language-server", "--stdio"],
            "env": [{"name":"MCP_MODE","value":"strict"}],
            "idleTimeout": "10m",
            "maxLifetime": "2h"
          }
        }
      ]
    }`
	servers, err := parseManifest([]byte(raw), nil)
	if err != nil {
		t.Fatalf("parseManifest() error = %v", err)
	}
	if len(servers) != 2 || servers[0].Ref.ID != "remote" || servers[1].Ref.ID != "typescript-server" {
		t.Fatalf("servers = %#v", servers)
	}
	if servers[0].OAuthClient == nil || len(servers[0].OAuthClient.Scopes) != 1 {
		t.Fatalf("oauth client = %#v", servers[0].OAuthClient)
	}
	if servers[0].Icon != "https://cdn.example/remote.png" {
		t.Fatalf("remote icon = %q", servers[0].Icon)
	}
	command := servers[1].Command
	if command == nil || command.IdleTimeout != 10*time.Minute || command.MaxLifetime != 2*time.Hour || command.Env["MCP_MODE"] != "strict" {
		t.Fatalf("command = %#v", command)
	}
}

func TestParseManifestNormalizesMarketplaceRunnerArtifact(t *testing.T) {
	t.Parallel()
	raw := `{
      "version": 1,
      "servers": [{
        "id": "marketplace-upstash-context7-2.2.0",
        "name": "Context7",
        "icon": "https://github.com/upstash.png",
        "transport": "stdio",
        "command": {"argv":["/opt/mcp-runner/node_modules/.bin/context7-mcp"]},
        "marketplace": {
          "provider": "lobehub",
          "identifier": "upstash-context7",
          "version": "2.2.0",
          "connectionType": "stdio",
          "installationMethod": "npm",
          "command": "npx",
          "args": ["ctx7"],
          "packageName": "@upstash/context7-mcp"
        }
      }]
    }`
	servers, err := parseManifest([]byte(raw), nil)
	if err != nil {
		t.Fatalf("parseManifest() error = %v", err)
	}
	artifact, ok := marketplaceArtifactFromServer(servers[0])
	if !ok {
		t.Fatalf("marketplace artifact metadata = %#v", servers[0].Metadata)
	}
	deployment := MarketplaceDeployment{
		ConnectionType: "stdio", InstallationMethod: "npm", Command: "npx",
		Args: []string{"ctx7"}, PackageName: "@upstash/context7-mcp",
	}
	wantHash, err := marketplaceDeploymentHash("upstash-context7", "2.2.0", deployment)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Provider != marketplaceProviderLobeHub || artifact.Identifier != "upstash-context7" ||
		artifact.Version != "2.2.0" || artifact.DeploymentHash != wantHash {
		t.Fatalf("marketplace artifact = %#v, want hash %q", artifact, wantHash)
	}
	if servers[0].Icon != "https://github.com/upstash.png" {
		t.Fatalf("marketplace artifact icon = %q", servers[0].Icon)
	}
}

func TestParseManifestBindsMarketplaceRunnerSecretFieldsIntoArtifactHash(t *testing.T) {
	t.Parallel()
	raw := `{"version":1,"servers":[{
	  "id":"marketplace-tavily-ai-tavily-mcp-0.2.19","name":"Tavily","transport":"stdio",
	  "command":{"argv":["/opt/mcp-runner/node_modules/.bin/tavily-mcp"],"userSecretEnv":["TAVILY_API_KEY"]},
	  "marketplace":{"provider":"lobehub","identifier":"tavily-ai-tavily-mcp","version":"0.2.19",
	    "connectionType":"stdio","installationMethod":"npm","command":"npx","args":["-y","tavily-mcp@latest"],"packageName":"tavily-mcp"}
	}]}`
	servers, err := parseManifest([]byte(raw), nil)
	if err != nil {
		t.Fatal(err)
	}
	artifact, ok := marketplaceArtifactFromServer(servers[0])
	if !ok {
		t.Fatal("missing Marketplace artifact")
	}
	deployment := MarketplaceDeployment{
		ConnectionType: "stdio", InstallationMethod: "npm", Command: "npx",
		Args: []string{"-y", "tavily-mcp@latest"}, PackageName: "tavily-mcp",
		SecretFields: []string{"TAVILY_API_KEY"},
	}
	want, _ := marketplaceDeploymentHash("tavily-ai-tavily-mcp", "0.2.19", deployment)
	if artifact.DeploymentHash != want || servers[0].AuthType != AuthNone {
		t.Fatalf("artifact=%#v server=%#v", artifact, servers[0])
	}
}

func TestParseManifestNormalizesReviewedCredentialProbe(t *testing.T) {
	t.Parallel()
	raw := `{"version":1,"servers":[{
	  "id":"tavily","name":"Tavily","transport":"stdio",
	  "command":{"argv":["/opt/mcp/tavily"],"userSecretEnv":["TAVILY_API_KEY"]},
	  "credentialProbe":{"endpointUrl":"https://api.tavily.com/usage","method":"GET",
	    "secretField":"TAVILY_API_KEY","headerName":"Authorization","headerPrefix":"Bearer "}
	}]}`
	servers, err := parseManifest([]byte(raw), nil)
	if err != nil {
		t.Fatal(err)
	}
	probe := servers[0].CredentialProbe
	if probe == nil || probe.EndpointURL != "https://api.tavily.com/usage" ||
		probe.SecretField != "TAVILY_API_KEY" || probe.HeaderPrefix != "Bearer " {
		t.Fatalf("credential probe = %#v", probe)
	}
}

func TestParseManifestRejectsUnboundOrMutableCredentialProbe(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`{"version":1,"servers":[{"id":"bad","name":"Bad","transport":"stdio","command":{"argv":["/opt/mcp/bad"],"userSecretEnv":["API_KEY"]},"credentialProbe":{"endpointUrl":"https://api.example.com/usage","method":"POST","secretField":"API_KEY","headerName":"Authorization"}}]}`,
		`{"version":1,"servers":[{"id":"bad","name":"Bad","transport":"stdio","command":{"argv":["/opt/mcp/bad"],"userSecretEnv":["API_KEY"]},"credentialProbe":{"endpointUrl":"https://api.example.com/usage?target=mutable","method":"GET","secretField":"API_KEY","headerName":"Authorization"}}]}`,
		`{"version":1,"servers":[{"id":"bad","name":"Bad","transport":"stdio","command":{"argv":["/opt/mcp/bad"],"userSecretEnv":["API_KEY"]},"credentialProbe":{"endpointUrl":"https://api.example.com/usage","method":"GET","secretField":"OTHER_KEY","headerName":"Authorization"}}]}`,
	} {
		if _, err := parseManifest([]byte(raw), nil); !errors.Is(err, ErrManifestInvalid) {
			t.Fatalf("parseManifest() error = %v, want ErrManifestInvalid", err)
		}
	}
}

func TestProductionTavilyManifestBindsCurrentPackageToolNames(t *testing.T) {
	t.Parallel()
	manifestPath := filepath.Join("..", "..", "..", "mcp", "manifest.json")
	servers, err := LoadManifest(manifestPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	var tavily Server
	for _, server := range servers {
		if server.Ref.ID == "marketplace-tavily-ai-tavily-mcp-0.2.19" {
			tavily = server
			break
		}
	}
	policy, _ := tavily.Metadata["toolPolicy"].(map[string]string)
	for _, name := range []string{
		"tavily_search", "tavily_extract", "tavily_map", "tavily_crawl", "tavily_research",
	} {
		if policy[name] != ClassificationRead {
			t.Fatalf("Tavily policy[%q] = %q, want read", name, policy[name])
		}
	}
	if tavily.CredentialProbe == nil || tavily.CredentialProbe.EndpointURL != "https://api.tavily.com/usage" {
		t.Fatalf("Tavily credential probe = %#v", tavily.CredentialProbe)
	}
}

func TestProductionManifestDoesNotShipRetiredBrowser(t *testing.T) {
	t.Parallel()
	manifestPath := filepath.Join("..", "..", "..", "mcp", "manifest.json")
	servers, err := LoadManifest(manifestPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, server := range servers {
		if server.Ref.ID == "playwright-browser-0.0.79" || server.Name == "Browser (Playwright)" {
			t.Fatalf("retired built-in Browser remains in production manifest: %#v", server.Ref)
		}
	}
}

func TestManifestToolAllowlistAndRunInstanceBinding(t *testing.T) {
	t.Parallel()
	server := Server{
		Ref:       ServerRef{Source: SourceManifest, ID: "browser"},
		Transport: TransportStdio,
		Metadata: map[string]any{
			manifestAllowedTools: []string{"browser_snapshot"},
			runnerInstanceScope:  manifestInstanceRun,
		},
	}
	if !manifestToolAllowed(server, "browser_snapshot") ||
		manifestToolAllowed(server, "browser_run_code_unsafe") {
		t.Fatal("manifest Tool allowlist did not fail closed")
	}
	server.Tools = []Tool{
		{Name: "browser_snapshot", Supported: true},
		{Name: "browser_run_code_unsafe", Supported: true},
	}
	server.ToolCount = 2
	filtered := filterManifestAllowedTools(server)
	if len(filtered.Tools) != 1 || filtered.Tools[0].Name != "browser_snapshot" ||
		filtered.ToolCount != 1 {
		t.Fatalf("filtered manifest snapshot = %#v", filtered)
	}
	first := bindManifestRunnerInstance(
		server, "11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333",
	)
	replay := bindManifestRunnerInstance(
		server, "11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333",
	)
	second := bindManifestRunnerInstance(
		server, "11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
		"44444444-4444-4444-8444-444444444444",
	)
	firstID, _ := first.Metadata[runnerInstanceID].(string)
	replayID, _ := replay.Metadata[runnerInstanceID].(string)
	secondID, _ := second.Metadata[runnerInstanceID].(string)
	if !manifestIDPattern.MatchString(firstID) || firstID != replayID || firstID == secondID {
		t.Fatalf("run instances = %q/%q/%q", firstID, replayID, secondID)
	}
	if _, mutated := server.Metadata[runnerInstanceID]; mutated {
		t.Fatal("run instance binding mutated the manifest authority")
	}
}

func TestParseManifestRejectsMarketplaceArtifactOnRemoteOrAuthenticatedServer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "remote transport",
			raw: `{"version":1,"servers":[{
              "id":"artifact","name":"Artifact","transport":"streamable_http",
              "endpointUrl":"https://mcp.example/tools",
              "marketplace":{"provider":"lobehub","identifier":"fixture","version":"1.0.0",
                "connectionType":"stdio","installationMethod":"npm","command":"npx","args":["fixture"]}
            }]}`,
		},
		{
			name: "authenticated stdio",
			raw: `{"version":1,"servers":[{
              "id":"artifact","name":"Artifact","transport":"stdio",
              "command":{"argv":["/opt/mcp/fixture"]},
              "auth":{"type":"oauth","clientId":"neo-chat"},
              "marketplace":{"provider":"lobehub","identifier":"fixture","version":"1.0.0",
                "connectionType":"stdio","installationMethod":"npm","command":"npx","args":["fixture"]}
            }]}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseManifest([]byte(test.raw), nil); !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("parseManifest() error = %v, want ErrManifestInvalid", err)
			}
		})
	}
}

func TestParseManifestOnlyResolvesAllowlistedSecretEnvironment(t *testing.T) {
	t.Parallel()
	manifest := func(ref string) string {
		return strings.ReplaceAll(`{"version":1,"servers":[{"id":"local","name":"Local","transport":"stdio","command":{"argv":["/opt/mcp/server"],"env":[{"name":"API_TOKEN","envRef":"ENV_REF"}]}}]}`, "ENV_REF", ref)
	}
	lookup := func(key string) (string, bool) {
		if key == "MCP_SECRET_TOKEN" {
			return "secret-value", true
		}
		return "", false
	}
	servers, err := parseManifest([]byte(manifest("MCP_SECRET_TOKEN")), lookup)
	if err != nil {
		t.Fatalf("parse allowlisted envRef: %v", err)
	}
	if got := servers[0].Command.Env["API_TOKEN"]; got != "secret-value" {
		t.Fatalf("resolved env = %q", got)
	}
	if _, err := parseManifest([]byte(manifest("HOME")), lookup); !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("unsafe envRef error = %v", err)
	}
}
