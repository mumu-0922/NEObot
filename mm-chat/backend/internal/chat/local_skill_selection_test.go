package chat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/localskills"
	"neo-chat/mm-chat/backend/internal/skillsupply"
)

func TestLocalSkillSlashInvocationLoadsDeterministicallyAndDoesNotReload(t *testing.T) {
	runtimeRoot := t.TempDir()
	skillRoot := filepath.Join(runtimeRoot, "fixture")
	if err := os.Mkdir(skillRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte("---\nname: fixture-skill\ndescription: fixture\n---\nFollow this exact guidance.\n")
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: runtimeRoot, WorkspaceRoot: t.TempDir(),
		ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: time.Second, RunTimeout: 5 * time.Second, MaxOutput: 4096,
		MaxCalls: 4, MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{{
		Name: "fixture-skill", Version: "1.0.0", Description: "Runs the fixture",
		RootPath: skillRoot, Files: []string{"SKILL.md"},
		PackageFingerprint: testRuntimeSkillFingerprint(map[string][]byte{
			"SKILL.md": body,
		}),
	}})
	prompt, err := runtime.prepareUserPrompt("/fixture-skill do it", "/fixture-skill do it")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.requiresSkillLoad() || !strings.Contains(prompt, "Follow this exact guidance") ||
		!strings.Contains(prompt, "user_authorized_skill_instructions") {
		t.Fatalf("slash prompt=%q required=%v", prompt, runtime.requiresSkillLoad())
	}
	result, err := runtime.execute(context.Background(), make(chan ProviderEvent, 4), ProviderToolCall{
		ID: "duplicate", Name: localSkillToolName, Arguments: `{"name":"fixture-skill"}`,
	}, 1, 1)
	if err != nil || result.IsError || !strings.Contains(result.Content, `"alreadyLoaded":true`) ||
		strings.Contains(result.Content, "Follow this exact guidance") {
		t.Fatalf("duplicate load=%#v error=%v", result, err)
	}
}

func TestLocalSkillDescriptionMatchRequiresExactSkillBeforeTaskRound(t *testing.T) {
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(t.TempDir(), "skills"),
		WorkspaceRoot: t.TempDir(), ShellPath: "/bin/sh",
		ApprovalMode: localskills.ApprovalSmart, CallTimeout: time.Second,
		RunTimeout: 5 * time.Second, MaxOutput: 4096, MaxCalls: 4,
		MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{{
		Name: "spreadsheet", Description: "合并 CSV 表格并生成 Excel 工作簿",
		Files: []string{"SKILL.md"},
	}})
	if _, err := runtime.prepareUserPrompt(
		"把 CSV 表格合并成 Excel 工作簿", "把 CSV 表格合并成 Excel 工作簿",
	); err != nil {
		t.Fatal(err)
	}
	definitions := runtime.requiredDefinition()
	if !runtime.requiresSkillLoad() || len(definitions) != 1 ||
		definitions[0].Function.Name != localSkillToolName {
		t.Fatalf("required definitions=%#v", definitions)
	}
	nameSchema := definitions[0].Function.Parameters["properties"].(map[string]any)["name"].(map[string]any)
	enum, ok := nameSchema["enum"].([]string)
	if !ok || len(enum) != 1 || enum[0] != "spreadsheet" {
		t.Fatalf("required Skill enum=%#v", nameSchema)
	}
}

func TestLocalSkillLatinDescriptionMatchSelectsOnlyTheUniqueSkill(t *testing.T) {
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(t.TempDir(), "skills"),
		WorkspaceRoot: t.TempDir(), ShellPath: "/bin/sh",
		ApprovalMode: localskills.ApprovalSmart, CallTimeout: time.Second,
		RunTimeout: 5 * time.Second, MaxOutput: 4096, MaxCalls: 4,
		MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{
		{Name: "spreadsheet", Description: "Merge CSV tables into Excel workbooks"},
		{Name: "translator", Description: "Translate prose between languages"},
	})
	if _, err := runtime.prepareUserPrompt(
		"Merge these CSV tables into an Excel workbook", "Merge these CSV tables into an Excel workbook",
	); err != nil {
		t.Fatal(err)
	}
	if required := runtime.requiredSkillName(); required != "spreadsheet" {
		t.Fatalf("required Skill=%q", required)
	}
}

func TestLocalSkillRequiredPreludeRejectsUnadvertisedTerminalBeforeExecution(t *testing.T) {
	workspace := t.TempDir()
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(t.TempDir(), "skills"),
		WorkspaceRoot: workspace, ShellPath: "/bin/sh",
		ApprovalMode: localskills.ApprovalSmart, CallTimeout: time.Second,
		RunTimeout: 5 * time.Second, MaxOutput: 4096, MaxCalls: 4,
		MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{{
		Name: "fixture-skill", Description: "Runs a fixture",
		Files: []string{"SKILL.md"},
	}})
	if _, err := runtime.prepareUserPrompt("use fixture-skill", "use fixture-skill"); err != nil {
		t.Fatal(err)
	}
	results, _, err := executeRequiredLocalSkillBatch(
		context.Background(), make(chan ProviderEvent, 4), runtime,
		[]ProviderToolCall{{
			ID: "forged-terminal", Name: localTerminalToolName,
			Arguments: `{"command":"printf ran > marker","skill":null,"workingDir":null,"timeoutSeconds":1}`,
		}},
		1,
	)
	result := results[0]
	if err != nil || !result.IsError ||
		!strings.Contains(result.Content, "skill_required_before_action") {
		t.Fatalf("required prelude result=%#v error=%v", result, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "marker")); !os.IsNotExist(err) {
		t.Fatalf("unadvertised terminal executed: %v", err)
	}
}

func TestLocalSkillRequiredPreludeRejectsDifferentInstalledSkill(t *testing.T) {
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(t.TempDir(), "skills"),
		WorkspaceRoot: t.TempDir(), ShellPath: "/bin/sh",
		ApprovalMode: localskills.ApprovalSmart, CallTimeout: time.Second,
		RunTimeout: 5 * time.Second, MaxOutput: 4096, MaxCalls: 4,
		MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{
		{Name: "alpha-skill", Description: "Run alpha checks"},
		{Name: "beta-skill", Description: "Run beta checks"},
	})
	if _, err := runtime.prepareUserPrompt("use alpha-skill", "use alpha-skill"); err != nil {
		t.Fatal(err)
	}
	results, _, err := executeRequiredLocalSkillBatch(
		context.Background(), make(chan ProviderEvent, 4), runtime,
		[]ProviderToolCall{{
			ID: "wrong-skill", Name: localSkillToolName,
			Arguments: `{"name":"beta-skill"}`,
		}},
		1,
	)
	if err != nil || !results[0].IsError ||
		!strings.Contains(results[0].Content, "skill_required_before_action") {
		t.Fatalf("wrong Skill result=%#v error=%v", results[0], err)
	}
	if runtime.loaded["beta-skill"] != "" || runtime.requiredSkillName() != "alpha-skill" {
		t.Fatalf("wrong Skill changed prelude state: loaded=%#v", runtime.loaded)
	}
}

func TestLocalSkillCatalogRevisionChangesWithReplacement(t *testing.T) {
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(t.TempDir(), "skills"),
		WorkspaceRoot: t.TempDir(), ShellPath: "/bin/sh",
		ApprovalMode: localskills.ApprovalSmart, CallTimeout: time.Second,
		RunTimeout: 5 * time.Second, MaxOutput: 4096, MaxCalls: 4,
		MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	first := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{{
		Name: "fixture", Version: "1.0.0", Description: "first",
	}})
	second := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{{
		Name: "fixture", Version: "2.0.0", Description: "second",
	}})
	if first.catalogRevision == second.catalogRevision ||
		!strings.Contains(first.promptInstruction(), `"replacement":true`) ||
		!strings.Contains(second.promptInstruction(), second.catalogRevision) {
		t.Fatalf("catalog revisions=%q/%q", first.catalogRevision, second.catalogRevision)
	}
}

func TestLocalSkillPromptDescribesOnlyConfiguredWorkspaceHostAlias(t *testing.T) {
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(t.TempDir(), "skills"),
		WorkspaceRoot:     t.TempDir(),
		WorkspaceHostRoot: "/home/mumu/projects/Oncall_Agent",
		ShellPath:         "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: time.Second, RunTimeout: 5 * time.Second, MaxOutput: 4096,
		MaxCalls: 4, MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt := newLocalSkillToolRuntime(executor, nil).promptInstruction()
	for _, required := range []string{
		"authorized_workspace_alias",
		`"hostRoot":"/home/mumu/projects/Oncall_Agent"`,
		"WSL UNC path",
		"use $PWD or relative paths",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("prompt omitted %q: %s", required, prompt)
		}
	}
	if strings.Contains(prompt, "/home/mumu/projects/private-project") {
		t.Fatalf("prompt widened workspace: %s", prompt)
	}
}
