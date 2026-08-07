package memorycapture

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	MemoryJudgeSliceDiagnosticReportSchemaVersion  = "neo-chat.memory-regression-slice-diagnostic.v19"
	MemoryJudgeSliceDiagnosticRunSchemaVersion     = "neo-chat.memory-regression-slice-diagnostic-run.v19"
	MemoryJudgeSliceDiagnosticAdmissionMode        = "development_slice_union_diagnostic_only"
	MemoryJudgeSliceDiagnosticArtifactName         = "memory-judge-slice-diagnostic-development.json"
	MemoryJudgeSliceDiagnosticRepetitions          = 3
	MemoryJudgeSliceDiagnosticCaseCount            = 85
	MemoryJudgeSliceDiagnosticExecutionCount       = 255
	MemoryJudgeSliceDiagnosticMaximumJudgeAttempts = 765
)

const (
	SliceDiagnosticCauseCandidateMissing  = "candidate_missing"
	SliceDiagnosticCauseRerankFailed      = "rerank_failed"
	SliceDiagnosticCauseLunaNotSelected   = "luna_not_selected"
	SliceDiagnosticCauseFinalRankOrBudget = "final_rank_or_budget"
	SliceDiagnosticCauseNone              = "none"
	SliceDiagnosticCauseNotApplicable     = "not_applicable"
	SliceDiagnosticCauseTerminalFailure   = "terminal_failure"
)

type MemoryJudgeSliceDiagnosticCase struct {
	CaseID                string `json:"caseId"`
	Repetition            int    `json:"repetition"`
	MixedLanguageEntity   bool   `json:"mixedLanguageEntity"`
	StableFact            bool   `json:"stableFact"`
	Intersection          bool   `json:"intersection"`
	ExpectedCurrentCount  int    `json:"expectedCurrentCount"`
	CandidateCurrentCount int    `json:"candidateCurrentCount"`
	RerankCurrentCount    int    `json:"rerankCurrentCount"`
	RerankCurrentRank     int    `json:"rerankCurrentRank"`
	JudgeCurrentCount     int    `json:"judgeCurrentCount"`
	FinalCurrentCount     int    `json:"finalCurrentCount"`
	RootCause             string `json:"rootCause"`
}

type MemoryJudgeSliceDiagnosticReport struct {
	SchemaVersion                   string                                  `json:"schemaVersion"`
	CorpusClass                     string                                  `json:"corpusClass"`
	AdmissionMode                   string                                  `json:"admissionMode"`
	PromotionEligible               bool                                    `json:"promotionEligible"`
	ReleaseEligible                 bool                                    `json:"releaseEligible"`
	PolicySelected                  bool                                    `json:"policySelected"`
	ExecutionComplete               bool                                    `json:"executionComplete"`
	EvidenceClass                   string                                  `json:"evidenceClass"`
	Split                           string                                  `json:"split"`
	CaseCount                       int                                     `json:"caseCount"`
	Repetitions                     int                                     `json:"repetitions"`
	ExecutionCount                  int                                     `json:"executionCount"`
	SliceUnion                      []string                                `json:"sliceUnion"`
	IntersectionCaseCount           int                                     `json:"intersectionCaseCount"`
	DiagnosticCaseOrderSHA256       string                                  `json:"diagnosticCaseOrderSha256"`
	PolicyID                        string                                  `json:"policyId"`
	ProfileID                       string                                  `json:"profileId"`
	ConfigurationSHA256             string                                  `json:"configurationSha256"`
	RelevancePolicyDescriptorSHA256 string                                  `json:"relevancePolicyDescriptorSha256"`
	ProviderEgressPolicy            string                                  `json:"providerEgressPolicy"`
	ProviderCostPolicy              string                                  `json:"providerCostPolicy"`
	ProviderCostAuthorized          bool                                    `json:"providerCostAuthorized"`
	JudgeProviderID                 string                                  `json:"judgeProviderId"`
	JudgeProviderType               string                                  `json:"judgeProviderType"`
	JudgeBaseURLSHA256              string                                  `json:"judgeBaseUrlSha256"`
	JudgeModelID                    string                                  `json:"judgeModelId"`
	JudgeAdapter                    string                                  `json:"judgeAdapter"`
	JudgePromptVersion              string                                  `json:"judgePromptVersion"`
	JudgePromptSHA256               string                                  `json:"judgePromptSha256"`
	JudgeDecodingProfile            string                                  `json:"judgeDecodingProfile"`
	ExecutionPolicy                 AccuracyFirstExecutionPolicy            `json:"executionPolicy"`
	Classification                  string                                  `json:"classification"`
	RootCauseCounts                 map[string]int                          `json:"rootCauseCounts"`
	Cases                           []MemoryJudgeSliceDiagnosticCase        `json:"cases"`
	ProviderAttempts                JudgeFailureDiagnosticProviderTelemetry `json:"providerAttempts"`
	CostAuthority                   CloudJudgeDevelopmentCostAuthority      `json:"costAuthority"`
}

type MemoryJudgeSliceDiagnosticRunManifest struct {
	SchemaVersion             string                `json:"schemaVersion"`
	RunID                     string                `json:"runId"`
	CaptureID                 string                `json:"captureId"`
	CorpusClass               string                `json:"corpusClass"`
	AdmissionMode             string                `json:"admissionMode"`
	PromotionEligible         bool                  `json:"promotionEligible"`
	ReleaseEligible           bool                  `json:"releaseEligible"`
	PolicySelected            bool                  `json:"policySelected"`
	CaptureMode               string                `json:"captureMode"`
	Split                     string                `json:"split"`
	ProviderMode              string                `json:"providerMode"`
	EvidenceClass             string                `json:"evidenceClass"`
	CaseCount                 int                   `json:"caseCount"`
	Repetitions               int                   `json:"repetitions"`
	ExecutionCount            int                   `json:"executionCount"`
	DiagnosticCaseOrderSHA256 string                `json:"diagnosticCaseOrderSha256"`
	ProfileID                 string                `json:"profileId"`
	PolicyID                  string                `json:"policyId"`
	ConfigurationSHA256       string                `json:"configurationSha256"`
	Classification            string                `json:"classification"`
	RootCauseCounts           map[string]int        `json:"rootCauseCounts"`
	StartedAt                 string                `json:"startedAt"`
	CompletedAt               string                `json:"completedAt"`
	CostBasisSHA256           string                `json:"costBasisSha256"`
	ProviderCostPolicy        string                `json:"providerCostPolicy"`
	Inputs                    RunInputHashes        `json:"inputs"`
	Artifacts                 []RunArtifactManifest `json:"artifacts"`
}

func SelectMemoryJudgeSliceDiagnosticDevelopment(
	pool memoryauthor.RegressionPool,
) (memoryauthor.RegressionPool, error) {
	if len(pool.Corpus.Cases) != 500 || len(pool.Fixtures.Fixtures) != 500 {
		return memoryauthor.RegressionPool{}, fmt.Errorf("%w: diagnostic corpus cardinality", ErrCaptureInvalid)
	}
	selected := pool
	selected.Corpus.Cases = make([]memoryeval.GoldenCase, 0, MemoryJudgeSliceDiagnosticCaseCount)
	aliases := make(map[string]struct{}, MemoryJudgeSliceDiagnosticCaseCount)
	mixedCount, stableCount, intersectionCount := 0, 0, 0
	for _, item := range pool.Corpus.Cases {
		mixed := containsString(item.Slices, "mixed_language_entity")
		stable := containsString(item.Slices, "stable_fact")
		if item.Split != DevelopmentCalibrationSplit || (!mixed && !stable) {
			continue
		}
		if _, duplicate := aliases[item.FixtureAlias]; duplicate {
			return memoryauthor.RegressionPool{}, fmt.Errorf("%w: diagnostic fixture binding", ErrCaptureInvalid)
		}
		aliases[item.FixtureAlias] = struct{}{}
		selected.Corpus.Cases = append(selected.Corpus.Cases, item)
		if mixed {
			mixedCount++
		}
		if stable {
			stableCount++
		}
		if mixed && stable {
			intersectionCount++
		}
	}
	selected.Fixtures.Fixtures = make([]memoryauthor.Fixture, 0, MemoryJudgeSliceDiagnosticCaseCount)
	for _, fixture := range pool.Fixtures.Fixtures {
		if _, ok := aliases[fixture.Alias]; !ok {
			continue
		}
		selected.Fixtures.Fixtures = append(selected.Fixtures.Fixtures, fixture)
		delete(aliases, fixture.Alias)
	}
	if len(selected.Corpus.Cases) != MemoryJudgeSliceDiagnosticCaseCount ||
		len(selected.Fixtures.Fixtures) != MemoryJudgeSliceDiagnosticCaseCount ||
		mixedCount != 60 || stableCount != 30 || intersectionCount != 5 ||
		len(aliases) != 0 {
		return memoryauthor.RegressionPool{}, fmt.Errorf("%w: diagnostic slice cardinality", ErrCaptureInvalid)
	}
	return selected, nil
}

func MemoryJudgeSliceDiagnosticCaseOrderSHA256(
	selected memoryauthor.RegressionPool,
) (string, error) {
	if len(selected.Corpus.Cases) != MemoryJudgeSliceDiagnosticCaseCount {
		return "", ErrCaptureInvalid
	}
	digest := sha256.New()
	for _, item := range selected.Corpus.Cases {
		if item.Split != DevelopmentCalibrationSplit || item.ID == "" ||
			(!containsString(item.Slices, "mixed_language_entity") &&
				!containsString(item.Slices, "stable_fact")) {
			return "", ErrCaptureInvalid
		}
		_, _ = digest.Write([]byte(item.ID))
		_, _ = digest.Write([]byte{'\n'})
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// ExpandMemoryJudgeSliceDiagnosticRuntimeCases inserts two additional message
// pairs only in the ephemeral benchmark database. It returns repetition-major
// order while preserving the original CaseID and query.
func ExpandMemoryJudgeSliceDiagnosticRuntimeCases(
	ctx context.Context,
	seedDB *sql.DB,
	runID string,
	seed SeedResult,
) (SeedResult, error) {
	if len(seed.Cases) != MemoryJudgeSliceDiagnosticCaseCount ||
		seed.RunID != runID || seedDB == nil {
		return SeedResult{}, ErrCaptureInvalid
	}
	if err := verifySeedDatabase(ctx, seedDB, runID); err != nil {
		return SeedResult{}, err
	}
	tx, err := seedDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return SeedResult{}, fmt.Errorf("%w: begin diagnostic message expansion", ErrCaptureUnavailable)
	}
	defer func() { _ = tx.Rollback() }()
	expanded := make([]RuntimeCase, 0, MemoryJudgeSliceDiagnosticExecutionCount)
	expanded = append(expanded, seed.Cases...)
	for repetition := 2; repetition <= MemoryJudgeSliceDiagnosticRepetitions; repetition++ {
		for _, item := range seed.Cases {
			userMessageID := deterministicUUID(
				fmt.Sprintf("slice-diagnostic-user-message-%d", repetition), item.CaseID,
			)
			assistantMessageID := deterministicUUID(
				fmt.Sprintf("slice-diagnostic-assistant-message-%d", repetition), item.CaseID,
			)
			userSequence := repetition*2 - 1
			assistantSequence := repetition * 2
			if _, err := tx.ExecContext(ctx, `
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content,
  completed_at, created_at, updated_at
) VALUES ($1::uuid, $2::uuid, $3::uuid, $4, 'user', 'completed', $5, $6, $6, $6);
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role,
  status, content, created_at, updated_at
) VALUES ($7::uuid, $2::uuid, $3::uuid, $1::uuid, $8, 'assistant',
  'streaming', '', $6, $6);
`, userMessageID, item.ConversationID, item.UserID, userSequence, item.Query,
				regressionSeedTime(), assistantMessageID, assistantSequence); err != nil {
				return SeedResult{}, fmt.Errorf("%w: expand diagnostic messages", ErrCaptureStateConflict)
			}
			expanded = append(expanded, RuntimeCase{
				CaseID: item.CaseID, Query: item.Query, UserID: item.UserID,
				ConversationID: item.ConversationID, AssistantMessageID: assistantMessageID,
			})
		}
	}
	if err := tx.Commit(); err != nil {
		return SeedResult{}, fmt.Errorf("%w: commit diagnostic message expansion", ErrCaptureUnavailable)
	}
	seed.Cases = expanded
	return seed, nil
}

func CaptureMemoryJudgeSliceDiagnostic(
	ctx context.Context,
	seedDB *sql.DB,
	runtimeDB *sql.DB,
	runID string,
	selected memoryauthor.RegressionPool,
	index FixtureIndex,
	seed SeedResult,
	provider usermemory.HybridShadowProvider,
	judge usermemory.HybridCandidateJudge,
	authority ConfiguredCandidateJudgeProfileAuthority,
	profileID string,
	configurationSHA256 string,
	cost memoryeval.ProviderCosts,
) (CapturedProfile, error) {
	if err := validateCaptureDatabases(ctx, seedDB, runtimeDB, runID, seed); err != nil {
		return CapturedProfile{}, err
	}
	if err := validateMemoryJudgeSliceDiagnosticRuntimePlan(selected, seed.Cases); err != nil {
		return CapturedProfile{}, err
	}
	hybrid, hybridOK := provider.(*accuracyFirstHybridProvider)
	candidateJudge, judgeOK := judge.(*accuracyFirstCandidateJudge)
	if !hybridOK || !judgeOK || hybrid.controller == nil ||
		hybrid.controller != candidateJudge.controller ||
		hybrid.controller.maximumJudgeRetries != 2 ||
		!hybrid.controller.judgeFailureDiagnostics ||
		!validFixedMemoryJudgeAuthority(authority) {
		return CapturedProfile{}, ErrCaptureInvalid
	}
	profile, err := captureCandidateProfile(
		ctx, runtimeDB, index, seed.Cases, provider,
		usermemory.HybridShadowSliceDiagnosticPolicy(), profileID,
		configurationSHA256, cost, judge, nil,
	)
	if err != nil {
		return CapturedProfile{}, err
	}
	profile.Profile.ReaderVersion = MemoryJudgeSliceDiagnosticReaderVersion
	return profile, nil
}

func BuildMemoryJudgeSliceDiagnosticReport(
	selected memoryauthor.RegressionPool,
	profile CapturedProfile,
	config ProfileConfig,
	authority ConfiguredCandidateJudgeProfileAuthority,
	costBasis CostBasis,
) (MemoryJudgeSliceDiagnosticReport, []byte, error) {
	orderSHA256, err := MemoryJudgeSliceDiagnosticCaseOrderSHA256(selected)
	if err != nil {
		return MemoryJudgeSliceDiagnosticReport{}, nil, err
	}
	configurationSHA256, err := ConfigurationSHA256(config)
	if err != nil {
		return MemoryJudgeSliceDiagnosticReport{}, nil, err
	}
	providerMode, err := providerModeForProfileID(profile.Profile.ID)
	if err != nil {
		return MemoryJudgeSliceDiagnosticReport{}, nil, err
	}
	executionPolicy, err := MemoryJudgeSliceDiagnosticExecutionPolicy(providerMode)
	if err != nil {
		return MemoryJudgeSliceDiagnosticReport{}, nil, err
	}
	descriptorSHA256, err := relevancePolicyDescriptorSHA256(
		usermemory.HybridShadowSliceDiagnosticPolicy(),
	)
	if err != nil {
		return MemoryJudgeSliceDiagnosticReport{}, nil, err
	}
	if profile.Profile.ReaderVersion != MemoryJudgeSliceDiagnosticReaderVersion ||
		profile.Profile.ConfigurationSHA256 != configurationSHA256 ||
		profile.Profile.ProviderEgressPolicy != memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 ||
		profile.Costs != costBasis.Candidate ||
		config.SchemaVersion != "neo-chat.memory-regression-profile-config.v19" ||
		config.CaptureMode != CaptureModeMemoryJudgeSliceDiagnostic ||
		config.EvaluationSplit != DevelopmentCalibrationSplit ||
		config.RelevancePolicyID != usermemory.HybridRelevanceSliceDiagnosticPolicyID ||
		config.DiagnosticCaseOrderSHA256 != orderSHA256 ||
		config.DiagnosticRepetitions != MemoryJudgeSliceDiagnosticRepetitions ||
		len(config.DiagnosticSliceUnion) != 2 ||
		config.DiagnosticSliceUnion[0] != "mixed_language_entity" ||
		config.DiagnosticSliceUnion[1] != "stable_fact" ||
		config.AccuracyFirstExecutionPolicy == nil ||
		*config.AccuracyFirstExecutionPolicy != executionPolicy ||
		config.ConfiguredCandidateJudgeAdapter != memoryjudge.BufferedChatAdapterVersion ||
		config.RelevancePolicyDescriptorSHA256 != descriptorSHA256 ||
		ValidateMemoryJudgeSliceDiagnosticCostAuthority(costBasis, authority) != nil {
		return MemoryJudgeSliceDiagnosticReport{}, nil, ErrCaptureInvalid
	}
	entries, counts, classification, logicalRequests, logicalInputTokens, terminalAttempts, err :=
		classifyMemoryJudgeSliceDiagnostic(selected, profile)
	if err != nil {
		return MemoryJudgeSliceDiagnosticReport{}, nil, err
	}
	telemetry := profile.ProviderAttempts
	if err := validateTransportStableMemoryJudgeTelemetry(
		telemetry, MemoryJudgeSliceDiagnosticExecutionCount, logicalRequests, terminalAttempts,
	); err != nil {
		return MemoryJudgeSliceDiagnosticReport{}, nil, err
	}
	if uint64(telemetry.JudgeInputTokenUpperBound) !=
		logicalInputTokens+uint64(telemetry.JudgeRetryInputTokenUpperBound) {
		return MemoryJudgeSliceDiagnosticReport{}, nil, ErrCaptureInvalid
	}
	costAuthority := costBasis.ConfiguredCandidateJudgeAuthority
	actualOutputTokens := uint64(telemetry.JudgeAttempts) *
		usermemory.HybridCandidateJudgeMaximumOutputTokens
	if telemetry.JudgeAttempts > costAuthority.RequestCount ||
		uint64(telemetry.JudgeInputTokenUpperBound) > costAuthority.MaximumInputTokens ||
		actualOutputTokens > costAuthority.MaximumOutputTokens {
		return MemoryJudgeSliceDiagnosticReport{}, nil, ErrCaptureInvalid
	}
	evidenceClass := ProductionValidationEvidenceLive
	if providerMode == ProviderModeFakeProtocol {
		evidenceClass = ProductionValidationEvidenceFake
	}
	report := MemoryJudgeSliceDiagnosticReport{
		SchemaVersion:     MemoryJudgeSliceDiagnosticReportSchemaVersion,
		CorpusClass:       memoryeval.RegressionCorpusClass,
		AdmissionMode:     MemoryJudgeSliceDiagnosticAdmissionMode,
		PromotionEligible: false, ReleaseEligible: false, PolicySelected: false,
		ExecutionComplete: true, EvidenceClass: evidenceClass,
		Split:                           DevelopmentCalibrationSplit,
		CaseCount:                       MemoryJudgeSliceDiagnosticCaseCount,
		Repetitions:                     MemoryJudgeSliceDiagnosticRepetitions,
		ExecutionCount:                  MemoryJudgeSliceDiagnosticExecutionCount,
		SliceUnion:                      []string{"mixed_language_entity", "stable_fact"},
		IntersectionCaseCount:           5,
		DiagnosticCaseOrderSHA256:       orderSHA256,
		PolicyID:                        usermemory.HybridRelevanceSliceDiagnosticPolicyID,
		ProfileID:                       profile.Profile.ID,
		ConfigurationSHA256:             configurationSHA256,
		RelevancePolicyDescriptorSHA256: descriptorSHA256,
		ProviderEgressPolicy:            memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
		ProviderCostPolicy:              costBasis.ProviderCostPolicy,
		ProviderCostAuthorized:          true,
		JudgeProviderID:                 authority.ProviderID,
		JudgeProviderType:               authority.ProviderType,
		JudgeBaseURLSHA256:              authority.BaseURLSHA256,
		JudgeModelID:                    authority.ModelID,
		JudgeAdapter:                    memoryjudge.BufferedChatAdapterVersion,
		JudgePromptVersion:              usermemory.HybridCandidateJudgePromptVersion,
		JudgePromptSHA256:               usermemory.HybridCandidateJudgePromptSHA256,
		JudgeDecodingProfile:            usermemory.HybridCandidateJudgeDecodingProfile,
		ExecutionPolicy:                 executionPolicy,
		Classification:                  classification,
		RootCauseCounts:                 counts,
		Cases:                           entries,
		ProviderAttempts: JudgeFailureDiagnosticProviderTelemetry{
			AccuracyFirstProviderTelemetry: telemetry,
			JudgeAttemptFailureCategoryCounts: cloneDiagnosticCounts(
				telemetry.JudgeAttemptFailureCategoryCounts,
			),
		},
		CostAuthority: CloudJudgeDevelopmentCostAuthority{
			Unit:                                costBasis.Candidate.Unit,
			AuthorizedRequestCount:              costAuthority.RequestCount,
			ActualRequestCount:                  telemetry.JudgeAttempts,
			AuthorizedMaximumInputTokens:        costAuthority.MaximumInputTokens,
			ActualInputTokenUpperBound:          uint64(telemetry.JudgeInputTokenUpperBound),
			AuthorizedMaximumOutputTokens:       costAuthority.MaximumOutputTokens,
			ActualOutputTokenUpperBound:         actualOutputTokens,
			MaximumJudgeCostMicrounits:          costAuthority.MaximumCostMicrounits,
			MaximumMemoryProviderCostMicrounits: costBasis.Candidate.MemoryProviderCostMicrounits,
		},
	}
	body, err := json.Marshal(report)
	if err != nil {
		return MemoryJudgeSliceDiagnosticReport{}, nil, errors.Join(ErrCaptureInvalid, err)
	}
	return report, append(body, '\n'), nil
}

func BuildMemoryJudgeSliceDiagnosticRunManifest(
	runID string,
	captureID string,
	providerMode string,
	startedAt time.Time,
	completedAt time.Time,
	protected ProtectedRegression,
	costBasisSHA256 string,
	report MemoryJudgeSliceDiagnosticReport,
	artifacts []Artifact,
) (MemoryJudgeSliceDiagnosticRunManifest, []byte, error) {
	if !runIDPattern.MatchString(runID) || captureID == "" ||
		startedAt.IsZero() || completedAt.Before(startedAt) ||
		!validSHA256String(costBasisSHA256) || len(artifacts) != 1 ||
		report.SchemaVersion != MemoryJudgeSliceDiagnosticReportSchemaVersion ||
		report.EvidenceClass == "" || report.ExecutionCount != MemoryJudgeSliceDiagnosticExecutionCount ||
		report.PromotionEligible || report.ReleaseEligible || report.PolicySelected {
		return MemoryJudgeSliceDiagnosticRunManifest{}, nil, ErrCaptureInvalid
	}
	artifactManifest, err := buildRunArtifactManifest(artifacts)
	if err != nil || artifactManifest[0].Name != MemoryJudgeSliceDiagnosticArtifactName {
		return MemoryJudgeSliceDiagnosticRunManifest{}, nil, ErrCaptureInvalid
	}
	manifest := MemoryJudgeSliceDiagnosticRunManifest{
		SchemaVersion: MemoryJudgeSliceDiagnosticRunSchemaVersion,
		RunID:         runID, CaptureID: captureID,
		CorpusClass:       memoryeval.RegressionCorpusClass,
		AdmissionMode:     MemoryJudgeSliceDiagnosticAdmissionMode,
		PromotionEligible: false, ReleaseEligible: false, PolicySelected: false,
		CaptureMode:  CaptureModeMemoryJudgeSliceDiagnostic,
		Split:        DevelopmentCalibrationSplit,
		ProviderMode: providerMode, EvidenceClass: report.EvidenceClass,
		CaseCount: report.CaseCount, Repetitions: report.Repetitions,
		ExecutionCount:            report.ExecutionCount,
		DiagnosticCaseOrderSHA256: report.DiagnosticCaseOrderSHA256,
		ProfileID:                 report.ProfileID, PolicyID: report.PolicyID,
		ConfigurationSHA256: report.ConfigurationSHA256,
		Classification:      report.Classification,
		RootCauseCounts:     cloneDiagnosticCounts(report.RootCauseCounts),
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
		return MemoryJudgeSliceDiagnosticRunManifest{}, nil, errors.Join(ErrCaptureInvalid, err)
	}
	return manifest, append(body, '\n'), nil
}

func ValidateMemoryJudgeSliceDiagnosticCostAuthority(
	cost CostBasis,
	authority ConfiguredCandidateJudgeProfileAuthority,
) error {
	if cost.SchemaVersion != "neo-chat.memory-regression-cost-basis.v14" ||
		cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
		cost.CloudJudgeAuthority != nil || cost.MemoryToolRouteAuthority != nil ||
		!validFixedMemoryJudgeAuthority(authority) {
		return fmt.Errorf("%w: slice-diagnostic Memory Judge cost policy", ErrCaptureInvalid)
	}
	return validateConfiguredCandidateJudgeCostAuthority(
		cost, authority, MemoryJudgeSliceDiagnosticMaximumJudgeAttempts,
	)
}

func validateMemoryJudgeSliceDiagnosticRuntimePlan(
	selected memoryauthor.RegressionPool,
	cases []RuntimeCase,
) error {
	if len(selected.Corpus.Cases) != MemoryJudgeSliceDiagnosticCaseCount ||
		len(cases) != MemoryJudgeSliceDiagnosticExecutionCount {
		return ErrCaptureInvalid
	}
	seenAssistant := make(map[string]struct{}, len(cases))
	for index, item := range cases {
		expected := selected.Corpus.Cases[index%MemoryJudgeSliceDiagnosticCaseCount].ID
		if item.CaseID != expected || item.AssistantMessageID == "" {
			return ErrCaptureInvalid
		}
		if _, duplicate := seenAssistant[item.AssistantMessageID]; duplicate {
			return ErrCaptureInvalid
		}
		seenAssistant[item.AssistantMessageID] = struct{}{}
	}
	return nil
}

func classifyMemoryJudgeSliceDiagnostic(
	selected memoryauthor.RegressionPool,
	profile CapturedProfile,
) ([]MemoryJudgeSliceDiagnosticCase, map[string]int, string, int, uint64, int, error) {
	if len(profile.Cases) != MemoryJudgeSliceDiagnosticExecutionCount ||
		len(profile.Calibration) != MemoryJudgeSliceDiagnosticExecutionCount {
		return nil, nil, "", 0, 0, 0, ErrCaptureInvalid
	}
	entries := make([]MemoryJudgeSliceDiagnosticCase, 0, MemoryJudgeSliceDiagnosticExecutionCount)
	counts := map[string]int{
		SliceDiagnosticCauseCandidateMissing:  0,
		SliceDiagnosticCauseRerankFailed:      0,
		SliceDiagnosticCauseLunaNotSelected:   0,
		SliceDiagnosticCauseFinalRankOrBudget: 0,
		SliceDiagnosticCauseNone:              0,
		SliceDiagnosticCauseNotApplicable:     0,
		SliceDiagnosticCauseTerminalFailure:   0,
	}
	perCase := make(map[string][]string, MemoryJudgeSliceDiagnosticCaseCount)
	logicalRequests := 0
	logicalInputTokens := uint64(0)
	terminalAttempts := 0
	for index, trace := range profile.Calibration {
		golden := selected.Corpus.Cases[index%MemoryJudgeSliceDiagnosticCaseCount]
		observation := profile.Cases[index]
		if trace.CaseID != golden.ID || observation.CaseID != golden.ID ||
			trace.FullObservation.CaseID != golden.ID {
			return nil, nil, "", 0, 0, 0, ErrCaptureInvalid
		}
		expected := stringSetSlice(golden.ExpectedCurrentMemoryIDs)
		candidateCount := countSetMembers(trace.FullObservation.CandidateMemoryIDs, expected)
		rerankCount := countSetMembers(trace.RerankMemoryIDs, expected)
		judgeCount := countSetMembers(trace.JudgeSelectedMemoryIDs, expected)
		finalCount := countSetMembers(trace.FullObservation.FinalMemoryIDs, expected)
		rerankRank := maximumMemberRank(trace.RerankMemoryIDs, expected)
		cause := SliceDiagnosticCauseNone
		if len(expected) == 0 {
			cause = SliceDiagnosticCauseNotApplicable
		} else if trace.CloudJudgeFailureCategory != "" && !trace.CloudJudgeReady {
			cause = SliceDiagnosticCauseTerminalFailure
		} else if candidateCount < len(expected) {
			cause = SliceDiagnosticCauseCandidateMissing
		} else if !trace.RerankReady || rerankCount < len(expected) {
			cause = SliceDiagnosticCauseRerankFailed
		} else if !trace.CloudJudgeReady || judgeCount < len(expected) {
			cause = SliceDiagnosticCauseLunaNotSelected
		} else if finalCount < len(expected) {
			cause = SliceDiagnosticCauseFinalRankOrBudget
		}
		mixed := containsString(golden.Slices, "mixed_language_entity")
		stable := containsString(golden.Slices, "stable_fact")
		entries = append(entries, MemoryJudgeSliceDiagnosticCase{
			CaseID:              golden.ID,
			Repetition:          index/MemoryJudgeSliceDiagnosticCaseCount + 1,
			MixedLanguageEntity: mixed, StableFact: stable,
			Intersection:          mixed && stable,
			ExpectedCurrentCount:  len(expected),
			CandidateCurrentCount: candidateCount,
			RerankCurrentCount:    rerankCount,
			RerankCurrentRank:     rerankRank,
			JudgeCurrentCount:     judgeCount,
			FinalCurrentCount:     finalCount,
			RootCause:             cause,
		})
		counts[cause]++
		perCase[golden.ID] = append(perCase[golden.ID], cause)
		if trace.CloudJudgeReady || trace.CloudJudgeFailureCategory != "" {
			logicalRequests++
			if trace.CloudJudgeInputTokenUpperBound < 1 {
				return nil, nil, "", 0, 0, 0, ErrCaptureInvalid
			}
			logicalInputTokens += uint64(trace.CloudJudgeInputTokenUpperBound)
		}
		if memoryjudge.AttemptFailureCategory(trace.CloudJudgeFailureCategory) {
			terminalAttempts++
		}
	}
	classification := "not_reproduced"
	for _, causes := range perCase {
		if len(causes) != MemoryJudgeSliceDiagnosticRepetitions {
			return nil, nil, "", 0, 0, 0, ErrCaptureInvalid
		}
		if causes[0] != SliceDiagnosticCauseNone &&
			causes[0] != SliceDiagnosticCauseNotApplicable &&
			causes[0] == causes[1] && causes[1] == causes[2] {
			classification = "systematic"
			break
		}
		if causes[0] != causes[1] || causes[1] != causes[2] ||
			causes[0] == SliceDiagnosticCauseTerminalFailure {
			classification = "stochastic"
		}
	}
	return entries, counts, classification, logicalRequests, logicalInputTokens, terminalAttempts, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func stringSetSlice(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func countSetMembers(values []string, expected map[string]struct{}) int {
	seen := make(map[string]struct{}, len(values))
	count := 0
	for _, value := range values {
		if _, ok := expected[value]; !ok {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		count++
	}
	return count
}

func maximumMemberRank(values []string, expected map[string]struct{}) int {
	if len(expected) == 0 {
		return 0
	}
	seen := make(map[string]struct{}, len(expected))
	maximum := 0
	for index, value := range values {
		if _, ok := expected[value]; !ok {
			continue
		}
		seen[value] = struct{}{}
		if index+1 > maximum {
			maximum = index + 1
		}
	}
	if len(seen) != len(expected) {
		return 0
	}
	return maximum
}
