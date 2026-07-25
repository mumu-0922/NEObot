package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/database"
	"neo-chat/mm-chat/backend/internal/providersecrets"
	"neo-chat/mm-chat/backend/internal/ragevalcapture"
	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/runtimeconfig"
)

const (
	operatorDatabaseEnv = "RAG_REPLAY_DATABASE_URL"
	commandTimeout      = 120 * time.Minute
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "rag-capture:", err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("rag-capture", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	goldenPath := flags.String("promotion-golden", "", "frozen promotion Golden JSON")
	curationPath := flags.String("curation-queue", "", "reviewed source-rich curation queue")
	reviewPath := flags.String("human-review-receipt", "", "human-review receipt JSON")
	importPath := flags.String("source-import-receipt", "", "source import receipt JSON")
	candidateGenerationID := flags.String("candidate-generation-id", "", "exact verified Candidate generation UUID")
	candidateManifestHash := flags.String("candidate-artifact-manifest-hash", "", "verified Candidate manifest SHA-256")
	answerProvider := flags.String("answer-provider", "server-default", "fixed answer provider (server-default only)")
	answerModel := flags.String("answer-model", "", "fixed answer model ID")
	splitsValue := flags.String("splits", "development,validation", "development/validation split selection")
	caseID := flags.String("case-id", "", "single Development/Validation smoke case; never complete")
	maximumCases := flags.Int("maximum-cases", 0, "bounded smoke subset; zero captures every selected case")
	concurrency := flags.Int("concurrency", 4, "parallel case captures (1..16)")
	executeFrozenHoldout := flags.Bool(
		"execute-frozen-holdout",
		false,
		"execute the precommitted one-shot frozen Holdout",
	)
	developmentPreflightPath := flags.String(
		"development-preflight",
		"",
		"complete Development preflight report for Holdout admission",
	)
	validationPreflightPath := flags.String(
		"validation-preflight",
		"",
		"complete Validation preflight report for Holdout admission",
	)
	holdoutSealPath := flags.String(
		"holdout-seal",
		"",
		"exclusive one-shot Holdout seal path",
	)
	supplementalNoAnswerPath := flags.String(
		"supplemental-no-answer",
		"",
		"50-case supplemental no-answer suite JSON",
	)
	supplementalLatencySourcePath := flags.String(
		"supplemental-latency-source-report",
		"",
		"failed supplemental report for paired cold/warm latency diagnosis",
	)
	outputPath := flags.String("output", "", "atomic output JSON path")
	pretty := flags.Bool("pretty", false, "indent JSON output")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("invalid arguments")
	}
	if strings.TrimSpace(*outputPath) == "" || strings.TrimSpace(*answerModel) == "" {
		return errors.New("-output and -answer-model are required")
	}
	if strings.TrimSpace(*answerProvider) != "server-default" {
		return errors.New("only the frozen server-default answer provider is supported")
	}
	holdoutMode := *executeFrozenHoldout
	supplementalNoAnswerMode := strings.TrimSpace(*supplementalNoAnswerPath) != ""
	if err := validateCaptureModeFlags(
		holdoutMode,
		*supplementalNoAnswerPath,
		*supplementalLatencySourcePath,
		*splitsValue,
		*caseID,
		*maximumCases,
		*developmentPreflightPath,
		*validationPreflightPath,
		*holdoutSealPath,
		*outputPath,
	); err != nil {
		return err
	}
	splits := []string(nil)
	if !holdoutMode && !supplementalNoAnswerMode {
		splits = splitCaptureValues(*splitsValue)
		if len(splits) == 0 {
			return errors.New("at least one preflight split is required")
		}
	}

	loaded, err := ragevalcapture.LoadInputs(
		*goldenPath,
		*curationPath,
		*reviewPath,
		*importPath,
	)
	if err != nil {
		return err
	}
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return errors.New("runtime configuration is invalid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	runtimeDB, err := database.Open(ctx, cfg)
	if err != nil || runtimeDB == nil || runtimeDB.SQL() == nil {
		return errors.New("runtime database is unavailable")
	}
	defer runtimeDB.Close()
	operatorDB, err := openOperatorDatabase(ctx, os.Getenv(operatorDatabaseEnv))
	if err != nil {
		return err
	}
	defer operatorDB.Close()
	vault, err := providersecrets.LoadVaultFile(cfg.ProviderSecrets.KeyringFile)
	if err != nil {
		return errors.New("provider secret vault is unavailable")
	}
	runtimeService := runtimeconfig.NewService(
		cfg,
		runtimeconfig.WithProviderConfigRepository(
			runtimeconfig.NewPostgresProviderConfigRepository(runtimeDB.SQL()),
		),
		runtimeconfig.WithProviderSecretVault(vault),
	)
	providerContext := auth.WithUser(ctx, auth.User{
		ID: cfg.Auth.BootstrapUserID, DisplayName: cfg.Auth.BootstrapDisplayName,
	})
	resolved, err := runtimeService.ResolveServerDefaultProvider(providerContext)
	if err != nil {
		return errors.New("answer provider resolution failed")
	}
	if !slices.Contains(resolved.Models, strings.TrimSpace(*answerModel)) {
		return errors.New("answer model is not present in the resolved provider")
	}
	answerer, err := newProviderAnswerer(resolved, strings.TrimSpace(*answerModel), cfg.Provider.Timeout)
	if err != nil {
		return err
	}
	ragGateway := ragproviders.NewProviderGateway(runtimeService)
	candidateRetrieval, err := ragGateway.ForRetrievalProfile(
		ragproviders.RetrievalProfileSiliconFlow,
	)
	if err != nil {
		return errors.New("Candidate retrieval provider construction failed")
	}
	captureInput := ragevalcapture.CaptureInput{
		LoadedInputs: loaded,
		Store:        ragevalcapture.NewPostgresStore(operatorDB),
		CandidateProvider: ragevalcapture.CaptureRetrievalProvider{
			Profile:  ragproviders.SiliconFlowRetrievalProfile,
			Embedder: candidateRetrieval,
			Reranker: candidateRetrieval,
		},
		Answerer:              answerer,
		CandidateGenerationID: strings.TrimSpace(*candidateGenerationID),
		CandidateManifestHash: strings.TrimSpace(*candidateManifestHash),
		AnswerProviderID:      "SERVER_DEFAULT",
		AnswerModelID:         strings.TrimSpace(*answerModel),
		Splits:                splits,
		CaseID:                strings.TrimSpace(*caseID),
		MaximumCases:          *maximumCases,
		Concurrency:           *concurrency,
		Clock:                 time.Now,
		NewUUID:               randomUUID,
	}
	if holdoutMode {
		development, err := ragevalcapture.LoadPreflightReport(
			*developmentPreflightPath,
		)
		if err != nil {
			return err
		}
		validation, err := ragevalcapture.LoadPreflightReport(
			*validationPreflightPath,
		)
		if err != nil {
			return err
		}
		absoluteOutputPath, err := filepath.Abs(strings.TrimSpace(*outputPath))
		if err != nil {
			return errors.New("resolve Holdout output path")
		}
		observations, err := ragevalcapture.CaptureFrozenHoldout(
			providerContext,
			ragevalcapture.FrozenHoldoutInput{
				CaptureInput:          captureInput,
				Development:           development,
				Validation:            validation,
				ObservationOutputPath: absoluteOutputPath,
				Seal: func(seal ragevalcapture.HoldoutSeal) error {
					return ragevalcapture.CreateHoldoutSeal(*holdoutSealPath, seal)
				},
			},
		)
		if err != nil {
			return err
		}
		if err := ragevalcapture.WritePromotionObservationsExclusive(
			*outputPath,
			observations,
			*pretty,
		); err != nil {
			return err
		}
		_, err = fmt.Fprintf(
			output,
			"captured one-shot Holdout and published %d Candidate observations to %s\n",
			len(observations.Cases),
			strings.TrimSpace(*outputPath),
		)
		return err
	}
	if supplementalNoAnswerMode {
		suite, err := ragevalcapture.LoadSupplementalNoAnswerSuite(
			*supplementalNoAnswerPath,
		)
		if err != nil {
			return err
		}
		if strings.TrimSpace(*supplementalLatencySourcePath) != "" {
			sourceReport, err := ragevalcapture.LoadSupplementalNoAnswerReport(
				*supplementalLatencySourcePath,
			)
			if err != nil {
				return err
			}
			diagnostic, err :=
				ragevalcapture.CaptureSupplementalNoAnswerLatencyDiagnostic(
					providerContext,
					ragevalcapture.SupplementalNoAnswerLatencyDiagnosticInput{
						SupplementalNoAnswerInput: ragevalcapture.SupplementalNoAnswerInput{
							CaptureInput: captureInput,
							LoadedSuite:  suite,
						},
						SourceReport: sourceReport,
					},
				)
			if err != nil {
				return err
			}
			if err := ragevalcapture.WriteSupplementalNoAnswerLatencyDiagnosticExclusive(
				*outputPath,
				diagnostic,
				*pretty,
			); err != nil {
				return err
			}
			_, err = fmt.Fprintf(
				output,
				"captured paired supplemental cold/warm latency diagnostic to %s (conclusion=%s)\n",
				strings.TrimSpace(*outputPath),
				diagnostic.Conclusion,
			)
			if err != nil {
				return err
			}
			if !diagnostic.DiagnosticIntegrityPassed {
				return errors.New("supplemental latency diagnostic integrity failed")
			}
			return nil
		}
		report, err := ragevalcapture.CaptureSupplementalNoAnswer(
			providerContext,
			ragevalcapture.SupplementalNoAnswerInput{
				CaptureInput: captureInput,
				LoadedSuite:  suite,
			},
		)
		if err != nil {
			return err
		}
		if err := ragevalcapture.WriteSupplementalNoAnswerReportExclusive(
			*outputPath,
			report,
			*pretty,
		); err != nil {
			return err
		}
		_, err = fmt.Fprintf(
			output,
			"captured %d supplemental no-answer cases to %s (passed=%t)\n",
			len(report.Cases),
			strings.TrimSpace(*outputPath),
			report.Passed,
		)
		if err != nil {
			return err
		}
		if !report.Passed {
			return errors.New("supplemental no-answer gate failed")
		}
		return nil
	}
	report, err := ragevalcapture.CapturePreflight(providerContext, captureInput)
	if err != nil {
		return err
	}
	encoded, err := encodeCaptureReport(report, *pretty)
	if err != nil {
		return err
	}
	if err := writeCaptureReport(*outputPath, encoded); err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		output,
		"captured %d Candidate preflight cases to %s\n",
		len(report.Candidate.Cases),
		strings.TrimSpace(*outputPath),
	)
	return err
}

func validateCaptureModeFlags(
	holdout bool,
	supplementalNoAnswerPath string,
	supplementalLatencySourcePath string,
	splits string,
	caseID string,
	maximumCases int,
	developmentPath string,
	validationPath string,
	sealPath string,
	outputPath string,
) error {
	supplementalNoAnswerPath = strings.TrimSpace(supplementalNoAnswerPath)
	supplementalLatencySourcePath = strings.TrimSpace(supplementalLatencySourcePath)
	developmentPath = strings.TrimSpace(developmentPath)
	validationPath = strings.TrimSpace(validationPath)
	sealPath = strings.TrimSpace(sealPath)
	outputPath = strings.TrimSpace(outputPath)
	if holdout && supplementalNoAnswerPath != "" {
		return errors.New("select exactly one Holdout or supplemental no-answer mode")
	}
	if supplementalLatencySourcePath != "" && supplementalNoAnswerPath == "" {
		return errors.New(
			"supplemental latency diagnosis requires -supplemental-no-answer",
		)
	}
	if !holdout && supplementalNoAnswerPath == "" {
		if developmentPath != "" || validationPath != "" || sealPath != "" {
			return errors.New("Holdout inputs require -execute-frozen-holdout")
		}
		return nil
	}
	if strings.TrimSpace(splits) != "development,validation" ||
		strings.TrimSpace(caseID) != "" || maximumCases != 0 {
		return errors.New("special capture modes do not accept preflight case selection")
	}
	if supplementalNoAnswerPath != "" {
		if developmentPath != "" || validationPath != "" || sealPath != "" {
			return errors.New("supplemental no-answer mode does not accept Holdout inputs")
		}
		if _, err := os.Lstat(outputPath); err == nil {
			return errors.New("supplemental no-answer output already exists")
		} else if !errors.Is(err, os.ErrNotExist) {
			return errors.New("inspect supplemental no-answer output path")
		}
		return nil
	}
	if developmentPath == "" || validationPath == "" || sealPath == "" {
		return errors.New(
			"frozen Holdout requires Development, Validation, and seal paths",
		)
	}
	outputAbsolute, err := filepath.Abs(outputPath)
	if err != nil {
		return errors.New("resolve Holdout output path")
	}
	sealAbsolute, err := filepath.Abs(sealPath)
	if err != nil || outputAbsolute == sealAbsolute {
		return errors.New("Holdout output and seal paths are invalid")
	}
	for _, path := range []string{outputPath, sealPath} {
		if _, err := os.Lstat(path); err == nil {
			return errors.New("Holdout output or seal already exists; execution is refused")
		} else if !errors.Is(err, os.ErrNotExist) {
			return errors.New("inspect Holdout output or seal path")
		}
	}
	return nil
}

func openOperatorDatabase(ctx context.Context, databaseURL string) (*sql.DB, error) {
	databaseURL = strings.TrimSpace(databaseURL)
	if databaseURL == "" {
		return nil, errors.New("RAG_REPLAY_DATABASE_URL is required")
	}
	parsed, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("operator database URL is invalid")
	}
	parsed.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	parsed.RuntimeParams["application_name"] = "mm-chat-rag-promotion-capture"
	parsed.RuntimeParams["statement_timeout"] = "30000"
	parsed.RuntimeParams["lock_timeout"] = "3000"
	parsed.RuntimeParams["idle_in_transaction_session_timeout"] = "30000"
	db := stdlib.OpenDB(*parsed)
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, errors.New("operator database is unavailable")
	}
	return db, nil
}

type providerAnswerer struct {
	provider chat.Provider
	model    chat.ModelRef
}

func newProviderAnswerer(
	resolved runtimeconfig.ResolvedProvider,
	modelID string,
	timeout time.Duration,
) (providerAnswerer, error) {
	providerConfig := chat.OpenAICompatibleProviderConfig{
		BaseURL: resolved.BaseURL, APIKey: resolved.APIKey,
		ProviderID: resolved.ID, Timeout: timeout,
	}
	var provider chat.Provider
	var err error
	switch resolved.Type {
	case runtimeconfig.ProviderTypeOpenAI:
		provider, err = chat.NewOpenAIProvider(providerConfig)
	case runtimeconfig.ProviderTypeOpenAICompatible:
		provider, err = chat.NewOpenAICompatibleProvider(providerConfig)
	default:
		return providerAnswerer{}, errors.New("answer provider type is unsupported")
	}
	resolved.APIKey = ""
	if err != nil {
		return providerAnswerer{}, errors.New("answer provider construction failed")
	}
	return providerAnswerer{
		provider: provider,
		model:    chat.ModelRef{ProviderID: resolved.ID, ModelID: modelID},
	}, nil
}

func (answerer providerAnswerer) Answer(
	ctx context.Context,
	systemPrompt string,
	prompt string,
) (ragevalcapture.AnswerResult, error) {
	if answerer.provider == nil {
		return ragevalcapture.AnswerResult{}, errors.New("answer provider is unavailable")
	}
	events, err := answerer.provider.StreamChat(ctx, chat.ProviderRequest{
		Prompt: prompt, SystemPrompt: systemPrompt, ModelRef: answerer.model,
	})
	if err != nil {
		return ragevalcapture.AnswerResult{}, err
	}
	var content strings.Builder
	result := ragevalcapture.AnswerResult{}
	for event := range events {
		if event.Error != nil {
			return ragevalcapture.AnswerResult{}, event.Error
		}
		switch event.Type {
		case chat.ProviderEventDelta:
			content.WriteString(event.Delta)
		case chat.ProviderEventUsage:
			if event.Usage != nil {
				result.Usage = ragevalcapture.AnswerUsage{
					PromptTokens:     event.Usage.PromptTokens,
					CompletionTokens: event.Usage.CompletionTokens,
					TotalTokens:      event.Usage.TotalTokens,
				}
			}
		}
	}
	result.Content = content.String()
	return result, nil
}

func splitCaptureValues(value string) []string {
	result := make([]string, 0, 2)
	seen := make(map[string]struct{})
	for _, item := range strings.Split(value, ",") {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if _, duplicate := seen[item]; duplicate {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func randomUUID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic("secure UUID generation failed")
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16],
	)
}

func encodeCaptureReport(report ragevalcapture.PreflightReport, pretty bool) ([]byte, error) {
	var encoded []byte
	var err error
	if pretty {
		encoded, err = json.MarshalIndent(report, "", "  ")
	} else {
		encoded, err = json.Marshal(report)
	}
	if err != nil {
		return nil, errors.New("encode capture report")
	}
	return append(encoded, '\n'), nil
}

func writeCaptureReport(path string, body []byte) error {
	path = strings.TrimSpace(path)
	if path == "" || len(body) == 0 {
		return errors.New("capture output is invalid")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return errors.New("create capture output directory")
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, body, 0o600); err != nil {
		return errors.New("write capture output")
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return errors.New("publish capture output")
	}
	return nil
}

var _ ragevalcapture.Answerer = providerAnswerer{}
