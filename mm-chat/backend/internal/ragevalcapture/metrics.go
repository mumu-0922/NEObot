package ragevalcapture

import (
	"fmt"
	"sort"

	"neo-chat/mm-chat/backend/internal/rageval"
)

type preflightMetrics struct {
	candidate rageval.PromotionProfileSummary
	slices    map[string]PreflightSliceResult
	budgets   PreflightBudgets
}

func summarizePreflightMetrics(
	cases []rageval.PromotionGoldenCase,
	candidate []PreflightObservation,
	criteria rageval.PromotionCriteria,
) (preflightMetrics, error) {
	observations := asPromotionObservations(candidate)
	candidateSummary, err := rageval.SummarizePromotionProfile(cases, observations)
	if err != nil {
		return preflightMetrics{}, fmt.Errorf("summarize Candidate capture: %w", err)
	}

	sliceCounts := preflightSliceCounts(cases)
	slices := make(map[string]PreflightSliceResult, len(sliceCounts))
	for _, name := range rageval.PromotionCriticalSlices() {
		count := sliceCounts[name]
		evaluated := count > 0
		metrics := candidateSummary.MetricsByCriticalSlice[name]
		integrity := preflightSliceIntegrity(cases, observations, name)
		failures := make([]string, 0)
		if evaluated {
			failures = append(failures, rageval.PromotionAbsoluteFailures(metrics)...)
			if !integrity.Passed {
				failures = append(
					failures,
					"citation/locator integrity is not complete",
				)
			}
		} else {
			failures = append(failures, "slice is not evaluated")
		}
		sort.Strings(failures)
		slices[name] = PreflightSliceResult{
			Cases: count, Evaluated: evaluated, Metrics: metrics,
			Integrity: integrity, Passed: len(failures) == 0, Failures: failures,
		}
	}

	return preflightMetrics{
		candidate: candidateSummary,
		slices:    slices,
		budgets: PreflightBudgets{
			CandidateP95LatencyMilliseconds: candidateSummary.P95LatencyMilliseconds,
			MaximumP95LatencyMilliseconds:   criteria.MaximumP95LatencyMilliseconds,
			CandidateAverageContextTokens:   candidateSummary.AverageContextTokens,
			MaximumAverageContextTokens:     criteria.MaximumAverageContextTokens,
			LatencyPassed: candidateSummary.P95LatencyMilliseconds <=
				criteria.MaximumP95LatencyMilliseconds,
			ContextTokenCostPassed: candidateSummary.AverageContextTokens <=
				criteria.MaximumAverageContextTokens,
		},
	}, nil
}

func preflightSliceIntegrity(
	cases []rageval.PromotionGoldenCase,
	observations []rageval.PromotionCaseObservation,
	slice string,
) rageval.PromotionIntegrity {
	caseIDs := make(map[string]struct{})
	for _, item := range cases {
		for _, name := range item.Slices {
			if name == slice {
				caseIDs[item.ID] = struct{}{}
				break
			}
		}
	}
	selected := make([]rageval.PromotionCaseObservation, 0, len(caseIDs))
	for _, item := range observations {
		if _, ok := caseIDs[item.CaseID]; ok {
			selected = append(selected, item)
		}
	}
	return rageval.SummarizePromotionIntegrity(selected)
}

func asPromotionObservations(
	observations []PreflightObservation,
) []rageval.PromotionCaseObservation {
	converted := make([]rageval.PromotionCaseObservation, len(observations))
	for index, item := range observations {
		converted[index] = rageval.PromotionCaseObservation{
			CaseID:               item.CaseID,
			RetrievedEvidenceIDs: item.RetrievedEvidenceIDs,
			FinalEvidenceIDs:     item.FinalEvidenceIDs,
			CitationEvidenceIDs:  item.CitationEvidenceIDs,
			Answered:             item.Answered,
			AnswerCorrectness:    item.AnswerCorrectness,
			Faithfulness:         item.Faithfulness,
			TableExactAnswer:     item.TableExactAnswer,
			LatencyMilliseconds:  item.LatencyMilliseconds,
			ContextTokens:        item.ContextTokens,
			Integrity:            item.Integrity,
			Leakage:              item.Leakage,
		}
	}
	return converted
}

func preflightSliceCounts(cases []rageval.PromotionGoldenCase) map[string]int {
	counts := make(map[string]int, len(rageval.PromotionCriticalSlices()))
	for _, name := range rageval.PromotionCriticalSlices() {
		counts[name] = 0
	}
	for _, item := range cases {
		seen := make(map[string]struct{}, len(item.Slices))
		for _, name := range item.Slices {
			if _, duplicate := seen[name]; duplicate {
				continue
			}
			seen[name] = struct{}{}
			if _, critical := counts[name]; critical {
				counts[name]++
			}
		}
	}
	return counts
}
