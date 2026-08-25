package chat

import (
	"sort"
	"strings"
)

func agentRuntimeStepAuthority(input externalWebToolLoopInput) any {
	knowledgeAuthority := []string{}
	if input.Knowledge != nil {
		knowledgeAuthority = append(
			knowledgeAuthority,
			input.Knowledge.SelectedCollectionIDs...,
		)
		sort.Strings(knowledgeAuthority)
	}
	return struct {
		KnowledgeAuthority []string `json:"knowledgeAuthority"`
		KnowledgeEnabled   bool     `json:"knowledgeEnabled"`
		MemoryEnabled      bool     `json:"memoryEnabled"`
		MemoryFirst        bool     `json:"memoryFirst"`
		GoalsEnabled       bool     `json:"goalsEnabled"`
		ResourceEnabled    bool     `json:"resourceEnabled"`
		ExternalWeb        bool     `json:"externalWeb"`
	}{
		KnowledgeAuthority: knowledgeAuthority,
		KnowledgeEnabled:   input.Knowledge.enabled(),
		MemoryEnabled:      input.Memory.enabled(),
		MemoryFirst:        input.Memory.requiresFirstRoundCall(),
		GoalsEnabled:       input.Goals.enabled(),
		ResourceEnabled:    input.Resource.enabled(),
		ExternalWeb:        externalWebToolEnabled(input),
	}
}

func (snapshot *agentRuntimeResourceSnapshot) runAuthority() any {
	authority := struct {
		MCPHash              string     `json:"mcpHash"`
		MCPSelectionRevision int64      `json:"mcpSelectionRevision"`
		SkillCatalogRevision string     `json:"skillCatalogRevision"`
		SkillPackages        [][]string `json:"skillPackages"`
		WorkspaceAuthority   string     `json:"workspaceAuthority"`
		KnowledgeAuthority   []string   `json:"knowledgeAuthority"`
		Memory               bool       `json:"memory"`
		MemoryFirst          bool       `json:"memoryFirst"`
		Goals                bool       `json:"goals"`
		Resource             bool       `json:"resource"`
		ExternalWeb          bool       `json:"externalWeb"`
	}{}
	if snapshot != nil && snapshot.mcp != nil {
		authority.MCPHash = strings.TrimSpace(snapshot.mcp.run.Snapshot.Hash)
		authority.MCPSelectionRevision = snapshot.mcp.run.Snapshot.SelectionRevision
	}
	if snapshot != nil && snapshot.localSkills != nil {
		authority.SkillCatalogRevision = snapshot.localSkills.catalogRevision
		authority.WorkspaceAuthority = agentRuntimeStableID(
			"workspace", snapshot.localSkills.workspaceID,
		)
		for _, skill := range snapshot.localSkills.skills {
			authority.SkillPackages = append(authority.SkillPackages, []string{
				strings.TrimSpace(skill.Name), strings.TrimSpace(skill.Version),
				strings.TrimSpace(skill.PackageFingerprint),
			})
		}
	}
	if snapshot != nil && snapshot.knowledge != nil {
		authority.KnowledgeAuthority = append(
			[]string(nil), snapshot.knowledge.SelectedCollectionIDs...,
		)
		sort.Strings(authority.KnowledgeAuthority)
	}
	if snapshot != nil {
		authority.Memory = snapshot.memory.enabled()
		authority.MemoryFirst = snapshot.memory.requiresFirstRoundCall()
		authority.Goals = snapshot.goals.enabled()
		authority.Resource = snapshot.resource.enabled()
		authority.ExternalWeb = snapshot.externalWeb
	}
	return authority
}
