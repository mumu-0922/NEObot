package rageval

import (
	"errors"
	"fmt"
	"math"
)

type promotionMetricAccumulator struct {
	cases                     int
	relevantCases             int
	negativeCases             int
	tableCases                int
	expectedEvidence          int
	retrievedRelevantEvidence int
	finalRelevantEvidence     int
	citedEvidence             int
	citedRelevantEvidence     int
	falseAnswerCases          int
	tableExactCases           int
	locatorValidCases         int
	provenanceValidCases      int
	cellLineageValidCases     int
	jointLineageValidCases    int
	aclLeaks                  int
	deletionLeaks             int
	secretLeaks               int
	unauthorizedEvidenceLeaks int
	latencies                 []int64
	contextTokens             int
	ndcg                      float64
	mrr                       float64
	faithfulness              float64
	answerCorrectness         float64
}

type evaluatedPromotionProfile struct {
	metrics              PromotionMetrics
	qualityScore         float64
	p95Latency           int64
	averageContextTokens float64
	sliceQuality         map[string]float64
	sliceMetrics         map[string]PromotionMetrics
}

// SummarizePromotionProfile computes aggregates with the same implementation
// as EvaluatePromotion, but does not apply frozen-corpus admission, Holdout, or
// promotion gates. The supplied cases and observations must match exactly.
func SummarizePromotionProfile(
	cases []PromotionGoldenCase,
	observations []PromotionCaseObservation,
) (PromotionProfileSummary, error) {
	evaluated, err := evaluatePromotionProfile(
		PromotionGoldenSet{Cases: cases},
		PromotionObservationSet{Cases: observations},
	)
	if err != nil {
		return PromotionProfileSummary{}, err
	}
	return PromotionProfileSummary{
		Metrics:                evaluated.metrics,
		QualityScore:           evaluated.qualityScore,
		P95LatencyMilliseconds: evaluated.p95Latency,
		AverageContextTokens:   evaluated.averageContextTokens,
		QualityScoreByCriticalSlice: clonePromotionSliceQuality(
			evaluated.sliceQuality,
		),
		MetricsByCriticalSlice: clonePromotionSliceMetrics(
			evaluated.sliceMetrics,
		),
	}, nil
}

func clonePromotionSliceMetrics(
	source map[string]PromotionMetrics,
) map[string]PromotionMetrics {
	cloned := make(map[string]PromotionMetrics, len(source))
	for name, metrics := range source {
		cloned[name] = metrics
	}
	return cloned
}

func clonePromotionSliceQuality(source map[string]float64) map[string]float64 {
	cloned := make(map[string]float64, len(source))
	for name, score := range source {
		cloned[name] = score
	}
	return cloned
}

func evaluatePromotionProfile(
	golden PromotionGoldenSet,
	observations PromotionObservationSet,
) (evaluatedPromotionProfile, error) {
	observedByCase := make(
		map[string]PromotionCaseObservation,
		len(observations.Cases),
	)
	for _, item := range observations.Cases {
		observedByCase[item.CaseID] = item
	}
	if len(observedByCase) != len(golden.Cases) {
		return evaluatedPromotionProfile{}, errors.New(
			"promotion observation cases do not exactly match the Golden corpus",
		)
	}

	all := promotionMetricAccumulator{}
	bySlice := make(map[string]*promotionMetricAccumulator)
	for _, name := range criticalPromotionSlices {
		bySlice[name] = &promotionMetricAccumulator{}
	}
	for _, goldenCase := range golden.Cases {
		observed, ok := observedByCase[goldenCase.ID]
		if !ok {
			return evaluatedPromotionProfile{}, fmt.Errorf(
				"missing promotion observation case %q",
				goldenCase.ID,
			)
		}
		delete(observedByCase, goldenCase.ID)
		accumulatePromotionCase(&all, goldenCase, observed)
		for _, name := range goldenCase.Slices {
			if accumulator, exists := bySlice[name]; exists {
				accumulatePromotionCase(accumulator, goldenCase, observed)
			}
		}
	}
	if len(observedByCase) != 0 {
		return evaluatedPromotionProfile{}, errors.New(
			"promotion observations contain unknown cases",
		)
	}

	metrics := all.metrics()
	sliceQuality := make(map[string]float64, len(bySlice))
	sliceMetrics := make(map[string]PromotionMetrics, len(bySlice))
	for name, accumulator := range bySlice {
		metrics := accumulator.metrics()
		sliceMetrics[name] = metrics
		sliceQuality[name] = promotionQualityScore(metrics)
	}
	return evaluatedPromotionProfile{
		metrics:              metrics,
		qualityScore:         promotionQualityScore(metrics),
		p95Latency:           percentile95(all.latencies),
		averageContextTokens: ratioFloat(float64(all.contextTokens), all.cases),
		sliceQuality:         sliceQuality,
		sliceMetrics:         sliceMetrics,
	}, nil
}

func accumulatePromotionCase(
	accumulator *promotionMetricAccumulator,
	golden PromotionGoldenCase,
	observed PromotionCaseObservation,
) {
	accumulator.cases++
	accumulator.latencies = append(
		accumulator.latencies,
		observed.LatencyMilliseconds,
	)
	accumulator.contextTokens += observed.ContextTokens
	if observed.Integrity.CitationLocatorValid {
		accumulator.locatorValidCases++
	}
	if observed.Integrity.ProvenanceValid {
		accumulator.provenanceValidCases++
	}
	if observed.Integrity.CellLineageValid {
		accumulator.cellLineageValidCases++
	}
	if observed.Integrity.ProvenanceValid && observed.Integrity.CellLineageValid {
		accumulator.jointLineageValidCases++
	}
	if observed.Leakage.ACL {
		accumulator.aclLeaks++
	}
	if observed.Leakage.Deletion {
		accumulator.deletionLeaks++
	}
	if observed.Leakage.Secret {
		accumulator.secretLeaks++
	}
	if observed.Leakage.UnauthorizedEvidence {
		accumulator.unauthorizedEvidenceLeaks++
	}
	accumulator.citedEvidence += len(observed.CitationEvidenceIDs)

	if golden.ExpectedNoAnswer {
		accumulator.negativeCases++
		if observed.Answered {
			accumulator.falseAnswerCases++
		}
		return
	}

	relevant := stringSet(golden.ExpectedRelevantEvidenceIDs)
	accumulator.relevantCases++
	accumulator.expectedEvidence += len(relevant)
	accumulator.retrievedRelevantEvidence += countAllowed(
		observed.RetrievedEvidenceIDs,
		relevant,
	)
	accumulator.finalRelevantEvidence += countAllowed(
		observed.FinalEvidenceIDs,
		relevant,
	)
	accumulator.citedRelevantEvidence += countAllowed(
		observed.CitationEvidenceIDs,
		relevant,
	)
	accumulator.ndcg += ndcgAt10(observed.FinalEvidenceIDs, relevant)
	accumulator.mrr += reciprocalRankAt10(observed.FinalEvidenceIDs, relevant)
	accumulator.faithfulness += observed.Faithfulness
	accumulator.answerCorrectness += observed.AnswerCorrectness
	if golden.TableExactAnswerRequired {
		accumulator.tableCases++
		if observed.TableExactAnswer {
			accumulator.tableExactCases++
		}
	}
}

func (value promotionMetricAccumulator) metrics() PromotionMetrics {
	return PromotionMetrics{
		RecallAt50:                    ratio(value.retrievedRelevantEvidence, value.expectedEvidence),
		FinalRecallAt10:               ratio(value.finalRelevantEvidence, value.expectedEvidence),
		NDCGAt10:                      ratioFloat(value.ndcg, value.relevantCases),
		MRRAt10:                       ratioFloat(value.mrr, value.relevantCases),
		CitationCorrectness:           ratio(value.citedRelevantEvidence, value.citedEvidence),
		CitationCompleteness:          ratio(value.citedRelevantEvidence, value.expectedEvidence),
		Faithfulness:                  ratioFloat(value.faithfulness, value.relevantCases),
		AnswerCorrectness:             ratioFloat(value.answerCorrectness, value.relevantCases),
		NoAnswerFalseAnswerRate:       errorRatio(value.falseAnswerCases, value.negativeCases),
		TableExactAnswer:              ratio(value.tableExactCases, value.tableCases),
		ProvenanceCellLineage:         ratio(value.jointLineageValidCases, value.cases),
		ACLLeakCount:                  value.aclLeaks,
		DeletionLeakCount:             value.deletionLeaks,
		SecretLeakCount:               value.secretLeaks,
		UnauthorizedEvidenceLeakCount: value.unauthorizedEvidenceLeaks,
	}
}

func promotionQualityScore(metrics PromotionMetrics) float64 {
	return (metrics.RecallAt50 +
		metrics.FinalRecallAt10 +
		metrics.NDCGAt10 +
		metrics.MRRAt10 +
		metrics.CitationCorrectness +
		metrics.CitationCompleteness +
		metrics.Faithfulness +
		metrics.AnswerCorrectness +
		metrics.TableExactAnswer +
		(1 - metrics.NoAnswerFalseAnswerRate)) / 10
}

func promotionSplitCounts(cases []PromotionGoldenCase) map[string]int {
	counts := map[string]int{
		"development": 0,
		"validation":  0,
		"holdout":     0,
	}
	for _, item := range cases {
		counts[item.Split]++
	}
	return counts
}

func promotionSliceCount(cases []PromotionGoldenCase, name string) int {
	count := 0
	for _, item := range cases {
		for _, slice := range item.Slices {
			if slice == name {
				count++
				break
			}
		}
	}
	return count
}

func candidateMetricRate(
	cases []PromotionCaseObservation,
	predicate func(PromotionCaseObservation) bool,
) float64 {
	valid := 0
	for _, item := range cases {
		if predicate(item) {
			valid++
		}
	}
	return ratio(valid, len(cases))
}

func countAllowed(values []string, allowed map[string]struct{}) int {
	count := 0
	for _, value := range values {
		if _, ok := allowed[value]; ok {
			count++
		}
	}
	return count
}

func reciprocalRankAt10(values []string, relevant map[string]struct{}) float64 {
	for index, value := range values {
		if index >= 10 {
			break
		}
		if _, ok := relevant[value]; ok {
			return 1 / float64(index+1)
		}
	}
	return 0
}

func ndcgAt10(values []string, relevant map[string]struct{}) float64 {
	dcg := 0.0
	for index, value := range values {
		if index >= 10 {
			break
		}
		if _, ok := relevant[value]; ok {
			dcg += 1 / math.Log2(float64(index)+2)
		}
	}
	idealHits := min(len(relevant), 10)
	ideal := 0.0
	for index := 0; index < idealHits; index++ {
		ideal += 1 / math.Log2(float64(index)+2)
	}
	if ideal == 0 {
		return 1
	}
	return dcg / ideal
}

func ratioFloat(numerator float64, denominator int) float64 {
	if denominator == 0 {
		return 1
	}
	return numerator / float64(denominator)
}

func errorRatio(numerator int, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
