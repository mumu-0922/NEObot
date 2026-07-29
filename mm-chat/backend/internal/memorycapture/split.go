package memorycapture

import (
	"fmt"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
)

const (
	DevelopmentCalibrationSplit = "development"
	FrozenValidationSplit       = "validation"
)

// SelectRegressionCaptureSplit derives an in-memory seed view from the
// already byte-verified 500-case pool. It deliberately refuses the visible
// machine holdout so neither calibration nor validation tooling can consume it
// by changing a string flag.
func SelectRegressionCaptureSplit(
	pool memoryauthor.RegressionPool,
	split string,
) (memoryauthor.RegressionPool, error) {
	expected := 0
	switch split {
	case DevelopmentCalibrationSplit:
		expected = 300
	case FrozenValidationSplit:
		expected = 100
	default:
		return memoryauthor.RegressionPool{}, fmt.Errorf("%w: capture split", ErrCaptureInvalid)
	}
	if len(pool.Corpus.Cases) != 500 || len(pool.Fixtures.Fixtures) != 500 {
		return memoryauthor.RegressionPool{}, fmt.Errorf("%w: capture corpus cardinality", ErrCaptureInvalid)
	}
	selected := pool
	selected.Corpus.Cases = make([]memoryeval.GoldenCase, 0, expected)
	fixtureAliases := make(map[string]struct{}, expected)
	for _, item := range pool.Corpus.Cases {
		if item.Split != split {
			continue
		}
		if _, duplicate := fixtureAliases[item.FixtureAlias]; duplicate {
			return memoryauthor.RegressionPool{}, fmt.Errorf("%w: capture fixture binding", ErrCaptureInvalid)
		}
		fixtureAliases[item.FixtureAlias] = struct{}{}
		selected.Corpus.Cases = append(selected.Corpus.Cases, item)
	}
	selected.Fixtures.Fixtures = make([]memoryauthor.Fixture, 0, expected)
	for _, fixture := range pool.Fixtures.Fixtures {
		if _, ok := fixtureAliases[fixture.Alias]; !ok {
			continue
		}
		selected.Fixtures.Fixtures = append(selected.Fixtures.Fixtures, fixture)
		delete(fixtureAliases, fixture.Alias)
	}
	if len(selected.Corpus.Cases) != expected ||
		len(selected.Fixtures.Fixtures) != expected || len(fixtureAliases) != 0 {
		return memoryauthor.RegressionPool{}, fmt.Errorf("%w: capture split cardinality", ErrCaptureInvalid)
	}
	return selected, nil
}
