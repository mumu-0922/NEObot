package skillsupply

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestValidateArchiveDeterministicInstructionPackage(t *testing.T) {
	files := []packageFile{
		{path: "SKILL.md", data: []byte(validSkillMarkdown("demo-skill"))},
		{path: "references/guide.txt", data: []byte("bounded reference\n")},
	}
	first := mustTestArchive(t, files, zip.Store, time.Date(2020, 1, 2, 3, 4, 0, 0, time.UTC))
	second := mustTestArchive(t, []packageFile{files[1], files[0]}, zip.Deflate,
		time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC))

	validatedOne, err := ValidateArchive(ArchiveSource{
		Type: SourceZIP, Ref: "zip:first", Data: first,
	})
	if err != nil {
		t.Fatal(err)
	}
	validatedTwo, err := ValidateArchive(ArchiveSource{
		Type: SourceZIP, Ref: "zip:second", Data: second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if validatedOne.SourceArtifactSHA256 == validatedTwo.SourceArtifactSHA256 {
		t.Fatal("source artifact identity ignored ZIP representation")
	}
	if validatedOne.Package.PackageFingerprint != validatedTwo.Package.PackageFingerprint ||
		validatedOne.Package.SBOMFingerprint != validatedTwo.Package.SBOMFingerprint ||
		!bytes.Equal(validatedOne.CanonicalArchive, validatedTwo.CanonicalArchive) ||
		!bytes.Equal(validatedOne.SBOM, validatedTwo.SBOM) {
		t.Fatalf("deterministic replay drifted:\n%#v\n%#v", validatedOne.Package, validatedTwo.Package)
	}
	if validatedOne.Package.HasRuntime || validatedOne.Package.RuntimeBundleFingerprint != "" {
		t.Fatal("instruction-only package gained runtime authority")
	}
	if len(validatedOne.Package.AllowedTools) != 2 ||
		validatedOne.Package.AllowedTools[0] != "Read" || validatedOne.Package.AllowedTools[1] != "Search" {
		t.Fatalf("allowed tools = %#v", validatedOne.Package.AllowedTools)
	}
	var bom map[string]any
	if err := json.Unmarshal(validatedOne.SBOM, &bom); err != nil || bom["specVersion"] != "1.6" {
		t.Fatalf("SBOM = %#v, error = %v", bom, err)
	}
	for _, forbidden := range []string{"timestamp", "serialNumber", "/home/", "bounded reference"} {
		if bytes.Contains(validatedOne.SBOM, []byte(forbidden)) {
			t.Fatalf("SBOM contains forbidden content %q", forbidden)
		}
	}
}

func TestValidateArchiveRuntimeAndAdmissionPolicy(t *testing.T) {
	archive := mustTestArchive(t, []packageFile{
		{path: "SKILL.md", data: []byte(validSkillMarkdown("runtime-skill"))},
		{path: "neo.runtime.json", data: []byte(validRuntimeManifestJSON("runtime-skill"))},
	}, zip.Store, time.Time{})
	validated, err := ValidateArchive(ArchiveSource{
		Type: SourceOfficial, Ref: "official:runtime-skill@1.2.3", Identifier: "runtime-skill",
		Version: "1.2.3", ExpectedName: "runtime-skill", Data: archive, ExecutableAllowed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !validated.Package.HasRuntime || !digestPattern.MatchString(validated.Package.RuntimeBundleFingerprint) {
		t.Fatalf("runtime package = %#v", validated.Package)
	}
	if !AdmissionEligible(ArchiveSource{Type: SourceOfficial, ExecutableAllowed: true}, validated) {
		t.Fatal("official synthetic read-only runtime should be eligible")
	}
	if AdmissionEligible(ArchiveSource{Type: SourceLobeHub}, validated) {
		t.Fatal("third-party runtime became eligible")
	}
}

func TestValidateArchiveRequiresAdapterParentNameMatch(t *testing.T) {
	archive := mustTestArchive(t, []packageFile{{path: "SKILL.md", data: []byte(validSkillMarkdown("declared-name"))}}, zip.Store, time.Time{})
	_, err := ValidateArchive(ArchiveSource{Type: SourceLobeHub,
		Ref: "lobehub:expected-name@1.2.3", Version: "1.2.3",
		ExpectedName: "expected-name", Data: archive})
	if !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("error = %v, want ErrManifestInvalid", err)
	}
}

func TestValidateArchiveRejectsUnsafeEntries(t *testing.T) {
	tests := []struct {
		name    string
		entries []testZipEntry
	}{
		{name: "traversal", entries: append(skillEntries("safe-skill"), testZipEntry{name: "../escape", body: "x"})},
		{name: "absolute", entries: append(skillEntries("safe-skill"), testZipEntry{name: "/escape", body: "x"})},
		{name: "windows", entries: append(skillEntries("safe-skill"), testZipEntry{name: `C:\escape`, body: "x"})},
		{name: "backslash", entries: append(skillEntries("safe-skill"), testZipEntry{name: `assets\x`, body: "x"})},
		{name: "duplicate", entries: append(skillEntries("safe-skill"), testZipEntry{name: "SKILL.md", body: validSkillMarkdown("safe-skill")})},
		{name: "case collision", entries: append(skillEntries("safe-skill"), testZipEntry{name: "skill.md", body: "x"})},
		{name: "unicode collision", entries: append(skillEntries("safe-skill"),
			testZipEntry{name: "assets/é.txt", body: "one"},
			testZipEntry{name: "assets/e\u0301.txt", body: "two"})},
		{name: "symlink", entries: append(skillEntries("safe-skill"), testZipEntry{name: "link", body: "../escape", mode: os.ModeSymlink | 0o777})},
		{name: "fifo", entries: append(skillEntries("safe-skill"), testZipEntry{name: "pipe", body: "x", mode: os.ModeNamedPipe | 0o600})},
		{name: "ambiguous roots", entries: append(skillEntries("safe-skill"), testZipEntry{name: "other/SKILL.md", body: validSkillMarkdown("other")})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ValidateArchive(ArchiveSource{Type: SourceZIP, Ref: "zip:test", Data: mustRawTestArchive(t, test.entries)})
			if !errors.Is(err, ErrArchiveInvalid) {
				t.Fatalf("error = %v, want ErrArchiveInvalid", err)
			}
		})
	}
}

func TestValidateArchiveRejectsMalformedMetadataAndRuntime(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		runtime  string
	}{
		{name: "duplicate yaml", markdown: "---\nname: bad\nname: bad\ndescription: duplicate\n---\n"},
		{name: "typed yaml", markdown: "---\nname: bad\ndescription: true\n---\n"},
		{name: "duplicate json", markdown: validSkillMarkdown("bad"), runtime: strings.Replace(validRuntimeManifestJSON("bad"), `"schemaVersion":`, `"schemaVersion":"neo.skill-runtime/v1","schemaVersion":`, 1)},
		{name: "unknown json", markdown: validSkillMarkdown("bad"), runtime: strings.Replace(validRuntimeManifestJSON("bad"), `"package":`, `"unknown":true,"package":`, 1)},
		{name: "trailing json", markdown: validSkillMarkdown("bad"), runtime: validRuntimeManifestJSON("bad") + `{}`},
		{name: "floating image", markdown: validSkillMarkdown("bad"), runtime: strings.Replace(validRuntimeManifestJSON("bad"), "@sha256:"+strings.Repeat("1", 64), ":latest", 1)},
		{name: "floating dependency", markdown: validSkillMarkdown("bad"), runtime: strings.Replace(validRuntimeManifestJSON("bad"), `"dependencies":[]`, `"dependencies":[{"name":"lib","digest":"latest"}]`, 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := []packageFile{{path: "SKILL.md", data: []byte(test.markdown)}}
			if test.runtime != "" {
				files = append(files, packageFile{path: "neo.runtime.json", data: []byte(test.runtime)})
			}
			_, err := ValidateArchive(ArchiveSource{Type: SourceZIP, Ref: "zip:test", Data: mustTestArchive(t, files, zip.Store, time.Time{})})
			if !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("error = %v, want ErrManifestInvalid", err)
			}
		})
	}
}

func TestParseSkillMarkdownAcceptsOpaqueOpenClawMetadata(t *testing.T) {
	metadata, err := parseSkillMarkdown([]packageFile{{
		path: "SKILL.md",
		data: []byte(openClawSkillMarkdown("summarize")),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Name != "summarize" || len(metadata.Metadata) != 0 {
		t.Fatalf("metadata = %#v", metadata)
	}

	archive := mustTestArchive(t, []packageFile{{
		path: "SKILL.md",
		data: []byte(openClawSkillMarkdown("summarize")),
	}}, zip.Store, time.Time{})
	validated, err := ValidateArchive(ArchiveSource{
		Type: SourceLobeHub, Ref: "lobehub:openclaw-openclaw-summarize@1.0.3",
		Identifier: "openclaw-openclaw-summarize", Version: "1.0.3",
		ExpectedName: "summarize", Data: archive,
	})
	if err != nil || validated.Package.Name != "summarize" || validated.Package.Version != "1.0.3" {
		t.Fatalf("validated = %#v, error = %v", validated.Package, err)
	}
}

func TestParseSkillMarkdownProjectsOnlyTopLevelStringMetadata(t *testing.T) {
	metadata, err := parseSkillMarkdown([]packageFile{{
		path: "SKILL.md",
		data: []byte("---\nname: projected\ndescription: Projection fixture.\nmetadata:\n  version: \"2.3.4\"\n  community:\n    install:\n      - kind: brew\n        formula: ignored\n---\n"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(metadata.Metadata) != 1 || metadata.Metadata["version"] != "2.3.4" {
		t.Fatalf("projected metadata = %#v", metadata.Metadata)
	}
}

func TestParseSkillMarkdownRejectsUnsafeOpaqueMetadata(t *testing.T) {
	entries := make([]string, maxSkillMetadataEntries+1)
	for index := range entries {
		entries[index] = fmt.Sprintf("  key-%02d: value", index)
	}
	deep := "value"
	for index := 0; index < maxSkillMetadataOpaqueDepth+1; index++ {
		deep = "[" + deep + "]"
	}
	tests := []struct {
		name     string
		metadata string
	}{
		{name: "non mapping", metadata: "  - value"},
		{name: "duplicate", metadata: "  version: one\n  version: two"},
		{name: "typed top level scalar", metadata: "  version: 123"},
		{name: "alias", metadata: "  first: &details\n    install: []\n  second: *details"},
		{name: "too many entries", metadata: strings.Join(entries, "\n")},
		{name: "too deep", metadata: "  community: " + deep},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseSkillMarkdown([]packageFile{{
				path: "SKILL.md",
				data: []byte("---\nname: invalid\ndescription: Invalid metadata fixture.\nmetadata:\n" + test.metadata + "\n---\n"),
			}})
			if !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("error = %v, want ErrManifestInvalid", err)
			}
		})
	}
}

func TestValidateArchiveRejectsCompressionBomb(t *testing.T) {
	entries := skillEntries("safe-skill")
	entries = append(entries, testZipEntry{name: "payload.bin", body: strings.Repeat("A", 1<<20), method: zip.Deflate})
	_, err := ValidateArchive(ArchiveSource{Type: SourceZIP, Ref: "zip:bomb", Data: mustRawTestArchive(t, entries)})
	if !errors.Is(err, ErrArchiveInvalid) {
		t.Fatalf("error = %v, want ErrArchiveInvalid", err)
	}
}

func validSkillMarkdown(name string) string {
	return "---\nname: " + name + "\ndescription: Deterministic no-execute Skill fixture.\nlicense: MIT\nmetadata:\n  version: \"1.2.3\"\nallowed-tools: Read Search Read\n---\n\n# Fixture\n"
}

func openClawSkillMarkdown(name string) string {
	return "---\nname: " + name + `
description: Summarize URLs or files with the summarize CLI.
homepage: https://summarize.sh
metadata:
  {
    "openclaw":
      {
        "emoji": "🧾",
        "requires": { "bins": ["summarize"] },
        "install":
          [
            {
              "id": "brew",
              "kind": "brew",
              "formula": "steipete/tap/summarize",
              "bins": ["summarize"],
              "label": "Install summarize (brew)",
            },
          ],
      },
  }
---

# Summarize
`
}

func validRuntimeManifestJSON(name string) string {
	return `{"schemaVersion":"neo.skill-runtime/v1","package":{"name":"` + name + `","version":"1.2.3"},` +
		`"runtime":{"kind":"rootless_oci","image":"registry.neo.invalid/runtime@sha256:` + strings.Repeat("1", 64) + `","platform":"linux/amd64","user":{"uid":10001,"gid":10001}},` +
		`"entrypoints":[{"name":"inspect","argv":["/opt/inspect"],"workingDirectory":"/workspace"}],"dependencies":[],` +
		`"capabilityRequests":[{"capability":"workspace.read","actions":["list","read"],"reason":"Read only fixture."}],` +
		`"egressRequests":[],"secretSlots":[],"resources":{"cpuMillis":500,"memoryMiB":128,"pids":32,"wallSeconds":30},` +
		`"limits":{"maxPackageFiles":16,"maxPackageBytes":1048576,"maxExpandedBytes":1048576,"maxStdoutBytes":65536,"maxStderrBytes":65536,"maxArtifactBytes":1}}`
}

func skillEntries(name string) []testZipEntry {
	return []testZipEntry{{name: "SKILL.md", body: validSkillMarkdown(name)}}
}

func mustTestArchive(t *testing.T, files []packageFile, method uint16, modified time.Time) []byte {
	t.Helper()
	entries := make([]testZipEntry, 0, len(files))
	for _, file := range files {
		entries = append(entries, testZipEntry{name: file.path, body: string(file.data), method: method, modified: modified})
	}
	return mustRawTestArchive(t, entries)
}

type testZipEntry struct {
	name     string
	body     string
	method   uint16
	mode     os.FileMode
	modified time.Time
}

func mustRawTestArchive(t *testing.T, entries []testZipEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, item := range entries {
		header := &zip.FileHeader{Name: item.name, Method: item.method, Modified: item.modified}
		if header.Method == 0 {
			header.Method = zip.Store
		}
		mode := item.mode
		if mode == 0 {
			mode = 0o644
		}
		header.SetMode(mode)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(item.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
