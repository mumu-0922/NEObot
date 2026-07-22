package chat

import (
	"encoding/json"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/websearch"
)

type toolProcessTrace struct {
	trace            *processTrace
	toolStepIDs      map[string]string
	webStepIDs       map[string]string
	knowledgeStepIDs map[string]string
}

func newToolProcessTrace(trace *processTrace) *toolProcessTrace {
	return &toolProcessTrace{
		trace:            trace,
		toolStepIDs:      map[string]string{},
		webStepIDs:       map[string]string{},
		knowledgeStepIDs: map[string]string{},
	}
}

func (runtime *toolProcessTrace) apply(
	event *ProviderToolExecutionEvent,
	at time.Time,
) []ProcessStep {
	if runtime == nil || runtime.trace == nil || event == nil ||
		strings.TrimSpace(event.ExecutionID) == "" {
		return nil
	}
	detail := toolProcessDetail(event)
	updates := make([]ProcessStep, 0, 2)
	toolStepID := runtime.toolStepIDs[event.ExecutionID]
	webStepID := runtime.webStepIDs[event.ExecutionID]
	knowledgeStepID := runtime.knowledgeStepIDs[event.ExecutionID]
	if toolStepID == "" {
		step := runtime.trace.startNext(
			ProcessStepKindTool,
			"process.tool",
			at,
			detail,
		)
		toolStepID = step.ID
		runtime.toolStepIDs[event.ExecutionID] = toolStepID
		updates = append(updates, step)
	}
	if event.Name == searchWebToolName && webStepID == "" {
		step := runtime.trace.startNext(
			ProcessStepKindWeb,
			"process.web",
			at,
			detail,
		)
		webStepID = step.ID
		runtime.webStepIDs[event.ExecutionID] = webStepID
		updates = append(updates, step)
	}
	if event.Name == searchKnowledgeToolName && knowledgeStepID == "" {
		step := runtime.trace.startNext(
			ProcessStepKindKnowledge,
			"process.knowledge",
			at,
			detail,
		)
		knowledgeStepID = step.ID
		runtime.knowledgeStepIDs[event.ExecutionID] = knowledgeStepID
		updates = append(updates, step)
	}
	if event.Status == ProcessStepStatusRunning {
		return updates
	}
	status := normalizeProcessStepStatus(event.Status)
	if status == "" {
		status = ProcessStepStatusFailed
	}
	if step, ok := runtime.trace.transitionID(toolStepID, status, at, detail); ok {
		updates = append(updates, step)
	}
	if webStepID != "" {
		if step, ok := runtime.trace.transitionID(webStepID, status, at, detail); ok {
			updates = append(updates, step)
		}
	}
	if knowledgeStepID != "" {
		if step, ok := runtime.trace.transitionID(knowledgeStepID, status, at, detail); ok {
			updates = append(updates, step)
		}
	}
	return updates
}

func toolProcessDetail(event *ProviderToolExecutionEvent) map[string]any {
	detail := map[string]any{
		"toolName": event.Name,
		"round":    event.Round,
		"mode":     event.Mode,
	}
	if query := strings.TrimSpace(event.Query); query != "" {
		detail["query"] = query
	}
	if len(event.Arguments) > 0 {
		if encoded, err := json.Marshal(event.Arguments); err == nil {
			detail["redactedArgs"] = string(encoded)
		}
	}
	if event.Search != nil {
		detail["sourceCount"] = len(event.Search.Sources)
	}
	if event.Name == searchKnowledgeToolName {
		detail["hitCount"] = len(event.CitationMarkers)
		if event.Knowledge != nil {
			detail["outcome"] = event.Knowledge.Outcome
			detail["queryRewritten"] = event.Knowledge.QueryRewritten
			if rerankStatus := strings.TrimSpace(event.Knowledge.RerankStatus); rerankStatus != "" {
				detail["rerankStatus"] = rerankStatus
			}
		}
	}
	if len(event.CitationMarkers) > 0 {
		detail["citationMarkers"] = append([]string(nil), event.CitationMarkers...)
	}
	if failure := strings.TrimSpace(event.FailureCategory); failure != "" {
		detail["failureCategory"] = failure
		detail["outcome"] = "degraded"
	} else if event.Status == ProcessStepStatusCompleted {
		if _, ok := detail["outcome"]; !ok {
			detail["outcome"] = "completed"
		}
	} else {
		detail["outcome"] = "running"
	}
	return detail
}

func addLegacyWebProcessStep(
	trace *processTrace,
	plan sourceFusionPlan,
	diagnostics sourceFusionDiagnostics,
	execution *websearch.ActiveExecution,
	query string,
	result websearch.Result,
	startedAt time.Time,
	durationMS int64,
) {
	if trace == nil || !plan.SearchRequested {
		return
	}
	if execution != nil && execution.Mode == websearch.ExecutionModelBuiltIn {
		return
	}

	status := ProcessStepStatusCompleted
	outcome := strings.TrimSpace(diagnostics.WebExecuteOutcome)
	if diagnostics.DegradationReason != "" || outcome == "degraded" ||
		outcome == "not_run" {
		status = ProcessStepStatusFailed
	}
	detail := map[string]any{
		"outcome":     outcome,
		"sourceCount": len(result.Sources),
	}
	if query = strings.TrimSpace(query); query != "" {
		detail["query"] = query
	}
	if execution != nil {
		detail["mode"] = string(execution.Mode)
		if execution.External != nil {
			detail["provider"] = string(execution.External.ID())
		}
	}
	if status == ProcessStepStatusFailed {
		failureCategory := strings.TrimSpace(diagnostics.DegradationReason)
		if failureCategory == "" {
			failureCategory = "unavailable"
		}
		detail["failureCategory"] = failureCategory
	}
	trace.add(terminalProcessStep(
		trace.stepID(ProcessStepKindWeb),
		ProcessStepKindWeb,
		status,
		startedAt,
		durationMS,
		detail,
	))
}

func startBuiltInWebProcessStep(
	trace *processTrace,
	execution *websearch.ActiveExecution,
	startedAt time.Time,
) ProcessStep {
	detail := map[string]any{
		"mode":    string(websearch.ExecutionModelBuiltIn),
		"outcome": "provider_stream",
	}
	if execution != nil && execution.ModelBuiltIn != "" {
		detail["provider"] = string(execution.ModelBuiltIn)
	}
	return trace.start(
		ProcessStepKindWeb,
		"process.web",
		startedAt,
		detail,
	)
}

func completeBuiltInWebProcessStep(
	trace *processTrace,
	status string,
	completedAt time.Time,
	result websearch.Result,
	failureCategory string,
) (ProcessStep, bool) {
	current, ok := trace.get(ProcessStepKindWeb)
	if !ok || isTerminalProcessStepStatus(current.Status) {
		return ProcessStep{}, false
	}
	detail := cloneProcessDetail(current.Detail)
	if detail == nil {
		detail = map[string]any{}
	}
	detail["sourceCount"] = len(result.Sources)
	_, citations := prepareWebSearchResult(result)
	if markers := webCitationMarkers(citations); len(markers) > 0 {
		detail["citationMarkers"] = markers
	}
	switch status {
	case ProcessStepStatusCompleted:
		if len(result.Sources) == 0 {
			detail["outcome"] = "no_results"
		} else {
			detail["outcome"] = "completed"
		}
	case ProcessStepStatusFailed:
		detail["outcome"] = "degraded"
		if failureCategory = strings.TrimSpace(failureCategory); failureCategory != "" {
			detail["failureCategory"] = failureCategory
		}
	case ProcessStepStatusCancelled:
		detail["outcome"] = "cancelled"
	}
	return trace.transition(ProcessStepKindWeb, status, completedAt, detail)
}

func reconcileProcessTraceCitations(
	trace *processTrace,
	content string,
) []ProcessStep {
	if trace == nil {
		return nil
	}
	updates := make([]ProcessStep, 0)
	for _, step := range trace.snapshot() {
		if step.Kind != ProcessStepKindWeb && step.Kind != ProcessStepKindKnowledge &&
			step.Kind != ProcessStepKindTool {
			continue
		}
		detail := cloneProcessDetail(step.Detail)
		rawMarkers, ok := detail["citationMarkers"].([]string)
		if !ok || len(rawMarkers) == 0 {
			continue
		}
		used := make([]string, 0, len(rawMarkers))
		for _, marker := range rawMarkers {
			if strings.Contains(content, marker) {
				used = append(used, marker)
			}
		}
		if len(used) == 0 {
			delete(detail, "citationMarkers")
			if step.Status == ProcessStepStatusCompleted {
				detail["outcome"] = "completed_unreferenced"
			}
		} else {
			detail["citationMarkers"] = used
		}
		step.Detail = detail
		updates = append(updates, trace.add(step))
	}
	return updates
}

func finishProcessTrace(
	trace *processTrace,
	terminalStatus string,
	completedAt time.Time,
	webResult websearch.Result,
) []ProcessStep {
	if trace == nil {
		return nil
	}
	stepStatus := processStepStatusForMessageStatus(terminalStatus)
	updates := make([]ProcessStep, 0, 3)
	if step, ok := trace.transition(
		ProcessStepKindReasoning,
		stepStatus,
		completedAt,
		nil,
	); ok {
		updates = append(updates, step)
	}
	if step, ok := completeBuiltInWebProcessStep(
		trace,
		stepStatus,
		completedAt,
		webResult,
		processFailureCategory(terminalStatus),
	); ok {
		updates = append(updates, step)
	}
	if step, ok := trace.transition(
		ProcessStepKindTool,
		stepStatus,
		completedAt,
		nil,
	); ok {
		updates = append(updates, step)
	}
	for _, current := range trace.snapshot() {
		if isTerminalProcessStepStatus(current.Status) ||
			(current.Kind != ProcessStepKindTool && current.Kind != ProcessStepKindWeb &&
				current.Kind != ProcessStepKindKnowledge) {
			continue
		}
		detail := cloneProcessDetail(current.Detail)
		if detail == nil {
			detail = map[string]any{}
		}
		detail["outcome"] = terminalStatus
		if failure := processFailureCategory(terminalStatus); failure != "" {
			detail["failureCategory"] = failure
		}
		if step, ok := trace.transitionID(
			current.ID,
			stepStatus,
			completedAt,
			detail,
		); ok {
			updates = append(updates, step)
		}
	}
	if step, ok := trace.transition(
		ProcessStepKindGeneration,
		stepStatus,
		completedAt,
		map[string]any{"outcome": terminalStatus},
	); ok {
		updates = append(updates, step)
	}
	return updates
}

func terminalProcessStep(
	id string,
	kind string,
	status string,
	startedAt time.Time,
	durationMS int64,
	detail map[string]any,
) ProcessStep {
	if durationMS < 0 {
		durationMS = 0
	}
	if durationMS > maxFusionStageDurationMillis {
		durationMS = maxFusionStageDurationMillis
	}
	completedAt := startedAt.Add(time.Duration(durationMS) * time.Millisecond)
	return ProcessStep{
		ID:          id,
		Kind:        kind,
		Status:      status,
		LabelKey:    "process." + kind,
		StartedAt:   formatTime(startedAt),
		CompletedAt: formatTime(completedAt),
		DurationMS:  durationMS,
		Detail:      detail,
	}
}

func processStepStatusForMessageStatus(status string) string {
	switch status {
	case "completed":
		return ProcessStepStatusCompleted
	case "cancelled":
		return ProcessStepStatusCancelled
	default:
		return ProcessStepStatusFailed
	}
}

func processFailureCategory(status string) string {
	switch status {
	case "cancelled":
		return "cancelled"
	case "failed":
		return "provider_failed"
	default:
		return ""
	}
}
