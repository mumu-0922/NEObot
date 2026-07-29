package memoryauthor

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"neo-chat/mm-chat/backend/internal/strictjson"
)

func CurrentStatus(root string) (Status, error) {
	state, err := LoadReviewState(root)
	if err != nil {
		return Status{}, err
	}
	diagnostic, err := validateReviewStateContent(state)
	if err != nil {
		return Status{}, err
	}
	accepted, rejected, pending := decisionCounts(state.Cases)
	manifestBody, err := readSecureArtifact(filepath.Join(root, CandidateManifestFile))
	if err != nil {
		return Status{}, fmt.Errorf("read candidate manifest for status: %w", err)
	}
	status := Status{
		SchemaVersion:         StatusSchemaVersion,
		Profile:               state.CandidateManifest.Generator.Profile,
		GeneratorVersion:      state.CandidateManifest.Generator.Version,
		CandidateManifestID:   state.CandidateManifest.ID,
		CandidateManifestHash: sha256Hex(manifestBody),
		FixtureContentSHA256:  state.CandidateManifest.FixtureContentSHA256,
		CandidateCount:        diagnostic.CaseCount,
		Accepted:              accepted, Rejected: rejected, Pending: pending,
		HoldoutState:   "not_frozen",
		SplitCounts:    diagnostic.SplitCounts,
		LanguageCounts: diagnostic.LanguageCounts,
		SliceCounts:    append([]SliceCount(nil), diagnostic.SliceCounts...),
	}
	if !frozenExists(root) {
		return status, nil
	}
	frozen, err := LoadFrozen(root)
	if err != nil {
		return Status{}, err
	}
	status.Frozen = true
	status.GoldenContentSHA256 = frozen.Manifest.GoldenContentSHA256
	status.HoldoutState = "sealed"
	markerPath := filepath.Join(root, HoldoutDirectory, HoldoutUseFile)
	body, err := readSecureArtifact(markerPath)
	if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return Status{}, fmt.Errorf("read Holdout consumed marker: %w", err)
	}
	var use HoldoutUse
	if err := strictjson.Decode(body, maximumArtifactBytes, &use); err != nil {
		return Status{}, fmt.Errorf("decode Holdout consumed marker: %w", err)
	}
	if err := validateHoldoutUse(use); err != nil {
		return Status{}, err
	}
	if use.HoldoutRunID != frozen.Manifest.HoldoutRunID ||
		use.GoldenContentSHA256 != frozen.Manifest.GoldenContentSHA256 ||
		use.FixtureRawSHA256 != frozen.Manifest.FixtureRawSHA256 {
		return Status{}, errors.New("Holdout consumed marker binding does not match")
	}
	status.HoldoutState = "consumed"
	return status, nil
}

func Verify(root string) (Status, error) {
	actual, err := LoadPool(root)
	if err != nil {
		return Status{}, err
	}
	expected, err := Generate()
	if err != nil {
		return Status{}, err
	}
	if !bytes.Equal(actual.FixtureJSON, expected.FixtureJSON) ||
		!bytes.Equal(actual.GoldenJSON, expected.GoldenJSON) ||
		!bytes.Equal(actual.ManifestJSON, expected.ManifestJSON) {
		return Status{}, errors.New("candidate pool does not byte-match the fixed generator profile")
	}
	status, err := CurrentStatus(root)
	if err != nil {
		return Status{}, err
	}
	if status.CandidateCount != 650 || status.Accepted+status.Rejected+status.Pending != 650 {
		return Status{}, errors.New("authoring status counts are inconsistent")
	}
	return status, nil
}
