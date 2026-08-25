package chat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

type agentRuntimeResourceKind string

const (
	agentRuntimeResourceKindToolSet      agentRuntimeResourceKind = "tool_set"
	agentRuntimeResourceKindSkillCatalog agentRuntimeResourceKind = "skill_catalog"
	agentRuntimeResourceKindSkillPackage agentRuntimeResourceKind = "skill_package"
)

type agentRuntimeResourceSource string

const (
	agentRuntimeResourceSourceBuiltin    agentRuntimeResourceSource = "builtin"
	agentRuntimeResourceSourceRetrieval  agentRuntimeResourceSource = "retrieval"
	agentRuntimeResourceSourceMCP        agentRuntimeResourceSource = "mcp"
	agentRuntimeResourceSourceLocalSkill agentRuntimeResourceSource = "local_skill"
)

type agentRuntimeResourceScope string

const (
	agentRuntimeResourceScopeServer agentRuntimeResourceScope = "server"
	agentRuntimeResourceScopeUser   agentRuntimeResourceScope = "user"
	agentRuntimeResourceScopeRun    agentRuntimeResourceScope = "run"
)

type agentRuntimeResourceStatus string

const (
	agentRuntimeResourceStatusEnabled     agentRuntimeResourceStatus = "enabled"
	agentRuntimeResourceStatusHidden      agentRuntimeResourceStatus = "hidden"
	agentRuntimeResourceStatusDegraded    agentRuntimeResourceStatus = "degraded"
	agentRuntimeResourceStatusUnavailable agentRuntimeResourceStatus = "unavailable"
)

const (
	agentRuntimeDiagnosticResourceUnavailable = "resource_unavailable"
	agentRuntimeDiagnosticToolNameCollision   = "tool_name_collision"
)

type agentRuntimeResourceInput struct {
	MCP         *mcpToolRuntime
	LocalSkills *localSkillToolRuntime
	Goals       *chatAgentGoalToolRuntime
	Knowledge   *knowledgeToolRuntime
	Memory      *memoryToolRuntime
	ExternalWeb bool
}

// agentRuntimeResourceSnapshot freezes the server-authorized runtime handles
// for one Run. Mutable execution counters and current MCP visibility remain in
// those handles; each model Step receives a newly immutable projection.
type agentRuntimeResourceSnapshot struct {
	runRevision string
	mcp         *mcpToolRuntime
	localSkills *localSkillToolRuntime
	goals       *chatAgentGoalToolRuntime
	knowledge   *knowledgeToolRuntime
	memory      *memoryToolRuntime
	externalWeb bool
}

type agentRuntimeToolProjection struct {
	Revision string
	Registry *chatToolRegistry
	Report   agentRuntimeResourceReport
}

func newAgentRuntimeResourceSnapshot(input agentRuntimeResourceInput) *agentRuntimeResourceSnapshot {
	snapshot := &agentRuntimeResourceSnapshot{
		mcp: input.MCP, localSkills: input.LocalSkills, goals: input.Goals,
		knowledge: input.Knowledge, memory: input.Memory, externalWeb: input.ExternalWeb,
	}
	snapshot.runRevision = agentRuntimeRevision(snapshot.runAuthority())
	return snapshot
}

func newAgentRuntimeResourceSnapshotFromToolLoopInput(
	input externalWebToolLoopInput,
) *agentRuntimeResourceSnapshot {
	return newAgentRuntimeResourceSnapshot(agentRuntimeResourceInput{
		MCP: input.MCP, LocalSkills: input.LocalSkills, Goals: input.Goals,
		Knowledge: input.Knowledge, Memory: input.Memory,
		ExternalWeb: externalWebToolEnabled(input),
	})
}

func (snapshot *agentRuntimeResourceSnapshot) bind(
	input externalWebToolLoopInput,
) externalWebToolLoopInput {
	if snapshot == nil {
		return input
	}
	input.Resources = snapshot
	input.MCP = snapshot.mcp
	input.LocalSkills = snapshot.localSkills
	if snapshot.goals != nil {
		input.Goals = snapshot.goals
	}
	if snapshot.knowledge != nil {
		input.Knowledge = snapshot.knowledge
	}
	if snapshot.memory != nil {
		input.Memory = snapshot.memory
	}
	return input
}

func (snapshot *agentRuntimeResourceSnapshot) toolLoopEnabled(
	includeKnowledge bool,
	includeExternalWeb bool,
) bool {
	return snapshot != nil && (snapshot.mcp.enabled() || snapshot.localSkills.enabled() ||
		snapshot.goals.enabled() || snapshot.memory.enabled() ||
		(includeKnowledge && snapshot.knowledge.enabled()) ||
		(includeExternalWeb && snapshot.externalWeb))
}

func (snapshot *agentRuntimeResourceSnapshot) prepareUserPrompt(
	base string,
	userText string,
) (string, error) {
	if snapshot == nil {
		return base, nil
	}
	return snapshot.localSkills.prepareUserPrompt(base, userText)
}

func (snapshot *agentRuntimeResourceSnapshot) promptInstruction() string {
	if snapshot == nil {
		return ""
	}
	return snapshot.localSkills.promptInstruction()
}

func (snapshot *agentRuntimeResourceSnapshot) project(
	input externalWebToolLoopInput,
	taskStep int,
	requiredSkillOnly bool,
) agentRuntimeToolProjection {
	if snapshot == nil {
		snapshot = newAgentRuntimeResourceSnapshotFromToolLoopInput(input)
	}
	input = snapshot.bind(input)
	var registry *chatToolRegistry
	if requiredSkillOnly {
		registry = buildRequiredLocalSkillRegistry(snapshot.localSkills)
	} else {
		registry = buildChatToolRegistry(input).forTaskStep(taskStep)
	}
	report := snapshot.report(input, registry, taskStep, requiredSkillOnly)
	return agentRuntimeToolProjection{
		Revision: report.ProjectionRevision, Registry: registry, Report: report,
	}
}

func (snapshot *agentRuntimeResourceSnapshot) report(
	input externalWebToolLoopInput,
	registry *chatToolRegistry,
	taskStep int,
	requiredSkillOnly bool,
) agentRuntimeResourceReport {
	descriptors := snapshot.resourceDescriptors(registry)
	diagnostics := snapshot.resourceDiagnostics(registry, descriptors)
	report := agentRuntimeResourceReport{
		RunRevision: snapshot.runRevision, TaskStep: taskStep,
		RequiredSkillOnly: requiredSkillOnly, Resources: descriptors,
		Diagnostics: diagnostics,
	}
	report.ProjectionRevision = agentRuntimeRevision(struct {
		RunRevision       string                           `json:"runRevision"`
		StepAuthority     any                              `json:"stepAuthority"`
		TaskStep          int                              `json:"taskStep"`
		RequiredSkillOnly bool                             `json:"requiredSkillOnly"`
		LoadedSkills      [][]string                       `json:"loadedSkills"`
		Definitions       []ToolDefinition                 `json:"definitions"`
		Resources         []agentRuntimeResourceDescriptor `json:"resources"`
		Diagnostics       []agentRuntimeResourceDiagnostic `json:"diagnostics"`
	}{
		RunRevision: snapshot.runRevision, TaskStep: taskStep,
		StepAuthority:     agentRuntimeStepAuthority(input),
		RequiredSkillOnly: requiredSkillOnly,
		LoadedSkills:      snapshot.loadedSkillState(),
		Definitions:       registry.definitions(taskStep), Resources: descriptors,
		Diagnostics: diagnostics,
	})
	return report
}

func (snapshot *agentRuntimeResourceSnapshot) resourceDescriptors(
	registry *chatToolRegistry,
) []agentRuntimeResourceDescriptor {
	byID := make(map[string]*agentRuntimeResourceDescriptor)
	if snapshot != nil && snapshot.localSkills != nil && !snapshot.localSkills.enabled() {
		id := agentRuntimeStableID("skill", "runtime")
		byID[id] = &agentRuntimeResourceDescriptor{
			ID: id, Kind: agentRuntimeResourceKindSkillCatalog,
			Source: agentRuntimeResourceSourceLocalSkill, Scope: agentRuntimeResourceScopeUser,
			Status:    agentRuntimeResourceStatusUnavailable,
			Revision:  agentRuntimeRevision([]string{"local_skill", "unavailable"}),
			ToolNames: []string{}, SkillNames: []string{},
			DiagnosticCodes: []string{agentRuntimeDiagnosticResourceUnavailable},
		}
	}
	if snapshot != nil && snapshot.mcp != nil && !snapshot.mcp.enabled() {
		id := agentRuntimeStableID("mcp", "runtime")
		byID[id] = &agentRuntimeResourceDescriptor{
			ID: id, Kind: agentRuntimeResourceKindToolSet,
			Source: agentRuntimeResourceSourceMCP, Scope: agentRuntimeResourceScopeRun,
			Status:    agentRuntimeResourceStatusUnavailable,
			Revision:  agentRuntimeRevision([]string{"mcp", "unavailable"}),
			ToolNames: []string{}, SkillNames: []string{},
			DiagnosticCodes: []string{agentRuntimeDiagnosticResourceUnavailable},
		}
	}
	if snapshot != nil && snapshot.localSkills != nil {
		for _, skill := range snapshot.localSkills.skills {
			name := strings.TrimSpace(skill.Name)
			id := agentRuntimeStableID("skill", name)
			byID[id] = &agentRuntimeResourceDescriptor{
				ID: id, Kind: agentRuntimeResourceKindSkillPackage,
				Source: agentRuntimeResourceSourceLocalSkill,
				Scope:  agentRuntimeResourceScopeUser,
				Status: agentRuntimeResourceStatusEnabled,
				Revision: agentRuntimeRevision([]string{
					name, strings.TrimSpace(skill.Version), strings.TrimSpace(skill.PackageFingerprint),
				}),
				SkillNames: []string{name}, ToolNames: []string{}, DiagnosticCodes: []string{},
			}
		}
	}
	if snapshot != nil && snapshot.mcp != nil {
		for _, server := range snapshot.mcp.run.Snapshot.Servers {
			id := agentRuntimeStableID("mcp", server.Ref.Key())
			status := agentRuntimeResourceStatusHidden
			tools := make([]string, 0, len(server.Tools))
			for _, tool := range server.Tools {
				if _, visible := snapshot.mcp.visible[tool.Alias]; visible {
					tools = append(tools, strings.TrimSpace(tool.Alias))
					status = agentRuntimeResourceStatusEnabled
				}
			}
			sort.Strings(tools)
			byID[id] = &agentRuntimeResourceDescriptor{
				ID: id, Kind: agentRuntimeResourceKindToolSet,
				Source: agentRuntimeResourceSourceMCP, Scope: agentRuntimeResourceScopeRun,
				Status: status, Revision: strings.TrimSpace(snapshot.mcp.run.Snapshot.Hash),
				ToolNames: tools, SkillNames: []string{}, DiagnosticCodes: []string{},
			}
		}
	}
	if registry != nil {
		for _, registration := range registry.ordered {
			id, kind, source, scope := snapshot.registrationResource(registration)
			descriptor := byID[id]
			if descriptor == nil {
				descriptor = &agentRuntimeResourceDescriptor{
					ID: id, Kind: kind, Source: source, Scope: scope,
					Status:    agentRuntimeResourceStatusEnabled,
					Revision:  agentRuntimeRevision(registrationRevisionInput(registration)),
					ToolNames: []string{}, SkillNames: []string{}, DiagnosticCodes: []string{},
				}
				byID[id] = descriptor
			}
			if registration.Definition == nil && len(descriptor.ToolNames) == 0 {
				descriptor.Status = agentRuntimeResourceStatusHidden
			}
			descriptor.ToolNames = appendUniqueString(descriptor.ToolNames, registration.Name)
		}
	}
	descriptors := make([]agentRuntimeResourceDescriptor, 0, len(byID))
	for _, descriptor := range byID {
		sort.Strings(descriptor.ToolNames)
		sort.Strings(descriptor.SkillNames)
		descriptors = append(descriptors, *descriptor)
	}
	sort.Slice(descriptors, func(left, right int) bool {
		return descriptors[left].ID < descriptors[right].ID
	})
	return descriptors
}

func (snapshot *agentRuntimeResourceSnapshot) resourceDiagnostics(
	registry *chatToolRegistry,
	descriptors []agentRuntimeResourceDescriptor,
) []agentRuntimeResourceDiagnostic {
	byID := make(map[string]int, len(descriptors))
	for index := range descriptors {
		byID[descriptors[index].ID] = index
	}
	diagnostics := make([]agentRuntimeResourceDiagnostic, 0)
	for index := range descriptors {
		if descriptors[index].Status != agentRuntimeResourceStatusUnavailable {
			continue
		}
		diagnostics = append(diagnostics, agentRuntimeResourceDiagnostic{
			Code:        agentRuntimeDiagnosticResourceUnavailable,
			ResourceIDs: []string{descriptors[index].ID},
		})
	}
	if registry != nil {
		names := make([]string, 0, len(registry.collisions))
		for name := range registry.collisions {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			ids := make([]string, 0, len(registry.collisions[name]))
			for _, registration := range registry.collisions[name] {
				id, _, _, _ := snapshot.registrationResource(registration)
				ids = appendUniqueString(ids, id)
				if index, exists := byID[id]; exists {
					descriptors[index].Status = agentRuntimeResourceStatusDegraded
					descriptors[index].DiagnosticCodes = appendUniqueString(
						descriptors[index].DiagnosticCodes,
						agentRuntimeDiagnosticToolNameCollision,
					)
				}
			}
			sort.Strings(ids)
			diagnostics = append(diagnostics, agentRuntimeResourceDiagnostic{
				Code:        agentRuntimeDiagnosticToolNameCollision,
				ResourceIDs: ids, ToolName: name,
			})
		}
	}
	sort.Slice(diagnostics, func(left, right int) bool {
		if diagnostics[left].Code != diagnostics[right].Code {
			return diagnostics[left].Code < diagnostics[right].Code
		}
		return strings.Join(diagnostics[left].ResourceIDs, "\x00") <
			strings.Join(diagnostics[right].ResourceIDs, "\x00")
	})
	return diagnostics
}

func (snapshot *agentRuntimeResourceSnapshot) registrationResource(
	registration chatToolRegistration,
) (string, agentRuntimeResourceKind, agentRuntimeResourceSource, agentRuntimeResourceScope) {
	name := strings.TrimSpace(registration.Name)
	switch registration.Backend {
	case chatToolBackendMCP:
		if snapshot != nil && snapshot.mcp != nil {
			if tool, ok := snapshot.mcp.visible[name]; ok {
				return agentRuntimeStableID("mcp", tool.ServerRef.Key()),
					agentRuntimeResourceKindToolSet, agentRuntimeResourceSourceMCP,
					agentRuntimeResourceScopeRun
			}
		}
		return agentRuntimeStableID("mcp", "tool-search"), agentRuntimeResourceKindToolSet,
			agentRuntimeResourceSourceMCP, agentRuntimeResourceScopeRun
	case chatToolBackendLocalSkill:
		if name == localSkillToolName || name == legacySkillsListToolName ||
			name == legacySkillViewToolName {
			return agentRuntimeStableID("skill", "catalog"), agentRuntimeResourceKindSkillCatalog,
				agentRuntimeResourceSourceLocalSkill, agentRuntimeResourceScopeUser
		}
		if registration.Definition == nil {
			return agentRuntimeStableID("builtin", "local-legacy"), agentRuntimeResourceKindToolSet,
				agentRuntimeResourceSourceBuiltin, agentRuntimeResourceScopeServer
		}
		return agentRuntimeStableID("builtin", "local-tools"), agentRuntimeResourceKindToolSet,
			agentRuntimeResourceSourceBuiltin, agentRuntimeResourceScopeServer
	case chatToolBackendWeb, chatToolBackendKnowledge, chatToolBackendMemory:
		return agentRuntimeStableID("retrieval", string(registration.Backend)),
			agentRuntimeResourceKindToolSet, agentRuntimeResourceSourceRetrieval,
			agentRuntimeResourceScopeRun
	case chatToolBackendGoal:
		return agentRuntimeStableID("builtin", "goals"), agentRuntimeResourceKindToolSet,
			agentRuntimeResourceSourceBuiltin, agentRuntimeResourceScopeRun
	default:
		return agentRuntimeStableID("builtin", string(registration.Backend)+":"+name),
			agentRuntimeResourceKindToolSet, agentRuntimeResourceSourceBuiltin,
			agentRuntimeResourceScopeServer
	}
}

func (snapshot *agentRuntimeResourceSnapshot) loadedSkillState() [][]string {
	if snapshot == nil || snapshot.localSkills == nil {
		return [][]string{}
	}
	state := make([][]string, 0, len(snapshot.localSkills.loaded)+1)
	for name, revision := range snapshot.localSkills.loaded {
		state = append(state, []string{strings.TrimSpace(name), strings.TrimSpace(revision)})
	}
	sort.Slice(state, func(left, right int) bool { return state[left][0] < state[right][0] })
	if required := snapshot.localSkills.requiredSkillName(); required != "" {
		state = append(state, []string{"required", required})
	}
	return state
}

func registrationRevisionInput(registration chatToolRegistration) any {
	return struct {
		Name           string               `json:"name"`
		Definition     *ToolDefinition      `json:"definition"`
		Backend        chatToolBackend      `json:"backend"`
		RiskClass      chatToolRiskClass    `json:"riskClass"`
		TimeoutNanos   int64                `json:"timeoutNanos"`
		MaxOutputBytes int64                `json:"maxOutputBytes"`
		AllowParallel  bool                 `json:"allowParallel"`
		ApprovalRule   chatToolApprovalRule `json:"approvalRule"`
		Presentation   chatToolPresentation `json:"presentation"`
		FirstTaskStep  bool                 `json:"firstTaskStep"`
	}{
		Name: registration.Name, Definition: registration.Definition,
		Backend: registration.Backend, RiskClass: registration.RiskClass,
		TimeoutNanos: int64(registration.Timeout), MaxOutputBytes: registration.MaxOutputBytes,
		AllowParallel: registration.AllowParallel, ApprovalRule: registration.ApprovalRule,
		Presentation: registration.Presentation, FirstTaskStep: registration.FirstTaskStep,
	}
}

func agentRuntimeStableID(prefix string, authority string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(prefix) + "\x00" + strings.TrimSpace(authority)))
	return strings.TrimSpace(prefix) + ":" + hex.EncodeToString(digest[:12])
}

func agentRuntimeRevision(value any) string {
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}
