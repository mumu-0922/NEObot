package memorycapture

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	ValidationReportSchemaVersion = "neo-chat.memory-regression-relevance-validation.v1"
	ValidationAdmissionMode       = "frozen_validation_only"
)

type ValidationReport struct {
	SchemaVersion                 string                          `json:"schemaVersion"`
	CorpusClass                   string                          `json:"corpusClass"`
	AdmissionMode                 string                          `json:"admissionMode"`
	PromotionEligible             bool                            `json:"promotionEligible"`
	Split                         string                          `json:"split"`
	CaseCount                     int                             `json:"caseCount"`
	PolicyID                      string                          `json:"policyId"`
	ProfileID                     string                          `json:"profileId"`
	ConfigurationSHA256           string                          `json:"configurationSha256"`
	ProviderSimilarityBasisPoints int                             `json:"providerSimilarityBasisPoints"`
	FinalRelevanceBasisPoints     int                             `json:"finalRelevanceBasisPoints"`
	Passed                        bool                            `json:"passed"`
	Evaluation                    memoryeval.ValidationEvaluation `json:"evaluation"`
}

func BuildFrozenValidation(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
) (ValidationReport, []byte, error) {
	policy, ready := usermemory.HybridShadowFrozenPolicy()
	if !ready {
		return ValidationReport{}, nil, fmt.Errorf("%w: frozen relevance policy unavailable", ErrCaptureInvalid)
	}
	descriptor, ok := usermemory.DescribeHybridShadowRelevancePolicy(policy)
	if !ok || (profile.Profile.ID != CandidateProfileID &&
		profile.Profile.ID != FakeCandidateProfileID) ||
		profile.Profile.Role != "candidate" || profile.Profile.ReaderVersion != ReaderVersion ||
		profile.Profile.CandidateLimit != usermemory.MaxHybridShadowResults ||
		profile.Profile.FinalLimit != usermemory.HybridShadowFinalLimit ||
		len(profile.Profile.ConfigurationSHA256) != 64 || profile.Costs.Unit == "" ||
		profile.Costs.MemoryProviderCostMicrounits == 0 ||
		profile.Costs.ChatProviderCostMicrounits == 0 {
		return ValidationReport{}, nil, ErrCaptureInvalid
	}
	if _, err := hex.DecodeString(profile.Profile.ConfigurationSHA256); err != nil {
		return ValidationReport{}, nil, ErrCaptureInvalid
	}
	splitCounts := map[string]int{}
	validation := make([]memoryeval.GoldenCase, 0, 100)
	for _, item := range pool.Corpus.Cases {
		splitCounts[item.Split]++
		if item.Split == FrozenValidationSplit {
			validation = append(validation, item)
		}
	}
	if len(pool.Corpus.Cases) != 500 || splitCounts[DevelopmentCalibrationSplit] != 300 ||
		splitCounts[FrozenValidationSplit] != 100 || splitCounts["holdout"] != 100 ||
		len(profile.Cases) != len(validation) {
		return ValidationReport{}, nil, fmt.Errorf("%w: validation corpus split", ErrCaptureInvalid)
	}
	for index, item := range validation {
		if profile.Cases[index].CaseID != item.ID {
			return ValidationReport{}, nil, fmt.Errorf("%w: validation case order", ErrCaptureInvalid)
		}
	}
	evaluation, err := memoryeval.EvaluateValidationSelection(
		validation,
		profile.Cases,
		pool.Corpus.Criteria,
		profile.Costs,
	)
	if err != nil {
		return ValidationReport{}, nil, err
	}
	report := ValidationReport{
		SchemaVersion: ValidationReportSchemaVersion,
		CorpusClass:   memoryeval.RegressionCorpusClass,
		AdmissionMode: ValidationAdmissionMode, PromotionEligible: false,
		Split: FrozenValidationSplit, CaseCount: len(validation),
		PolicyID: descriptor.ID, ProfileID: profile.Profile.ID,
		ConfigurationSHA256:           profile.Profile.ConfigurationSHA256,
		ProviderSimilarityBasisPoints: descriptor.MinimumProviderSimilarityBasisPoints,
		FinalRelevanceBasisPoints:     descriptor.MinimumFinalRelevanceBasisPoints,
		Passed:                        evaluation.Passed, Evaluation: evaluation,
	}
	body, err := json.Marshal(report)
	if err != nil {
		return ValidationReport{}, nil, fmt.Errorf("%w: encode validation report", ErrCaptureInvalid)
	}
	return report, append(body, '\n'), nil
}
