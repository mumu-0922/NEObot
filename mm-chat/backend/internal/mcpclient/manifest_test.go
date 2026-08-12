package mcpclient

import (
	"errors"
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
