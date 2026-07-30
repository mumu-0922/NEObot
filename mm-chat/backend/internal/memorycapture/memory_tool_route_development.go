package memorycapture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	MemoryToolRouteDevelopmentReportSchemaVersion      = "neo-chat.memory-regression-relevance-calibration.v6"
	MemoryToolRouteDevelopmentAdmissionMode            = "development_main_model_memory_tool_route_only"
	memoryToolRouteSelectionAlgorithm                  = "main-model-tool-call_then-bge-order_top5-token-budget_v1"
	MemoryToolFirstRoundDevelopmentReportSchemaVersion = "neo-chat.memory-regression-relevance-calibration.v7"
	MemoryToolFirstRoundDevelopmentAdmissionMode       = "development_main_model_first_tool_round_only"
	memoryToolFirstRoundSelectionAlgorithm             = "first-tool-round-call_then-bge-order_top5-token-budget_v1"
	MemoryToolFirstRoundDiagnosticReportSchemaVersion  = "neo-chat.memory-regression-relevance-calibration.v9"
	MemoryToolFirstRoundDiagnosticAdmissionMode        = "development_main_model_first_tool_round_route_failure_diagnostic_only"
	MemoryToolRouteFailureTaxonomyVersion              = "memory-tool-route-failure-taxonomy-v1"
	MemoryToolRouteFailureTaxonomySHA256               = "66f11e91edc0cf5a6a9dbf5dd30336e58a52860adee968fb4658d6ccd70d52a0"
	MemoryToolRouteDiagnosticCompletenessPolicy        = "route_complete_retrieval_fail_closed_v1"
)

type memoryToolRouteReportInvalidReason string

const (
	memoryToolRouteInvalidProfileID             memoryToolRouteReportInvalidReason = "profile_id"
	memoryToolRouteInvalidFailureTaxonomy       memoryToolRouteReportInvalidReason = "failure_taxonomy"
	memoryToolRouteInvalidProfileAuthority      memoryToolRouteReportInvalidReason = "profile_authority"
	memoryToolRouteInvalidConfigurationHash     memoryToolRouteReportInvalidReason = "configuration_hash"
	memoryToolRouteInvalidPolicyAuthority       memoryToolRouteReportInvalidReason = "policy_authority"
	memoryToolRouteInvalidCostAuthority         memoryToolRouteReportInvalidReason = "cost_authority"
	memoryToolRouteInvalidDevelopmentSplit      memoryToolRouteReportInvalidReason = "development_split"
	memoryToolRouteInvalidObservationID         memoryToolRouteReportInvalidReason = "observation_id"
	memoryToolRouteInvalidObservationDuplicate  memoryToolRouteReportInvalidReason = "observation_duplicate"
	memoryToolRouteInvalidTraceIdentity         memoryToolRouteReportInvalidReason = "trace_identity"
	memoryToolRouteInvalidTraceDuplicate        memoryToolRouteReportInvalidReason = "trace_duplicate"
	memoryToolRouteInvalidTraceObservation      memoryToolRouteReportInvalidReason = "trace_observation"
	memoryToolRouteInvalidEmptyCandidateState   memoryToolRouteReportInvalidReason = "empty_candidate_state"
	memoryToolRouteInvalidAdmissionState        memoryToolRouteReportInvalidReason = "admission_state"
	memoryToolRouteInvalidIncompleteRetrieval   memoryToolRouteReportInvalidReason = "incomplete_retrieval_state"
	memoryToolRouteInvalidOutputTokenAuthority  memoryToolRouteReportInvalidReason = "output_token_authority"
	memoryToolRouteInvalidReadyFailureCategory  memoryToolRouteReportInvalidReason = "ready_failure_category"
	memoryToolRouteInvalidAbstainedFinalState   memoryToolRouteReportInvalidReason = "abstained_final_state"
	memoryToolRouteInvalidCompletedRerankState  memoryToolRouteReportInvalidReason = "completed_rerank_state"
	memoryToolRouteInvalidFailureCategory       memoryToolRouteReportInvalidReason = "failure_category"
	memoryToolRouteInvalidFailedFinalState      memoryToolRouteReportInvalidReason = "failed_final_state"
	memoryToolRouteInvalidFailureCategoryTotals memoryToolRouteReportInvalidReason = "failure_category_totals"
)

type MemoryToolRouteDevelopmentDiagnostics struct {
	EmptyCandidateCaseCount    int            `json:"emptyCandidateCaseCount"`
	RouteCompletedCaseCount    int            `json:"routeCompletedCaseCount"`
	RouteUsedCaseCount         int            `json:"routeUsedCaseCount"`
	RouteAbstainedCaseCount    int            `json:"routeAbstainedCaseCount"`
	FailedCaseCount            int            `json:"failedCaseCount"`
	FailureCodeCounts          map[string]int `json:"failureCodeCounts"`
	RouteFailureCategoryCounts map[string]int `json:"routeFailureCategoryCounts,omitempty"`
	RetrievalIncompleteCount   int            `json:"retrievalIncompleteCaseCount,omitempty"`
	RetrievalFailureCodeCounts map[string]int `json:"retrievalFailureCodeCounts,omitempty"`
}

type MemoryToolRouteDevelopmentCostAuthority struct {
	Unit                                string `json:"unit"`
	AuthorizedRequestCount              int    `json:"authorizedRequestCount"`
	ActualRequestCount                  int    `json:"actualRequestCount"`
	AuthorizedMaximumInputTokens        uint64 `json:"authorizedMaximumInputTokens"`
	ActualInputTokenUpperBound          uint64 `json:"actualInputTokenUpperBound"`
	AuthorizedMaximumOutputTokens       uint64 `json:"authorizedMaximumOutputTokens"`
	ActualOutputTokenUpperBound         uint64 `json:"actualOutputTokenUpperBound"`
	MaximumRouteCostMicrounits          uint64 `json:"maximumRouteCostMicrounits"`
	MaximumMemoryProviderCostMicrounits uint64 `json:"maximumMemoryProviderCostMicrounits"`
}

type MemoryToolRouteDevelopmentEvaluation struct {
	memoryeval.CalibrationEvaluation
	ProviderCostRatio float64 `json:"providerCostRatio"`
}

type MemoryToolRouteDevelopmentReport struct {
	SchemaVersion           string                                  `json:"schemaVersion"`
	CorpusClass             string                                  `json:"corpusClass"`
	AdmissionMode           string                                  `json:"admissionMode"`
	PromotionEligible       bool                                    `json:"promotionEligible"`
	Split                   string                                  `json:"split"`
	CaseCount               int                                     `json:"caseCount"`
	PolicyID                string                                  `json:"policyId"`
	ProfileID               string                                  `json:"profileId"`
	ConfigurationSHA256     string                                  `json:"configurationSha256"`
	ProviderEgressPolicy    string                                  `json:"providerEgressPolicy"`
	ProviderCostPolicy      string                                  `json:"providerCostPolicy"`
	ProviderCostAuthorized  bool                                    `json:"providerCostAuthorized"`
	RouteProviderID         string                                  `json:"routeProviderId"`
	RouteProviderType       string                                  `json:"routeProviderType"`
	RouteBaseURLSHA256      string                                  `json:"routeBaseUrlSha256"`
	RouteModelID            string                                  `json:"routeModelId"`
	ToolName                string                                  `json:"toolName"`
	ToolContractVersion     string                                  `json:"toolContractVersion"`
	ToolContractSHA256      string                                  `json:"toolContractSha256"`
	ToolAdapterVersion      string                                  `json:"toolAdapterVersion,omitempty"`
	ToolDecodingProfile     string                                  `json:"toolDecodingProfile,omitempty"`
	ToolMaximumOutputTokens int                                     `json:"toolMaximumOutputTokens,omitempty"`
	ToolTemperature         float64                                 `json:"toolTemperature,omitempty"`
	ToolDisableThinking     bool                                    `json:"toolDisableThinking,omitempty"`
	FailureTaxonomyVersion  string                                  `json:"failureTaxonomyVersion,omitempty"`
	FailureTaxonomySHA256   string                                  `json:"failureTaxonomySha256,omitempty"`
	DiagnosticCompleteness  string                                  `json:"diagnosticCompleteness,omitempty"`
	SelectionAlgorithm      string                                  `json:"selectionAlgorithm"`
	Passed                  bool                                    `json:"passed"`
	Evaluation              MemoryToolRouteDevelopmentEvaluation    `json:"evaluation"`
	Diagnostics             MemoryToolRouteDevelopmentDiagnostics   `json:"diagnostics"`
	CostAuthority           MemoryToolRouteDevelopmentCostAuthority `json:"costAuthority"`
}

// BuildMemoryToolRouteDevelopmentReport evaluates one exact main-model Tool
// route on Development only. It retains aggregate metrics and status counts;
// route queries, Memory content, Tool calls, case IDs, and raw scores never
// enter the report.
func BuildMemoryToolRouteDevelopmentReport(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	authority MemoryToolRouteProfileAuthority,
	costBasis CostBasis,
) (MemoryToolRouteDevelopmentReport, []byte, error) {
	return buildMemoryToolRouteDevelopmentReport(
		pool,
		profile,
		authority,
		costBasis,
		MemoryToolFirstRoundReaderVersion,
		MemoryToolFirstRoundDevelopmentReportSchemaVersion,
		MemoryToolFirstRoundDevelopmentAdmissionMode,
		"",
		"",
	)
}

// BuildMemoryToolRouteDiagnosticReport emits a schema-separated aggregate
// report whose fixed failure taxonomy can explain route failures without
// retaining Provider error text, queries, Tool payloads, or Memory content.
func BuildMemoryToolRouteDiagnosticReport(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	authority MemoryToolRouteProfileAuthority,
	costBasis CostBasis,
) (MemoryToolRouteDevelopmentReport, []byte, error) {
	return buildMemoryToolRouteDevelopmentReport(
		pool,
		profile,
		authority,
		costBasis,
		MemoryToolFirstRoundDiagnosticReaderVersion,
		MemoryToolFirstRoundDiagnosticReportSchemaVersion,
		MemoryToolFirstRoundDiagnosticAdmissionMode,
		MemoryToolRouteFailureTaxonomyVersion,
		MemoryToolRouteDiagnosticCompletenessPolicy,
	)
}

func buildMemoryToolRouteDevelopmentReport(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	authority MemoryToolRouteProfileAuthority,
	costBasis CostBasis,
	readerVersion string,
	reportSchemaVersion string,
	admissionMode string,
	failureTaxonomyVersion string,
	diagnosticCompleteness string,
) (MemoryToolRouteDevelopmentReport, []byte, error) {
	if profile.Profile.ID != CandidateProfileID &&
		profile.Profile.ID != FakeCandidateProfileID {
		return MemoryToolRouteDevelopmentReport{}, nil,
			invalidMemoryToolRouteReport(memoryToolRouteInvalidProfileID)
	}
	if failureTaxonomyVersion != "" &&
		memoryToolRouteFailureTaxonomySHA256() !=
			MemoryToolRouteFailureTaxonomySHA256 {
		return MemoryToolRouteDevelopmentReport{}, nil,
			invalidMemoryToolRouteReport(memoryToolRouteInvalidFailureTaxonomy)
	}
	diagnosticRouteOnly := diagnosticCompleteness != ""
	if diagnosticRouteOnly != (failureTaxonomyVersion != "") ||
		(diagnosticRouteOnly &&
			diagnosticCompleteness != MemoryToolRouteDiagnosticCompletenessPolicy) {
		return MemoryToolRouteDevelopmentReport{}, nil,
			invalidMemoryToolRouteReport(memoryToolRouteInvalidFailureTaxonomy)
	}
	if profile.Profile.ReaderVersion != readerVersion ||
		len(profile.Profile.ConfigurationSHA256) != 64 ||
		profile.Profile.ProviderEgressPolicy !=
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 ||
		profile.Costs.Unit == "" ||
		profile.Costs.MemoryProviderCostMicrounits == 0 ||
		profile.Costs.ChatProviderCostMicrounits == 0 {
		return MemoryToolRouteDevelopmentReport{}, nil,
			invalidMemoryToolRouteReport(memoryToolRouteInvalidProfileAuthority)
	}
	if _, err := hex.DecodeString(profile.Profile.ConfigurationSHA256); err != nil {
		return MemoryToolRouteDevelopmentReport{}, nil,
			invalidMemoryToolRouteReport(memoryToolRouteInvalidConfigurationHash)
	}
	policy, ok := usermemory.DescribeHybridShadowRelevancePolicy(
		usermemory.HybridShadowMemoryFirstToolRoundCalibrationPolicy(authority.ModelID),
	)
	if !ok || !policy.MemoryToolRouteRequired ||
		policy.MemoryToolRouteContractVersion != usermemory.HybridMemoryToolContractVersion ||
		policy.MemoryToolRouteContractSHA256 != usermemory.HybridMemoryToolContractSHA256 ||
		policy.ID != usermemory.HybridRelevanceMemoryFirstToolRoundPolicyID ||
		policy.MemoryToolRouteDecodingProfile != "none" ||
		policy.MemoryToolRouteMaximumOutputTokens != 0 ||
		policy.MemoryToolRouteTemperature != 0 || policy.MemoryToolRouteDisableThinking {
		return MemoryToolRouteDevelopmentReport{}, nil,
			invalidMemoryToolRouteReport(memoryToolRouteInvalidPolicyAuthority)
	}
	if err := ValidateMemoryToolFirstRoundCostAuthority(costBasis, authority); err != nil ||
		costBasis.Candidate != profile.Costs {
		return MemoryToolRouteDevelopmentReport{}, nil,
			invalidMemoryToolRouteReport(memoryToolRouteInvalidCostAuthority)
	}

	development := make([]memoryeval.GoldenCase, 0, 300)
	for _, item := range pool.Corpus.Cases {
		if item.Split == DevelopmentCalibrationSplit {
			development = append(development, item)
		}
	}
	if len(pool.Corpus.Cases) != 500 || len(development) != 300 ||
		len(profile.Cases) != len(development) ||
		len(profile.Calibration) != len(development) {
		return MemoryToolRouteDevelopmentReport{}, nil,
			invalidMemoryToolRouteReport(memoryToolRouteInvalidDevelopmentSplit)
	}
	caseByID := make(map[string]memoryeval.CaseObservation, len(profile.Cases))
	for _, observed := range profile.Cases {
		if observed.CaseID == "" {
			return MemoryToolRouteDevelopmentReport{}, nil,
				invalidMemoryToolRouteReport(memoryToolRouteInvalidObservationID)
		}
		if _, duplicate := caseByID[observed.CaseID]; duplicate {
			return MemoryToolRouteDevelopmentReport{}, nil,
				invalidMemoryToolRouteReport(memoryToolRouteInvalidObservationDuplicate)
		}
		caseByID[observed.CaseID] = observed
	}
	traceByID := make(map[string]CandidateCalibrationTrace, len(profile.Calibration))
	for _, trace := range profile.Calibration {
		if trace.CaseID == "" || trace.FullObservation.CaseID != trace.CaseID {
			return MemoryToolRouteDevelopmentReport{}, nil,
				invalidMemoryToolRouteReport(memoryToolRouteInvalidTraceIdentity)
		}
		if _, duplicate := traceByID[trace.CaseID]; duplicate {
			return MemoryToolRouteDevelopmentReport{}, nil,
				invalidMemoryToolRouteReport(memoryToolRouteInvalidTraceDuplicate)
		}
		traceByID[trace.CaseID] = trace
	}

	ordered := make([]memoryeval.CaseObservation, len(development))
	diagnostics := MemoryToolRouteDevelopmentDiagnostics{
		FailureCodeCounts: make(map[string]int),
	}
	if failureTaxonomyVersion != "" {
		diagnostics.RouteFailureCategoryCounts = make(map[string]int)
		diagnostics.RetrievalFailureCodeCounts = make(map[string]int)
	}
	actualRequests := 0
	actualInputTokenUpperBound := uint64(0)
	actualOutputTokenUpperBound := uint64(0)
	costAuthority := costBasis.MemoryToolRouteAuthority
	authorizedOutputPerRequest := costAuthority.MaximumOutputTokens /
		uint64(costAuthority.RequestCount)
	for index, item := range development {
		observed, observedOK := caseByID[item.ID]
		trace, traceOK := traceByID[item.ID]
		if !observedOK || !traceOK || !trace.PreparedReady ||
			!equalCaseObservation(observed, trace.FullObservation) {
			return MemoryToolRouteDevelopmentReport{}, nil,
				invalidMemoryToolRouteReport(memoryToolRouteInvalidTraceObservation)
		}
		candidateCount := len(observed.CandidateMemoryIDs)
		if candidateCount == 0 {
			diagnostics.EmptyCandidateCaseCount++
			if trace.AdmissionReady || trace.RerankReady ||
				len(observed.ProviderSentMemoryIDs) != 0 ||
				len(observed.FinalMemoryIDs) != 0 {
				return MemoryToolRouteDevelopmentReport{}, nil,
					invalidMemoryToolRouteReport(memoryToolRouteInvalidEmptyCandidateState)
			}
		} else if !trace.AdmissionReady {
			if !diagnosticRouteOnly {
				return MemoryToolRouteDevelopmentReport{}, nil,
					invalidMemoryToolRouteReport(memoryToolRouteInvalidAdmissionState)
			}
			if len(observed.ProviderSentMemoryIDs) != 0 ||
				len(observed.FinalMemoryIDs) != 0 ||
				len(observed.InjectedMemoryIDs) != 0 ||
				observed.PromptMemoryTokens != 0 {
				return MemoryToolRouteDevelopmentReport{}, nil,
					invalidMemoryToolRouteReport(memoryToolRouteInvalidIncompleteRetrieval)
			}
			diagnostics.RetrievalIncompleteCount++
			diagnostics.RetrievalFailureCodeCounts[memoryToolRouteRetrievalFailureCode(trace)]++
		} else if diagnosticRouteOnly && !trace.RerankReady {
			if len(observed.FinalMemoryIDs) != 0 ||
				len(observed.InjectedMemoryIDs) != 0 ||
				observed.PromptMemoryTokens != 0 {
				return MemoryToolRouteDevelopmentReport{}, nil,
					invalidMemoryToolRouteReport(memoryToolRouteInvalidIncompleteRetrieval)
			}
			diagnostics.RetrievalIncompleteCount++
			diagnostics.RetrievalFailureCodeCounts[memoryToolRouteRetrievalFailureCode(trace)]++
		}
		if trace.MemoryToolRouteInputTokenUpperBound > 0 {
			actualRequests++
			actualInputTokenUpperBound += uint64(trace.MemoryToolRouteInputTokenUpperBound)
			if trace.MemoryToolRouteReady {
				if trace.MemoryToolRouteOutputTokenUpperBound <= 0 ||
					uint64(trace.MemoryToolRouteOutputTokenUpperBound) > authorizedOutputPerRequest {
					return MemoryToolRouteDevelopmentReport{}, nil,
						invalidMemoryToolRouteReport(memoryToolRouteInvalidOutputTokenAuthority)
				}
				actualOutputTokenUpperBound +=
					uint64(trace.MemoryToolRouteOutputTokenUpperBound)
			} else {
				actualOutputTokenUpperBound += authorizedOutputPerRequest
			}
		}
		if trace.MemoryToolRouteReady {
			if failureTaxonomyVersion != "" && trace.MemoryToolRouteFailureCategory != "" {
				return MemoryToolRouteDevelopmentReport{}, nil,
					invalidMemoryToolRouteReport(memoryToolRouteInvalidReadyFailureCategory)
			}
			diagnostics.RouteCompletedCaseCount++
			if trace.MemoryToolRouteUsed {
				diagnostics.RouteUsedCaseCount++
			} else {
				diagnostics.RouteAbstainedCaseCount++
				if len(observed.FinalMemoryIDs) != 0 ||
					len(observed.InjectedMemoryIDs) != 0 ||
					observed.PromptMemoryTokens != 0 {
					return MemoryToolRouteDevelopmentReport{}, nil,
						invalidMemoryToolRouteReport(memoryToolRouteInvalidAbstainedFinalState)
				}
			}
			if candidateCount > 0 && !trace.RerankReady && !diagnosticRouteOnly {
				return MemoryToolRouteDevelopmentReport{}, nil,
					invalidMemoryToolRouteReport(memoryToolRouteInvalidCompletedRerankState)
			}
		} else {
			diagnostics.FailedCaseCount++
			if failureTaxonomyVersion != "" {
				if !usermemory.ValidHybridMemoryToolRouteFailureCategory(
					trace.MemoryToolRouteFailureCategory,
				) {
					return MemoryToolRouteDevelopmentReport{}, nil,
						invalidMemoryToolRouteReport(memoryToolRouteInvalidFailureCategory)
				}
				diagnostics.RouteFailureCategoryCounts[trace.MemoryToolRouteFailureCategory]++
			}
			code := normalizeCalibrationCode(trace.AbstentionCode)
			if code == "NONE" {
				code = normalizeCalibrationCode(trace.ResultCode)
			}
			diagnostics.FailureCodeCounts[code]++
			if len(observed.FinalMemoryIDs) != 0 ||
				len(observed.InjectedMemoryIDs) != 0 ||
				observed.PromptMemoryTokens != 0 {
				return MemoryToolRouteDevelopmentReport{}, nil,
					invalidMemoryToolRouteReport(memoryToolRouteInvalidFailedFinalState)
			}
		}
		ordered[index] = observed
	}
	if failureTaxonomyVersion != "" &&
		sumDiagnosticCounts(diagnostics.RouteFailureCategoryCounts) !=
			diagnostics.FailedCaseCount {
		return MemoryToolRouteDevelopmentReport{}, nil,
			invalidMemoryToolRouteReport(memoryToolRouteInvalidFailureCategoryTotals)
	}
	if diagnosticRouteOnly &&
		sumDiagnosticCounts(diagnostics.RetrievalFailureCodeCounts) !=
			diagnostics.RetrievalIncompleteCount {
		return MemoryToolRouteDevelopmentReport{}, nil,
			invalidMemoryToolRouteReport(memoryToolRouteInvalidIncompleteRetrieval)
	}

	if actualRequests > costAuthority.RequestCount ||
		actualInputTokenUpperBound > costAuthority.MaximumInputTokens ||
		actualOutputTokenUpperBound > costAuthority.MaximumOutputTokens {
		return MemoryToolRouteDevelopmentReport{}, nil, fmt.Errorf(
			"%w: Memory Tool route cost authority exceeded",
			ErrCaptureInvalid,
		)
	}
	evaluation, err := memoryeval.EvaluateCalibrationSelectionWithProviderEgressPolicy(
		development,
		ordered,
		pool.Corpus.Criteria,
		memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
	)
	if err != nil {
		return MemoryToolRouteDevelopmentReport{}, nil, err
	}
	providerCostRatio := float64(profile.Costs.MemoryProviderCostMicrounits) /
		float64(profile.Costs.ChatProviderCostMicrounits)
	report := MemoryToolRouteDevelopmentReport{
		SchemaVersion:          reportSchemaVersion,
		CorpusClass:            memoryeval.RegressionCorpusClass,
		AdmissionMode:          admissionMode,
		PromotionEligible:      false,
		Split:                  DevelopmentCalibrationSplit,
		CaseCount:              len(development),
		PolicyID:               policy.ID,
		ProfileID:              profile.Profile.ID,
		ConfigurationSHA256:    profile.Profile.ConfigurationSHA256,
		ProviderEgressPolicy:   memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
		ProviderCostPolicy:     ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
		ProviderCostAuthorized: true,
		RouteProviderID:        authority.ProviderID,
		RouteProviderType:      authority.ProviderType,
		RouteBaseURLSHA256:     authority.BaseURLSHA256,
		RouteModelID:           authority.ModelID,
		ToolName:               usermemory.HybridMemoryToolName,
		ToolContractVersion:    policy.MemoryToolRouteContractVersion,
		ToolContractSHA256:     policy.MemoryToolRouteContractSHA256,
		ToolAdapterVersion:     chat.MemoryToolFirstRoundAdapterVersion,
		FailureTaxonomyVersion: failureTaxonomyVersion,
		DiagnosticCompleteness: diagnosticCompleteness,
		FailureTaxonomySHA256: func() string {
			if failureTaxonomyVersion == "" {
				return ""
			}
			return MemoryToolRouteFailureTaxonomySHA256
		}(),
		SelectionAlgorithm: memoryToolFirstRoundSelectionAlgorithm,
		Passed:             evaluation.Passed,
		Evaluation: MemoryToolRouteDevelopmentEvaluation{
			CalibrationEvaluation: evaluation,
			ProviderCostRatio:     providerCostRatio,
		},
		Diagnostics: diagnostics,
		CostAuthority: MemoryToolRouteDevelopmentCostAuthority{
			Unit:                                costBasis.Candidate.Unit,
			AuthorizedRequestCount:              costAuthority.RequestCount,
			ActualRequestCount:                  actualRequests,
			AuthorizedMaximumInputTokens:        costAuthority.MaximumInputTokens,
			ActualInputTokenUpperBound:          actualInputTokenUpperBound,
			AuthorizedMaximumOutputTokens:       costAuthority.MaximumOutputTokens,
			ActualOutputTokenUpperBound:         actualOutputTokenUpperBound,
			MaximumRouteCostMicrounits:          costAuthority.MaximumCostMicrounits,
			MaximumMemoryProviderCostMicrounits: costBasis.Candidate.MemoryProviderCostMicrounits,
		},
	}
	body, err := json.Marshal(report)
	if err != nil {
		return MemoryToolRouteDevelopmentReport{}, nil, errors.Join(ErrCaptureInvalid, err)
	}
	return report, append(body, '\n'), nil
}

func sumDiagnosticCounts(counts map[string]int) int {
	total := 0
	for _, count := range counts {
		if count < 0 {
			return -1
		}
		total += count
	}
	return total
}

func memoryToolRouteRetrievalFailureCode(trace CandidateCalibrationTrace) string {
	code := normalizeCalibrationCode(trace.AbstentionCode)
	if code == "NONE" {
		code = normalizeCalibrationCode(trace.ResultCode)
	}
	if code == "NONE" || code == "INVALID" {
		return "RETRIEVAL_FAILURE_UNCLASSIFIED"
	}
	return code
}

func invalidMemoryToolRouteReport(reason memoryToolRouteReportInvalidReason) error {
	return fmt.Errorf("%w: Memory Tool-route report %s", ErrCaptureInvalid, reason)
}

func memoryToolRouteFailureTaxonomySHA256() string {
	body, err := json.Marshal(usermemory.HybridMemoryToolRouteFailureCategories())
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
