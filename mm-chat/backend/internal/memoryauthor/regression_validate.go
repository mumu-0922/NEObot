package memoryauthor

import (
	"bytes"
	"errors"
	"fmt"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

func validateRegressionFixtureManifest(manifest RegressionFixtureManifest) error {
	if manifest.SchemaVersion != RegressionFixtureSchemaVersion ||
		!validIdentifier(manifest.ID) || !validText(manifest.Description, 4096) ||
		manifest.CorpusClass != memoryeval.RegressionCorpusClass ||
		manifest.AdmissionMode != memoryeval.RegressionAdmissionMode ||
		manifest.PromotionEligible == nil || *manifest.PromotionEligible ||
		!validDataPolicy(manifest.DataPolicy) ||
		manifest.Generator != expectedRegressionGenerator() ||
		!validSHA256(manifest.ContentSHA256) || len(manifest.Fixtures) != 500 {
		return errors.New("regression fixture manifest header is invalid")
	}
	aliases := make(map[string]struct{}, len(manifest.Fixtures))
	memoryIDs := make(map[string]struct{})
	for _, fixture := range manifest.Fixtures {
		if !validIdentifier(fixture.Alias) || !validIdentifier(fixture.UserAlias) ||
			len(fixture.Memories) == 0 {
			return fmt.Errorf("regression fixture %q is invalid", fixture.Alias)
		}
		if _, duplicate := aliases[fixture.Alias]; duplicate {
			return fmt.Errorf("regression fixture alias %q is duplicated", fixture.Alias)
		}
		aliases[fixture.Alias] = struct{}{}
		for _, memory := range fixture.Memories {
			if err := validateFixtureMemory(memory); err != nil {
				return fmt.Errorf("regression fixture %q: %w", fixture.Alias, err)
			}
			if _, duplicate := memoryIDs[memory.ID]; duplicate {
				return fmt.Errorf("regression Memory ID %q is duplicated", memory.ID)
			}
			memoryIDs[memory.ID] = struct{}{}
		}
	}
	return nil
}

func validateRegressionManifest(manifest RegressionManifest) error {
	if manifest.SchemaVersion != RegressionManifestSchemaVersion ||
		!validIdentifier(manifest.ID) ||
		manifest.CorpusClass != memoryeval.RegressionCorpusClass ||
		manifest.AdmissionMode != memoryeval.RegressionAdmissionMode ||
		manifest.PromotionEligible == nil || *manifest.PromotionEligible ||
		!validDataPolicy(manifest.DataPolicy) ||
		manifest.Generator != expectedRegressionGenerator() ||
		manifest.CaseCount != 500 ||
		manifest.SplitCounts != (CountBySplit{Development: 300, Validation: 100, Holdout: 100}) ||
		manifest.LanguageCounts != (CountByLanguage{Chinese: 350, Mixed: 100, English: 50}) ||
		manifest.QuerySkeletonCount < 100 ||
		!validSHA256(manifest.FixtureContentSHA256) ||
		!validSHA256(manifest.FixtureRawSHA256) ||
		!validSHA256(manifest.CorpusContentSHA256) ||
		!validSHA256(manifest.CorpusRawSHA256) ||
		!validSHA256(manifest.AuditContentSHA256) ||
		!validSHA256(manifest.AuditRawSHA256) {
		return errors.New("regression manifest header is invalid")
	}
	critical := memoryeval.CriticalSlices()
	if len(manifest.SliceCounts) != len(critical) {
		return errors.New("regression manifest slice counts are incomplete")
	}
	for index, count := range manifest.SliceCounts {
		if count.Name != critical[index] || count.Total < 50 || count.Development < 30 ||
			count.Validation < 10 || count.Holdout < 10 ||
			count.Total != count.Development+count.Validation+count.Holdout {
			return fmt.Errorf("regression manifest slice %q is invalid", count.Name)
		}
	}
	return nil
}

func ValidateRegressionPool(pool RegressionPool) error {
	if err := validateRegressionFixtureManifest(pool.Fixtures); err != nil {
		return err
	}
	fixtureContentHash, err := RegressionFixtureContentSHA256(pool.Fixtures)
	if err != nil || fixtureContentHash != pool.Fixtures.ContentSHA256 {
		return errors.New("regression fixture content hash does not match")
	}
	fixtureJSON, err := marshalCanonical(pool.Fixtures)
	if err != nil {
		return err
	}
	corpusJSON, err := marshalCanonical(pool.Corpus)
	if err != nil {
		return err
	}
	auditJSON, err := marshalCanonical(pool.Audit)
	if err != nil {
		return err
	}
	manifestJSON, err := marshalCanonical(pool.Manifest)
	if err != nil {
		return err
	}
	for name, pair := range map[string][2][]byte{
		"fixture":  {pool.FixtureJSON, fixtureJSON},
		"corpus":   {pool.CorpusJSON, corpusJSON},
		"audit":    {pool.AuditJSON, auditJSON},
		"manifest": {pool.ManifestJSON, manifestJSON},
	} {
		if len(pair[0]) > 0 && !bytes.Equal(pair[0], pair[1]) {
			return fmt.Errorf("regression %s bytes are not canonical", name)
		}
	}
	if err := memoryeval.ValidateRegressionAdmission(pool.Corpus, pool.Audit); err != nil {
		return fmt.Errorf("admit regression corpus: %w", err)
	}
	if pool.Corpus.FixtureManifestSHA256 != fixtureContentHash {
		return errors.New("regression corpus fixture binding does not match")
	}
	if len(pool.Fixtures.Fixtures) != len(pool.Corpus.Cases) {
		return errors.New("regression fixture and corpus counts differ")
	}
	fixtures := make(map[string]Fixture, len(pool.Fixtures.Fixtures))
	for _, fixture := range pool.Fixtures.Fixtures {
		fixtures[fixture.Alias] = fixture
	}
	for _, item := range pool.Corpus.Cases {
		fixture, ok := fixtures[item.FixtureAlias]
		if !ok {
			return fmt.Errorf("regression case %q has no fixture", item.ID)
		}
		if err := regressionFixtureCaseBindingError(fixture, item); err != nil {
			return fmt.Errorf("regression case %q: %w", item.ID, err)
		}
	}
	replayedAudit, err := AuditRegression(pool.Fixtures, pool.Corpus)
	if err != nil {
		return err
	}
	replayedAuditJSON, err := marshalCanonical(replayedAudit)
	if err != nil || !bytes.Equal(replayedAuditJSON, auditJSON) {
		return errors.New("regression machine audit does not replay")
	}
	if err := validateRegressionManifest(pool.Manifest); err != nil {
		return err
	}
	manifest := pool.Manifest
	if manifest.CaseCount != pool.Audit.CaseCount ||
		manifest.SplitCounts != authorSplitCounts(pool.Audit.SplitCounts) ||
		manifest.LanguageCounts != authorLanguageCounts(pool.Audit.LanguageCounts) ||
		!sliceCountsEqual(manifest.SliceCounts, authorSliceCounts(pool.Audit.SliceCounts)) ||
		manifest.QuerySkeletonCount != pool.Audit.Semantic.QuerySkeletonCount ||
		manifest.FixtureContentSHA256 != fixtureContentHash ||
		manifest.FixtureRawSHA256 != sha256Hex(fixtureJSON) ||
		manifest.CorpusContentSHA256 != pool.Corpus.CorpusContentSHA256 ||
		manifest.CorpusRawSHA256 != sha256Hex(corpusJSON) ||
		manifest.AuditContentSHA256 != pool.Audit.ContentSHA256 ||
		manifest.AuditRawSHA256 != sha256Hex(auditJSON) {
		return errors.New("regression manifest artifact binding is invalid")
	}
	return nil
}
