package main

import (
	"strings"
	"testing"
)

func TestReadPasswordLinePreservesSpacesAndRemovesLineEnding(t *testing.T) {
	password, err := readPasswordLine(strings.NewReader("  password with spaces  \r\n"))
	if err != nil {
		t.Fatalf("readPasswordLine() error = %v", err)
	}
	if password != "  password with spaces  " {
		t.Fatalf("password = %q, want spaces preserved", password)
	}
}

func TestReadPasswordLineRejectsMultipleLinesAndOversize(t *testing.T) {
	for name, input := range map[string]string{
		"multiple": "first\nsecond\n",
		"oversize": strings.Repeat("x", 1025),
		"empty":    "\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readPasswordLine(strings.NewReader(input)); err == nil {
				t.Fatal("readPasswordLine() error = nil, want error")
			}
		})
	}
}

func TestAdminRunRequiresExplicitCommandArguments(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"unknown"},
		{"bootstrap-identity"},
		{"bootstrap-identity", "--email", "owner@example.test"},
		{"disable-account"},
		{"disable-account", "--user-id"},
		{"governance-apply"},
		{"governance-disable", "--processor", "mineru"},
	} {
		if err := run(args, strings.NewReader("password\n"), &strings.Builder{}); err == nil {
			t.Fatalf("run(%v) error = nil, want usage error", args)
		}
	}
}

func TestReadGovernanceManifestIsStrictAndBounded(t *testing.T) {
	valid := `{"processor":"mineru","endpointId":"default","modelApiVersion":"v1",` +
		`"allowedPurposes":["parse"],"allowedDataTypes":["application/pdf"],` +
		`"region":"global","retentionPolicy":"none","deletionContract":"delete",` +
		`"trainingUse":"disabled"}`
	manifest, err := readGovernanceManifest(strings.NewReader(valid))
	if err != nil || manifest.Processor != "mineru" {
		t.Fatalf("manifest = %#v, err=%v", manifest, err)
	}
	for name, input := range map[string]string{
		"unknown":      strings.TrimSuffix(valid, "}") + `,"apiKey":"secret"}`,
		"duplicate":    strings.TrimSuffix(valid, "}") + `,"processor":"other"}`,
		"case variant": strings.Replace(valid, `"processor"`, `"Processor"`, 1),
		"trailing":     valid + `{}`,
		"empty":        "",
		"oversize":     valid + strings.Repeat(" ", 64<<10),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readGovernanceManifest(strings.NewReader(input)); err == nil {
				t.Fatal("readGovernanceManifest() error = nil")
			}
		})
	}
}
