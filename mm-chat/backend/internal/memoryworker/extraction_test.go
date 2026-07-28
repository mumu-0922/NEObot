package memoryworker

import (
	"strings"
	"testing"
)

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
