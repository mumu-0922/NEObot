package chat

type agentRuntimeResourceDescriptor struct {
	ID               string                     `json:"id"`
	Kind             agentRuntimeResourceKind   `json:"kind"`
	Source           agentRuntimeResourceSource `json:"source"`
	Scope            agentRuntimeResourceScope  `json:"scope"`
	Status           agentRuntimeResourceStatus `json:"status"`
	ActivationSource string                     `json:"activationSource,omitempty"`
	Revision         string                     `json:"revision"`
	ToolNames        []string                   `json:"toolNames"`
	SkillNames       []string                   `json:"skillNames"`
	DiagnosticCodes  []string                   `json:"diagnosticCodes"`
}

type agentRuntimeResourceDiagnostic struct {
	Code        string   `json:"code"`
	ResourceIDs []string `json:"resourceIds"`
	ToolName    string   `json:"toolName,omitempty"`
}

type agentRuntimeResourceReport struct {
	RunRevision        string                           `json:"runRevision"`
	ProjectionRevision string                           `json:"projectionRevision"`
	TaskStep           int                              `json:"taskStep"`
	RequiredSkillOnly  bool                             `json:"requiredSkillOnly"`
	Resources          []agentRuntimeResourceDescriptor `json:"resources"`
	Diagnostics        []agentRuntimeResourceDiagnostic `json:"diagnostics"`
}
