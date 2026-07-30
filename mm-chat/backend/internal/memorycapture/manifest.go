package memorycapture

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	RunManifestSchemaVersion          = "neo-chat.memory-regression-native-run.v1"
	RelevanceRunManifestSchemaVersion = "neo-chat.memory-regression-relevance-run.v1"
)

type RunManifest struct {
	SchemaVersion     string                `json:"schemaVersion"`
	RunID             string                `json:"runId"`
	CaptureID         string                `json:"captureId"`
	CorpusClass       string                `json:"corpusClass"`
	AdmissionMode     string                `json:"admissionMode"`
	PromotionEligible bool                  `json:"promotionEligible"`
	ProviderMode      string                `json:"providerMode"`
	StartedAt         string                `json:"startedAt"`
	CompletedAt       string                `json:"completedAt"`
	CostBasisSHA256   string                `json:"costBasisSha256"`
	Inputs            RunInputHashes        `json:"inputs"`
	Profiles          []RunProfileManifest  `json:"profiles"`
	Artifacts         []RunArtifactManifest `json:"artifacts"`
}

type RunInputHashes struct {
	FixtureRawSHA256  string `json:"fixtureRawSha256"`
	CorpusRawSHA256   string `json:"corpusRawSha256"`
	AuditRawSHA256    string `json:"auditRawSha256"`
	ManifestRawSHA256 string `json:"manifestRawSha256"`
}

type RunProfileManifest struct {
	Role                string `json:"role"`
	ProfileID           string `json:"profileId"`
	ConfigurationSHA256 string `json:"configurationSha256"`
	Passed              bool   `json:"passed"`
}

type RunArtifactManifest struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

type RelevanceRunManifest struct {
	SchemaVersion       string                `json:"schemaVersion"`
	RunID               string                `json:"runId"`
	CaptureID           string                `json:"captureId"`
	CorpusClass         string                `json:"corpusClass"`
	AdmissionMode       string                `json:"admissionMode"`
	PromotionEligible   bool                  `json:"promotionEligible"`
	CaptureMode         string                `json:"captureMode"`
	Split               string                `json:"split"`
	ProviderMode        string                `json:"providerMode"`
	ProfileID           string                `json:"profileId"`
	PolicyID            string                `json:"policyId"`
	ConfigurationSHA256 string                `json:"configurationSha256"`
	Passed              bool                  `json:"passed"`
	StartedAt           string                `json:"startedAt"`
	CompletedAt         string                `json:"completedAt"`
	CostBasisSHA256     string                `json:"costBasisSha256"`
	ProviderCostPolicy  string                `json:"providerCostPolicy,omitempty"`
	Inputs              RunInputHashes        `json:"inputs"`
	Artifacts           []RunArtifactManifest `json:"artifacts"`
}

func BuildRunManifest(
	runID string,
	captureID string,
	providerMode string,
	startedAt time.Time,
	completedAt time.Time,
	protected ProtectedRegression,
	costBasisSHA256 string,
	profileHashes ProfileHashes,
	baseline memoryeval.RegressionReport,
	candidate memoryeval.RegressionReport,
	artifacts []Artifact,
) (RunManifest, []byte, error) {
	if !runIDPattern.MatchString(runID) || captureID == "" ||
		startedAt.IsZero() || completedAt.Before(startedAt) ||
		len(costBasisSHA256) != 64 || len(artifacts) == 0 {
		return RunManifest{}, nil, ErrCaptureInvalid
	}
	if _, err := candidateProfileID(providerMode); err != nil {
		return RunManifest{}, nil, err
	}
	profiles := []RunProfileManifest{
		{Role: "baseline", ProfileID: baseline.Profile.ProfileID,
			ConfigurationSHA256: profileHashes.Baseline, Passed: baseline.Passed},
		{Role: "candidate", ProfileID: candidate.Profile.ProfileID,
			ConfigurationSHA256: profileHashes.Candidate, Passed: candidate.Passed},
	}
	if profileHashes.BaselineProfileID != profiles[0].ProfileID ||
		profileHashes.CandidateProfileID != profiles[1].ProfileID ||
		len(profileHashes.Baseline) != 64 || len(profileHashes.Candidate) != 64 {
		return RunManifest{}, nil, ErrCaptureInvalid
	}
	artifactManifest, err := buildRunArtifactManifest(artifacts)
	if err != nil {
		return RunManifest{}, nil, err
	}
	manifest := RunManifest{
		SchemaVersion: RunManifestSchemaVersion, RunID: runID, CaptureID: captureID,
		CorpusClass:   memoryeval.RegressionCorpusClass,
		AdmissionMode: memoryeval.RegressionAdmissionMode, PromotionEligible: false,
		ProviderMode:    providerMode,
		StartedAt:       startedAt.UTC().Format(time.RFC3339),
		CompletedAt:     completedAt.UTC().Format(time.RFC3339),
		CostBasisSHA256: costBasisSHA256,
		Inputs: RunInputHashes{
			FixtureRawSHA256:  protected.FixtureRawSHA256,
			CorpusRawSHA256:   protected.CorpusRawSHA256,
			AuditRawSHA256:    protected.AuditRawSHA256,
			ManifestRawSHA256: protected.ManifestRawSHA256,
		},
		Profiles: profiles, Artifacts: artifactManifest,
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return RunManifest{}, nil, fmt.Errorf("%w: encode run manifest", ErrCaptureInvalid)
	}
	return manifest, append(body, '\n'), nil
}

func BuildCalibrationRunManifest(
	runID string,
	captureID string,
	providerMode string,
	startedAt time.Time,
	completedAt time.Time,
	protected ProtectedRegression,
	costBasisSHA256 string,
	report CalibrationReport,
	artifacts []Artifact,
) (RelevanceRunManifest, []byte, error) {
	if !runIDPattern.MatchString(runID) || captureID == "" ||
		startedAt.IsZero() || completedAt.Before(startedAt) ||
		len(costBasisSHA256) != 64 || len(artifacts) != 1 ||
		report.SchemaVersion != CalibrationReportSchemaVersion ||
		report.CorpusClass != memoryeval.RegressionCorpusClass ||
		report.AdmissionMode != CalibrationAdmissionMode || report.PromotionEligible ||
		report.Split != DevelopmentCalibrationSplit || report.CaseCount != 300 ||
		report.PolicyID != usermemory.HybridRelevanceIntentCalibrationPolicyID ||
		len(report.ConfigurationSHA256) != 64 || report.EvaluatedPairCount != 20301 ||
		report.IntentSelectionAlgorithm != calibrationIntentSelectionAlgorithm ||
		report.IntentEvaluatedThresholdCount != 201 ||
		!validCalibrationDiagnostics(report.Diagnostics, report.CaseCount) ||
		(report.Selected == nil && report.FeasiblePairCount != 0) ||
		(report.Selected != nil && (report.FeasiblePairCount == 0 ||
			!report.Selected.Evaluation.Passed || !report.ProviderCostPassed)) ||
		(report.IntentSelected == nil && report.IntentFeasibleThresholdCount != 0) ||
		(report.IntentSelected != nil && (report.IntentFeasibleThresholdCount == 0 ||
			report.IntentSelected.MinimumMemoryIntentMarginBasisPoints < calibrationMinimumAdmissionBP ||
			report.IntentSelected.MinimumMemoryIntentMarginBasisPoints > calibrationMaximumAdmissionBP ||
			!report.IntentSelected.Evaluation.Passed || !report.ProviderCostPassed)) {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	if _, err := hex.DecodeString(report.ConfigurationSHA256); err != nil {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	expectedProfileID, err := candidateProfileID(providerMode)
	if err != nil {
		return RelevanceRunManifest{}, nil, err
	}
	if report.ProfileID != expectedProfileID {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	artifactManifest, err := buildRunArtifactManifest(artifacts)
	if err != nil || artifactManifest[0].Name != "relevance-calibration.json" {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	manifest := RelevanceRunManifest{
		SchemaVersion: RelevanceRunManifestSchemaVersion,
		RunID:         runID, CaptureID: captureID,
		CorpusClass:   memoryeval.RegressionCorpusClass,
		AdmissionMode: CalibrationAdmissionMode, PromotionEligible: false,
		CaptureMode: CaptureModeCalibration, Split: DevelopmentCalibrationSplit,
		ProviderMode: providerMode, ProfileID: report.ProfileID,
		PolicyID: report.PolicyID, ConfigurationSHA256: report.ConfigurationSHA256,
		Passed:          report.Selected != nil || report.IntentSelected != nil,
		StartedAt:       startedAt.UTC().Format(time.RFC3339),
		CompletedAt:     completedAt.UTC().Format(time.RFC3339),
		CostBasisSHA256: costBasisSHA256,
		Inputs: RunInputHashes{
			FixtureRawSHA256:  protected.FixtureRawSHA256,
			CorpusRawSHA256:   protected.CorpusRawSHA256,
			AuditRawSHA256:    protected.AuditRawSHA256,
			ManifestRawSHA256: protected.ManifestRawSHA256,
		},
		Artifacts: artifactManifest,
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return RelevanceRunManifest{}, nil, fmt.Errorf("%w: encode relevance run manifest", ErrCaptureInvalid)
	}
	return manifest, append(body, '\n'), nil
}

func BuildCloudJudgeDevelopmentRunManifest(
	runID string,
	captureID string,
	providerMode string,
	startedAt time.Time,
	completedAt time.Time,
	protected ProtectedRegression,
	costBasisSHA256 string,
	report CloudJudgeDevelopmentReport,
	artifacts []Artifact,
) (RelevanceRunManifest, []byte, error) {
	if !runIDPattern.MatchString(runID) || captureID == "" ||
		startedAt.IsZero() || completedAt.Before(startedAt) ||
		len(costBasisSHA256) != 64 || len(artifacts) != 1 ||
		!validCloudJudgeDevelopmentCostPolicy(report) ||
		report.CorpusClass != memoryeval.RegressionCorpusClass ||
		report.AdmissionMode != CloudJudgeDevelopmentAdmissionMode ||
		report.PromotionEligible || report.Split != DevelopmentCalibrationSplit ||
		report.CaseCount != 300 ||
		report.PolicyID != usermemory.HybridRelevanceCloudJudgeCalibrationPolicyID ||
		report.ProviderEgressPolicy !=
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 ||
		report.JudgePromptVersion != usermemory.HybridCandidateJudgePromptVersion ||
		report.JudgePromptSHA256 != usermemory.HybridCandidateJudgePromptSHA256 ||
		report.JudgeDecodingProfile != usermemory.HybridCandidateJudgeDecodingProfile ||
		report.SelectionAlgorithm != cloudJudgeSelectionAlgorithm ||
		report.Passed != report.Evaluation.Passed ||
		len(report.ConfigurationSHA256) != 64 ||
		report.Diagnostics.FailureCodeCounts == nil ||
		report.Diagnostics.EmptyCandidateCaseCount < 0 ||
		report.Diagnostics.JudgeCompletedCaseCount < 0 ||
		report.Diagnostics.JudgeAbstainedCaseCount < 0 ||
		report.Diagnostics.FailedCaseCount < 0 ||
		report.Diagnostics.EmptyCandidateCaseCount+
			report.Diagnostics.JudgeCompletedCaseCount+
			report.Diagnostics.FailedCaseCount != report.CaseCount ||
		report.CostAuthority.AuthorizedRequestCount != 300 ||
		report.CostAuthority.ActualRequestCount < 0 ||
		report.CostAuthority.ActualRequestCount >
			report.CostAuthority.AuthorizedRequestCount ||
		report.CostAuthority.ActualInputTokenUpperBound >
			report.CostAuthority.AuthorizedMaximumInputTokens ||
		report.CostAuthority.ActualOutputTokenUpperBound !=
			uint64(report.CostAuthority.ActualRequestCount)*
				usermemory.HybridCandidateJudgeMaximumOutputTokens ||
		report.CostAuthority.ActualOutputTokenUpperBound >
			report.CostAuthority.AuthorizedMaximumOutputTokens {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	if _, err := hex.DecodeString(report.ConfigurationSHA256); err != nil {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	expectedProfileID, err := candidateProfileID(providerMode)
	if err != nil {
		return RelevanceRunManifest{}, nil, err
	}
	if report.ProfileID != expectedProfileID {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	artifactManifest, err := buildRunArtifactManifest(artifacts)
	if err != nil || artifactManifest[0].Name != "cloud-judge-development.json" {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	manifest := RelevanceRunManifest{
		SchemaVersion: RelevanceRunManifestSchemaVersion,
		RunID:         runID, CaptureID: captureID,
		CorpusClass:         memoryeval.RegressionCorpusClass,
		AdmissionMode:       CloudJudgeDevelopmentAdmissionMode,
		PromotionEligible:   false,
		CaptureMode:         CaptureModeCloudJudgeDevelopment,
		Split:               DevelopmentCalibrationSplit,
		ProviderMode:        providerMode,
		ProfileID:           report.ProfileID,
		PolicyID:            report.PolicyID,
		ConfigurationSHA256: report.ConfigurationSHA256,
		Passed:              report.Passed,
		StartedAt:           startedAt.UTC().Format(time.RFC3339),
		CompletedAt:         completedAt.UTC().Format(time.RFC3339),
		CostBasisSHA256:     costBasisSHA256,
		ProviderCostPolicy:  report.ProviderCostPolicy,
		Inputs: RunInputHashes{
			FixtureRawSHA256:  protected.FixtureRawSHA256,
			CorpusRawSHA256:   protected.CorpusRawSHA256,
			AuditRawSHA256:    protected.AuditRawSHA256,
			ManifestRawSHA256: protected.ManifestRawSHA256,
		},
		Artifacts: artifactManifest,
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return RelevanceRunManifest{}, nil, fmt.Errorf(
			"%w: encode cloud-judge run manifest",
			ErrCaptureInvalid,
		)
	}
	return manifest, append(body, '\n'), nil
}

func BuildMemoryToolRouteDevelopmentRunManifest(
	runID string,
	captureID string,
	providerMode string,
	startedAt time.Time,
	completedAt time.Time,
	protected ProtectedRegression,
	costBasisSHA256 string,
	report MemoryToolRouteDevelopmentReport,
	artifacts []Artifact,
) (RelevanceRunManifest, []byte, error) {
	return buildMemoryToolRouteRunManifest(
		runID,
		captureID,
		providerMode,
		startedAt,
		completedAt,
		protected,
		costBasisSHA256,
		report,
		artifacts,
		MemoryToolFirstRoundDevelopmentReportSchemaVersion,
		MemoryToolFirstRoundDevelopmentAdmissionMode,
		CaptureModeMemoryToolRouteDevelopment,
		"memory-first-tool-round-development.json",
		"",
	)
}

func BuildMemoryToolRouteDiagnosticRunManifest(
	runID string,
	captureID string,
	providerMode string,
	startedAt time.Time,
	completedAt time.Time,
	protected ProtectedRegression,
	costBasisSHA256 string,
	report MemoryToolRouteDevelopmentReport,
	artifacts []Artifact,
) (RelevanceRunManifest, []byte, error) {
	return buildMemoryToolRouteRunManifest(
		runID,
		captureID,
		providerMode,
		startedAt,
		completedAt,
		protected,
		costBasisSHA256,
		report,
		artifacts,
		MemoryToolFirstRoundDiagnosticReportSchemaVersion,
		MemoryToolFirstRoundDiagnosticAdmissionMode,
		CaptureModeMemoryToolRouteDiagnostic,
		"memory-first-tool-round-diagnostic-development.json",
		MemoryToolRouteFailureTaxonomyVersion,
	)
}

func buildMemoryToolRouteRunManifest(
	runID string,
	captureID string,
	providerMode string,
	startedAt time.Time,
	completedAt time.Time,
	protected ProtectedRegression,
	costBasisSHA256 string,
	report MemoryToolRouteDevelopmentReport,
	artifacts []Artifact,
	reportSchemaVersion string,
	admissionMode string,
	captureMode string,
	artifactName string,
	failureTaxonomyVersion string,
) (RelevanceRunManifest, []byte, error) {
	if !runIDPattern.MatchString(runID) || captureID == "" ||
		startedAt.IsZero() || completedAt.Before(startedAt) ||
		len(costBasisSHA256) != 64 || len(artifacts) != 1 ||
		report.SchemaVersion != reportSchemaVersion ||
		report.CorpusClass != memoryeval.RegressionCorpusClass ||
		report.AdmissionMode != admissionMode ||
		report.PromotionEligible || report.Split != DevelopmentCalibrationSplit ||
		report.CaseCount != 300 ||
		report.PolicyID != usermemory.HybridRelevanceMemoryFirstToolRoundPolicyID ||
		report.ProviderEgressPolicy !=
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 ||
		report.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
		!report.ProviderCostAuthorized ||
		report.ToolName != usermemory.HybridMemoryToolName ||
		report.ToolContractVersion != usermemory.HybridMemoryToolContractVersion ||
		report.ToolContractSHA256 != usermemory.HybridMemoryToolContractSHA256 ||
		report.ToolAdapterVersion != chat.MemoryToolFirstRoundAdapterVersion ||
		report.ToolDecodingProfile != "" || report.ToolMaximumOutputTokens != 0 ||
		report.ToolTemperature != 0 || report.ToolDisableThinking ||
		report.FailureTaxonomyVersion != failureTaxonomyVersion ||
		(failureTaxonomyVersion != "" &&
			report.FailureTaxonomySHA256 != MemoryToolRouteFailureTaxonomySHA256) ||
		(failureTaxonomyVersion == "" && report.FailureTaxonomySHA256 != "") ||
		report.SelectionAlgorithm != memoryToolFirstRoundSelectionAlgorithm ||
		report.Passed != report.Evaluation.Passed ||
		len(report.ConfigurationSHA256) != 64 ||
		report.Diagnostics.FailureCodeCounts == nil ||
		(failureTaxonomyVersion != "" &&
			report.Diagnostics.RouteFailureCategoryCounts == nil) ||
		report.Diagnostics.EmptyCandidateCaseCount < 0 ||
		report.Diagnostics.RouteCompletedCaseCount < 0 ||
		report.Diagnostics.RouteUsedCaseCount < 0 ||
		report.Diagnostics.RouteAbstainedCaseCount < 0 ||
		report.Diagnostics.FailedCaseCount < 0 ||
		report.Diagnostics.RouteUsedCaseCount+
			report.Diagnostics.RouteAbstainedCaseCount !=
			report.Diagnostics.RouteCompletedCaseCount ||
		report.Diagnostics.RouteCompletedCaseCount+
			report.Diagnostics.FailedCaseCount != report.CaseCount ||
		(failureTaxonomyVersion != "" &&
			sumDiagnosticCounts(report.Diagnostics.RouteFailureCategoryCounts) !=
				report.Diagnostics.FailedCaseCount) ||
		(failureTaxonomyVersion != "" &&
			!validMemoryToolRouteFailureCounts(
				report.Diagnostics.RouteFailureCategoryCounts,
			)) ||
		report.CostAuthority.AuthorizedRequestCount != 300 ||
		report.CostAuthority.ActualRequestCount < 0 ||
		report.CostAuthority.ActualRequestCount >
			report.CostAuthority.AuthorizedRequestCount ||
		report.CostAuthority.ActualInputTokenUpperBound >
			report.CostAuthority.AuthorizedMaximumInputTokens ||
		report.CostAuthority.ActualOutputTokenUpperBound == 0 ||
		report.CostAuthority.ActualOutputTokenUpperBound >
			report.CostAuthority.AuthorizedMaximumOutputTokens ||
		report.CostAuthority.MaximumMemoryProviderCostMicrounits <
			report.CostAuthority.MaximumRouteCostMicrounits {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	if _, err := hex.DecodeString(report.ConfigurationSHA256); err != nil {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	expectedProfileID, err := candidateProfileID(providerMode)
	if err != nil {
		return RelevanceRunManifest{}, nil, err
	}
	if report.ProfileID != expectedProfileID {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	artifactManifest, err := buildRunArtifactManifest(artifacts)
	if err != nil || artifactManifest[0].Name != artifactName {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	manifest := RelevanceRunManifest{
		SchemaVersion: RelevanceRunManifestSchemaVersion,
		RunID:         runID, CaptureID: captureID,
		CorpusClass:         memoryeval.RegressionCorpusClass,
		AdmissionMode:       admissionMode,
		PromotionEligible:   false,
		CaptureMode:         captureMode,
		Split:               DevelopmentCalibrationSplit,
		ProviderMode:        providerMode,
		ProfileID:           report.ProfileID,
		PolicyID:            report.PolicyID,
		ConfigurationSHA256: report.ConfigurationSHA256,
		Passed:              report.Passed,
		StartedAt:           startedAt.UTC().Format(time.RFC3339),
		CompletedAt:         completedAt.UTC().Format(time.RFC3339),
		CostBasisSHA256:     costBasisSHA256,
		ProviderCostPolicy:  report.ProviderCostPolicy,
		Inputs: RunInputHashes{
			FixtureRawSHA256:  protected.FixtureRawSHA256,
			CorpusRawSHA256:   protected.CorpusRawSHA256,
			AuditRawSHA256:    protected.AuditRawSHA256,
			ManifestRawSHA256: protected.ManifestRawSHA256,
		},
		Artifacts: artifactManifest,
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return RelevanceRunManifest{}, nil, fmt.Errorf(
			"%w: encode Memory Tool route run manifest",
			ErrCaptureInvalid,
		)
	}
	return manifest, append(body, '\n'), nil
}

func validMemoryToolRouteFailureCounts(counts map[string]int) bool {
	for category, count := range counts {
		if count < 0 ||
			!usermemory.ValidHybridMemoryToolRouteFailureCategory(category) {
			return false
		}
	}
	return true
}

func validCloudJudgeDevelopmentCostPolicy(report CloudJudgeDevelopmentReport) bool {
	switch report.SchemaVersion {
	case cloudJudgeDevelopmentLegacyReportSchemaVersion:
		return report.ProviderCostPolicy == "" &&
			!report.ProviderCostAuthorized &&
			report.Evaluation.ProviderCostPassed != nil &&
			report.CostAuthority.Unit == "" &&
			report.CostAuthority.MaximumMemoryProviderCostMicrounits == 0
	case CloudJudgeDevelopmentReportSchemaVersion:
		return report.ProviderCostPolicy == ProviderCostPolicyOwnerAuthorizedAbsoluteV1 &&
			report.ProviderCostAuthorized &&
			report.Evaluation.ProviderCostPassed == nil &&
			report.CostAuthority.Unit != "" &&
			report.CostAuthority.MaximumMemoryProviderCostMicrounits >=
				report.CostAuthority.MaximumJudgeCostMicrounits
	default:
		return false
	}
}

func validCalibrationDiagnostics(value CalibrationDiagnostics, caseCount int) bool {
	if value.Version != calibrationDiagnosticsVersion || value.OtherCaseCount < 0 ||
		value.FailurePairCounts == nil || value.IntentFailureThresholdCounts == nil ||
		value.BestSafetyAttempt == nil || value.BestRecallAttempt == nil ||
		value.BestIntentSafetyAttempt == nil || value.BestIntentRecallAttempt == nil ||
		value.BestSafetyAttempt.ProviderSimilarityBasisPoints < calibrationMinimumAdmissionBP ||
		value.BestSafetyAttempt.ProviderSimilarityBasisPoints > calibrationMaximumAdmissionBP ||
		value.BestSafetyAttempt.FinalRelevanceBasisPoints < calibrationMinimumFinalBP ||
		value.BestSafetyAttempt.FinalRelevanceBasisPoints > calibrationMaximumFinalBP ||
		value.BestRecallAttempt.ProviderSimilarityBasisPoints < calibrationMinimumAdmissionBP ||
		value.BestRecallAttempt.ProviderSimilarityBasisPoints > calibrationMaximumAdmissionBP ||
		value.BestRecallAttempt.FinalRelevanceBasisPoints < calibrationMinimumFinalBP ||
		value.BestRecallAttempt.FinalRelevanceBasisPoints > calibrationMaximumFinalBP ||
		value.BestIntentSafetyAttempt.MinimumMemoryIntentMarginBasisPoints < calibrationMinimumAdmissionBP ||
		value.BestIntentSafetyAttempt.MinimumMemoryIntentMarginBasisPoints > calibrationMaximumAdmissionBP ||
		value.BestIntentRecallAttempt.MinimumMemoryIntentMarginBasisPoints < calibrationMinimumAdmissionBP ||
		value.BestIntentRecallAttempt.MinimumMemoryIntentMarginBasisPoints > calibrationMaximumAdmissionBP {
		return false
	}
	for name, count := range value.FailurePairCounts {
		if name == "" || count < 0 {
			return false
		}
	}
	for name, count := range value.IntentFailureThresholdCounts {
		if name == "" || count < 0 {
			return false
		}
	}
	curves := []struct {
		value            CalibrationThresholdCurve
		minimum, maximum int
	}{
		{value.MemoryIntentMarginCurve, calibrationMinimumAdmissionBP, calibrationMaximumAdmissionBP},
		{value.AdmissionSimilarityCurve, calibrationMinimumAdmissionBP, calibrationMaximumAdmissionBP},
		{value.MaximumRerankScoreCurve, calibrationMinimumFinalBP, calibrationMaximumFinalBP},
		{value.TopTwoRerankMarginCurve, calibrationMinimumFinalBP, calibrationMaximumFinalBP},
	}
	for _, curve := range curves {
		if !validCalibrationThresholdCurve(curve.value, curve.minimum, curve.maximum) ||
			value.OtherCaseCount+
				curve.value.RelevantEligibleCaseCount+
				curve.value.RelevantMissingCaseCount+
				curve.value.UnrelatedNegativeEligibleCaseCount+
				curve.value.UnrelatedNegativeMissingCaseCount != caseCount {
			return false
		}
	}
	return true
}

func validCalibrationThresholdCurve(
	value CalibrationThresholdCurve,
	minimum int,
	maximum int,
) bool {
	expectedCount := (maximum-minimum)/calibrationStepBP + 1
	if value.MinimumBasisPoints != minimum || value.MaximumBasisPoints != maximum ||
		value.StepBasisPoints != calibrationStepBP ||
		value.RelevantEligibleCaseCount < 0 || value.RelevantMissingCaseCount < 0 ||
		value.UnrelatedNegativeEligibleCaseCount < 0 ||
		value.UnrelatedNegativeMissingCaseCount < 0 ||
		len(value.RelevantPassingCaseCounts) != expectedCount ||
		len(value.UnrelatedNegativePassingCaseCounts) != expectedCount ||
		value.RelevantPassingCaseCounts[0] != value.RelevantEligibleCaseCount ||
		value.UnrelatedNegativePassingCaseCounts[0] !=
			value.UnrelatedNegativeEligibleCaseCount {
		return false
	}
	return validCumulativePassingCounts(
		value.RelevantPassingCaseCounts,
		value.RelevantEligibleCaseCount,
	) && validCumulativePassingCounts(
		value.UnrelatedNegativePassingCaseCounts,
		value.UnrelatedNegativeEligibleCaseCount,
	)
}

func validCumulativePassingCounts(values []int, eligible int) bool {
	previous := eligible
	for _, value := range values {
		if value < 0 || value > previous {
			return false
		}
		previous = value
	}
	return true
}

func BuildValidationRunManifest(
	runID string,
	captureID string,
	providerMode string,
	startedAt time.Time,
	completedAt time.Time,
	protected ProtectedRegression,
	costBasisSHA256 string,
	report ValidationReport,
	artifacts []Artifact,
) (RelevanceRunManifest, []byte, error) {
	if !runIDPattern.MatchString(runID) || captureID == "" ||
		startedAt.IsZero() || completedAt.Before(startedAt) ||
		len(costBasisSHA256) != 64 || len(artifacts) != 1 ||
		report.SchemaVersion != ValidationReportSchemaVersion ||
		report.CorpusClass != memoryeval.RegressionCorpusClass ||
		report.AdmissionMode != ValidationAdmissionMode || report.PromotionEligible ||
		report.Split != FrozenValidationSplit || report.CaseCount != 100 ||
		report.PolicyID != usermemory.HybridRelevanceFrozenPolicyID ||
		len(report.ConfigurationSHA256) != 64 ||
		report.Passed != report.Evaluation.Passed ||
		(report.Passed && !report.Evaluation.ProviderCostPassed) {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	if _, err := hex.DecodeString(report.ConfigurationSHA256); err != nil {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	expectedProfileID, err := candidateProfileID(providerMode)
	if err != nil {
		return RelevanceRunManifest{}, nil, err
	}
	if report.ProfileID != expectedProfileID {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	artifactManifest, err := buildRunArtifactManifest(artifacts)
	if err != nil || artifactManifest[0].Name != "relevance-validation.json" {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	manifest := RelevanceRunManifest{
		SchemaVersion: RelevanceRunManifestSchemaVersion,
		RunID:         runID, CaptureID: captureID,
		CorpusClass:   memoryeval.RegressionCorpusClass,
		AdmissionMode: ValidationAdmissionMode, PromotionEligible: false,
		CaptureMode: CaptureModeFrozenValidation, Split: FrozenValidationSplit,
		ProviderMode: providerMode, ProfileID: report.ProfileID,
		PolicyID: report.PolicyID, ConfigurationSHA256: report.ConfigurationSHA256,
		Passed:          report.Passed,
		StartedAt:       startedAt.UTC().Format(time.RFC3339),
		CompletedAt:     completedAt.UTC().Format(time.RFC3339),
		CostBasisSHA256: costBasisSHA256,
		Inputs: RunInputHashes{
			FixtureRawSHA256:  protected.FixtureRawSHA256,
			CorpusRawSHA256:   protected.CorpusRawSHA256,
			AuditRawSHA256:    protected.AuditRawSHA256,
			ManifestRawSHA256: protected.ManifestRawSHA256,
		},
		Artifacts: artifactManifest,
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return RelevanceRunManifest{}, nil, fmt.Errorf("%w: encode validation run manifest", ErrCaptureInvalid)
	}
	return manifest, append(body, '\n'), nil
}

func buildRunArtifactManifest(artifacts []Artifact) ([]RunArtifactManifest, error) {
	if len(artifacts) == 0 {
		return nil, ErrCaptureInvalid
	}
	artifactManifest := make([]RunArtifactManifest, len(artifacts))
	seen := make(map[string]struct{}, len(artifacts))
	for index, artifact := range artifacts {
		if !validArtifactName(artifact.Name) || len(artifact.Body) == 0 {
			return nil, ErrCaptureInvalid
		}
		if _, duplicate := seen[artifact.Name]; duplicate {
			return nil, ErrCaptureInvalid
		}
		seen[artifact.Name] = struct{}{}
		digest := sha256.Sum256(artifact.Body)
		artifactManifest[index] = RunArtifactManifest{
			Name: artifact.Name, SHA256: hex.EncodeToString(digest[:]), Bytes: len(artifact.Body),
		}
	}
	sort.Slice(artifactManifest, func(i, j int) bool {
		return artifactManifest[i].Name < artifactManifest[j].Name
	})
	return artifactManifest, nil
}

// VerifyRetainedArtifactsLeakFree rejects exact synthetic queries, Memory
// plaintext, and supplied live credential bytes in the retained content-free
// bundle. Errors never echo the matched value.
func VerifyRetainedArtifactsLeakFree(
	pool memoryauthor.RegressionPool,
	artifacts []Artifact,
	credential []byte,
) error {
	forbidden := make([][]byte, 0, len(pool.Corpus.Cases)+len(pool.Fixtures.Fixtures)*2+1)
	for _, item := range pool.Corpus.Cases {
		forbidden = appendForbiddenRepresentations(forbidden, item.Query)
	}
	for _, fixture := range pool.Fixtures.Fixtures {
		for _, memory := range fixture.Memories {
			forbidden = appendForbiddenRepresentations(forbidden, memory.CanonicalContent)
		}
	}
	if len(credential) > 0 {
		forbidden = append(forbidden, append([]byte(nil), credential...))
	}
	for _, artifact := range artifacts {
		for _, value := range forbidden {
			if len(value) >= 8 && bytes.Contains(artifact.Body, value) {
				return fmt.Errorf("%w: retained artifact contains protected plaintext", ErrCaptureStateConflict)
			}
		}
	}
	return nil
}

func appendForbiddenRepresentations(values [][]byte, text string) [][]byte {
	text = strings.TrimSpace(text)
	if len([]byte(text)) < 8 {
		return values
	}
	values = append(values, []byte(text))
	encoded, err := json.Marshal(text)
	if err == nil && len(encoded) >= 2 {
		values = append(values, encoded[1:len(encoded)-1])
	}
	return values
}
