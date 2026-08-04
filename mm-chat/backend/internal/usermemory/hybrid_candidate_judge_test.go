package usermemory

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/ragproviders"
)

const hybridJudgeTestModel = "Pro/test/Memory-Judge"

func TestHybridCandidateJudgePromptAndStrictOutputContract(t *testing.T) {
	input := HybridCandidateJudgeInput{
		Query: "Should this answer use my preference?",
		Candidates: []HybridCandidateJudgeCandidate{
			{Ordinal: 0, Content: "Keep answers concise."},
			{Ordinal: 1, Content: "Unrelated personal note."},
		},
	}
	systemPrompt, userPrompt, err := BuildHybridCandidateJudgePrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if sha256String(systemPrompt) != HybridCandidateJudgePromptSHA256 ||
		!strings.Contains(systemPrompt, "untrusted data") ||
		!strings.Contains(userPrompt, input.Query) ||
		strings.Contains(userPrompt, "memoryId") ||
		strings.Contains(userPrompt, "relevanceScore") {
		t.Fatalf("judge prompt contract drifted: system=%q user=%q", systemPrompt, userPrompt)
	}

	tests := []struct {
		name string
		body string
		want []int
	}{
		{name: "selected", body: judgeOutputJSON(1, 0), want: []int{1, 0}},
		{name: "no memory", body: judgeOutputJSON(), want: []int{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selected, err := DecodeHybridCandidateJudgeOutput([]byte(test.body), 2)
			if err != nil || !equalInts(selected, test.want) {
				t.Fatalf("selected=%v err=%v", selected, err)
			}
		})
	}

	invalid := []string{
		`{"schemaVersion":"neo-chat.memory-cloud-candidate-judge-output.v1"}`,
		`{"schemaVersion":"neo-chat.memory-cloud-candidate-judge-output.v1","selectedOrdinals":[],"reason":"none"}`,
		`{"schemaVersion":"neo-chat.memory-cloud-candidate-judge-output.v1","schemaVersion":"neo-chat.memory-cloud-candidate-judge-output.v1","selectedOrdinals":[]}`,
		`{"schemaVersion":"drifted","selectedOrdinals":[]}`,
		`{"schemaVersion":"neo-chat.memory-cloud-candidate-judge-output.v1","selectedOrdinals":null}`,
		`{"schemaVersion":"neo-chat.memory-cloud-candidate-judge-output.v1","selectedOrdinals":[0,0]}`,
		`{"schemaVersion":"neo-chat.memory-cloud-candidate-judge-output.v1","selectedOrdinals":[2]}`,
		`{"schemaVersion":"neo-chat.memory-cloud-candidate-judge-output.v1","selectedOrdinals":[0,1,0,1,0,1]}`,
		"```json\n" + judgeOutputJSON() + "\n```",
		judgeOutputJSON() + judgeOutputJSON(),
	}
	for index, body := range invalid {
		if selected, err := DecodeHybridCandidateJudgeOutput([]byte(body), 2); err == nil {
			t.Fatalf("invalid[%d] selected=%v", index, selected)
		}
	}
}

func TestHybridCandidateJudgeOutputErrorsAreStructurallyTyped(t *testing.T) {
	tests := []struct {
		name string
		body string
		kind HybridCandidateJudgeOutputErrorKind
	}{
		{name: "empty", body: "", kind: HybridCandidateJudgeOutputJSONInvalid},
		{name: "syntax", body: `{`, kind: HybridCandidateJudgeOutputJSONInvalid},
		{name: "trailing", body: judgeOutputJSON() + `{}`, kind: HybridCandidateJudgeOutputJSONInvalid},
		{name: "duplicate key", body: `{"schemaVersion":"neo-chat.memory-cloud-candidate-judge-output.v1","schemaVersion":"neo-chat.memory-cloud-candidate-judge-output.v1","selectedOrdinals":[]}`, kind: HybridCandidateJudgeOutputSchemaInvalid},
		{name: "extra key", body: `{"schemaVersion":"neo-chat.memory-cloud-candidate-judge-output.v1","selectedOrdinals":[],"reason":"private"}`, kind: HybridCandidateJudgeOutputSchemaInvalid},
		{name: "missing key", body: `{"schemaVersion":"neo-chat.memory-cloud-candidate-judge-output.v1"}`, kind: HybridCandidateJudgeOutputSchemaInvalid},
		{name: "wrong type", body: `{"schemaVersion":"neo-chat.memory-cloud-candidate-judge-output.v1","selectedOrdinals":"zero"}`, kind: HybridCandidateJudgeOutputSchemaInvalid},
		{name: "schema drift", body: `{"schemaVersion":"drifted","selectedOrdinals":[]}`, kind: HybridCandidateJudgeOutputSchemaInvalid},
		{name: "duplicate ordinal", body: judgeOutputJSON(0, 0), kind: HybridCandidateJudgeOutputOrdinalInvalid},
		{name: "ordinal out of range", body: judgeOutputJSON(2), kind: HybridCandidateJudgeOutputOrdinalInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeHybridCandidateJudgeOutput([]byte(test.body), 2)
			kind, ok := HybridCandidateJudgeOutputErrorKindOf(err)
			if !ok || kind != test.kind {
				t.Fatalf("kind=%q/%t err=%v", kind, ok, err)
			}
		})
	}
}

func TestFixedMemoryJudgeDevelopmentPolicyVersionsOnlyCutoffAndIdentity(t *testing.T) {
	const modelID = HybridFixedMemoryJudgeModelID
	legacyDescriptor, legacyOK := DescribeHybridShadowRelevancePolicy(
		HybridShadowCloudJudgeCalibrationPolicy(modelID),
	)
	fixedDescriptor, fixedOK := DescribeHybridShadowRelevancePolicy(
		HybridShadowFixedMemoryJudgeDevelopmentPolicy(),
	)
	if !legacyOK || !fixedOK {
		t.Fatal("candidate-judge policy description failed")
	}
	if legacyDescriptor.ID != HybridRelevanceCloudJudgeCalibrationPolicyID ||
		legacyDescriptor.HardCutoffMilliseconds != 2000 ||
		fixedDescriptor.ID != HybridRelevanceFixedMemoryJudgePolicyID ||
		fixedDescriptor.HardCutoffMilliseconds != 3000 {
		t.Fatalf("policy identities/cutoffs = %#v / %#v", legacyDescriptor, fixedDescriptor)
	}
	legacyDescriptor.ID = fixedDescriptor.ID
	legacyDescriptor.Mode = fixedDescriptor.Mode
	legacyDescriptor.HardCutoffMilliseconds = fixedDescriptor.HardCutoffMilliseconds
	if legacyDescriptor != fixedDescriptor {
		t.Fatalf("fixed policy drifted beyond identity/cutoff: %#v / %#v", legacyDescriptor, fixedDescriptor)
	}
}

func TestAccuracyFirstMemoryJudgePolicyHasNoApplicationCutoff(t *testing.T) {
	descriptor, ok := DescribeHybridShadowRelevancePolicy(
		HybridShadowAccuracyFirstMemoryJudgeDevelopmentPolicy(),
	)
	if !ok || descriptor.ID != HybridRelevanceAccuracyFirstJudgePolicyID ||
		descriptor.Mode != hybridPolicyModeAccuracyFirstJudge ||
		descriptor.HardCutoffMilliseconds != 0 ||
		descriptor.CloudCandidateJudgeModelID != HybridFixedMemoryJudgeModelID {
		t.Fatalf("accuracy-first policy = %#v, %v", descriptor, ok)
	}
}

func TestFixedMemoryJudgeProductionPolicyHasSeparateAccuracyFirstIdentity(t *testing.T) {
	descriptor, ok := DescribeHybridShadowRelevancePolicy(
		HybridShadowFixedMemoryJudgeProductionPolicy(),
	)
	if !ok || descriptor.ID != HybridRelevanceProductionJudgePolicyID ||
		descriptor.Mode != hybridPolicyModeProductionJudge ||
		descriptor.HardCutoffMilliseconds != 0 ||
		!descriptor.CloudCandidateJudgeRequired ||
		descriptor.CloudCandidateJudgeModelID != HybridFixedMemoryJudgeModelID {
		t.Fatalf("production policy = %#v, %v", descriptor, ok)
	}
}

func TestAccuracyFirstMemoryJudgeRunsRerankThenJudgeWithoutDeadlines(t *testing.T) {
	repository := cloudJudgeTestRepository()
	order := make([]string, 0, 2)
	provider := &serialHybridProvider{order: &order}
	judge := &serialHybridJudge{order: &order}
	_, _, err := NewService(
		repository,
		WithHybridShadowProvider(provider),
		WithHybridCandidateJudge(judge),
		WithHybridShadowRelevancePolicy(
			HybridShadowAccuracyFirstMemoryJudgeDevelopmentPolicy(),
		),
	).SearchRelevantWithHybridShadow(
		context.Background(), "saved preference", hybridTestConversation,
		hybridTestAssistant, MaxSearchResults,
	)
	if err != nil || !equalStrings(order, []string{"rerank", "judge"}) ||
		provider.sawDeadline || judge.sawDeadline ||
		len(repository.recordInput.Final) != 1 ||
		repository.recordInput.FallbackCode != "NONE" {
		t.Fatalf(
			"accuracy-first order=%v providerDeadline=%v judgeDeadline=%v record=%#v err=%v",
			order,
			provider.sawDeadline,
			judge.sawDeadline,
			repository.recordInput,
			err,
		)
	}
}

func TestSearchRelevantWithCloudJudgeIntersectsBGEOrder(t *testing.T) {
	repository := cloudJudgeTestRepository()
	provider := &hybridTestProvider{
		embedding: validHybridTestEmbedding(),
		rerank: []ragproviders.RerankResult{
			{Index: 2, RelevanceScore: 0.7},
			{Index: 1, RelevanceScore: 0.8},
			{Index: 0, RelevanceScore: 0.9},
		},
	}
	judge := &hybridTestCandidateJudge{
		result: validHybridJudgeResult(hybridJudgeTestModel, 2, 0),
	}
	_, _, err := NewService(
		repository,
		WithHybridShadowProvider(provider),
		WithHybridCandidateJudge(judge),
		WithHybridShadowRelevancePolicy(
			HybridShadowCloudJudgeCalibrationPolicy(hybridJudgeTestModel),
		),
	).SearchRelevantWithHybridShadow(
		context.Background(), "Use my saved answer preference",
		hybridTestConversation, hybridTestAssistant, MaxSearchResults,
	)
	if err != nil {
		t.Fatal(err)
	}
	if provider.rerankCalls != 1 || judge.calls != 1 || repository.admissionCalls != 1 ||
		len(repository.recordInput.Reranked) != 3 || len(repository.recordInput.Final) != 2 {
		t.Fatalf("cloud judge calls/result = provider:%#v judge:%#v record:%#v",
			provider, judge, repository.recordInput)
	}
	if repository.recordInput.Reranked[0].MemoryID != repository.preparation.Candidates[0].MemoryID ||
		repository.recordInput.Final[0].MemoryID != repository.preparation.Candidates[0].MemoryID ||
		repository.recordInput.Final[1].MemoryID != repository.preparation.Candidates[2].MemoryID {
		t.Fatalf("BGE/intersection order = %#v", repository.recordInput)
	}
	if judge.input.Query != "Use my saved answer preference" ||
		len(judge.input.Candidates) != 3 ||
		judge.input.Candidates[2].Ordinal != 2 ||
		judge.input.Candidates[2].Content != repository.preparation.Candidates[2].Content {
		t.Fatalf("judge input = %#v", judge.input)
	}
}

func TestSearchRelevantWithCloudJudgeFailsClosed(t *testing.T) {
	tests := []struct {
		name     string
		judge    *hybridTestCandidateJudge
		fallback string
	}{
		{
			name: "no memory",
			judge: &hybridTestCandidateJudge{
				result: validHybridJudgeResult(hybridJudgeTestModel),
			},
			fallback: "CANDIDATE_JUDGE_ABSTAINED",
		},
		{
			name: "malformed",
			judge: &hybridTestCandidateJudge{
				result: HybridCandidateJudgeResult{
					RawOutput:     []byte(`{"selectedOrdinals":[0]}`),
					ModelID:       hybridJudgeTestModel,
					PromptVersion: HybridCandidateJudgePromptVersion,
					PromptSHA256:  HybridCandidateJudgePromptSHA256,
				},
			},
			fallback: "CANDIDATE_JUDGE_FAILED",
		},
		{
			name: "model drift",
			judge: &hybridTestCandidateJudge{
				result: validHybridJudgeResult("drifted", 0),
			},
			fallback: "CANDIDATE_JUDGE_FAILED",
		},
		{
			name:     "provider failure",
			judge:    &hybridTestCandidateJudge{err: errors.New("private failure")},
			fallback: "CANDIDATE_JUDGE_FAILED",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := cloudJudgeTestRepository()
			provider := &hybridTestProvider{
				embedding: validHybridTestEmbedding(),
				rerank: []ragproviders.RerankResult{
					{Index: 0, RelevanceScore: 1},
					{Index: 1, RelevanceScore: 0.9},
					{Index: 2, RelevanceScore: 0.8},
				},
			}
			_, _, err := NewService(
				repository,
				WithHybridShadowProvider(provider),
				WithHybridCandidateJudge(test.judge),
				WithHybridShadowRelevancePolicy(
					HybridShadowCloudJudgeCalibrationPolicy(hybridJudgeTestModel),
				),
			).SearchRelevantWithHybridShadow(
				context.Background(), "unrelated query", hybridTestConversation,
				hybridTestAssistant, MaxSearchResults,
			)
			if err != nil || len(repository.recordInput.Final) != 0 ||
				repository.recordInput.EstimatedTokens != 0 ||
				repository.recordInput.FallbackCode != test.fallback {
				t.Fatalf("fail closed = record:%#v err:%v", repository.recordInput, err)
			}
		})
	}
}

func TestCloudJudgeAndBGERerankRunConcurrently(t *testing.T) {
	repository := cloudJudgeTestRepository()
	rerankStarted := make(chan struct{})
	judgeStarted := make(chan struct{})
	provider := &coordinatedHybridProvider{
		rerankStarted: rerankStarted,
		judgeStarted:  judgeStarted,
	}
	judge := &coordinatedHybridJudge{
		rerankStarted: rerankStarted,
		judgeStarted:  judgeStarted,
	}
	_, _, err := NewService(
		repository,
		WithHybridShadowProvider(provider),
		WithHybridCandidateJudge(judge),
		WithHybridShadowRelevancePolicy(
			HybridShadowCloudJudgeCalibrationPolicy(hybridJudgeTestModel),
		),
	).SearchRelevantWithHybridShadow(
		context.Background(), "saved preference", hybridTestConversation,
		hybridTestAssistant, MaxSearchResults,
	)
	if err != nil || len(repository.recordInput.Final) != 1 ||
		repository.recordInput.FallbackCode != "NONE" {
		t.Fatalf("concurrent stages = %#v err=%v", repository.recordInput, err)
	}
}

func TestCloudJudgeStageDeadlineDoesNotWaitForIgnoringProvider(t *testing.T) {
	release := make(chan struct{})
	judge := &blockingHybridJudge{release: release}
	provider := &hybridTestProvider{
		rerank: []ragproviders.RerankResult{{Index: 0, RelevanceScore: 1}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, _, fallback, err := executeHybridCandidateStages(
		ctx,
		provider,
		judge,
		nil,
		HybridShadowCloudJudgeCalibrationPolicy(hybridJudgeTestModel),
		"saved preference",
		[]HybridShadowCandidate{
			hybridCandidate("44444444-4444-4444-8444-444444444444", "First preference"),
		},
	)
	close(release)
	if !errors.Is(err, context.DeadlineExceeded) ||
		fallback != "CANDIDATE_JUDGE_FAILED" || time.Since(started) >= time.Second {
		t.Fatalf("ignoring judge cutoff = fallback:%q duration:%s err:%v",
			fallback, time.Since(started), err)
	}
}

func cloudJudgeTestRepository() *hybridTestRepository {
	candidates := []HybridShadowCandidate{
		hybridCandidate("44444444-4444-4444-8444-444444444444", "First preference"),
		hybridCandidate("55555555-5555-4555-8555-555555555555", "Second preference"),
		hybridCandidate("66666666-6666-4666-8666-666666666666", "Third preference"),
	}
	return &hybridTestRepository{
		fakeRepository: hybridV1Repository(),
		preparation: HybridShadowPreparation{
			Summary: HybridShadowSummary{
				ProfileID: HybridShadowProfileID, Status: "pending",
				ResultCode: "CANDIDATES_READY", FallbackCode: "NONE", RRFCount: len(candidates),
			},
			Candidates: candidates,
		},
		recordSummary: HybridShadowSummary{
			ProfileID: HybridShadowProfileID, Status: "completed",
			ResultCode: "OK", FallbackCode: "NONE", RRFCount: len(candidates),
		},
	}
}

func judgeOutputJSON(ordinals ...int) string {
	body, _ := json.Marshal(hybridCandidateJudgeOutput{
		SchemaVersion:    HybridCandidateJudgeOutputSchemaVersion,
		SelectedOrdinals: append([]int{}, ordinals...),
	})
	return string(body)
}

func validHybridJudgeResult(modelID string, ordinals ...int) HybridCandidateJudgeResult {
	return HybridCandidateJudgeResult{
		RawOutput:     []byte(judgeOutputJSON(ordinals...)),
		ModelID:       modelID,
		PromptVersion: HybridCandidateJudgePromptVersion,
		PromptSHA256:  HybridCandidateJudgePromptSHA256,
	}
}

func equalInts(left []int, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type hybridTestCandidateJudge struct {
	input  HybridCandidateJudgeInput
	result HybridCandidateJudgeResult
	err    error
	calls  int
}

func (judge *hybridTestCandidateJudge) JudgeHybridCandidates(
	_ context.Context,
	input HybridCandidateJudgeInput,
) (HybridCandidateJudgeResult, error) {
	judge.calls++
	judge.input = input
	return judge.result, judge.err
}

type coordinatedHybridProvider struct {
	rerankStarted chan struct{}
	judgeStarted  chan struct{}
}

func (provider *coordinatedHybridProvider) EmbedQuery(
	context.Context,
	string,
) (ragproviders.QueryEmbedding, error) {
	return validHybridTestEmbedding(), nil
}

func (provider *coordinatedHybridProvider) Rerank(
	ctx context.Context,
	_ string,
	documents []string,
) ([]ragproviders.RerankResult, error) {
	close(provider.rerankStarted)
	select {
	case <-provider.judgeStarted:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	results := make([]ragproviders.RerankResult, len(documents))
	for index := range documents {
		results[index] = ragproviders.RerankResult{Index: index, RelevanceScore: 1}
	}
	return results, nil
}

type coordinatedHybridJudge struct {
	rerankStarted chan struct{}
	judgeStarted  chan struct{}
}

type blockingHybridJudge struct {
	release <-chan struct{}
}

type serialHybridProvider struct {
	order       *[]string
	sawDeadline bool
}

func (provider *serialHybridProvider) EmbedQuery(
	ctx context.Context,
	_ string,
) (ragproviders.QueryEmbedding, error) {
	_, provider.sawDeadline = ctx.Deadline()
	return validHybridTestEmbedding(), nil
}

func (provider *serialHybridProvider) Rerank(
	ctx context.Context,
	_ string,
	documents []string,
) ([]ragproviders.RerankResult, error) {
	_, hasDeadline := ctx.Deadline()
	provider.sawDeadline = provider.sawDeadline || hasDeadline
	*provider.order = append(*provider.order, "rerank")
	results := make([]ragproviders.RerankResult, len(documents))
	for index := range documents {
		results[index] = ragproviders.RerankResult{Index: index, RelevanceScore: 1}
	}
	return results, nil
}

type serialHybridJudge struct {
	order       *[]string
	sawDeadline bool
}

func (judge *serialHybridJudge) JudgeHybridCandidates(
	ctx context.Context,
	_ HybridCandidateJudgeInput,
) (HybridCandidateJudgeResult, error) {
	_, judge.sawDeadline = ctx.Deadline()
	*judge.order = append(*judge.order, "judge")
	return validHybridJudgeResult(HybridFixedMemoryJudgeModelID, 0), nil
}

func (judge *blockingHybridJudge) JudgeHybridCandidates(
	_ context.Context,
	_ HybridCandidateJudgeInput,
) (HybridCandidateJudgeResult, error) {
	<-judge.release
	return validHybridJudgeResult(hybridJudgeTestModel, 0), nil
}

func (judge *coordinatedHybridJudge) JudgeHybridCandidates(
	ctx context.Context,
	_ HybridCandidateJudgeInput,
) (HybridCandidateJudgeResult, error) {
	close(judge.judgeStarted)
	select {
	case <-judge.rerankStarted:
	case <-ctx.Done():
		return HybridCandidateJudgeResult{}, ctx.Err()
	}
	return validHybridJudgeResult(hybridJudgeTestModel, 0), nil
}

var (
	_ HybridCandidateJudge = (*hybridTestCandidateJudge)(nil)
	_ HybridShadowProvider = (*coordinatedHybridProvider)(nil)
	_ HybridCandidateJudge = (*coordinatedHybridJudge)(nil)
	_ HybridCandidateJudge = (*blockingHybridJudge)(nil)
	_ HybridShadowProvider = (*serialHybridProvider)(nil)
	_ HybridCandidateJudge = (*serialHybridJudge)(nil)
)
