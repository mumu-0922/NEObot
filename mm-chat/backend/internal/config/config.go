package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultAddr                   = ":8080"
	DefaultVersion                = "dev"
	DefaultDBMaxOpenConns         = 10
	DefaultDBMaxIdleConns         = 5
	DefaultDBConnMaxLifetime      = 30 * time.Minute
	DefaultRedisKeyPrefix         = "mm-chat"
	DefaultRedisRunCancelTTL      = 10 * time.Minute
	DefaultRedisSessionCacheTTL   = 5 * time.Minute
	DefaultRedisRateLimitEnabled  = false
	DefaultRedisRateLimitRequests = 120
	DefaultRedisRateLimitWindow   = time.Minute
	DefaultProviderTimeout        = 2 * time.Minute
	DefaultStorageBackend         = "local"
	DefaultLocalStorageDir        = "./data/files"
	DefaultS3Region               = "us-east-1"
	DefaultMaxUploadBytes         = int64(25 << 20)
	AuthModeDevelopment           = "development"
	AuthModeRequired              = "required"
	DefaultAuthMode               = AuthModeDevelopment
	DefaultAuthBootstrapUserID    = "00000000-0000-0000-0000-000000000001"
	DefaultAuthBootstrapUserName  = "Owner"
	DefaultAuthSessionTTL         = 7 * 24 * time.Hour
	DefaultAuthRecoveryTTL        = 30 * time.Minute
	DefaultAuthSMTPQueueSize      = 100
	DefaultAuthSMTPTimeout        = 10 * time.Second
	DefaultTeamMailWorkerLease    = 30 * time.Second
	DefaultTeamMailWorkerPoll     = 500 * time.Millisecond
	DefaultTeamMailBackoffBase    = 5 * time.Second
	DefaultTeamMailBackoffMax     = 15 * time.Minute
	maximumAuthSMTPQueueSize      = 10_000

	EnvAddr                   = "MM_CHAT_ADDR"
	EnvVersion                = "MM_CHAT_VERSION"
	EnvDatabaseURL            = "DATABASE_URL"
	EnvDBMaxOpenConns         = "DB_MAX_OPEN_CONNS"
	EnvDBMaxIdleConns         = "DB_MAX_IDLE_CONNS"
	EnvDBConnMaxLifetime      = "DB_CONN_MAX_LIFETIME"
	EnvRedisURL               = "REDIS_URL"
	EnvRedisKeyPrefix         = "REDIS_KEY_PREFIX"
	EnvRedisRunCancelTTL      = "REDIS_RUN_CANCEL_TTL"
	EnvRedisSessionCacheTTL   = "REDIS_SESSION_CACHE_TTL"
	EnvRedisRateLimitEnabled  = "REDIS_RATE_LIMIT_ENABLED"
	EnvRedisRateLimitRequests = "REDIS_RATE_LIMIT_REQUESTS"
	EnvRedisRateLimitWindow   = "REDIS_RATE_LIMIT_WINDOW"
	EnvProviderType           = "PROVIDER_TYPE"
	EnvProviderBaseURL        = "PROVIDER_BASE_URL"
	EnvProviderModel          = "PROVIDER_MODEL"
	EnvProviderAPIKey         = "PROVIDER_API_KEY"
	EnvProviderTimeout        = "PROVIDER_TIMEOUT"
	EnvStorageBackend         = "STORAGE_BACKEND"
	EnvLocalStorageDir        = "LOCAL_STORAGE_DIR"
	EnvS3Endpoint             = "S3_ENDPOINT"
	EnvS3Bucket               = "S3_BUCKET"
	EnvS3Region               = "S3_REGION"
	EnvS3AccessKeyID          = "S3_ACCESS_KEY_ID"
	EnvS3SecretAccessKey      = "S3_SECRET_ACCESS_KEY"
	EnvS3UseSSL               = "S3_USE_SSL"
	EnvS3ForcePathStyle       = "S3_FORCE_PATH_STYLE"
	EnvS3BucketAutoCreate     = "S3_BUCKET_AUTO_CREATE"
	EnvMaxUploadBytes         = "MAX_UPLOAD_BYTES"
	EnvAuthMode               = "AUTH_MODE"
	EnvAuthBootstrapUserID    = "AUTH_BOOTSTRAP_USER_ID"
	EnvAuthBootstrapUserName  = "AUTH_BOOTSTRAP_DISPLAY_NAME"
	EnvAuthSessionTTL         = "AUTH_SESSION_TTL"
	EnvAuthRecoveryTTL        = "AUTH_RECOVERY_TTL"
	EnvAuthSMTPAddr           = "AUTH_SMTP_ADDR"
	EnvAuthSMTPUsername       = "AUTH_SMTP_USERNAME"
	EnvAuthSMTPPassword       = "AUTH_SMTP_PASSWORD"
	EnvAuthSMTPFrom           = "AUTH_SMTP_FROM"
	EnvAuthSMTPQueueSize      = "AUTH_SMTP_QUEUE_SIZE"
	EnvAuthSMTPTimeout        = "AUTH_SMTP_TIMEOUT"
	EnvTeamCursorActiveKeyID  = "TEAM_CURSOR_ACTIVE_KEY_ID"
	EnvTeamCursorKeyring      = "TEAM_CURSOR_KEYRING"
	EnvTeamMailActiveKeyID    = "TEAM_MAIL_ACTIVE_KEY_ID"
	EnvTeamMailKeyring        = "TEAM_MAIL_KEYRING"
	EnvTeamInviteAcceptURL    = "TEAM_INVITE_ACCEPT_URL_BASE"
	EnvTeamMailWorkerLease    = "TEAM_MAIL_WORKER_LEASE_DURATION"
	EnvTeamMailWorkerPoll     = "TEAM_MAIL_WORKER_POLL_INTERVAL"
	EnvTeamMailBackoffBase    = "TEAM_MAIL_WORKER_BACKOFF_BASE"
	EnvTeamMailBackoffMax     = "TEAM_MAIL_WORKER_BACKOFF_MAX"
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
	Auth     AuthConfig
	Team     TeamConfig
}

// RedisConfig contains non-authoritative temporary-state settings. Redis must
// not store canonical conversations, messages, files, or provider secrets.
type RedisConfig struct {
	URL               string
	KeyPrefix         string
	RunCancelTTL      time.Duration
	SessionCacheTTL   time.Duration
	RateLimitEnabled  bool
	RateLimitRequests int
	RateLimitWindow   time.Duration
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

// AuthConfig contains local account/session bootstrap settings. Secrets must
// never be logged or serialized into API responses.
type AuthConfig struct {
	Mode                 string
	BootstrapUserID      string
	BootstrapDisplayName string
	SessionTTL           time.Duration
	RecoveryTTL          time.Duration
	SMTP                 SMTPRecoveryConfig
}

// SMTPRecoveryConfig contains server-only mailbox delivery settings. The
// password and raw recovery tokens must never be logged or serialized.
type SMTPRecoveryConfig struct {
	Addr      string
	Username  string
	Password  string
	From      string
	QueueSize int
	Timeout   time.Duration
}

// TeamConfig contains Team cursor signing and durable Invite delivery
// settings. Encoded keyrings remain server-only and must never be logged.
type TeamConfig struct {
	Cursor              TeamKeyringConfig
	Mail                TeamKeyringConfig
	InviteAcceptURLBase string
	MailWorker          TeamMailWorkerConfig

	invalidFields []string
}

// TeamKeyringConfig stores an active key ID and a comma-separated keyring in
// key-id=base64 form. Retained non-active keys are verify/decrypt-only.
type TeamKeyringConfig struct {
	ActiveKeyID string
	Keyring     string
}

// TeamMailWorkerConfig controls the durable Invite mail worker loop.
type TeamMailWorkerConfig struct {
	LeaseDuration  time.Duration
	PollInterval   time.Duration
	BackoffBase    time.Duration
	BackoffMaximum time.Duration
}

func (cfg AuthConfig) RequireAuth() bool {
	return cfg.Mode == AuthModeRequired
}

// Validate rejects malformed Team settings without including encoded key
// material in the returned error.
func (cfg Config) Validate() error {
	if len(cfg.Team.invalidFields) > 0 {
		return fmt.Errorf("invalid configuration for %s", cfg.Team.invalidFields[0])
	}
	if err := validateOptionalKeyringPair(
		cfg.Team.Cursor,
		EnvTeamCursorActiveKeyID,
		EnvTeamCursorKeyring,
	); err != nil {
		return err
	}
	if err := validateOptionalKeyringPair(
		cfg.Team.Mail,
		EnvTeamMailActiveKeyID,
		EnvTeamMailKeyring,
	); err != nil {
		return err
	}
	hasCursorKeys := keyringConfigured(cfg.Team.Cursor)
	if cfg.Auth.RequireAuth() && !hasCursorKeys {
		return fmt.Errorf(
			"%s and %s are required when %s=%s",
			EnvTeamCursorActiveKeyID,
			EnvTeamCursorKeyring,
			EnvAuthMode,
			AuthModeRequired,
		)
	}

	hasMailKeys := keyringConfigured(cfg.Team.Mail)
	hasInviteURL := strings.TrimSpace(cfg.Team.InviteAcceptURLBase) != ""
	if hasMailKeys != hasInviteURL {
		return fmt.Errorf(
			"%s, %s, and %s must be configured together",
			EnvTeamMailActiveKeyID,
			EnvTeamMailKeyring,
			EnvTeamInviteAcceptURL,
		)
	}
	if hasMailKeys && smtpTransportBlank(cfg.Auth.SMTP) {
		return fmt.Errorf(
			"%s and %s are required when Team Invite delivery is configured",
			EnvAuthSMTPAddr,
			EnvAuthSMTPFrom,
		)
	}
	worker := cfg.Team.MailWorker
	for _, setting := range []struct {
		field string
		value time.Duration
	}{
		{field: EnvTeamMailWorkerLease, value: worker.LeaseDuration},
		{field: EnvTeamMailWorkerPoll, value: worker.PollInterval},
		{field: EnvTeamMailBackoffBase, value: worker.BackoffBase},
		{field: EnvTeamMailBackoffMax, value: worker.BackoffMaximum},
	} {
		if setting.value < 0 {
			return fmt.Errorf("%s must be positive", setting.field)
		}
	}
	if worker.BackoffBase > 0 && worker.BackoffMaximum > 0 &&
		worker.BackoffMaximum < worker.BackoffBase {
		return fmt.Errorf(
			"%s must be greater than or equal to %s",
			EnvTeamMailBackoffMax,
			EnvTeamMailBackoffBase,
		)
	}
	return nil
}

// Load reads configuration from the process environment.
func Load() Config {
	return LoadFromEnv(os.LookupEnv)
}

// LoadFromEnv reads configuration from the supplied lookup function. Empty or
// whitespace-only values fall back to defaults. Call Validate before startup;
// strict Team fields retain invalid-input state instead of silently enabling a
// partially configured runtime.
func LoadFromEnv(lookup func(string) (string, bool)) Config {
	teamWorker, invalidTeamFields := loadTeamMailWorkerConfig(lookup)
	return Config{
		Addr:        envOrDefault(lookup, EnvAddr, DefaultAddr),
		Version:     envOrDefault(lookup, EnvVersion, DefaultVersion),
		DatabaseURL: optionalEnv(lookup, EnvDatabaseURL),

		DBMaxOpenConns:    intEnvOrDefault(lookup, EnvDBMaxOpenConns, DefaultDBMaxOpenConns),
		DBMaxIdleConns:    intEnvOrDefault(lookup, EnvDBMaxIdleConns, DefaultDBMaxIdleConns),
		DBConnMaxLifetime: durationEnvOrDefault(lookup, EnvDBConnMaxLifetime, DefaultDBConnMaxLifetime),

		Redis: RedisConfig{
			URL:               optionalEnv(lookup, EnvRedisURL),
			KeyPrefix:         envOrDefault(lookup, EnvRedisKeyPrefix, DefaultRedisKeyPrefix),
			RunCancelTTL:      durationEnvOrDefault(lookup, EnvRedisRunCancelTTL, DefaultRedisRunCancelTTL),
			SessionCacheTTL:   durationEnvOrDefault(lookup, EnvRedisSessionCacheTTL, DefaultRedisSessionCacheTTL),
			RateLimitEnabled:  boolEnvOrDefault(lookup, EnvRedisRateLimitEnabled, DefaultRedisRateLimitEnabled),
			RateLimitRequests: intEnvOrDefault(lookup, EnvRedisRateLimitRequests, DefaultRedisRateLimitRequests),
			RateLimitWindow:   durationEnvOrDefault(lookup, EnvRedisRateLimitWindow, DefaultRedisRateLimitWindow),
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

		Auth: AuthConfig{
			Mode:                 authModeEnvOrDefault(lookup, EnvAuthMode, DefaultAuthMode),
			BootstrapUserID:      envOrDefault(lookup, EnvAuthBootstrapUserID, DefaultAuthBootstrapUserID),
			BootstrapDisplayName: envOrDefault(lookup, EnvAuthBootstrapUserName, DefaultAuthBootstrapUserName),
			SessionTTL:           durationEnvOrDefault(lookup, EnvAuthSessionTTL, DefaultAuthSessionTTL),
			RecoveryTTL:          durationEnvOrDefault(lookup, EnvAuthRecoveryTTL, DefaultAuthRecoveryTTL),
			SMTP: SMTPRecoveryConfig{
				Addr:      optionalEnv(lookup, EnvAuthSMTPAddr),
				Username:  optionalEnv(lookup, EnvAuthSMTPUsername),
				Password:  optionalEnv(lookup, EnvAuthSMTPPassword),
				From:      optionalEnv(lookup, EnvAuthSMTPFrom),
				QueueSize: intEnvOrDefault(lookup, EnvAuthSMTPQueueSize, DefaultAuthSMTPQueueSize),
				Timeout:   durationEnvOrDefault(lookup, EnvAuthSMTPTimeout, DefaultAuthSMTPTimeout),
			},
		},

		Team: TeamConfig{
			Cursor: TeamKeyringConfig{
				ActiveKeyID: optionalEnv(lookup, EnvTeamCursorActiveKeyID),
				Keyring:     optionalEnv(lookup, EnvTeamCursorKeyring),
			},
			Mail: TeamKeyringConfig{
				ActiveKeyID: optionalEnv(lookup, EnvTeamMailActiveKeyID),
				Keyring:     optionalEnv(lookup, EnvTeamMailKeyring),
			},
			InviteAcceptURLBase: optionalEnv(lookup, EnvTeamInviteAcceptURL),
			MailWorker:          teamWorker,
			invalidFields:       invalidTeamFields,
		},
	}
}

// ParseBase64Keyring parses comma-separated key-id=base64 entries. Errors
// identify only the field, entry, or key ID and never echo encoded key bytes.
func ParseBase64Keyring(field string, value string) (map[string][]byte, error) {
	field = strings.TrimSpace(field)
	if field == "" {
		field = "keyring"
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("%s is required", field)
	}

	entries := strings.Split(value, ",")
	keys := make(map[string][]byte, len(entries))
	for index, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return nil, fmt.Errorf("%s entry %d is empty", field, index+1)
		}
		keyID, encoded, ok := strings.Cut(entry, "=")
		keyID = strings.TrimSpace(keyID)
		encoded = strings.TrimSpace(encoded)
		if !ok || keyID == "" || encoded == "" {
			return nil, fmt.Errorf(
				"%s entry %d must use key-id=base64 format",
				field,
				index+1,
			)
		}
		if !validTeamKeyID(keyID) {
			return nil, fmt.Errorf("%s entry %d has an invalid key id", field, index+1)
		}
		if _, exists := keys[keyID]; exists {
			return nil, fmt.Errorf("%s key id %q is duplicated", field, keyID)
		}
		decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if err != nil || len(decoded) == 0 {
			return nil, fmt.Errorf("%s key id %q is not valid base64", field, keyID)
		}
		keys[keyID] = append([]byte(nil), decoded...)
	}
	return keys, nil
}

func validateOptionalKeyringPair(
	keyring TeamKeyringConfig,
	activeField string,
	keyringField string,
) error {
	hasActive := strings.TrimSpace(keyring.ActiveKeyID) != ""
	hasKeyring := strings.TrimSpace(keyring.Keyring) != ""
	if hasActive == hasKeyring {
		return nil
	}
	return fmt.Errorf("%s and %s must be configured together", activeField, keyringField)
}

func keyringConfigured(keyring TeamKeyringConfig) bool {
	return strings.TrimSpace(keyring.ActiveKeyID) != "" &&
		strings.TrimSpace(keyring.Keyring) != ""
}

func smtpTransportBlank(cfg SMTPRecoveryConfig) bool {
	return strings.TrimSpace(cfg.Addr) == "" &&
		strings.TrimSpace(cfg.Username) == "" &&
		cfg.Password == "" &&
		strings.TrimSpace(cfg.From) == ""
}

func validTeamKeyID(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for _, current := range value {
		if current >= 'a' && current <= 'z' ||
			current >= 'A' && current <= 'Z' ||
			current >= '0' && current <= '9' ||
			current == '-' || current == '_' || current == '.' {
			continue
		}
		return false
	}
	return true
}

func loadTeamMailWorkerConfig(
	lookup func(string) (string, bool),
) (TeamMailWorkerConfig, []string) {
	invalidFields := make([]string, 0, 6)
	if value, configured := optionalLookup(lookup, EnvAuthSMTPQueueSize); configured {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > maximumAuthSMTPQueueSize {
			invalidFields = append(invalidFields, EnvAuthSMTPQueueSize)
		}
	}
	if value, configured := optionalLookup(lookup, EnvAuthSMTPTimeout); configured {
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 {
			invalidFields = append(invalidFields, EnvAuthSMTPTimeout)
		}
	}
	lease, ok := positiveDurationEnvOrDefault(
		lookup,
		EnvTeamMailWorkerLease,
		DefaultTeamMailWorkerLease,
	)
	if !ok {
		invalidFields = append(invalidFields, EnvTeamMailWorkerLease)
	}
	poll, ok := positiveDurationEnvOrDefault(
		lookup,
		EnvTeamMailWorkerPoll,
		DefaultTeamMailWorkerPoll,
	)
	if !ok {
		invalidFields = append(invalidFields, EnvTeamMailWorkerPoll)
	}
	base, ok := positiveDurationEnvOrDefault(
		lookup,
		EnvTeamMailBackoffBase,
		DefaultTeamMailBackoffBase,
	)
	if !ok {
		invalidFields = append(invalidFields, EnvTeamMailBackoffBase)
	}
	maximum, ok := positiveDurationEnvOrDefault(
		lookup,
		EnvTeamMailBackoffMax,
		DefaultTeamMailBackoffMax,
	)
	if !ok {
		invalidFields = append(invalidFields, EnvTeamMailBackoffMax)
	}

	return TeamMailWorkerConfig{
		LeaseDuration:  lease,
		PollInterval:   poll,
		BackoffBase:    base,
		BackoffMaximum: maximum,
	}, invalidFields
}

func positiveDurationEnvOrDefault(
	lookup func(string) (string, bool),
	key string,
	fallback time.Duration,
) (time.Duration, bool) {
	value, ok := optionalLookup(lookup, key)
	if !ok {
		return fallback, true
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback, false
	}
	return parsed, true
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

func authModeEnvOrDefault(lookup func(string) (string, bool), key string, fallback string) string {
	value, ok := optionalLookup(lookup, key)
	if !ok {
		return fallback
	}

	switch strings.ToLower(value) {
	case AuthModeDevelopment, "dev", "local":
		return AuthModeDevelopment
	case AuthModeRequired, "hosted", "server":
		return AuthModeRequired
	default:
		return AuthModeRequired
	}
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
