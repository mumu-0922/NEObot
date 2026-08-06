package usermemory

import (
	"context"
	"encoding/json"
	"testing"

	"neo-chat/mm-chat/backend/internal/ragproviders"
)

func TestNegativePolicyQueryGuardMatchesBilingualMetaPolicyQuestions(t *testing.T) {
	for _, query := range []string{
		"Project Pebble里的无关记录应该影响检索策略吗？",
		"这些不相关的记忆是否应该控制当前回答？",
		"Should an unrelated note influence the cache policy?",
		"Can irrelevant memories override this answer?",
	} {
		if !matchesNegativePolicyQuery(query) {
			t.Fatalf("negative policy query did not match: %q", query)
		}
	}
}

func TestNegativePolicyQueryGuardLeavesRelevantMemoryRequestsUnmatched(t *testing.T) {
	for _, query := range []string{
		"请使用我保存的记忆回答我在哪所学校。",
		"我有哪些无关记录？",
		"在不采用无关记录的前提下，按我保存的长期偏好，分支命名应该怎么处理？",
		"Use my saved preference to answer this question.",
		"Which unrelated notes are stored in my Memory?",
	} {
		if matchesNegativePolicyQuery(query) {
			t.Fatalf("relevant or non-policy query unexpectedly matched: %q", query)
		}
	}
}

func TestNegativePolicyQueryGuardContractIsFrozen(t *testing.T) {
	if got := negativePolicyQueryGuardSHA256(); got != NegativePolicyQueryGuardSHA256 {
		t.Fatalf("negative policy guard hash = %q, want %q", got, NegativePolicyQueryGuardSHA256)
	}
	if NegativePolicyQueryGuardVersion != "memory-negative-policy-query-guard-v1" {
		t.Fatalf("negative policy guard version = %q", NegativePolicyQueryGuardVersion)
	}
}

func TestNegativePolicyGuardDevelopmentPolicyIsHashBoundAndAccuracyFirst(t *testing.T) {
	policy := HybridShadowNegativePolicyGuardDevelopmentPolicy()
	descriptor, ok := DescribeHybridShadowRelevancePolicy(policy)
	if !ok || descriptor.ID != HybridRelevanceNegativePolicyGuardDevelopmentPolicyID ||
		descriptor.Mode != hybridPolicyModeNegativePolicyGuard ||
		descriptor.HardCutoffMilliseconds != 0 ||
		!descriptor.CloudCandidateJudgeRequired ||
		descriptor.CloudCandidateJudgeModelID != HybridFixedMemoryJudgeModelID ||
		!descriptor.NegativePolicyQueryGuardRequired ||
		descriptor.NegativePolicyQueryGuardVersion != NegativePolicyQueryGuardVersion ||
		descriptor.NegativePolicyQueryGuardSHA256 != NegativePolicyQueryGuardSHA256 ||
		descriptor.MinimumProviderSimilarityBasisPoints != -100 ||
		descriptor.MinimumFinalRelevanceBasisPoints != 0 {
		t.Fatalf("negative guard Development policy = %#v, %v", descriptor, ok)
	}

	policy.NegativePolicyQueryGuardRequired = false
	if _, ok := DescribeHybridShadowRelevancePolicy(policy); ok {
		t.Fatal("negative guard policy admitted without its guard")
	}
	production := HybridShadowFixedMemoryJudgeProductionPolicy()
	production.NegativePolicyQueryGuardRequired = true
	if _, ok := DescribeHybridShadowRelevancePolicy(production); ok {
		t.Fatal("production policy admitted a Development guard")
	}
	guardedProduction := HybridShadowNegativePolicyGuardProductionPolicy()
	descriptor, ok = DescribeHybridShadowRelevancePolicy(guardedProduction)
	if !ok || descriptor.ID != HybridRelevanceNegativePolicyGuardProductionPolicyID ||
		descriptor.Mode != hybridPolicyModeGuardProductionJudge ||
		!descriptor.NegativePolicyQueryGuardRequired ||
		descriptor.NegativePolicyQueryGuardSHA256 != NegativePolicyQueryGuardSHA256 ||
		descriptor.HardCutoffMilliseconds != 0 {
		t.Fatalf("negative guard production policy = %#v, %v", descriptor, ok)
	}
	guardedProduction.NegativePolicyQueryGuardRequired = false
	if _, ok := DescribeHybridShadowRelevancePolicy(guardedProduction); ok {
		t.Fatal("negative guard production policy admitted without guard")
	}
}

func TestNegativePolicyGuardAbstainsBeforeCandidateProviderEgress(t *testing.T) {
	repository := cloudJudgeTestRepository()
	repository.recordSummary.FallbackCode = "NEGATIVE_POLICY_QUERY_ABSTAINED"
	provider := &hybridTestProvider{
		embedding: validHybridTestEmbedding(),
		rerank: []ragproviders.RerankResult{
			{Index: 0, RelevanceScore: 1},
		},
	}
	judge := &hybridTestCandidateJudge{
		result: validHybridJudgeResult(HybridFixedMemoryJudgeModelID, 0),
	}
	_, summary, err := NewService(
		repository,
		WithHybridShadowProvider(provider),
		WithHybridCandidateJudge(judge),
		WithHybridShadowRelevancePolicy(
			HybridShadowNegativePolicyGuardDevelopmentPolicy(),
		),
	).SearchRelevantWithHybridShadow(
		context.Background(),
		"Should an unrelated note influence this answer?",
		hybridTestConversation,
		hybridTestAssistant,
		MaxSearchResults,
	)
	if err != nil {
		t.Fatal(err)
	}
	if provider.embedCalls != 1 || repository.prepareCalls != 1 ||
		repository.admissionCalls != 0 || provider.rerankCalls != 0 ||
		judge.calls != 0 || repository.recordCalls != 1 {
		t.Fatalf(
			"guarded calls = embed:%d prepare:%d admission:%d rerank:%d judge:%d record:%d",
			provider.embedCalls,
			repository.prepareCalls,
			repository.admissionCalls,
			provider.rerankCalls,
			judge.calls,
			repository.recordCalls,
		)
	}
	if len(repository.recordInput.Reranked) != 0 || len(repository.recordInput.Final) != 0 ||
		repository.recordInput.EstimatedTokens != 0 ||
		repository.recordInput.RerankStatus != "skipped" ||
		repository.recordInput.FallbackCode != "NEGATIVE_POLICY_QUERY_ABSTAINED" ||
		summary.FallbackCode != "NEGATIVE_POLICY_QUERY_ABSTAINED" {
		t.Fatalf("guarded record=%#v summary=%#v", repository.recordInput, summary)
	}
}

func TestProductionPolicyDescriptorHashRemainsFrozen(t *testing.T) {
	descriptor, ok := DescribeHybridShadowRelevancePolicy(
		HybridShadowFixedMemoryJudgeProductionPolicy(),
	)
	if !ok {
		t.Fatal("production policy descriptor is invalid")
	}
	body, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	const want = "c65c2b0bee2561ebbc8d97a65c4cc0c64db243b8a09334a8f1836250d799095c"
	if got := sha256String(string(body)); got != want {
		t.Fatalf("production policy descriptor hash = %q, want %q; json=%s", got, want, body)
	}
}
