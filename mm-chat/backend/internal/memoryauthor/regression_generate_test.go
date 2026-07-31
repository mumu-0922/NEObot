package memoryauthor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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

func TestGenerateRegressionLegacyHashesRemainFrozen(t *testing.T) {
	pool, err := GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"fixture content": "965072e0e5b36d687c1838aaa675beb8a81475e0794d7fdded34f99d7673c7af",
		"corpus content":  "5ae712163fc7bf81efe754b877cd02b1d0de74450a04ea7e119402320e4ba144",
		"audit content":   "c0f5b9406acc707f087afc7caf6f5f3e1dbc50aa6b1492b7ef1ec5479fe051f3",
		"fixture raw":     "f2f51a7a72f99dc66b2b0d3a30a34775f9dca2baee6232dbbbb66fb171ec8a3c",
		"corpus raw":      "51401414b6f71f4052ddc7084185d62e39eeeb2fd47ab8826382b48893185be5",
		"audit raw":       "4c2ac1dd6b54cd0025178bfa0d058666607eeea9c9469e51f6cfa0bfca3108ce",
		"manifest raw":    "22148741a0063554ee3566c658dfa99bab9e55b8b1b85e7e6db0ae3a7752162a",
	}
	got := map[string]string{
		"fixture content": pool.Fixtures.ContentSHA256,
		"corpus content":  pool.Corpus.CorpusContentSHA256,
		"audit content":   pool.Audit.ContentSHA256,
		"fixture raw":     sha256Hex(pool.FixtureJSON),
		"corpus raw":      sha256Hex(pool.CorpusJSON),
		"audit raw":       sha256Hex(pool.AuditJSON),
		"manifest raw":    sha256Hex(pool.ManifestJSON),
	}
	for name, expected := range want {
		if got[name] != expected {
			t.Fatalf("legacy %s SHA-256 = %s, want %s", name, got[name], expected)
		}
	}
}

func TestGenerateRegressionV3IsDeterministicAndUsesGenuineHardNegatives(t *testing.T) {
	first, err := GenerateRegressionV3()
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateRegressionV3()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.FixtureJSON, second.FixtureJSON) ||
		!bytes.Equal(first.CorpusJSON, second.CorpusJSON) ||
		!bytes.Equal(first.AuditJSON, second.AuditJSON) ||
		!bytes.Equal(first.ManifestJSON, second.ManifestJSON) {
		t.Fatal("GenerateRegressionV3() is not byte deterministic")
	}
	v3Hashes := map[string][2]string{
		"fixture content": {first.Fixtures.ContentSHA256, "49cae3861be4eade46c8d042ab4d4d4c3d779ba95a6c6b29cf77de5374e7c71a"},
		"corpus content": {
			first.Corpus.CorpusContentSHA256,
			"cfa666c0771f6375058a23d117613b57f32beb22ae460eb16db50ddb325897df",
		},
		"audit content": {first.Audit.ContentSHA256, "2803504064da8625e78c60dcad5043adfc7cf1cde2079daa3f60fd060ccd68f9"},
		"fixture raw":   {sha256Hex(first.FixtureJSON), "f563f5876c432c0b57df2d8107f284a53d1bdb78ec201e4d0279b5161be6c98e"},
		"corpus raw":    {sha256Hex(first.CorpusJSON), "359c3cb3ce98a20fbe4615b0bcd013e96559103fd4c73b40c23c36089a78e9a2"},
		"audit raw":     {sha256Hex(first.AuditJSON), "d160dce71822f078805528c21105c7df2d6c254fb0997aae97f9aa0094da180e"},
		"manifest raw": {
			sha256Hex(first.ManifestJSON),
			"dd87967b2d2e2c48c6c21c2b17b2b0d0b2cecd6c3998b23dbee351e92056bf09",
		},
	}
	for name, pair := range v3Hashes {
		if pair[0] != pair[1] {
			t.Fatalf("v3 %s SHA-256 = %s, want %s", name, pair[0], pair[1])
		}
	}
	if first.Manifest.Generator != repairedRegressionProfile().generator ||
		first.Audit.Auditor != RegressionRepairedAuditor ||
		first.Audit.AuditedAt != RegressionRepairedAuditedAt ||
		first.Manifest.CaseCount != 500 ||
		first.Manifest.SplitCounts != (CountBySplit{Development: 300, Validation: 100, Holdout: 100}) ||
		first.Manifest.LanguageCounts != (CountByLanguage{Chinese: 350, Mixed: 100, English: 50}) ||
		first.Audit.Verdict != "passed" {
		t.Fatalf("v3 regression profile = %+v audit = %+v", first.Manifest, first.Audit)
	}

	fixtureByAlias := make(map[string]Fixture, len(first.Fixtures.Fixtures))
	for _, fixture := range first.Fixtures.Fixtures {
		fixtureByAlias[fixture.Alias] = fixture
	}
	unrelatedBySplit := map[string]int{}
	unrelatedByLanguage := map[string]int{}
	for _, item := range first.Corpus.Cases {
		if !contains(item.Slices, "unrelated_negative") {
			continue
		}
		unrelatedBySplit[item.Split]++
		unrelatedByLanguage[item.Language]++
		candidate := regressionExcludedText(item, fixtureByAlias[item.FixtureAlias], "irrelevant")
		combined := strings.ToLower(item.Query + " " + candidate)
		for _, forbidden := range []string{"unrelated", "无关", "no bearing", "没有关系"} {
			if strings.Contains(combined, forbidden) {
				t.Fatalf("v3 unrelated-negative contains self-referential term %q", forbidden)
			}
		}
		if item.Language == "en" {
			if !strings.Contains(item.Query, "agenda heading") || !strings.Contains(candidate, "weather board") {
				t.Fatalf("v3 English hard negative lacks semantic markers: query=%q candidate=%q", item.Query, candidate)
			}
		} else if !strings.Contains(item.Query, "议程标题") || !strings.Contains(candidate, "天气牌") {
			t.Fatalf("v3 CJK hard negative lacks semantic markers: query=%q candidate=%q", item.Query, candidate)
		}
	}
	if unrelatedBySplit["development"] != 30 || unrelatedBySplit["validation"] != 10 ||
		unrelatedBySplit["holdout"] != 10 || unrelatedByLanguage["zh"] == 0 ||
		unrelatedByLanguage["mixed"] == 0 || unrelatedByLanguage["en"] == 0 ||
		unrelatedByLanguage["zh"]+unrelatedByLanguage["mixed"]+unrelatedByLanguage["en"] != 50 {
		t.Fatalf("v3 unrelated-negative distribution: splits=%v languages=%v", unrelatedBySplit, unrelatedByLanguage)
	}
	for _, count := range first.Manifest.SliceCounts {
		if count.Total < 50 || count.Development < 30 || count.Validation < 10 || count.Holdout < 10 {
			t.Fatalf("v3 slice count = %+v", count)
		}
	}
}

func TestRegressionV3AuditRejectsLegacySelfReferentialNegative(t *testing.T) {
	pool, err := GenerateRegressionV3()
	if err != nil {
		t.Fatal(err)
	}
	for index, item := range pool.Corpus.Cases {
		if !contains(item.Slices, "unrelated_negative") {
			continue
		}
		pool.Corpus.Cases[index].Query = "Should an unrelated note influence Atlas's document-format work?"
		for memoryIndex := range pool.Fixtures.Fixtures[index].Memories {
			memory := &pool.Fixtures.Fixtures[index].Memories[memoryIndex]
			if memory.State == StateIrrelevant {
				memory.CanonicalContent = "An unrelated weather note that has no bearing on Atlas's document-format work."
				break
			}
		}
		break
	}
	audit, err := AuditRegression(pool.Fixtures, pool.Corpus)
	if err != nil {
		t.Fatal(err)
	}
	if audit.Verdict != "failed" || audit.Semantic.SliceSemanticFailureCount == 0 {
		t.Fatalf("v3 repaired semantic audit = %+v", audit.Semantic)
	}
}

func TestValidateRegressionPoolRejectsMixedGeneratorArtifacts(t *testing.T) {
	legacy, err := GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	repaired, err := GenerateRegressionV3()
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*RegressionPool){
		"fixtures": func(pool *RegressionPool) {
			pool.Fixtures, pool.FixtureJSON = repaired.Fixtures, repaired.FixtureJSON
		},
		"corpus": func(pool *RegressionPool) {
			pool.Corpus, pool.CorpusJSON = repaired.Corpus, repaired.CorpusJSON
		},
		"audit": func(pool *RegressionPool) {
			pool.Audit, pool.AuditJSON = repaired.Audit, repaired.AuditJSON
		},
		"manifest": func(pool *RegressionPool) {
			pool.Manifest, pool.ManifestJSON = repaired.Manifest, repaired.ManifestJSON
		},
	}
	for name, mix := range tests {
		t.Run(name, func(t *testing.T) {
			pool := legacy
			mix(&pool)
			if err := ValidateRegressionPool(pool); err == nil {
				t.Fatal("mixed legacy/v3 artifacts were accepted")
			}
		})
	}
}

func TestRegressionRejectsUnknownGeneratorTuple(t *testing.T) {
	pool, err := GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	pool.Fixtures.Generator.Profile = "memory-regression-unknown"
	if _, err := AuditRegression(pool.Fixtures, pool.Corpus); err == nil ||
		!strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unknown audit generator error = %v", err)
	}
	if err := ValidateRegressionPool(pool); err == nil {
		t.Fatal("unknown regression generator was accepted")
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
	tests := []struct {
		name     string
		rootName string
		profile  string
		generate func() (RegressionPool, error)
	}{
		{name: "legacy", rootName: "v2-regression", profile: RegressionProfileID, generate: GenerateRegression},
		{name: "repaired", rootName: "v3-regression", profile: RegressionRepairedProfileID, generate: GenerateRegressionV3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pool, err := test.generate()
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(t.TempDir(), test.rootName)
			if err := PublishRegression(root, pool); err != nil {
				t.Fatal(err)
			}
			status, err := VerifyRegression(root)
			if err != nil {
				t.Fatal(err)
			}
			if status.CaseCount != 500 || status.AuditVerdict != "passed" ||
				status.PromotionEligible || status.Profile != test.profile {
				t.Fatalf("regression status = %+v", status)
			}
			rootInfo, err := os.Stat(root)
			if err != nil {
				t.Fatal(err)
			}
			if rootInfo.Mode().Perm() != 0o700 {
				t.Fatalf("regression root mode = %o", rootInfo.Mode().Perm())
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
		})
	}
}
