package memorycapture

import (
	"context"
	"database/sql"
	"fmt"

	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

type ProfileHashes struct {
	Baseline           string
	Candidate          string
	BaselineProfileID  string
	CandidateProfileID string
}

func BuildProfileConfigs(
	protected ProtectedRegression,
	costBasisSHA256 string,
	providerMode string,
) (ProfileConfig, ProfileConfig, error) {
	candidateProfileID, err := candidateProfileID(providerMode)
	if err != nil {
		return ProfileConfig{}, ProfileConfig{}, err
	}
	common := ProfileConfig{
		SchemaVersion:     "neo-chat.memory-regression-profile-config.v1",
		ReaderVersion:     ReaderVersion,
		FixtureRawSHA256:  protected.FixtureRawSHA256,
		CorpusRawSHA256:   protected.CorpusRawSHA256,
		AuditRawSHA256:    protected.AuditRawSHA256,
		ManifestRawSHA256: protected.ManifestRawSHA256,
		CostBasisSHA256:   costBasisSHA256,
		CandidateLimit:    usermemory.MaxHybridShadowResults,
		FinalLimit:        usermemory.HybridShadowFinalLimit,
		TargetTokens:      usermemory.HybridShadowTargetTokens,
		MaximumTokens:     usermemory.HybridShadowMaximumTokens,
		HardCutoffMillis:  2000,
		FixtureMapping:    fixtureMappingVersion,
	}
	baseline := common
	baseline.ProfileID = BaselineProfileID
	baseline.ProviderMode = ProviderModeNone
	baseline.CounterfactualInject = false
	candidate := common
	candidate.ProfileID = candidateProfileID
	candidate.ProviderMode = providerMode
	candidate.EmbeddingProfileID = usermemory.HybridEmbeddingProfile
	candidate.EmbeddingModelID = ragproviders.SiliconFlowEmbeddingModel
	candidate.EmbeddingDimensions = ragproviders.SiliconFlowEmbeddingDimensions
	candidate.RerankModelID = ragproviders.SiliconFlowRerankModel
	candidate.CounterfactualInject = true
	return baseline, candidate, nil
}

func HashProfileConfigs(baseline, candidate ProfileConfig) (ProfileHashes, error) {
	if baseline.ProfileID != BaselineProfileID || baseline.ProviderMode != ProviderModeNone {
		return ProfileHashes{}, ErrCaptureInvalid
	}
	expectedCandidateProfileID, err := candidateProfileID(candidate.ProviderMode)
	if err != nil || candidate.ProfileID != expectedCandidateProfileID {
		return ProfileHashes{}, ErrCaptureInvalid
	}
	baselineHash, err := ConfigurationSHA256(baseline)
	if err != nil {
		return ProfileHashes{}, err
	}
	candidateHash, err := ConfigurationSHA256(candidate)
	if err != nil {
		return ProfileHashes{}, err
	}
	return ProfileHashes{
		Baseline: baselineHash, Candidate: candidateHash,
		BaselineProfileID: baseline.ProfileID, CandidateProfileID: candidate.ProfileID,
	}, nil
}

func candidateProfileID(providerMode string) (string, error) {
	switch providerMode {
	case ProviderModeFakeProtocol:
		return FakeCandidateProfileID, nil
	case ProviderModeLiveSiliconFlow:
		return CandidateProfileID, nil
	default:
		return "", fmt.Errorf("%w: provider mode", ErrCaptureInvalid)
	}
}

// CaptureProfiles executes both native profiles against the same ephemeral
// state. It does not score or publish; callers must assemble and validate both
// complete observation sets before exclusive publication.
func CaptureProfiles(
	ctx context.Context,
	seedDB *sql.DB,
	runtimeDB *sql.DB,
	runID string,
	index FixtureIndex,
	seed SeedResult,
	provider usermemory.HybridShadowProvider,
	hashes ProfileHashes,
	cost CostBasis,
) (CapturedProfile, CapturedProfile, error) {
	if seed.RunID != runID || seed.DatabaseName == "" {
		return CapturedProfile{}, CapturedProfile{}, ErrCaptureInvalid
	}
	if err := verifySeedDatabase(ctx, seedDB, runID); err != nil {
		return CapturedProfile{}, CapturedProfile{}, err
	}
	if err := VerifyRuntimeDatabase(ctx, runtimeDB, runID); err != nil {
		return CapturedProfile{}, CapturedProfile{}, err
	}
	for _, db := range []*sql.DB{seedDB, runtimeDB} {
		var databaseName string
		if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&databaseName); err != nil ||
			databaseName != seed.DatabaseName {
			return CapturedProfile{}, CapturedProfile{}, fmt.Errorf("%w: seeded database binding", ErrCaptureInvalid)
		}
	}
	if provider == nil || len(seed.Cases) == 0 || hashes.Baseline == "" || hashes.Candidate == "" ||
		hashes.BaselineProfileID != BaselineProfileID || hashes.CandidateProfileID == "" {
		return CapturedProfile{}, CapturedProfile{}, ErrCaptureInvalid
	}
	repository := usermemory.NewPostgresRepository(runtimeDB)
	baselineService := usermemory.NewService(repository)
	baselineCases := make([]memoryeval.CaseObservation, 0, len(seed.Cases))
	for _, item := range seed.Cases {
		observed, err := CaptureBaseline(ctx, baselineService, index, item)
		if err != nil {
			return CapturedProfile{}, CapturedProfile{}, fmt.Errorf("capture baseline case: %w", err)
		}
		baselineCases = append(baselineCases, observed)
	}
	if err := resetEphemeralReaderState(ctx, seedDB, runID); err != nil {
		return CapturedProfile{}, CapturedProfile{}, err
	}

	recorder := &Recorder{}
	decoratedRepository, err := NewRepositoryDecorator(repository, repository, recorder)
	if err != nil {
		return CapturedProfile{}, CapturedProfile{}, err
	}
	decoratedProvider, err := NewProviderDecorator(provider, recorder)
	if err != nil {
		return CapturedProfile{}, CapturedProfile{}, err
	}
	candidateService := usermemory.NewService(
		decoratedRepository,
		usermemory.WithHybridShadowProvider(decoratedProvider),
	)
	candidateCases := make([]memoryeval.CaseObservation, 0, len(seed.Cases))
	for _, item := range seed.Cases {
		observed, err := CaptureCandidate(ctx, candidateService, recorder, index, item)
		if err != nil {
			return CapturedProfile{}, CapturedProfile{}, fmt.Errorf("capture candidate case: %w", err)
		}
		candidateCases = append(candidateCases, observed)
	}
	baseline := CapturedProfile{
		Profile: memoryeval.Profile{
			ID: hashes.BaselineProfileID, Role: "baseline", ReaderVersion: ReaderVersion,
			ConfigurationSHA256: hashes.Baseline,
			CandidateLimit:      usermemory.MaxHybridShadowResults,
			FinalLimit:          usermemory.HybridShadowFinalLimit,
		},
		Costs: cost.Baseline, Cases: baselineCases,
	}
	candidate := CapturedProfile{
		Profile: memoryeval.Profile{
			ID: hashes.CandidateProfileID, Role: "candidate", ReaderVersion: ReaderVersion,
			ConfigurationSHA256: hashes.Candidate,
			CandidateLimit:      usermemory.MaxHybridShadowResults,
			FinalLimit:          usermemory.HybridShadowFinalLimit,
		},
		Costs: cost.Candidate, Cases: candidateCases,
	}
	return baseline, candidate, nil
}

func resetEphemeralReaderState(ctx context.Context, db *sql.DB, runID string) error {
	if err := verifySeedDatabase(ctx, db, runID); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
UPDATE user_memories SET last_used_at = NULL WHERE last_used_at IS NOT NULL
`); err != nil {
		return fmt.Errorf("%w: reset baseline reader side effects", ErrCaptureStateConflict)
	}
	return nil
}
