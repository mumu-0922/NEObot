package memoryauthor

import (
	"bytes"
	"errors"
	"fmt"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

func validateReviewStateContent(state ReviewState) (Diagnostic, error) {
	fixtures, golden, err := materializeReviewDraft(state)
	if err != nil {
		return Diagnostic{}, err
	}
	if err := validateFixtureManifest(fixtures); err != nil {
		return Diagnostic{}, err
	}
	body, err := marshalCanonical(golden)
	if err != nil {
		return Diagnostic{}, err
	}
	decoded, err := memoryeval.DecodeGoldenSet(bytes.NewReader(body))
	if err != nil {
		return Diagnostic{}, fmt.Errorf("validate current review Golden: %w", err)
	}
	if err := validateGoldenFixtureBinding(fixtures, decoded); err != nil {
		return Diagnostic{}, err
	}
	return diagnoseCaseCounts(decoded.Cases)
}

func materializeReviewDraft(state ReviewState) (FixtureManifest, memoryeval.GoldenSet, error) {
	if len(state.Cases) != state.CandidateManifest.CaseCount ||
		len(state.Cases) != len(state.Golden.Cases) {
		return FixtureManifest{}, memoryeval.GoldenSet{}, errors.New("review state case count is inconsistent")
	}
	fixtures := state.FixtureManifest
	fixtures.Fixtures = make([]Fixture, 0, len(state.Cases))
	golden := state.Golden
	golden.Cases = make([]memoryeval.GoldenCase, 0, len(state.Cases))
	for index, item := range state.Cases {
		original := state.Golden.Cases[index]
		if item.Snapshot.Case.ID != original.ID ||
			item.Snapshot.Case.FixtureAlias != original.FixtureAlias ||
			item.Snapshot.Fixture.Alias != original.FixtureAlias {
			return FixtureManifest{}, memoryeval.GoldenSet{}, errors.New("review state changes an immutable case identifier")
		}
		digest, err := CaseContentSHA256(item.Snapshot)
		if err != nil || digest != item.ContentSHA256 {
			return FixtureManifest{}, memoryeval.GoldenSet{}, fmt.Errorf(
				"review case %q content hash does not match", item.Snapshot.Case.ID,
			)
		}
		fixtures.Fixtures = append(fixtures.Fixtures, item.Snapshot.Fixture)
		golden.Cases = append(golden.Cases, item.Snapshot.Case)
	}
	fixtureContentHash, err := FixtureContentSHA256(fixtures)
	if err != nil {
		return FixtureManifest{}, memoryeval.GoldenSet{}, err
	}
	fixtures.ContentSHA256 = fixtureContentHash
	golden.FixtureManifestSHA256 = fixtureContentHash
	return fixtures, golden, nil
}
