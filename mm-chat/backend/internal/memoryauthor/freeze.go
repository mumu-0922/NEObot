package memoryauthor

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

type FreezeInput struct {
	HoldoutRunID string
	Clock        func() time.Time
}

type HoldoutInput struct {
	HoldoutRunID string
	OutputPath   string
	Clock        func() time.Time
}

func Freeze(root string, input FreezeInput) (FrozenArtifacts, error) {
	var result FrozenArtifacts
	err := withWriterLock(root, func() error {
		if frozenExists(root) {
			return errors.New("frozen corpus directory already exists")
		}
		holdoutRunID := strings.TrimSpace(input.HoldoutRunID)
		if !validUUID(holdoutRunID) {
			return errors.New("precommitted Holdout run ID is invalid")
		}
		state, err := LoadReviewState(root)
		if err != nil {
			return err
		}
		accepted, rejected, pending := decisionCounts(state.Cases)
		if accepted != 500 || rejected != 150 || pending != 0 {
			return fmt.Errorf(
				"freeze requires exactly 500 accepted, 150 rejected, and 0 pending cases; got %d/%d/%d",
				accepted, rejected, pending,
			)
		}
		clock := input.Clock
		if clock == nil {
			clock = time.Now
		}
		frozenAtTime := clock().UTC().Truncate(time.Second)
		frozenAt := frozenAtTime.Format(time.RFC3339)
		promotion := false
		fixtures := FixtureManifest{
			SchemaVersion:     FixtureSchemaVersion,
			ID:                "memory-benchmark-v1-frozen-fixtures",
			Description:       "Human-reviewed synthetic-only Memory benchmark fixtures.",
			PromotionEligible: &promotion,
			DataPolicy:        DataPolicy{SyntheticOnly: true},
			Generator:         expectedGenerator(),
			ContentSHA256:     strings.Repeat("0", 64),
			Fixtures:          make([]Fixture, 0, 500),
		}
		golden := state.Golden
		golden.ID = "memory-benchmark-v1-golden"
		golden.Description = "Human-reviewed synthetic Memory benchmark Golden corpus."
		golden.PromotionEligible = &promotion
		golden.Cases = make([]memoryeval.GoldenCase, 0, 500)
		for _, candidate := range state.Cases {
			if candidate.Decision != DecisionAccepted {
				continue
			}
			reviewedAt, err := time.Parse(time.RFC3339, candidate.ReviewedAt)
			if err != nil || reviewedAt.After(frozenAtTime) {
				return fmt.Errorf("accepted case %q review time is outside the freeze window", candidate.Snapshot.Case.ID)
			}
			item := candidate.Snapshot.Case
			item.Review = memoryeval.Review{
				State: "human_reviewed", ReviewerID: candidate.ReviewerID,
				ReviewedAt: candidate.ReviewedAt,
			}
			golden.Cases = append(golden.Cases, item)
			fixtures.Fixtures = append(fixtures.Fixtures, candidate.Snapshot.Fixture)
		}
		fixtureContentHash, err := FixtureContentSHA256(fixtures)
		if err != nil {
			return err
		}
		fixtures.ContentSHA256 = fixtureContentHash
		golden.FixtureManifestSHA256 = fixtureContentHash
		golden.Lifecycle = memoryeval.GoldenLifecycle{
			State: "frozen", FrozenAt: frozenAt, HoldoutRunID: holdoutRunID,
		}
		goldenContentHash, err := memoryeval.GoldenContentSHA256(golden)
		if err != nil {
			return err
		}
		golden.Lifecycle.FrozenContentSHA256 = goldenContentHash
		if language := countLanguages(golden.Cases); language != (CountByLanguage{350, 100, 50}) {
			return fmt.Errorf("accepted corpus language counts are invalid: %+v", language)
		}
		if err := validateFixtureManifest(fixtures); err != nil {
			return err
		}
		if err := validateGoldenFixtureBinding(fixtures, golden); err != nil {
			return err
		}
		if err := memoryeval.ValidateGoldenAdmission(golden); err != nil {
			return fmt.Errorf("admit frozen Memory Golden: %w", err)
		}
		fixtureBody, err := marshalCanonical(fixtures)
		if err != nil {
			return err
		}
		goldenBody, err := marshalCanonical(golden)
		if err != nil {
			return err
		}
		manifest := FreezeManifest{
			SchemaVersion:         FreezeManifestVersion,
			CandidateManifestID:   state.CandidateManifest.ID,
			FrozenAt:              frozenAt,
			HoldoutRunID:          holdoutRunID,
			FixtureContentSHA256:  fixtureContentHash,
			FixtureRawSHA256:      sha256Hex(fixtureBody),
			GoldenRawSHA256:       sha256Hex(goldenBody),
			GoldenContentSHA256:   goldenContentHash,
			ReviewLastSequence:    state.LastSequence,
			ReviewLastEventSHA256: state.LastEventSHA256,
		}
		fixtureByAlias := make(map[string]Fixture, len(fixtures.Fixtures))
		for _, fixture := range fixtures.Fixtures {
			fixtureByAlias[fixture.Alias] = fixture
		}
		for _, item := range golden.Cases {
			if item.Split != "holdout" {
				continue
			}
			manifest.OrderedHoldoutCaseIDs = append(manifest.OrderedHoldoutCaseIDs, item.ID)
			digest, err := CaseContentSHA256(CaseSnapshot{Case: item, Fixture: fixtureByAlias[item.FixtureAlias]})
			if err != nil {
				return err
			}
			manifest.OrderedHoldoutCaseHashes = append(manifest.OrderedHoldoutCaseHashes, digest)
		}
		if err := validateFreezeManifest(manifest); err != nil {
			return err
		}
		manifestBody, err := marshalCanonical(manifest)
		if err != nil {
			return err
		}
		frozenDirectory := filepath.Join(root, FrozenDirectory)
		if err := os.Mkdir(frozenDirectory, 0o700); err != nil {
			return errors.New("create frozen corpus directory")
		}
		complete := false
		defer func() {
			if !complete {
				_ = os.WriteFile(filepath.Join(frozenDirectory, ".freeze-incomplete"), []byte("incomplete\n"), 0o600)
			}
		}()
		if err := os.Mkdir(filepath.Join(root, HoldoutDirectory), 0o700); err != nil {
			return errors.New("create sealed Holdout directory")
		}
		if err := writeExclusiveBytes(filepath.Join(frozenDirectory, FrozenFixtureFile), fixtureBody); err != nil {
			return err
		}
		if err := writeExclusiveBytes(filepath.Join(frozenDirectory, FrozenGoldenFile), goldenBody); err != nil {
			return err
		}
		if err := writeExclusiveBytes(filepath.Join(frozenDirectory, FreezeManifestFile), manifestBody); err != nil {
			return err
		}
		complete = true
		result = FrozenArtifacts{Fixtures: fixtures, Golden: golden, Manifest: manifest}
		return nil
	})
	return result, err
}

func LoadFrozen(root string) (FrozenArtifacts, error) {
	directory := filepath.Join(root, FrozenDirectory)
	if err := validateSecureDirectory(directory); err != nil {
		return FrozenArtifacts{}, fmt.Errorf("validate frozen corpus directory: %w", err)
	}
	if _, err := os.Lstat(filepath.Join(directory, ".freeze-incomplete")); err == nil {
		return FrozenArtifacts{}, errors.New("frozen corpus publication is incomplete")
	} else if !errors.Is(err, os.ErrNotExist) {
		return FrozenArtifacts{}, errors.New("inspect frozen corpus marker")
	}
	fixtureBody, err := readSecureArtifact(filepath.Join(directory, FrozenFixtureFile))
	if err != nil {
		return FrozenArtifacts{}, fmt.Errorf("read frozen fixtures: %w", err)
	}
	goldenBody, err := readSecureArtifact(filepath.Join(directory, FrozenGoldenFile))
	if err != nil {
		return FrozenArtifacts{}, fmt.Errorf("read frozen Golden: %w", err)
	}
	manifestBody, err := readSecureArtifact(filepath.Join(directory, FreezeManifestFile))
	if err != nil {
		return FrozenArtifacts{}, fmt.Errorf("read freeze manifest: %w", err)
	}
	fixtures, err := DecodeFixtureManifest(bytes.NewReader(fixtureBody))
	if err != nil {
		return FrozenArtifacts{}, err
	}
	golden, err := memoryeval.DecodeGoldenSet(bytes.NewReader(goldenBody))
	if err != nil {
		return FrozenArtifacts{}, fmt.Errorf("decode frozen Golden: %w", err)
	}
	manifest, err := decodeFreezeManifest(manifestBody)
	if err != nil {
		return FrozenArtifacts{}, err
	}
	review, err := LoadReviewState(root)
	if err != nil {
		return FrozenArtifacts{}, fmt.Errorf("replay frozen review authority: %w", err)
	}
	if manifest.CandidateManifestID != review.CandidateManifest.ID ||
		manifest.ReviewLastSequence != review.LastSequence ||
		manifest.ReviewLastEventSHA256 != review.LastEventSHA256 ||
		manifest.FixtureContentSHA256 != fixtures.ContentSHA256 ||
		manifest.FixtureRawSHA256 != sha256Hex(fixtureBody) ||
		manifest.GoldenRawSHA256 != sha256Hex(goldenBody) ||
		manifest.GoldenContentSHA256 != golden.Lifecycle.FrozenContentSHA256 ||
		manifest.HoldoutRunID != golden.Lifecycle.HoldoutRunID || manifest.FrozenAt != golden.Lifecycle.FrozenAt {
		return FrozenArtifacts{}, errors.New("frozen artifact binding does not match")
	}
	if err := validateGoldenFixtureBinding(fixtures, golden); err != nil {
		return FrozenArtifacts{}, err
	}
	if err := memoryeval.ValidateGoldenAdmission(golden); err != nil {
		return FrozenArtifacts{}, err
	}
	fixtureByAlias := make(map[string]Fixture, len(fixtures.Fixtures))
	for _, fixture := range fixtures.Fixtures {
		fixtureByAlias[fixture.Alias] = fixture
	}
	holdoutIndex := 0
	for _, item := range golden.Cases {
		if item.Split != "holdout" {
			continue
		}
		if holdoutIndex >= len(manifest.OrderedHoldoutCaseIDs) ||
			manifest.OrderedHoldoutCaseIDs[holdoutIndex] != item.ID {
			return FrozenArtifacts{}, errors.New("frozen Holdout case order does not match")
		}
		digest, err := CaseContentSHA256(CaseSnapshot{Case: item, Fixture: fixtureByAlias[item.FixtureAlias]})
		if err != nil || manifest.OrderedHoldoutCaseHashes[holdoutIndex] != digest {
			return FrozenArtifacts{}, errors.New("frozen Holdout case hash does not match")
		}
		holdoutIndex++
	}
	if holdoutIndex != 100 {
		return FrozenArtifacts{}, errors.New("frozen Holdout must contain exactly 100 cases")
	}
	return FrozenArtifacts{Fixtures: fixtures, Golden: golden, Manifest: manifest}, nil
}

func BeginHoldout(root string, input HoldoutInput) (HoldoutUse, error) {
	var result HoldoutUse
	err := withWriterLock(root, func() error {
		frozen, err := LoadFrozen(root)
		if err != nil {
			return err
		}
		if strings.TrimSpace(input.HoldoutRunID) != frozen.Manifest.HoldoutRunID {
			return errors.New("Holdout run ID does not match the frozen precommitment")
		}
		holdoutDirectory, err := filepath.Abs(filepath.Join(root, HoldoutDirectory))
		if err != nil {
			return errors.New("resolve protected Holdout directory")
		}
		if err := validateSecureDirectory(holdoutDirectory); err != nil {
			return err
		}
		output, err := filepath.Abs(strings.TrimSpace(input.OutputPath))
		if err != nil || !pathWithin(output, holdoutDirectory) || output == holdoutDirectory ||
			filepath.Base(output) == HoldoutUseFile {
			return errors.New("Holdout output must be a new file inside the protected Holdout directory")
		}
		if err := rejectSymlinkComponents(output); err != nil {
			return err
		}
		if err := validateSecureDirectory(filepath.Dir(output)); err != nil {
			return errors.New("Holdout output directory must already exist and be private")
		}
		if _, err := os.Lstat(output); err == nil {
			return errors.New("Holdout output already exists")
		} else if !errors.Is(err, os.ErrNotExist) {
			return errors.New("inspect Holdout output")
		}
		fixtureByAlias := make(map[string]Fixture, len(frozen.Fixtures.Fixtures))
		for _, fixture := range frozen.Fixtures.Fixtures {
			fixtureByAlias[fixture.Alias] = fixture
		}
		bundle := HoldoutBundle{
			SchemaVersion: HoldoutBundleVersion,
			HoldoutRunID:  frozen.Manifest.HoldoutRunID, Ordinal: 1,
			GoldenSetID:         frozen.Golden.ID,
			GoldenContentSHA256: frozen.Manifest.GoldenContentSHA256,
			FixtureRawSHA256:    frozen.Manifest.FixtureRawSHA256,
		}
		for _, item := range frozen.Golden.Cases {
			if item.Split != "holdout" {
				continue
			}
			bundle.Cases = append(bundle.Cases, item)
			bundle.Fixtures = append(bundle.Fixtures, fixtureByAlias[item.FixtureAlias])
		}
		if err := validateHoldoutBundle(bundle); err != nil {
			return err
		}
		bundleBody, err := marshalCanonical(bundle)
		if err != nil {
			return err
		}
		clock := input.Clock
		if clock == nil {
			clock = time.Now
		}
		consumedAt := clock().UTC().Truncate(time.Second)
		frozenAt, _ := time.Parse(time.RFC3339, frozen.Manifest.FrozenAt)
		if consumedAt.Before(frozenAt) {
			return errors.New("Holdout consumption precedes corpus freeze")
		}
		absoluteRoot, err := filepath.Abs(root)
		if err != nil {
			return errors.New("resolve authoring root")
		}
		relativeOutput, _ := filepath.Rel(absoluteRoot, output)
		use := HoldoutUse{
			SchemaVersion: HoldoutUseVersion, State: "consumed",
			HoldoutRunID: frozen.Manifest.HoldoutRunID, Ordinal: 1,
			ConsumedAt:          consumedAt.Format(time.RFC3339),
			GoldenContentSHA256: frozen.Manifest.GoldenContentSHA256,
			FixtureRawSHA256:    frozen.Manifest.FixtureRawSHA256,
			OutputPath:          filepath.ToSlash(relativeOutput),
		}
		if err := validateHoldoutUse(use); err != nil {
			return err
		}
		useBody, err := marshalCanonical(use)
		if err != nil {
			return err
		}
		// This marker is intentionally committed before any Holdout content is
		// published outside the frozen artifact. A later failure permanently
		// consumes the run.
		if err := writeExclusiveBytes(filepath.Join(holdoutDirectory, HoldoutUseFile), useBody); err != nil {
			if strings.Contains(err.Error(), "already exists") {
				return errors.New("Holdout was already consumed; retry is permanently refused")
			}
			return fmt.Errorf("commit Holdout consumed marker: %w", err)
		}
		if err := writeExclusiveBytes(output, bundleBody); err != nil {
			return fmt.Errorf("Holdout consumed but input publication failed: %w", err)
		}
		result = use
		return nil
	})
	return result, err
}

func validateFreezeManifest(manifest FreezeManifest) error {
	if manifest.SchemaVersion != FreezeManifestVersion ||
		!validIdentifier(manifest.CandidateManifestID) || !validTimestamp(manifest.FrozenAt) ||
		!validUUID(manifest.HoldoutRunID) || !validSHA256(manifest.FixtureContentSHA256) ||
		!validSHA256(manifest.FixtureRawSHA256) || !validSHA256(manifest.GoldenRawSHA256) ||
		!validSHA256(manifest.GoldenContentSHA256) || manifest.ReviewLastSequence < 650 ||
		!validSHA256(manifest.ReviewLastEventSHA256) || len(manifest.OrderedHoldoutCaseIDs) != 100 ||
		len(manifest.OrderedHoldoutCaseHashes) != 100 {
		return errors.New("freeze manifest is invalid")
	}
	seen := make(map[string]struct{}, 100)
	for index, caseID := range manifest.OrderedHoldoutCaseIDs {
		if !validIdentifier(caseID) || !validSHA256(manifest.OrderedHoldoutCaseHashes[index]) {
			return errors.New("freeze manifest Holdout binding is invalid")
		}
		if _, duplicate := seen[caseID]; duplicate {
			return errors.New("freeze manifest repeats a Holdout case")
		}
		seen[caseID] = struct{}{}
	}
	return nil
}

func validateHoldoutUse(use HoldoutUse) error {
	if use.SchemaVersion != HoldoutUseVersion || use.State != "consumed" ||
		!validUUID(use.HoldoutRunID) || use.Ordinal != 1 || !validTimestamp(use.ConsumedAt) ||
		!validSHA256(use.GoldenContentSHA256) || !validSHA256(use.FixtureRawSHA256) ||
		strings.TrimSpace(use.OutputPath) != use.OutputPath || use.OutputPath == "" ||
		filepath.IsAbs(use.OutputPath) || strings.HasPrefix(use.OutputPath, "../") {
		return errors.New("Holdout consumed marker is invalid")
	}
	return nil
}

func validateHoldoutBundle(bundle HoldoutBundle) error {
	if bundle.SchemaVersion != HoldoutBundleVersion || !validUUID(bundle.HoldoutRunID) ||
		bundle.Ordinal != 1 || !validIdentifier(bundle.GoldenSetID) ||
		!validSHA256(bundle.GoldenContentSHA256) || !validSHA256(bundle.FixtureRawSHA256) ||
		len(bundle.Cases) != 100 || len(bundle.Fixtures) != 100 {
		return errors.New("Holdout bundle is invalid")
	}
	for index, item := range bundle.Cases {
		if item.Split != "holdout" || item.FixtureAlias != bundle.Fixtures[index].Alias {
			return errors.New("Holdout bundle case order or fixture binding is invalid")
		}
	}
	return nil
}

func countLanguages(cases []memoryeval.GoldenCase) CountByLanguage {
	var result CountByLanguage
	for _, item := range cases {
		switch item.Language {
		case "zh":
			result.Chinese++
		case "mixed":
			result.Mixed++
		case "en":
			result.English++
		}
	}
	return result
}

func frozenExists(root string) bool {
	_, err := os.Lstat(filepath.Join(root, FrozenDirectory))
	return err == nil
}
