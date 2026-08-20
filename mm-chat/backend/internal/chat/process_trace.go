package chat

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	ProcessStepKindReasoning  = "reasoning"
	ProcessStepKindKnowledge  = "knowledge"
	ProcessStepKindWeb        = "web"
	ProcessStepKindTool       = "tool"
	ProcessStepKindGeneration = "generation"

	ProcessStepStatusPending          = "pending"
	ProcessStepStatusRunning          = "running"
	ProcessStepStatusAwaitingApproval = "awaiting_approval"
	ProcessStepStatusCompleted        = "completed"
	ProcessStepStatusFailed           = "failed"
	ProcessStepStatusSkipped          = "skipped"
	ProcessStepStatusCancelled        = "cancelled"
	ProcessStepStatusOutcomeUnknown   = "outcome_unknown"
	ProcessStepStatusInterrupted      = "interrupted"

	processTraceMetadataKey = "processTrace"
	reasoningMetadataKey    = "reasoning"

	maxProcessDetailStringBytes     = 2048
	maxProcessTerminalCommandBytes  = 4096
	maxProcessTerminalCWDBytes      = 1024
	maxProcessPresentationTextBytes = 64 << 10
	maxProcessPresentationItemBytes = 2048
	maxProcessPresentationItems     = 64
	maxPersistedReasoningBytes      = 1024 * 1024

	// Keep enough sanitized suffix un-emitted for a credential pattern split
	// across adjacent provider chunks to become recognizable before SSE output.
	processReasoningStreamHoldbackBytes = 64
)

var (
	processSecretAssignmentPattern = regexp.MustCompile(
		`(?i)(authorization|api[-_ ]?key|token|secret|password)\s*[:=]\s*([^\s,;]+)`,
	)
	processBearerPattern = regexp.MustCompile(
		`(?i)bearer\s+[a-z0-9._~+/-]{8,}`,
	)
	processOpenAIKeyPattern = regexp.MustCompile(
		`\bsk-[A-Za-z0-9_-]{12,}\b`,
	)
)

type ProcessStep struct {
	ID           string                   `json:"id"`
	Kind         string                   `json:"kind"`
	Status       string                   `json:"status"`
	LabelKey     string                   `json:"labelKey"`
	StartedAt    string                   `json:"startedAt,omitempty"`
	CompletedAt  string                   `json:"completedAt,omitempty"`
	DurationMS   int64                    `json:"durationMs,omitempty"`
	Detail       map[string]any           `json:"detail,omitempty"`
	Presentation *ProcessStepPresentation `json:"presentation,omitempty"`
}

type ProcessStepPresentation struct {
	Version        int                          `json:"version,omitempty"`
	Card           string                       `json:"card"`
	Title          string                       `json:"title,omitempty"`
	Summary        string                       `json:"summary,omitempty"`
	Command        string                       `json:"command,omitempty"`
	CWD            string                       `json:"cwd,omitempty"`
	Transcript     []ProcessTranscriptEntry     `json:"transcript,omitempty"`
	ExitCode       *int                         `json:"exitCode,omitempty"`
	TimedOut       bool                         `json:"timedOut,omitempty"`
	Truncated      bool                         `json:"truncated,omitempty"`
	Background     bool                         `json:"background,omitempty"`
	Provider       string                       `json:"provider,omitempty"`
	Query          string                       `json:"query,omitempty"`
	Count          int                          `json:"count,omitempty"`
	Operation      string                       `json:"operation,omitempty"`
	Path           string                       `json:"path,omitempty"`
	Content        string                       `json:"content,omitempty"`
	Diff           string                       `json:"diff,omitempty"`
	Size           int64                        `json:"size,omitempty"`
	Offset         int64                        `json:"offset,omitempty"`
	NextOffset     int64                        `json:"nextOffset,omitempty"`
	JobID          string                       `json:"jobId,omitempty"`
	JobStatus      string                       `json:"jobStatus,omitempty"`
	JobStartedAt   string                       `json:"jobStartedAt,omitempty"`
	JobCompletedAt string                       `json:"jobCompletedAt,omitempty"`
	JobDurationMS  int64                        `json:"jobDurationMs,omitempty"`
	Items          []ProcessPresentationItem    `json:"items,omitempty"`
	Approval       *ProcessApprovalPresentation `json:"approval,omitempty"`
}

type ProcessApprovalPresentation struct {
	ID                string `json:"id"`
	Revision          int64  `json:"revision"`
	Status            string `json:"status"`
	Decision          string `json:"decision,omitempty"`
	ExpiresAt         string `json:"expiresAt"`
	AllowConversation bool   `json:"allowConversation"`
}

type ProcessTranscriptEntry struct {
	Sequence int    `json:"sequence"`
	Stream   string `json:"stream"`
	Content  string `json:"content"`
}

type ProcessPresentationItem struct {
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
}

type processTrace struct {
	messageID string
	steps     []ProcessStep
	indexes   map[string]int
	counters  map[string]int
}

type processReasoningStream struct {
	raw     strings.Builder
	emitted string
}

func newProcessReasoningStream() *processReasoningStream {
	return &processReasoningStream{}
}

func (stream *processReasoningStream) append(value string) string {
	if stream == nil || value == "" {
		return ""
	}
	stream.raw.WriteString(value)
	sanitized := sanitizeProviderReasoningDelta(stream.raw.String())
	stableBytes := len(sanitized) - processReasoningStreamHoldbackBytes
	if stableBytes <= len(stream.emitted) {
		return ""
	}
	for stableBytes > len(stream.emitted) &&
		!utf8.ValidString(sanitized[:stableBytes]) {
		stableBytes--
	}
	if !strings.HasPrefix(sanitized, stream.emitted) {
		return ""
	}
	delta := sanitized[len(stream.emitted):stableBytes]
	stream.emitted = sanitized[:stableBytes]
	return delta
}

func (stream *processReasoningStream) flush() string {
	if stream == nil {
		return ""
	}
	sanitized := sanitizeProviderReasoningDelta(stream.raw.String())
	if !strings.HasPrefix(sanitized, stream.emitted) {
		return ""
	}
	delta := sanitized[len(stream.emitted):]
	stream.emitted = sanitized
	return delta
}

func (stream *processReasoningStream) String() string {
	if stream == nil {
		return ""
	}
	return sanitizeProviderReasoningDelta(stream.raw.String())
}

func newProcessTrace(messageID string) *processTrace {
	return &processTrace{
		messageID: strings.TrimSpace(messageID),
		steps:     []ProcessStep{},
		indexes:   map[string]int{},
		counters:  map[string]int{},
	}
}

func (trace *processTrace) stepID(kind string) string {
	return trace.messageID + ":" + kind + ":1"
}

func (trace *processTrace) add(step ProcessStep) ProcessStep {
	step.ID = strings.TrimSpace(step.ID)
	step.Kind = normalizeProcessStepKind(step.Kind)
	step.Status = normalizeProcessStepStatus(step.Status)
	step.LabelKey = normalizeProcessLabelKey(step.Kind, step.LabelKey)
	step.Detail = sanitizeProcessDetail(step.Detail)
	step.Presentation = sanitizeProcessStepPresentation(
		step.Kind,
		step.Detail,
		step.Presentation,
	)
	if step.ID == "" || step.Kind == "" || step.Status == "" {
		return ProcessStep{}
	}
	if index, ok := trace.indexes[step.ID]; ok {
		trace.steps[index] = step
		return cloneProcessStep(step)
	}
	trace.indexes[step.ID] = len(trace.steps)
	trace.steps = append(trace.steps, step)
	return cloneProcessStep(step)
}

func (trace *processTrace) start(
	kind string,
	labelKey string,
	startedAt time.Time,
	detail map[string]any,
) ProcessStep {
	return trace.startWithID(
		trace.stepID(kind),
		kind,
		labelKey,
		startedAt,
		detail,
	)
}

func (trace *processTrace) startNext(
	kind string,
	labelKey string,
	startedAt time.Time,
	detail map[string]any,
) ProcessStep {
	var id string
	for {
		trace.counters[kind]++
		id = trace.messageID + ":" + kind + ":" + strconv.Itoa(trace.counters[kind])
		if _, exists := trace.indexes[id]; !exists {
			break
		}
	}
	return trace.startWithID(
		id,
		kind,
		labelKey,
		startedAt,
		detail,
	)
}

func (trace *processTrace) startWithID(
	id string,
	kind string,
	labelKey string,
	startedAt time.Time,
	detail map[string]any,
) ProcessStep {
	return trace.add(ProcessStep{
		ID:        id,
		Kind:      kind,
		Status:    ProcessStepStatusRunning,
		LabelKey:  labelKey,
		StartedAt: formatTime(startedAt),
		Detail:    detail,
	})
}

func (trace *processTrace) transition(
	kind string,
	status string,
	completedAt time.Time,
	detail map[string]any,
) (ProcessStep, bool) {
	return trace.transitionID(
		trace.stepID(kind),
		status,
		completedAt,
		detail,
	)
}

func (trace *processTrace) transitionID(
	id string,
	status string,
	completedAt time.Time,
	detail map[string]any,
) (ProcessStep, bool) {
	index, ok := trace.indexes[id]
	if !ok {
		return ProcessStep{}, false
	}
	step := trace.steps[index]
	if isTerminalProcessStepStatus(step.Status) {
		return cloneProcessStep(step), false
	}
	status = normalizeProcessStepStatus(status)
	if status == "" {
		return ProcessStep{}, false
	}
	step.Status = status
	if isTerminalProcessStepStatus(status) {
		step.CompletedAt = formatTime(completedAt)
		step.DurationMS = processStepDurationMillis(step.StartedAt, completedAt)
	}
	if detail != nil {
		step.Detail = sanitizeProcessDetail(detail)
	}
	trace.steps[index] = step
	return cloneProcessStep(step), true
}

func (trace *processTrace) get(kind string) (ProcessStep, bool) {
	index, ok := trace.indexes[trace.stepID(kind)]
	if !ok {
		return ProcessStep{}, false
	}
	return cloneProcessStep(trace.steps[index]), true
}

func (trace *processTrace) snapshot() []ProcessStep {
	steps := make([]ProcessStep, 0, len(trace.steps))
	for _, step := range trace.steps {
		steps = append(steps, cloneProcessStep(step))
	}
	return steps
}

func (trace *processTrace) shouldPersist(reasoning string) bool {
	if strings.TrimSpace(reasoning) != "" {
		return true
	}
	return len(trace.steps) > 0
}

func withProcessTraceMessageMetadata(
	base map[string]any,
	reasoning string,
	trace *processTrace,
) map[string]any {
	metadata := ensureObject(base)
	delete(metadata, processTraceMetadataKey)
	delete(metadata, reasoningMetadataKey)
	if trace == nil {
		return metadata
	}

	reasoning, reasoningTruncated := sanitizePersistedReasoning(reasoning)
	if !trace.shouldPersist(reasoning) {
		return metadata
	}
	if reasoning != "" {
		metadata[reasoningMetadataKey] = reasoning
	}
	steps := trace.snapshot()
	if reasoningTruncated {
		for index := range steps {
			if steps[index].Kind != ProcessStepKindReasoning {
				continue
			}
			detail := cloneProcessDetail(steps[index].Detail)
			if detail == nil {
				detail = map[string]any{}
			}
			detail["truncated"] = true
			steps[index].Detail = detail
		}
	}
	metadata[processTraceMetadataKey] = steps
	return metadata
}

func sanitizeProviderReasoningDelta(value string) string {
	return redactProcessSecrets(value)
}

func sanitizePersistedReasoning(value string) (string, bool) {
	value = redactProcessSecrets(value)
	if len(value) <= maxPersistedReasoningBytes {
		return value, false
	}
	return truncateProcessUTF8(value, maxPersistedReasoningBytes), true
}

func sanitizeProcessDetail(detail map[string]any) map[string]any {
	if len(detail) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		"hitCount": {}, "sourceCount": {}, "citationMarkers": {},
		"provider": {}, "mode": {}, "outcome": {},
		"failureCategory": {}, "queryRewritten": {}, "rerankStatus": {},
		"toolName": {}, "server": {}, "serverName": {}, "classification": {}, "callStatus": {},
		"argumentSummary": {}, "round": {}, "selectedCount": {}, "truncated": {},
		"durability": {},
	}
	sanitized := make(map[string]any, len(detail))
	for key, value := range detail {
		if _, ok := allowed[key]; !ok {
			continue
		}
		if normalized, ok := sanitizeProcessDetailValue(value); ok {
			sanitized[key] = normalized
		}
	}
	if len(sanitized) == 0 {
		return nil
	}
	return sanitized
}

func sanitizeProcessDetailValue(value any) (any, bool) {
	switch typed := value.(type) {
	case string:
		return truncateProcessUTF8(redactProcessSecrets(typed), maxProcessDetailStringBytes), true
	case bool:
		return typed, true
	case int:
		return typed, typed >= 0
	case int64:
		return typed, typed >= 0
	case float64:
		return typed, typed >= 0
	case []string:
		values := make([]string, 0, min(len(typed), 32))
		for _, item := range typed {
			if len(values) == 32 {
				break
			}
			values = append(values, truncateProcessUTF8(redactProcessSecrets(item), 256))
		}
		return values, true
	default:
		return nil, false
	}
}

func sanitizeProcessStepPresentation(
	kind string,
	detail map[string]any,
	presentation *ProcessStepPresentation,
) *ProcessStepPresentation {
	if presentation == nil {
		return nil
	}
	version := presentation.Version
	if version == 0 {
		version = 1
	}
	if version != 1 {
		return nil
	}
	toolName := processDetailString(detail, "toolName")
	mode := processDetailString(detail, "mode")
	card := strings.TrimSpace(presentation.Card)
	valid := false
	switch card {
	case "terminal":
		valid = kind == ProcessStepKindTool && toolName == localTerminalToolName && mode == "local_direct"
	case "search":
		valid = kind == ProcessStepKindWeb || kind == ProcessStepKindKnowledge ||
			toolName == "search_memory"
	case "file":
		valid = kind == ProcessStepKindTool && mode == "local_direct" &&
			(toolName == localFileReadToolName || toolName == localFileWriteToolName ||
				toolName == localFileEditToolName || toolName == localFileSearchToolName ||
				toolName == localPublishFileToolName)
	case "job":
		valid = kind == ProcessStepKindTool && mode == "local_direct" &&
			((toolName == localTerminalToolName && presentation.Background) ||
				toolName == localJobListToolName || toolName == localJobOutputToolName ||
				toolName == localJobKillToolName)
	case "skill":
		valid = kind == ProcessStepKindTool && mode == "local_direct" && toolName == localSkillToolName
	case "goal":
		valid = kind == ProcessStepKindTool && mode == "goal"
	case "browser", "mcp":
		valid = kind == ProcessStepKindTool && mode == "mcp"
	}
	if !valid {
		return nil
	}

	result := &ProcessStepPresentation{
		Version: version, Card: card,
		Title:     sanitizePresentationText(presentation.Title, maxProcessPresentationItemBytes),
		Summary:   sanitizePresentationText(presentation.Summary, maxProcessPresentationItemBytes),
		Provider:  sanitizePresentationText(presentation.Provider, 256),
		Query:     sanitizePresentationText(presentation.Query, maxProcessPresentationItemBytes),
		Count:     max(presentation.Count, 0),
		Operation: sanitizePresentationText(presentation.Operation, 128),
		Path:      sanitizePresentationText(presentation.Path, 4096),
		Content:   sanitizePresentationText(presentation.Content, maxProcessPresentationTextBytes),
		Diff:      sanitizePresentationText(presentation.Diff, maxProcessPresentationTextBytes),
		Size:      max(presentation.Size, 0), Offset: max(presentation.Offset, 0),
		NextOffset:     max(presentation.NextOffset, 0),
		JobID:          sanitizePresentationText(presentation.JobID, 128),
		JobStatus:      sanitizeProcessJobStatus(presentation.JobStatus),
		JobStartedAt:   sanitizeProcessJobTimestamp(presentation.JobStartedAt),
		JobCompletedAt: sanitizeProcessJobTimestamp(presentation.JobCompletedAt),
		JobDurationMS:  max(presentation.JobDurationMS, 0),
		TimedOut:       presentation.TimedOut, Truncated: presentation.Truncated,
		Background: presentation.Background,
		Items:      sanitizePresentationItems(presentation.Items),
	}
	if card == "job" {
		result.Command = sanitizePresentationText(
			presentation.Command, maxProcessTerminalCommandBytes,
		)
		result.CWD = sanitizePresentationText(
			presentation.CWD, maxProcessTerminalCWDBytes,
		)
		if presentation.ExitCode != nil {
			if *presentation.ExitCode < -1 || *presentation.ExitCode > 255 {
				return nil
			}
			exitCode := *presentation.ExitCode
			result.ExitCode = &exitCode
		}
		result.Transcript, result.Truncated = sanitizeProcessTranscript(
			presentation.Transcript, result.Truncated,
		)
		return result
	}
	if card != "terminal" {
		return result
	}
	result.Command = sanitizePresentationText(presentation.Command, maxProcessTerminalCommandBytes)
	if result.Command == "" {
		return nil
	}
	result.CWD = sanitizePresentationText(presentation.CWD, maxProcessTerminalCWDBytes)
	result.Approval = sanitizeProcessApprovalPresentation(presentation.Approval)
	var exitCode *int
	if presentation.ExitCode != nil {
		if *presentation.ExitCode < -1 || *presentation.ExitCode > 255 {
			return nil
		}
		value := *presentation.ExitCode
		exitCode = &value
	}
	result.ExitCode = exitCode
	result.Transcript, result.Truncated = sanitizeProcessTranscript(
		presentation.Transcript, result.Truncated,
	)
	return result
}

func sanitizeProcessJobStatus(value string) string {
	switch strings.TrimSpace(value) {
	case "running", "stopping", "completed", "killed", "failed", "interrupted", "unknown":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func sanitizeProcessJobTimestamp(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
		return ""
	}
	return value
}

func sanitizeProcessApprovalPresentation(
	approval *ProcessApprovalPresentation,
) *ProcessApprovalPresentation {
	if approval == nil || !isUUID(strings.TrimSpace(approval.ID)) ||
		approval.Revision < 1 {
		return nil
	}
	status := strings.TrimSpace(approval.Status)
	switch status {
	case ChatAgentApprovalPending, ChatAgentApprovalAllowed,
		ChatAgentApprovalDenied, ChatAgentApprovalExpired:
	default:
		return nil
	}
	decision := strings.TrimSpace(approval.Decision)
	if decision != "" {
		switch decision {
		case ChatAgentApprovalAllowOnce, ChatAgentApprovalAllowConversation,
			ChatAgentApprovalDeny, chatAgentApprovalExpire, chatAgentApprovalRestartDeny:
		default:
			return nil
		}
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(approval.ExpiresAt))
	if err != nil {
		return nil
	}
	return &ProcessApprovalPresentation{
		ID: approval.ID, Revision: approval.Revision, Status: status,
		Decision: decision, ExpiresAt: formatTime(expiresAt),
		AllowConversation: approval.AllowConversation,
	}
}

func sanitizePresentationText(value string, limit int) string {
	value = redactProcessSecrets(strings.TrimSpace(value))
	value = strings.Map(func(character rune) rune {
		if character == '\n' || character == '\r' || character == '\t' || character >= 0x20 {
			return character
		}
		return -1
	}, value)
	return truncateProcessUTF8(value, limit)
}

func sanitizePresentationItems(items []ProcessPresentationItem) []ProcessPresentationItem {
	if len(items) == 0 {
		return nil
	}
	result := make([]ProcessPresentationItem, 0, min(len(items), maxProcessPresentationItems))
	for _, item := range items {
		if len(result) == maxProcessPresentationItems {
			break
		}
		label := sanitizePresentationText(item.Label, 1024)
		if label == "" {
			continue
		}
		result = append(result, ProcessPresentationItem{
			Label:  label,
			Detail: sanitizePresentationText(item.Detail, maxProcessPresentationItemBytes),
		})
	}
	return result
}

func sanitizeProcessTranscript(
	entries []ProcessTranscriptEntry,
	alreadyTruncated bool,
) ([]ProcessTranscriptEntry, bool) {
	sanitized := make([]ProcessTranscriptEntry, 0, len(entries))
	total := 0
	for _, entry := range entries {
		if entry.Stream != "stdout" && entry.Stream != "stderr" {
			continue
		}
		content := sanitizePresentationText(entry.Content, 8<<20)
		if content == "" {
			continue
		}
		sanitized = append(sanitized, ProcessTranscriptEntry{
			Sequence: len(sanitized) + 1, Stream: entry.Stream, Content: content,
		})
		total += len(content)
	}
	if total <= maxProcessPresentationTextBytes {
		return sanitized, alreadyTruncated
	}
	head := transcriptPrefix(sanitized, maxProcessPresentationTextBytes/2)
	tail := transcriptSuffix(sanitized, maxProcessPresentationTextBytes/2)
	result := append(head, tail...)
	for index := range result {
		result[index].Sequence = index + 1
	}
	return result, true
}

func transcriptPrefix(entries []ProcessTranscriptEntry, budget int) []ProcessTranscriptEntry {
	result := make([]ProcessTranscriptEntry, 0, len(entries))
	for _, entry := range entries {
		if budget <= 0 {
			break
		}
		content := truncateProcessUTF8(entry.Content, budget)
		if content != "" {
			entry.Content = content
			result = append(result, entry)
			budget -= len(content)
		}
	}
	return result
}

func transcriptSuffix(entries []ProcessTranscriptEntry, budget int) []ProcessTranscriptEntry {
	reversed := make([]ProcessTranscriptEntry, 0, len(entries))
	for index := len(entries) - 1; index >= 0 && budget > 0; index-- {
		entry := entries[index]
		content := entry.Content
		if len(content) > budget {
			content = content[len(content)-budget:]
			for !utf8.ValidString(content) && len(content) > 0 {
				content = content[1:]
			}
		}
		if content != "" {
			entry.Content = content
			reversed = append(reversed, entry)
			budget -= len(content)
		}
	}
	result := make([]ProcessTranscriptEntry, len(reversed))
	for index := range reversed {
		result[len(reversed)-1-index] = reversed[index]
	}
	return result
}

func processDetailString(detail map[string]any, key string) string {
	value, _ := detail[key].(string)
	return strings.TrimSpace(value)
}

func normalizeProcessStepKind(value string) string {
	switch strings.TrimSpace(value) {
	case ProcessStepKindReasoning, ProcessStepKindKnowledge, ProcessStepKindWeb,
		ProcessStepKindTool, ProcessStepKindGeneration:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func normalizeProcessStepStatus(value string) string {
	switch strings.TrimSpace(value) {
	case ProcessStepStatusPending, ProcessStepStatusRunning,
		ProcessStepStatusAwaitingApproval, ProcessStepStatusCompleted,
		ProcessStepStatusFailed, ProcessStepStatusSkipped,
		ProcessStepStatusCancelled, ProcessStepStatusOutcomeUnknown,
		ProcessStepStatusInterrupted:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func normalizeProcessLabelKey(kind string, value string) string {
	expected := "process." + kind
	if strings.TrimSpace(value) != expected {
		return expected
	}
	return expected
}

func isTerminalProcessStepStatus(status string) bool {
	switch status {
	case ProcessStepStatusCompleted, ProcessStepStatusFailed,
		ProcessStepStatusSkipped, ProcessStepStatusCancelled,
		ProcessStepStatusOutcomeUnknown, ProcessStepStatusInterrupted:
		return true
	default:
		return false
	}
}

func processStepDurationMillis(startedAt string, completedAt time.Time) int64 {
	started, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil || completedAt.Before(started) {
		return 0
	}
	duration := completedAt.Sub(started).Milliseconds()
	if duration > maxFusionStageDurationMillis {
		return maxFusionStageDurationMillis
	}
	return duration
}

func cloneProcessStep(step ProcessStep) ProcessStep {
	step.Detail = cloneProcessDetail(step.Detail)
	if step.Presentation != nil {
		presentation := *step.Presentation
		if step.Presentation.ExitCode != nil {
			exitCode := *step.Presentation.ExitCode
			presentation.ExitCode = &exitCode
		}
		presentation.Transcript = append([]ProcessTranscriptEntry(nil), step.Presentation.Transcript...)
		presentation.Items = append([]ProcessPresentationItem(nil), step.Presentation.Items...)
		if step.Presentation.Approval != nil {
			approval := *step.Presentation.Approval
			presentation.Approval = &approval
		}
		step.Presentation = &presentation
	}
	return step
}

func cloneProcessDetail(detail map[string]any) map[string]any {
	if len(detail) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(detail))
	for key, value := range detail {
		switch typed := value.(type) {
		case []string:
			cloned[key] = append([]string(nil), typed...)
		default:
			cloned[key] = typed
		}
	}
	return cloned
}

func redactProcessSecrets(value string) string {
	if value == "" {
		return ""
	}
	value = processBearerPattern.ReplaceAllString(value, "Bearer [REDACTED]")
	value = processSecretAssignmentPattern.ReplaceAllString(value, "$1=[REDACTED]")
	return processOpenAIKeyPattern.ReplaceAllString(value, "[REDACTED]")
}

func truncateProcessUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return value
}
