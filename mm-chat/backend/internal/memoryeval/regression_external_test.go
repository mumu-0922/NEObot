package memoryeval_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
)

func TestEvaluateRegressionUsesSeparateNonPromotionalAdmission(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	observations := perfectRegressionObservations(pool)
	report, err := memoryeval.EvaluateRegression(memoryeval.RegressionEvaluationInput{
		Corpus:             pool.Corpus,
		CorpusRawSHA256:    strings.Repeat("a", 64),
		Audit:              pool.Audit,
		AuditRawSHA256:     strings.Repeat("b", 64),
		Observations:       observations,
		ObservationsSHA256: strings.Repeat("c", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed || report.PromotionEligible ||
		report.CorpusClass != memoryeval.RegressionCorpusClass ||
		report.AdmissionMode != memoryeval.RegressionAdmissionMode ||
		report.Corpus.TotalCases != 500 || report.Corpus.AuditVerdict != "passed" {
		t.Fatalf("regression report = %+v", report)
	}
	body, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("holdoutRun")) || bytes.Contains(body, []byte("human_reviewed")) {
		t.Fatalf("regression report claimed formal authority: %s", body)
	}
	for name, slice := range report.Slices {
		if !slice.Passed || slice.Cases < 50 {
			t.Fatalf("slice %s = %+v", name, slice)
		}
	}
}

func TestRegressionAndGoldenSchemasCannotCrossAdmission(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memoryeval.DecodeGoldenSet(bytes.NewReader(pool.CorpusJSON)); err == nil {
		t.Fatal("formal Golden decoder accepted a regression corpus")
	}
	formal, err := memoryauthor.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memoryeval.DecodeRegressionCorpus(bytes.NewReader(formal.GoldenJSON)); err == nil {
		t.Fatal("regression decoder accepted a Golden draft")
	}

	mutated := pool.Corpus
	mutated.Cases = append([]memoryeval.GoldenCase(nil), mutated.Cases...)
	mutated.Cases[0].Review = memoryeval.Review{
		State:      "human_reviewed",
		ReviewerID: "11111111-1111-4111-8111-111111111111",
		ReviewedAt: "2026-07-29T11:00:00Z",
	}
	if err := memoryeval.ValidateRegressionAdmission(mutated, pool.Audit); err == nil ||
		!strings.Contains(err.Error(), "human attestation") {
		t.Fatalf("human-attested regression admission error = %v", err)
	}

	mutated = pool.Corpus
	mutated.PromotionEligible = nil
	if err := memoryeval.ValidateRegressionAdmission(mutated, pool.Audit); err == nil ||
		!strings.Contains(err.Error(), "header") {
		t.Fatalf("missing promotion denial error = %v", err)
	}
	mutated = pool.Corpus
	claim := true
	mutated.PromotionEligible = &claim
	if err := memoryeval.ValidateRegressionAdmission(mutated, pool.Audit); err == nil ||
		!strings.Contains(err.Error(), "header") {
		t.Fatalf("promotion claim error = %v", err)
	}
	mutated = pool.Corpus
	mutated.CorpusClass = "human_reviewed_golden"
	if err := memoryeval.ValidateRegressionAdmission(mutated, pool.Audit); err == nil ||
		!strings.Contains(err.Error(), "header") {
		t.Fatalf("wrong corpus class error = %v", err)
	}
}

func TestEvaluateRegressionRejectsObservationOrderAndAuditDrift(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	input := memoryeval.RegressionEvaluationInput{
		Corpus:             pool.Corpus,
		CorpusRawSHA256:    strings.Repeat("a", 64),
		Audit:              pool.Audit,
		AuditRawSHA256:     strings.Repeat("b", 64),
		Observations:       perfectRegressionObservations(pool),
		ObservationsSHA256: strings.Repeat("c", 64),
	}
	input.Observations.Cases[0], input.Observations.Cases[1] =
		input.Observations.Cases[1], input.Observations.Cases[0]
	if _, err := memoryeval.EvaluateRegression(input); err == nil ||
		!strings.Contains(err.Error(), "order differs") {
		t.Fatalf("order drift error = %v", err)
	}

	input.Observations = perfectRegressionObservations(pool)
	input.Observations.AuditContentSHA256 = strings.Repeat("d", 64)
	if _, err := memoryeval.EvaluateRegression(input); err == nil ||
		!strings.Contains(err.Error(), "not bound") {
		t.Fatalf("audit drift error = %v", err)
	}
}

func perfectRegressionObservations(pool memoryauthor.RegressionPool) memoryeval.RegressionObservationSet {
	cases := make([]memoryeval.CaseObservation, 0, len(pool.Corpus.Cases))
	for _, item := range pool.Corpus.Cases {
		candidate := append([]string(nil), item.ExpectedRelevantMemoryIDs...)
		final := append([]string(nil), item.ExpectedRelevantMemoryIDs...)
		fallback := "none"
		if item.ExpectedNoMemory {
			fallback = "no_memory"
		}
		cases = append(cases, memoryeval.CaseObservation{
			CaseID:              item.ID,
			CandidateMemoryIDs:  candidate,
			FinalMemoryIDs:      final,
			InjectedMemoryIDs:   append([]string(nil), final...),
			LatencyMilliseconds: 25,
			PromptMemoryTokens:  len(final) * 20,
			Fallback:            fallback,
		})
	}
	return memoryeval.RegressionObservationSet{
		SchemaVersion:         memoryeval.RegressionObservationSchemaVersion,
		CorpusID:              pool.Corpus.ID,
		CorpusContentSHA256:   pool.Corpus.CorpusContentSHA256,
		AuditContentSHA256:    pool.Audit.ContentSHA256,
		FixtureManifestSHA256: pool.Corpus.FixtureManifestSHA256,
		CapturedAt:            "2026-07-29T13:00:00Z",
		CaptureID:             "22222222-2222-4222-8222-222222222222",
		Profile: memoryeval.Profile{
			ID:                  "fixture-regression-profile",
			Role:                "baseline",
			ReaderVersion:       "fixture-reader-v1",
			ConfigurationSHA256: strings.Repeat("d", 64),
			CandidateLimit:      20,
			FinalLimit:          5,
		},
		Costs: memoryeval.ProviderCosts{
			Unit:                         "synthetic-microunit",
			MemoryProviderCostMicrounits: 1,
			ChatProviderCostMicrounits:   100,
		},
		Cases: cases,
	}
}
