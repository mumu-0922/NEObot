package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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

func main() {
	cfg := config.Load()

	openCtx, openCancel := context.WithTimeout(context.Background(), databaseOpenTimeout)
	db, err := database.Open(openCtx, cfg)
	openCancel()
	if err != nil {
		log.Fatalf("mm-chat database open failed: %v", err)
	}

	redisCtx, redisCancel := context.WithTimeout(context.Background(), redisOpenTimeout)
	redisClient, runCancellationStore, rateLimitStore, sessionCache, err := newRedisState(redisCtx, cfg)
	redisCancel()
	if err != nil {
		_ = db.Close()
		log.Fatalf("mm-chat redis open failed: %v", err)
	}

	var chatRepo chat.Repository
	var fileRepo files.Repository
	var importRepo browserimport.Repository
	var sessionResolver httpserver.SessionResolver
	if sqlDB := db.SQL(); sqlDB != nil {
		chatRepo = chat.NewPostgresRepository(sqlDB)
		fileRepo = files.NewPostgresRepository(sqlDB)
		sessionResolver = auth.NewSessionResolver(
			auth.NewPostgresSessionRepository(sqlDB),
			auth.WithSessionCache(sessionCache),
		)
	}

	chatProvider, err := newChatProvider(cfg)
	if err != nil {
		_ = redisClient.Close()
		_ = db.Close()
		log.Fatalf("mm-chat provider config failed: %v", err)
	}
	if chatProvider == nil && strings.TrimSpace(cfg.Provider.Type) != "" {
		log.Printf("mm-chat provider disabled: %s requires PROVIDER_BASE_URL, PROVIDER_MODEL, and PROVIDER_API_KEY", cfg.Provider.Type)
	}

	objectStore, err := newObjectStore(cfg)
	if err != nil {
		_ = redisClient.Close()
		_ = db.Close()
		log.Fatalf("mm-chat storage config failed: %v", err)
	}
	if sqlDB := db.SQL(); sqlDB != nil {
		importRepo = browserimport.NewPostgresRepository(
			sqlDB,
			browserimport.WithObjectStore(objectStore),
			browserimport.WithStorageBackend(cfg.Storage.Backend),
		)
	}

	server := httpserver.New(
		cfg,
		httpserver.WithReadyChecker(db),
		httpserver.WithChatRepository(chatRepo),
		httpserver.WithChatProvider(chatProvider),
		httpserver.WithRunCancellationStore(runCancellationStore),
		httpserver.WithRateLimitStore(rateLimitStore),
		httpserver.WithSessionResolver(sessionResolver),
		httpserver.WithFileRepository(fileRepo),
		httpserver.WithObjectStore(objectStore),
		httpserver.WithMaxUploadBytes(cfg.Storage.MaxUploadBytes),
		httpserver.WithBrowserImportRepository(importRepo),
		httpserver.WithMaxImportBytes(cfg.Storage.MaxUploadBytes),
	)

	errorsCh := make(chan error, 1)
	go func() {
		log.Printf("mm-chat api listening on %s version=%s", cfg.Addr, cfg.Version)
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
		log.Fatalf("mm-chat api failed: %v", err)
	case sig := <-stopCh:
		log.Printf("mm-chat api shutting down: signal=%s", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		_ = redisClient.Close()
		_ = db.Close()
		log.Fatalf("mm-chat api shutdown failed: %v", err)
	}
	if err := redisClient.Close(); err != nil {
		log.Printf("mm-chat redis close failed: %v", err)
	}
	if err := db.Close(); err != nil {
		log.Printf("mm-chat database close failed: %v", err)
	}
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
