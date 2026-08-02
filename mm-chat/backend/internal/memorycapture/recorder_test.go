package memorycapture

import (
	"context"
	"errors"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestMemoryToolRouterDecoratorRecordsBoundedFailureCategory(t *testing.T) {
	recorder := &Recorder{}
	if err := recorder.Begin(captureAssistantID); err != nil {
		t.Fatal(err)
	}
	decorator, err := NewMemoryToolRouterDecorator(
		failingMemoryToolRouter{}, recorder, "route-model",
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = decorator.RouteHybridMemory(
		context.Background(),
		usermemory.HybridMemoryToolRouteInput{Query: "private query"},
	)
	if err == nil || usermemory.HybridMemoryToolRouteFailureCategory(err) !=
		usermemory.HybridMemoryToolRouteFailureRateLimited {
		t.Fatalf("route error = %v", err)
	}
	transient, err := recorder.Finish(captureAssistantID)
	if err != nil {
		t.Fatal(err)
	}
	if transient.memoryToolRouteFailureCategory !=
		usermemory.HybridMemoryToolRouteFailureRateLimited ||
		transient.memoryToolRouteReady {
		t.Fatalf("transient route failure = %#v", transient)
	}
}

func TestCandidateJudgeDecoratorRecordsTerminalFailureCategory(t *testing.T) {
	tests := []struct {
		name  string
		judge usermemory.HybridCandidateJudge
		want  string
	}{
		{
			name:  "unknown",
			judge: captureCandidateJudge{err: errors.New("private response body")},
			want:  memoryjudge.FailureUnclassified,
		},
		{
			name:  "provenance",
			judge: captureCandidateJudge{result: captureJudgeResult("drifted-model")},
			want:  memoryjudge.FailureProvenanceDrift,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := preparedJudgeRecorder(t)
			decorator, err := NewCandidateJudgeDecorator(test.judge, recorder, "expected-model")
			if err != nil {
				t.Fatal(err)
			}
			_, err = decorator.JudgeHybridCandidates(
				context.Background(),
				captureJudgeInput(),
			)
			if got := memoryjudge.FailureCategory(err); got != test.want {
				t.Fatalf("category=%q want=%q err=%v", got, test.want, err)
			}
			transient, finishErr := recorder.Finish(captureAssistantID)
			if finishErr != nil {
				t.Fatal(finishErr)
			}
			if transient.cloudJudgeReady ||
				transient.cloudJudgeFailureCategory != test.want {
				t.Fatalf("transient=%#v", transient)
			}
		})
	}
}

func TestCandidateJudgeDecoratorTypesRecorderConflict(t *testing.T) {
	decorator, err := NewCandidateJudgeDecorator(
		captureCandidateJudge{result: captureJudgeResult("expected-model")},
		&Recorder{},
		"expected-model",
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = decorator.JudgeHybridCandidates(
		context.Background(),
		captureJudgeInput(),
	)
	if got := memoryjudge.FailureCategory(err); got != memoryjudge.FailureRecorderStateConflict {
		t.Fatalf("category=%q err=%v", got, err)
	}
}

func preparedJudgeRecorder(t *testing.T) *Recorder {
	t.Helper()
	recorder := &Recorder{}
	if err := recorder.Begin(captureAssistantID); err != nil {
		t.Fatal(err)
	}
	if err := recorder.recordPrepared(captureAssistantID, []usermemory.HybridShadowCandidate{
		{MemoryID: captureMemoryOne},
	}); err != nil {
		t.Fatal(err)
	}
	return recorder
}

func captureJudgeInput() usermemory.HybridCandidateJudgeInput {
	return usermemory.HybridCandidateJudgeInput{
		Query: "private query",
		Candidates: []usermemory.HybridCandidateJudgeCandidate{
			{Ordinal: 0, Content: "private Memory"},
		},
	}
}

func captureJudgeResult(modelID string) usermemory.HybridCandidateJudgeResult {
	return usermemory.HybridCandidateJudgeResult{
		RawOutput:     []byte(`{"schemaVersion":"neo-chat.memory-cloud-candidate-judge-output.v1","selectedOrdinals":[]}`),
		ModelID:       modelID,
		PromptVersion: usermemory.HybridCandidateJudgePromptVersion,
		PromptSHA256:  usermemory.HybridCandidateJudgePromptSHA256,
	}
}

type captureCandidateJudge struct {
	result usermemory.HybridCandidateJudgeResult
	err    error
}

func (judge captureCandidateJudge) JudgeHybridCandidates(
	context.Context,
	usermemory.HybridCandidateJudgeInput,
) (usermemory.HybridCandidateJudgeResult, error) {
	return judge.result, judge.err
}

func TestMemoryToolRouterDecoratorRecordsProvenanceDrift(t *testing.T) {
	recorder := &Recorder{}
	if err := recorder.Begin(captureAssistantID); err != nil {
		t.Fatal(err)
	}
	decorator, err := NewMemoryToolRouterDecorator(
		driftedMemoryToolRouter{}, recorder, "expected-model",
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = decorator.RouteHybridMemory(
		context.Background(),
		usermemory.HybridMemoryToolRouteInput{Query: "private query"},
	)
	if usermemory.HybridMemoryToolRouteFailureCategory(err) !=
		usermemory.HybridMemoryToolRouteFailureProvenanceDrift {
		t.Fatalf("provenance error = %v", err)
	}
	transient, err := recorder.Finish(captureAssistantID)
	if err != nil {
		t.Fatal(err)
	}
	if transient.memoryToolRouteFailureCategory !=
		usermemory.HybridMemoryToolRouteFailureProvenanceDrift {
		t.Fatalf("provenance transient = %#v", transient)
	}
}

func TestMemoryToolRouterDecoratorDoesNotWaitForCancellationIgnoringRouter(t *testing.T) {
	recorder := &Recorder{}
	if err := recorder.Begin(captureAssistantID); err != nil {
		t.Fatal(err)
	}
	router := &cancellationIgnoringMemoryToolRouter{release: make(chan struct{})}
	decorator, err := NewMemoryToolRouterDecorator(router, recorder, "route-model")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	completed := make(chan error, 1)
	go func() {
		_, routeErr := decorator.RouteHybridMemory(
			ctx,
			usermemory.HybridMemoryToolRouteInput{Query: "private query"},
		)
		completed <- routeErr
	}()

	var routeErr error
	select {
	case routeErr = <-completed:
	case <-time.After(100 * time.Millisecond):
		close(router.release)
		<-completed
		t.Fatal("decorator waited for a router that ignored cancellation")
	}
	if category := usermemory.HybridMemoryToolRouteFailureCategory(routeErr); category !=
		usermemory.HybridMemoryToolRouteFailureContextDeadline {
		t.Fatalf("route error category = %q (%v)", category, routeErr)
	}
	transient, err := recorder.Finish(captureAssistantID)
	if err != nil {
		t.Fatal(err)
	}
	if transient.memoryToolRouteFailureCategory !=
		usermemory.HybridMemoryToolRouteFailureContextDeadline {
		t.Fatalf("deadline transient = %#v", transient)
	}
	close(router.release)
}

func TestRecorderRejectsMemoryToolRouteResultFromPreviousGeneration(t *testing.T) {
	recorder := &Recorder{}
	if err := recorder.Begin(captureAssistantID); err != nil {
		t.Fatal(err)
	}
	oldToken, err := recorder.recordMemoryToolRouteInput(
		usermemory.HybridMemoryToolRouteInput{Query: "first private query"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recorder.Finish(captureAssistantID); err != nil {
		t.Fatal(err)
	}

	// Reusing the same assistant identity must not let a late result from the
	// previous sequential case attach to the new capture generation.
	if err := recorder.Begin(captureAssistantID); err != nil {
		t.Fatal(err)
	}
	currentToken, err := recorder.recordMemoryToolRouteInput(
		usermemory.HybridMemoryToolRouteInput{Query: "second private query"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.recordMemoryToolRouteFailure(
		oldToken,
		usermemory.HybridMemoryToolRouteFailureRateLimited,
	); err == nil {
		t.Fatal("previous route generation mutated the current capture")
	}
	if err := recorder.recordMemoryToolRouteResult(
		currentToken,
		usermemory.HybridMemoryToolRouteResult{
			UseMemory:             true,
			ContractVersion:       usermemory.HybridMemoryToolContractVersion,
			ContractSHA256:        usermemory.HybridMemoryToolContractSHA256,
			OutputTokenUpperBound: 1,
		},
	); err != nil {
		t.Fatal(err)
	}
	transient, err := recorder.Finish(captureAssistantID)
	if err != nil {
		t.Fatal(err)
	}
	if !transient.memoryToolRouteReady || !transient.memoryToolRouteUsed ||
		transient.memoryToolRouteFailureCategory != "" {
		t.Fatalf("current route generation = %#v", transient)
	}
}

type failingMemoryToolRouter struct{}

func (failingMemoryToolRouter) RouteHybridMemory(
	context.Context,
	usermemory.HybridMemoryToolRouteInput,
) (usermemory.HybridMemoryToolRouteResult, error) {
	return usermemory.HybridMemoryToolRouteResult{},
		usermemory.NewHybridMemoryToolRouteError(
			usermemory.HybridMemoryToolRouteFailureRateLimited,
		)
}

type driftedMemoryToolRouter struct{}

func (driftedMemoryToolRouter) RouteHybridMemory(
	context.Context,
	usermemory.HybridMemoryToolRouteInput,
) (usermemory.HybridMemoryToolRouteResult, error) {
	return usermemory.HybridMemoryToolRouteResult{
		ModelID:               "other-model",
		ContractVersion:       usermemory.HybridMemoryToolContractVersion,
		ContractSHA256:        usermemory.HybridMemoryToolContractSHA256,
		OutputTokenUpperBound: 1,
	}, nil
}

type cancellationIgnoringMemoryToolRouter struct {
	release chan struct{}
}

func (router *cancellationIgnoringMemoryToolRouter) RouteHybridMemory(
	context.Context,
	usermemory.HybridMemoryToolRouteInput,
) (usermemory.HybridMemoryToolRouteResult, error) {
	<-router.release
	return usermemory.HybridMemoryToolRouteResult{
		ModelID:               "route-model",
		ContractVersion:       usermemory.HybridMemoryToolContractVersion,
		ContractSHA256:        usermemory.HybridMemoryToolContractSHA256,
		OutputTokenUpperBound: 1,
	}, nil
}

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
