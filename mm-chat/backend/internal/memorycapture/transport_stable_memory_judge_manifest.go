package memorycapture

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

func BuildTransportStableMemoryJudgeRunManifest(
	runID string,
	captureID string,
	providerMode string,
	startedAt time.Time,
	completedAt time.Time,
	protected ProtectedRegression,
	costBasisSHA256 string,
	report TransportStableMemoryJudgeDevelopmentReport,
	artifacts []Artifact,
) (RelevanceRunManifest, []byte, error) {
	if !validTransportStableMemoryJudgeDevelopmentReport(report) ||
		!runIDPattern.MatchString(runID) || captureID == "" ||
		startedAt.IsZero() || completedAt.Before(startedAt) ||
		len(costBasisSHA256) != 64 || len(artifacts) != 1 {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	if _, err := hex.DecodeString(costBasisSHA256); err != nil {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	expectedPolicy, err := TransportStableDevelopmentExecutionPolicy(providerMode)
	if err != nil || report.ExecutionPolicy != expectedPolicy {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	expectedProfileID, err := candidateProfileID(providerMode)
	if err != nil || report.ProfileID != expectedProfileID {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	artifactManifest, err := buildRunArtifactManifest(artifacts)
	if err != nil || artifactManifest[0].Name != TransportStableMemoryJudgeArtifactName {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	manifest := RelevanceRunManifest{
		SchemaVersion:       RelevanceRunManifestSchemaVersion,
		RunID:               runID,
		CaptureID:           captureID,
		CorpusClass:         memoryeval.RegressionCorpusClass,
		AdmissionMode:       TransportStableMemoryJudgeAdmissionMode,
		PromotionEligible:   false,
		CaptureMode:         CaptureModeTransportStableMemoryJudge,
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
		return RelevanceRunManifest{}, nil, errors.Join(ErrCaptureInvalid, err)
	}
	return manifest, append(body, '\n'), nil
}
