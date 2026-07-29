package memorycapture

import (
	"encoding/json"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
)

func TestBuildRunManifestIsContentFreeAndNonPromotional(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	protected := ProtectedRegression{
		Pool:             pool,
		FixtureRawSHA256: sha256String("fixture"), CorpusRawSHA256: sha256String("corpus"),
		AuditRawSHA256: sha256String("audit"), ManifestRawSHA256: sha256String("manifest"),
	}
	baseline := protocolRegressionReport(BaselineProfileID, "baseline", true)
	candidate := protocolRegressionReport(FakeCandidateProfileID, "candidate", false)
	artifacts := []Artifact{
		{Name: "baseline.observations.json", Body: []byte("{\"ids\":[]}\n")},
		{Name: "candidate.report.json", Body: []byte("{\"passed\":false}\n")},
	}
	started := time.Date(2026, 7, 29, 13, 0, 0, 0, time.UTC)
	manifest, body, err := BuildRunManifest(
		"run-1", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		ProviderModeFakeProtocol, started, started.Add(time.Minute), protected,
		sha256String("cost"), ProfileHashes{
			Baseline: sha256String("baseline"), Candidate: sha256String("candidate"),
			BaselineProfileID: BaselineProfileID, CandidateProfileID: FakeCandidateProfileID,
		}, baseline, candidate, artifacts,
	)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.PromotionEligible || manifest.CorpusClass != memoryeval.RegressionCorpusClass ||
		manifest.AdmissionMode != memoryeval.RegressionAdmissionMode || len(body) == 0 {
		t.Fatalf("manifest = %#v", manifest)
	}
	all := append(append([]Artifact(nil), artifacts...), Artifact{Name: "run-manifest.json", Body: body})
	if err := VerifyRetainedArtifactsLeakFree(pool, all, []byte("fixture-live-key")); err != nil {
		t.Fatalf("content-free manifest leak check: %v", err)
	}
}

func TestVerifyRetainedArtifactsLeakFreeRejectsPlaintextAndCredential(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	query := pool.Corpus.Cases[0].Query
	memoryText := pool.Fixtures.Fixtures[0].Memories[0].CanonicalContent
	credential := []byte("fixture-live-key")
	for name, body := range map[string][]byte{
		"query":      []byte(`{"query":` + quoteJSON(query) + `}`),
		"memory":     []byte(memoryText),
		"credential": credential,
	} {
		t.Run(name, func(t *testing.T) {
			if err := VerifyRetainedArtifactsLeakFree(pool, []Artifact{{
				Name: "leak.json", Body: body,
			}}, credential); err == nil {
				t.Fatal("protected plaintext was accepted")
			}
		})
	}
}

func protocolRegressionReport(profileID, role string, passed bool) memoryeval.RegressionReport {
	return memoryeval.RegressionReport{
		Passed: passed,
		Profile: memoryeval.ProfileSummary{
			ProfileID: profileID, ProfileRole: role, ReaderVersion: ReaderVersion,
		},
	}
}

func quoteJSON(value string) string {
	body, _ := json.Marshal(value)
	return string(body)
}
