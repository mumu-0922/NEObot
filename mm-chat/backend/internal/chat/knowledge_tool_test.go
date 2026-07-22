package chat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/knowledge"
)

func TestSearchKnowledgeToolDefinitionKeepsCollectionAuthorityOnServer(t *testing.T) {
	tool := searchKnowledgeToolDefinition()
	encoded, err := json.Marshal(tool.Function.Parameters)
	if err != nil {
		t.Fatal(err)
	}
	if tool.Function.Name != searchKnowledgeToolName ||
		strings.Contains(string(encoded), "collection") {
		t.Fatalf("tool definition leaks collection authority = %#v", tool)
	}
}

func TestExecuteKnowledgeToolUsesSelectedCollectionsAndMintsEvidence(t *testing.T) {
	candidate := &fakeRAGCandidateSource{refs: []knowledge.EvidenceCandidateReference{
		validRAGCandidate(),
	}}
	hydrator := &fakeRAGHydrator{evidence: []knowledge.HydratedEvidence{
		validHydratedEvidence(),
	}}
	gate := &fakeRAGAnswerGovernanceGate{authority: RAGAnswerAuthority{
		Processor: "openai_compatible", ModelID: "fixture", CollectionCount: 1,
	}}
	runtime := &knowledgeToolRuntime{
		Assembler:             NewRAGAnswerAssembler(candidate, hydrator),
		AnswerGate:            gate,
		ActorUserID:           "user-1",
		SessionID:             "session-1",
		ConversationID:        testConversationID,
		SelectedCollectionIDs: []string{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"},
		GovernanceModelRef: ModelRef{
			ProviderID: "openai_compatible", ModelID: "fixture",
		},
	}

	decision := executeKnowledgeTool(context.Background(), runtime, "standalone query")
	if !decision.ReadyForAnswer() || len(decision.Citations) != 1 ||
		decision.Citations[0].Marker != "[K1]" {
		t.Fatalf("decision = %#v", decision)
	}
	if len(candidate.queries) != 1 ||
		candidate.queries[0].QueryText != "standalone query" ||
		len(candidate.queries[0].CollectionIDs) != 1 ||
		candidate.queries[0].CollectionIDs[0] != runtime.SelectedCollectionIDs[0] {
		t.Fatalf("candidate inputs = %#v", candidate.queries)
	}
	if gate.input.ModelRef != runtime.GovernanceModelRef ||
		len(gate.input.SelectedCollectionIDs) != 1 {
		t.Fatalf("governance input = %#v", gate.input)
	}
	result := knowledgeToolSuccessResult(decision)
	if !strings.Contains(result, `"marker":"[K1]"`) ||
		!strings.Contains(result, `"ok":true`) ||
		strings.Contains(result, decision.Citations[0].ContentHash) {
		t.Fatalf("tool result = %s", result)
	}
}

func TestExecuteKnowledgeToolTreatsMissAsSuccessfulEmptyResult(t *testing.T) {
	decision := executeKnowledgeTool(context.Background(), &knowledgeToolRuntime{
		Assembler: NewRAGAnswerAssembler(
			&fakeRAGCandidateSource{},
			&fakeRAGHydrator{},
		),
		AnswerGate:            &fakeRAGAnswerGovernanceGate{},
		ActorUserID:           "user-1",
		SessionID:             "session-1",
		ConversationID:        testConversationID,
		SelectedCollectionIDs: []string{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"},
		GovernanceModelRef:    ModelRef{ProviderID: "fixture", ModelID: "fixture"},
	}, "unrelated")

	if decision.Outcome != "no_evidence" || decision.ReadyForAnswer() {
		t.Fatalf("decision = %#v", decision)
	}
	result := knowledgeToolSuccessResult(decision)
	if !strings.Contains(result, `"ok":true`) ||
		!strings.Contains(result, `"sources":[]`) ||
		strings.Contains(result, "[K1]") {
		t.Fatalf("empty tool result = %s", result)
	}
}

func TestExecuteKnowledgeToolFailsClosedBeforeReturningPrivateEvidence(t *testing.T) {
	runtime := &knowledgeToolRuntime{
		Assembler: NewRAGAnswerAssembler(
			&fakeRAGCandidateSource{refs: []knowledge.EvidenceCandidateReference{validRAGCandidate()}},
			&fakeRAGHydrator{evidence: []knowledge.HydratedEvidence{validHydratedEvidence()}},
		),
		AnswerGate: &fakeRAGAnswerGovernanceGate{
			err: ErrRAGAnswerGovernanceRequired,
		},
		ActorUserID:           "user-1",
		SessionID:             "session-1",
		ConversationID:        testConversationID,
		SelectedCollectionIDs: []string{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"},
		GovernanceModelRef:    ModelRef{ProviderID: "fixture", ModelID: "fixture"},
	}
	decision := executeKnowledgeTool(context.Background(), runtime, "private fixture")
	if decision.Outcome != "answer_governance_required" ||
		len(decision.Citations) != 0 || len(decision.Evidence) != 0 {
		t.Fatalf("decision = %#v", decision)
	}
	result := knowledgeToolFailureResult(knowledgeToolFailureCategory(decision))
	if strings.Contains(result, validHydratedEvidence().SourceText) ||
		!strings.Contains(result, `"ok":false`) {
		t.Fatalf("failure result leaked evidence = %s", result)
	}
}

func TestMergeKnowledgeToolDecisionKeepsStableMarkersAcrossCalls(t *testing.T) {
	firstEvidence := validHydratedEvidence()
	firstCitations, err := mintRAGCitations([]knowledge.HydratedEvidence{firstEvidence})
	if err != nil {
		t.Fatal(err)
	}
	authority := &RAGAnswerAuthority{Processor: "fixture", ModelID: "fixture", CollectionCount: 1}
	first := autoRAGDecision{
		Outcome: "evidence_ready", Evidence: []knowledge.HydratedEvidence{firstEvidence},
		Citations: firstCitations, Authority: authority,
	}
	merged := mergeKnowledgeToolDecision(autoRAGDecision{}, first)

	secondEvidence := validHydratedEvidence()
	secondEvidence.ChildChunkID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	secondEvidence.SourceSpanHash = strings.Repeat("c", 64)
	secondEvidence.ContentHash = strings.Repeat("d", 64)
	secondCitations, err := mintRAGCitations([]knowledge.HydratedEvidence{
		firstEvidence,
		secondEvidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	second := autoRAGDecision{
		Outcome:   "evidence_ready",
		Evidence:  []knowledge.HydratedEvidence{firstEvidence, secondEvidence},
		Citations: secondCitations, Authority: authority,
	}
	merged = mergeKnowledgeToolDecision(merged.Cumulative, second)

	if len(merged.Current.Citations) != 2 ||
		merged.Current.Citations[0].Marker != "[K1]" ||
		merged.Current.Citations[1].Marker != "[K2]" ||
		len(merged.Cumulative.Citations) != 2 ||
		merged.Cumulative.Citations[0].Marker != "[K1]" ||
		merged.Cumulative.Citations[1].Marker != "[K2]" {
		t.Fatalf("merged decisions = %#v", merged)
	}
}

func TestValidateSearchKnowledgeToolCallRejectsCollectionOverride(t *testing.T) {
	query, args, failure := validateSearchKnowledgeToolCall(ProviderToolCall{
		Name:      searchKnowledgeToolName,
		Arguments: `{"query":"fixture","collectionIds":["attacker"]}`,
	})
	if failure != "" || query != "fixture" || len(args) != 1 || args["query"] != "fixture" {
		t.Fatalf("validation = %q / %#v / %q", query, args, failure)
	}

	_, _, failure = validateSearchKnowledgeToolCall(ProviderToolCall{
		Name:      searchKnowledgeToolName,
		Arguments: `{"query":`,
	})
	if failure != "invalid_arguments" {
		t.Fatalf("malformed failure = %q", failure)
	}
}

func TestKnowledgeToolDependencyFailureClassification(t *testing.T) {
	decision := executeKnowledgeTool(context.Background(), nil, "fixture")
	if decision.Outcome != "dependency_unavailable" ||
		knowledgeToolFailureCategory(decision) != "dependency_unavailable" {
		t.Fatalf("decision = %#v", decision)
	}
	if got := knowledgeToolFailureCategory(autoRAGDecision{Outcome: "no_evidence"}); got != "" {
		t.Fatalf("miss failure category = %q", got)
	}
}
