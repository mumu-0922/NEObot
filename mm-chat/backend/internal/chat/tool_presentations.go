package chat

import (
	"encoding/json"
	"fmt"
	"strings"
)

func (runtime *localSkillToolRuntime) localProcessPresentation(
	call ProviderToolCall,
) *ProcessStepPresentation {
	name := strings.TrimSpace(call.Name)
	if name == localTerminalToolName {
		presentation := runtime.terminalProcessPresentation(call)
		if presentation != nil {
			presentation.Version = 1
		}
		return presentation
	}
	arguments := decodePresentationObject(call.Arguments)
	switch name {
	case localFileReadToolName, localFileWriteToolName, localFileEditToolName,
		localFileSearchToolName, localPublishFileToolName:
		presentation := &ProcessStepPresentation{
			Version: 1, Card: "file", Operation: strings.TrimPrefix(name, "file_"),
			Path: presentationString(arguments, "path"),
		}
		if name == localPublishFileToolName {
			presentation.Operation = "publish"
		}
		if name == localFileReadToolName {
			presentation.Offset = int64(presentationInt(arguments, "offset"))
		}
		if name == localFileSearchToolName {
			presentation.Summary = presentationString(arguments, "glob")
		}
		if name == localFileWriteToolName {
			presentation.Diff = unifiedPresentationDiff(
				presentation.Path, "", presentationString(arguments, "content"),
			)
		}
		if name == localFileEditToolName {
			presentation.Diff = unifiedPresentationDiff(
				presentation.Path,
				presentationString(arguments, "oldText"),
				presentationString(arguments, "newText"),
			)
		}
		return presentation
	case localJobListToolName, localJobOutputToolName, localJobKillToolName:
		return &ProcessStepPresentation{
			Version: 1, Card: "job", Operation: strings.TrimPrefix(name, "job_"),
			JobID: presentationString(arguments, "jobId"),
		}
	case localSkillToolName:
		return &ProcessStepPresentation{
			Version: 1, Card: "skill", Title: presentationString(arguments, "name"),
			Operation: "load",
		}
	default:
		return nil
	}
}

func completeLocalProcessPresentation(
	presentation *ProcessStepPresentation,
	result ProviderToolResult,
) *ProcessStepPresentation {
	if presentation == nil {
		return nil
	}
	completed := *presentation
	completed.Transcript = append([]ProcessTranscriptEntry(nil), presentation.Transcript...)
	completed.Items = append([]ProcessPresentationItem(nil), presentation.Items...)
	if presentation.Approval != nil {
		approval := *presentation.Approval
		completed.Approval = &approval
	}
	if result.IsError {
		return &completed
	}
	payload := decodePresentationObject(result.Content)
	resultValue, _ := payload["result"].(map[string]any)
	switch completed.Card {
	case "terminal":
		exitCode := presentationInt(payload, "exitCode")
		completed.ExitCode = &exitCode
		completed.TimedOut = presentationBool(payload, "timedOut")
		completed.Truncated = presentationBool(payload, "truncated")
		completed.Transcript = transcriptFromOutput(
			presentationString(payload, "stdout"), presentationString(payload, "stderr"),
		)
	case "file":
		if resultValue != nil {
			if path := presentationString(resultValue, "path"); path != "" {
				completed.Path = path
			}
			completed.Content = presentationString(resultValue, "content")
			completed.Size = int64(presentationInt(resultValue, "size"))
			completed.Offset = int64(presentationInt(resultValue, "offset"))
			completed.NextOffset = int64(presentationInt(resultValue, "nextOffset"))
			completed.Truncated = presentationBool(resultValue, "truncated")
			if completed.Operation == "search" {
				completed.Items = presentationSearchMatches(resultValue["matches"])
				completed.Count = len(completed.Items)
				completed.Summary = fmt.Sprintf(
					"%d files · %d bytes",
					presentationInt(resultValue, "filesScanned"),
					presentationInt(resultValue, "bytesScanned"),
				)
			}
			if completed.Operation == "publish" {
				completed.Title = presentationString(resultValue, "fileName")
				completed.Summary = presentationString(resultValue, "contentType")
				completed.Items = []ProcessPresentationItem{
					{Label: "SHA-256", Detail: presentationString(resultValue, "sha256")},
				}
			}
		}
	case "job":
		if resultValue != nil {
			completed.JobID = presentationString(resultValue, "jobId")
			completed.JobStatus = presentationString(resultValue, "status")
			completed.TimedOut = presentationBool(resultValue, "timedOut")
			completed.Truncated = presentationBool(resultValue, "truncated")
			completed.Transcript = transcriptFromOutput(
				presentationString(resultValue, "stdout"), presentationString(resultValue, "stderr"),
			)
		}
	case "skill":
		if name := presentationString(payload, "name"); name != "" {
			completed.Title = name
		}
		completed.Summary = presentationString(payload, "version")
	}
	return &completed
}

func searchProcessPresentation(
	toolName string,
	_ string,
	provider string,
	count int,
) *ProcessStepPresentation {
	title := "Web search"
	switch strings.TrimSpace(toolName) {
	case searchKnowledgeToolName:
		title = "Knowledge search"
	case "search_memory":
		title = "Memory search"
	}
	return &ProcessStepPresentation{
		Version: 1, Card: "search", Title: title,
		Provider: provider, Count: max(count, 0),
	}
}

func goalProcessPresentation(call ProviderToolCall) *ProcessStepPresentation {
	arguments := decodePresentationObject(call.Arguments)
	return &ProcessStepPresentation{
		Version: 1, Card: "goal", Operation: strings.TrimPrefix(call.Name, "goal_"),
		Title:   presentationString(arguments, "objective"),
		Summary: presentationString(arguments, "action"),
	}
}

func mcpProcessPresentation(
	serverName string,
	toolName string,
) *ProcessStepPresentation {
	card := "mcp"
	if strings.HasPrefix(strings.TrimSpace(toolName), "browser_") {
		card = "browser"
	}
	return &ProcessStepPresentation{
		Version: 1, Card: card, Title: strings.TrimSpace(serverName),
		Operation: strings.TrimSpace(toolName),
		Summary:   "Details hidden by the safe presenter",
	}
}

func transcriptFromOutput(stdout, stderr string) []ProcessTranscriptEntry {
	entries := make([]ProcessTranscriptEntry, 0, 2)
	if stdout != "" {
		entries = append(entries, ProcessTranscriptEntry{Sequence: 1, Stream: "stdout", Content: stdout})
	}
	if stderr != "" {
		entries = append(entries, ProcessTranscriptEntry{Sequence: len(entries) + 1, Stream: "stderr", Content: stderr})
	}
	return entries
}

func presentationSearchMatches(value any) []ProcessPresentationItem {
	values, _ := value.([]any)
	items := make([]ProcessPresentationItem, 0, min(len(values), maxProcessPresentationItems))
	for _, value := range values {
		item, _ := value.(map[string]any)
		path := presentationString(item, "path")
		if path == "" {
			continue
		}
		line := presentationInt(item, "line")
		if line > 0 {
			path = fmt.Sprintf("%s:%d", path, line)
		}
		items = append(items, ProcessPresentationItem{
			Label: path, Detail: presentationString(item, "preview"),
		})
	}
	return items
}

func unifiedPresentationDiff(path, before, after string) string {
	var builder strings.Builder
	builder.WriteString("--- a/")
	builder.WriteString(path)
	builder.WriteString("\n+++ b/")
	builder.WriteString(path)
	builder.WriteString("\n@@\n")
	for _, line := range strings.Split(before, "\n") {
		if line != "" {
			builder.WriteString("-")
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}
	for _, line := range strings.Split(after, "\n") {
		if line != "" {
			builder.WriteString("+")
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func decodePresentationObject(raw string) map[string]any {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return nil
	}
	return value
}

func presentationString(value map[string]any, key string) string {
	result, _ := value[key].(string)
	return strings.TrimSpace(result)
}

func presentationBool(value map[string]any, key string) bool {
	result, _ := value[key].(bool)
	return result
}

func presentationInt(value map[string]any, key string) int {
	switch typed := value[key].(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	case json.Number:
		result, _ := typed.Int64()
		return int(result)
	default:
		return 0
	}
}
