package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memorycapture"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/memoryroute"
	"neo-chat/mm-chat/backend/internal/providerfactory"
	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/runtimeconfig"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	adminDatabaseURLEnv                 = "MM_CHAT_MEMORY_REGRESSION_ADMIN_DATABASE_URL"
	runtimeDatabaseURLEnv               = "MM_CHAT_MEMORY_REGRESSION_RUNTIME_DATABASE_URL"
	liveEnabledEnv                      = "MM_CHAT_MEMORY_REGRESSION_LIVE_ENABLED"
	liveApprovalEnv                     = "MM_CHAT_MEMORY_REGRESSION_LIVE_APPROVAL"
	liveRunIDEnv                        = "MM_CHAT_MEMORY_REGRESSION_LIVE_RUN_ID"
	liveProviderIDEnv                   = "MM_CHAT_MEMORY_REGRESSION_LIVE_PROVIDER_ID"
	liveEmbeddingModelEnv               = "MM_CHAT_MEMORY_REGRESSION_LIVE_EMBEDDING_MODEL_ID"
	liveRerankModelEnv                  = "MM_CHAT_MEMORY_REGRESSION_LIVE_RERANK_MODEL_ID"
	liveCloudJudgeModelEnv              = "MM_CHAT_MEMORY_REGRESSION_LIVE_CLOUD_JUDGE_MODEL_ID"
	liveMemoryToolRouteApprovalEnv      = "MM_CHAT_MEMORY_REGRESSION_LIVE_MEMORY_TOOL_ROUTE_APPROVAL"
	liveMemoryToolRouteProviderIDEnv    = "MM_CHAT_MEMORY_REGRESSION_LIVE_MEMORY_TOOL_ROUTE_PROVIDER_ID"
	liveMemoryToolRouteProviderTypeEnv  = "MM_CHAT_MEMORY_REGRESSION_LIVE_MEMORY_TOOL_ROUTE_PROVIDER_TYPE"
	liveMemoryToolRouteBaseURLSHA256Env = "MM_CHAT_MEMORY_REGRESSION_LIVE_MEMORY_TOOL_ROUTE_BASE_URL_SHA256"
	liveMemoryToolRouteModelIDEnv       = "MM_CHAT_MEMORY_REGRESSION_LIVE_MEMORY_TOOL_ROUTE_MODEL_ID"

	defaultCaptureTimeout = 45 * time.Minute
	maximumCredentialSize = 4096
)

var (
	errMetricsFailed = errors.New("native Memory regression metric gates failed")
	commandRunIDRE   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
)

type commandOptions struct {
	root                string
	outputDir           string
	costBasisPath       string
	providerMode        string
	captureMode         string
	runID               string
	credentialPath      string
	pretty              bool
	judgeModelID        string
	routeCredentialPath string
	routeProviderID     string
	routeProviderType   string
	routeBaseURL        string
	routeModelID        string
}

type environmentLookup func(string) (string, bool)

type providerBundle struct {
	passage memorycapture.PassageEmbedder
	hybrid  usermemory.HybridShadowProvider
	judge   usermemory.HybridCandidateJudge
	router  usermemory.HybridMemoryToolRouter
	secrets [][]byte
	clear   func()
}

type commandSummary struct {
	SchemaVersion     string `json:"schemaVersion"`
	RunID             string `json:"runId"`
	CaptureID         string `json:"captureId"`
	CorpusClass       string `json:"corpusClass"`
	AdmissionMode     string `json:"admissionMode"`
	PromotionEligible bool   `json:"promotionEligible"`
	ProviderMode      string `json:"providerMode"`
	CaptureMode       string `json:"captureMode"`
	Split             string `json:"split"`
	BaselinePassed    bool   `json:"baselinePassed"`
	CandidatePassed   bool   `json:"candidatePassed"`
	PolicySelected    bool   `json:"policySelected"`
	OutputDirectory   string `json:"outputDirectory"`
}

func main() {
	log.SetFlags(0)
	if err := run(context.Background(), os.Args[1:], os.LookupEnv, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(
	parent context.Context,
	args []string,
	getenv environmentLookup,
	stdout io.Writer,
) error {
	options, err := parseCommand(args)
	if err != nil {
		return err
	}
	startedAt := time.Now().UTC()
	ctx, cancel := context.WithTimeout(parent, defaultCaptureTimeout)
	defer cancel()

	costBody, err := readRegularBoundedFile(options.costBasisPath, 64<<10, false)
	if err != nil {
		return errors.New("read Memory regression cost basis failed")
	}
	cost, costHash, err := memorycapture.DecodeCostBasis(costBody)
	if err != nil {
		return err
	}
	protected, err := memorycapture.LoadProtectedRegression(options.root)
	if err != nil {
		return err
	}
	authorization := loadLiveAuthorization(getenv)
	if err := memorycapture.AuthorizeProviderMode(options.providerMode, options.runID, authorization); err != nil {
		return err
	}
	var profileHashes memorycapture.ProfileHashes
	var calibrationConfig memorycapture.ProfileConfig
	var validationConfig memorycapture.ProfileConfig
	var cloudJudgeConfig memorycapture.ProfileConfig
	var memoryToolRouteConfig memorycapture.ProfileConfig
	var memoryToolRouteAuthority memorycapture.MemoryToolRouteProfileAuthority
	var relevanceConfigurationHash string
	var artifactNames []string
	var artifactPrefix string
	switch options.captureMode {
	case memorycapture.CaptureModeFullRegression:
		baselineConfig, candidateConfig, buildErr := memorycapture.BuildProfileConfigs(
			protected,
			costHash,
			options.providerMode,
		)
		if buildErr != nil {
			return buildErr
		}
		profileHashes, err = memorycapture.HashProfileConfigs(baselineConfig, candidateConfig)
		if err != nil {
			return err
		}
		artifactPrefix = strings.ReplaceAll(profileHashes.CandidateProfileID, "_", "-")
		artifactNames = []string{
			"native-v1-lexical.observations.json",
			"native-v1-lexical.report.json",
			artifactPrefix + ".observations.json",
			artifactPrefix + ".report.json",
			"run-manifest.json",
		}
	case memorycapture.CaptureModeCalibration:
		calibrationConfig, err = memorycapture.BuildDevelopmentCalibrationProfileConfig(
			protected,
			costHash,
			options.providerMode,
		)
		if err != nil {
			return err
		}
		relevanceConfigurationHash, err = memorycapture.ConfigurationSHA256(calibrationConfig)
		if err != nil {
			return err
		}
		artifactNames = []string{"relevance-calibration.json", "run-manifest.json"}
	case memorycapture.CaptureModeCloudJudgeDevelopment:
		if err := memorycapture.AuthorizeCloudJudgeTarget(
			options.providerMode,
			options.judgeModelID,
			authorization,
		); err != nil {
			return err
		}
		if err := memorycapture.ValidateCloudJudgeCostAuthority(
			cost,
			options.judgeModelID,
		); err != nil {
			return err
		}
		cloudJudgeConfig, err = memorycapture.BuildCloudJudgeDevelopmentProfileConfig(
			protected,
			costHash,
			options.providerMode,
			options.judgeModelID,
			cost.ProviderCostPolicy,
		)
		if err != nil {
			return err
		}
		relevanceConfigurationHash, err = memorycapture.ConfigurationSHA256(cloudJudgeConfig)
		if err != nil {
			return err
		}
		artifactNames = []string{"cloud-judge-development.json", "run-manifest.json"}
	case memorycapture.CaptureModeMemoryToolRouteDevelopment:
		memoryToolRouteAuthority, err = buildMemoryToolRouteAuthority(options)
		if err != nil {
			return err
		}
		if err := memorycapture.AuthorizeMemoryToolRouteTarget(
			options.providerMode,
			memoryToolRouteAuthority,
			authorization,
		); err != nil {
			return err
		}
		if err := memorycapture.ValidateMemoryToolFirstRoundCostAuthority(
			cost,
			memoryToolRouteAuthority,
		); err != nil {
			return err
		}
		memoryToolRouteConfig, err =
			memorycapture.BuildMemoryToolRouteDevelopmentProfileConfig(
				protected,
				costHash,
				options.providerMode,
				memoryToolRouteAuthority,
				cost.ProviderCostPolicy,
			)
		if err != nil {
			return err
		}
		relevanceConfigurationHash, err =
			memorycapture.ConfigurationSHA256(memoryToolRouteConfig)
		if err != nil {
			return err
		}
		artifactNames = []string{"memory-first-tool-round-development.json", "run-manifest.json"}
	case memorycapture.CaptureModeFrozenValidation:
		validationConfig, err = memorycapture.BuildFrozenValidationProfileConfig(
			protected,
			costHash,
			options.providerMode,
		)
		if err != nil {
			return err
		}
		relevanceConfigurationHash, err = memorycapture.ConfigurationSHA256(validationConfig)
		if err != nil {
			return err
		}
		artifactNames = []string{"relevance-validation.json", "run-manifest.json"}
	default:
		return usageError()
	}
	if err := memorycapture.PreflightArtifactOutputs(options.outputDir, artifactNames); err != nil {
		return err
	}

	providers, err := buildProviders(options)
	if err != nil {
		return err
	}
	if providers.clear != nil {
		defer providers.clear()
	}

	adminConfig, runtimeConfig, err := loadDatabaseConfigs(getenv)
	if err != nil {
		return err
	}
	adminDB, err := openDatabase(ctx, adminConfig)
	if err != nil {
		return err
	}
	defer adminDB.Close()
	runtimeDB, err := openDatabase(ctx, runtimeConfig)
	if err != nil {
		return err
	}
	defer runtimeDB.Close()
	if options.captureMode == memorycapture.CaptureModeCalibration {
		return runDevelopmentCalibration(
			ctx, stdout, options, startedAt, protected, cost, costHash,
			calibrationConfig, relevanceConfigurationHash, providers, adminDB, runtimeDB,
		)
	}
	if options.captureMode == memorycapture.CaptureModeCloudJudgeDevelopment {
		return runCloudJudgeDevelopment(
			ctx, stdout, options, startedAt, protected, cost, costHash,
			cloudJudgeConfig, relevanceConfigurationHash, providers, adminDB, runtimeDB,
		)
	}
	if options.captureMode == memorycapture.CaptureModeMemoryToolRouteDevelopment {
		return runMemoryToolRouteDevelopment(
			ctx, stdout, options, startedAt, protected, cost, costHash,
			memoryToolRouteConfig, memoryToolRouteAuthority,
			relevanceConfigurationHash, providers, adminDB, runtimeDB,
		)
	}
	if options.captureMode == memorycapture.CaptureModeFrozenValidation {
		return runFrozenValidation(
			ctx, stdout, options, startedAt, protected, cost, costHash,
			validationConfig, relevanceConfigurationHash, providers, adminDB, runtimeDB,
		)
	}

	index, err := memorycapture.BuildFixtureIndex(protected.Pool)
	if err != nil {
		return err
	}
	seed, err := memorycapture.SeedEphemeralDatabase(
		ctx,
		adminDB,
		protected.Pool,
		index,
		options.runID,
	)
	if err != nil {
		return err
	}
	if _, err := memorycapture.PopulateProjectionVectors(
		ctx,
		adminDB,
		options.runID,
		providers.passage,
	); err != nil {
		return err
	}
	baselineCapture, candidateCapture, err := memorycapture.CaptureProfiles(
		ctx,
		adminDB,
		runtimeDB,
		options.runID,
		index,
		seed,
		providers.hybrid,
		profileHashes,
		cost,
	)
	if err != nil {
		return err
	}
	captureID, err := newCaptureID()
	if err != nil {
		return errors.New("create Memory regression capture ID failed")
	}
	capturedAt, err := memorycapture.RegressionCaptureTimestamp(protected.Pool, time.Now().UTC())
	if err != nil {
		return err
	}
	baselineObservations, baselineObservationBody, err := memorycapture.AssembleRegressionObservations(
		protected.Pool,
		capturedAt,
		captureID,
		baselineCapture,
	)
	if err != nil {
		return err
	}
	candidateObservations, candidateObservationBody, err := memorycapture.AssembleRegressionObservations(
		protected.Pool,
		capturedAt,
		captureID,
		candidateCapture,
	)
	if err != nil {
		return err
	}
	baselineReport, err := evaluateRegression(protected, baselineObservations, baselineObservationBody)
	if err != nil {
		return err
	}
	candidateReport, err := evaluateRegression(protected, candidateObservations, candidateObservationBody)
	if err != nil {
		return err
	}
	baselineReportBody, err := marshalJSON(baselineReport, options.pretty)
	if err != nil {
		return err
	}
	candidateReportBody, err := marshalJSON(candidateReport, options.pretty)
	if err != nil {
		return err
	}

	artifacts := []memorycapture.Artifact{
		{Name: "native-v1-lexical.observations.json", Body: baselineObservationBody},
		{Name: "native-v1-lexical.report.json", Body: baselineReportBody},
		{Name: artifactPrefix + ".observations.json", Body: candidateObservationBody},
		{Name: artifactPrefix + ".report.json", Body: candidateReportBody},
	}
	_, manifestBody, err := memorycapture.BuildRunManifest(
		options.runID,
		captureID,
		options.providerMode,
		startedAt,
		time.Now().UTC(),
		protected,
		costHash,
		profileHashes,
		baselineReport,
		candidateReport,
		artifacts,
	)
	if err != nil {
		return err
	}
	artifacts = append(artifacts, memorycapture.Artifact{Name: "run-manifest.json", Body: manifestBody})
	if err := verifyRetainedArtifactsLeakFree(
		protected.Pool, artifacts, providers.secrets,
	); err != nil {
		return err
	}
	if _, err := memorycapture.PublishArtifactsExclusive(options.outputDir, artifacts); err != nil {
		return err
	}
	summary := commandSummary{
		SchemaVersion: "neo-chat.memory-regression-native-summary.v2",
		RunID:         options.runID, CaptureID: captureID,
		CorpusClass:   memoryeval.RegressionCorpusClass,
		AdmissionMode: memoryeval.RegressionAdmissionMode, PromotionEligible: false,
		ProviderMode:   options.providerMode,
		CaptureMode:    memorycapture.CaptureModeFullRegression,
		Split:          "all",
		BaselinePassed: baselineReport.Passed, CandidatePassed: candidateReport.Passed,
		OutputDirectory: filepath.Clean(options.outputDir),
	}
	if err := json.NewEncoder(stdout).Encode(summary); err != nil {
		return errors.New("write Memory regression summary failed")
	}
	if !baselineReport.Passed || !candidateReport.Passed {
		return errMetricsFailed
	}
	return nil
}

func runDevelopmentCalibration(
	ctx context.Context,
	stdout io.Writer,
	options commandOptions,
	startedAt time.Time,
	protected memorycapture.ProtectedRegression,
	cost memorycapture.CostBasis,
	costHash string,
	config memorycapture.ProfileConfig,
	configurationHash string,
	providers providerBundle,
	adminDB *sql.DB,
	runtimeDB *sql.DB,
) error {
	selectedPool, err := memorycapture.SelectRegressionCaptureSplit(
		protected.Pool,
		memorycapture.DevelopmentCalibrationSplit,
	)
	if err != nil {
		return err
	}
	index, err := memorycapture.BuildFixtureIndex(selectedPool)
	if err != nil {
		return err
	}
	seed, err := memorycapture.SeedEphemeralDatabase(
		ctx,
		adminDB,
		selectedPool,
		index,
		options.runID,
	)
	if err != nil {
		return err
	}
	if len(seed.Cases) != 300 {
		return memorycapture.ErrCaptureInvalid
	}
	if _, err := memorycapture.PopulateProjectionVectors(
		ctx,
		adminDB,
		options.runID,
		providers.passage,
	); err != nil {
		return err
	}
	captured, err := memorycapture.CaptureDevelopmentCalibration(
		ctx,
		adminDB,
		runtimeDB,
		options.runID,
		protected.Pool,
		index,
		seed,
		providers.hybrid,
		config.ProfileID,
		configurationHash,
		cost.Candidate,
	)
	if err != nil {
		return err
	}
	report, reportBody, err := memorycapture.BuildDevelopmentCalibration(
		protected.Pool,
		config.ProfileID,
		configurationHash,
		cost.Candidate,
		captured.Calibration,
	)
	if err != nil {
		return err
	}
	if options.pretty {
		reportBody, err = marshalJSON(report, true)
		if err != nil {
			return err
		}
	}
	captureID, err := newCaptureID()
	if err != nil {
		return errors.New("create Memory calibration capture ID failed")
	}
	artifacts := []memorycapture.Artifact{{Name: "relevance-calibration.json", Body: reportBody}}
	_, manifestBody, err := memorycapture.BuildCalibrationRunManifest(
		options.runID,
		captureID,
		options.providerMode,
		startedAt,
		time.Now().UTC(),
		protected,
		costHash,
		report,
		artifacts,
	)
	if err != nil {
		return err
	}
	artifacts = append(artifacts, memorycapture.Artifact{Name: "run-manifest.json", Body: manifestBody})
	if err := verifyRetainedArtifactsLeakFree(
		protected.Pool, artifacts, providers.secrets,
	); err != nil {
		return err
	}
	if _, err := memorycapture.PublishArtifactsExclusive(options.outputDir, artifacts); err != nil {
		return err
	}
	selected := report.Selected != nil || report.IntentSelected != nil
	summary := commandSummary{
		SchemaVersion: "neo-chat.memory-regression-native-summary.v2",
		RunID:         options.runID, CaptureID: captureID,
		CorpusClass:       memoryeval.RegressionCorpusClass,
		AdmissionMode:     memorycapture.CalibrationAdmissionMode,
		PromotionEligible: false, ProviderMode: options.providerMode,
		CaptureMode:     memorycapture.CaptureModeCalibration,
		Split:           memorycapture.DevelopmentCalibrationSplit,
		CandidatePassed: selected, PolicySelected: selected,
		OutputDirectory: filepath.Clean(options.outputDir),
	}
	if err := json.NewEncoder(stdout).Encode(summary); err != nil {
		return errors.New("write Memory calibration summary failed")
	}
	if !selected {
		return errMetricsFailed
	}
	return nil
}

func runCloudJudgeDevelopment(
	ctx context.Context,
	stdout io.Writer,
	options commandOptions,
	startedAt time.Time,
	protected memorycapture.ProtectedRegression,
	cost memorycapture.CostBasis,
	costHash string,
	config memorycapture.ProfileConfig,
	configurationHash string,
	providers providerBundle,
	adminDB *sql.DB,
	runtimeDB *sql.DB,
) error {
	selectedPool, err := memorycapture.SelectRegressionCaptureSplit(
		protected.Pool,
		memorycapture.DevelopmentCalibrationSplit,
	)
	if err != nil {
		return err
	}
	index, err := memorycapture.BuildFixtureIndex(selectedPool)
	if err != nil {
		return err
	}
	seed, err := memorycapture.SeedEphemeralDatabase(
		ctx,
		adminDB,
		selectedPool,
		index,
		options.runID,
	)
	if err != nil {
		return err
	}
	if len(seed.Cases) != 300 || providers.judge == nil {
		return memorycapture.ErrCaptureInvalid
	}
	if _, err := memorycapture.PopulateProjectionVectors(
		ctx,
		adminDB,
		options.runID,
		providers.passage,
	); err != nil {
		return err
	}
	captured, err := memorycapture.CaptureCloudJudgeDevelopment(
		ctx,
		adminDB,
		runtimeDB,
		options.runID,
		protected.Pool,
		index,
		seed,
		providers.hybrid,
		providers.judge,
		options.judgeModelID,
		config.ProfileID,
		configurationHash,
		cost.Candidate,
	)
	if err != nil {
		return err
	}
	report, reportBody, err := memorycapture.BuildCloudJudgeDevelopmentReport(
		protected.Pool,
		captured,
		options.judgeModelID,
		cost,
	)
	if err != nil {
		return err
	}
	if options.pretty {
		reportBody, err = marshalJSON(report, true)
		if err != nil {
			return err
		}
	}
	captureID, err := newCaptureID()
	if err != nil {
		return errors.New("create Memory cloud-judge capture ID failed")
	}
	artifacts := []memorycapture.Artifact{{
		Name: "cloud-judge-development.json",
		Body: reportBody,
	}}
	_, manifestBody, err := memorycapture.BuildCloudJudgeDevelopmentRunManifest(
		options.runID,
		captureID,
		options.providerMode,
		startedAt,
		time.Now().UTC(),
		protected,
		costHash,
		report,
		artifacts,
	)
	if err != nil {
		return err
	}
	artifacts = append(artifacts, memorycapture.Artifact{
		Name: "run-manifest.json",
		Body: manifestBody,
	})
	if err := verifyRetainedArtifactsLeakFree(
		protected.Pool, artifacts, providers.secrets,
	); err != nil {
		return err
	}
	if _, err := memorycapture.PublishArtifactsExclusive(
		options.outputDir,
		artifacts,
	); err != nil {
		return err
	}
	summary := commandSummary{
		SchemaVersion:     "neo-chat.memory-regression-native-summary.v3",
		RunID:             options.runID,
		CaptureID:         captureID,
		CorpusClass:       memoryeval.RegressionCorpusClass,
		AdmissionMode:     memorycapture.CloudJudgeDevelopmentAdmissionMode,
		PromotionEligible: false,
		ProviderMode:      options.providerMode,
		CaptureMode:       memorycapture.CaptureModeCloudJudgeDevelopment,
		Split:             memorycapture.DevelopmentCalibrationSplit,
		CandidatePassed:   report.Passed,
		PolicySelected:    report.Passed,
		OutputDirectory:   filepath.Clean(options.outputDir),
	}
	if err := json.NewEncoder(stdout).Encode(summary); err != nil {
		return errors.New("write Memory cloud-judge summary failed")
	}
	if !report.Passed {
		return errMetricsFailed
	}
	return nil
}

func runMemoryToolRouteDevelopment(
	ctx context.Context,
	stdout io.Writer,
	options commandOptions,
	startedAt time.Time,
	protected memorycapture.ProtectedRegression,
	cost memorycapture.CostBasis,
	costHash string,
	config memorycapture.ProfileConfig,
	authority memorycapture.MemoryToolRouteProfileAuthority,
	configurationHash string,
	providers providerBundle,
	adminDB *sql.DB,
	runtimeDB *sql.DB,
) error {
	selectedPool, err := memorycapture.SelectRegressionCaptureSplit(
		protected.Pool,
		memorycapture.DevelopmentCalibrationSplit,
	)
	if err != nil {
		return err
	}
	index, err := memorycapture.BuildFixtureIndex(selectedPool)
	if err != nil {
		return err
	}
	seed, err := memorycapture.SeedEphemeralDatabase(
		ctx,
		adminDB,
		selectedPool,
		index,
		options.runID,
	)
	if err != nil {
		return err
	}
	if len(seed.Cases) != 300 || providers.router == nil {
		return memorycapture.ErrCaptureInvalid
	}
	if _, err := memorycapture.PopulateProjectionVectors(
		ctx,
		adminDB,
		options.runID,
		providers.passage,
	); err != nil {
		return err
	}
	captured, err := memorycapture.CaptureMemoryToolRouteDevelopment(
		ctx,
		adminDB,
		runtimeDB,
		options.runID,
		protected.Pool,
		index,
		seed,
		providers.hybrid,
		providers.router,
		authority.ModelID,
		config.ProfileID,
		configurationHash,
		cost.Candidate,
	)
	if err != nil {
		return err
	}
	report, reportBody, err := memorycapture.BuildMemoryToolRouteDevelopmentReport(
		protected.Pool,
		captured,
		authority,
		cost,
	)
	if err != nil {
		return err
	}
	if options.pretty {
		reportBody, err = marshalJSON(report, true)
		if err != nil {
			return err
		}
	}
	captureID, err := newCaptureID()
	if err != nil {
		return errors.New("create Memory Tool-route capture ID failed")
	}
	artifacts := []memorycapture.Artifact{{
		Name: "memory-first-tool-round-development.json",
		Body: reportBody,
	}}
	_, manifestBody, err := memorycapture.BuildMemoryToolRouteDevelopmentRunManifest(
		options.runID,
		captureID,
		options.providerMode,
		startedAt,
		time.Now().UTC(),
		protected,
		costHash,
		report,
		artifacts,
	)
	if err != nil {
		return err
	}
	artifacts = append(artifacts, memorycapture.Artifact{
		Name: "run-manifest.json",
		Body: manifestBody,
	})
	if err := verifyRetainedArtifactsLeakFree(
		protected.Pool, artifacts, providers.secrets,
	); err != nil {
		return err
	}
	if _, err := memorycapture.PublishArtifactsExclusive(
		options.outputDir,
		artifacts,
	); err != nil {
		return err
	}
	summary := newMemoryToolRouteCommandSummary(options, captureID, report)
	if err := json.NewEncoder(stdout).Encode(summary); err != nil {
		return errors.New("write Memory Tool-route summary failed")
	}
	if !report.Passed {
		return errMetricsFailed
	}
	return nil
}

func newMemoryToolRouteCommandSummary(
	options commandOptions,
	captureID string,
	report memorycapture.MemoryToolRouteDevelopmentReport,
) commandSummary {
	return commandSummary{
		SchemaVersion:     "neo-chat.memory-regression-native-summary.v4",
		RunID:             options.runID,
		CaptureID:         captureID,
		CorpusClass:       report.CorpusClass,
		AdmissionMode:     report.AdmissionMode,
		PromotionEligible: false,
		ProviderMode:      options.providerMode,
		CaptureMode:       memorycapture.CaptureModeMemoryToolRouteDevelopment,
		Split:             report.Split,
		CandidatePassed:   report.Passed,
		PolicySelected:    report.Passed,
		OutputDirectory:   filepath.Clean(options.outputDir),
	}
}

func runFrozenValidation(
	ctx context.Context,
	stdout io.Writer,
	options commandOptions,
	startedAt time.Time,
	protected memorycapture.ProtectedRegression,
	cost memorycapture.CostBasis,
	costHash string,
	config memorycapture.ProfileConfig,
	configurationHash string,
	providers providerBundle,
	adminDB *sql.DB,
	runtimeDB *sql.DB,
) error {
	selectedPool, err := memorycapture.SelectRegressionCaptureSplit(
		protected.Pool,
		memorycapture.FrozenValidationSplit,
	)
	if err != nil {
		return err
	}
	index, err := memorycapture.BuildFixtureIndex(selectedPool)
	if err != nil {
		return err
	}
	seed, err := memorycapture.SeedEphemeralDatabase(
		ctx, adminDB, selectedPool, index, options.runID,
	)
	if err != nil {
		return err
	}
	if len(seed.Cases) != 100 {
		return memorycapture.ErrCaptureInvalid
	}
	if _, err := memorycapture.PopulateProjectionVectors(
		ctx, adminDB, options.runID, providers.passage,
	); err != nil {
		return err
	}
	captured, err := memorycapture.CaptureFrozenValidation(
		ctx, adminDB, runtimeDB, options.runID, protected.Pool, index, seed,
		providers.hybrid, config.ProfileID, configurationHash, cost.Candidate,
	)
	if err != nil {
		return err
	}
	report, reportBody, err := memorycapture.BuildFrozenValidation(
		protected.Pool,
		captured,
	)
	if err != nil {
		return err
	}
	if options.pretty {
		reportBody, err = marshalJSON(report, true)
		if err != nil {
			return err
		}
	}
	captureID, err := newCaptureID()
	if err != nil {
		return errors.New("create Memory validation capture ID failed")
	}
	artifacts := []memorycapture.Artifact{{Name: "relevance-validation.json", Body: reportBody}}
	_, manifestBody, err := memorycapture.BuildValidationRunManifest(
		options.runID, captureID, options.providerMode, startedAt, time.Now().UTC(),
		protected, costHash, report, artifacts,
	)
	if err != nil {
		return err
	}
	artifacts = append(artifacts, memorycapture.Artifact{Name: "run-manifest.json", Body: manifestBody})
	if err := verifyRetainedArtifactsLeakFree(
		protected.Pool, artifacts, providers.secrets,
	); err != nil {
		return err
	}
	if _, err := memorycapture.PublishArtifactsExclusive(options.outputDir, artifacts); err != nil {
		return err
	}
	summary := commandSummary{
		SchemaVersion: "neo-chat.memory-regression-native-summary.v2",
		RunID:         options.runID, CaptureID: captureID,
		CorpusClass:       memoryeval.RegressionCorpusClass,
		AdmissionMode:     memorycapture.ValidationAdmissionMode,
		PromotionEligible: false, ProviderMode: options.providerMode,
		CaptureMode:     memorycapture.CaptureModeFrozenValidation,
		Split:           memorycapture.FrozenValidationSplit,
		CandidatePassed: report.Passed, PolicySelected: true,
		OutputDirectory: filepath.Clean(options.outputDir),
	}
	if err := json.NewEncoder(stdout).Encode(summary); err != nil {
		return errors.New("write Memory validation summary failed")
	}
	if !report.Passed {
		return errMetricsFailed
	}
	return nil
}

func parseCommand(args []string) (commandOptions, error) {
	options := commandOptions{}
	flags := flag.NewFlagSet("memory-regression-capture", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.root, "root", "../data/memory-benchmark/v2-regression", "protected regression root")
	flags.StringVar(&options.outputDir, "output-dir", "", "new private output directory")
	flags.StringVar(&options.costBasisPath, "cost-basis", "", "versioned cost basis JSON")
	flags.StringVar(
		&options.providerMode,
		"provider-mode",
		memorycapture.ProviderModeFakeProtocol,
		"fake_protocol or live_siliconflow",
	)
	flags.StringVar(
		&options.captureMode,
		"capture-mode",
		memorycapture.CaptureModeFullRegression,
		"full_regression, development_calibration, development_cloud_judge, "+
			"development_memory_tool_route, or frozen_validation",
	)
	flags.StringVar(&options.runID, "run-id", "", "ephemeral run identifier")
	flags.StringVar(&options.credentialPath, "credential-file", "", "mode-0600 live credential file")
	flags.StringVar(
		&options.routeCredentialPath,
		"memory-tool-route-credential-file",
		"",
		"independent mode-0600 chat Provider credential file",
	)
	flags.StringVar(
		&options.routeProviderID,
		"memory-tool-route-provider-id",
		"",
		"exact configured chat Provider ID",
	)
	flags.StringVar(
		&options.routeProviderType,
		"memory-tool-route-provider-type",
		"",
		"openai or openai_compatible",
	)
	flags.StringVar(
		&options.routeBaseURL,
		"memory-tool-route-base-url",
		"",
		"exact configured chat Provider base URL",
	)
	flags.StringVar(
		&options.routeModelID,
		"memory-tool-route-model",
		"",
		"exact configured chat model ID",
	)
	flags.StringVar(
		&options.judgeModelID,
		"cloud-judge-model",
		memorycapture.DefaultSiliconFlowCloudJudgeModelID,
		"fixed cloud candidate-judge model ID",
	)
	flags.BoolVar(&options.pretty, "pretty", false, "pretty-print report JSON")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return commandOptions{}, usageError()
	}
	options.root = strings.TrimSpace(options.root)
	options.outputDir = strings.TrimSpace(options.outputDir)
	options.costBasisPath = strings.TrimSpace(options.costBasisPath)
	options.providerMode = strings.TrimSpace(options.providerMode)
	options.captureMode = strings.TrimSpace(options.captureMode)
	options.runID = strings.TrimSpace(options.runID)
	options.credentialPath = strings.TrimSpace(options.credentialPath)
	options.judgeModelID = strings.TrimSpace(options.judgeModelID)
	options.routeCredentialPath = strings.TrimSpace(options.routeCredentialPath)
	options.routeProviderID = strings.TrimSpace(options.routeProviderID)
	options.routeProviderType = strings.TrimSpace(options.routeProviderType)
	options.routeBaseURL = strings.TrimSpace(options.routeBaseURL)
	options.routeModelID = strings.TrimSpace(options.routeModelID)
	if options.root == "" || options.outputDir == "" || options.costBasisPath == "" ||
		!commandRunIDRE.MatchString(options.runID) {
		return commandOptions{}, usageError()
	}
	switch options.providerMode {
	case memorycapture.ProviderModeFakeProtocol:
		if options.credentialPath != "" || options.routeCredentialPath != "" {
			return commandOptions{}, errors.New("fake protocol does not accept credential files")
		}
	case memorycapture.ProviderModeLiveSiliconFlow:
		if options.credentialPath == "" {
			return commandOptions{}, errors.New("live SiliconFlow capture requires -credential-file")
		}
		if options.captureMode == memorycapture.CaptureModeFullRegression {
			return commandOptions{}, errors.New("live SiliconFlow capture requires split-safe calibration or validation mode")
		}
	default:
		return commandOptions{}, usageError()
	}
	switch options.captureMode {
	case memorycapture.CaptureModeFullRegression:
		if options.providerMode != memorycapture.ProviderModeFakeProtocol {
			return commandOptions{}, usageError()
		}
	case memorycapture.CaptureModeCalibration,
		memorycapture.CaptureModeCloudJudgeDevelopment,
		memorycapture.CaptureModeMemoryToolRouteDevelopment,
		memorycapture.CaptureModeFrozenValidation:
		if options.captureMode == memorycapture.CaptureModeCloudJudgeDevelopment &&
			options.judgeModelID == "" {
			return commandOptions{}, usageError()
		}
		if options.captureMode == memorycapture.CaptureModeMemoryToolRouteDevelopment {
			if options.routeProviderID == "" || options.routeProviderType == "" ||
				options.routeBaseURL == "" || options.routeModelID == "" {
				return commandOptions{}, errors.New("Memory Tool-route capture requires exact Provider ID/type/base URL/model")
			}
			if options.providerMode == memorycapture.ProviderModeLiveSiliconFlow &&
				options.routeCredentialPath == "" {
				return commandOptions{}, errors.New("live Memory Tool-route capture requires an independent credential file")
			}
		} else if options.routeCredentialPath != "" || options.routeProviderID != "" ||
			options.routeProviderType != "" || options.routeBaseURL != "" ||
			options.routeModelID != "" {
			return commandOptions{}, errors.New("Memory Tool-route inputs require development_memory_tool_route mode")
		}
	default:
		return commandOptions{}, usageError()
	}
	return options, nil
}

func usageError() error {
	return errors.New(
		"usage: memory-regression-capture -root DIR -output-dir DIR " +
			"-cost-basis FILE -provider-mode fake_protocol|live_siliconflow " +
			"-capture-mode full_regression|development_calibration|" +
			"development_cloud_judge|development_memory_tool_route|" +
			"frozen_validation -run-id ID [-credential-file FILE] " +
			"[-cloud-judge-model MODEL] " +
			"[-memory-tool-route-credential-file FILE " +
			"-memory-tool-route-provider-id ID " +
			"-memory-tool-route-provider-type openai|openai_compatible " +
			"-memory-tool-route-base-url URL " +
			"-memory-tool-route-model MODEL] [-pretty]",
	)
}

func buildProviders(options commandOptions) (providerBundle, error) {
	if options.providerMode == memorycapture.ProviderModeFakeProtocol {
		provider := memorycapture.NewFakeProtocolProvider()
		bundle := providerBundle{passage: provider, hybrid: provider, clear: func() {}}
		if options.captureMode == memorycapture.CaptureModeCloudJudgeDevelopment {
			bundle.judge = memorycapture.NewFakeProtocolCandidateJudge(options.judgeModelID)
		}
		if options.captureMode == memorycapture.CaptureModeMemoryToolRouteDevelopment {
			toolProvider := memorycapture.NewFakeProtocolMemoryToolRoundProvider(
				options.routeModelID,
			)
			router, adapterErr := memoryroute.NewChatToolAdapter(
				toolProvider,
				chat.ModelRef{
					ProviderID: options.routeProviderID,
					ModelID:    options.routeModelID,
				},
			)
			if adapterErr != nil {
				return providerBundle{}, errors.New("construct fake Memory first Tool-round adapter failed")
			}
			bundle.router = router
		}
		return bundle, nil
	}
	credential, err := readRegularBoundedFile(options.credentialPath, maximumCredentialSize, true)
	if err != nil {
		return providerBundle{}, errors.New("read live SiliconFlow credential failed")
	}
	credential = trimSingleLineEnding(credential)
	if !validCredential(credential) {
		clearBytes(credential)
		return providerBundle{}, errors.New("live SiliconFlow credential is invalid")
	}
	resolver := &ephemeralCredentialResolver{credential: credential}
	gateway := ragproviders.NewProviderGateway(resolver)
	profile, err := gateway.ForRetrievalProfile(ragproviders.RetrievalProfileSiliconFlow)
	if err != nil {
		resolver.clear()
		return providerBundle{}, errors.New("construct live SiliconFlow retrieval profile failed")
	}
	bundle := providerBundle{
		passage: gateway, hybrid: profile, secrets: [][]byte{credential},
		clear: resolver.clear,
	}
	if options.captureMode == memorycapture.CaptureModeCloudJudgeDevelopment {
		chatProvider, providerErr := providerfactory.NewChatProvider(providerfactory.ChatConfig{
			ProviderID: "siliconflow",
			Type:       runtimeconfig.ProviderTypeOpenAICompatible,
			BaseURL:    "https://api.siliconflow.cn/v1",
			APIKey:     string(credential),
			Timeout:    2 * time.Second,
		})
		if providerErr != nil {
			resolver.clear()
			return providerBundle{}, errors.New("construct live SiliconFlow cloud judge failed")
		}
		judge, judgeErr := memoryjudge.NewChatAdapter(chatProvider, chat.ModelRef{
			ProviderID: "siliconflow",
			ModelID:    options.judgeModelID,
		})
		if judgeErr != nil {
			resolver.clear()
			return providerBundle{}, errors.New("construct live SiliconFlow cloud judge failed")
		}
		bundle.judge = judge
	}
	if options.captureMode == memorycapture.CaptureModeMemoryToolRouteDevelopment {
		routeCredential, credentialErr := readRegularBoundedFile(
			options.routeCredentialPath,
			maximumCredentialSize,
			true,
		)
		if credentialErr != nil {
			resolver.clear()
			return providerBundle{}, errors.New("read live Memory Tool-route credential failed")
		}
		routeCredential = trimSingleLineEnding(routeCredential)
		if !validCredential(routeCredential) ||
			sameRegularFile(options.credentialPath, options.routeCredentialPath) ||
			bytes.Equal(credential, routeCredential) {
			clearBytes(routeCredential)
			resolver.clear()
			return providerBundle{}, errors.New("live Memory Tool-route credential is invalid or not independent")
		}
		authority, routeProviderType, routeBaseURL, routeErr :=
			resolveMemoryToolRoute(options)
		if routeErr != nil {
			clearBytes(routeCredential)
			resolver.clear()
			return providerBundle{}, routeErr
		}
		chatProvider, providerErr := providerfactory.NewChatProvider(providerfactory.ChatConfig{
			ProviderID: authority.ProviderID,
			Type:       routeProviderType,
			BaseURL:    routeBaseURL,
			APIKey:     string(routeCredential),
			Timeout:    2 * time.Second,
		})
		if providerErr != nil {
			clearBytes(routeCredential)
			resolver.clear()
			return providerBundle{}, errors.New("construct live Memory Tool-route Provider failed")
		}
		toolRoundProvider, ok := chatProvider.(chat.ToolRoundProvider)
		if !ok {
			clearBytes(routeCredential)
			resolver.clear()
			return providerBundle{}, errors.New("live Memory Tool-route Provider has no ToolRound support")
		}
		router, routerErr := memoryroute.NewChatToolAdapter(toolRoundProvider, chat.ModelRef{
			ProviderID: authority.ProviderID,
			ModelID:    authority.ModelID,
		})
		if routerErr != nil {
			clearBytes(routeCredential)
			resolver.clear()
			return providerBundle{}, errors.New("construct live Memory Tool-route adapter failed")
		}
		bundle.router = router
		bundle.secrets = append(bundle.secrets, routeCredential)
		bundle.clear = func() {
			clearBytes(routeCredential)
			resolver.clear()
		}
	}
	return bundle, nil
}

func buildMemoryToolRouteAuthority(
	options commandOptions,
) (memorycapture.MemoryToolRouteProfileAuthority, error) {
	authority, _, _, err := resolveMemoryToolRoute(options)
	return authority, err
}

func resolveMemoryToolRoute(
	options commandOptions,
) (
	memorycapture.MemoryToolRouteProfileAuthority,
	runtimeconfig.ProviderType,
	string,
	error,
) {
	providerID := strings.TrimSpace(options.routeProviderID)
	modelID := strings.TrimSpace(options.routeModelID)
	if !commandRunIDRE.MatchString(providerID) || !validRouteLabel(modelID, 512) {
		return memorycapture.MemoryToolRouteProfileAuthority{}, "", "",
			errors.New("Memory Tool-route Provider/model authority is invalid")
	}
	var providerType runtimeconfig.ProviderType
	var authorityType string
	switch strings.ToLower(strings.TrimSpace(options.routeProviderType)) {
	case "openai":
		providerType = runtimeconfig.ProviderTypeOpenAI
		authorityType = "openai"
	case "openai compatible", "openai-compatible", "openai_compatible":
		providerType = runtimeconfig.ProviderTypeOpenAICompatible
		authorityType = "openai_compatible"
	default:
		return memorycapture.MemoryToolRouteProfileAuthority{}, "", "",
			errors.New("Memory Tool-route Provider type is unsupported")
	}
	normalizedBaseURL, err := normalizeMemoryToolRouteBaseURL(options.routeBaseURL)
	if err != nil {
		return memorycapture.MemoryToolRouteProfileAuthority{}, "", "", err
	}
	return memorycapture.MemoryToolRouteProfileAuthority{
		ProviderID:    providerID,
		ProviderType:  authorityType,
		BaseURLSHA256: sha256Bytes([]byte(normalizedBaseURL)),
		ModelID:       modelID,
	}, providerType, normalizedBaseURL, nil
}

func normalizeMemoryToolRouteBaseURL(raw string) (string, error) {
	value := strings.TrimSuffix(strings.TrimSpace(raw), "#")
	value = strings.TrimRight(value, "/")
	if value == "" || value == "default" || len(value) > 2048 {
		return "", errors.New("Memory Tool-route base URL is invalid")
	}
	if !strings.HasSuffix(value, "/v1") {
		value += "/v1"
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("Memory Tool-route base URL is invalid")
	}
	return value, nil
}

func validRouteLabel(value string, maximum int) bool {
	if value == "" || len(value) > maximum || value != strings.TrimSpace(value) {
		return false
	}
	for _, current := range []byte(value) {
		if current < 33 || current > 126 {
			return false
		}
	}
	return true
}

func sameRegularFile(left, right string) bool {
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	return leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo)
}

type ephemeralCredentialResolver struct{ credential []byte }

func (resolver *ephemeralCredentialResolver) ResolveRAGProviderCredential(
	_ context.Context,
	providerID string,
) (string, error) {
	if resolver == nil || providerID != "siliconflow" || len(resolver.credential) == 0 {
		return "", ragproviders.ErrProviderGatewayUnavailable
	}
	return string(resolver.credential), nil
}

func (resolver *ephemeralCredentialResolver) clear() {
	if resolver == nil {
		return
	}
	clearBytes(resolver.credential)
	resolver.credential = nil
}

func loadLiveAuthorization(getenv environmentLookup) memorycapture.LiveAuthorization {
	value := func(key string) string {
		if getenv == nil {
			return ""
		}
		result, _ := getenv(key)
		return strings.TrimSpace(result)
	}
	return memorycapture.LiveAuthorization{
		Enabled: parseBool(value(liveEnabledEnv)), Approval: value(liveApprovalEnv),
		RunID: value(liveRunIDEnv), ProviderID: value(liveProviderIDEnv),
		EmbeddingModelID: value(liveEmbeddingModelEnv), RerankModelID: value(liveRerankModelEnv),
		CloudJudgeModelID:            value(liveCloudJudgeModelEnv),
		MemoryToolRouteApproval:      value(liveMemoryToolRouteApprovalEnv),
		MemoryToolRouteProviderID:    value(liveMemoryToolRouteProviderIDEnv),
		MemoryToolRouteProviderType:  value(liveMemoryToolRouteProviderTypeEnv),
		MemoryToolRouteBaseURLSHA256: value(liveMemoryToolRouteBaseURLSHA256Env),
		MemoryToolRouteModelID:       value(liveMemoryToolRouteModelIDEnv),
	}
}

func verifyRetainedArtifactsLeakFree(
	pool memoryauthor.RegressionPool,
	artifacts []memorycapture.Artifact,
	secrets [][]byte,
) error {
	if len(secrets) == 0 {
		return memorycapture.VerifyRetainedArtifactsLeakFree(pool, artifacts, nil)
	}
	for _, secret := range secrets {
		if err := memorycapture.VerifyRetainedArtifactsLeakFree(
			pool,
			artifacts,
			secret,
		); err != nil {
			return err
		}
	}
	return nil
}

func loadDatabaseConfigs(getenv environmentLookup) (*pgx.ConnConfig, *pgx.ConnConfig, error) {
	if getenv == nil {
		return nil, nil, errors.New("Memory regression database environment is unavailable")
	}
	adminURL, adminOK := getenv(adminDatabaseURLEnv)
	runtimeURL, runtimeOK := getenv(runtimeDatabaseURLEnv)
	if !adminOK || !runtimeOK || strings.TrimSpace(adminURL) == "" || strings.TrimSpace(runtimeURL) == "" {
		return nil, nil, errors.New("Memory regression database URLs are required")
	}
	admin, err := pgx.ParseConfig(strings.TrimSpace(adminURL))
	if err != nil {
		return nil, nil, errors.New("Memory regression admin database URL is invalid")
	}
	runtime, err := pgx.ParseConfig(strings.TrimSpace(runtimeURL))
	if err != nil {
		return nil, nil, errors.New("Memory regression runtime database URL is invalid")
	}
	if !strings.HasPrefix(admin.Database, "mm_chat_memory_regression_") ||
		admin.Database != runtime.Database || admin.Host != runtime.Host || admin.Port != runtime.Port ||
		admin.RuntimeParams["role"] == "go_api_runtime" || runtime.RuntimeParams["role"] != "go_api_runtime" {
		return nil, nil, errors.New("Memory regression database authority is invalid")
	}
	admin.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	runtime.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	return admin, runtime, nil
}

func openDatabase(ctx context.Context, config *pgx.ConnConfig) (*sql.DB, error) {
	if config == nil {
		return nil, errors.New("Memory regression database configuration is invalid")
	}
	db := stdlib.OpenDB(*config)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, errors.New("open Memory regression database failed")
	}
	return db, nil
}

func evaluateRegression(
	protected memorycapture.ProtectedRegression,
	observations memoryeval.RegressionObservationSet,
	body []byte,
) (memoryeval.RegressionReport, error) {
	return memoryeval.EvaluateRegression(memoryeval.RegressionEvaluationInput{
		Corpus: protected.Pool.Corpus, CorpusRawSHA256: protected.CorpusRawSHA256,
		Audit: protected.Pool.Audit, AuditRawSHA256: protected.AuditRawSHA256,
		Observations: observations, ObservationsSHA256: sha256Bytes(body),
	})
}

func marshalJSON(value any, pretty bool) ([]byte, error) {
	var body []byte
	var err error
	if pretty {
		body, err = json.MarshalIndent(value, "", "  ")
	} else {
		body, err = json.Marshal(value)
	}
	if err != nil {
		return nil, errors.New("encode Memory regression artifact failed")
	}
	return append(body, '\n'), nil
}

func readRegularBoundedFile(path string, maximum int, requireMode0600 bool) ([]byte, error) {
	pathInfo, err := os.Lstat(path)
	if err != nil || !pathInfo.Mode().IsRegular() || pathInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("file authority is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("open bounded file failed")
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(pathInfo, openedInfo) ||
		openedInfo.Size() < 1 || openedInfo.Size() > int64(maximum) ||
		(requireMode0600 && openedInfo.Mode().Perm() != 0o600) {
		return nil, errors.New("file authority is invalid")
	}
	body, err := io.ReadAll(io.LimitReader(file, int64(maximum)+1))
	if err != nil || len(body) < 1 || len(body) > maximum {
		return nil, errors.New("read bounded file failed")
	}
	return body, nil
}

func trimSingleLineEnding(value []byte) []byte {
	if len(value) > 0 && value[len(value)-1] == '\n' {
		value[len(value)-1] = 0
		value = value[:len(value)-1]
		if len(value) > 0 && value[len(value)-1] == '\r' {
			value[len(value)-1] = 0
			value = value[:len(value)-1]
		}
	}
	return value
}

func validCredential(value []byte) bool {
	if len(value) == 0 || len(value) > maximumCredentialSize {
		return false
	}
	for _, current := range value {
		if current < 33 || current > 126 {
			return false
		}
	}
	return true
}

func clearBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func newCaptureID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" +
		encoded[16:20] + "-" + encoded[20:32], nil
}

func sha256Bytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

var _ ragproviders.ProviderCredentialResolver = (*ephemeralCredentialResolver)(nil)
