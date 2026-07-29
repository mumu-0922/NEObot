package memorycapture

import (
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestBuildCloudJudgeDevelopmentReportUsesPolicyAwareSafety(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingCloudJudgeDevelopmentTraces(pool)
	profile := cloudJudgeDevelopmentProfile(traces)
	report, body, err := BuildCloudJudgeDevelopmentReport(
		pool,
		profile,
		hybridJudgeTestModelID,
		cloudJudgeTestCostBasis(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed || !report.Evaluation.Passed ||
		report.SchemaVersion != CloudJudgeDevelopmentReportSchemaVersion ||
		report.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
		!report.ProviderCostAuthorized || report.Evaluation.ProviderCostPassed != nil ||
		report.Evaluation.ProviderCostRatio != 0.5 ||
		report.PolicyID != usermemory.HybridRelevanceCloudJudgeCalibrationPolicyID ||
		report.ProviderEgressPolicy !=
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 ||
		report.Evaluation.Safety.UnauthorizedProviderEgressCount != 0 ||
		report.Evaluation.Metrics.FalseInjectionCases != 0 ||
		report.Evaluation.Metrics.CandidateRecallAt20 != 1 ||
		report.Evaluation.Metrics.FinalRecallAt5 != 1 ||
		report.Evaluation.Metrics.CurrentFactAccuracy != 1 ||
		report.Diagnostics.JudgeAbstainedCaseCount == 0 ||
		report.Diagnostics.FailedCaseCount != 0 {
		t.Fatalf("cloud judge report = %#v", report)
	}
	if strings.Contains(string(body), traces[0].CaseID) ||
		strings.Contains(string(body), "memory-current") ||
		strings.Contains(string(body), "First preference") {
		t.Fatalf("aggregate report retained case/content evidence: %s", body)
	}
}

func TestBuildCloudJudgeDevelopmentReportPreservesLegacyRelativeCostGate(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	cost := cloudJudgeTestCostBasis()
	cost.SchemaVersion = "neo-chat.memory-regression-cost-basis.v2"
	cost.ProviderCostPolicy = ""
	profile := cloudJudgeDevelopmentProfile(passingCloudJudgeDevelopmentTraces(pool))
	profile.Costs = cost.Candidate
	report, _, err := BuildCloudJudgeDevelopmentReport(
		pool,
		profile,
		hybridJudgeTestModelID,
		cost,
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || report.Evaluation.Passed ||
		report.SchemaVersion != cloudJudgeDevelopmentLegacyReportSchemaVersion ||
		report.ProviderCostPolicy != "" || report.ProviderCostAuthorized ||
		report.Evaluation.ProviderCostPassed == nil ||
		*report.Evaluation.ProviderCostPassed ||
		report.Evaluation.ProviderCostRatio != 0.5 {
		t.Fatalf("legacy relative-cost result drifted = %#v", report)
	}
}

func TestBuildCloudJudgeDevelopmentReportStillRejectsForbiddenEgress(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingCloudJudgeDevelopmentTraces(pool)
	for index, item := range pool.Corpus.Cases {
		if item.Split != DevelopmentCalibrationSplit {
			continue
		}
		for _, exclusion := range item.Exclusions {
			if exclusion.Reason != "cross_user" {
				continue
			}
			traceIndex := developmentTraceIndex(pool, index)
			traces[traceIndex].AdmissionReady = true
			traces[traceIndex].RerankReady = true
			traces[traceIndex].CloudJudgeReady = true
			traces[traceIndex].FullObservation.CandidateMemoryIDs = []string{exclusion.MemoryID}
			traces[traceIndex].FullObservation.ProviderSentMemoryIDs = []string{exclusion.MemoryID}
			profile := cloudJudgeDevelopmentProfile(traces)
			report, _, err := BuildCloudJudgeDevelopmentReport(
				pool,
				profile,
				hybridJudgeTestModelID,
				cloudJudgeTestCostBasis(),
			)
			if err != nil {
				t.Fatal(err)
			}
			if report.Passed || report.Evaluation.Safety.CrossUserLeakCount != 1 ||
				report.Evaluation.Safety.UnauthorizedProviderEgressCount != 1 ||
				report.Evaluation.Safety.Passed {
				t.Fatalf("forbidden egress was authorized = %#v", report.Evaluation.Safety)
			}
			return
		}
	}
	t.Fatal("cross-user Development case not found")
}

const hybridJudgeTestModelID = "Pro/test/Memory-Judge"

func passingCloudJudgeDevelopmentTraces(
	pool memoryauthor.RegressionPool,
) []CandidateCalibrationTrace {
	traces := passingDevelopmentCalibrationTraces(pool)
	for index := range traces {
		trace := &traces[index]
		trace.MemoryIntentReady = false
		trace.MemoryIntentMargin = 0
		candidateReady := len(trace.FullObservation.CandidateMemoryIDs) > 0
		trace.CloudJudgeReady = candidateReady
		if candidateReady {
			trace.CloudJudgeInputTokenUpperBound = 1000
		}
		if trace.FullObservation.CaseID == "" {
			continue
		}
		for _, item := range pool.Corpus.Cases {
			if item.ID != trace.CaseID || !item.ExpectedNoMemory {
				continue
			}
			trace.FullObservation.FinalMemoryIDs = []string{}
			trace.FullObservation.InjectedMemoryIDs = []string{}
			trace.FullObservation.PromptMemoryTokens = 0
			trace.FullObservation.Fallback = "no_memory"
			trace.FinalRelevanceScores = []float64{}
			break
		}
	}
	return traces
}

func cloudJudgeDevelopmentProfile(
	traces []CandidateCalibrationTrace,
) CapturedProfile {
	cases := make([]memoryeval.CaseObservation, len(traces))
	for index, trace := range traces {
		cases[index] = trace.FullObservation
	}
	return CapturedProfile{
		Profile: memoryeval.Profile{
			ID: CandidateProfileID, Role: "candidate",
			ReaderVersion:        CloudJudgeReaderVersion,
			ConfigurationSHA256:  strings.Repeat("a", 64),
			CandidateLimit:       usermemory.MaxHybridShadowResults,
			FinalLimit:           usermemory.HybridShadowFinalLimit,
			ProviderEgressPolicy: memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
		},
		Costs:       cloudJudgeTestCostBasis().Candidate,
		Cases:       cases,
		Calibration: traces,
	}
}

func cloudJudgeTestCostBasis() CostBasis {
	return CostBasis{
		SchemaVersion:      "neo-chat.memory-regression-cost-basis.v3",
		ProviderCostPolicy: ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
		Baseline: memoryeval.ProviderCosts{
			Unit: "cny_microunits", ChatProviderCostMicrounits: 100,
		},
		Candidate: memoryeval.ProviderCosts{
			Unit: "cny_microunits", MemoryProviderCostMicrounits: 50,
			ChatProviderCostMicrounits: 100,
		},
		Source: "test", EffectiveAt: "2026-07-29T00:00:00Z",
		CloudJudgeAuthority: &CloudJudgeCostAuthority{
			ModelID:                          hybridJudgeTestModelID,
			RequestCount:                     300,
			MaximumInputTokens:               300_000,
			MaximumOutputTokens:              300 * usermemory.HybridCandidateJudgeMaximumOutputTokens,
			InputMicrounitsPerMillionTokens:  1,
			OutputMicrounitsPerMillionTokens: 1,
			MaximumCostMicrounits:            2,
		},
	}
}

func developmentTraceIndex(pool memoryauthor.RegressionPool, corpusIndex int) int {
	index := 0
	for position, item := range pool.Corpus.Cases {
		if position == corpusIndex {
			return index
		}
		if item.Split == DevelopmentCalibrationSplit {
			index++
		}
	}
	return -1
}
