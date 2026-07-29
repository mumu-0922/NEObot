package hindsightfixture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

const (
	maximumManifestBytes = 8 << 20
	maximumFixtures      = 600
	maximumMemories      = 10_000
	maximumContentRunes  = 16_384
)

func DecodeManifest(reader io.Reader) (Manifest, error) {
	manifest, err := DecodeManifestForHash(reader)
	if err != nil {
		return Manifest{}, err
	}
	digest, err := ManifestContentSHA256(manifest)
	if err != nil {
		return Manifest{}, err
	}
	if manifest.ContentSHA256 != digest {
		return Manifest{}, errors.New("fixture manifest content hash does not match")
	}
	return manifest, nil
}

// DecodeManifestForHash validates the complete schema and policy but permits a
// syntactically valid placeholder content hash for the offline authoring flow.
func DecodeManifestForHash(reader io.Reader) (Manifest, error) {
	if reader == nil {
		return Manifest{}, errors.New("fixture manifest input is required")
	}
	body, err := io.ReadAll(io.LimitReader(reader, maximumManifestBytes+1))
	if err != nil {
		return Manifest{}, errors.New("read fixture manifest")
	}
	if len(body) == 0 || len(body) > maximumManifestBytes {
		return Manifest{}, errors.New("fixture manifest size is invalid")
	}
	var manifest Manifest
	if err := strictjson.Decode(body, maximumManifestBytes, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode fixture manifest: %w", err)
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// ManifestContentSHA256 hashes canonical JSON after clearing the
// self-referential contentSha256 field.
func ManifestContentSHA256(manifest Manifest) (string, error) {
	manifest.ContentSHA256 = ""
	body, err := json.Marshal(manifest)
	if err != nil {
		return "", errors.New("encode fixture manifest")
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func ValidateGoldenBinding(
	manifest Manifest,
	golden memoryeval.GoldenSet,
	goldenRawSHA256 string,
) error {
	if err := validateManifest(manifest); err != nil {
		return err
	}
	digest, err := ManifestContentSHA256(manifest)
	if err != nil || digest != manifest.ContentSHA256 {
		return errors.New("fixture manifest content hash does not match")
	}
	if !validSHA256(goldenRawSHA256) {
		return errors.New("Golden raw hash is invalid")
	}
	if golden.PromotionEligible == nil || *golden.PromotionEligible ||
		!golden.DataPolicy.SyntheticOnly || golden.DataPolicy.ContainsRealUserData ||
		golden.DataPolicy.ContainsSensitiveData {
		return errors.New("Golden data policy is not fixture-only")
	}
	if golden.FixtureManifestSHA256 != manifest.ContentSHA256 {
		return errors.New("Golden fixture manifest hash does not match")
	}
	if golden.Lifecycle.State == "frozen" {
		if err := memoryeval.ValidateGoldenAdmission(golden); err != nil {
			return fmt.Errorf("admit frozen Golden corpus: %w", err)
		}
	} else if golden.Lifecycle.State != "draft" {
		return errors.New("Golden lifecycle is invalid for fixture comparison")
	}

	fixtures := make(map[string]FixtureSet, len(manifest.Fixtures))
	allMemoryIDs := make(map[string]struct{})
	for _, fixture := range manifest.Fixtures {
		fixtures[fixture.Alias] = fixture
		for _, memory := range fixture.Memories {
			allMemoryIDs[memory.ID] = struct{}{}
		}
	}
	for _, item := range golden.Cases {
		fixture, ok := fixtures[item.FixtureAlias]
		if !ok || fixture.UserAlias != item.Scope.UserAlias {
			return fmt.Errorf("Golden case %q fixture authority does not match", item.ID)
		}
		owned := make(map[string]struct{}, len(fixture.Memories))
		for _, memory := range fixture.Memories {
			owned[memory.ID] = struct{}{}
		}
		for _, memoryID := range append(
			append([]string(nil), item.ExpectedRelevantMemoryIDs...),
			item.ExpectedCurrentMemoryIDs...,
		) {
			if _, ok := owned[memoryID]; !ok {
				return fmt.Errorf("Golden case %q expected Memory is outside its fixture", item.ID)
			}
		}
		for _, exclusion := range item.Exclusions {
			if _, ok := allMemoryIDs[exclusion.MemoryID]; !ok {
				return fmt.Errorf("Golden case %q exclusion is absent from the manifest", item.ID)
			}
		}
	}
	return nil
}

func validateManifest(manifest Manifest) error {
	if manifest.SchemaVersion != ManifestSchemaVersion ||
		!validIdentifier(manifest.ID) || manifest.PromotionEligible == nil ||
		*manifest.PromotionEligible || !validSHA256(manifest.ContentSHA256) ||
		!manifest.DataPolicy.SyntheticOnly || manifest.DataPolicy.ContainsRealUserData ||
		manifest.DataPolicy.ContainsSensitiveData || len(manifest.Fixtures) == 0 ||
		len(manifest.Fixtures) > maximumFixtures {
		return errors.New("fixture manifest header is invalid")
	}
	aliases := make(map[string]struct{}, len(manifest.Fixtures))
	memoryIDs := make(map[string]struct{})
	totalMemories := 0
	for _, fixture := range manifest.Fixtures {
		if !validIdentifier(fixture.Alias) || !validIdentifier(fixture.UserAlias) {
			return errors.New("fixture alias is invalid")
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
			if err := validateMemory(memory); err != nil {
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

func validateMemory(memory Memory) error {
	if !validIdentifier(memory.ID) || !validContent(memory.CanonicalContent) ||
		!validContent(memory.RawEventContent) {
		return fmt.Errorf("Memory %q content or identifier is invalid", memory.ID)
	}
	if memory.Scope.ProjectAlias != "" && !validIdentifier(memory.Scope.ProjectAlias) {
		return fmt.Errorf("Memory %q project scope is invalid", memory.ID)
	}
	if memory.Scope.ConversationAlias != "" && !validIdentifier(memory.Scope.ConversationAlias) {
		return fmt.Errorf("Memory %q conversation scope is invalid", memory.ID)
	}
	if _, err := time.Parse(time.RFC3339, memory.OccurredAt); err != nil {
		return fmt.Errorf("Memory %q occurredAt is invalid", memory.ID)
	}
	switch memory.State {
	case StateActive, StateDeleted, StateSecretRejected, StateUntrustedRejected:
	default:
		return fmt.Errorf("Memory %q state is invalid", memory.ID)
	}
	return nil
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

func validContent(value string) bool {
	if strings.TrimSpace(value) != value || value == "" || !utf8.ValidString(value) ||
		utf8.RuneCountInString(value) > maximumContentRunes {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) && character != '\n' {
			return false
		}
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
