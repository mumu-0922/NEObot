package rageval

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
)

func Evaluate(golden GoldenSet, observations ObservationSet) (Report, error) {
	if err := validateGoldenSet(golden); err != nil {
		return Report{}, err
	}
	if err := validateObservationSet(observations); err != nil {
		return Report{}, err
	}
	if observations.GoldenSetID != golden.ID {
		return Report{}, errors.New("observation golden set id does not match")
	}

	observedByCase := make(map[string]CaseObservation, len(observations.Cases))
	for _, observed := range observations.Cases {
		if _, duplicate := observedByCase[observed.CaseID]; duplicate {
			return Report{}, fmt.Errorf("duplicate observation case %q", observed.CaseID)
		}
		observedByCase[observed.CaseID] = observed
	}
	if len(observedByCase) != len(golden.Cases) {
		return Report{}, errors.New("observation cases do not exactly match golden cases")
	}

	type recallCounts struct{ hits, expected int }
	laneCounts := make(map[string]recallCounts)
	latencies := make([]int64, 0, len(golden.Cases))
	finalRelevant, finalTotal := 0, 0
	noEvidenceCorrect := 0
	negativeCases := 0
	negativeFalseCitationCases := 0
	relevantCases := 0
	caseFailures := make([]string, 0)

	for _, goldenCase := range golden.Cases {
		observed, ok := observedByCase[goldenCase.ID]
		if !ok {
			return Report{}, fmt.Errorf("missing observation case %q", goldenCase.ID)
		}
		delete(observedByCase, goldenCase.ID)
		latencies = append(latencies, observed.LatencyMilliseconds)
		relevant := stringSet(goldenCase.ExpectedRelevantEvidenceIDs)

		if observed.NoEvidence == goldenCase.ExpectedNoEvidence {
			noEvidenceCorrect++
		}
		if goldenCase.ExpectedNoEvidence {
			negativeCases++
			if len(observed.CitationEvidenceIDs) > 0 {
				negativeFalseCitationCases++
			}
			if !observed.NoEvidence || len(observed.FinalContextEvidenceIDs) > 0 || len(observed.CitationEvidenceIDs) > 0 {
				caseFailures = append(caseFailures, goldenCase.ID+": unrelated case was not rejected")
			}
		} else {
			relevantCases++
			if observed.NoEvidence || !containsAll(observed.FinalContextEvidenceIDs, relevant) || !containsAll(observed.CitationEvidenceIDs, relevant) {
				caseFailures = append(caseFailures, goldenCase.ID+": approved evidence was not retained")
			}
			if !containsOnly(observed.CitationEvidenceIDs, relevant) {
				caseFailures = append(caseFailures, goldenCase.ID+": citation included unrelated evidence")
			}
		}

		for _, evidenceID := range observed.FinalContextEvidenceIDs {
			finalTotal++
			if _, ok := relevant[evidenceID]; ok {
				finalRelevant++
			}
		}
		for _, lane := range goldenCase.RequiredLanes {
			counts := laneCounts[lane]
			counts.expected += len(relevant)
			for evidenceID := range relevant {
				if laneContains(observed.LaneResults[lane], evidenceID) {
					counts.hits++
				}
			}
			laneCounts[lane] = counts
		}
	}
	if len(observedByCase) != 0 {
		return Report{}, errors.New("observation contains unknown cases")
	}

	report := Report{
		SchemaVersion:                  ReportSchemaVersion,
		GoldenSetID:                    golden.ID,
		ProfileID:                      observations.Profile.ID,
		CaseCount:                      len(golden.Cases),
		RelevantCaseCount:              relevantCases,
		NegativeCaseCount:              negativeCases,
		FinalContextPrecision:          ratio(finalRelevant, finalTotal),
		NegativeFalseCitationRate:      ratio(negativeFalseCitationCases, negativeCases),
		NegativeFalseCitationCaseCount: negativeFalseCitationCases,
		NoEvidenceAccuracy:             ratio(noEvidenceCorrect, len(golden.Cases)),
		P95LatencyMilliseconds:         percentile95(latencies),
		Failures:                       caseFailures,
	}
	lanes := make([]string, 0, len(laneCounts))
	for lane := range laneCounts {
		lanes = append(lanes, lane)
	}
	sort.Strings(lanes)
	for _, lane := range lanes {
		counts := laneCounts[lane]
		report.LaneRecall = append(report.LaneRecall, LaneRecall{
			Lane: lane, Hits: counts.hits, Expected: counts.expected,
			Recall: ratio(counts.hits, counts.expected),
		})
	}
	applyCriteria(&report, golden.Criteria)
	sort.Strings(report.Failures)
	report.Passed = len(report.Failures) == 0
	return report, nil
}

func validateGoldenSet(value GoldenSet) error {
	if value.SchemaVersion != GoldenSchemaVersion || strings.TrimSpace(value.ID) == "" || len(value.Cases) == 0 {
		return errors.New("golden set header is invalid")
	}
	if err := validateCriteria(value.Criteria); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(value.Cases))
	for _, item := range value.Cases {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Category) == "" || strings.TrimSpace(item.Query) == "" || len(item.SelectedCollectionAliases) == 0 {
			return fmt.Errorf("golden case %q is invalid", item.ID)
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return fmt.Errorf("duplicate golden case %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		if item.ExpectedNoEvidence == (len(item.ExpectedRelevantEvidenceIDs) > 0) {
			return fmt.Errorf("golden case %q evidence expectation is inconsistent", item.ID)
		}
		if !item.ExpectedNoEvidence && len(item.RequiredLanes) == 0 {
			return fmt.Errorf("golden case %q requires at least one lane", item.ID)
		}
		if hasBlankOrDuplicate(item.SelectedCollectionAliases) || hasBlankOrDuplicate(item.ExpectedRelevantEvidenceIDs) || hasBlankOrDuplicate(item.RequiredLanes) {
			return fmt.Errorf("golden case %q contains blank or duplicate identifiers", item.ID)
		}
	}
	return nil
}

func validateCriteria(value Criteria) error {
	for lane, threshold := range value.MinimumLaneRecall {
		if strings.TrimSpace(lane) == "" || !validRate(threshold) {
			return errors.New("golden criteria lane recall is invalid")
		}
	}
	if !validRate(value.MinimumFinalContextPrecision) ||
		!validRate(value.MaximumNegativeFalseCitationRate) ||
		!validRate(value.MinimumNoEvidenceAccuracy) ||
		value.MaximumP95LatencyMilliseconds <= 0 {
		return errors.New("golden criteria is invalid")
	}
	return nil
}

func validateObservationSet(value ObservationSet) error {
	if value.SchemaVersion != ObservationSchemaVersion || strings.TrimSpace(value.GoldenSetID) == "" || strings.TrimSpace(value.CapturedOn) == "" || strings.TrimSpace(value.CaptureKind) == "" || strings.TrimSpace(value.Profile.ID) == "" || len(value.Cases) == 0 {
		return errors.New("observation set header is invalid")
	}
	if strings.TrimSpace(value.Profile.LexicalEngine) == "" || strings.TrimSpace(value.Profile.DenseEngine) == "" || strings.TrimSpace(value.Profile.EmbeddingModel) == "" || strings.TrimSpace(value.Profile.RerankerModel) == "" || strings.TrimSpace(value.Profile.RelevancePolicy) == "" || value.Profile.CandidateLimit <= 0 || value.Profile.EvidenceLimit <= 0 || value.Profile.RRFConstant <= 0 || math.IsNaN(value.Profile.RRFConstant) || math.IsInf(value.Profile.RRFConstant, 0) {
		return errors.New("observation retrieval profile is invalid")
	}
	seenCases := make(map[string]struct{}, len(value.Cases))
	for _, item := range value.Cases {
		if strings.TrimSpace(item.CaseID) == "" || item.LatencyMilliseconds < 0 {
			return errors.New("observation case is invalid")
		}
		if _, duplicate := seenCases[item.CaseID]; duplicate {
			return fmt.Errorf("duplicate observation case %q", item.CaseID)
		}
		seenCases[item.CaseID] = struct{}{}
		if hasBlankOrDuplicate(item.FinalContextEvidenceIDs) || hasBlankOrDuplicate(item.CitationEvidenceIDs) {
			return fmt.Errorf("observation case %q contains blank or duplicate evidence", item.CaseID)
		}
		for lane, ranked := range item.LaneResults {
			if strings.TrimSpace(lane) == "" {
				return fmt.Errorf("observation case %q contains a blank lane", item.CaseID)
			}
			seenEvidence := make(map[string]struct{}, len(ranked))
			seenRanks := make(map[int]struct{}, len(ranked))
			for _, result := range ranked {
				if strings.TrimSpace(result.EvidenceID) == "" || result.Rank <= 0 || math.IsNaN(result.Score) || math.IsInf(result.Score, 0) {
					return fmt.Errorf("observation case %q lane %q is invalid", item.CaseID, lane)
				}
				if _, duplicate := seenEvidence[result.EvidenceID]; duplicate {
					return fmt.Errorf("observation case %q lane %q repeats evidence", item.CaseID, lane)
				}
				if _, duplicate := seenRanks[result.Rank]; duplicate {
					return fmt.Errorf("observation case %q lane %q repeats rank", item.CaseID, lane)
				}
				seenEvidence[result.EvidenceID] = struct{}{}
				seenRanks[result.Rank] = struct{}{}
			}
		}
	}
	return nil
}

func applyCriteria(report *Report, criteria Criteria) {
	recallByLane := make(map[string]float64, len(report.LaneRecall))
	for _, lane := range report.LaneRecall {
		recallByLane[lane.Lane] = lane.Recall
	}
	criteriaLanes := make([]string, 0, len(criteria.MinimumLaneRecall))
	for lane := range criteria.MinimumLaneRecall {
		criteriaLanes = append(criteriaLanes, lane)
	}
	sort.Strings(criteriaLanes)
	for _, lane := range criteriaLanes {
		if recallByLane[lane] < criteria.MinimumLaneRecall[lane] {
			report.Failures = append(report.Failures, lane+": lane recall below criterion")
		}
	}
	if report.FinalContextPrecision < criteria.MinimumFinalContextPrecision {
		report.Failures = append(report.Failures, "final context precision below criterion")
	}
	if report.NegativeFalseCitationRate > criteria.MaximumNegativeFalseCitationRate {
		report.Failures = append(report.Failures, "negative false citation rate above criterion")
	}
	if report.NoEvidenceAccuracy < criteria.MinimumNoEvidenceAccuracy {
		report.Failures = append(report.Failures, "no-evidence accuracy below criterion")
	}
	if report.P95LatencyMilliseconds > criteria.MaximumP95LatencyMilliseconds {
		report.Failures = append(report.Failures, "p95 latency above criterion")
	}
}

func validRate(value float64) bool {
	return value >= 0 && value <= 1 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func containsAll(values []string, expected map[string]struct{}) bool {
	actual := stringSet(values)
	for value := range expected {
		if _, ok := actual[value]; !ok {
			return false
		}
	}
	return true
}

func containsOnly(values []string, allowed map[string]struct{}) bool {
	for _, value := range values {
		if _, ok := allowed[value]; !ok {
			return false
		}
	}
	return true
}

func laneContains(values []RankedEvidence, expected string) bool {
	return slices.ContainsFunc(values, func(value RankedEvidence) bool {
		return value.EvidenceID == expected
	})
}

func hasBlankOrDuplicate(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
		if _, duplicate := seen[value]; duplicate {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 1
	}
	return float64(numerator) / float64(denominator)
}

func percentile95(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	copyOfValues := append([]int64(nil), values...)
	sort.Slice(copyOfValues, func(i, j int) bool { return copyOfValues[i] < copyOfValues[j] })
	index := int(math.Ceil(0.95*float64(len(copyOfValues)))) - 1
	return copyOfValues[max(index, 0)]
}
