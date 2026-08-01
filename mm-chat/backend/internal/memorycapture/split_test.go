package memorycapture

import (
	"testing"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
)

func TestSelectRegressionCaptureSplitCannotConsumeMachineHoldout(t *testing.T) {
	profiles := []struct {
		name     string
		generate func() (memoryauthor.RegressionPool, error)
	}{
		{name: "v2", generate: memoryauthor.GenerateRegression},
		{name: "v3", generate: memoryauthor.GenerateRegressionV3},
		{name: "v4", generate: memoryauthor.GenerateRegressionV4},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			pool, err := profile.generate()
			if err != nil {
				t.Fatal(err)
			}
			for _, test := range []struct {
				split string
				count int
			}{
				{split: DevelopmentCalibrationSplit, count: 300},
				{split: FrozenValidationSplit, count: 100},
			} {
				selected, err := SelectRegressionCaptureSplit(pool, test.split)
				if err != nil {
					t.Fatalf("select %s: %v", test.split, err)
				}
				if len(selected.Corpus.Cases) != test.count ||
					len(selected.Fixtures.Fixtures) != test.count {
					t.Fatalf("%s cardinality = cases:%d fixtures:%d", test.split,
						len(selected.Corpus.Cases), len(selected.Fixtures.Fixtures))
				}
				aliases := make(map[string]struct{}, test.count)
				for _, item := range selected.Corpus.Cases {
					if item.Split != test.split {
						t.Fatalf("%s view contains %s case", test.split, item.Split)
					}
					aliases[item.FixtureAlias] = struct{}{}
				}
				for _, fixture := range selected.Fixtures.Fixtures {
					if _, ok := aliases[fixture.Alias]; !ok {
						t.Fatalf("%s view contains unrelated fixture %q", test.split, fixture.Alias)
					}
				}
			}
			if _, err := SelectRegressionCaptureSplit(pool, "holdout"); err == nil {
				t.Fatal("machine-visible holdout was accepted by capture split selector")
			}
		})
	}
}
