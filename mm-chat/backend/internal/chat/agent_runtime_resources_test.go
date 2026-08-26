package chat

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/localskills"
	"neo-chat/mm-chat/backend/internal/mcpclient"
	"neo-chat/mm-chat/backend/internal/skillsupply"
)

func TestAgentRuntimeResourceSnapshotRevisionIsStableAndStepScoped(t *testing.T) {
	memory := &memoryToolRuntime{
		Searcher:           &memoryToolTestSearcher{},
		ConversationID:     "11111111-1111-4111-8111-111111111111",
		AssistantMessageID: "22222222-2222-4222-8222-222222222222",
		Query:              "remembered value",
		forceFirstCall:     true,
	}
	snapshot := newAgentRuntimeResourceSnapshot(agentRuntimeResourceInput{Memory: memory})
	input := externalWebToolLoopInput{Resources: snapshot}

	first := snapshot.project(input, 1, false)
	repeated := snapshot.project(input, 1, false)
	second := snapshot.project(input, 2, false)
	if first.Revision == "" || first.Revision != repeated.Revision {
		t.Fatalf("unstable projection revisions: %q / %q", first.Revision, repeated.Revision)
	}
	if first.Report.RunRevision != repeated.Report.RunRevision ||
		first.Report.RunRevision == "" {
		t.Fatalf("unstable Run revisions: %#v / %#v", first.Report, repeated.Report)
	}
	if first.Revision == second.Revision {
		t.Fatal("task-step change did not change the projection revision")
	}
	if _, ok := first.Registry.lookup(SearchMemoryToolDefinition().Function.Name); !ok {
		t.Fatal("first-step Memory Tool is missing")
	}
	if _, ok := second.Registry.lookup(SearchMemoryToolDefinition().Function.Name); ok {
		t.Fatal("first-step Memory Tool remained executable in a later projection")
	}
}

func TestAgentRuntimeResourceSnapshotUsesOneRequiredSkillProjection(t *testing.T) {
	executor := newAgentRuntimeResourceTestExecutor(t)
	runtime := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{{
		Name: "fixture-skill", Version: "1.0.0", Description: "fixture workflow",
		PackageFingerprint: "sha256:" + strings.Repeat("a", 64),
	}})
	snapshot := newAgentRuntimeResourceSnapshot(agentRuntimeResourceInput{LocalSkills: runtime})
	if _, err := snapshot.prepareUserPrompt("use fixture-skill", "use fixture-skill"); err != nil {
		t.Fatal(err)
	}

	normalBefore := snapshot.project(externalWebToolLoopInput{}, 1, false)
	required := snapshot.project(externalWebToolLoopInput{}, 1, true)
	definitions := required.Registry.definitions(1)
	if len(definitions) != 1 || definitions[0].Function.Name != localSkillToolName {
		t.Fatalf("required projection definitions=%#v", definitions)
	}
	if _, ok := required.Registry.lookup(localTerminalToolName); ok {
		t.Fatal("bash leaked into the required Skill projection")
	}
	if normalBefore.Report.RunRevision != required.Report.RunRevision {
		t.Fatal("required prelude did not use the same Run snapshot")
	}

	runtime.loaded["fixture-skill"] = runtime.catalogRevision
	runtime.required = nil
	normalAfter := snapshot.project(externalWebToolLoopInput{}, 1, false)
	if normalBefore.Revision == normalAfter.Revision {
		t.Fatal("loaded Skill state did not change the Step projection revision")
	}
}

func TestAgentRuntimeResourceSnapshotRevisionTracksMCPVisibility(t *testing.T) {
	runtime := newSchedulerMCPRuntime(t, &schedulerMCPConnector{})
	runtime.setVisible(nil, true)
	snapshot := newAgentRuntimeResourceSnapshot(agentRuntimeResourceInput{MCP: runtime})
	input := externalWebToolLoopInput{Resources: snapshot}
	searchOnly := snapshot.project(input, 1, false)

	tool := runtime.run.Snapshot.Servers[0].Tools[0]
	runtime.setVisible([]mcpclient.Tool{tool}, true)
	expanded := snapshot.project(input, 2, false)
	if searchOnly.Revision == expanded.Revision {
		t.Fatal("MCP visibility expansion did not change the projection revision")
	}
	if _, ok := expanded.Registry.lookup(tool.Alias); !ok {
		t.Fatalf("expanded MCP Tool %q is missing", tool.Alias)
	}
}

func TestAgentRuntimeResourceReportCarriesActivationProvenance(t *testing.T) {
	executor := newAgentRuntimeResourceTestExecutor(t)
	skills := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{{
		Name: "fixture-skill", Version: "1.0.0",
		PackageFingerprint: "sha256:" + strings.Repeat("c", 64),
		ActivationSource:   skillsupply.RuntimeSkillActivationAgentAuto,
	}})
	mcp := newSchedulerMCPRuntime(t, &schedulerMCPConnector{})
	mcp.run.Snapshot.Servers[0].ActivationSource = "user_selected"
	mcp.setVisible(mcp.run.Snapshot.Servers[0].Tools, true)
	snapshot := newAgentRuntimeResourceSnapshot(agentRuntimeResourceInput{
		LocalSkills: skills,
		MCP:         mcp,
	})
	report := snapshot.project(externalWebToolLoopInput{}, 1, false).Report
	want := map[string]bool{
		skillsupply.RuntimeSkillActivationAgentAuto: false,
		"user_selected": false,
	}
	for _, resource := range report.Resources {
		if _, expected := want[resource.ActivationSource]; expected {
			want[resource.ActivationSource] = true
		}
	}
	for source, found := range want {
		if !found {
			t.Fatalf("activation source %q missing from report=%#v", source, report.Resources)
		}
	}
}

func TestAgentRuntimeResourceSnapshotRevisionTracksRetrievalAuthority(t *testing.T) {
	snapshot := newAgentRuntimeResourceSnapshot(agentRuntimeResourceInput{})
	first := snapshot.project(externalWebToolLoopInput{
		Knowledge: &knowledgeToolRuntime{
			SelectedCollectionIDs: []string{"11111111-1111-4111-8111-111111111111"},
		},
	}, 1, false)
	second := snapshot.project(externalWebToolLoopInput{
		Knowledge: &knowledgeToolRuntime{
			SelectedCollectionIDs: []string{"22222222-2222-4222-8222-222222222222"},
		},
	}, 1, false)
	if first.Revision == second.Revision {
		t.Fatal("Knowledge authority change did not change the projection revision")
	}
}

func TestAgentRuntimeResourceReportFailsClosedAndRedactsAuthority(t *testing.T) {
	executor := newAgentRuntimeResourceTestExecutor(t)
	secret := "sk-super-secret-registry-value"
	hostPath := filepath.Join(t.TempDir(), "private", secret)
	runtime := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{{
		Name: "fixture-skill", Version: "1.0.0", Description: secret,
		PackageFingerprint: "sha256:" + strings.Repeat("b", 64), RootPath: hostPath,
		Files: []string{"SKILL.md"},
	}})
	snapshot := newAgentRuntimeResourceSnapshot(agentRuntimeResourceInput{LocalSkills: runtime})
	registry := &chatToolRegistry{}
	webDefinition := searchWebToolDefinition()
	localDefinition := ToolDefinition{Type: "function", Function: ToolFunctionDefinition{
		Name: searchWebToolName, Description: secret,
	}}
	registry.register(retrievalToolRegistration(
		webDefinition, chatToolBackendWeb, chatToolRiskExternal, false,
	))
	registry.register(chatToolRegistration{
		Name: searchWebToolName, Definition: &localDefinition,
		Backend: chatToolBackendLocalSkill, RiskClass: chatToolRiskRead,
		ProjectForModel: identityChatToolResult,
	})
	report := snapshot.report(externalWebToolLoopInput{}, registry, 1, false)
	if _, ok := registry.lookup(searchWebToolName); ok {
		t.Fatal("colliding Tool remained executable")
	}
	if len(report.Diagnostics) != 1 ||
		report.Diagnostics[0].Code != agentRuntimeDiagnosticToolNameCollision {
		t.Fatalf("collision diagnostics=%#v", report.Diagnostics)
	}
	degraded := 0
	for _, resource := range report.Resources {
		if resource.Status == agentRuntimeResourceStatusDegraded {
			degraded++
		}
	}
	if degraded < 2 {
		t.Fatalf("colliding resources were not degraded: %#v", report.Resources)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{secret, hostPath, runtime.config().RuntimeRoot, "arguments", "results"} {
		if forbidden != "" && strings.Contains(string(encoded), forbidden) {
			t.Fatalf("runtime report leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestAgentRuntimeResourceReportCarriesUnavailableDiagnostic(t *testing.T) {
	snapshot := newAgentRuntimeResourceSnapshot(agentRuntimeResourceInput{
		LocalSkills: &localSkillToolRuntime{},
	})
	report := snapshot.project(externalWebToolLoopInput{}, 1, false).Report
	if len(report.Resources) != 1 ||
		report.Resources[0].Status != agentRuntimeResourceStatusUnavailable ||
		len(report.Diagnostics) != 1 ||
		report.Diagnostics[0].Code != agentRuntimeDiagnosticResourceUnavailable {
		t.Fatalf("unavailable report=%#v", report)
	}
}

func newAgentRuntimeResourceTestExecutor(t *testing.T) *localskills.Executor {
	t.Helper()
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(t.TempDir(), "skills"),
		WorkspaceRoot: t.TempDir(), ShellPath: "/bin/sh",
		ApprovalMode: localskills.ApprovalSmart, CallTimeout: time.Second,
		RunTimeout: 5 * time.Second, MaxOutput: 4096, MaxCalls: 8,
		MaxRounds: 8, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = executor.Close() })
	return executor
}
