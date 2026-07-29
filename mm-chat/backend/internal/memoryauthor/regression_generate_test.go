package memoryauthor

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

func TestGenerateRegressionIsDeterministicAndPassesSemanticAudit(t *testing.T) {
	first, err := GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.FixtureJSON, second.FixtureJSON) ||
		!bytes.Equal(first.CorpusJSON, second.CorpusJSON) ||
		!bytes.Equal(first.AuditJSON, second.AuditJSON) ||
		!bytes.Equal(first.ManifestJSON, second.ManifestJSON) {
		t.Fatal("GenerateRegression() is not byte deterministic")
	}
	if first.Manifest.CaseCount != 500 ||
		first.Manifest.SplitCounts != (CountBySplit{Development: 300, Validation: 100, Holdout: 100}) ||
		first.Manifest.LanguageCounts != (CountByLanguage{Chinese: 350, Mixed: 100, English: 50}) {
		t.Fatalf("regression profile = %+v", first.Manifest)
	}
	if first.Audit.Verdict != "passed" || first.Audit.Semantic.QuerySkeletonCount < 100 ||
		!regressionAuditPasses(first.Audit) {
		t.Fatalf("regression audit = %+v", first.Audit)
	}
	for _, count := range first.Manifest.SliceCounts {
		if count.Total < 50 || count.Development < 30 || count.Validation < 10 || count.Holdout < 10 {
			t.Fatalf("slice count = %+v", count)
		}
	}
	if err := memoryeval.ValidateRegressionAdmission(first.Corpus, first.Audit); err != nil {
		t.Fatalf("ValidateRegressionAdmission() error = %v", err)
	}
}

func TestRegressionAuditDetectsShortcutAndWeakFallback(t *testing.T) {
	pool, err := GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	shortcutCase := 0
	pool.Corpus.Cases[shortcutCase].Query += " scenario 42"
	pool.Fixtures.Fixtures[shortcutCase].Memories[0].CanonicalContent += " scenario 42"
	audit, err := AuditRegression(pool.Fixtures, pool.Corpus)
	if err != nil {
		t.Fatal(err)
	}
	if audit.Verdict != "failed" || audit.Semantic.OrdinalShortcutCount == 0 {
		t.Fatalf("shortcut audit = %+v", audit.Semantic)
	}

	pool, err = GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	for index, item := range pool.Corpus.Cases {
		if contains(item.Slices, "failure_fallback") {
			pool.Corpus.Cases[index].Query = "What should happen for this saved setting?"
			break
		}
	}
	audit, err = AuditRegression(pool.Fixtures, pool.Corpus)
	if err != nil {
		t.Fatal(err)
	}
	if audit.Verdict != "failed" || audit.Semantic.FallbackSemanticFailureCount == 0 {
		t.Fatalf("fallback audit = %+v", audit.Semantic)
	}

	pool, err = GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	for index, item := range pool.Corpus.Cases {
		if contains(item.Slices, "multi_hop") {
			pool.Corpus.Cases[index].ExpectedRelevantMemoryIDs =
				pool.Corpus.Cases[index].ExpectedRelevantMemoryIDs[:1]
			pool.Corpus.Cases[index].ExpectedCurrentMemoryIDs =
				pool.Corpus.Cases[index].ExpectedCurrentMemoryIDs[:1]
			break
		}
	}
	audit, err = AuditRegression(pool.Fixtures, pool.Corpus)
	if err != nil {
		t.Fatal(err)
	}
	if audit.Verdict != "failed" || audit.Semantic.MultiHopSemanticFailureCount == 0 {
		t.Fatalf("multi-hop audit = %+v", audit.Semantic)
	}
}

func TestPublishAndVerifyRegressionUsesPrivateImmutableFiles(t *testing.T) {
	pool, err := GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir() + "/v2-regression"
	if err := PublishRegression(root, pool); err != nil {
		t.Fatal(err)
	}
	status, err := VerifyRegression(root)
	if err != nil {
		t.Fatal(err)
	}
	if status.CaseCount != 500 || status.AuditVerdict != "passed" || status.PromotionEligible {
		t.Fatalf("regression status = %+v", status)
	}
	for _, name := range []string{
		RegressionFixtureFile,
		RegressionCorpusFile,
		RegressionAuditFile,
		RegressionManifestFile,
	} {
		info, err := os.Stat(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %o", name, info.Mode().Perm())
		}
	}
	if err := PublishRegression(root, pool); err == nil {
		t.Fatal("PublishRegression() overwrote an existing root")
	}
}
