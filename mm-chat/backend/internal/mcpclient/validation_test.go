package mcpclient

import (
	"errors"
	"regexp"
	"testing"
)

func TestToolAliasIsStableUniqueAndProviderCompatible(t *testing.T) {
	t.Parallel()
	first := toolAlias(ServerRef{Source: SourceManifest, ID: "alpha"}, "get-weather")
	second := toolAlias(ServerRef{Source: SourceManifest, ID: "beta"}, "get-weather")
	if first == second {
		t.Fatalf("aliases collide: %q", first)
	}
	if first != toolAlias(ServerRef{Source: SourceManifest, ID: "alpha"}, "get-weather") {
		t.Fatal("alias is not stable")
	}
	if len(first) > maxProviderAliasBytes || !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`).MatchString(first) {
		t.Fatalf("invalid provider alias %q", first)
	}
}

func TestNormalizeToolAndValidateArguments(t *testing.T) {
	t.Parallel()
	tool := normalizeTool(
		ServerRef{Source: SourceManifest, ID: "weather"},
		"lookup",
		"Lookup",
		"Look up weather",
		map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"city"},
			"properties": map[string]any{
				"city": map[string]any{"type": "string", "minLength": float64(1)},
			},
		},
		ClassificationRead,
	)
	if !tool.Supported {
		t.Fatalf("tool unsupported: %s", tool.Unsupported)
	}
	if err := ValidateToolArguments(tool, map[string]any{"city": "Shanghai"}); err != nil {
		t.Fatalf("valid arguments: %v", err)
	}
	for _, arguments := range []map[string]any{{}, {"city": ""}, {"city": "Shanghai", "token": "unexpected"}} {
		if err := ValidateToolArguments(tool, arguments); !errors.Is(err, ErrToolArgumentsInvalid) {
			t.Fatalf("arguments %#v error = %v", arguments, err)
		}
	}
}

func TestNormalizeToolDisablesUnsupportedSchemaWithoutWeakeningIt(t *testing.T) {
	t.Parallel()
	tool := normalizeTool(
		ServerRef{Source: SourcePrivate, ID: "server"},
		"complex",
		"",
		"",
		map[string]any{
			"type": "object",
			"oneOf": []any{
				map[string]any{"required": []any{"a"}},
				map[string]any{"required": []any{"b"}},
			},
		},
		ClassificationRead,
	)
	if tool.Supported || tool.Unsupported != "schema_unsupported" {
		t.Fatalf("tool = %#v", tool)
	}
	if tool.Classification != ClassificationUnknown {
		t.Fatalf("private classification = %q, want unknown", tool.Classification)
	}
}
