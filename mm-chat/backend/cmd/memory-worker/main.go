package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/database"
	"neo-chat/mm-chat/backend/internal/memoryworker"
	"neo-chat/mm-chat/backend/internal/providersecrets"
	"neo-chat/mm-chat/backend/internal/redisstate"
	"neo-chat/mm-chat/backend/internal/runtimeconfig"
)

const (
	envDatabaseURL      = "MEMORY_WORKER_DATABASE_URL"
	envMaxOpenConns     = "MEMORY_WORKER_MAX_OPEN_CONNS"
	envMaxIdleConns     = "MEMORY_WORKER_MAX_IDLE_CONNS"
	envConnMaxLifetime  = "MEMORY_WORKER_CONN_MAX_LIFETIME"
	envConcurrency      = "MEMORY_WORKER_CONCURRENCY"
	envLeaseDuration    = "MEMORY_WORKER_LEASE_DURATION"
	envPollInterval     = "MEMORY_WORKER_POLL_INTERVAL"
	envBackoffBase      = "MEMORY_WORKER_BACKOFF_BASE"
	envBackoffMax       = "MEMORY_WORKER_BACKOFF_MAX"
	envProviderTimeout  = "PROVIDER_TIMEOUT"
	envProviderKeyring  = "PROVIDER_SECRET_KEYRING_FILE"
	envRedisURL         = "REDIS_URL"
	envRedisKeyPrefix   = "REDIS_KEY_PREFIX"
	envHybridShadow     = config.EnvMemoryHybridShadow
	envL2SceneShadow    = config.EnvMemoryL2SceneShadow
	databaseOpenTimeout = 10 * time.Second
	redisOpenTimeout    = 2 * time.Second
	healthcheckTimeout  = 10 * time.Second
)

type workerConfig struct {
	databaseURL          string
	maxOpenConns         int
	maxIdleConns         int
	connMaxLifetime      time.Duration
	concurrency          int
	leaseDuration        time.Duration
	pollInterval         time.Duration
	backoffBase          time.Duration
	backoffMax           time.Duration
	providerTimeout      time.Duration
	providerKeyring      string
	redisURL             string
	redisKeyPrefix       string
	hybridShadowEnabled  bool
	l2SceneShadowEnabled bool
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(os.Args[1:], os.LookupEnv, logger); err != nil {
		logger.Error("memory_worker_exit", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(
	args []string,
	lookup func(string) (string, bool),
	logger *slog.Logger,
) error {
	if len(args) != 1 || (args[0] != "run" && args[0] != "healthcheck") {
		return errors.New("usage: memory-worker run | memory-worker healthcheck")
	}
	resolved, err := loadWorkerConfig(lookup)
	if err != nil {
		return err
	}
	if logger == nil {
		logger = slog.Default()
	}
	openCtx, cancelOpen := context.WithTimeout(context.Background(), databaseOpenTimeout)
	db, err := database.Open(openCtx, config.Config{
		DatabaseURL:       resolved.databaseURL,
		DBMaxOpenConns:    resolved.maxOpenConns,
		DBMaxIdleConns:    resolved.maxIdleConns,
		DBConnMaxLifetime: resolved.connMaxLifetime,
	})
	cancelOpen()
	if err != nil || db == nil || db.SQL() == nil {
		return errors.New("open memory worker database failed")
	}
	defer db.Close()
	repository := memoryworker.NewPostgresRepository(db.SQL())

	vault, err := providersecrets.LoadVaultFile(resolved.providerKeyring)
	if err != nil {
		return errors.New("load memory worker provider keyring failed")
	}
	if args[0] == "healthcheck" {
		ctx, cancel := context.WithTimeout(context.Background(), healthcheckTimeout)
		defer cancel()
		readiness, err := repository.CheckReady(ctx)
		if err != nil || !readiness.ConsumerReady {
			return errors.New("memory worker readiness failed")
		}
		return nil
	}

	runtimeService := runtimeconfig.NewService(
		config.Config{},
		runtimeconfig.WithProviderSecretVault(vault),
	)
	worker, err := memoryworker.New(
		repository,
		memoryworker.NewStoredProviderResolver(runtimeService, resolved.providerTimeout),
		memoryworker.WithEmbeddingProvider(
			memoryworker.NewStoredRAGEmbeddingProvider(runtimeService),
		),
		memoryworker.WithEmbeddingEnabled(resolved.hybridShadowEnabled),
		memoryworker.WithSceneShadowEnabled(resolved.l2SceneShadowEnabled),
		memoryworker.WithLeaseDuration(resolved.leaseDuration),
		memoryworker.WithProviderTimeout(resolved.providerTimeout),
		memoryworker.WithPollInterval(resolved.pollInterval),
		memoryworker.WithBackoff(resolved.backoffBase, resolved.backoffMax),
		memoryworker.WithConcurrency(resolved.concurrency),
		memoryworker.WithLogger(logger),
	)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	wake := make(chan struct{}, 1)
	redisClient, subscription := openOptionalRedisWake(ctx, resolved, logger, wake)
	if subscription != nil {
		defer subscription.Close()
	}
	if redisClient != nil {
		defer redisClient.Close()
	}
	logger.Info("memory_worker_started", slog.Int("concurrency", resolved.concurrency))
	if err := worker.Run(ctx, wake); err != nil {
		return err
	}
	logger.Info("memory_worker_stopped")
	return nil
}

func openOptionalRedisWake(
	ctx context.Context,
	resolved workerConfig,
	logger *slog.Logger,
	wake chan<- struct{},
) (*redisstate.Client, *redisstate.MemoryWakeSubscription) {
	if strings.TrimSpace(resolved.redisURL) == "" {
		return nil, nil
	}
	openCtx, cancel := context.WithTimeout(ctx, redisOpenTimeout)
	defer cancel()
	client, err := redisstate.Open(openCtx, config.RedisConfig{
		URL: resolved.redisURL, KeyPrefix: resolved.redisKeyPrefix,
	})
	if err != nil {
		logger.Warn("memory_worker_redis_unavailable")
		return nil, nil
	}
	subscription, err := client.SubscribeMemoryWake(ctx)
	if err != nil {
		logger.Warn("memory_worker_redis_subscribe_unavailable")
		_ = client.Close()
		return nil, nil
	}
	go func() {
		for range subscription.C() {
			select {
			case wake <- struct{}{}:
			default:
			}
		}
	}()
	return client, subscription
}

func loadWorkerConfig(lookup func(string) (string, bool)) (workerConfig, error) {
	databaseURL := env(lookup, envDatabaseURL, "")
	keyring := env(lookup, envProviderKeyring, "")
	if databaseURL == "" {
		return workerConfig{}, errors.New("MEMORY_WORKER_DATABASE_URL is required")
	}
	if keyring == "" {
		return workerConfig{}, errors.New("PROVIDER_SECRET_KEYRING_FILE is required")
	}
	maxOpen, err := intSetting(lookup, envMaxOpenConns, 4, 1, 32)
	if err != nil {
		return workerConfig{}, err
	}
	maxIdle, err := intSetting(lookup, envMaxIdleConns, 2, 0, maxOpen)
	if err != nil {
		return workerConfig{}, err
	}
	concurrency, err := intSetting(lookup, envConcurrency, 2, 1, 32)
	if err != nil {
		return workerConfig{}, err
	}
	connLifetime, err := durationSetting(lookup, envConnMaxLifetime, 30*time.Minute, time.Minute, 24*time.Hour)
	if err != nil {
		return workerConfig{}, err
	}
	lease, err := durationSetting(lookup, envLeaseDuration, 2*time.Minute, 10*time.Second, 15*time.Minute)
	if err != nil {
		return workerConfig{}, err
	}
	poll, err := durationSetting(lookup, envPollInterval, time.Second, 100*time.Millisecond, time.Minute)
	if err != nil {
		return workerConfig{}, err
	}
	base, err := durationSetting(lookup, envBackoffBase, 5*time.Second, time.Second, time.Hour)
	if err != nil {
		return workerConfig{}, err
	}
	maximum, err := durationSetting(lookup, envBackoffMax, 15*time.Minute, base, 24*time.Hour)
	if err != nil {
		return workerConfig{}, err
	}
	providerTimeout, err := durationSetting(lookup, envProviderTimeout, 45*time.Second, time.Second, 10*time.Minute)
	if err != nil {
		return workerConfig{}, err
	}
	if providerTimeout+5*time.Second >= lease {
		return workerConfig{}, errors.New("PROVIDER_TIMEOUT must be at least 5s below MEMORY_WORKER_LEASE_DURATION")
	}
	hybridShadowEnabled, err := boolSetting(lookup, envHybridShadow, false)
	if err != nil {
		return workerConfig{}, err
	}
	l2SceneShadowEnabled, err := boolSetting(lookup, envL2SceneShadow, false)
	if err != nil {
		return workerConfig{}, err
	}
	return workerConfig{
		databaseURL: databaseURL, maxOpenConns: maxOpen, maxIdleConns: maxIdle,
		connMaxLifetime: connLifetime, concurrency: concurrency,
		leaseDuration: lease, pollInterval: poll, backoffBase: base,
		backoffMax: maximum, providerTimeout: providerTimeout,
		providerKeyring: keyring, redisURL: env(lookup, envRedisURL, ""),
		redisKeyPrefix:       env(lookup, envRedisKeyPrefix, config.DefaultRedisKeyPrefix),
		hybridShadowEnabled:  hybridShadowEnabled,
		l2SceneShadowEnabled: l2SceneShadowEnabled,
	}, nil
}

func boolSetting(
	lookup func(string) (string, bool),
	name string,
	fallback bool,
) (bool, error) {
	value := env(lookup, name, "")
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s is invalid", name)
	}
	return parsed, nil
}

func env(lookup func(string) (string, bool), name string, fallback string) string {
	if value, ok := lookup(name); ok {
		return strings.TrimSpace(value)
	}
	return fallback
}

func intSetting(
	lookup func(string) (string, bool),
	name string,
	fallback int,
	minimum int,
	maximum int,
) (int, error) {
	value := env(lookup, name, "")
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s is invalid", name)
	}
	return parsed, nil
}

func durationSetting(
	lookup func(string) (string, bool),
	name string,
	fallback time.Duration,
	minimum time.Duration,
	maximum time.Duration,
) (time.Duration, error) {
	value := env(lookup, name, "")
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s is invalid", name)
	}
	return parsed, nil
}
