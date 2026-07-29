package memoryauthor

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

func Diagnose(fixtures FixtureManifest, golden memoryeval.GoldenSet) (Diagnostic, error) {
	if len(fixtures.Fixtures) != len(golden.Cases) {
		return Diagnostic{}, errors.New("candidate fixture and Golden counts differ")
	}
	result, err := diagnoseCaseCounts(golden.Cases)
	if err != nil {
		return Diagnostic{}, err
	}
	witness, err := selectFeasibilityWitness(golden.Cases)
	if err != nil {
		return Diagnostic{}, err
	}
	result.WitnessCaseIDs = make([]string, 0, len(witness))
	for _, item := range witness {
		result.WitnessCaseIDs = append(result.WitnessCaseIDs, item.ID)
	}
	if err := validateWitness(witness); err != nil {
		return Diagnostic{}, err
	}
	return result, nil
}

func diagnoseCaseCounts(cases []memoryeval.GoldenCase) (Diagnostic, error) {
	result := Diagnostic{CaseCount: len(cases)}
	seenQueries := make(map[string]string, len(cases))
	normalizedQueries := make(map[string]string, len(cases))
	for _, item := range cases {
		if previous, duplicate := seenQueries[item.Query]; duplicate {
			return Diagnostic{}, fmt.Errorf("cases %q and %q have duplicate queries", previous, item.ID)
		}
		seenQueries[item.Query] = item.ID
		normalized := normalizeQuery(item.Query)
		if previous, duplicate := normalizedQueries[normalized]; duplicate {
			return Diagnostic{}, fmt.Errorf("cases %q and %q have normalized duplicate queries", previous, item.ID)
		}
		normalizedQueries[normalized] = item.ID
		switch item.Split {
		case "development":
			result.SplitCounts.Development++
		case "validation":
			result.SplitCounts.Validation++
		case "holdout":
			result.SplitCounts.Holdout++
		}
		switch item.Language {
		case "zh":
			result.LanguageCounts.Chinese++
		case "mixed":
			result.LanguageCounts.Mixed++
		case "en":
			result.LanguageCounts.English++
		}
	}
	for _, slice := range memoryeval.CriticalSlices() {
		count := SliceCount{Name: slice}
		for _, item := range cases {
			if !contains(item.Slices, slice) {
				continue
			}
			count.Total++
			switch item.Split {
			case "development":
				count.Development++
			case "validation":
				count.Validation++
			case "holdout":
				count.Holdout++
			}
		}
		result.SliceCounts = append(result.SliceCounts, count)
	}
	return result, nil
}

func selectFeasibilityWitness(cases []memoryeval.GoldenCase) ([]memoryeval.GoldenCase, error) {
	targets := map[string]map[string]int{
		"development": {"zh": 210, "mixed": 60, "en": 30},
		"validation":  {"zh": 70, "mixed": 20, "en": 10},
		"holdout":     {"zh": 70, "mixed": 20, "en": 10},
	}
	counts := map[string]map[string]int{
		"development": {"zh": 0, "mixed": 0, "en": 0},
		"validation":  {"zh": 0, "mixed": 0, "en": 0},
		"holdout":     {"zh": 0, "mixed": 0, "en": 0},
	}
	witness := make([]memoryeval.GoldenCase, 0, 500)
	for _, item := range cases {
		if counts[item.Split][item.Language] >= targets[item.Split][item.Language] {
			continue
		}
		counts[item.Split][item.Language]++
		witness = append(witness, item)
	}
	if len(witness) != 500 {
		return nil, errors.New("candidate pool has no exact split/language feasibility witness")
	}
	return witness, nil
}

func validateWitness(cases []memoryeval.GoldenCase) error {
	if len(cases) != 500 {
		return errors.New("feasibility witness must contain exactly 500 cases")
	}
	for _, slice := range memoryeval.CriticalSlices() {
		count := SliceCount{Name: slice}
		for _, item := range cases {
			if !contains(item.Slices, slice) {
				continue
			}
			count.Total++
			switch item.Split {
			case "development":
				count.Development++
			case "validation":
				count.Validation++
			case "holdout":
				count.Holdout++
			}
		}
		if count.Total < 50 || count.Development < 30 || count.Validation < 10 || count.Holdout < 10 {
			return fmt.Errorf("feasibility witness slice %q lacks 30/10/10 coverage", slice)
		}
	}
	return nil
}

func normalizeQuery(value string) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(value) {
		if unicode.IsLetter(character) || unicode.IsNumber(character) {
			builder.WriteRune(character)
		}
	}
	return builder.String()
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
