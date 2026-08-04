package memoryworker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/chat"
)

func TestMemoryExtractionToolDefinitionRequiresExactCandidateShape(t *testing.T) {
	repository := newWorkerTestRepository()
	messages, visible := prepareProviderMessages(repository.job, repository.capture)
	if !visible {
		t.Fatal("source message is not visible")
	}
	tool := memoryExtractionToolDefinition(repository.job.SourceMessageID, messages)
	if tool.Type != "function" || tool.Function.Name != memoryExtractionToolName ||
		!tool.Function.Strict {
		t.Fatalf("tool = %#v", tool)
	}
	parameters := tool.Function.Parameters
	if parameters["additionalProperties"] != false {
		t.Fatalf("parameters = %#v", parameters)
	}
	properties, ok := parameters["properties"].(map[string]any)
	if !ok || len(properties) != 1 {
		t.Fatalf("properties = %#v", parameters["properties"])
	}
	memories, ok := properties["memories"].(map[string]any)
	if !ok || memories["maxItems"] != 5 {
		t.Fatalf("memories = %#v", properties["memories"])
	}
	items, ok := memories["items"].(map[string]any)
	if !ok || items["additionalProperties"] != false {
		t.Fatalf("items = %#v", memories["items"])
	}
	required, ok := items["required"].([]string)
	if !ok || len(required) != len(rawCaptureCandidateKeys) {
		t.Fatalf("required = %#v", items["required"])
	}
	candidateProperties, ok := items["properties"].(map[string]any)
	if !ok {
		t.Fatalf("candidate properties = %#v", items["properties"])
	}
	authority, ok := candidateProperties["authorityUserMessageIds"].(map[string]any)
	if !ok || authority["maxItems"] != 1 {
		t.Fatalf("authority evidence schema = %#v", authority)
	}
	authorityItems, ok := authority["items"].(map[string]any)
	if !ok || !stringSliceContains(authorityItems["enum"], testMessageID) {
		t.Fatalf("authority evidence IDs = %#v", authorityItems)
	}
	contextIDs, ok := candidateProperties["contextMessageIds"].(map[string]any)
	if !ok || contextIDs["maxItems"] != 1 {
		t.Fatalf("assistant context schema = %#v", contextIDs)
	}
	contextItems, ok := contextIDs["items"].(map[string]any)
	if !ok || !stringSliceContains(contextItems["enum"], testAssistantID) ||
		stringSliceContains(contextItems["enum"], testMessageID) {
		t.Fatalf("assistant context IDs = %#v", contextItems)
	}
}

func stringSliceContains(value any, expected string) bool {
	values, ok := value.([]string)
	if !ok {
		return false
	}
	for _, candidate := range values {
		if candidate == expected {
			return true
		}
	}
	return false
}

func TestMemoryExtractionToolDefinitionDisallowsUnavailableAssistantEvidence(t *testing.T) {
	tool := memoryExtractionToolDefinition(testMessageID, []providerMessage{{
		ID: testMessageID, Role: "user", Content: "Remember this",
	}})
	parameters := tool.Function.Parameters
	properties := parameters["properties"].(map[string]any)
	memories := properties["memories"].(map[string]any)
	items := memories["items"].(map[string]any)
	candidateProperties := items["properties"].(map[string]any)
	contextIDs := candidateProperties["contextMessageIds"].(map[string]any)
	if contextIDs["maxItems"] != 0 {
		t.Fatalf("assistant context schema = %#v", contextIDs)
	}
	confirmation := candidateProperties["confirmationKind"].(map[string]any)
	values, ok := confirmation["enum"].([]string)
	if !ok || len(values) != 1 || values[0] != "explicit_user" {
		t.Fatalf("confirmation schema = %#v", confirmation)
	}
}

func TestMemoryDecisionToolDefinitionRequiresExactDecisionShape(t *testing.T) {
	tool := memoryDecisionToolDefinition()
	if tool.Type != "function" || tool.Function.Name != memoryDecisionToolName ||
		!tool.Function.Strict {
		t.Fatalf("tool = %#v", tool)
	}
	parameters := tool.Function.Parameters
	if parameters["additionalProperties"] != false {
		t.Fatalf("parameters = %#v", parameters)
	}
	properties, ok := parameters["properties"].(map[string]any)
	if !ok || len(properties) != 1 {
		t.Fatalf("properties = %#v", parameters["properties"])
	}
	decisions, ok := properties["decisions"].(map[string]any)
	if !ok || decisions["maxItems"] != 5 {
		t.Fatalf("decisions = %#v", properties["decisions"])
	}
	items, ok := decisions["items"].(map[string]any)
	if !ok || items["additionalProperties"] != false {
		t.Fatalf("items = %#v", decisions["items"])
	}
	required, ok := items["required"].([]string)
	if !ok || len(required) != 3 {
		t.Fatalf("required = %#v", items["required"])
	}
}

func TestExtractCandidatesUsesOneRequiredToolCall(t *testing.T) {
	repository := newWorkerTestRepository()
	provider := &workerTestProvider{output: validCandidateOutput()}
	candidates, err := extractCandidates(
		context.Background(), provider,
		chat.ModelRef{ProviderID: testProviderID, ModelID: "fixture-model"},
		repository.job, repository.capture,
	)
	if err != nil || len(candidates) != 1 ||
		candidates[0].Content != "Use concise answers" {
		t.Fatalf("candidates=%#v err=%v", candidates, err)
	}
	if provider.roundRequest.ToolChoice != chat.ProviderToolChoiceRequired ||
		!provider.roundRequest.ProviderRequest.DisableThinking {
		t.Fatalf("request = %#v", provider.roundRequest)
	}
}

func TestExtractCandidatesRejectsNonCanonicalToolRound(t *testing.T) {
	validCall := func() *chat.ProviderToolCall {
		return &chat.ProviderToolCall{
			ID: "call-1", Name: memoryExtractionToolName,
			Arguments: validCandidateOutput(),
		}
	}
	tests := []struct {
		name    string
		round   []chat.ProviderEvent
		syncErr error
	}{
		{name: "missing call", round: []chat.ProviderEvent{}},
		{name: "prose without call", round: []chat.ProviderEvent{{
			Type: chat.ProviderEventDelta, Delta: "ignored prose",
		}}},
		{name: "reasoning without call", round: []chat.ProviderEvent{{
			Type: chat.ProviderEventReasoningDelta, ReasoningDelta: "ignored reasoning",
		}}},
		{name: "missing id", round: []chat.ProviderEvent{{
			Type:     chat.ProviderEventToolCallCompleted,
			ToolCall: &chat.ProviderToolCall{Name: memoryExtractionToolName, Arguments: validCandidateOutput()},
		}}},
		{name: "synthetic id", round: []chat.ProviderEvent{{
			Type: chat.ProviderEventToolCallCompleted,
			ToolCall: &chat.ProviderToolCall{
				ID: "call_0_0", SyntheticID: true,
				Name: memoryExtractionToolName, Arguments: validCandidateOutput(),
			},
		}}},
		{name: "missing completed call", round: []chat.ProviderEvent{{
			Type: chat.ProviderEventToolCallCompleted,
		}}},
		{name: "empty arguments", round: []chat.ProviderEvent{{
			Type: chat.ProviderEventToolCallCompleted,
			ToolCall: &chat.ProviderToolCall{
				ID: "call-1", Name: memoryExtractionToolName,
			},
		}}},
		{name: "oversized arguments", round: []chat.ProviderEvent{{
			Type: chat.ProviderEventToolCallCompleted,
			ToolCall: &chat.ProviderToolCall{
				ID: "call-1", Name: memoryExtractionToolName,
				Arguments: strings.Repeat("x", memoryExtractionOutputBytes+1),
			},
		}}},
		{name: "wrong name", round: []chat.ProviderEvent{{
			Type:     chat.ProviderEventToolCallCompleted,
			ToolCall: &chat.ProviderToolCall{ID: "call-1", Name: "other", Arguments: validCandidateOutput()},
		}}},
		{name: "failed call", round: []chat.ProviderEvent{{
			Type: chat.ProviderEventToolCallCompleted,
			ToolCall: &chat.ProviderToolCall{
				ID: "call-1", Name: memoryExtractionToolName,
				Arguments: validCandidateOutput(), FailureCategory: "invalid_arguments",
			},
		}}},
		{name: "duplicate", round: []chat.ProviderEvent{
			{Type: chat.ProviderEventToolCallCompleted, ToolCall: validCall()},
			{Type: chat.ProviderEventToolCallCompleted, ToolCall: validCall()},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := newWorkerTestRepository()
			provider := &workerTestProvider{rounds: [][]chat.ProviderEvent{test.round}, err: test.syncErr}
			_, err := extractCandidates(
				context.Background(), provider,
				chat.ModelRef{ProviderID: testProviderID, ModelID: "fixture-model"},
				repository.job, repository.capture,
			)
			if !errors.Is(err, errExtractionInvalid) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestExtractCandidatesIgnoresNonAuthoritativeTextAroundValidToolCall(t *testing.T) {
	repository := newWorkerTestRepository()
	provider := &workerTestProvider{rounds: [][]chat.ProviderEvent{{
		{Type: chat.ProviderEventReasoningDelta, ReasoningDelta: "private reasoning"},
		{Type: chat.ProviderEventDelta, Delta: "non-authoritative prose"},
		{Type: chat.ProviderEventToolCallCompleted, ToolCall: &chat.ProviderToolCall{
			ID: "call-1", Name: memoryExtractionToolName,
			Arguments: validCandidateOutput(),
		}},
	}}}
	candidates, err := extractCandidates(
		context.Background(), provider,
		chat.ModelRef{ProviderID: testProviderID, ModelID: "fixture-model"},
		repository.job, repository.capture,
	)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("candidates=%#v err=%v", candidates, err)
	}
}

func TestStrictDecodeProviderJSONRejectsUnknownDuplicateAndTrailingFields(t *testing.T) {
	for _, value := range []string{
		`{"memories":[],"unknown":true}`,
		`{"memories":[],"memories":[]}`,
		`{"memories":[]} {"memories":[]}`,
	} {
		var response struct {
			Memories []rawCaptureCandidate `json:"memories"`
		}
		if err := strictDecodeProviderJSON([]byte(value), &response); err == nil {
			t.Fatalf("strict decoder accepted %s", value)
		}
	}
}

func TestStrictDecodeProviderJSONRequiresEveryCandidateAndDecisionField(t *testing.T) {
	missingCandidateField := `{"memories":[{` +
		`"type":"preference","content":"Use concise answers",` +
		`"importance":3,"confidence":0.9,"tags":[],"subjectKey":null,` +
		`"factKey":null,"sensitivity":"normal","authorityUserMessageIds":[],` +
		`"contextMessageIds":[],"confirmationKind":"explicit_user",` +
		`"proposedScopeType":"global","scopeConfidence":0.9,` +
		`"temporalBasis":"none","validFrom":null,"validTo":null}]}`
	var candidates struct {
		Memories []rawCaptureCandidate `json:"memories"`
	}
	if err := strictDecodeProviderJSON([]byte(missingCandidateField), &candidates); err == nil {
		t.Fatal("strict decoder accepted a candidate with a missing factExpiresAt field")
	}

	var decisions struct {
		Decisions []rawDecision `json:"decisions"`
	}
	if err := strictDecodeProviderJSON(
		[]byte(`{"decisions":[{"ordinal":1,"action":"ADD"}]}`),
		&decisions,
	); err == nil {
		t.Fatal("strict decoder accepted a decision with a missing targetMemoryIds field")
	}
}

func TestPrepareProviderMessagesRedactsSecretAndDisabledSensitiveSegments(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.capture.Messages[0].Content = "My API key is sk-abcdefghijk. I have diabetes. Use concise answers."
	messages, visible := prepareProviderMessages(repository.job, repository.capture)
	if !visible || len(messages) == 0 {
		t.Fatalf("messages=%#v visible=%t", messages, visible)
	}
	joined := ""
	for _, message := range messages {
		joined += message.Content
	}
	if strings.Contains(joined, "sk-abcdefghijk") || strings.Contains(joined, "diabetes") ||
		!strings.Contains(joined, "Use concise answers") {
		t.Fatalf("redacted provider context = %q", joined)
	}
}

func TestRedactProviderTextRemovesGenericEnglishAndChineseSecretAssignments(t *testing.T) {
	input := "My credential is abcdefgh. secret=ijklmnop. 密钥：qrstuvwx。Keep concise."
	redacted := redactProviderText(input, true)
	for _, forbidden := range []string{"abcdefgh", "ijklmnop", "qrstuvwx"} {
		if strings.Contains(redacted, forbidden) {
			t.Fatalf("redacted provider text retained %q: %q", forbidden, redacted)
		}
	}
	if !strings.Contains(redacted, "Keep concise") {
		t.Fatalf("redacted provider text removed safe segment: %q", redacted)
	}
}

func TestBuildCaptureProposalsNeverCarriesSecretPlaintext(t *testing.T) {
	repository := newWorkerTestRepository()
	importance := 5
	confidence := 0.99
	scopeConfidence := 0.99
	candidates := []rawCaptureCandidate{{
		Type: "fact", Content: "My password is hunter2", Importance: &importance,
		Confidence: &confidence, Tags: []string{"credential"}, Sensitivity: "normal",
		AuthorityUserMessageIDs: []string{testMessageID}, ContextMessageIDs: []string{},
		ConfirmationKind: "explicit_user", ProposedScopeType: "global",
		ScopeConfidence: &scopeConfidence, TemporalBasis: "none",
	}}
	ordinal := 1
	proposals, err := buildCaptureProposals(
		repository.job,
		repository.capture,
		candidates,
		map[int]rawDecision{1: {Ordinal: &ordinal, Action: "ADD", TargetMemoryIDs: []string{}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals) != 1 || proposals[0].Sensitivity != "secret" ||
		proposals[0].Content != nil || proposals[0].NormalizedContent != nil ||
		len(proposals[0].Tags) != 0 || proposals[0].ProposedAction != "REJECT" {
		t.Fatalf("secret proposal = %#v", proposals)
	}
}

func TestProposalValidationFailureCategoryIsBounded(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{errors.New("memory candidate user authority is invalid"),
			"PROPOSAL_USER_AUTHORITY_INVALID"},
		{errors.New("timeless memory candidate carries temporal values"),
			"PROPOSAL_TEMPORAL_INVALID"},
		{errors.New("memory candidate decision target is invalid"),
			"PROPOSAL_DECISION_INVALID"},
		{errors.New("unclassified internal validation"),
			"PROPOSAL_VALIDATION_INVALID"},
	}
	for _, test := range tests {
		if got := proposalValidationFailureCategory(test.err); got != test.want {
			t.Fatalf("category = %q, want %q", got, test.want)
		}
	}
}
