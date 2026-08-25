package chat

import (
	"encoding/json"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/mcpclient"
)

type chatToolRiskClass string

const (
	chatToolRiskRead     chatToolRiskClass = "read"
	chatToolRiskWrite    chatToolRiskClass = "write"
	chatToolRiskExecute  chatToolRiskClass = "execute"
	chatToolRiskExternal chatToolRiskClass = "external"
)

type chatToolBackend string

const (
	chatToolBackendWeb        chatToolBackend = "web"
	chatToolBackendKnowledge  chatToolBackend = "knowledge"
	chatToolBackendMemory     chatToolBackend = "memory"
	chatToolBackendMCP        chatToolBackend = "mcp"
	chatToolBackendLocalSkill chatToolBackend = "local_skill"
	chatToolBackendGoal       chatToolBackend = "goal"
)

type chatToolApprovalRule string

const (
	chatToolApprovalNone        chatToolApprovalRule = "none"
	chatToolApprovalMCPPolicy   chatToolApprovalRule = "mcp_policy"
	chatToolApprovalLocalPolicy chatToolApprovalRule = "local_policy"
)

type chatToolPresentation string

const (
	chatToolPresentationSearch chatToolPresentation = "search"
	chatToolPresentationTool   chatToolPresentation = "tool"
)

type chatToolResultProjector func(ProviderToolResult) ProviderToolResult

type chatToolRegistration struct {
	Name            string
	Definition      *ToolDefinition
	Backend         chatToolBackend
	RiskClass       chatToolRiskClass
	Timeout         time.Duration
	MaxOutputBytes  int64
	AllowParallel   bool
	ApprovalRule    chatToolApprovalRule
	Presentation    chatToolPresentation
	FirstTaskStep   bool
	ProjectForModel chatToolResultProjector
}

type chatToolRegistry struct {
	ordered            []chatToolRegistration
	byName             map[string]chatToolRegistration
	colliding          map[string]struct{}
	requiredLocalSkill bool
}

type chatToolBatchExecution struct {
	Results       map[int]ProviderToolResult
	BudgetReached bool
	Stop          bool
}

func newChatToolRegistry(input externalWebToolLoopInput) *chatToolRegistry {
	registry := &chatToolRegistry{
		ordered:   make([]chatToolRegistration, 0, 7+len(input.MCP.definitions())),
		byName:    make(map[string]chatToolRegistration),
		colliding: make(map[string]struct{}),
	}
	registry.registerGoals(input.Goals)
	if input.Memory.requiresFirstRoundCall() {
		registry.register(retrievalToolRegistration(
			SearchMemoryToolDefinition(), chatToolBackendMemory, chatToolRiskRead, true,
		))
	}
	if externalWebToolEnabled(input) {
		registry.register(retrievalToolRegistration(
			searchWebToolDefinition(), chatToolBackendWeb, chatToolRiskExternal, false,
		))
		registry.register(retrievalToolRegistration(
			readWebURLToolDefinition(), chatToolBackendWeb, chatToolRiskExternal, false,
		))
	}
	if input.Knowledge.enabled() {
		registry.register(retrievalToolRegistration(
			searchKnowledgeToolDefinition(), chatToolBackendKnowledge, chatToolRiskRead, false,
		))
	}
	if input.Memory.enabled() && !input.Memory.requiresFirstRoundCall() {
		registry.register(retrievalToolRegistration(
			SearchMemoryToolDefinition(), chatToolBackendMemory, chatToolRiskRead, true,
		))
	}
	registry.registerMCP(input.MCP)
	registry.registerLocalSkills(input.LocalSkills)
	return registry
}

func (registry *chatToolRegistry) registerGoals(runtime *chatAgentGoalToolRuntime) {
	if registry == nil || !runtime.enabled() {
		return
	}
	for _, definition := range chatAgentGoalToolDefinitions() {
		risk := chatToolRiskRead
		if definition.Function.Name == chatAgentCreateGoalToolName ||
			definition.Function.Name == chatAgentUpdateGoalToolName {
			risk = chatToolRiskWrite
		}
		registry.register(chatToolRegistration{
			Name: definition.Function.Name, Definition: &definition,
			Backend: chatToolBackendGoal, RiskClass: risk,
			MaxOutputBytes:  maxEvidenceRecoveryOutputBytes,
			ApprovalRule:    chatToolApprovalNone,
			Presentation:    chatToolPresentationTool,
			ProjectForModel: identityChatToolResult,
		})
	}
}

func newRequiredLocalSkillRegistry(runtime *localSkillToolRuntime) *chatToolRegistry {
	registry := &chatToolRegistry{
		ordered:            make([]chatToolRegistration, 0, 1),
		byName:             make(map[string]chatToolRegistration, 1),
		colliding:          make(map[string]struct{}),
		requiredLocalSkill: true,
	}
	definitions := runtime.requiredDefinition()
	if len(definitions) == 1 {
		registry.register(localSkillToolRegistration(definitions[0], runtime))
	}
	return registry
}

func retrievalToolRegistration(
	definition ToolDefinition,
	backend chatToolBackend,
	risk chatToolRiskClass,
	firstTaskStep bool,
) chatToolRegistration {
	return chatToolRegistration{
		Name: definition.Function.Name, Definition: &definition, Backend: backend,
		RiskClass: risk, MaxOutputBytes: maxEvidenceRecoveryOutputBytes,
		ApprovalRule: chatToolApprovalNone, Presentation: chatToolPresentationSearch,
		FirstTaskStep: firstTaskStep, ProjectForModel: identityChatToolResult,
	}
}

func localSkillToolRegistration(
	definition ToolDefinition,
	runtime *localSkillToolRuntime,
) chatToolRegistration {
	name := strings.TrimSpace(definition.Function.Name)
	canonicalName := canonicalLocalToolName(name)
	risk := chatToolRiskRead
	approval := chatToolApprovalNone
	if canonicalName == localTerminalToolName {
		risk = chatToolRiskExecute
		approval = chatToolApprovalLocalPolicy
	} else if canonicalName == localJobKillToolName {
		risk = chatToolRiskExecute
		approval = chatToolApprovalLocalPolicy
	} else if canonicalName == localFileWriteToolName || canonicalName == localFileEditToolName {
		risk = chatToolRiskWrite
	} else if canonicalName == localPublishFileToolName {
		risk = chatToolRiskWrite
	}
	config := runtime.config()
	return chatToolRegistration{
		Name: name, Definition: &definition, Backend: chatToolBackendLocalSkill,
		RiskClass: risk, Timeout: config.CallTimeout, MaxOutputBytes: int64(config.MaxOutput),
		AllowParallel: localSkillToolAllowsParallel(name),
		ApprovalRule:  approval, Presentation: chatToolPresentationTool,
		ProjectForModel: identityChatToolResult,
	}
}

func (registry *chatToolRegistry) registerMCP(runtime *mcpToolRuntime) {
	if registry == nil || !runtime.enabled() {
		return
	}
	config := runtime.service.Config()
	for _, definition := range runtime.definitions() {
		name := strings.TrimSpace(definition.Function.Name)
		classification := mcpclient.ClassificationRead
		if tool, ok := runtime.visible[name]; ok {
			classification = tool.Classification
		}
		risk := chatToolRiskRead
		approval := chatToolApprovalNone
		parallel := classification == mcpclient.ClassificationRead &&
			name != mcpclient.ToolSearchAlias
		switch classification {
		case mcpclient.ClassificationWrite:
			risk = chatToolRiskWrite
			approval = chatToolApprovalMCPPolicy
		case mcpclient.ClassificationUnknown:
			risk = chatToolRiskExternal
			approval = chatToolApprovalMCPPolicy
		}
		registry.register(chatToolRegistration{
			Name: name, Definition: &definition, Backend: chatToolBackendMCP,
			RiskClass: risk, Timeout: config.CallTimeout,
			MaxOutputBytes: config.MaxResultCallBytes, AllowParallel: parallel,
			ApprovalRule: approval, Presentation: chatToolPresentationTool,
			ProjectForModel: identityChatToolResult,
		})
	}
}

func (registry *chatToolRegistry) registerLocalSkills(runtime *localSkillToolRuntime) {
	if registry == nil || !runtime.enabled() {
		return
	}
	for _, definition := range runtime.definitions() {
		registry.register(localSkillToolRegistration(definition, runtime))
	}
	// Keep names used by pre-migration in-flight continuations executable, but
	// never advertise them to a new Provider round.
	for _, name := range []string{
		legacyFileReadToolName, legacyFileWriteToolName, legacyFileEditToolName,
		legacyFileSearchToolName, legacyTerminalToolName,
	} {
		registration := localSkillToolRegistration(ToolDefinition{
			Function: ToolFunctionDefinition{Name: name},
		}, runtime)
		registration.Definition = nil
		registry.register(registration)
	}
	// These names remain executable only for bounded in-Turn continuation
	// compatibility. They are intentionally absent from model definitions.
	if !runtime.skillsAvailable() {
		return
	}
	for _, name := range []string{legacySkillsListToolName, legacySkillViewToolName} {
		registry.register(chatToolRegistration{
			Name: name, Backend: chatToolBackendLocalSkill, RiskClass: chatToolRiskRead,
			Timeout:        runtime.config().CallTimeout,
			MaxOutputBytes: int64(runtime.config().MaxOutput),
			AllowParallel:  localSkillToolAllowsParallel(name),
			ApprovalRule:   chatToolApprovalNone, Presentation: chatToolPresentationTool,
			ProjectForModel: identityChatToolResult,
		})
	}
}

func (registry *chatToolRegistry) register(registration chatToolRegistration) bool {
	if registry == nil {
		return false
	}
	name := normalizedToolName(registration.Name)
	if name == "unknown" || registration.ProjectForModel == nil {
		return false
	}
	registration.Name = name
	if _, collided := registry.colliding[name]; collided {
		return false
	}
	if _, exists := registry.byName[name]; exists {
		delete(registry.byName, name)
		registry.colliding[name] = struct{}{}
		return false
	}
	registry.byName[name] = registration
	registry.ordered = append(registry.ordered, registration)
	return true
}

func (registry *chatToolRegistry) lookup(name string) (chatToolRegistration, bool) {
	if registry == nil {
		return chatToolRegistration{}, false
	}
	name = normalizedToolName(name)
	if _, collided := registry.colliding[name]; collided {
		return chatToolRegistration{}, false
	}
	registration, ok := registry.byName[name]
	return registration, ok
}

func (registry *chatToolRegistry) definitions(taskStep int) []ToolDefinition {
	if registry == nil {
		return nil
	}
	definitions := make([]ToolDefinition, 0, len(registry.ordered))
	for _, registration := range registry.ordered {
		if registration.Definition == nil ||
			(registration.FirstTaskStep && taskStep > 1) {
			continue
		}
		if _, available := registry.lookup(registration.Name); !available {
			continue
		}
		definitions = append(definitions, *registration.Definition)
	}
	return definitions
}

func (registry *chatToolRegistry) projectResult(result ProviderToolResult) ProviderToolResult {
	registration, ok := registry.lookup(result.Name)
	if !ok {
		return result
	}
	return registration.ProjectForModel(result)
}

func (registry *chatToolRegistry) callsForBackend(
	calls []ProviderToolCall,
	backend chatToolBackend,
) []ProviderToolCall {
	filtered := make([]ProviderToolCall, len(calls))
	for index, call := range calls {
		registration, ok := registry.lookup(call.Name)
		if ok && registration.Backend == backend {
			filtered[index] = call
		}
	}
	return filtered
}

func (registry *chatToolRegistry) projectResults(
	results map[int]ProviderToolResult,
) map[int]ProviderToolResult {
	projected := make(map[int]ProviderToolResult, len(results))
	for index, result := range results {
		projected[index] = registry.projectResult(result)
	}
	return projected
}

func identityChatToolResult(result ProviderToolResult) ProviderToolResult {
	return result
}

func chatToolFailureResult(call ProviderToolCall, category string) ProviderToolResult {
	encoded, _ := json.Marshal(map[string]any{
		"untrustedChatToolResult": true,
		"isError":                 true,
		"error":                   strings.TrimSpace(category),
	})
	return ProviderToolResult{
		CallID: call.ID, Name: call.Name, Content: string(encoded), IsError: true,
	}
}
