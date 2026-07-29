package memorycapture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
)

func TestAssembleRegressionObservationsPreservesCorpusOrder(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	cases := make([]memoryeval.CaseObservation, len(pool.Corpus.Cases))
	for index, item := range pool.Corpus.Cases {
		cases[len(cases)-index-1] = memoryeval.CaseObservation{
			CaseID: item.ID, CandidateMemoryIDs: []string{}, FinalMemoryIDs: []string{},
			InjectedMemoryIDs: []string{}, PersistedMemoryIDs: []string{},
			ProviderSentMemoryIDs: []string{}, Fallback: "no_memory",
		}
	}
	digest := sha256.Sum256([]byte("profile"))
	value, body, err := AssembleRegressionObservations(
		pool,
		time.Date(2026, 7, 29, 13, 0, 0, 0, time.UTC),
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		CapturedProfile{
			Profile: memoryeval.Profile{ID: BaselineProfileID, Role: "baseline", ReaderVersion: ReaderVersion,
				ConfigurationSHA256: hex.EncodeToString(digest[:]), CandidateLimit: 20, FinalLimit: 5},
			Costs: memoryeval.ProviderCosts{Unit: "cny_microunits", ChatProviderCostMicrounits: 100},
			Cases: cases,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) == 0 || len(value.Cases) != 500 {
		t.Fatalf("assembled observations = cases:%d bytes:%d", len(value.Cases), len(body))
	}
	for index, item := range pool.Corpus.Cases {
		if value.Cases[index].CaseID != item.ID {
			t.Fatalf("case order differs at %d", index)
		}
	}
}

func TestCostBasisRequiresExplicitSameUnitAuthority(t *testing.T) {
	valid := CostBasis{
		SchemaVersion: "neo-chat.memory-regression-cost-basis.v1",
		Baseline:      memoryeval.ProviderCosts{Unit: "cny_microunits", ChatProviderCostMicrounits: 100},
		Candidate:     memoryeval.ProviderCosts{Unit: "cny_microunits", MemoryProviderCostMicrounits: 10, ChatProviderCostMicrounits: 100},
		Source:        "operator rate card", EffectiveAt: "2026-07-29T13:00:00Z",
	}
	if digest, err := CostBasisSHA256(valid); err != nil || len(digest) != 64 {
		t.Fatalf("valid cost basis = %q/%v", digest, err)
	}
	valid.Candidate.Unit = "usd_microunits"
	if _, err := CostBasisSHA256(valid); err == nil {
		t.Fatal("mismatched cost units were accepted")
	}
}

func TestDecodeCostBasisRejectsAmbiguousOrFabricatedCost(t *testing.T) {
	valid := CostBasis{
		SchemaVersion: "neo-chat.memory-regression-cost-basis.v1",
		Baseline: memoryeval.ProviderCosts{
			Unit: "cny_microunits", ChatProviderCostMicrounits: 100,
		},
		Candidate: memoryeval.ProviderCosts{
			Unit: "cny_microunits", MemoryProviderCostMicrounits: 10,
			ChatProviderCostMicrounits: 100,
		},
		Source: "operator rate card", EffectiveAt: "2026-07-29T13:00:00Z",
	}
	body, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, digest, err := DecodeCostBasis(body); err != nil ||
		decoded.Source != valid.Source || len(digest) != 64 {
		t.Fatalf("DecodeCostBasis() = %#v, %q, %v", decoded, digest, err)
	}

	invalid := []string{
		`{"schemaVersion":"neo-chat.memory-regression-cost-basis.v1","schemaVersion":"duplicate"}`,
		string(body[:len(body)-1]) + `,"unknown":true}`,
		string(body) + `{}`,
	}
	for _, candidate := range invalid {
		if _, _, err := DecodeCostBasis([]byte(candidate)); err == nil {
			t.Fatalf("ambiguous cost basis was accepted: %s", candidate)
		}
	}

	valid.Candidate.MemoryProviderCostMicrounits = 0
	if _, err := CostBasisSHA256(valid); err == nil {
		t.Fatal("zero candidate Memory cost was accepted")
	}
	valid.Candidate.MemoryProviderCostMicrounits = 10
	valid.Baseline.MemoryProviderCostMicrounits = 1
	if _, err := CostBasisSHA256(valid); err == nil {
		t.Fatal("non-zero baseline Memory cost was accepted")
	}
	valid.Baseline.MemoryProviderCostMicrounits = 0
	valid.Candidate.ChatProviderCostMicrounits++
	if _, err := CostBasisSHA256(valid); err == nil {
		t.Fatal("different chat denominator was accepted")
	}
}

func TestRegressionCaptureTimestampHonorsDeterministicAuditFloor(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	auditedAt, err := time.Parse(time.RFC3339, pool.Audit.AuditedAt)
	if err != nil {
		t.Fatal(err)
	}
	before, err := RegressionCaptureTimestamp(pool, auditedAt.Add(-time.Hour))
	if err != nil || !before.Equal(auditedAt) {
		t.Fatalf("capture before audit = %s/%v, want %s", before, err, auditedAt)
	}
	afterInput := auditedAt.Add(time.Hour)
	after, err := RegressionCaptureTimestamp(pool, afterInput)
	if err != nil || !after.Equal(afterInput) {
		t.Fatalf("capture after audit = %s/%v, want %s", after, err, afterInput)
	}
}
