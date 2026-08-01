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

func TestGenerateRegressionV4IsDeterministicAndSemanticallyAligned(t *testing.T) {
	first, err := GenerateRegressionV4()
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateRegressionV4()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.FixtureJSON, second.FixtureJSON) ||
		!bytes.Equal(first.CorpusJSON, second.CorpusJSON) ||
		!bytes.Equal(first.AuditJSON, second.AuditJSON) ||
		!bytes.Equal(first.ManifestJSON, second.ManifestJSON) {
		t.Fatal("GenerateRegressionV4() is not byte deterministic")
	}
	v4Hashes := map[string][2]string{
		"fixture content": {first.Fixtures.ContentSHA256, "579a9c18b63c80841da4165ed2e463168f5916d9a2dced8445a5ae0fd549bb82"},
		"corpus content": {
			first.Corpus.CorpusContentSHA256,
			"04afb09eedb09d3a12b55d25b8e5f6e81635951d0b13b768a9998ba1b5894918",
		},
		"audit content": {first.Audit.ContentSHA256, "1a0541248fe0adef53371b25fa1c1e8e93b75400b88792812263301422ef8377"},
		"fixture raw":   {sha256Hex(first.FixtureJSON), "46927aaff8291a8afabd4f6f9d743485d98227f734a9a3144817a016b4beedb6"},
		"corpus raw":    {sha256Hex(first.CorpusJSON), "6500aaa253fffb8c2dc66cef27462971ad1e50bc1451c07f2141ec8353185839"},
		"audit raw":     {sha256Hex(first.AuditJSON), "718567aa6f64ba40d9c0f5619ba97a45334f6e60611f4575e37a22faf121d5d3"},
		"manifest raw":  {sha256Hex(first.ManifestJSON), "5817c8752ee25f0d52fabb4d2163245236e84253078071c79786d5843fc6322e"},
	}
	for name, pair := range v4Hashes {
		if pair[0] != pair[1] {
			t.Fatalf("v4 %s SHA-256 = %s, want %s", name, pair[0], pair[1])
		}
	}
	if first.Manifest.Generator != semanticRegressionProfile().generator ||
		first.Audit.Auditor != RegressionSemanticAuditor ||
		first.Audit.AuditedAt != RegressionSemanticAuditedAt ||
		first.Manifest.CaseCount != 500 ||
		first.Manifest.SplitCounts != (CountBySplit{Development: 300, Validation: 100, Holdout: 100}) ||
		first.Manifest.LanguageCounts != (CountByLanguage{Chinese: 350, Mixed: 100, English: 50}) ||
		first.Audit.Verdict != "passed" || !regressionAuditPasses(first.Audit) {
		t.Fatalf("v4 regression profile = %+v audit = %+v", first.Manifest, first.Audit)
	}

	fixtureByAlias := make(map[string]Fixture, len(first.Fixtures.Fixtures))
	for _, fixture := range first.Fixtures.Fixtures {
		fixtureByAlias[fixture.Alias] = fixture
	}
	positiveCount := 0
	unrelatedCount := 0
	positiveSplits := map[string]int{}
	positiveLanguages := map[string]int{}
	unrelatedSplits := map[string]int{}
	unrelatedLanguages := map[string]int{}
	for _, item := range first.Corpus.Cases {
		fixture := fixtureByAlias[item.FixtureAlias]
		if !item.ExpectedNoMemory {
			positiveCount++
			positiveSplits[item.Split]++
			positiveLanguages[item.Language]++
			if !semanticPositiveSemanticsMatch(item, fixture) {
				t.Fatalf("v4 positive is not subject/value compatible: case=%s", item.ID)
			}
		}
		if !contains(item.Slices, "unrelated_negative") {
			continue
		}
		unrelatedCount++
		unrelatedSplits[item.Split]++
		unrelatedLanguages[item.Language]++
		if !semanticUnrelatedSemanticsMatch(item, fixture) {
			t.Fatalf("v4 unrelated negative is not deletion-invariant: case=%s", item.ID)
		}
		subjectIndex, ok := regressionSubjectIndex(item.Query)
		if !ok {
			t.Fatalf("v4 unrelated query lacks one exact subject: %q", item.Query)
		}
		candidate := strings.ToLower(regressionExcludedText(item, fixture, "irrelevant"))
		if strings.Contains(candidate, strings.ToLower(regressionSubjectsZH[subjectIndex])) ||
			strings.Contains(candidate, strings.ToLower(regressionSubjectsEN[subjectIndex])) {
			t.Fatalf("v4 unrelated candidate repeats queried subject: query=%q candidate=%q", item.Query, candidate)
		}
	}
	if positiveCount != 275 || unrelatedCount != 50 {
		t.Fatalf("v4 semantic cardinality: positive=%d unrelated=%d", positiveCount, unrelatedCount)
	}
	for _, name := range []string{"development", "validation", "holdout"} {
		if positiveSplits[name] == 0 || unrelatedSplits[name] == 0 {
			t.Fatalf("v4 split coverage: positives=%v unrelated=%v", positiveSplits, unrelatedSplits)
		}
	}
	for _, name := range []string{"zh", "mixed", "en"} {
		if positiveLanguages[name] == 0 || unrelatedLanguages[name] == 0 {
			t.Fatalf("v4 language coverage: positives=%v unrelated=%v", positiveLanguages, unrelatedLanguages)
		}
	}
}

func TestRegressionV4AuditRejectsSemanticMutations(t *testing.T) {
	t.Run("current value from another subject", func(t *testing.T) {
		pool, err := GenerateRegressionV4()
		if err != nil {
			t.Fatal(err)
		}
		for caseIndex, item := range pool.Corpus.Cases {
			if item.ExpectedNoMemory || !contains(item.Slices, "multi_hop") {
				continue
			}
			subjectIndex, ok := regressionSubjectIndex(item.Query)
			if !ok {
				t.Fatal("generated query does not identify one subject")
			}
			values := regressionSemanticCurrentValuesZH
			if item.Language == "en" {
				values = regressionSemanticCurrentValuesEN
			}
			memory := regressionFixtureMemoryByID(
				t,
				&pool.Fixtures.Fixtures[caseIndex],
				item.ExpectedRelevantMemoryIDs[0],
			)
			memory.CanonicalContent = strings.ReplaceAll(
				memory.CanonicalContent,
				values[subjectIndex],
				values[(subjectIndex+1)%len(values)],
			)
			break
		}
		assertRegressionV4SemanticAuditFails(t, pool)
	})

	t.Run("superseded value from another subject", func(t *testing.T) {
		pool, err := GenerateRegressionV4()
		if err != nil {
			t.Fatal(err)
		}
		for caseIndex, item := range pool.Corpus.Cases {
			if !contains(item.Slices, "temporal_correction") {
				continue
			}
			subjectIndex, ok := regressionSubjectIndex(item.Query)
			if !ok {
				t.Fatal("generated query does not identify one subject")
			}
			values := regressionSemanticOldValuesZH
			if item.Language == "en" {
				values = regressionSemanticOldValuesEN
			}
			var supersededID string
			for _, exclusion := range item.Exclusions {
				if exclusion.Reason == "superseded" {
					supersededID = exclusion.MemoryID
					break
				}
			}
			memory := regressionFixtureMemoryByID(t, &pool.Fixtures.Fixtures[caseIndex], supersededID)
			memory.CanonicalContent = strings.ReplaceAll(
				memory.CanonicalContent,
				values[subjectIndex],
				values[(subjectIndex+1)%len(values)],
			)
			break
		}
		assertRegressionV4SemanticAuditFails(t, pool)
	})

	t.Run("unrelated candidate claims exact task", func(t *testing.T) {
		pool, err := GenerateRegressionV4()
		if err != nil {
			t.Fatal(err)
		}
		for caseIndex, item := range pool.Corpus.Cases {
			if !contains(item.Slices, "unrelated_negative") {
				continue
			}
			subjectIndex, ok := regressionSubjectIndex(item.Query)
			if !ok {
				t.Fatal("generated query does not identify one subject")
			}
			memory := &pool.Fixtures.Fixtures[caseIndex].Memories[0]
			memory.CanonicalContent += " 该会议讨论了" + regressionSubjectsZH[subjectIndex] + "议程。"
			break
		}
		assertRegressionV4SemanticAuditFails(t, pool)
	})
}

func assertRegressionV4SemanticAuditFails(t *testing.T, pool RegressionPool) {
	t.Helper()
	audit, err := AuditRegression(pool.Fixtures, pool.Corpus)
	if err != nil {
		t.Fatal(err)
	}
	if audit.Verdict != "failed" || audit.Semantic.SliceSemanticFailureCount == 0 {
		t.Fatalf("v4 semantic mutation audit = %+v", audit.Semantic)
	}
}

func regressionFixtureMemoryByID(t *testing.T, fixture *Fixture, id string) *FixtureMemory {
	t.Helper()
	for index := range fixture.Memories {
		if fixture.Memories[index].ID == id {
			return &fixture.Memories[index]
		}
	}
	t.Fatalf("fixture %q does not contain Memory %q", fixture.Alias, id)
	return nil
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
	semantic, err := GenerateRegressionV4()
	if err != nil {
		t.Fatal(err)
	}
	versions := []struct {
		name string
		pool RegressionPool
	}{
		{name: "v2", pool: legacy},
		{name: "v3", pool: repaired},
		{name: "v4", pool: semantic},
	}
	for _, base := range versions {
		for _, donor := range versions {
			if base.name == donor.name {
				continue
			}
			for _, artifact := range []string{"fixtures", "corpus", "audit", "manifest"} {
				t.Run(base.name+"_with_"+donor.name+"_"+artifact, func(t *testing.T) {
					pool := base.pool
					switch artifact {
					case "fixtures":
						pool.Fixtures, pool.FixtureJSON = donor.pool.Fixtures, donor.pool.FixtureJSON
					case "corpus":
						pool.Corpus, pool.CorpusJSON = donor.pool.Corpus, donor.pool.CorpusJSON
					case "audit":
						pool.Audit, pool.AuditJSON = donor.pool.Audit, donor.pool.AuditJSON
					case "manifest":
						pool.Manifest, pool.ManifestJSON = donor.pool.Manifest, donor.pool.ManifestJSON
					}
					if err := ValidateRegressionPool(pool); err == nil {
						t.Fatalf("mixed %s/%s artifacts were accepted", base.name, donor.name)
					}
				})
			}
		}
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
		{name: "semantic", rootName: "v4-regression", profile: RegressionSemanticProfileID, generate: GenerateRegressionV4},
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
