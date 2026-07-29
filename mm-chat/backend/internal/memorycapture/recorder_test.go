package memorycapture

import (
	"context"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestCloneTransientCapturePreservesRerankScoresAndSlicePresence(t *testing.T) {
	original := transientCapture{
		candidates:   []string{"memory-one"},
		final:        []string{},
		providerSent: []string{"memory-one"},
		rerankReady:  true,
		rerankScores: map[string]float64{"memory-one": 0.75},
	}
	cloned := cloneTransientCapture(original)
	if cloned.candidates == nil || cloned.final == nil || cloned.providerSent == nil ||
		!cloned.rerankReady || cloned.rerankScores["memory-one"] != 0.75 {
		t.Fatalf("cloned transient capture = %#v", cloned)
	}
	cloned.candidates[0] = "changed"
	cloned.rerankScores["memory-one"] = 0
	if original.candidates[0] != "memory-one" || original.rerankScores["memory-one"] != 0.75 {
		t.Fatal("transient capture clone aliases mutable state")
	}
}

func TestProviderDecoratorDoesNotAuthorizeLateRerankScores(t *testing.T) {
	recorder := &Recorder{}
	if err := recorder.Begin(captureAssistantID); err != nil {
		t.Fatal(err)
	}
	if err := recorder.recordPrepared(captureAssistantID, []usermemory.HybridShadowCandidate{
		{MemoryID: captureMemoryOne},
	}); err != nil {
		t.Fatal(err)
	}
	decorator, err := NewProviderDecorator(lateCaptureProvider{}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	results, err := decorator.Rerank(ctx, "query", []string{"document"})
	if err != nil || len(results) != 1 {
		t.Fatalf("late rerank result = %#v/%v", results, err)
	}
	transient, err := recorder.Finish(captureAssistantID)
	if err != nil {
		t.Fatal(err)
	}
	if transient.rerankReady || transient.rerankScores != nil ||
		len(transient.providerSent) != 1 {
		t.Fatalf("late rerank recorder state = %#v", transient)
	}
}

func TestRecorderUnionsConcurrentRerankAndCloudJudgeEgress(t *testing.T) {
	recorder := &Recorder{}
	if err := recorder.Begin(captureAssistantID); err != nil {
		t.Fatal(err)
	}
	if err := recorder.recordPrepared(captureAssistantID, []usermemory.HybridShadowCandidate{
		{MemoryID: captureMemoryOne},
	}); err != nil {
		t.Fatal(err)
	}
	if err := recorder.recordProviderSent("cloud_judge", 1); err != nil {
		t.Fatal(err)
	}
	if err := recorder.recordProviderSent("rerank", 1); err != nil {
		t.Fatal(err)
	}
	if err := recorder.recordCloudJudgeInput(usermemory.HybridCandidateJudgeInput{
		Query: "query",
		Candidates: []usermemory.HybridCandidateJudgeCandidate{
			{Ordinal: 0, Content: "candidate"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	result := usermemory.HybridCandidateJudgeResult{
		RawOutput:     []byte(`{"schemaVersion":"neo-chat.memory-cloud-candidate-judge-output.v1","selectedOrdinals":[]}`),
		ModelID:       "Pro/test/judge",
		PromptVersion: usermemory.HybridCandidateJudgePromptVersion,
		PromptSHA256:  usermemory.HybridCandidateJudgePromptSHA256,
	}
	if err := recorder.recordCloudJudgeResult(result, 1); err != nil {
		t.Fatal(err)
	}
	if err := recorder.recordProviderSent("cloud_judge", 1); err == nil {
		t.Fatal("duplicate cloud-judge egress was accepted")
	}
	transient, err := recorder.Finish(captureAssistantID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transient.providerSent) != 1 || !transient.rerankEgressReady ||
		!transient.judgeEgressReady || !transient.cloudJudgeReady ||
		transient.cloudJudgeInputTokenUpperBound <= 0 {
		t.Fatalf("cloud recorder = %#v", transient)
	}
}

type lateCaptureProvider struct{}

func (lateCaptureProvider) EmbedQuery(context.Context, string) (ragproviders.QueryEmbedding, error) {
	return ragproviders.QueryEmbedding{}, nil
}

func (lateCaptureProvider) Rerank(ctx context.Context, _ string, _ []string) ([]ragproviders.RerankResult, error) {
	<-ctx.Done()
	return []ragproviders.RerankResult{{Index: 0, RelevanceScore: 0.9}}, nil
}
