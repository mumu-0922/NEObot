package main

import (
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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/memorycapture"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	adminDatabaseURLEnv   = "MM_CHAT_MEMORY_REGRESSION_ADMIN_DATABASE_URL"
	runtimeDatabaseURLEnv = "MM_CHAT_MEMORY_REGRESSION_RUNTIME_DATABASE_URL"
	liveEnabledEnv        = "MM_CHAT_MEMORY_REGRESSION_LIVE_ENABLED"
	liveApprovalEnv       = "MM_CHAT_MEMORY_REGRESSION_LIVE_APPROVAL"
	liveRunIDEnv          = "MM_CHAT_MEMORY_REGRESSION_LIVE_RUN_ID"
	liveProviderIDEnv     = "MM_CHAT_MEMORY_REGRESSION_LIVE_PROVIDER_ID"
	liveEmbeddingModelEnv = "MM_CHAT_MEMORY_REGRESSION_LIVE_EMBEDDING_MODEL_ID"
	liveRerankModelEnv    = "MM_CHAT_MEMORY_REGRESSION_LIVE_RERANK_MODEL_ID"

	defaultCaptureTimeout = 45 * time.Minute
	maximumCredentialSize = 4096
)

var (
	errMetricsFailed = errors.New("native Memory regression metric gates failed")
	commandRunIDRE   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
)

type commandOptions struct {
	root           string
	outputDir      string
	costBasisPath  string
	providerMode   string
	runID          string
	credentialPath string
	pretty         bool
}

type environmentLookup func(string) (string, bool)

type providerBundle struct {
	passage memorycapture.PassageEmbedder
	hybrid  usermemory.HybridShadowProvider
	secret  []byte
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
	BaselinePassed    bool   `json:"baselinePassed"`
	CandidatePassed   bool   `json:"candidatePassed"`
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
	baselineConfig, candidateConfig, err := memorycapture.BuildProfileConfigs(
		protected,
		costHash,
		options.providerMode,
	)
	if err != nil {
		return err
	}
	profileHashes, err := memorycapture.HashProfileConfigs(baselineConfig, candidateConfig)
	if err != nil {
		return err
	}
	artifactPrefix := strings.ReplaceAll(profileHashes.CandidateProfileID, "_", "-")
	artifactNames := []string{
		"native-v1-lexical.observations.json",
		"native-v1-lexical.report.json",
		artifactPrefix + ".observations.json",
		artifactPrefix + ".report.json",
		"run-manifest.json",
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
	if err := memorycapture.VerifyRetainedArtifactsLeakFree(
		protected.Pool,
		artifacts,
		providers.secret,
	); err != nil {
		return err
	}
	if _, err := memorycapture.PublishArtifactsExclusive(options.outputDir, artifacts); err != nil {
		return err
	}
	summary := commandSummary{
		SchemaVersion: "neo-chat.memory-regression-native-summary.v1",
		RunID:         options.runID, CaptureID: captureID,
		CorpusClass:   memoryeval.RegressionCorpusClass,
		AdmissionMode: memoryeval.RegressionAdmissionMode, PromotionEligible: false,
		ProviderMode:   options.providerMode,
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

func parseCommand(args []string) (commandOptions, error) {
	options := commandOptions{}
	flags := flag.NewFlagSet("memory-regression-capture", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.root, "root", "../data/memory-benchmark/v2-regression", "protected regression root")
	flags.StringVar(&options.outputDir, "output-dir", "", "new private output directory")
	flags.StringVar(&options.costBasisPath, "cost-basis", "", "versioned cost basis JSON")
	flags.StringVar(&options.providerMode, "provider-mode", memorycapture.ProviderModeFakeProtocol, "fake_protocol or live_siliconflow")
	flags.StringVar(&options.runID, "run-id", "", "ephemeral run identifier")
	flags.StringVar(&options.credentialPath, "credential-file", "", "mode-0600 live credential file")
	flags.BoolVar(&options.pretty, "pretty", false, "pretty-print report JSON")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return commandOptions{}, usageError()
	}
	options.root = strings.TrimSpace(options.root)
	options.outputDir = strings.TrimSpace(options.outputDir)
	options.costBasisPath = strings.TrimSpace(options.costBasisPath)
	options.providerMode = strings.TrimSpace(options.providerMode)
	options.runID = strings.TrimSpace(options.runID)
	options.credentialPath = strings.TrimSpace(options.credentialPath)
	if options.root == "" || options.outputDir == "" || options.costBasisPath == "" ||
		!commandRunIDRE.MatchString(options.runID) {
		return commandOptions{}, usageError()
	}
	switch options.providerMode {
	case memorycapture.ProviderModeFakeProtocol:
		if options.credentialPath != "" {
			return commandOptions{}, errors.New("fake protocol does not accept a credential file")
		}
	case memorycapture.ProviderModeLiveSiliconFlow:
		if options.credentialPath == "" {
			return commandOptions{}, errors.New("live SiliconFlow capture requires -credential-file")
		}
	default:
		return commandOptions{}, usageError()
	}
	return options, nil
}

func usageError() error {
	return errors.New("usage: memory-regression-capture -root DIR -output-dir DIR -cost-basis FILE -provider-mode fake_protocol|live_siliconflow -run-id ID [-credential-file FILE] [-pretty]")
}

func buildProviders(options commandOptions) (providerBundle, error) {
	if options.providerMode == memorycapture.ProviderModeFakeProtocol {
		provider := memorycapture.NewFakeProtocolProvider()
		return providerBundle{passage: provider, hybrid: provider, clear: func() {}}, nil
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
	return providerBundle{
		passage: gateway, hybrid: profile, secret: credential,
		clear: resolver.clear,
	}, nil
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
	}
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
		value = value[:len(value)-1]
		if len(value) > 0 && value[len(value)-1] == '\r' {
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
