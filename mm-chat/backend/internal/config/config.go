package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultAddr              = ":8080"
	DefaultVersion           = "dev"
	DefaultDBMaxOpenConns    = 10
	DefaultDBMaxIdleConns    = 5
	DefaultDBConnMaxLifetime = 30 * time.Minute
	DefaultRedisKeyPrefix    = "mm-chat"
	DefaultRedisRunCancelTTL = 10 * time.Minute
	DefaultProviderTimeout   = 2 * time.Minute
	DefaultStorageBackend    = "local"
	DefaultLocalStorageDir   = "./data/files"
	DefaultS3Region          = "us-east-1"
	DefaultMaxUploadBytes    = int64(25 << 20)

	EnvAddr               = "MM_CHAT_ADDR"
	EnvVersion            = "MM_CHAT_VERSION"
	EnvDatabaseURL        = "DATABASE_URL"
	EnvDBMaxOpenConns     = "DB_MAX_OPEN_CONNS"
	EnvDBMaxIdleConns     = "DB_MAX_IDLE_CONNS"
	EnvDBConnMaxLifetime  = "DB_CONN_MAX_LIFETIME"
	EnvRedisURL           = "REDIS_URL"
	EnvRedisKeyPrefix     = "REDIS_KEY_PREFIX"
	EnvRedisRunCancelTTL  = "REDIS_RUN_CANCEL_TTL"
	EnvProviderType       = "PROVIDER_TYPE"
	EnvProviderBaseURL    = "PROVIDER_BASE_URL"
	EnvProviderModel      = "PROVIDER_MODEL"
	EnvProviderAPIKey     = "PROVIDER_API_KEY"
	EnvProviderTimeout    = "PROVIDER_TIMEOUT"
	EnvStorageBackend     = "STORAGE_BACKEND"
	EnvLocalStorageDir    = "LOCAL_STORAGE_DIR"
	EnvS3Endpoint         = "S3_ENDPOINT"
	EnvS3Bucket           = "S3_BUCKET"
	EnvS3Region           = "S3_REGION"
	EnvS3AccessKeyID      = "S3_ACCESS_KEY_ID"
	EnvS3SecretAccessKey  = "S3_SECRET_ACCESS_KEY"
	EnvS3UseSSL           = "S3_USE_SSL"
	EnvS3ForcePathStyle   = "S3_FORCE_PATH_STYLE"
	EnvS3BucketAutoCreate = "S3_BUCKET_AUTO_CREATE"
	EnvMaxUploadBytes     = "MAX_UPLOAD_BYTES"
)

// Config contains the process-level settings required to start the API.
type Config struct {
	Addr        string
	Version     string
	DatabaseURL string

	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration

	Redis RedisConfig

	Provider ProviderConfig
	Storage  StorageConfig
}

// RedisConfig contains non-authoritative temporary-state settings. Redis must
// not store canonical conversations, messages, files, or provider secrets.
type RedisConfig struct {
	URL          string
	KeyPrefix    string
	RunCancelTTL time.Duration
}

// ProviderConfig contains outbound model-provider settings. Secrets must never
// be logged or serialized into API responses.
type ProviderConfig struct {
	Type    string
	BaseURL string
	Model   string
	APIKey  string
	Timeout time.Duration
}

// StorageConfig contains file-byte storage settings. Object-store secrets must
// never be logged or serialized into API responses.
type StorageConfig struct {
	Backend        string
	LocalDir       string
	S3             S3Config
	MaxUploadBytes int64
}

// S3Config contains MinIO/S3-compatible object storage settings.
type S3Config struct {
	Endpoint         string
	Bucket           string
	Region           string
	AccessKeyID      string
	SecretAccessKey  string
	UseSSL           bool
	ForcePathStyle   bool
	BucketAutoCreate bool
}

// Load reads configuration from the process environment.
func Load() Config {
	return LoadFromEnv(os.LookupEnv)
}

// LoadFromEnv reads configuration from the supplied lookup function. Empty or
// whitespace-only values fall back to defaults so a partially configured
// environment still starts with safe development settings.
func LoadFromEnv(lookup func(string) (string, bool)) Config {
	return Config{
		Addr:        envOrDefault(lookup, EnvAddr, DefaultAddr),
		Version:     envOrDefault(lookup, EnvVersion, DefaultVersion),
		DatabaseURL: optionalEnv(lookup, EnvDatabaseURL),

		DBMaxOpenConns:    intEnvOrDefault(lookup, EnvDBMaxOpenConns, DefaultDBMaxOpenConns),
		DBMaxIdleConns:    intEnvOrDefault(lookup, EnvDBMaxIdleConns, DefaultDBMaxIdleConns),
		DBConnMaxLifetime: durationEnvOrDefault(lookup, EnvDBConnMaxLifetime, DefaultDBConnMaxLifetime),

		Redis: RedisConfig{
			URL:          optionalEnv(lookup, EnvRedisURL),
			KeyPrefix:    envOrDefault(lookup, EnvRedisKeyPrefix, DefaultRedisKeyPrefix),
			RunCancelTTL: durationEnvOrDefault(lookup, EnvRedisRunCancelTTL, DefaultRedisRunCancelTTL),
		},

		Provider: ProviderConfig{
			Type:    optionalEnv(lookup, EnvProviderType),
			BaseURL: optionalEnv(lookup, EnvProviderBaseURL),
			Model:   optionalEnv(lookup, EnvProviderModel),
			APIKey:  optionalEnv(lookup, EnvProviderAPIKey),
			Timeout: durationEnvOrDefault(lookup, EnvProviderTimeout, DefaultProviderTimeout),
		},

		Storage: StorageConfig{
			Backend:  strings.ToLower(envOrDefault(lookup, EnvStorageBackend, DefaultStorageBackend)),
			LocalDir: envOrDefault(lookup, EnvLocalStorageDir, DefaultLocalStorageDir),
			S3: S3Config{
				Endpoint:         optionalEnv(lookup, EnvS3Endpoint),
				Bucket:           optionalEnv(lookup, EnvS3Bucket),
				Region:           envOrDefault(lookup, EnvS3Region, DefaultS3Region),
				AccessKeyID:      optionalEnv(lookup, EnvS3AccessKeyID),
				SecretAccessKey:  optionalEnv(lookup, EnvS3SecretAccessKey),
				UseSSL:           boolEnvOrDefault(lookup, EnvS3UseSSL, false),
				ForcePathStyle:   boolEnvOrDefault(lookup, EnvS3ForcePathStyle, false),
				BucketAutoCreate: boolEnvOrDefault(lookup, EnvS3BucketAutoCreate, false),
			},
			MaxUploadBytes: int64EnvOrDefault(lookup, EnvMaxUploadBytes, DefaultMaxUploadBytes),
		},
	}
}

func envOrDefault(lookup func(string) (string, bool), key string, fallback string) string {
	value, ok := optionalLookup(lookup, key)
	if !ok {
		return fallback
	}

	return value
}

func optionalEnv(lookup func(string) (string, bool), key string) string {
	value, ok := optionalLookup(lookup, key)
	if !ok {
		return ""
	}

	return value
}

func intEnvOrDefault(lookup func(string) (string, bool), key string, fallback int) int {
	value, ok := optionalLookup(lookup, key)
	if !ok {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}

	return parsed
}

func int64EnvOrDefault(lookup func(string) (string, bool), key string, fallback int64) int64 {
	value, ok := optionalLookup(lookup, key)
	if !ok {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return fallback
	}

	return parsed
}

func boolEnvOrDefault(lookup func(string) (string, bool), key string, fallback bool) bool {
	value, ok := optionalLookup(lookup, key)
	if !ok {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func durationEnvOrDefault(
	lookup func(string) (string, bool),
	key string,
	fallback time.Duration,
) time.Duration {
	value, ok := optionalLookup(lookup, key)
	if !ok {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < 0 {
		return fallback
	}

	return parsed
}

func optionalLookup(lookup func(string) (string, bool), key string) (string, bool) {
	value, ok := lookup(key)
	if !ok {
		return "", false
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}

	return value, true
}
