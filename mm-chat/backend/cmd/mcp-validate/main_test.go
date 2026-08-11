package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunValidatesManifestWithoutPrintingDefinitionsOrSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(`{
  "version": 1,
  "servers": [{
    "id": "remote",
    "name": "Private name",
    "transport": "streamable_http",
    "endpointUrl": "https://mcp.example/secret-path"
  }]
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run([]string{"--manifest", path}, &output); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	got := output.String()
	if !strings.Contains(got, "manifest=1 remote=1 stdio=0") ||
		strings.Contains(got, "Private name") || strings.Contains(got, "secret-path") {
		t.Fatalf("validation output = %q", got)
	}
}

func TestRunRejectsInvalidManifestAndArguments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"servers":[],"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{}, {"--manifest", path}, {"--manifest", path, "extra"}} {
		if len(args) == 2 {
			if err := run(args, ioDiscard{}); err == nil {
				t.Fatalf("run(%v) accepted invalid manifest", args)
			}
			continue
		}
		if err := run(args, ioDiscard{}); err == nil {
			t.Fatalf("run(%v) accepted invalid arguments", args)
		}
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(data []byte) (int, error) { return len(data), nil }
