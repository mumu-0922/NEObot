package memorycapture

import (
	"context"
	"errors"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	captureUserID         = "11111111-1111-4111-8111-111111111111"
	captureConversationID = "22222222-2222-4222-8222-222222222222"
	captureAssistantID    = "33333333-3333-4333-8333-333333333333"
	captureMemoryOne      = "44444444-4444-4444-8444-444444444441"
	captureMemoryTwo      = "44444444-4444-4444-8444-444444444442"
)

func TestCaptureBaselineUsesActualV1TopFiveSurface(t *testing.T) {
	repository := &captureRepository{memories: []usermemory.Memory{
		{ID: captureMemoryOne, Type: "preference", Content: "Keep answers concise", NormalizedContent: "keep answers concise", Importance: 5, Enabled: true, Revision: 1, ScopeType: "global"},
		{ID: captureMemoryTwo, Type: "fact", Content: "Unrelated marker", NormalizedContent: "unrelated marker", Importance: 3, Enabled: true, Revision: 1, ScopeType: "global"},
	}}
	index := captureFixtureIndex()
	observed, err := CaptureBaseline(context.Background(), usermemory.NewService(repository), index, RuntimeCase{
		CaseID: "case-one", Query: "Please keep answers concise", UserID: captureUserID,
		ConversationID: captureConversationID, AssistantMessageID: captureAssistantID,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertObservationIDs(t, observed, []string{"memory-one"}, []string{"memory-one"}, nil)
	if observed.Fallback != "none" || observed.PromptMemoryTokens <= 0 || repository.marked != captureMemoryOne {
		t.Fatalf("baseline metadata = %#v marked=%q", observed, repository.marked)
	}
}

func TestCaptureCandidateRecordsProductionHybridSurfaces(t *testing.T) {
	base := &captureRepository{memories: []usermemory.Memory{
		{ID: captureMemoryOne, Type: "preference", Content: "Keep answers concise", NormalizedContent: "keep answers concise", Importance: 5, Enabled: true, Revision: 1, ScopeType: "global"},
	}}
	recorder := &Recorder{}
	repository, err := NewRepositoryDecorator(base, base, recorder)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewProviderDecorator(captureProvider{}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	service := usermemory.NewService(repository, usermemory.WithHybridShadowProvider(provider),
		usermemory.WithHybridShadowRelevancePolicy(usermemory.HybridShadowCalibrationPolicy()))
	observed, err := CaptureCandidate(context.Background(), service, recorder, captureFixtureIndex(), RuntimeCase{
		CaseID: "case-one", Query: "Please keep answers concise", UserID: captureUserID,
		ConversationID: captureConversationID, AssistantMessageID: captureAssistantID,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertObservationIDs(
		t,
		observed,
		[]string{"memory-one", "memory-two"},
		[]string{"memory-two", "memory-one"},
		[]string{"memory-one", "memory-two"},
	)
	if observed.Fallback != "none" || observed.PromptMemoryTokens <= 0 ||
		len(observed.InjectedMemoryIDs) != len(observed.FinalMemoryIDs) {
		t.Fatalf("candidate metadata = %#v", observed)
	}
}

func TestCaptureCandidateAbstainsWhenPrepareFails(t *testing.T) {
	base := &captureRepository{
		memories: []usermemory.Memory{{
			ID: captureMemoryOne, Type: "preference", Content: "Keep answers concise",
			NormalizedContent: "keep answers concise", Importance: 5, Enabled: true,
			Revision: 1, ScopeType: "global",
		}},
		prepareErr: errors.New("fixture prepare failure"),
	}
	recorder := &Recorder{}
	repository, _ := NewRepositoryDecorator(base, base, recorder)
	provider, _ := NewProviderDecorator(captureProvider{}, recorder)
	observed, err := CaptureCandidate(
		context.Background(),
		usermemory.NewService(repository, usermemory.WithHybridShadowProvider(provider),
			usermemory.WithHybridShadowRelevancePolicy(usermemory.HybridShadowCalibrationPolicy())),
		recorder,
		captureFixtureIndex(),
		RuntimeCase{CaseID: "case-one", Query: "keep answers concise", UserID: captureUserID,
			ConversationID: captureConversationID, AssistantMessageID: captureAssistantID},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertObservationIDs(t, observed, []string{}, []string{}, nil)
	if observed.Fallback != "no_memory" || observed.PromptMemoryTokens != 0 {
		t.Fatalf("fallback = %#v", observed)
	}
}

func TestCaptureCandidateRecordsRerankFailureWithoutHidingProviderSurface(t *testing.T) {
	base := &captureRepository{memories: []usermemory.Memory{{
		ID: captureMemoryOne, Type: "preference", Content: "Keep answers concise",
		NormalizedContent: "keep answers concise", Importance: 5, Enabled: true,
		Revision: 1, ScopeType: "global",
	}}}
	recorder := &Recorder{}
	repository, _ := NewRepositoryDecorator(base, base, recorder)
	provider, _ := NewProviderDecorator(captureProvider{rerankErr: errors.New("fixture failure")}, recorder)
	observed, err := CaptureCandidate(
		context.Background(),
		usermemory.NewService(repository, usermemory.WithHybridShadowProvider(provider),
			usermemory.WithHybridShadowRelevancePolicy(usermemory.HybridShadowCalibrationPolicy())),
		recorder,
		captureFixtureIndex(),
		RuntimeCase{CaseID: "case-one", Query: "keep answers concise", UserID: captureUserID,
			ConversationID: captureConversationID, AssistantMessageID: captureAssistantID},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertObservationIDs(
		t, observed,
		[]string{"memory-one", "memory-two"},
		[]string{},
		[]string{"memory-one", "memory-two"},
	)
	if observed.Fallback != "no_memory" || observed.HardCutoffApplied {
		t.Fatalf("rerank failure observation = %#v", observed)
	}
}

func TestCaptureCandidateRecordsHardCutoffAsBoundedFallback(t *testing.T) {
	base := &captureRepository{memories: []usermemory.Memory{{
		ID: captureMemoryOne, Type: "preference", Content: "Keep answers concise",
		NormalizedContent: "keep answers concise", Importance: 5, Enabled: true,
		Revision: 1, ScopeType: "global",
	}}}
	recorder := &Recorder{}
	repository, _ := NewRepositoryDecorator(base, base, recorder)
	provider, _ := NewProviderDecorator(captureProvider{waitForCutoff: true}, recorder)
	observed, err := CaptureCandidate(
		context.Background(),
		usermemory.NewService(repository, usermemory.WithHybridShadowProvider(provider),
			usermemory.WithHybridShadowRelevancePolicy(usermemory.HybridShadowCalibrationPolicy())),
		recorder,
		captureFixtureIndex(),
		RuntimeCase{CaseID: "case-one", Query: "keep answers concise", UserID: captureUserID,
			ConversationID: captureConversationID, AssistantMessageID: captureAssistantID},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !observed.HardCutoffApplied || observed.Fallback != "no_memory" ||
		len(observed.ProviderSentMemoryIDs) != 2 {
		t.Fatalf("hard-cutoff observation = %#v", observed)
	}
}

func TestCaptureCandidatePreservesAuthorizedCandidatesButDropsUnrecordedFinal(t *testing.T) {
	base := &captureRepository{
		memories: []usermemory.Memory{{
			ID: captureMemoryOne, Type: "preference", Content: "Keep answers concise",
			NormalizedContent: "keep answers concise", Importance: 5, Enabled: true,
			Revision: 1, ScopeType: "global",
		}},
		recordErr: errors.New("fixture record failure"),
	}
	recorder := &Recorder{}
	repository, _ := NewRepositoryDecorator(base, base, recorder)
	provider, _ := NewProviderDecorator(captureProvider{}, recorder)
	observed, err := CaptureCandidate(
		context.Background(),
		usermemory.NewService(repository, usermemory.WithHybridShadowProvider(provider),
			usermemory.WithHybridShadowRelevancePolicy(usermemory.HybridShadowCalibrationPolicy())),
		recorder,
		captureFixtureIndex(),
		RuntimeCase{CaseID: "case-one", Query: "keep answers concise", UserID: captureUserID,
			ConversationID: captureConversationID, AssistantMessageID: captureAssistantID},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertObservationIDs(
		t, observed,
		[]string{"memory-one", "memory-two"},
		[]string{},
		[]string{"memory-one", "memory-two"},
	)
	if observed.Fallback != "no_memory" || observed.PromptMemoryTokens != 0 {
		t.Fatalf("record-failure observation = %#v", observed)
	}
}

func assertObservationIDs(
	t *testing.T,
	observed memoryeval.CaseObservation,
	candidates []string,
	final []string,
	provider []string,
) {
	t.Helper()
	assertStrings(t, "candidates", observed.CandidateMemoryIDs, candidates)
	assertStrings(t, "final", observed.FinalMemoryIDs, final)
	assertStrings(t, "injected", observed.InjectedMemoryIDs, final)
	assertStrings(t, "provider", observed.ProviderSentMemoryIDs, provider)
	if len(observed.PersistedMemoryIDs) != 0 {
		t.Fatalf("persisted IDs = %#v", observed.PersistedMemoryIDs)
	}
}

func assertStrings(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %#v, want %#v", label, got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("%s[%d] = %q, want %q", label, index, got[index], want[index])
		}
	}
}

func captureFixtureIndex() FixtureIndex {
	return FixtureIndex{UUIDToMemory: map[string]string{
		captureMemoryOne: "memory-one",
		captureMemoryTwo: "memory-two",
	}}
}

type captureRepository struct {
	memories   []usermemory.Memory
	marked     string
	prepareErr error
	recordErr  error
}

func (repository *captureRepository) GetSettings(context.Context) (usermemory.Settings, bool, error) {
	return usermemory.Settings{Enabled: true, SearchEnabled: true}, true, nil
}

func (repository *captureRepository) UpsertSettings(context.Context, usermemory.Settings) (usermemory.Settings, error) {
	return usermemory.Settings{}, errors.New("not implemented")
}

func (repository *captureRepository) List(context.Context) ([]usermemory.Memory, error) {
	return append([]usermemory.Memory(nil), repository.memories...), nil
}

func (repository *captureRepository) Create(context.Context, usermemory.CreateInput) (usermemory.Memory, error) {
	return usermemory.Memory{}, errors.New("not implemented")
}

func (repository *captureRepository) Update(context.Context, string, usermemory.UpdateInput) (usermemory.Memory, error) {
	return usermemory.Memory{}, errors.New("not implemented")
}

func (repository *captureRepository) Delete(context.Context, string) error {
	return errors.New("not implemented")
}

func (repository *captureRepository) MarkUsed(_ context.Context, ids []string, _ time.Time) error {
	if len(ids) > 0 {
		repository.marked = ids[0]
	}
	return nil
}

func (repository *captureRepository) PrepareHybridShadow(
	_ context.Context,
	input usermemory.HybridShadowPrepareInput,
) (usermemory.HybridShadowPreparation, error) {
	if repository.prepareErr != nil {
		return usermemory.HybridShadowPreparation{}, repository.prepareErr
	}
	return usermemory.HybridShadowPreparation{
		ObservationID: input.ObservationID,
		Summary:       usermemory.HybridShadowSummary{ProfileID: usermemory.HybridShadowProfileID, Status: "pending", ResultCode: "CANDIDATES_READY", FallbackCode: "NONE"},
		Candidates: []usermemory.HybridShadowCandidate{
			{MemoryID: captureMemoryOne, Revision: 1, ScopeType: "global", Content: "Keep answers concise"},
			{MemoryID: captureMemoryTwo, Revision: 1, ScopeType: "global", Content: "Keep output short"},
		},
	}, nil
}

func (repository *captureRepository) AuthorizeHybridRerank(
	context.Context,
	usermemory.HybridShadowAdmissionInput,
) (usermemory.HybridShadowAdmission, error) {
	return usermemory.HybridShadowAdmission{
		CandidateCount: 2, VectorCandidateCount: 2, MaximumVectorSimilarity: 1,
	}, nil
}

func (repository *captureRepository) RecordHybridShadow(
	_ context.Context,
	input usermemory.HybridShadowRecordInput,
) (usermemory.HybridShadowSummary, error) {
	if repository.recordErr != nil {
		return usermemory.HybridShadowSummary{}, repository.recordErr
	}
	return usermemory.HybridShadowSummary{
		ProfileID: usermemory.HybridShadowProfileID, Status: "completed", ResultCode: "OK",
		FallbackCode: input.FallbackCode, RRFCount: 2, RerankCount: len(input.Reranked),
		FinalCount: len(input.Final), EstimatedTokens: input.EstimatedTokens,
		DurationMillis: input.DurationMillis,
	}, nil
}

type captureProvider struct {
	rerankErr     error
	waitForCutoff bool
}

func (captureProvider) EmbedQuery(context.Context, string) (ragproviders.QueryEmbedding, error) {
	vector := make([]float32, ragproviders.SiliconFlowEmbeddingDimensions)
	vector[0] = 1
	return ragproviders.QueryEmbedding{ModelID: ragproviders.SiliconFlowEmbeddingModel, Dimensions: len(vector), Vector: vector}, nil
}

func (provider captureProvider) Rerank(ctx context.Context, _ string, documents []string) ([]ragproviders.RerankResult, error) {
	if provider.waitForCutoff {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if provider.rerankErr != nil {
		return nil, provider.rerankErr
	}
	result := make([]ragproviders.RerankResult, len(documents))
	for index := range documents {
		result[index] = ragproviders.RerankResult{
			Index: index, RelevanceScore: float64(index+1) / float64(len(documents)),
		}
	}
	return result, nil
}
