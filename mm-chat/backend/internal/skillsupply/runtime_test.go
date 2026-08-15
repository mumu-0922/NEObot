package skillsupply

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrepareRuntimeSkillsMaterializesInstalledPackageWithoutExecuting(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "must-not-run")
	archive := mustTestArchive(t, []packageFile{
		{path: "SKILL.md", data: []byte(validSkillMarkdown("local-demo"))},
		{path: "references/guide.md", data: []byte("trusted fixture guide\n")},
		{path: "scripts/run.sh", data: []byte("#!/bin/sh\ntouch " + marker + "\n")},
	}, 0, time.Time{})
	repository := newMemoryRepository()
	objects := newMemoryObjectStore()
	service := NewService(WithRepository(repository), WithObjectStore(objects),
		WithAdministratorUserID(testSkillAdmin))
	service.newID = sequenceIDs("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	candidate, err := service.IngestZIP(context.Background(), testSkillAdmin, archive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ReviewCandidate(context.Background(), testSkillAdmin, candidate.ID, ReviewInput{
		Status: StatusAdmitted, ExpectedRevision: 1,
		PackageFingerprint: candidate.Package.PackageFingerprint,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Install(context.Background(), testSkillUser, candidate.ID,
		candidate.Package.PackageFingerprint); err != nil {
		t.Fatal(err)
	}

	runtimeRoot := filepath.Join(t.TempDir(), "runtime")
	skills, err := service.PrepareRuntimeSkills(context.Background(), testSkillUser, runtimeRoot)
	if err != nil || len(skills) != 1 || skills[0].Name != "local-demo" ||
		!filepath.IsAbs(skills[0].RootPath) {
		t.Fatalf("skills=%#v error=%v", skills, err)
	}
	body, err := ReadRuntimeSkillFile(skills[0], "references/guide.md")
	if err != nil || string(body) != "trusted fixture guide\n" {
		t.Fatalf("body=%q error=%v", body, err)
	}
	if info, err := os.Stat(filepath.Join(skills[0].RootPath, "scripts/run.sh")); err != nil ||
		info.Mode().Perm() != 0o700 {
		t.Fatalf("script mode=%v error=%v", info, err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("materialization executed package script: %v", err)
	}

	// The immutable marker makes a second prepare idempotent.
	again, err := service.PrepareRuntimeSkills(context.Background(), testSkillUser, runtimeRoot)
	if err != nil || len(again) != 1 || again[0].RootPath != skills[0].RootPath {
		t.Fatalf("second prepare=%#v error=%v", again, err)
	}
}

func TestReadRuntimeSkillFileRejectsTraversalMissingAndSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("body"), 0o600); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := runtimeTreeFingerprint(root, []string{"SKILL.md"})
	if err != nil {
		t.Fatal(err)
	}
	skill := RuntimeSkill{
		RootPath: root, Files: []string{"SKILL.md"}, PackageFingerprint: fingerprint,
	}
	for _, path := range []string{"../SKILL.md", "/etc/passwd", "missing.md"} {
		if _, err := ReadRuntimeSkillFile(skill, path); !errors.Is(err, ErrRuntimeFileNotFound) {
			t.Fatalf("path %q error=%v", path, err)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.md")); err != nil {
		t.Fatal(err)
	}
	skill.Files = []string{"SKILL.md", "link.md"}
	if _, err := ReadRuntimeSkillFile(skill, "link.md"); !errors.Is(err, ErrPackageChanged) {
		t.Fatalf("symlink error=%v", err)
	}
}

func TestPrepareRuntimeSkillsRejectsPackageObjectDrift(t *testing.T) {
	repository := newMemoryRepository()
	objects := newMemoryObjectStore()
	service := NewService(WithRepository(repository), WithObjectStore(objects),
		WithAdministratorUserID(testSkillAdmin))
	service.newID = sequenceIDs("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	archive := mustTestArchive(t, []packageFile{{
		path: "SKILL.md", data: []byte(validSkillMarkdown("drift-demo")),
	}}, 0, time.Time{})
	candidate, err := service.IngestZIP(context.Background(), testSkillAdmin, archive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ReviewCandidate(context.Background(), testSkillAdmin, candidate.ID, ReviewInput{
		Status: StatusAdmitted, ExpectedRevision: 1,
		PackageFingerprint: candidate.Package.PackageFingerprint,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Install(context.Background(), testSkillUser, candidate.ID,
		candidate.Package.PackageFingerprint); err != nil {
		t.Fatal(err)
	}
	objects.objects[candidate.Package.PackageObjectKey] = []byte("not a zip")
	if _, err := service.PrepareRuntimeSkills(context.Background(), testSkillUser,
		filepath.Join(t.TempDir(), "runtime")); !errors.Is(err, ErrPackageChanged) {
		t.Fatalf("drift error=%v", err)
	}
}

func TestRuntimeSkillReadRejectsMaterializedContentDrift(t *testing.T) {
	root := t.TempDir()
	body := []byte("canonical")
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := runtimeTreeFingerprint(root, []string{"SKILL.md"})
	if err != nil {
		t.Fatal(err)
	}
	skill := RuntimeSkill{
		RootPath: root, Files: []string{"SKILL.md"}, PackageFingerprint: fingerprint,
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRuntimeSkillFile(skill, "SKILL.md"); !errors.Is(err, ErrPackageChanged) {
		t.Fatalf("content drift error=%v", err)
	}
}

func TestRuntimeSkillReadRejectsExtraTreeEntries(t *testing.T) {
	root := t.TempDir()
	body := []byte("canonical")
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := runtimeTreeFingerprint(root, []string{"SKILL.md"})
	if err != nil {
		t.Fatal(err)
	}
	skill := RuntimeSkill{
		RootPath: root, Files: []string{"SKILL.md"}, PackageFingerprint: fingerprint,
	}
	if err := os.WriteFile(filepath.Join(root, "injected.sh"), []byte("malicious"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRuntimeSkillFile(skill, "SKILL.md"); !errors.Is(err, ErrPackageChanged) {
		t.Fatalf("extra file drift error=%v", err)
	}
}

func TestRuntimeSkillReadRejectsOversizedView(t *testing.T) {
	root := t.TempDir()
	body := make([]byte, maxRuntimeViewBytes+1)
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := runtimeTreeFingerprint(root, []string{"SKILL.md"})
	if err != nil {
		t.Fatal(err)
	}
	skill := RuntimeSkill{
		RootPath: root, Files: []string{"SKILL.md"}, PackageFingerprint: fingerprint,
	}
	if _, err := ReadRuntimeSkillFile(skill, "SKILL.md"); !errors.Is(err, ErrRuntimeFileNotFound) {
		t.Fatalf("oversized view error=%v", err)
	}
}
