package memorycapture

import (
	"encoding/json"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestSelectMemoryV20AbstentionDiagnosticDevelopmentIsStableAndDevelopmentOnly(t *testing.T) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	selected, err := SelectMemoryV20AbstentionDiagnosticDevelopment(pool)
	if err != nil {
		t.Fatal(err)
	}
	temporal, stable, intersection := 0, 0, 0
	for _, item := range selected.Corpus.Cases {
		if item.Split != DevelopmentCalibrationSplit {
			t.Fatalf("selected protected split %q", item.Split)
		}
		hasTemporal := containsString(item.Slices, "temporal_correction")
		hasStable := containsString(item.Slices, "stable_fact")
		if !hasTemporal && !hasStable {
			t.Fatalf("case %q has neither diagnostic slice", item.ID)
		}
		if hasTemporal {
			temporal++
		}
		if hasStable {
			stable++
		}
		if hasTemporal && hasStable {
			intersection++
		}
	}
	if len(selected.Corpus.Cases) != 57 || len(selected.Fixtures.Fixtures) != 57 ||
		temporal != 30 || stable != 30 || intersection != 3 {
		t.Fatalf("diagnostic cardinality cases=%d fixtures=%d temporal=%d stable=%d intersection=%d",
			len(selected.Corpus.Cases), len(selected.Fixtures.Fixtures), temporal, stable, intersection)
	}
	first, err := MemoryV20AbstentionDiagnosticCaseOrderSHA256(selected)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MemoryV20AbstentionDiagnosticCaseOrderSHA256(selected)
	if err != nil || first != second || !validSHA256String(first) {
		t.Fatalf("stable order hash %q/%q err=%v", first, second, err)
	}
	drifted := selected
	drifted.Corpus.Cases = append([]memoryeval.GoldenCase(nil), selected.Corpus.Cases...)
	drifted.Corpus.Cases[0], drifted.Corpus.Cases[1] =
		drifted.Corpus.Cases[1], drifted.Corpus.Cases[0]
	driftHash, err := MemoryV20AbstentionDiagnosticCaseOrderSHA256(drifted)
	if err != nil || driftHash == first {
		t.Fatal("case order drift did not alter diagnostic hash")
	}
}

func TestMemoryV20AbstentionDiagnosticProfileAndReportAreNonPromotional(t *testing.T) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	selected, err := SelectMemoryV20AbstentionDiagnosticDevelopment(pool)
	if err != nil {
		t.Fatal(err)
	}
	protected := productionBufferedValidationProtected(pool)
	cost := memoryV20AbstentionDiagnosticTestCostBasis()
	costSHA256, err := CostBasisSHA256(cost)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, ok := usermemory.DescribeHybridShadowRelevancePolicy(
		usermemory.HybridShadowV20AbstentionDiagnosticPolicy(),
	)
	if !ok || descriptor.HardCutoffMilliseconds != 0 ||
		descriptor.CloudCandidateJudgePromptVersion !=
			usermemory.HybridCandidateJudgeAccuracyPromptVersion {
		t.Fatalf("diagnostic policy descriptor=%#v ok=%t", descriptor, ok)
	}
	config, err := BuildMemoryV20AbstentionDiagnosticProfileConfig(
		protected, costSHA256, ProviderModeFakeProtocol,
		FixedMemoryJudgeAuthority(), ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if config.SchemaVersion != "neo-chat.memory-regression-profile-config.v20-abstention-diagnostic.v1" ||
		config.ReaderVersion != MemoryV20AbstentionDiagnosticReaderVersion ||
		config.RelevancePolicyID != usermemory.HybridRelevanceV20AbstentionDiagnosticPolicyID ||
		config.RelevancePolicyMode != "fixed_cloud_candidate_judge_accuracy_v20_abstention_diagnostic" ||
		config.DiagnosticRepetitions != 3 || len(config.DiagnosticSliceUnion) != 2 ||
		!validSHA256String(config.DiagnosticCaseOrderSHA256) ||
		config.AccuracyFirstExecutionPolicy == nil {
		t.Fatalf("diagnostic config=%#v", config)
	}
	configurationSHA256, err := ConfigurationSHA256(config)
	if err != nil {
		t.Fatal(err)
	}
	profile := memoryV20AbstentionDiagnosticPassingProfile(t, selected, config, cost.Candidate)
	report, body, err := BuildMemoryV20AbstentionDiagnosticReport(
		selected, profile, config, FixedMemoryJudgeAuthority(), cost,
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.ConfigurationSHA256 != configurationSHA256 ||
		report.PromotionEligible || report.ReleaseEligible || report.PolicySelected ||
		!report.ExecutionComplete || report.Classification != "not_reproduced" ||
		len(report.Cases) != 171 ||
		report.RootCauseCounts[V20AbstentionDiagnosticCauseNone]+
			report.RootCauseCounts[V20AbstentionDiagnosticCauseNotApplicable] != 171 {
		t.Fatalf("diagnostic report=%#v", report)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"query":`, `"canonicalContent":`, `"rerankRelevanceScores":`} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("diagnostic artifact retained forbidden field %q", forbidden)
		}
	}
	if report.Cases[0].Repetition != 1 || report.Cases[57].Repetition != 2 ||
		report.Cases[114].Repetition != 3 || report.Cases[0].CaseID != report.Cases[57].CaseID ||
		report.Cases[57].CaseID != report.Cases[114].CaseID {
		t.Fatal("diagnostic report order is not repetition-major")
	}
}

func TestMemoryV20AbstentionDiagnosticClassifiesSystematicAndStochasticStageLoss(t *testing.T) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	selected, err := SelectMemoryV20AbstentionDiagnosticDevelopment(pool)
	if err != nil {
		t.Fatal(err)
	}
	config := ProfileConfig{ProfileID: FakeCandidateProfileID}
	profile := memoryV20AbstentionDiagnosticPassingProfile(
		t, selected, config, memoryV20AbstentionDiagnosticTestCostBasis().Candidate,
	)
	for repetition := 0; repetition < 3; repetition++ {
		index := repetition * 57
		profile.Calibration[index].JudgeSelectedMemoryIDs = []string{}
		profile.Calibration[index].FullObservation.FinalMemoryIDs = []string{}
		profile.Cases[index].FinalMemoryIDs = []string{}
	}
	entries, counts, classification, _, _, _, err :=
		classifyMemoryV20AbstentionDiagnostic(selected, profile)
	if err != nil {
		t.Fatal(err)
	}
	if classification != "systematic" || counts[V20AbstentionDiagnosticCauseLunaNotSelected] != 3 ||
		entries[0].RootCause != V20AbstentionDiagnosticCauseLunaNotSelected {
		t.Fatalf("systematic classification=%q counts=%v", classification, counts)
	}
	profile.Calibration[57].JudgeSelectedMemoryIDs = append(
		[]string(nil), selected.Corpus.Cases[0].ExpectedCurrentMemoryIDs...,
	)
	profile.Calibration[57].FullObservation.FinalMemoryIDs = append(
		[]string(nil), selected.Corpus.Cases[0].ExpectedCurrentMemoryIDs...,
	)
	profile.Cases[57].FinalMemoryIDs = append(
		[]string(nil), selected.Corpus.Cases[0].ExpectedCurrentMemoryIDs...,
	)
	_, _, classification, _, _, _, err = classifyMemoryV20AbstentionDiagnostic(selected, profile)
	if err != nil || classification != "stochastic" {
		t.Fatalf("stochastic classification=%q err=%v", classification, err)
	}
}

func memoryV20AbstentionDiagnosticPassingProfile(
	t *testing.T,
	selected memoryauthor.RegressionPool,
	config ProfileConfig,
	cost memoryeval.ProviderCosts,
) CapturedProfile {
	t.Helper()
	configurationSHA256, err := ConfigurationSHA256(config)
	if err != nil {
		t.Fatal(err)
	}
	profile := CapturedProfile{
		Profile: memoryeval.Profile{
			ID: FakeCandidateProfileID, Role: "candidate",
			ReaderVersion:        MemoryV20AbstentionDiagnosticReaderVersion,
			ConfigurationSHA256:  configurationSHA256,
			CandidateLimit:       usermemory.MaxHybridShadowResults,
			FinalLimit:           usermemory.HybridShadowFinalLimit,
			ProviderEgressPolicy: memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
		},
		Costs:       cost,
		Cases:       make([]memoryeval.CaseObservation, 0, 171),
		Calibration: make([]CandidateCalibrationTrace, 0, 171),
	}
	for repetition := 0; repetition < 3; repetition++ {
		for _, golden := range selected.Corpus.Cases {
			ids := append([]string(nil), golden.ExpectedCurrentMemoryIDs...)
			observation := memoryeval.CaseObservation{
				CaseID: golden.ID, CandidateMemoryIDs: ids,
				FinalMemoryIDs: ids, InjectedMemoryIDs: ids,
			}
			profile.Cases = append(profile.Cases, observation)
			profile.Calibration = append(profile.Calibration, CandidateCalibrationTrace{
				CaseID: golden.ID, RerankReady: true, CloudJudgeReady: true,
				CloudJudgeInputTokenUpperBound: 10,
				FullObservation:                observation,
				RerankMemoryIDs:                ids,
				RerankRelevanceScores:          make([]float64, len(ids)),
				JudgeSelectedMemoryIDs:         ids,
			})
		}
	}
	profile.ProviderAttempts = AccuracyFirstProviderTelemetry{
		PassageEmbeddingAttempts:          1,
		QueryEmbeddingAttempts:            171,
		RerankAttempts:                    171,
		JudgeAttempts:                     171,
		JudgeInputTokenUpperBound:         1710,
		JudgeAttemptFailureCategoryCounts: map[string]int{},
		InterCaseCooldownCount:            170,
		InterCaseCooldownMilliseconds:     170000,
		PassageEmbeddingLatency:           testAccuracyFirstLatency(1),
		QueryEmbeddingLatency:             testAccuracyFirstLatency(171),
		RerankLatency:                     testAccuracyFirstLatency(171),
		JudgeLatency:                      testAccuracyFirstLatency(171),
	}
	return profile
}

func memoryV20AbstentionDiagnosticTestCostBasis() CostBasis {
	authority := FixedMemoryJudgeAuthority()
	return CostBasis{
		SchemaVersion:      "neo-chat.memory-regression-cost-basis.v20-abstention-diagnostic.v1",
		ProviderCostPolicy: ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
		Baseline: memoryeval.ProviderCosts{
			Unit: "cny_microunits", ChatProviderCostMicrounits: 100,
		},
		Candidate: memoryeval.ProviderCosts{
			Unit: "cny_microunits", MemoryProviderCostMicrounits: 50,
			ChatProviderCostMicrounits: 100,
		},
		Source: "test", EffectiveAt: "2026-08-07T00:00:00Z",
		ConfiguredCandidateJudgeAuthority: &ConfiguredCandidateJudgeCostAuthority{
			ProviderID: authority.ProviderID, ProviderType: authority.ProviderType,
			BaseURLSHA256: authority.BaseURLSHA256, ModelID: authority.ModelID,
			RequestCount: 513, MaximumInputTokens: 513_000,
			MaximumOutputTokens:              513 * usermemory.HybridCandidateJudgeMaximumOutputTokens,
			InputMicrounitsPerMillionTokens:  1,
			OutputMicrounitsPerMillionTokens: 1,
			MaximumCostMicrounits:            2,
		},
	}
}

func TestMemoryV20AbstentionDiagnosticCostAuthorityIsExactlyBounded(t *testing.T) {
	cost := memoryV20AbstentionDiagnosticTestCostBasis()
	if err := ValidateMemoryV20AbstentionDiagnosticCostAuthority(
		cost, FixedMemoryJudgeAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	drifted := cost
	authority := *cost.ConfiguredCandidateJudgeAuthority
	drifted.ConfiguredCandidateJudgeAuthority = &authority
	drifted.ConfiguredCandidateJudgeAuthority.RequestCount = 512
	if err := ValidateMemoryV20AbstentionDiagnosticCostAuthority(
		drifted, FixedMemoryJudgeAuthority(),
	); err == nil {
		t.Fatal("diagnostic cost authority accepted a request-count drift")
	}
	if memoryjudge.BufferedChatAdapterVersion == "" {
		t.Fatal("buffered adapter identity is unavailable")
	}
}
