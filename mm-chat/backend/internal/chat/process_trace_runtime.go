package chat

import (
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/websearch"
)

func addLegacyKnowledgeProcessStep(
	trace *processTrace,
	selection ragSelection,
	decision autoRAGDecision,
	startedAt time.Time,
	durationMS int64,
) {
	if trace == nil || !selection.Enabled {
		return
	}
	status := ProcessStepStatusCompleted
	outcome := strings.TrimSpace(decision.Outcome)
	if outcome == "" {
		outcome = "no_evidence"
	}
	if outcome == "dependency_unavailable" ||
		outcome == "answer_governance_required" {
		status = ProcessStepStatusFailed
	}
	detail := map[string]any{
		"outcome":        outcome,
		"hitCount":       len(decision.Citations),
		"selectedCount":  len(selection.CollectionIDs),
		"queryRewritten": decision.QueryRewritten,
	}
	if status == ProcessStepStatusFailed {
		detail["failureCategory"] = outcome
	}
	trace.add(terminalProcessStep(
		trace.stepID(ProcessStepKindKnowledge),
		ProcessStepKindKnowledge,
		status,
		startedAt,
		durationMS,
		detail,
	))
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
