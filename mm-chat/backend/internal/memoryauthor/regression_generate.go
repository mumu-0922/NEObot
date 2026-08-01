package memoryauthor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

type regressionGenerationProfile struct {
	fixtureID          string
	corpusID           string
	manifestID         string
	fixtureDescription string
	corpusDescription  string
	generator          GeneratorProvenance
	auditor            string
	auditedAt          string
	repairedUnrelated  bool
	strictSemantics    bool
	universalUnrelated bool
}

func GenerateRegression() (RegressionPool, error) {
	return generateRegression(legacyRegressionProfile())
}

// GenerateRegressionV3 creates the separately versioned corpus whose
// unrelated-negative cases are genuine hard negatives rather than
// self-referential Memory-governance questions.
func GenerateRegressionV3() (RegressionPool, error) {
	return generateRegression(repairedRegressionProfile())
}

// GenerateRegressionV4 creates the separately versioned corpus whose
// positive values are subject-compatible and whose unrelated candidates are
// deletion-invariant under the fixed prompt-v1 usefulness contract.
func GenerateRegressionV4() (RegressionPool, error) {
	return generateRegression(semanticRegressionProfile())
}

// GenerateRegressionV5 creates the separately versioned corpus whose hard
// negative is semantically disjoint from every benchmark Subject, not merely
// from the Subject selected by the current case.
func GenerateRegressionV5() (RegressionPool, error) {
	return generateRegression(universalRegressionProfile())
}

func generateRegression(profile regressionGenerationProfile) (RegressionPool, error) {
	drafts, err := buildRegressionPlan()
	if err != nil {
		return RegressionPool{}, err
	}
	fixtures := make([]Fixture, 0, len(drafts))
	cases := make([]memoryeval.GoldenCase, 0, len(drafts))
	for _, draft := range drafts {
		fixture, item := generateRegressionCase(draft, profile)
		fixtures = append(fixtures, fixture)
		cases = append(cases, item)
	}
	promotion := false
	fixtureManifest := RegressionFixtureManifest{
		SchemaVersion:     RegressionFixtureSchemaVersion,
		ID:                profile.fixtureID,
		Description:       profile.fixtureDescription,
		CorpusClass:       memoryeval.RegressionCorpusClass,
		AdmissionMode:     memoryeval.RegressionAdmissionMode,
		PromotionEligible: &promotion,
		DataPolicy:        DataPolicy{SyntheticOnly: true},
		Generator:         profile.generator,
		ContentSHA256:     strings.Repeat("0", 64),
		Fixtures:          fixtures,
	}
	fixtureContentHash, err := RegressionFixtureContentSHA256(fixtureManifest)
	if err != nil {
		return RegressionPool{}, err
	}
	fixtureManifest.ContentSHA256 = fixtureContentHash
	corpus := memoryeval.RegressionCorpus{
		SchemaVersion:         memoryeval.RegressionCorpusSchemaVersion,
		ID:                    profile.corpusID,
		Description:           profile.corpusDescription,
		CorpusClass:           memoryeval.RegressionCorpusClass,
		AdmissionMode:         memoryeval.RegressionAdmissionMode,
		PromotionEligible:     &promotion,
		DataPolicy:            memoryeval.DataPolicy{SyntheticOnly: true},
		FixtureManifestSHA256: fixtureContentHash,
		MachineAudit: memoryeval.RegressionAuditBinding{
			SchemaVersion: memoryeval.RegressionAuditSchemaVersion,
			Verdict:       "passed",
			Auditor:       profile.auditor,
			AuditedAt:     profile.auditedAt,
		},
		Criteria: benchmarkCriteria(),
		Cases:    cases,
	}
	corpusContentHash, err := memoryeval.RegressionCorpusContentSHA256(corpus)
	if err != nil {
		return RegressionPool{}, err
	}
	corpus.CorpusContentSHA256 = corpusContentHash
	audit, err := AuditRegression(fixtureManifest, corpus)
	if err != nil {
		return RegressionPool{}, err
	}
	corpus.MachineAudit.ContentSHA256 = audit.ContentSHA256

	fixtureJSON, err := marshalCanonical(fixtureManifest)
	if err != nil {
		return RegressionPool{}, err
	}
	corpusJSON, err := marshalCanonical(corpus)
	if err != nil {
		return RegressionPool{}, err
	}
	auditJSON, err := marshalCanonical(audit)
	if err != nil {
		return RegressionPool{}, err
	}
	manifest := RegressionManifest{
		SchemaVersion:        RegressionManifestSchemaVersion,
		ID:                   profile.manifestID,
		CorpusClass:          memoryeval.RegressionCorpusClass,
		AdmissionMode:        memoryeval.RegressionAdmissionMode,
		PromotionEligible:    &promotion,
		DataPolicy:           DataPolicy{SyntheticOnly: true},
		Generator:            profile.generator,
		CaseCount:            audit.CaseCount,
		SplitCounts:          authorSplitCounts(audit.SplitCounts),
		LanguageCounts:       authorLanguageCounts(audit.LanguageCounts),
		SliceCounts:          authorSliceCounts(audit.SliceCounts),
		QuerySkeletonCount:   audit.Semantic.QuerySkeletonCount,
		FixtureContentSHA256: fixtureContentHash,
		FixtureRawSHA256:     sha256Hex(fixtureJSON),
		CorpusContentSHA256:  corpusContentHash,
		CorpusRawSHA256:      sha256Hex(corpusJSON),
		AuditContentSHA256:   audit.ContentSHA256,
		AuditRawSHA256:       sha256Hex(auditJSON),
	}
	manifestJSON, err := marshalCanonical(manifest)
	if err != nil {
		return RegressionPool{}, err
	}
	pool := RegressionPool{
		Fixtures: fixtureManifest, Corpus: corpus, Audit: audit, Manifest: manifest,
		FixtureJSON: fixtureJSON, CorpusJSON: corpusJSON, AuditJSON: auditJSON,
		ManifestJSON: manifestJSON,
	}
	if err := ValidateRegressionPool(pool); err != nil {
		return RegressionPool{}, fmt.Errorf("validate generated regression pool: %w", err)
	}
	return pool, nil
}

func buildRegressionPlan() ([]regressionDraft, error) {
	drafts := make([]regressionDraft, 0, 500)
	global := 0
	for _, profile := range regressionSplits {
		start := len(drafts)
		for index := 0; index < profile.total; index++ {
			drafts = append(drafts, regressionDraft{
				index:   global,
				split:   profile.name,
				primary: regressionCoreSlices[index%len(regressionCoreSlices)],
				slices: map[string]struct{}{
					regressionCoreSlices[index%len(regressionCoreSlices)]: {},
				},
			})
			global++
		}
		assignRegressionLanguages(drafts[start:], profile)
		for index := start; index < len(drafts); index++ {
			switch drafts[index].language {
			case "zh":
				drafts[index].slices["chinese_paraphrase"] = struct{}{}
			case "mixed":
				drafts[index].slices["mixed_language_entity"] = struct{}{}
			}
		}
		for _, target := range regressionCoreSlices {
			if err := addRegressionCoverage(drafts[start:], target, profile.poolSliceMin); err != nil {
				return nil, fmt.Errorf("%s: %w", profile.name, err)
			}
		}
	}
	return drafts, nil
}

func assignRegressionLanguages(drafts []regressionDraft, profile splitProfile) {
	remaining := map[string]int{"zh": profile.zh, "mixed": profile.mixed, "en": profile.en}
	for _, language := range []string{"zh", "mixed", "en"} {
		for index := range drafts {
			if remaining[language] == 0 {
				break
			}
			if drafts[index].language != "" {
				continue
			}
			drafts[index].language = language
			remaining[language]--
		}
	}
}

func addRegressionCoverage(drafts []regressionDraft, target string, minimum int) error {
	count := 0
	for _, draft := range drafts {
		if _, ok := draft.slices[target]; ok {
			count++
		}
	}
	for index := range drafts {
		if count >= minimum {
			return nil
		}
		if _, exists := drafts[index].slices[target]; exists ||
			!regressionSlicesCompatible(drafts[index].slices, target) {
			continue
		}
		drafts[index].slices[target] = struct{}{}
		count++
	}
	return fmt.Errorf("cannot satisfy slice %s coverage", target)
}

func regressionSlicesCompatible(actual map[string]struct{}, target string) bool {
	targetNegative := isNegativeSlice(target)
	targetPositive := isRegressionPositiveSlice(target)
	for name := range actual {
		if targetNegative && isRegressionPositiveSlice(name) {
			return false
		}
		if targetPositive && isNegativeSlice(name) {
			return false
		}
	}
	return true
}

func isRegressionPositiveSlice(name string) bool {
	switch name {
	case "stable_fact", "preference_instruction", "project_decision",
		"temporal_correction", "failure_fallback", "multi_hop":
		return true
	default:
		return false
	}
}

func regressionHasSlice(draft regressionDraft, name string) bool {
	_, ok := draft.slices[name]
	return ok
}

func hasRegressionNegative(draft regressionDraft) bool {
	for name := range draft.slices {
		if isNegativeSlice(name) {
			return true
		}
	}
	return false
}

func opaqueRegressionIDForSeed(seed string, kind string, index int) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", seed, kind, index)))
	return kind + "-" + hex.EncodeToString(digest[:8])
}

func legacyRegressionProfile() regressionGenerationProfile {
	return regressionGenerationProfile{
		fixtureID:          "memory-regression-v2-fixtures",
		corpusID:           "memory-regression-v2-corpus",
		manifestID:         "memory-regression-v2-manifest",
		fixtureDescription: "Deterministic synthetic fixtures for the machine-reviewed regression corpus.",
		corpusDescription:  "Machine-reviewed synthetic regression corpus; never promotion evidence.",
		generator: GeneratorProvenance{
			Version: RegressionGeneratorVersion,
			Profile: RegressionProfileID,
			Seed:    RegressionProfileSeed,
		},
		auditor:   RegressionAuditor,
		auditedAt: RegressionAuditedAt,
	}
}

func repairedRegressionProfile() regressionGenerationProfile {
	return regressionGenerationProfile{
		fixtureID:          "memory-regression-v3-fixtures",
		corpusID:           "memory-regression-v3-corpus",
		manifestID:         "memory-regression-v3-manifest",
		fixtureDescription: "Deterministic synthetic fixtures for the repaired machine-reviewed regression corpus.",
		corpusDescription: "Machine-reviewed synthetic regression corpus with non-self-referential " +
			"hard negatives; never promotion evidence.",
		generator: GeneratorProvenance{
			Version: RegressionRepairedGeneratorVersion,
			Profile: RegressionRepairedProfileID,
			Seed:    RegressionRepairedProfileSeed,
		},
		auditor:           RegressionRepairedAuditor,
		auditedAt:         RegressionRepairedAuditedAt,
		repairedUnrelated: true,
	}
}

func semanticRegressionProfile() regressionGenerationProfile {
	return regressionGenerationProfile{
		fixtureID:          "memory-regression-v4-fixtures",
		corpusID:           "memory-regression-v4-corpus",
		manifestID:         "memory-regression-v4-manifest",
		fixtureDescription: "Deterministic synthetic fixtures for the semantically aligned machine-reviewed regression corpus.",
		corpusDescription:  "Machine-reviewed synthetic regression corpus with compatible positive facts and deletion-invariant hard negatives; never promotion evidence.",
		generator: GeneratorProvenance{
			Version: RegressionSemanticGeneratorVersion,
			Profile: RegressionSemanticProfileID,
			Seed:    RegressionSemanticProfileSeed,
		},
		auditor:           RegressionSemanticAuditor,
		auditedAt:         RegressionSemanticAuditedAt,
		repairedUnrelated: true,
		strictSemantics:   true,
	}
}

func universalRegressionProfile() regressionGenerationProfile {
	return regressionGenerationProfile{
		fixtureID:          "memory-regression-v5-fixtures",
		corpusID:           "memory-regression-v5-corpus",
		manifestID:         "memory-regression-v5-manifest",
		fixtureDescription: "Deterministic synthetic fixtures for the universally unrelated machine-reviewed regression corpus.",
		corpusDescription:  "Machine-reviewed synthetic regression corpus with compatible positive facts and universally unrelated hard negatives; never promotion evidence.",
		generator: GeneratorProvenance{
			Version: RegressionUniversalGeneratorVersion,
			Profile: RegressionUniversalProfileID,
			Seed:    RegressionUniversalProfileSeed,
		},
		auditor:            RegressionUniversalAuditor,
		auditedAt:          RegressionUniversalAuditedAt,
		repairedUnrelated:  true,
		strictSemantics:    true,
		universalUnrelated: true,
	}
}

func regressionProfileForGenerator(
	generator GeneratorProvenance,
) (regressionGenerationProfile, bool) {
	for _, profile := range []regressionGenerationProfile{
		legacyRegressionProfile(),
		repairedRegressionProfile(),
		semanticRegressionProfile(),
		universalRegressionProfile(),
	} {
		if generator == profile.generator {
			return profile, true
		}
	}
	return regressionGenerationProfile{}, false
}

func benchmarkCriteria() memoryeval.Criteria {
	return memoryeval.Criteria{
		MinimumCandidateRecallAt20:       0.95,
		MinimumFinalRecallAt5:            0.90,
		MinimumCurrentFactAccuracy:       0.95,
		MaximumFalseInjectionRate:        0.02,
		MaximumP95LatencyMilliseconds:    900,
		MaximumP99LatencyMilliseconds:    1500,
		HardCutoffMilliseconds:           2000,
		MaximumAveragePromptMemoryTokens: 600,
		MaximumPromptMemoryTokens:        900,
		MaximumProviderCostRatio:         0.15,
	}
}

func authorSplitCounts(value memoryeval.RegressionSplitCounts) CountBySplit {
	return CountBySplit{Development: value.Development, Validation: value.Validation, Holdout: value.Holdout}
}

func authorLanguageCounts(value memoryeval.RegressionLanguageCounts) CountByLanguage {
	return CountByLanguage{Chinese: value.Chinese, Mixed: value.Mixed, English: value.English}
}

func authorSliceCounts(values []memoryeval.RegressionSliceCount) []SliceCount {
	result := make([]SliceCount, 0, len(values))
	for _, value := range values {
		result = append(result, SliceCount{
			Name: value.Name, Total: value.Total, Development: value.Development,
			Validation: value.Validation, Holdout: value.Holdout,
		})
	}
	return result
}
