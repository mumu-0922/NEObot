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

func TestSelectMemoryJudgeSliceDiagnosticDevelopmentIsStableAndDevelopmentOnly(t *testing.T) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	selected, err := SelectMemoryJudgeSliceDiagnosticDevelopment(pool)
	if err != nil {
		t.Fatal(err)
	}
	mixed, stable, intersection := 0, 0, 0
	for _, item := range selected.Corpus.Cases {
		if item.Split != DevelopmentCalibrationSplit {
			t.Fatalf("selected protected split %q", item.Split)
		}
		hasMixed := containsString(item.Slices, "mixed_language_entity")
		hasStable := containsString(item.Slices, "stable_fact")
		if !hasMixed && !hasStable {
			t.Fatalf("case %q has neither diagnostic slice", item.ID)
		}
		if hasMixed {
			mixed++
		}
		if hasStable {
			stable++
		}
		if hasMixed && hasStable {
			intersection++
		}
	}
	if len(selected.Corpus.Cases) != 85 || len(selected.Fixtures.Fixtures) != 85 ||
		mixed != 60 || stable != 30 || intersection != 5 {
		t.Fatalf("diagnostic cardinality cases=%d fixtures=%d mixed=%d stable=%d intersection=%d",
			len(selected.Corpus.Cases), len(selected.Fixtures.Fixtures), mixed, stable, intersection)
	}
	first, err := MemoryJudgeSliceDiagnosticCaseOrderSHA256(selected)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MemoryJudgeSliceDiagnosticCaseOrderSHA256(selected)
	if err != nil || first != second || !validSHA256String(first) {
		t.Fatalf("stable order hash %q/%q err=%v", first, second, err)
	}
	drifted := selected
	drifted.Corpus.Cases = append([]memoryeval.GoldenCase(nil), selected.Corpus.Cases...)
	drifted.Corpus.Cases[0], drifted.Corpus.Cases[1] =
		drifted.Corpus.Cases[1], drifted.Corpus.Cases[0]
	driftHash, err := MemoryJudgeSliceDiagnosticCaseOrderSHA256(drifted)
	if err != nil || driftHash == first {
		t.Fatal("case order drift did not alter diagnostic hash")
	}
}

func TestMemoryJudgeSliceDiagnosticProfileAndReportAreNonPromotional(t *testing.T) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	selected, err := SelectMemoryJudgeSliceDiagnosticDevelopment(pool)
	if err != nil {
		t.Fatal(err)
	}
	protected := productionBufferedValidationProtected(pool)
	cost := memoryJudgeSliceDiagnosticTestCostBasis()
	costSHA256, err := CostBasisSHA256(cost)
	if err != nil {
		t.Fatal(err)
	}
	config, err := BuildMemoryJudgeSliceDiagnosticProfileConfig(
		protected, costSHA256, ProviderModeFakeProtocol,
		FixedMemoryJudgeAuthority(), ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if config.SchemaVersion != "neo-chat.memory-regression-profile-config.v19" ||
		config.ReaderVersion != MemoryJudgeSliceDiagnosticReaderVersion ||
		config.RelevancePolicyID != usermemory.HybridRelevanceSliceDiagnosticPolicyID ||
		config.RelevancePolicyMode != "fixed_cloud_candidate_judge_negative_guard_slice_diagnostic" ||
		config.DiagnosticRepetitions != 3 || len(config.DiagnosticSliceUnion) != 2 ||
		!validSHA256String(config.DiagnosticCaseOrderSHA256) ||
		config.AccuracyFirstExecutionPolicy == nil {
		t.Fatalf("diagnostic config=%#v", config)
	}
	configurationSHA256, err := ConfigurationSHA256(config)
	if err != nil {
		t.Fatal(err)
	}
	profile := memoryJudgeSliceDiagnosticPassingProfile(t, selected, config, cost.Candidate)
	report, body, err := BuildMemoryJudgeSliceDiagnosticReport(
		selected, profile, config, FixedMemoryJudgeAuthority(), cost,
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.ConfigurationSHA256 != configurationSHA256 ||
		report.PromotionEligible || report.ReleaseEligible || report.PolicySelected ||
		!report.ExecutionComplete || report.Classification != "not_reproduced" ||
		len(report.Cases) != 255 ||
		report.RootCauseCounts[SliceDiagnosticCauseNone]+
			report.RootCauseCounts[SliceDiagnosticCauseNotApplicable] != 255 {
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
	if report.Cases[0].Repetition != 1 || report.Cases[85].Repetition != 2 ||
		report.Cases[170].Repetition != 3 || report.Cases[0].CaseID != report.Cases[85].CaseID ||
		report.Cases[85].CaseID != report.Cases[170].CaseID {
		t.Fatal("diagnostic report order is not repetition-major")
	}
}

func TestMemoryJudgeSliceDiagnosticClassifiesSystematicAndStochasticStageLoss(t *testing.T) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	selected, err := SelectMemoryJudgeSliceDiagnosticDevelopment(pool)
	if err != nil {
		t.Fatal(err)
	}
	config := ProfileConfig{ProfileID: FakeCandidateProfileID}
	profile := memoryJudgeSliceDiagnosticPassingProfile(
		t, selected, config, memoryJudgeSliceDiagnosticTestCostBasis().Candidate,
	)
	for repetition := 0; repetition < 3; repetition++ {
		index := repetition * 85
		profile.Calibration[index].JudgeSelectedMemoryIDs = []string{}
		profile.Calibration[index].FullObservation.FinalMemoryIDs = []string{}
		profile.Cases[index].FinalMemoryIDs = []string{}
	}
	entries, counts, classification, _, _, _, err :=
		classifyMemoryJudgeSliceDiagnostic(selected, profile)
	if err != nil {
		t.Fatal(err)
	}
	if classification != "systematic" || counts[SliceDiagnosticCauseLunaNotSelected] != 3 ||
		entries[0].RootCause != SliceDiagnosticCauseLunaNotSelected {
		t.Fatalf("systematic classification=%q counts=%v", classification, counts)
	}
	profile.Calibration[85].JudgeSelectedMemoryIDs = append(
		[]string(nil), selected.Corpus.Cases[0].ExpectedCurrentMemoryIDs...,
	)
	profile.Calibration[85].FullObservation.FinalMemoryIDs = append(
		[]string(nil), selected.Corpus.Cases[0].ExpectedCurrentMemoryIDs...,
	)
	profile.Cases[85].FinalMemoryIDs = append(
		[]string(nil), selected.Corpus.Cases[0].ExpectedCurrentMemoryIDs...,
	)
	_, _, classification, _, _, _, err = classifyMemoryJudgeSliceDiagnostic(selected, profile)
	if err != nil || classification != "stochastic" {
		t.Fatalf("stochastic classification=%q err=%v", classification, err)
	}
}

func memoryJudgeSliceDiagnosticPassingProfile(
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
			ReaderVersion:        MemoryJudgeSliceDiagnosticReaderVersion,
			ConfigurationSHA256:  configurationSHA256,
			CandidateLimit:       usermemory.MaxHybridShadowResults,
			FinalLimit:           usermemory.HybridShadowFinalLimit,
			ProviderEgressPolicy: memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
		},
		Costs:       cost,
		Cases:       make([]memoryeval.CaseObservation, 0, 255),
		Calibration: make([]CandidateCalibrationTrace, 0, 255),
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
		QueryEmbeddingAttempts:            255,
		RerankAttempts:                    255,
		JudgeAttempts:                     255,
		JudgeInputTokenUpperBound:         2550,
		JudgeAttemptFailureCategoryCounts: map[string]int{},
		InterCaseCooldownCount:            254,
		InterCaseCooldownMilliseconds:     254000,
		PassageEmbeddingLatency:           testAccuracyFirstLatency(1),
		QueryEmbeddingLatency:             testAccuracyFirstLatency(255),
		RerankLatency:                     testAccuracyFirstLatency(255),
		JudgeLatency:                      testAccuracyFirstLatency(255),
	}
	return profile
}

func memoryJudgeSliceDiagnosticTestCostBasis() CostBasis {
	authority := FixedMemoryJudgeAuthority()
	return CostBasis{
		SchemaVersion:      "neo-chat.memory-regression-cost-basis.v14",
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
			RequestCount: 765, MaximumInputTokens: 765_000,
			MaximumOutputTokens:              765 * usermemory.HybridCandidateJudgeMaximumOutputTokens,
			InputMicrounitsPerMillionTokens:  1,
			OutputMicrounitsPerMillionTokens: 1,
			MaximumCostMicrounits:            2,
		},
	}
}

func TestMemoryJudgeSliceDiagnosticCostAuthorityIsExactlyBounded(t *testing.T) {
	cost := memoryJudgeSliceDiagnosticTestCostBasis()
	if err := ValidateMemoryJudgeSliceDiagnosticCostAuthority(
		cost, FixedMemoryJudgeAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	drifted := cost
	authority := *cost.ConfiguredCandidateJudgeAuthority
	drifted.ConfiguredCandidateJudgeAuthority = &authority
	drifted.ConfiguredCandidateJudgeAuthority.RequestCount = 764
	if err := ValidateMemoryJudgeSliceDiagnosticCostAuthority(
		drifted, FixedMemoryJudgeAuthority(),
	); err == nil {
		t.Fatal("diagnostic cost authority accepted a request-count drift")
	}
	if memoryjudge.BufferedChatAdapterVersion == "" {
		t.Fatal("buffered adapter identity is unavailable")
	}
}
