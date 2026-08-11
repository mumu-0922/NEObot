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
	command := servers[1].Command
	if command == nil || command.IdleTimeout != 10*time.Minute || command.MaxLifetime != 2*time.Hour || command.Env["MCP_MODE"] != "strict" {
		t.Fatalf("command = %#v", command)
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
