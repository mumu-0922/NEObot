package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/browserimport"
	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/database"
	"neo-chat/mm-chat/backend/internal/files"
	"neo-chat/mm-chat/backend/internal/httpserver"
	"neo-chat/mm-chat/backend/internal/imagejobs"
	"neo-chat/mm-chat/backend/internal/jobartifacts"
	"neo-chat/mm-chat/backend/internal/jobaudit"
	"neo-chat/mm-chat/backend/internal/knowledge"
	"neo-chat/mm-chat/backend/internal/plugins"
	"neo-chat/mm-chat/backend/internal/providersecrets"
	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/ragsource"
	"neo-chat/mm-chat/backend/internal/ratelimit"
	"neo-chat/mm-chat/backend/internal/redisstate"
	"neo-chat/mm-chat/backend/internal/runtimeconfig"
	"neo-chat/mm-chat/backend/internal/sessioncache"
	"neo-chat/mm-chat/backend/internal/storage"
	"neo-chat/mm-chat/backend/internal/teams"
	"neo-chat/mm-chat/backend/internal/usermemory"
	"neo-chat/mm-chat/backend/internal/voicejobs"
)

const (
	databaseOpenTimeout = 5 * time.Second
	redisOpenTimeout    = 5 * time.Second
	storageOpenTimeout  = 10 * time.Second
	shutdownTimeout     = 10 * time.Second
)

var publishedExampleTeamKeys = [][]byte{
	[]byte("fake-cursor-key-not-production!!"),
	[]byte("fake-mail-key-not-production!!!!"),
}

var (
	sensitiveURLUserInfoRE = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^\s]*@`)
	sensitiveAssignmentRE  = regexp.MustCompile(`(?i)([A-Za-z0-9_.-]*(?:api[_-]?key|authorization|password|secret|token|keyring)[A-Za-z0-9_.-]*\s*[=:]\s*)([^\s&]+)`)
	bearerTokenRE          = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]+`)
)

type teamRuntime struct {
	service           *teams.Service
	worker            *teams.InviteMailOutboxWorker
	cursor            *teams.CursorCodec
	memoryPortability *usermemory.PortabilityPlanCodec
}

type teamWorker interface {
	Run(context.Context) error
}

type runtimeFailure struct {
	component string
	err       error
}

type teamWorkerReadiness struct {
	gate teams.InviteDeliveryGate
}

type answerProviderConfigReader interface {
	ListProviderConfigs(context.Context, string) ([]runtimeconfig.StoredProviderConfig, error)
}

func (check teamWorkerReadiness) CheckReady(ctx context.Context) error {
	if check.gate == nil {
		return teams.ErrInviteDeliveryUnavailable
	}
	return check.gate.AdmitInviteDelivery(ctx)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		logger.Error("config_failed", slog.String("error", redactSensitiveLogText(err.Error())))
		os.Exit(1)
	}
	providerSecretVault, err := newProviderSecretVault(cfg)
	if err != nil {
		logger.Error("provider_secret_keyring_failed")
		os.Exit(1)
	}

	openCtx, openCancel := context.WithTimeout(context.Background(), databaseOpenTimeout)
	db, err := database.Open(openCtx, cfg)
	openCancel()
	if err != nil {
		logger.Error("database_open_failed", slog.String("error", redactSensitiveLogText(err.Error())))
		os.Exit(1)
	}

	redisCtx, redisCancel := context.WithTimeout(context.Background(), redisOpenTimeout)
	redisClient, runCancellationStore, rateLimitStore, sessionCache, err := newRedisState(redisCtx, cfg)
	redisCancel()
	if err != nil {
		_ = db.Close()
		logger.Error("redis_open_failed", slog.String("error", redactSensitiveLogText(err.Error())))
		os.Exit(1)
	}

	recoveryDelivery, err := newRecoveryDelivery(cfg)
	if err != nil {
		_ = redisClient.Close()
		_ = db.Close()
		logger.Error("auth_recovery_delivery_config_failed", slog.String("error", redactSensitiveLogText(err.Error())))
		os.Exit(1)
	}

	var chatRepo chat.Repository
	var fileRepo files.Repository
	var importRepo browserimport.Repository
	var pluginRegistry plugins.Registry
	var pluginAuditRecorder plugins.AuditRecorder
	var runtimeConfigRepo runtimeconfig.ProviderConfigRepository
	var taskModelRepo runtimeconfig.TaskModelSettingsRepository
	var userMemoryRepo usermemory.Repository
	var sessionResolver httpserver.SessionResolver
	var developmentSession *auth.Session
	var authService *auth.Service
	sqlDB := db.SQL()
	if sqlDB != nil {
		authRepo := auth.NewPostgresSessionRepository(sqlDB)
		if cfg.Auth.Mode == config.AuthModeDevelopment {
			developmentSessionCtx, developmentSessionCancel := context.WithTimeout(
				context.Background(),
				databaseOpenTimeout,
			)
			session, sessionErr := authRepo.EnsureDevelopmentSession(
				developmentSessionCtx,
				time.Now().UTC().Add(cfg.Auth.SessionTTL),
			)
			developmentSessionCancel()
			if sessionErr != nil {
				_ = redisClient.Close()
				_ = db.Close()
				logger.Error("development_session_bootstrap_failed", slog.String("error", redactSensitiveLogText(sessionErr.Error())))
				os.Exit(1)
			}
			developmentSession = &session
		}
		chatRepo = chat.NewPostgresRepository(sqlDB)
		fileRepo = files.NewPostgresRepository(sqlDB)
		pluginRegistry = plugins.NewPostgresRegistry(sqlDB, plugins.BuiltInPlugins()...)
		pluginAuditRecorder = plugins.NewPostgresAuditRecorder(sqlDB)
		runtimeConfigRepo = runtimeconfig.NewPostgresProviderConfigRepository(sqlDB)
		taskModelRepo = runtimeconfig.NewPostgresTaskModelSettingsRepository(sqlDB)
		userMemoryRepo = usermemory.NewPostgresRepository(sqlDB)
		sessionResolver = auth.NewSessionResolver(
			authRepo,
			auth.WithSessionCache(sessionCache),
		)
		authService = auth.NewService(
			authRepo,
			auth.WithAuthSessionCache(sessionCache),
			auth.WithRecoveryDelivery(recoveryDelivery),
			auth.WithSessionTTL(cfg.Auth.SessionTTL),
			auth.WithRecoveryTTL(cfg.Auth.RecoveryTTL),
		)
	}

	teamRuntime, err := newTeamRuntime(sqlDB, cfg)
	if err != nil {
		_ = redisClient.Close()
		_ = db.Close()
		logger.Error("team_config_failed", slog.String("error", redactSensitiveLogText(err.Error())))
		os.Exit(1)
	}
	objectStore, err := newObjectStore(cfg)
	if err != nil {
		_ = redisClient.Close()
		_ = db.Close()
		logger.Error("storage_config_failed", slog.String("error", redactSensitiveLogText(err.Error())))
		os.Exit(1)
	}
	var knowledgeRepo knowledge.Repository
	var consentExpiryWorker teamWorker
	if sqlDB != nil {
		postgresKnowledgeRepo := knowledge.NewPostgresRepository(sqlDB)
		knowledgeRepo = postgresKnowledgeRepo
		consentExpiryWorker, err = knowledge.NewConsentExpiryWorker(postgresKnowledgeRepo, 0, 0)
		if err != nil {
			_ = redisClient.Close()
			_ = db.Close()
			logger.Error("consent_expiry_worker_config_failed", slog.String("error", redactSensitiveLogText(err.Error())))
			os.Exit(1)
		}
	}
	providerRuntimeService := runtimeconfig.NewService(
		cfg,
		runtimeconfig.WithProviderConfigRepository(runtimeConfigRepo),
		runtimeconfig.WithProviderSecretVault(providerSecretVault),
	)
	ragProviderGateway := ragproviders.NewProviderGateway(providerRuntimeService)
	rerankConfigured := cfg.Auth.Mode == config.AuthModeDevelopment &&
		runtimeConfigRepo != nil && providerSecretVault != nil
	knowledgeOptions := []knowledge.ServiceOption{
		knowledge.WithCursorCodec(teamRuntime.cursor),
		knowledge.WithObjectStore(objectStore),
		knowledge.WithSingleUserCollectionConsents(),
	}
	if rerankConfigured {
		knowledgeOptions = append(
			knowledgeOptions,
			knowledge.WithSingleUserSiliconFlowRetrievalConsents(),
		)
	}
	answerIdentities := make([]knowledge.ProcessorModelIdentity, 0)
	if cfg.Auth.Mode == config.AuthModeDevelopment {
		answerConfigCtx, answerConfigCancel := context.WithTimeout(
			context.Background(),
			databaseOpenTimeout,
		)
		answerIdentities, err = singleUserAnswerIdentities(
			answerConfigCtx,
			runtimeConfigRepo,
			cfg.Auth.BootstrapUserID,
		)
		answerConfigCancel()
		if err != nil {
			_ = redisClient.Close()
			_ = db.Close()
			logger.Error("knowledge_answer_processing_config_failed", slog.String("error", redactSensitiveLogText(err.Error())))
			os.Exit(1)
		}
		for _, identity := range answerIdentities {
			knowledgeOptions = append(
				knowledgeOptions,
				knowledge.WithSingleUserAnswerConsent(identity),
			)
		}
	}
	knowledgeService := knowledge.NewService(knowledgeRepo, knowledgeOptions...)
	if sqlDB != nil {
		bootstrapCtx, bootstrapCancel := context.WithTimeout(context.Background(), databaseOpenTimeout)
		governanceService := knowledge.NewGovernanceService(knowledge.NewPostgresRepository(sqlDB))
		err = knowledge.BootstrapSingleUserNativeProcessing(
			bootstrapCtx,
			knowledgeService,
			governanceService,
			auth.User{ID: cfg.Auth.BootstrapUserID, DisplayName: cfg.Auth.BootstrapDisplayName},
		)
		if err == nil && cfg.Auth.Mode == config.AuthModeDevelopment {
			for _, identity := range answerIdentities {
				err = knowledge.BootstrapSingleUserAnswerProcessing(
					bootstrapCtx,
					knowledgeService,
					governanceService,
					auth.User{ID: cfg.Auth.BootstrapUserID, DisplayName: cfg.Auth.BootstrapDisplayName},
					identity,
				)
				if err != nil {
					break
				}
			}
		}
		if err == nil && rerankConfigured {
			err = knowledge.BootstrapSingleUserSiliconFlowRetrievalProcessing(
				bootstrapCtx,
				knowledgeService,
				governanceService,
				auth.User{ID: cfg.Auth.BootstrapUserID, DisplayName: cfg.Auth.BootstrapDisplayName},
			)
		}
		bootstrapCancel()
		if err != nil {
			_ = redisClient.Close()
			_ = db.Close()
			logger.Error("knowledge_processing_bootstrap_failed", slog.String("error", redactSensitiveLogText(err.Error())))
			os.Exit(1)
		}
	}
	var ragSourceService *ragsource.Service
	if sqlDB != nil {
		ragSourceService = ragsource.NewService(
			ragsource.NewPostgresRepository(sqlDB),
			objectStore,
			ragsource.WithInternalToken(cfg.RAG.SourceGatewayToken),
		)
	}
	if sqlDB := db.SQL(); sqlDB != nil {
		importRepo = browserimport.NewPostgresRepository(
			sqlDB,
			browserimport.WithObjectStore(objectStore),
			browserimport.WithStorageBackend(cfg.Storage.Backend),
		)
	}

	serverOptions := []httpserver.Option{
		httpserver.WithChatRepository(chatRepo),
		httpserver.WithRunCancellationStore(runCancellationStore),
		httpserver.WithRateLimitStore(rateLimitStore),
		httpserver.WithSessionResolver(sessionResolver),
		httpserver.WithAuthService(authService),
		httpserver.WithFileRepository(fileRepo),
		httpserver.WithObjectStore(objectStore),
		httpserver.WithMaxUploadBytes(cfg.Storage.MaxUploadBytes),
		httpserver.WithBrowserImportRepository(importRepo),
		httpserver.WithMaxImportBytes(cfg.Storage.MaxUploadBytes),
		httpserver.WithTeamService(teamRuntime.service),
		httpserver.WithKnowledgeService(knowledgeService),
		httpserver.WithRAGSourceService(ragSourceService),
		httpserver.WithRAGQueryEmbedder(ragProviderGateway),
		httpserver.WithRAGReranker(ragProviderGateway),
		httpserver.WithRuntimeConfigRepository(runtimeConfigRepo),
		httpserver.WithTaskModelSettingsRepository(taskModelRepo),
		httpserver.WithUserMemoryRepository(userMemoryRepo),
		httpserver.WithMemoryPortabilityPlanCodec(teamRuntime.memoryPortability),
		httpserver.WithMemoryWakePublisher(redisClient),
		httpserver.WithProviderSecretVault(providerSecretVault),
		httpserver.WithPluginRegistry(pluginRegistry),
		httpserver.WithPluginAuditRecorder(pluginAuditRecorder),
		httpserver.WithLogger(logger),
	}
	if runtimeConfigRepo != nil {
		serverOptions = append(
			serverOptions,
			httpserver.WithWebSearchResolver(providerRuntimeService),
		)
	}
	if developmentSession != nil {
		serverOptions = append(serverOptions, httpserver.WithDevelopmentSession(*developmentSession))
	}
	if db.SQL() != nil {
		serverOptions = append(serverOptions, httpserver.WithReadyCheck("database", db))
		serverOptions = append(serverOptions, httpserver.WithDatabaseStatsProvider(db.SQL()))
	}
	if redisClient != nil {
		serverOptions = append(serverOptions, httpserver.WithReadyCheck("redis", redisClient))
	}
	if teamRuntime.worker != nil {
		serverOptions = append(
			serverOptions,
			httpserver.WithReadyCheck(
				"team_mail_worker",
				teamWorkerReadiness{gate: teamRuntime.worker},
			),
		)
	}
	if checker, ok := objectStore.(interface {
		CheckReady(context.Context) error
	}); ok {
		serverOptions = append(serverOptions, httpserver.WithReadyCheck("storage", checker))
	}
	imageJobService, err := newImageJobService(
		cfg,
		fileRepo,
		objectStore,
		newJobAuditRecorder(logger),
		runtimeConfigRepo,
		providerSecretVault,
	)
	if err != nil {
		_ = redisClient.Close()
		_ = db.Close()
		logger.Error("image_job_config_failed", slog.String("error", redactSensitiveLogText(err.Error())))
		os.Exit(1)
	}
	serverOptions = append(serverOptions, httpserver.WithImageJobService(imageJobService))
	voiceJobService, err := newVoiceJobService(
		cfg,
		fileRepo,
		objectStore,
		newJobAuditRecorder(logger),
		providerRuntimeService,
		sqlDB,
	)
	if err != nil {
		_ = redisClient.Close()
		_ = db.Close()
		logger.Error("voice_job_config_failed", slog.String("error", redactSensitiveLogText(err.Error())))
		os.Exit(1)
	}
	serverOptions = append(serverOptions, httpserver.WithVoiceJobService(voiceJobService))

	server := httpserver.New(cfg, serverOptions...)

	errorsCh := make(chan runtimeFailure, 3)
	runtimeCtx, cancelRuntime := context.WithCancel(context.Background())
	var teamWorkerDone <-chan struct{}
	if teamRuntime.worker != nil {
		done := make(chan struct{})
		teamWorkerDone = done
		go func() {
			defer close(done)
			if err := runTeamWorker(runtimeCtx, teamRuntime.worker); err != nil {
				logger.Error(
					"team_mail_worker_failed",
					slog.String("error", redactSensitiveLogText(err.Error())),
				)
				select {
				case errorsCh <- runtimeFailure{component: "team_mail_worker", err: err}:
				case <-runtimeCtx.Done():
				}
			}
		}()
	}
	var consentExpiryWorkerDone <-chan struct{}
	if consentExpiryWorker != nil {
		done := make(chan struct{})
		consentExpiryWorkerDone = done
		go func() {
			defer close(done)
			if err := runBackgroundWorker(runtimeCtx, "consent expiry worker", consentExpiryWorker); err != nil {
				logger.Error("consent_expiry_worker_failed", slog.String("error", redactSensitiveLogText(err.Error())))
				select {
				case errorsCh <- runtimeFailure{component: "consent_expiry_worker", err: err}:
				case <-runtimeCtx.Done():
				}
			}
		}()
	}
	voiceCacheWorkerDone := make(chan struct{})
	go func() {
		defer close(voiceCacheWorkerDone)
		runVoiceCacheCleanupWorker(runtimeCtx, voiceJobService, logger)
	}()
	go func() {
		logger.Info("api_listening", slog.String("addr", cfg.Addr), slog.String("version", cfg.Version))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorsCh <- runtimeFailure{component: "api", err: err}
		}
	}()

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	var runtimeErr error
	select {
	case failure := <-errorsCh:
		runtimeErr = failure.err
		logger.Error(
			failure.component+"_runtime_failed",
			slog.String("error", redactSensitiveLogText(failure.err.Error())),
		)
	case sig := <-stopCh:
		logger.Info("api_shutting_down", slog.String("signal", sig.String()))
	}
	cancelRuntime()

	apiShutdownCtx, cancelAPIShutdown := context.WithTimeout(
		context.Background(),
		shutdownTimeout,
	)
	if err := server.Shutdown(apiShutdownCtx); err != nil {
		logger.Error("api_shutdown_failed", slog.String("error", redactSensitiveLogText(err.Error())))
		runtimeErr = errors.Join(runtimeErr, fmt.Errorf("shutdown api: %w", err))
	}
	cancelAPIShutdown()

	workerShutdownCtx, cancelWorkerShutdown := context.WithTimeout(
		context.Background(),
		teamWorkerShutdownTimeout(cfg.Auth.SMTP.Timeout),
	)
	if err := waitForTeamWorker(workerShutdownCtx, teamWorkerDone); err != nil {
		logger.Error("team_mail_worker_shutdown_failed", slog.String("error", err.Error()))
		runtimeErr = errors.Join(runtimeErr, err)
	}
	if err := waitForBackgroundWorker(workerShutdownCtx, consentExpiryWorkerDone, "consent expiry worker"); err != nil {
		logger.Error("consent_expiry_worker_shutdown_failed", slog.String("error", err.Error()))
		runtimeErr = errors.Join(runtimeErr, err)
	}
	if err := waitForBackgroundWorker(workerShutdownCtx, voiceCacheWorkerDone, "voice cache cleanup worker"); err != nil {
		logger.Error("voice_cache_cleanup_worker_shutdown_failed", slog.String("error", err.Error()))
		runtimeErr = errors.Join(runtimeErr, err)
	}
	cancelWorkerShutdown()
	if err := redisClient.Close(); err != nil {
		logger.Warn("redis_close_failed", slog.String("error", redactSensitiveLogText(err.Error())))
	}
	if err := db.Close(); err != nil {
		logger.Warn("database_close_failed", slog.String("error", redactSensitiveLogText(err.Error())))
	}
	if closer, ok := recoveryDelivery.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			logger.Warn("auth_recovery_delivery_close_failed")
		}
	}
	if runtimeErr != nil {
		os.Exit(1)
	}
}

func newProviderSecretVault(cfg config.Config) (*providersecrets.Vault, error) {
	if strings.TrimSpace(cfg.ProviderSecrets.KeyringFile) == "" {
		return nil, nil
	}
	return providersecrets.LoadVaultFile(cfg.ProviderSecrets.KeyringFile)
}

func singleUserAnswerIdentities(
	ctx context.Context,
	repo answerProviderConfigReader,
	ownerUserID string,
) ([]knowledge.ProcessorModelIdentity, error) {
	identities := make([]knowledge.ProcessorModelIdentity, 0)
	appendModels := func(processor string, endpointID string, models []string) {
		processor = strings.TrimSpace(processor)
		endpointID = strings.TrimSpace(endpointID)
		if processor == "" || processor == "none" || endpointID == "" {
			return
		}
		for _, modelID := range models {
			modelID = strings.TrimSpace(modelID)
			if modelID == "" {
				continue
			}
			identities = append(identities, knowledge.ProcessorModelIdentity{
				Processor: processor, EndpointID: endpointID, ModelID: modelID,
			})
		}
	}

	if repo != nil {
		stored, err := repo.ListProviderConfigs(ctx, strings.TrimSpace(ownerUserID))
		if err != nil {
			return nil, fmt.Errorf("list configured answer providers: %w", err)
		}
		for _, provider := range stored {
			if !runtimeconfig.IsModelProviderConfig(provider) ||
				!provider.Config.Enabled ||
				!runtimeconfig.ProviderConnectionTestValid(provider) {
				continue
			}
			providerID := strings.TrimSpace(provider.ProviderID)
			processor := canonicalAnswerProcessor(providerID)
			endpointID := "server-stored"
			if providerID == "SERVER_DEFAULT" {
				providerType := string(provider.Config.Type)
				processor = canonicalAnswerProcessor(providerType)
				endpointID = "server-default"
			}
			appendModels(processor, endpointID, provider.Config.Models)
		}
	}

	sort.Slice(identities, func(i, j int) bool {
		left := identities[i]
		right := identities[j]
		if left.Processor != right.Processor {
			return left.Processor < right.Processor
		}
		if left.EndpointID != right.EndpointID {
			return left.EndpointID < right.EndpointID
		}
		return left.ModelID < right.ModelID
	})
	unique := identities[:0]
	for _, identity := range identities {
		if len(unique) > 0 && unique[len(unique)-1] == identity {
			continue
		}
		unique = append(unique, identity)
	}
	return unique, nil
}

func canonicalAnswerProcessor(providerType string) string {
	return knowledge.CanonicalAnswerProcessor(providerType)
}

func redactSensitiveLogText(value string) string {
	value = sensitiveURLUserInfoRE.ReplaceAllString(value, "${1}[redacted]@")
	value = bearerTokenRE.ReplaceAllString(value, "Bearer [redacted]")
	value = sensitiveAssignmentRE.ReplaceAllString(value, "$1[redacted]")
	return value
}

func newRedisState(
	ctx context.Context,
	cfg config.Config,
) (*redisstate.Client, chat.RunCancellationStore, ratelimit.Store, sessioncache.Store, error) {
	if cfg.Redis.RateLimitEnabled && strings.TrimSpace(cfg.Redis.URL) == "" {
		return nil, nil, nil, nil, fmt.Errorf("%s requires %s", config.EnvRedisRateLimitEnabled, config.EnvRedisURL)
	}

	client, err := redisstate.Open(ctx, cfg.Redis)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if client == nil {
		return nil, nil, nil, nil, nil
	}

	return client,
		client.RunCancellationStore(cfg.Redis.RunCancelTTL),
		client.RateLimitStore(),
		client.SessionCacheStore(cfg.Redis.SessionCacheTTL),
		nil
}

func newObjectStore(cfg config.Config) (storage.ObjectStore, error) {
	storageBackend := strings.ToLower(strings.TrimSpace(cfg.Storage.Backend))
	switch storageBackend {
	case "", "local":
		return storage.NewLocalStore(cfg.Storage.LocalDir)
	case "minio", "s3":
		forcePathStyle := cfg.Storage.S3.ForcePathStyle || storageBackend == "minio"
		store, err := storage.NewS3Store(storage.S3Config{
			Endpoint:        cfg.Storage.S3.Endpoint,
			Bucket:          cfg.Storage.S3.Bucket,
			Region:          cfg.Storage.S3.Region,
			AccessKeyID:     cfg.Storage.S3.AccessKeyID,
			SecretAccessKey: cfg.Storage.S3.SecretAccessKey,
			UseSSL:          cfg.Storage.S3.UseSSL,
			ForcePathStyle:  forcePathStyle,
		})
		if err != nil {
			return nil, err
		}
		if !cfg.Storage.S3.BucketAutoCreate {
			return store, nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), storageOpenTimeout)
		defer cancel()
		if err := store.EnsureBucket(ctx); err != nil {
			return nil, err
		}
		return store, nil
	default:
		return nil, fmt.Errorf("unsupported STORAGE_BACKEND %q", cfg.Storage.Backend)
	}
}

func newJobAuditRecorder(logger *slog.Logger) jobaudit.Recorder {
	if logger == nil {
		logger = slog.Default()
	}
	return jobaudit.RecorderFunc(func(ctx context.Context, event jobaudit.Event) error {
		event = jobaudit.NormalizeEvent(ctx, event)
		logger.InfoContext(
			ctx,
			"job_audit",
			slog.String("kind", string(event.Kind)),
			slog.String("status", string(event.Status)),
			slog.String("user_id", event.UserID),
			slog.String("provider_id", event.ProviderID),
			slog.String("model_id", event.ModelID),
			slog.String("language", event.Language),
			slog.String("reason", event.Reason),
		)
		return nil
	})
}

func newImageJobService(
	cfg config.Config,
	fileRepo files.Repository,
	objectStore storage.ObjectStore,
	auditRecorder jobaudit.Recorder,
	providerConfigRepo runtimeconfig.ProviderConfigRepository,
	providerSecretVault *providersecrets.Vault,
) (*imagejobs.Service, error) {
	options := []imagejobs.ServiceOption{
		imagejobs.WithAuditRecorder(auditRecorder),
		imagejobs.WithExecutorResolver(modelProviderImageExecutorResolver{
			service: runtimeconfig.NewService(
				cfg,
				runtimeconfig.WithProviderConfigRepository(providerConfigRepo),
				runtimeconfig.WithProviderSecretVault(providerSecretVault),
			),
			timeout: cfg.Provider.Timeout,
		}),
	}
	if fileRepo != nil && objectStore != nil {
		options = append(options, imagejobs.WithArtifactStore(jobartifacts.NewService(
			files.NewService(
				fileRepo,
				objectStore,
				files.WithStorageBackend(cfg.Storage.Backend),
			),
		)))
	}

	return imagejobs.NewService(options...), nil
}

type modelProviderImageExecutorResolver struct {
	service imageProviderConfigResolver
	timeout time.Duration
}

type imageProviderConfigResolver interface {
	ResolveServerDefaultProvider(context.Context) (runtimeconfig.ResolvedProvider, error)
	ResolveStoredProvider(context.Context, string) (runtimeconfig.ResolvedProvider, error)
}

func (r modelProviderImageExecutorResolver) ResolveImageExecutor(
	ctx context.Context,
	modelRef imagejobs.ModelRef,
) (imagejobs.Executor, error) {
	if r.service == nil {
		return nil, imagejobs.ErrImageJobsUnavailable
	}
	providerID := strings.TrimSpace(modelRef.ProviderID)
	legacyServerDefaultAlias := isLegacyServerDefaultImageProviderAlias(providerID)
	var provider runtimeconfig.ResolvedProvider
	var err error
	if providerID == "SERVER_DEFAULT" || legacyServerDefaultAlias {
		provider, err = r.service.ResolveServerDefaultProvider(ctx)
	} else {
		provider, err = r.service.ResolveStoredProvider(ctx, providerID)
	}
	if err != nil || (provider.Type != runtimeconfig.ProviderTypeOpenAI &&
		provider.Type != runtimeconfig.ProviderTypeOpenAICompatible) {
		return nil, imagejobs.ErrImageJobsUnavailable
	}
	if legacyServerDefaultAlias && !containsExactModel(provider.Models, modelRef.ModelID) {
		return nil, imagejobs.ErrImageJobsUnavailable
	}
	executor, err := imagejobs.NewOpenAICompatibleExecutor(
		imagejobs.OpenAICompatibleExecutorConfig{
			BaseURL: provider.BaseURL,
			APIKey:  provider.APIKey,
			Timeout: r.timeout,
		},
	)
	if err != nil {
		return nil, imagejobs.ErrImageJobsUnavailable
	}
	return resolvedModelProviderImageExecutor{executor: executor}, nil
}

func isLegacyServerDefaultImageProviderAlias(providerID string) bool {
	switch strings.ToLower(strings.TrimSpace(providerID)) {
	case "openai", "openai_compatible", "openai-compatible":
		return true
	default:
		return false
	}
}

func containsExactModel(models []string, modelID string) bool {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return false
	}
	for _, candidate := range models {
		if strings.TrimSpace(candidate) == modelID {
			return true
		}
	}
	return false
}

type resolvedModelProviderImageExecutor struct {
	executor imagejobs.Executor
}

func (e resolvedModelProviderImageExecutor) Generate(
	ctx context.Context,
	request imagejobs.GenerateRequest,
) (imagejobs.GenerateResult, error) {
	request.ModelRef.ProviderID = "openai_compatible"
	return e.executor.Generate(ctx, request)
}

func newVoiceJobService(
	cfg config.Config,
	fileRepo files.Repository,
	objectStore storage.ObjectStore,
	auditRecorder jobaudit.Recorder,
	providerResolver voiceProviderConfigResolver,
	db *sql.DB,
) (*voicejobs.Service, error) {
	options := []voicejobs.ServiceOption{
		voicejobs.WithAuditRecorder(auditRecorder),
		voicejobs.WithSynthesisExecutorResolver(siliconFlowVoiceExecutorResolver{
			service: providerResolver,
			timeout: cfg.Provider.Timeout,
		}),
	}
	if fileRepo != nil && objectStore != nil {
		fileService := files.NewService(
			fileRepo,
			objectStore,
			files.WithStorageBackend(cfg.Storage.Backend),
		)
		options = append(options, voicejobs.WithArtifactStore(jobartifacts.NewService(
			fileService,
		)), voicejobs.WithArtifactDeleter(fileService))
		if db != nil {
			options = append(options, voicejobs.WithSynthesisCache(
				voicejobs.NewPostgresSynthesisCacheRepository(db),
			))
		}
	}

	return voicejobs.NewService(options...), nil
}

type voiceProviderConfigResolver interface {
	ResolveVoiceProvider(context.Context) (runtimeconfig.ResolvedVoiceProvider, error)
}

type siliconFlowVoiceExecutorResolver struct {
	service voiceProviderConfigResolver
	timeout time.Duration
}

func (r siliconFlowVoiceExecutorResolver) ResolveSynthesisExecutor(
	ctx context.Context,
) (voicejobs.SynthesisExecution, error) {
	if r.service == nil {
		return voicejobs.SynthesisExecution{}, voicejobs.ErrVoiceJobsUnavailable
	}
	provider, err := r.service.ResolveVoiceProvider(ctx)
	if err != nil || provider.ProviderID != "siliconflow" ||
		provider.ModelID != runtimeconfig.SiliconFlowVoiceModelID ||
		provider.VoiceID != runtimeconfig.SiliconFlowVoiceID {
		return voicejobs.SynthesisExecution{}, voicejobs.ErrVoiceJobsUnavailable
	}
	executor, err := voicejobs.NewOpenAICompatibleExecutor(
		voicejobs.OpenAICompatibleExecutorConfig{
			BaseURL:            provider.BaseURL,
			APIKey:             provider.APIKey,
			Timeout:            r.timeout,
			DefaultSpeechModel: provider.ModelID,
			DefaultSpeechVoice: provider.VoiceID,
		},
	)
	provider.APIKey = ""
	if err != nil {
		return voicejobs.SynthesisExecution{}, voicejobs.ErrVoiceJobsUnavailable
	}
	return voicejobs.SynthesisExecution{
		Executor:   executor,
		ProviderID: "siliconflow",
		ModelID:    runtimeconfig.SiliconFlowVoiceModelID,
		VoiceID:    runtimeconfig.SiliconFlowVoiceID,
	}, nil
}

func runVoiceCacheCleanupWorker(
	ctx context.Context,
	service *voicejobs.Service,
	logger *slog.Logger,
) {
	const cleanupInterval = 5 * time.Minute
	runCleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := service.CleanupArtifacts(cleanupCtx, 64); err != nil &&
			!errors.Is(err, context.Canceled) {
			logger.Warn("voice_cache_cleanup_failed", slog.String("error", redactSensitiveLogText(err.Error())))
		}
	}
	runCleanup()
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runCleanup()
		}
	}
}

func newRecoveryDelivery(cfg config.Config) (auth.RecoveryDelivery, error) {
	smtpCfg := cfg.Auth.SMTP
	if smtpConfigBlank(smtpCfg) {
		return nil, nil
	}

	return auth.NewSMTPRecoveryDelivery(auth.SMTPRecoveryConfig{
		Addr:      smtpCfg.Addr,
		Username:  smtpCfg.Username,
		Password:  smtpCfg.Password,
		From:      smtpCfg.From,
		QueueSize: smtpCfg.QueueSize,
		Timeout:   smtpCfg.Timeout,
	})
}

func newTeamRuntime(db *sql.DB, cfg config.Config) (*teamRuntime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	var repo teams.Repository
	if db != nil {
		repo = teams.NewPostgresRepository(db)
	}
	serviceOptions := make([]teams.ServiceOption, 0, 4)

	cursorKeys, cursorCodec, err := newTeamCursorCodec(cfg.Team.Cursor)
	if err != nil {
		return nil, err
	}
	if cursorCodec != nil {
		serviceOptions = append(serviceOptions, teams.WithCursorCodec(cursorCodec))
	}
	var memoryPortabilityCodec *usermemory.PortabilityPlanCodec
	if cursorCodec != nil {
		memoryPortabilityCodec, err = usermemory.NewPortabilityPlanCodec(
			usermemory.PortabilityPlanKeyring{
				ActiveKeyID: cfg.Team.Cursor.ActiveKeyID,
				Keys:        cursorKeys,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("configure memory portability plan codec: %w", err)
		}
	}

	mailKeys, mailCipher, err := newTeamMailCipher(cfg.Team.Mail)
	if err != nil {
		return nil, err
	}
	if keyringsShareMaterial(cursorKeys, mailKeys) {
		return nil, fmt.Errorf(
			"%s and %s must contain distinct key material",
			config.EnvTeamCursorKeyring,
			config.EnvTeamMailKeyring,
		)
	}
	if cfg.Auth.RequireAuth() && keyringUsesPublishedExample(cursorKeys, mailKeys) {
		return nil, fmt.Errorf(
			"%s and %s must not use committed example key material when %s=%s",
			config.EnvTeamCursorKeyring,
			config.EnvTeamMailKeyring,
			config.EnvAuthMode,
			config.AuthModeRequired,
		)
	}
	for _, keyring := range []struct {
		field string
		keys  map[string][]byte
	}{
		{field: config.EnvTeamCursorKeyring, keys: cursorKeys},
		{field: config.EnvTeamMailKeyring, keys: mailKeys},
	} {
		if secretField := keyringReusedSecretField(keyring.keys, cfg); secretField != "" {
			return nil, fmt.Errorf(
				"%s must not reuse %s secret material",
				keyring.field,
				secretField,
			)
		}
	}
	if mailCipher != nil {
		serviceOptions = append(serviceOptions, teams.WithMailCipher(mailCipher))
	}

	inviteURLBuilder, err := newInviteURLBuilder(
		cfg.Team.InviteAcceptURLBase,
		cfg.Auth.RequireAuth(),
	)
	if err != nil {
		return nil, err
	}
	if inviteURLBuilder != nil {
		serviceOptions = append(
			serviceOptions,
			teams.WithInviteURLBuilder(inviteURLBuilder),
		)
	}

	smtpTransport, err := newTeamSMTPTransport(cfg.Auth.SMTP)
	if err != nil {
		return nil, err
	}

	runtime := &teamRuntime{
		cursor: cursorCodec, memoryPortability: memoryPortabilityCodec,
	}
	if db != nil && mailCipher != nil && smtpTransport != nil &&
		inviteURLBuilder != nil {
		workerConfig := normalizedTeamMailWorkerConfig(cfg.Team.MailWorker)
		runtime.worker, err = teams.NewInviteMailOutboxWorker(
			db,
			mailCipher,
			smtpTransport,
			teams.WithInviteMailWorkerLeaseDuration(workerConfig.LeaseDuration),
			teams.WithInviteMailWorkerPollInterval(workerConfig.PollInterval),
			teams.WithInviteMailWorkerBackoff(
				workerConfig.BackoffBase,
				workerConfig.BackoffMaximum,
			),
		)
		if err != nil {
			return nil, fmt.Errorf("configure team mail worker: %w", err)
		}
		serviceOptions = append(
			serviceOptions,
			teams.WithInviteDeliveryGate(runtime.worker),
		)
	}

	runtime.service = teams.NewService(repo, serviceOptions...)
	return runtime, nil
}

func newTeamCursorCodec(
	cfg config.TeamKeyringConfig,
) (map[string][]byte, *teams.CursorCodec, error) {
	if strings.TrimSpace(cfg.ActiveKeyID) == "" &&
		strings.TrimSpace(cfg.Keyring) == "" {
		return nil, nil, nil
	}
	keys, err := config.ParseBase64Keyring(
		config.EnvTeamCursorKeyring,
		cfg.Keyring,
	)
	if err != nil {
		return nil, nil, err
	}
	codec, err := teams.NewCursorCodec(teams.CursorKeyring{
		ActiveKeyID: cfg.ActiveKeyID,
		Keys:        keys,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("configure %s: %w", config.EnvTeamCursorKeyring, err)
	}
	return keys, codec, nil
}

func newTeamMailCipher(
	cfg config.TeamKeyringConfig,
) (map[string][]byte, *teams.MailCipher, error) {
	if strings.TrimSpace(cfg.ActiveKeyID) == "" &&
		strings.TrimSpace(cfg.Keyring) == "" {
		return nil, nil, nil
	}
	keys, err := config.ParseBase64Keyring(
		config.EnvTeamMailKeyring,
		cfg.Keyring,
	)
	if err != nil {
		return nil, nil, err
	}
	cipher, err := teams.NewMailCipher(teams.MailKeyring{
		ActiveKeyID: cfg.ActiveKeyID,
		Keys:        keys,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("configure %s: %w", config.EnvTeamMailKeyring, err)
	}
	return keys, cipher, nil
}

func keyringsShareMaterial(left map[string][]byte, right map[string][]byte) bool {
	for _, leftKey := range left {
		for _, rightKey := range right {
			if bytes.Equal(leftKey, rightKey) {
				return true
			}
		}
	}
	return false
}

func keyringUsesPublishedExample(keyrings ...map[string][]byte) bool {
	for _, keyring := range keyrings {
		for _, key := range keyring {
			for _, example := range publishedExampleTeamKeys {
				if bytes.Equal(key, example) {
					return true
				}
			}
		}
	}
	return false
}

func keyringReusedSecretField(keys map[string][]byte, cfg config.Config) string {
	for _, secret := range []struct {
		field string
		value string
	}{
		{field: config.EnvDatabaseURL, value: urlCredentialPassword(cfg.DatabaseURL)},
		{field: config.EnvRedisURL, value: urlCredentialPassword(cfg.Redis.URL)},
		{field: config.EnvAuthSMTPPassword, value: cfg.Auth.SMTP.Password},
		{field: config.EnvS3SecretAccessKey, value: cfg.Storage.S3.SecretAccessKey},
	} {
		if secret.value == "" {
			continue
		}
		for _, key := range keys {
			if bytes.Equal(key, []byte(secret.value)) {
				return secret.field
			}
		}
	}
	return ""
}

func urlCredentialPassword(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil || parsed.User == nil {
		return ""
	}
	password, ok := parsed.User.Password()
	if !ok {
		return ""
	}
	return password
}

func newTeamSMTPTransport(
	cfg config.SMTPRecoveryConfig,
) (*auth.SMTPSyncTransport, error) {
	if smtpConfigBlank(cfg) {
		return nil, nil
	}
	transport, err := auth.NewSMTPSyncTransport(auth.SMTPTransportConfig{
		Addr:     cfg.Addr,
		Username: cfg.Username,
		Password: cfg.Password,
		From:     cfg.From,
		Timeout:  cfg.Timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("configure team smtp transport: %w", err)
	}
	return transport, nil
}

func smtpConfigBlank(cfg config.SMTPRecoveryConfig) bool {
	return strings.TrimSpace(cfg.Addr) == "" &&
		strings.TrimSpace(cfg.Username) == "" &&
		cfg.Password == "" &&
		strings.TrimSpace(cfg.From) == ""
}

func newInviteURLBuilder(
	value string,
	requireHTTPS bool,
) (func(string) (string, error), error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := url.Parse(value)
	scheme := ""
	if parsed != nil {
		scheme = strings.ToLower(parsed.Scheme)
	}
	if err != nil || parsed == nil || !parsed.IsAbs() || parsed.Host == "" ||
		(scheme != "https" && scheme != "http") ||
		parsed.User != nil || parsed.Fragment != "" {
		return nil, fmt.Errorf(
			"%s must be an absolute HTTP(S) URL without user info or fragment",
			config.EnvTeamInviteAcceptURL,
		)
	}
	if scheme == "http" && (requireHTTPS || !isLoopbackHostname(parsed.Hostname())) {
		return nil, fmt.Errorf(
			"%s must use HTTPS outside loopback development",
			config.EnvTeamInviteAcceptURL,
		)
	}
	parsed.Scheme = scheme
	for key := range parsed.Query() {
		if strings.EqualFold(strings.TrimSpace(key), "token") {
			return nil, fmt.Errorf(
				"%s must not contain a token query parameter",
				config.EnvTeamInviteAcceptURL,
			)
		}
	}
	base := *parsed
	return func(token string) (string, error) {
		token, err := teams.NormalizeInviteToken(token)
		if err != nil {
			return "", errors.New("invite token is invalid")
		}
		result := base
		fragment := url.Values{}
		fragment.Set("token", token)
		result.Fragment = fragment.Encode()
		result.RawFragment = ""
		return result.String(), nil
	}, nil
}

func isLoopbackHostname(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func normalizedTeamMailWorkerConfig(
	cfg config.TeamMailWorkerConfig,
) config.TeamMailWorkerConfig {
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = config.DefaultTeamMailWorkerLease
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = config.DefaultTeamMailWorkerPoll
	}
	if cfg.BackoffBase <= 0 {
		cfg.BackoffBase = config.DefaultTeamMailBackoffBase
	}
	if cfg.BackoffMaximum <= 0 {
		cfg.BackoffMaximum = config.DefaultTeamMailBackoffMax
	}
	return cfg
}

func runTeamWorker(ctx context.Context, worker teamWorker) error {
	if worker == nil {
		return nil
	}
	err := worker.Run(ctx)
	if ctx.Err() != nil &&
		(err == nil || errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded)) {
		return nil
	}
	if err == nil {
		return errors.New("team mail worker exited unexpectedly")
	}
	return fmt.Errorf("team mail worker stopped: %w", err)
}

func runBackgroundWorker(ctx context.Context, name string, worker teamWorker) error {
	if worker == nil {
		return nil
	}
	err := worker.Run(ctx)
	if ctx.Err() != nil && (err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("%s exited unexpectedly", name)
	}
	return fmt.Errorf("%s stopped: %w", name, err)
}

func waitForBackgroundWorker(ctx context.Context, done <-chan struct{}, name string) error {
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for %s", name)
	}
}

func waitForTeamWorker(ctx context.Context, done <-chan struct{}) error {
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return errors.New("timed out waiting for team mail worker")
	}
}

func teamWorkerShutdownTimeout(smtpTimeout time.Duration) time.Duration {
	workerTimeout := smtpTimeout + time.Second
	if workerTimeout > shutdownTimeout {
		return workerTimeout
	}
	return shutdownTimeout
}
