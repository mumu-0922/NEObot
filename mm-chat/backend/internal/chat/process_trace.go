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

	processTraceMetadataKey = "processTrace"
	reasoningMetadataKey    = "reasoning"

	maxProcessDetailStringBytes = 2048
	maxPersistedReasoningBytes  = 1024 * 1024

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
	ID          string         `json:"id"`
	Kind        string         `json:"kind"`
	Status      string         `json:"status"`
	LabelKey    string         `json:"labelKey"`
	StartedAt   string         `json:"startedAt,omitempty"`
	CompletedAt string         `json:"completedAt,omitempty"`
	DurationMS  int64          `json:"durationMs,omitempty"`
	Detail      map[string]any `json:"detail,omitempty"`
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
	for _, step := range trace.steps {
		if step.Kind != ProcessStepKindGeneration ||
			step.Status == ProcessStepStatusFailed ||
			step.Status == ProcessStepStatusCancelled {
			return true
		}
	}
	return false
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
		"query": {}, "redactedArgs": {}, "hitCount": {}, "sourceCount": {},
		"citationMarkers": {}, "provider": {}, "mode": {}, "outcome": {},
		"failureCategory": {}, "queryRewritten": {}, "toolName": {},
		"round": {}, "selectedCount": {}, "truncated": {},
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
		ProcessStepStatusCancelled:
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
		ProcessStepStatusSkipped, ProcessStepStatusCancelled:
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
