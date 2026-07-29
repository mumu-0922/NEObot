package memoryauthor

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

const (
	maximumFixtures  = 1_000
	maximumMemories  = 10_000
	maximumTextRunes = 16_384
)

func validateFixtureManifest(manifest FixtureManifest) error {
	if manifest.SchemaVersion != FixtureSchemaVersion ||
		!validIdentifier(manifest.ID) || !validText(manifest.Description, 4_096) ||
		manifest.PromotionEligible == nil || *manifest.PromotionEligible ||
		!validDataPolicy(manifest.DataPolicy) ||
		manifest.Generator != expectedGenerator() ||
		!validSHA256(manifest.ContentSHA256) || len(manifest.Fixtures) == 0 ||
		len(manifest.Fixtures) > maximumFixtures {
		return errors.New("fixture manifest header is invalid")
	}
	aliases := make(map[string]struct{}, len(manifest.Fixtures))
	memoryIDs := make(map[string]struct{})
	totalMemories := 0
	for _, fixture := range manifest.Fixtures {
		if !validIdentifier(fixture.Alias) || !validIdentifier(fixture.UserAlias) ||
			len(fixture.Memories) == 0 {
			return fmt.Errorf("fixture %q is invalid", fixture.Alias)
		}
		if _, duplicate := aliases[fixture.Alias]; duplicate {
			return fmt.Errorf("fixture alias %q is duplicated", fixture.Alias)
		}
		aliases[fixture.Alias] = struct{}{}
		totalMemories += len(fixture.Memories)
		if totalMemories > maximumMemories {
			return errors.New("fixture manifest has too many memories")
		}
		for _, memory := range fixture.Memories {
			if err := validateFixtureMemory(memory); err != nil {
				return fmt.Errorf("fixture %q: %w", fixture.Alias, err)
			}
			if _, duplicate := memoryIDs[memory.ID]; duplicate {
				return fmt.Errorf("logical Memory ID %q is duplicated", memory.ID)
			}
			memoryIDs[memory.ID] = struct{}{}
		}
	}
	return nil
}

func validateFixtureMemory(memory FixtureMemory) error {
	if !validIdentifier(memory.ID) || !validIdentifier(memory.UserAlias) ||
		!validText(memory.CanonicalContent, maximumTextRunes) ||
		!validText(memory.RawEventContent, maximumTextRunes) ||
		!validScope(memory.Scope) || memory.UserAlias != memory.Scope.UserAlias {
		return fmt.Errorf("Memory %q content, owner, or scope is invalid", memory.ID)
	}
	if _, err := time.Parse(time.RFC3339, memory.OccurredAt); err != nil {
		return fmt.Errorf("Memory %q occurredAt is invalid", memory.ID)
	}
	switch memory.State {
	case StateActive, StateSuperseded, StateDeleted, StateSecretRejected,
		StateUntrustedRejected, StateIrrelevant, StateOutOfScope:
	default:
		return fmt.Errorf("Memory %q state is invalid", memory.ID)
	}
	return nil
}

func validateCandidateManifest(manifest CandidateManifest) error {
	if manifest.SchemaVersion != CandidateSchemaVersion || !validIdentifier(manifest.ID) ||
		manifest.PromotionEligible == nil || *manifest.PromotionEligible ||
		!validDataPolicy(manifest.DataPolicy) || manifest.Generator != expectedGenerator() ||
		manifest.CaseCount != 650 || manifest.SplitCounts != (CountBySplit{390, 130, 130}) ||
		manifest.LanguageCounts != (CountByLanguage{455, 130, 65}) ||
		!validSHA256(manifest.FixtureContentSHA256) ||
		!validSHA256(manifest.FixtureRawSHA256) ||
		!validSHA256(manifest.GoldenRawSHA256) ||
		!validSHA256(manifest.FeasibilityWitnessSHA256) {
		return errors.New("candidate manifest header is invalid")
	}
	critical := memoryeval.CriticalSlices()
	if len(manifest.SliceCounts) != len(critical) {
		return errors.New("candidate manifest slice counts are incomplete")
	}
	for index, count := range manifest.SliceCounts {
		if count.Name != critical[index] || count.Total < 65 || count.Development < 39 ||
			count.Validation < 13 || count.Holdout < 13 ||
			count.Total != count.Development+count.Validation+count.Holdout {
			return fmt.Errorf("candidate manifest slice %q is invalid", count.Name)
		}
	}
	return nil
}

func ValidatePool(pool GeneratedPool) error {
	if err := validateFixtureManifest(pool.FixtureManifest); err != nil {
		return err
	}
	fixtureContentHash, err := FixtureContentSHA256(pool.FixtureManifest)
	if err != nil || fixtureContentHash != pool.FixtureManifest.ContentSHA256 {
		return errors.New("fixture manifest content hash does not match")
	}
	fixtureJSON, err := marshalCanonical(pool.FixtureManifest)
	if err != nil {
		return err
	}
	goldenJSON, err := marshalCanonical(pool.Golden)
	if err != nil {
		return err
	}
	manifestJSON, err := marshalCanonical(pool.Manifest)
	if err != nil {
		return err
	}
	if len(pool.FixtureJSON) > 0 && !bytes.Equal(pool.FixtureJSON, fixtureJSON) {
		return errors.New("fixture bytes are not canonical")
	}
	if len(pool.GoldenJSON) > 0 && !bytes.Equal(pool.GoldenJSON, goldenJSON) {
		return errors.New("Golden bytes are not canonical")
	}
	if len(pool.ManifestJSON) > 0 && !bytes.Equal(pool.ManifestJSON, manifestJSON) {
		return errors.New("candidate manifest bytes are not canonical")
	}
	decodedGolden, err := memoryeval.DecodeGoldenSet(bytes.NewReader(goldenJSON))
	if err != nil {
		return fmt.Errorf("validate candidate Golden: %w", err)
	}
	if decodedGolden.Lifecycle.State != "draft" || len(decodedGolden.Cases) != 650 ||
		decodedGolden.FixtureManifestSHA256 != fixtureContentHash {
		return errors.New("candidate Golden binding is invalid")
	}
	if err := validateGoldenFixtureBinding(pool.FixtureManifest, decodedGolden); err != nil {
		return err
	}
	diagnostic, err := Diagnose(pool.FixtureManifest, decodedGolden)
	if err != nil {
		return err
	}
	if err := validateCandidateManifest(pool.Manifest); err != nil {
		return err
	}
	if pool.Manifest.CaseCount != diagnostic.CaseCount ||
		pool.Manifest.SplitCounts != diagnostic.SplitCounts ||
		pool.Manifest.LanguageCounts != diagnostic.LanguageCounts ||
		!sliceCountsEqual(pool.Manifest.SliceCounts, diagnostic.SliceCounts) ||
		pool.Manifest.FixtureContentSHA256 != fixtureContentHash ||
		pool.Manifest.FixtureRawSHA256 != sha256Hex(fixtureJSON) ||
		pool.Manifest.GoldenRawSHA256 != sha256Hex(goldenJSON) ||
		pool.Manifest.FeasibilityWitnessSHA256 != witnessHash(diagnostic.WitnessCaseIDs) {
		return errors.New("candidate manifest artifact binding is invalid")
	}
	return nil
}

func validateGoldenFixtureBinding(manifest FixtureManifest, golden memoryeval.GoldenSet) error {
	fixtures := make(map[string]Fixture, len(manifest.Fixtures))
	for _, fixture := range manifest.Fixtures {
		fixtures[fixture.Alias] = fixture
	}
	for _, item := range golden.Cases {
		fixture, ok := fixtures[item.FixtureAlias]
		if !ok || fixture.UserAlias != item.Scope.UserAlias {
			return fmt.Errorf("Golden case %q fixture authority does not match", item.ID)
		}
		owned := make(map[string]FixtureMemory, len(fixture.Memories))
		for _, memory := range fixture.Memories {
			owned[memory.ID] = memory
		}
		for _, memoryID := range item.ExpectedRelevantMemoryIDs {
			memory, ok := owned[memoryID]
			if !ok || memory.State != StateActive || memory.UserAlias != item.Scope.UserAlias ||
				!scopeContains(item.Scope, memory.Scope) {
				return fmt.Errorf("Golden case %q expected Memory %q is not active and authorized", item.ID, memoryID)
			}
		}
		for _, exclusion := range item.Exclusions {
			memory, ok := owned[exclusion.MemoryID]
			if !ok || !exclusionMatches(exclusion.Reason, item.Scope, memory) {
				return fmt.Errorf("Golden case %q exclusion %q is not evidenced", item.ID, exclusion.MemoryID)
			}
		}
	}
	return nil
}

func exclusionMatches(reason string, scope memoryeval.Scope, memory FixtureMemory) bool {
	switch reason {
	case "cross_user":
		return memory.UserAlias != scope.UserAlias
	case "out_of_scope":
		return memory.UserAlias == scope.UserAlias &&
			(memory.State == StateOutOfScope || !scopeContains(scope, memory.Scope))
	case "deleted":
		return memory.State == StateDeleted
	case "irrelevant":
		return memory.State == StateIrrelevant
	case "secret":
		return memory.State == StateSecretRejected
	case "superseded":
		return memory.State == StateSuperseded
	case "untrusted_source":
		return memory.State == StateUntrustedRejected
	default:
		return false
	}
}

func scopeContains(query, memory memoryeval.Scope) bool {
	if query.UserAlias == "" || query.UserAlias != memory.UserAlias {
		return false
	}
	if memory.ConversationAlias != "" {
		return query.ConversationAlias != "" &&
			query.ConversationAlias == memory.ConversationAlias
	}
	if memory.ProjectAlias != "" {
		return query.ProjectAlias != "" && query.ProjectAlias == memory.ProjectAlias
	}
	return true
}

func validScope(scope memoryeval.Scope) bool {
	if !validIdentifier(scope.UserAlias) {
		return false
	}
	if scope.ProjectAlias != "" && !validIdentifier(scope.ProjectAlias) {
		return false
	}
	if scope.ConversationAlias != "" &&
		(!validIdentifier(scope.ConversationAlias) || scope.ProjectAlias == "") {
		return false
	}
	return true
}

func validDataPolicy(policy DataPolicy) bool {
	return policy.SyntheticOnly && !policy.ContainsRealUserData &&
		!policy.ContainsSensitiveData
}

func expectedGenerator() GeneratorProvenance {
	return GeneratorProvenance{Version: GeneratorVersion, Profile: ProfileID, Seed: ProfileSeed}
}

func validIdentifier(value string) bool {
	if strings.TrimSpace(value) != value || value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func validText(value string, maximumRunes int) bool {
	if strings.TrimSpace(value) != value || value == "" || !utf8.ValidString(value) ||
		utf8.RuneCountInString(value) > maximumRunes {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) && character != '\n' {
			return false
		}
	}
	return true
}

func validUUID(value string) bool {
	if len(value) != 36 || strings.ToLower(value) != value {
		return false
	}
	for index, character := range value {
		switch index {
		case 8, 13, 18, 23:
			if character != '-' {
				return false
			}
		default:
			if !strings.ContainsRune("0123456789abcdef", character) {
				return false
			}
		}
	}
	return true
}

func validTimestamp(value string) bool {
	if strings.TrimSpace(value) != value || value == "" {
		return false
	}
	_, err := time.Parse(time.RFC3339, value)
	return err == nil
}

func sliceCountsEqual(left, right []SliceCount) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func witnessHash(ids []string) string {
	return sha256Hex([]byte(strings.Join(ids, "\n") + "\n"))
}
