package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
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
	"neo-chat/mm-chat/backend/internal/ratelimit"
	"neo-chat/mm-chat/backend/internal/redisstate"
	"neo-chat/mm-chat/backend/internal/sessioncache"
	"neo-chat/mm-chat/backend/internal/storage"
)

const (
	databaseOpenTimeout = 5 * time.Second
	redisOpenTimeout    = 5 * time.Second
	storageOpenTimeout  = 10 * time.Second
	shutdownTimeout     = 10 * time.Second
)

var (
	sensitiveURLUserInfoRE = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^\s]*@`)
	sensitiveAssignmentRE  = regexp.MustCompile(`(?i)([A-Za-z0-9_.-]*(?:api[_-]?key|authorization|password|secret|token)[A-Za-z0-9_.-]*\s*[=:]\s*)([^\s&]+)`)
	bearerTokenRE          = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]+`)
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	cfg := config.Load()

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
	var sessionResolver httpserver.SessionResolver
	var authService *auth.Service
	if sqlDB := db.SQL(); sqlDB != nil {
		authRepo := auth.NewPostgresSessionRepository(sqlDB)
		chatRepo = chat.NewPostgresRepository(sqlDB)
		fileRepo = files.NewPostgresRepository(sqlDB)
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

	chatProvider, err := newChatProvider(cfg)
	if err != nil {
		_ = redisClient.Close()
		_ = db.Close()
		logger.Error("provider_config_failed", slog.String("error", redactSensitiveLogText(err.Error())))
		os.Exit(1)
	}
	if chatProvider == nil && strings.TrimSpace(cfg.Provider.Type) != "" {
		logger.Warn("provider_disabled", slog.String("provider_type", cfg.Provider.Type))
	}

	objectStore, err := newObjectStore(cfg)
	if err != nil {
		_ = redisClient.Close()
		_ = db.Close()
		logger.Error("storage_config_failed", slog.String("error", redactSensitiveLogText(err.Error())))
		os.Exit(1)
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
		httpserver.WithChatProvider(chatProvider),
		httpserver.WithRunCancellationStore(runCancellationStore),
		httpserver.WithRateLimitStore(rateLimitStore),
		httpserver.WithSessionResolver(sessionResolver),
		httpserver.WithAuthService(authService),
		httpserver.WithFileRepository(fileRepo),
		httpserver.WithObjectStore(objectStore),
		httpserver.WithMaxUploadBytes(cfg.Storage.MaxUploadBytes),
		httpserver.WithBrowserImportRepository(importRepo),
		httpserver.WithMaxImportBytes(cfg.Storage.MaxUploadBytes),
		httpserver.WithLogger(logger),
	}
	if db.SQL() != nil {
		serverOptions = append(serverOptions, httpserver.WithReadyCheck("database", db))
		serverOptions = append(serverOptions, httpserver.WithDatabaseStatsProvider(db.SQL()))
	}
	if redisClient != nil {
		serverOptions = append(serverOptions, httpserver.WithReadyCheck("redis", redisClient))
	}
	if checker, ok := objectStore.(interface {
		CheckReady(context.Context) error
	}); ok {
		serverOptions = append(serverOptions, httpserver.WithReadyCheck("storage", checker))
	}

	server := httpserver.New(cfg, serverOptions...)

	errorsCh := make(chan error, 1)
	go func() {
		logger.Info("api_listening", slog.String("addr", cfg.Addr), slog.String("version", cfg.Version))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorsCh <- err
		}
	}()

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errorsCh:
		_ = redisClient.Close()
		_ = db.Close()
		logger.Error("api_failed", slog.String("error", redactSensitiveLogText(err.Error())))
		os.Exit(1)
	case sig := <-stopCh:
		logger.Info("api_shutting_down", slog.String("signal", sig.String()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		_ = redisClient.Close()
		_ = db.Close()
		logger.Error("api_shutdown_failed", slog.String("error", redactSensitiveLogText(err.Error())))
		os.Exit(1)
	}
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

func newRecoveryDelivery(cfg config.Config) (auth.RecoveryDelivery, error) {
	smtpCfg := cfg.Auth.SMTP
	if strings.TrimSpace(smtpCfg.Addr) == "" &&
		strings.TrimSpace(smtpCfg.Username) == "" &&
		smtpCfg.Password == "" &&
		strings.TrimSpace(smtpCfg.From) == "" {
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

func newChatProvider(cfg config.Config) (chat.Provider, error) {
	providerType := strings.ToLower(strings.TrimSpace(cfg.Provider.Type))
	switch providerType {
	case "", "none":
		return nil, nil
	case "openai", "openai_compatible", "openai-compatible":
		if cfg.Provider.BaseURL == "" || cfg.Provider.Model == "" || cfg.Provider.APIKey == "" {
			return nil, nil
		}

		return chat.NewOpenAICompatibleProvider(chat.OpenAICompatibleProviderConfig{
			BaseURL:      cfg.Provider.BaseURL,
			APIKey:       cfg.Provider.APIKey,
			DefaultModel: cfg.Provider.Model,
			Timeout:      cfg.Provider.Timeout,
		})
	default:
		return nil, fmt.Errorf("unsupported PROVIDER_TYPE %q", cfg.Provider.Type)
	}
}
