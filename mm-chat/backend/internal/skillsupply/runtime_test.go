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

func TestPrepareConversationRuntimeSkillsUsesRunOnlyActivationWithoutPersisting(t *testing.T) {
	const conversationID = "33333333-3333-4333-8333-333333333333"
	repository := newMemoryRepository()
	repository.conversations[conversationID] = testSkillUser
	repository.newID = sequenceIDs(
		"44444444-4444-4444-8444-444444444444",
		"55555555-5555-4555-8555-555555555555",
	)
	objects := newMemoryObjectStore()
	service := NewService(WithRepository(repository), WithObjectStore(objects),
		WithAdministratorUserID(testSkillAdmin))
	service.newID = sequenceIDs(
		"66666666-6666-4666-8666-666666666666",
		"77777777-7777-4777-8777-777777777777",
	)

	install := func(name, description string) Installation {
		t.Helper()
		markdown := "---\nname: " + name + "\ndescription: " + description +
			"\nlicense: MIT\nmetadata:\n  version: \"1.0.0\"\nallowed-tools: Read\n---\n\n# Fixture\n"
		archive := mustTestArchive(t, []packageFile{{path: "SKILL.md", data: []byte(markdown)}}, 0, time.Time{})
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
		installation, err := service.Install(context.Background(), testSkillUser, candidate.ID,
			candidate.Package.PackageFingerprint)
		if err != nil {
			t.Fatal(err)
		}
		return installation
	}
	xlsx := install("office-xlsx", "Create and validate Excel xlsx spreadsheets.")
	text := install("plain-text", "Edit plain text notes.")

	runtimeRoot := filepath.Join(t.TempDir(), "runtime")
	activated, err := service.PrepareConversationRuntimeSkills(
		context.Background(), testSkillUser, conversationID, runtimeRoot,
		"请创建 Excel xlsx 表格", true,
	)
	if err != nil || len(activated) != 1 || activated[0].InstallationID != xlsx.ID ||
		activated[0].ActivationSource != RuntimeSkillActivationAgentAuto {
		t.Fatalf("auto activated=%#v error=%v", activated, err)
	}
	selection, err := service.GetConversationSelection(context.Background(), testSkillUser, conversationID)
	if err != nil || selection.Revision != 0 || len(selection.Skills) != 0 {
		t.Fatalf("run-only activation persisted selection=%#v error=%v", selection, err)
	}
	selected, err := service.ReplaceConversationSelection(
		context.Background(), testSkillUser, conversationID, 0, []string{text.ID},
	)
	if err != nil || selected.Revision != 1 {
		t.Fatalf("replace selection=%#v error=%v", selected, err)
	}
	pinned, err := service.PrepareConversationRuntimeSkills(
		context.Background(), testSkillUser, conversationID, runtimeRoot, "unrelated", true,
	)
	if err != nil || len(pinned) != 1 || pinned[0].InstallationID != text.ID ||
		pinned[0].ActivationSource != RuntimeSkillActivationUserSelected {
		t.Fatalf("pinned runtime=%#v error=%v", pinned, err)
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
