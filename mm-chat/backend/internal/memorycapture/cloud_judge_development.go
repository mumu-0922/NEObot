package memorycapture

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	CloudJudgeDevelopmentReportSchemaVersion       = "neo-chat.memory-regression-relevance-calibration.v5"
	cloudJudgeDevelopmentLegacyReportSchemaVersion = "neo-chat.memory-regression-relevance-calibration.v4"
	CloudJudgeDevelopmentAdmissionMode             = "development_cloud_judge_only"
	cloudJudgeSelectionAlgorithm                   = "strict-ordinal_intersect-bge-order_top5-token-budget_v1"
)

type CloudJudgeDevelopmentDiagnostics struct {
	EmptyCandidateCaseCount               int            `json:"emptyCandidateCaseCount"`
	NegativePolicyQueryAbstainedCaseCount int            `json:"negativePolicyQueryAbstainedCaseCount,omitempty"`
	JudgeCompletedCaseCount               int            `json:"judgeCompletedCaseCount"`
	JudgeAbstainedCaseCount               int            `json:"judgeAbstainedCaseCount"`
	FailedCaseCount                       int            `json:"failedCaseCount"`
	FailureCodeCounts                     map[string]int `json:"failureCodeCounts"`
}

type CloudJudgeDevelopmentCostAuthority struct {
	Unit                                string `json:"unit,omitempty"`
	AuthorizedRequestCount              int    `json:"authorizedRequestCount"`
	ActualRequestCount                  int    `json:"actualRequestCount"`
	AuthorizedMaximumInputTokens        uint64 `json:"authorizedMaximumInputTokens"`
	ActualInputTokenUpperBound          uint64 `json:"actualInputTokenUpperBound"`
	AuthorizedMaximumOutputTokens       uint64 `json:"authorizedMaximumOutputTokens"`
	ActualOutputTokenUpperBound         uint64 `json:"actualOutputTokenUpperBound"`
	MaximumJudgeCostMicrounits          uint64 `json:"maximumJudgeCostMicrounits"`
	MaximumMemoryProviderCostMicrounits uint64 `json:"maximumMemoryProviderCostMicrounits,omitempty"`
}

type CloudJudgeDevelopmentReport struct {
	SchemaVersion          string                             `json:"schemaVersion"`
	CorpusClass            string                             `json:"corpusClass"`
	AdmissionMode          string                             `json:"admissionMode"`
	PromotionEligible      bool                               `json:"promotionEligible"`
	Split                  string                             `json:"split"`
	CaseCount              int                                `json:"caseCount"`
	PolicyID               string                             `json:"policyId"`
	ProfileID              string                             `json:"profileId"`
	ConfigurationSHA256    string                             `json:"configurationSha256"`
	ProviderEgressPolicy   string                             `json:"providerEgressPolicy"`
	ProviderCostPolicy     string                             `json:"providerCostPolicy,omitempty"`
	ProviderCostAuthorized bool                               `json:"providerCostAuthorized,omitempty"`
	JudgeModelID           string                             `json:"judgeModelId"`
	JudgePromptVersion     string                             `json:"judgePromptVersion"`
	JudgePromptSHA256      string                             `json:"judgePromptSha256"`
	JudgeDecodingProfile   string                             `json:"judgeDecodingProfile"`
	SelectionAlgorithm     string                             `json:"selectionAlgorithm"`
	Passed                 bool                               `json:"passed"`
	Evaluation             CloudJudgeDevelopmentEvaluation    `json:"evaluation"`
	Diagnostics            CloudJudgeDevelopmentDiagnostics   `json:"diagnostics"`
	CostAuthority          CloudJudgeDevelopmentCostAuthority `json:"costAuthority"`
}

type CloudJudgeDevelopmentEvaluation struct {
	memoryeval.CalibrationEvaluation
	ProviderCostRatio  float64 `json:"providerCostRatio"`
	ProviderCostPassed *bool   `json:"providerCostPassed,omitempty"`
}

// BuildCloudJudgeDevelopmentReport evaluates the one precommitted cloud-judge
// policy on Development only. It retains aggregate metrics/status counts and
// never serializes traces, case IDs, query text, Memory content, scores, or raw
// judge output.
func BuildCloudJudgeDevelopmentReport(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	judgeModelID string,
	costBasis CostBasis,
) (CloudJudgeDevelopmentReport, []byte, error) {
	return buildCloudJudgeDevelopmentReport(
		pool,
		profile,
		judgeModelID,
		costBasis,
		false,
	)
}

func buildCloudJudgeDevelopmentReport(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	judgeModelID string,
	costBasis CostBasis,
	allowPreJudgeRetrievalFailure bool,
) (CloudJudgeDevelopmentReport, []byte, error) {
	policy := usermemory.HybridShadowCloudJudgeCalibrationPolicy(judgeModelID)
	return buildCloudJudgeDevelopmentReportForPolicy(
		pool,
		profile,
		policy,
		pool.Corpus.Criteria,
		costBasis,
		allowPreJudgeRetrievalFailure,
	)
}

func buildCloudJudgeDevelopmentReportForPolicy(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	policyValue usermemory.HybridShadowRelevancePolicy,
	criteria memoryeval.Criteria,
	costBasis CostBasis,
	allowPreJudgeRetrievalFailure bool,
) (CloudJudgeDevelopmentReport, []byte, error) {
	if profile.Profile.ID != CandidateProfileID &&
		profile.Profile.ID != FakeCandidateProfileID {
		return CloudJudgeDevelopmentReport{}, nil, ErrCaptureInvalid
	}
	if profile.Profile.ReaderVersion != CloudJudgeReaderVersion ||
		len(profile.Profile.ConfigurationSHA256) != 64 ||
		profile.Profile.ProviderEgressPolicy !=
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 ||
		profile.Costs.Unit == "" ||
		profile.Costs.MemoryProviderCostMicrounits == 0 ||
		profile.Costs.ChatProviderCostMicrounits == 0 {
		return CloudJudgeDevelopmentReport{}, nil, ErrCaptureInvalid
	}
	if _, err := hex.DecodeString(profile.Profile.ConfigurationSHA256); err != nil {
		return CloudJudgeDevelopmentReport{}, nil, ErrCaptureInvalid
	}
	policy, ok := usermemory.DescribeHybridShadowRelevancePolicy(
		policyValue,
	)
	if !ok {
		return CloudJudgeDevelopmentReport{}, nil, ErrCaptureInvalid
	}
	if err := ValidateCloudJudgeCostAuthority(
		costBasis,
		policy.CloudCandidateJudgeModelID,
	); err != nil ||
		costBasis.Candidate != profile.Costs {
		return CloudJudgeDevelopmentReport{}, nil, ErrCaptureInvalid
	}
	aggregate, err := aggregateCloudJudgeDevelopment(
		pool,
		profile,
		allowPreJudgeRetrievalFailure,
	)
	if err != nil {
		return CloudJudgeDevelopmentReport{}, nil, err
	}
	development := aggregate.development
	ordered := aggregate.ordered
	diagnostics := aggregate.diagnostics
	actualJudgeRequests := aggregate.logicalJudgeRequests
	actualInputTokenUpperBound := aggregate.logicalInputTokenUpperBound
	authority := costBasis.CloudJudgeAuthority
	actualOutputTokenUpperBound := uint64(actualJudgeRequests) *
		usermemory.HybridCandidateJudgeMaximumOutputTokens
	if actualJudgeRequests > authority.RequestCount ||
		actualInputTokenUpperBound > authority.MaximumInputTokens ||
		actualOutputTokenUpperBound > authority.MaximumOutputTokens {
		return CloudJudgeDevelopmentReport{}, nil, fmt.Errorf(
			"%w: cloud-judge cost authority exceeded",
			ErrCaptureInvalid,
		)
	}
	evaluation, schemaVersion, costAuthorized, err := evaluateCloudJudgeDevelopment(
		development, ordered, criteria, profile.Costs,
		costBasis.ProviderCostPolicy,
	)
	if err != nil {
		return CloudJudgeDevelopmentReport{}, nil, err
	}
	report := CloudJudgeDevelopmentReport{
		SchemaVersion:          schemaVersion,
		CorpusClass:            memoryeval.RegressionCorpusClass,
		AdmissionMode:          CloudJudgeDevelopmentAdmissionMode,
		PromotionEligible:      false,
		Split:                  DevelopmentCalibrationSplit,
		CaseCount:              len(development),
		PolicyID:               policy.ID,
		ProfileID:              profile.Profile.ID,
		ConfigurationSHA256:    profile.Profile.ConfigurationSHA256,
		ProviderEgressPolicy:   memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
		ProviderCostPolicy:     costBasis.ProviderCostPolicy,
		ProviderCostAuthorized: costAuthorized,
		JudgeModelID:           policy.CloudCandidateJudgeModelID,
		JudgePromptVersion:     policy.CloudCandidateJudgePromptVersion,
		JudgePromptSHA256:      policy.CloudCandidateJudgePromptSHA256,
		JudgeDecodingProfile:   policy.CloudCandidateJudgeDecodingProfile,
		SelectionAlgorithm:     cloudJudgeSelectionAlgorithm,
		Passed:                 evaluation.Passed,
		Evaluation:             evaluation,
		Diagnostics:            diagnostics,
		CostAuthority: CloudJudgeDevelopmentCostAuthority{
			Unit:                                costUnitForPolicy(costBasis),
			AuthorizedRequestCount:              authority.RequestCount,
			ActualRequestCount:                  actualJudgeRequests,
			AuthorizedMaximumInputTokens:        authority.MaximumInputTokens,
			ActualInputTokenUpperBound:          actualInputTokenUpperBound,
			AuthorizedMaximumOutputTokens:       authority.MaximumOutputTokens,
			ActualOutputTokenUpperBound:         actualOutputTokenUpperBound,
			MaximumJudgeCostMicrounits:          authority.MaximumCostMicrounits,
			MaximumMemoryProviderCostMicrounits: maximumMemoryProviderCostForPolicy(costBasis),
		},
	}
	body, err := json.Marshal(report)
	if err != nil {
		return CloudJudgeDevelopmentReport{}, nil, errors.Join(ErrCaptureInvalid, err)
	}
	return report, append(body, '\n'), nil
}

type cloudJudgeDevelopmentAggregate struct {
	development                 []memoryeval.GoldenCase
	ordered                     []memoryeval.CaseObservation
	diagnostics                 CloudJudgeDevelopmentDiagnostics
	logicalJudgeRequests        int
	logicalInputTokenUpperBound uint64
}

func aggregateCloudJudgeDevelopment(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	allowPreJudgeRetrievalFailure bool,
) (cloudJudgeDevelopmentAggregate, error) {
	return aggregateCloudJudgeCaptureSplit(
		pool,
		profile,
		DevelopmentCalibrationSplit,
		300,
		allowPreJudgeRetrievalFailure,
		false,
	)
}

func aggregateCloudJudgeCaptureSplit(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	split string,
	expectedCaseCount int,
	allowPreJudgeRetrievalFailure bool,
	allowNegativePolicyGuardAbstention bool,
) (cloudJudgeDevelopmentAggregate, error) {
	if (split != DevelopmentCalibrationSplit && split != FrozenValidationSplit) ||
		expectedCaseCount < 1 {
		return cloudJudgeDevelopmentAggregate{}, ErrCaptureInvalid
	}
	selected := make([]memoryeval.GoldenCase, 0, expectedCaseCount)
	for _, item := range pool.Corpus.Cases {
		if item.Split == split {
			selected = append(selected, item)
		}
	}
	if len(pool.Corpus.Cases) != 500 || len(selected) != expectedCaseCount ||
		len(profile.Cases) != len(selected) ||
		len(profile.Calibration) != len(selected) {
		return cloudJudgeDevelopmentAggregate{}, fmt.Errorf(
			"%w: cloud-judge capture split",
			ErrCaptureInvalid,
		)
	}
	caseByID := make(map[string]memoryeval.CaseObservation, len(profile.Cases))
	for _, observed := range profile.Cases {
		if observed.CaseID == "" {
			return cloudJudgeDevelopmentAggregate{}, ErrCaptureInvalid
		}
		if _, duplicate := caseByID[observed.CaseID]; duplicate {
			return cloudJudgeDevelopmentAggregate{}, ErrCaptureInvalid
		}
		caseByID[observed.CaseID] = observed
	}
	traceByID := make(map[string]CandidateCalibrationTrace, len(profile.Calibration))
	for _, trace := range profile.Calibration {
		if trace.CaseID == "" || trace.FullObservation.CaseID != trace.CaseID {
			return cloudJudgeDevelopmentAggregate{}, ErrCaptureInvalid
		}
		if _, duplicate := traceByID[trace.CaseID]; duplicate {
			return cloudJudgeDevelopmentAggregate{}, ErrCaptureInvalid
		}
		traceByID[trace.CaseID] = trace
	}

	aggregate := cloudJudgeDevelopmentAggregate{
		development: selected,
		ordered:     make([]memoryeval.CaseObservation, len(selected)),
		diagnostics: CloudJudgeDevelopmentDiagnostics{
			FailureCodeCounts: make(map[string]int),
		},
	}
	for index, item := range selected {
		observed, observedOK := caseByID[item.ID]
		trace, traceOK := traceByID[item.ID]
		if !observedOK || !traceOK || !trace.PreparedReady ||
			!equalCaseObservation(observed, trace.FullObservation) {
			return cloudJudgeDevelopmentAggregate{}, ErrCaptureInvalid
		}
		candidateCount := len(observed.CandidateMemoryIDs)
		if candidateCount == 0 {
			if trace.AdmissionReady || trace.RerankReady || trace.CloudJudgeReady ||
				trace.CloudJudgeInputTokenUpperBound != 0 ||
				len(observed.ProviderSentMemoryIDs) != 0 ||
				len(observed.FinalMemoryIDs) != 0 {
				return cloudJudgeDevelopmentAggregate{}, ErrCaptureInvalid
			}
			aggregate.diagnostics.EmptyCandidateCaseCount++
		} else {
			if !trace.AdmissionReady {
				guardAbstained := allowNegativePolicyGuardAbstention &&
					trace.AbstentionCode == "NEGATIVE_POLICY_QUERY_ABSTAINED" &&
					trace.ResultCode == "NO_CANDIDATES"
				if guardAbstained {
					if trace.RerankReady || trace.CloudJudgeReady ||
						trace.CloudJudgeInputTokenUpperBound != 0 ||
						trace.CloudJudgeFailureCategory != "" ||
						trace.MemoryToolRouteReady || trace.MemoryToolRouteUsed ||
						trace.MemoryToolRouteFailureCategory != "" ||
						trace.MemoryToolRouteInputTokenUpperBound != 0 ||
						trace.MemoryToolRouteOutputTokenUpperBound != 0 ||
						len(trace.FinalRelevanceScores) != 0 ||
						len(observed.ProviderSentMemoryIDs) != 0 ||
						len(observed.FinalMemoryIDs) != 0 ||
						len(observed.InjectedMemoryIDs) != 0 ||
						observed.PromptMemoryTokens != 0 ||
						observed.HardCutoffApplied || observed.Fallback != "no_memory" {
						return cloudJudgeDevelopmentAggregate{}, ErrCaptureInvalid
					}
					aggregate.diagnostics.NegativePolicyQueryAbstainedCaseCount++
					aggregate.ordered[index] = observed
					continue
				}
				if !allowPreJudgeRetrievalFailure || trace.RerankReady ||
					trace.CloudJudgeReady || trace.CloudJudgeInputTokenUpperBound != 0 ||
					len(observed.ProviderSentMemoryIDs) != 0 ||
					len(observed.FinalMemoryIDs) != 0 ||
					len(observed.InjectedMemoryIDs) != 0 || observed.PromptMemoryTokens != 0 {
					return cloudJudgeDevelopmentAggregate{}, ErrCaptureInvalid
				}
				aggregate.diagnostics.FailedCaseCount++
				code := normalizeCalibrationCode(trace.AbstentionCode)
				if code == "NONE" {
					code = normalizeCalibrationCode(trace.ResultCode)
				}
				aggregate.diagnostics.FailureCodeCounts[code]++
				aggregate.ordered[index] = observed
				continue
			}
			if trace.RerankReady && trace.CloudJudgeReady {
				if trace.CloudJudgeInputTokenUpperBound <= 0 {
					return cloudJudgeDevelopmentAggregate{}, ErrCaptureInvalid
				}
				aggregate.diagnostics.JudgeCompletedCaseCount++
				if len(observed.FinalMemoryIDs) == 0 {
					aggregate.diagnostics.JudgeAbstainedCaseCount++
				}
			} else {
				aggregate.diagnostics.FailedCaseCount++
				code := normalizeCalibrationCode(trace.AbstentionCode)
				if code == "NONE" {
					code = normalizeCalibrationCode(trace.ResultCode)
				}
				aggregate.diagnostics.FailureCodeCounts[code]++
				if len(observed.FinalMemoryIDs) != 0 ||
					len(observed.InjectedMemoryIDs) != 0 ||
					observed.PromptMemoryTokens != 0 {
					return cloudJudgeDevelopmentAggregate{}, ErrCaptureInvalid
				}
			}
			if trace.CloudJudgeInputTokenUpperBound > 0 {
				aggregate.logicalJudgeRequests++
				aggregate.logicalInputTokenUpperBound +=
					uint64(trace.CloudJudgeInputTokenUpperBound)
			}
		}
		aggregate.ordered[index] = observed
	}
	return aggregate, nil
}

func evaluateCloudJudgeDevelopment(
	cases []memoryeval.GoldenCase,
	observations []memoryeval.CaseObservation,
	criteria memoryeval.Criteria,
	costs memoryeval.ProviderCosts,
	providerCostPolicy string,
) (CloudJudgeDevelopmentEvaluation, string, bool, error) {
	if providerCostPolicy == "" {
		evaluation, err := memoryeval.EvaluateValidationSelectionWithProviderEgressPolicy(
			cases,
			observations,
			criteria,
			costs,
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
		)
		costPassed := evaluation.ProviderCostPassed
		return CloudJudgeDevelopmentEvaluation{
			CalibrationEvaluation: evaluation.CalibrationEvaluation,
			ProviderCostRatio:     evaluation.ProviderCostRatio,
			ProviderCostPassed:    &costPassed,
		}, cloudJudgeDevelopmentLegacyReportSchemaVersion, false, err
	}
	if providerCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 {
		return CloudJudgeDevelopmentEvaluation{}, "", false, ErrCaptureInvalid
	}
	evaluation, err := memoryeval.EvaluateCalibrationSelectionWithProviderEgressPolicy(
		cases,
		observations,
		criteria,
		memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
	)
	if err != nil {
		return CloudJudgeDevelopmentEvaluation{}, "", false, err
	}
	return CloudJudgeDevelopmentEvaluation{
		CalibrationEvaluation: evaluation,
		ProviderCostRatio: float64(costs.MemoryProviderCostMicrounits) /
			float64(costs.ChatProviderCostMicrounits),
	}, CloudJudgeDevelopmentReportSchemaVersion, true, nil
}

func costUnitForPolicy(cost CostBasis) string {
	if cost.ProviderCostPolicy == ProviderCostPolicyOwnerAuthorizedAbsoluteV1 {
		return cost.Candidate.Unit
	}
	return ""
}

func maximumMemoryProviderCostForPolicy(cost CostBasis) uint64 {
	if cost.ProviderCostPolicy == ProviderCostPolicyOwnerAuthorizedAbsoluteV1 {
		return cost.Candidate.MemoryProviderCostMicrounits
	}
	return 0
}

func equalCaseObservation(left, right memoryeval.CaseObservation) bool {
	leftBody, leftErr := json.Marshal(left)
	rightBody, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftBody) == string(rightBody)
}
