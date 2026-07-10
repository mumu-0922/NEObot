package config

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestLoadFromEnvDefaults(t *testing.T) {
	cfg := LoadFromEnv(func(string) (string, bool) {
		return "", false
	})

	if cfg.Addr != DefaultAddr {
		t.Fatalf("Addr = %q, want %q", cfg.Addr, DefaultAddr)
	}
	if cfg.Version != DefaultVersion {
		t.Fatalf("Version = %q, want %q", cfg.Version, DefaultVersion)
	}
	if cfg.DatabaseURL != "" {
		t.Fatalf("DatabaseURL = %q, want empty", cfg.DatabaseURL)
	}
	if cfg.DBMaxOpenConns != DefaultDBMaxOpenConns {
		t.Fatalf("DBMaxOpenConns = %d, want %d", cfg.DBMaxOpenConns, DefaultDBMaxOpenConns)
	}
	if cfg.DBMaxIdleConns != DefaultDBMaxIdleConns {
		t.Fatalf("DBMaxIdleConns = %d, want %d", cfg.DBMaxIdleConns, DefaultDBMaxIdleConns)
	}
	if cfg.DBConnMaxLifetime != DefaultDBConnMaxLifetime {
		t.Fatalf("DBConnMaxLifetime = %s, want %s", cfg.DBConnMaxLifetime, DefaultDBConnMaxLifetime)
	}
	if cfg.Redis.URL != "" {
		t.Fatalf("Redis.URL = %q, want empty", cfg.Redis.URL)
	}
	if cfg.Redis.KeyPrefix != DefaultRedisKeyPrefix {
		t.Fatalf("Redis.KeyPrefix = %q, want %q", cfg.Redis.KeyPrefix, DefaultRedisKeyPrefix)
	}
	if cfg.Redis.RunCancelTTL != DefaultRedisRunCancelTTL {
		t.Fatalf("Redis.RunCancelTTL = %s, want %s", cfg.Redis.RunCancelTTL, DefaultRedisRunCancelTTL)
	}
	if cfg.Redis.SessionCacheTTL != DefaultRedisSessionCacheTTL {
		t.Fatalf(
			"Redis.SessionCacheTTL = %s, want %s",
			cfg.Redis.SessionCacheTTL,
			DefaultRedisSessionCacheTTL,
		)
	}
	if cfg.Redis.RateLimitEnabled != DefaultRedisRateLimitEnabled {
		t.Fatalf("Redis.RateLimitEnabled = %v, want %v", cfg.Redis.RateLimitEnabled, DefaultRedisRateLimitEnabled)
	}
	if cfg.Redis.RateLimitRequests != DefaultRedisRateLimitRequests {
		t.Fatalf("Redis.RateLimitRequests = %d, want %d", cfg.Redis.RateLimitRequests, DefaultRedisRateLimitRequests)
	}
	if cfg.Redis.RateLimitWindow != DefaultRedisRateLimitWindow {
		t.Fatalf("Redis.RateLimitWindow = %s, want %s", cfg.Redis.RateLimitWindow, DefaultRedisRateLimitWindow)
	}
	if cfg.Provider.Type != "" {
		t.Fatalf("Provider.Type = %q, want empty", cfg.Provider.Type)
	}
	if cfg.Provider.BaseURL != "" {
		t.Fatalf("Provider.BaseURL = %q, want empty", cfg.Provider.BaseURL)
	}
	if cfg.Provider.Model != "" {
		t.Fatalf("Provider.Model = %q, want empty", cfg.Provider.Model)
	}
	if cfg.Provider.APIKey != "" {
		t.Fatalf("Provider.APIKey = %q, want empty", cfg.Provider.APIKey)
	}
	if cfg.Provider.Timeout != DefaultProviderTimeout {
		t.Fatalf("Provider.Timeout = %s, want %s", cfg.Provider.Timeout, DefaultProviderTimeout)
	}
	if cfg.Storage.Backend != DefaultStorageBackend {
		t.Fatalf("Storage.Backend = %q, want %q", cfg.Storage.Backend, DefaultStorageBackend)
	}
	if cfg.Storage.LocalDir != DefaultLocalStorageDir {
		t.Fatalf("Storage.LocalDir = %q, want %q", cfg.Storage.LocalDir, DefaultLocalStorageDir)
	}
	if cfg.Storage.S3.Endpoint != "" ||
		cfg.Storage.S3.Bucket != "" ||
		cfg.Storage.S3.AccessKeyID != "" ||
		cfg.Storage.S3.SecretAccessKey != "" {
		t.Fatalf("Storage.S3 = %#v, want blank endpoint/bucket/credentials", cfg.Storage.S3)
	}
	if cfg.Storage.S3.Region != DefaultS3Region {
		t.Fatalf("Storage.S3.Region = %q, want %q", cfg.Storage.S3.Region, DefaultS3Region)
	}
	if cfg.Storage.S3.UseSSL || cfg.Storage.S3.ForcePathStyle || cfg.Storage.S3.BucketAutoCreate {
		t.Fatalf("Storage.S3 booleans = %#v, want false", cfg.Storage.S3)
	}
	if cfg.Storage.MaxUploadBytes != DefaultMaxUploadBytes {
		t.Fatalf("Storage.MaxUploadBytes = %d, want %d", cfg.Storage.MaxUploadBytes, DefaultMaxUploadBytes)
	}
	if cfg.Auth.Mode != DefaultAuthMode {
		t.Fatalf("Auth.Mode = %q, want %q", cfg.Auth.Mode, DefaultAuthMode)
	}
	if cfg.Auth.RequireAuth() {
		t.Fatal("Auth.RequireAuth() = true, want false for default development mode")
	}
	if cfg.Auth.BootstrapUserID != DefaultAuthBootstrapUserID {
		t.Fatalf("Auth.BootstrapUserID = %q, want %q", cfg.Auth.BootstrapUserID, DefaultAuthBootstrapUserID)
	}
	if cfg.Auth.BootstrapDisplayName != DefaultAuthBootstrapUserName {
		t.Fatalf("Auth.BootstrapDisplayName = %q, want %q", cfg.Auth.BootstrapDisplayName, DefaultAuthBootstrapUserName)
	}
	if cfg.Auth.SessionTTL != DefaultAuthSessionTTL {
		t.Fatalf("Auth.SessionTTL = %s, want %s", cfg.Auth.SessionTTL, DefaultAuthSessionTTL)
	}
	if cfg.Auth.RecoveryTTL != DefaultAuthRecoveryTTL {
		t.Fatalf("Auth.RecoveryTTL = %s, want %s", cfg.Auth.RecoveryTTL, DefaultAuthRecoveryTTL)
	}
	if cfg.Auth.SMTP.Addr != "" || cfg.Auth.SMTP.Username != "" ||
		cfg.Auth.SMTP.Password != "" || cfg.Auth.SMTP.From != "" ||
		cfg.Auth.SMTP.QueueSize != DefaultAuthSMTPQueueSize ||
		cfg.Auth.SMTP.Timeout != DefaultAuthSMTPTimeout {
		t.Fatalf("Auth.SMTP = %#v, want blank/default values", cfg.Auth.SMTP)
	}
	if cfg.Team.Cursor != (TeamKeyringConfig{}) ||
		cfg.Team.Mail != (TeamKeyringConfig{}) ||
		cfg.Team.InviteAcceptURLBase != "" {
		t.Fatalf("Team key/url config = %#v, want blank", cfg.Team)
	}
	if cfg.Team.MailWorker.LeaseDuration != DefaultTeamMailWorkerLease ||
		cfg.Team.MailWorker.PollInterval != DefaultTeamMailWorkerPoll ||
		cfg.Team.MailWorker.BackoffBase != DefaultTeamMailBackoffBase ||
		cfg.Team.MailWorker.BackoffMaximum != DefaultTeamMailBackoffMax {
		t.Fatalf("Team.MailWorker = %#v, want defaults", cfg.Team.MailWorker)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() defaults error = %v", err)
	}
}

func TestLoadFromEnvOverrides(t *testing.T) {
	values := map[string]string{
		EnvAddr:                   "127.0.0.1:9090",
		EnvVersion:                "test-version",
		EnvDatabaseURL:            " postgres://user:pass@localhost:5432/mmchat?sslmode=disable ",
		EnvDBMaxOpenConns:         "12",
		EnvDBMaxIdleConns:         "7",
		EnvDBConnMaxLifetime:      "45m",
		EnvRedisURL:               " redis://:redis-pass@redis:6379/1 ",
		EnvRedisKeyPrefix:         " neo-test ",
		EnvRedisRunCancelTTL:      "15m",
		EnvRedisSessionCacheTTL:   "3m",
		EnvRedisRateLimitEnabled:  "true",
		EnvRedisRateLimitRequests: "42",
		EnvRedisRateLimitWindow:   "30s",
		EnvProviderType:           " openai_compatible ",
		EnvProviderBaseURL:        " https://sub.example.test/v1/ ",
		EnvProviderModel:          " gpt-5.5 ",
		EnvProviderAPIKey:         " secret-key ",
		EnvProviderTimeout:        "90s",
		EnvStorageBackend:         " MINIO ",
		EnvLocalStorageDir:        " /srv/mm-chat/files ",
		EnvS3Endpoint:             " http://minio:9000 ",
		EnvS3Bucket:               " neo-chat-files ",
		EnvS3Region:               " us-west-2 ",
		EnvS3AccessKeyID:          " minio-user ",
		EnvS3SecretAccessKey:      " minio-secret ",
		EnvS3UseSSL:               " true ",
		EnvS3ForcePathStyle:       " true ",
		EnvS3BucketAutoCreate:     " true ",
		EnvMaxUploadBytes:         "1048576",
		EnvAuthMode:               " required ",
		EnvAuthBootstrapUserID:    " 77777777-7777-4777-8777-777777777777 ",
		EnvAuthBootstrapUserName:  " Server Owner ",
		EnvAuthSessionTTL:         "24h",
		EnvAuthRecoveryTTL:        "45m",
		EnvAuthSMTPAddr:           " smtp.example.test:587 ",
		EnvAuthSMTPUsername:       " mailer ",
		EnvAuthSMTPPassword:       " mail-secret ",
		EnvAuthSMTPFrom:           " no-reply@example.test ",
		EnvAuthSMTPQueueSize:      "25",
		EnvAuthSMTPTimeout:        "12s",
	}

	cfg := LoadFromEnv(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})

	if cfg.Addr != "127.0.0.1:9090" {
		t.Fatalf("Addr = %q, want %q", cfg.Addr, "127.0.0.1:9090")
	}
	if cfg.Version != "test-version" {
		t.Fatalf("Version = %q, want %q", cfg.Version, "test-version")
	}
	if cfg.DatabaseURL != "postgres://user:pass@localhost:5432/mmchat?sslmode=disable" {
		t.Fatalf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.DBMaxOpenConns != 12 {
		t.Fatalf("DBMaxOpenConns = %d, want 12", cfg.DBMaxOpenConns)
	}
	if cfg.DBMaxIdleConns != 7 {
		t.Fatalf("DBMaxIdleConns = %d, want 7", cfg.DBMaxIdleConns)
	}
	if cfg.DBConnMaxLifetime != 45*time.Minute {
		t.Fatalf("DBConnMaxLifetime = %s, want 45m", cfg.DBConnMaxLifetime)
	}
	if cfg.Redis.URL != "redis://:redis-pass@redis:6379/1" {
		t.Fatalf("Redis.URL = %q", cfg.Redis.URL)
	}
	if cfg.Redis.KeyPrefix != "neo-test" {
		t.Fatalf("Redis.KeyPrefix = %q", cfg.Redis.KeyPrefix)
	}
	if cfg.Redis.RunCancelTTL != 15*time.Minute {
		t.Fatalf("Redis.RunCancelTTL = %s, want 15m", cfg.Redis.RunCancelTTL)
	}
	if cfg.Redis.SessionCacheTTL != 3*time.Minute {
		t.Fatalf("Redis.SessionCacheTTL = %s, want 3m", cfg.Redis.SessionCacheTTL)
	}
	if !cfg.Redis.RateLimitEnabled {
		t.Fatal("Redis.RateLimitEnabled = false, want true")
	}
	if cfg.Redis.RateLimitRequests != 42 {
		t.Fatalf("Redis.RateLimitRequests = %d, want 42", cfg.Redis.RateLimitRequests)
	}
	if cfg.Redis.RateLimitWindow != 30*time.Second {
		t.Fatalf("Redis.RateLimitWindow = %s, want 30s", cfg.Redis.RateLimitWindow)
	}
	if cfg.Provider.Type != "openai_compatible" {
		t.Fatalf("Provider.Type = %q, want openai_compatible", cfg.Provider.Type)
	}
	if cfg.Provider.BaseURL != "https://sub.example.test/v1/" {
		t.Fatalf("Provider.BaseURL = %q", cfg.Provider.BaseURL)
	}
	if cfg.Provider.Model != "gpt-5.5" {
		t.Fatalf("Provider.Model = %q, want gpt-5.5", cfg.Provider.Model)
	}
	if cfg.Provider.APIKey != "secret-key" {
		t.Fatalf("Provider.APIKey = %q, want secret-key", cfg.Provider.APIKey)
	}
	if cfg.Provider.Timeout != 90*time.Second {
		t.Fatalf("Provider.Timeout = %s, want 90s", cfg.Provider.Timeout)
	}
	if cfg.Storage.Backend != "minio" {
		t.Fatalf("Storage.Backend = %q, want minio", cfg.Storage.Backend)
	}
	if cfg.Storage.LocalDir != "/srv/mm-chat/files" {
		t.Fatalf("Storage.LocalDir = %q", cfg.Storage.LocalDir)
	}
	if cfg.Storage.S3.Endpoint != "http://minio:9000" {
		t.Fatalf("Storage.S3.Endpoint = %q", cfg.Storage.S3.Endpoint)
	}
	if cfg.Storage.S3.Bucket != "neo-chat-files" {
		t.Fatalf("Storage.S3.Bucket = %q", cfg.Storage.S3.Bucket)
	}
	if cfg.Storage.S3.Region != "us-west-2" {
		t.Fatalf("Storage.S3.Region = %q", cfg.Storage.S3.Region)
	}
	if cfg.Storage.S3.AccessKeyID != "minio-user" {
		t.Fatalf("Storage.S3.AccessKeyID = %q", cfg.Storage.S3.AccessKeyID)
	}
	if cfg.Storage.S3.SecretAccessKey != "minio-secret" {
		t.Fatalf("Storage.S3.SecretAccessKey = %q", cfg.Storage.S3.SecretAccessKey)
	}
	if !cfg.Storage.S3.UseSSL || !cfg.Storage.S3.ForcePathStyle || !cfg.Storage.S3.BucketAutoCreate {
		t.Fatalf("Storage.S3 booleans = %#v, want true", cfg.Storage.S3)
	}
	if cfg.Storage.MaxUploadBytes != 1048576 {
		t.Fatalf("Storage.MaxUploadBytes = %d, want 1048576", cfg.Storage.MaxUploadBytes)
	}
	if cfg.Auth.Mode != AuthModeRequired {
		t.Fatalf("Auth.Mode = %q, want required", cfg.Auth.Mode)
	}
	if !cfg.Auth.RequireAuth() {
		t.Fatal("Auth.RequireAuth() = false, want true")
	}
	if cfg.Auth.BootstrapUserID != "77777777-7777-4777-8777-777777777777" {
		t.Fatalf("Auth.BootstrapUserID = %q", cfg.Auth.BootstrapUserID)
	}
	if cfg.Auth.BootstrapDisplayName != "Server Owner" {
		t.Fatalf("Auth.BootstrapDisplayName = %q", cfg.Auth.BootstrapDisplayName)
	}
	if cfg.Auth.SessionTTL != 24*time.Hour {
		t.Fatalf("Auth.SessionTTL = %s, want 24h", cfg.Auth.SessionTTL)
	}
	if cfg.Auth.RecoveryTTL != 45*time.Minute {
		t.Fatalf("Auth.RecoveryTTL = %s, want 45m", cfg.Auth.RecoveryTTL)
	}
	if cfg.Auth.SMTP.Addr != "smtp.example.test:587" ||
		cfg.Auth.SMTP.Username != "mailer" ||
		cfg.Auth.SMTP.Password != "mail-secret" ||
		cfg.Auth.SMTP.From != "no-reply@example.test" ||
		cfg.Auth.SMTP.QueueSize != 25 ||
		cfg.Auth.SMTP.Timeout != 12*time.Second {
		t.Fatalf("Auth.SMTP = %#v, want overrides", cfg.Auth.SMTP)
	}
}

func TestLoadFromEnvIgnoresBlankValues(t *testing.T) {
	values := map[string]string{
		EnvAddr:                  "   ",
		EnvVersion:               "\t",
		EnvDatabaseURL:           " \n ",
		EnvDBMaxOpenConns:        " ",
		EnvDBMaxIdleConns:        "\t",
		EnvDBConnMaxLifetime:     " \n",
		EnvRedisURL:              " ",
		EnvRedisKeyPrefix:        "\t",
		EnvRedisRunCancelTTL:     "\n",
		EnvRedisSessionCacheTTL:  " ",
		EnvProviderType:          " ",
		EnvProviderBaseURL:       "\t",
		EnvProviderModel:         " \n ",
		EnvProviderAPIKey:        " ",
		EnvProviderTimeout:       "\t",
		EnvStorageBackend:        " ",
		EnvLocalStorageDir:       "\t",
		EnvS3Endpoint:            " ",
		EnvS3Bucket:              "\t",
		EnvS3Region:              "\n",
		EnvS3AccessKeyID:         " ",
		EnvS3SecretAccessKey:     "\t",
		EnvS3UseSSL:              " ",
		EnvS3ForcePathStyle:      " ",
		EnvS3BucketAutoCreate:    "\n",
		EnvMaxUploadBytes:        "\n",
		EnvAuthMode:              "\t",
		EnvAuthBootstrapUserID:   "\t",
		EnvAuthBootstrapUserName: "\n",
		EnvAuthSessionTTL:        " ",
		EnvAuthRecoveryTTL:       " ",
		EnvAuthSMTPAddr:          " ",
		EnvAuthSMTPUsername:      " ",
		EnvAuthSMTPPassword:      " ",
		EnvAuthSMTPFrom:          " ",
		EnvAuthSMTPQueueSize:     " ",
		EnvAuthSMTPTimeout:       " ",
	}

	cfg := LoadFromEnv(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})

	if cfg.Addr != DefaultAddr {
		t.Fatalf("Addr = %q, want %q", cfg.Addr, DefaultAddr)
	}
	if cfg.Version != DefaultVersion {
		t.Fatalf("Version = %q, want %q", cfg.Version, DefaultVersion)
	}
	if cfg.DatabaseURL != "" {
		t.Fatalf("DatabaseURL = %q, want empty", cfg.DatabaseURL)
	}
	if cfg.DBMaxOpenConns != DefaultDBMaxOpenConns {
		t.Fatalf("DBMaxOpenConns = %d, want %d", cfg.DBMaxOpenConns, DefaultDBMaxOpenConns)
	}
	if cfg.DBMaxIdleConns != DefaultDBMaxIdleConns {
		t.Fatalf("DBMaxIdleConns = %d, want %d", cfg.DBMaxIdleConns, DefaultDBMaxIdleConns)
	}
	if cfg.DBConnMaxLifetime != DefaultDBConnMaxLifetime {
		t.Fatalf("DBConnMaxLifetime = %s, want %s", cfg.DBConnMaxLifetime, DefaultDBConnMaxLifetime)
	}
	if cfg.Redis.URL != "" ||
		cfg.Redis.KeyPrefix != DefaultRedisKeyPrefix ||
		cfg.Redis.RunCancelTTL != DefaultRedisRunCancelTTL ||
		cfg.Redis.SessionCacheTTL != DefaultRedisSessionCacheTTL ||
		cfg.Redis.RateLimitEnabled != DefaultRedisRateLimitEnabled ||
		cfg.Redis.RateLimitRequests != DefaultRedisRateLimitRequests ||
		cfg.Redis.RateLimitWindow != DefaultRedisRateLimitWindow {
		t.Fatalf("Redis = %#v, want defaults", cfg.Redis)
	}
	if cfg.Provider.Type != "" || cfg.Provider.BaseURL != "" ||
		cfg.Provider.Model != "" || cfg.Provider.APIKey != "" {
		t.Fatalf("Provider = %#v, want blank strings", cfg.Provider)
	}
	if cfg.Provider.Timeout != DefaultProviderTimeout {
		t.Fatalf("Provider.Timeout = %s, want %s", cfg.Provider.Timeout, DefaultProviderTimeout)
	}
	if cfg.Storage.Backend != DefaultStorageBackend ||
		cfg.Storage.LocalDir != DefaultLocalStorageDir ||
		cfg.Storage.S3.Region != DefaultS3Region ||
		cfg.Storage.MaxUploadBytes != DefaultMaxUploadBytes {
		t.Fatalf("Storage = %#v, want defaults", cfg.Storage)
	}
	if cfg.Storage.S3.Endpoint != "" ||
		cfg.Storage.S3.Bucket != "" ||
		cfg.Storage.S3.AccessKeyID != "" ||
		cfg.Storage.S3.SecretAccessKey != "" ||
		cfg.Storage.S3.UseSSL ||
		cfg.Storage.S3.ForcePathStyle ||
		cfg.Storage.S3.BucketAutoCreate {
		t.Fatalf("Storage.S3 = %#v, want blank/false defaults", cfg.Storage.S3)
	}
	if cfg.Auth.Mode != DefaultAuthMode ||
		cfg.Auth.BootstrapUserID != DefaultAuthBootstrapUserID ||
		cfg.Auth.BootstrapDisplayName != DefaultAuthBootstrapUserName ||
		cfg.Auth.SessionTTL != DefaultAuthSessionTTL ||
		cfg.Auth.RecoveryTTL != DefaultAuthRecoveryTTL ||
		cfg.Auth.SMTP.QueueSize != DefaultAuthSMTPQueueSize ||
		cfg.Auth.SMTP.Timeout != DefaultAuthSMTPTimeout {
		t.Fatalf("Auth = %#v, want defaults", cfg.Auth)
	}
}

func TestLoadFromEnvFallsBackForInvalidDBValues(t *testing.T) {
	values := map[string]string{
		EnvDBMaxOpenConns:         "not-an-int",
		EnvDBMaxIdleConns:         "-1",
		EnvDBConnMaxLifetime:      "not-a-duration",
		EnvRedisRunCancelTTL:      "not-a-duration",
		EnvRedisSessionCacheTTL:   "-1s",
		EnvRedisRateLimitEnabled:  "not-a-bool",
		EnvRedisRateLimitRequests: "-1",
		EnvRedisRateLimitWindow:   "not-a-duration",
		EnvProviderTimeout:        "-1s",
		EnvS3UseSSL:               "not-a-bool",
		EnvS3ForcePathStyle:       "not-a-bool",
		EnvS3BucketAutoCreate:     "not-a-bool",
		EnvMaxUploadBytes:         "-1",
		EnvAuthMode:               "typo",
		EnvAuthSessionTTL:         "not-a-duration",
		EnvAuthRecoveryTTL:        "not-a-duration",
		EnvAuthSMTPQueueSize:      "-1",
		EnvAuthSMTPTimeout:        "not-a-duration",
	}

	cfg := LoadFromEnv(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})

	if cfg.DBMaxOpenConns != DefaultDBMaxOpenConns {
		t.Fatalf("DBMaxOpenConns = %d, want %d", cfg.DBMaxOpenConns, DefaultDBMaxOpenConns)
	}
	if cfg.DBMaxIdleConns != DefaultDBMaxIdleConns {
		t.Fatalf("DBMaxIdleConns = %d, want %d", cfg.DBMaxIdleConns, DefaultDBMaxIdleConns)
	}
	if cfg.DBConnMaxLifetime != DefaultDBConnMaxLifetime {
		t.Fatalf("DBConnMaxLifetime = %s, want %s", cfg.DBConnMaxLifetime, DefaultDBConnMaxLifetime)
	}
	if cfg.Redis.RunCancelTTL != DefaultRedisRunCancelTTL {
		t.Fatalf("Redis.RunCancelTTL = %s, want %s", cfg.Redis.RunCancelTTL, DefaultRedisRunCancelTTL)
	}
	if cfg.Redis.SessionCacheTTL != DefaultRedisSessionCacheTTL {
		t.Fatalf(
			"Redis.SessionCacheTTL = %s, want %s",
			cfg.Redis.SessionCacheTTL,
			DefaultRedisSessionCacheTTL,
		)
	}
	if cfg.Redis.RateLimitEnabled != DefaultRedisRateLimitEnabled {
		t.Fatalf("Redis.RateLimitEnabled = %v, want %v", cfg.Redis.RateLimitEnabled, DefaultRedisRateLimitEnabled)
	}
	if cfg.Redis.RateLimitRequests != DefaultRedisRateLimitRequests {
		t.Fatalf("Redis.RateLimitRequests = %d, want %d", cfg.Redis.RateLimitRequests, DefaultRedisRateLimitRequests)
	}
	if cfg.Redis.RateLimitWindow != DefaultRedisRateLimitWindow {
		t.Fatalf("Redis.RateLimitWindow = %s, want %s", cfg.Redis.RateLimitWindow, DefaultRedisRateLimitWindow)
	}
	if cfg.Provider.Timeout != DefaultProviderTimeout {
		t.Fatalf("Provider.Timeout = %s, want %s", cfg.Provider.Timeout, DefaultProviderTimeout)
	}
	if cfg.Storage.MaxUploadBytes != DefaultMaxUploadBytes {
		t.Fatalf("Storage.MaxUploadBytes = %d, want %d", cfg.Storage.MaxUploadBytes, DefaultMaxUploadBytes)
	}
	if cfg.Storage.S3.UseSSL || cfg.Storage.S3.ForcePathStyle || cfg.Storage.S3.BucketAutoCreate {
		t.Fatalf("Storage.S3 booleans = %#v, want false fallback", cfg.Storage.S3)
	}
	if cfg.Auth.SessionTTL != DefaultAuthSessionTTL {
		t.Fatalf("Auth.SessionTTL = %s, want %s", cfg.Auth.SessionTTL, DefaultAuthSessionTTL)
	}
	if cfg.Auth.RecoveryTTL != DefaultAuthRecoveryTTL ||
		cfg.Auth.SMTP.QueueSize != DefaultAuthSMTPQueueSize ||
		cfg.Auth.SMTP.Timeout != DefaultAuthSMTPTimeout {
		t.Fatalf("Auth recovery/SMTP defaults = %#v", cfg.Auth)
	}
	if cfg.Auth.Mode != AuthModeRequired {
		t.Fatalf("Auth.Mode = %q, want required fail-closed fallback for unknown non-empty mode", cfg.Auth.Mode)
	}
}

func TestLoadFromEnvLoadsTeamRuntimeSettings(t *testing.T) {
	cursorActive := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	cursorOld := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 48))
	mailActive := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))
	mailOld := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{4}, 32))
	values := map[string]string{
		EnvTeamCursorActiveKeyID: " cursor-v2 ",
		EnvTeamCursorKeyring:     " cursor-v2=" + cursorActive + ",cursor-v1=" + cursorOld + " ",
		EnvTeamMailActiveKeyID:   " mail-v2 ",
		EnvTeamMailKeyring:       " mail-v2=" + mailActive + ",mail-v1=" + mailOld + " ",
		EnvTeamInviteAcceptURL:   " https://chat.example.test/invites/accept ",
		EnvAuthSMTPAddr:          "smtp.example.test:587",
		EnvAuthSMTPFrom:          "no-reply@example.test",
		EnvTeamMailWorkerLease:   "45s",
		EnvTeamMailWorkerPoll:    "750ms",
		EnvTeamMailBackoffBase:   "3s",
		EnvTeamMailBackoffMax:    "10m",
	}
	cfg := LoadFromEnv(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})

	if cfg.Team.Cursor.ActiveKeyID != "cursor-v2" ||
		cfg.Team.Cursor.Keyring != strings.TrimSpace(values[EnvTeamCursorKeyring]) {
		t.Fatalf("Team.Cursor = %#v, want trimmed configured values", cfg.Team.Cursor)
	}
	if cfg.Team.Mail.ActiveKeyID != "mail-v2" ||
		cfg.Team.InviteAcceptURLBase != "https://chat.example.test/invites/accept" {
		t.Fatalf("Team mail/url config = %#v", cfg.Team)
	}
	if cfg.Team.MailWorker.LeaseDuration != 45*time.Second ||
		cfg.Team.MailWorker.PollInterval != 750*time.Millisecond ||
		cfg.Team.MailWorker.BackoffBase != 3*time.Second ||
		cfg.Team.MailWorker.BackoffMaximum != 10*time.Minute {
		t.Fatalf("Team.MailWorker = %#v, want overrides", cfg.Team.MailWorker)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsPartialAndInvalidTeamSettings(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
		want   string
	}{
		{
			name:   "cursor active without keyring",
			values: map[string]string{EnvTeamCursorActiveKeyID: "cursor-v1"},
			want:   EnvTeamCursorKeyring,
		},
		{
			name:   "mail keyring without active",
			values: map[string]string{EnvTeamMailKeyring: "mail-v1=ZmFrZQ=="},
			want:   EnvTeamMailActiveKeyID,
		},
		{
			name: "mail keys without acceptance URL",
			values: map[string]string{
				EnvTeamMailActiveKeyID: "mail-v1",
				EnvTeamMailKeyring:     "mail-v1=ZmFrZS1tYWlsLWtleS1ub3QtcHJvZHVjdGlvbiEhISE=",
			},
			want: EnvTeamInviteAcceptURL,
		},
		{
			name:   "acceptance URL without mail keys",
			values: map[string]string{EnvTeamInviteAcceptURL: "https://chat.example.test/invites/accept"},
			want:   EnvTeamMailKeyring,
		},
		{
			name: "team mail without SMTP",
			values: map[string]string{
				EnvTeamMailActiveKeyID: "mail-v1",
				EnvTeamMailKeyring:     "mail-v1=ZmFrZS1tYWlsLWtleS1ub3QtcHJvZHVjdGlvbiEhISE=",
				EnvTeamInviteAcceptURL: "https://chat.example.test/invites/accept",
			},
			want: EnvAuthSMTPAddr,
		},
		{
			name:   "required auth without cursor keys",
			values: map[string]string{EnvAuthMode: AuthModeRequired},
			want:   EnvTeamCursorKeyring,
		},
		{
			name:   "invalid lease",
			values: map[string]string{EnvTeamMailWorkerLease: "not-a-duration"},
			want:   EnvTeamMailWorkerLease,
		},
		{
			name:   "invalid smtp timeout",
			values: map[string]string{EnvAuthSMTPTimeout: "not-a-duration"},
			want:   EnvAuthSMTPTimeout,
		},
		{
			name:   "invalid smtp queue",
			values: map[string]string{EnvAuthSMTPQueueSize: "0"},
			want:   EnvAuthSMTPQueueSize,
		},
		{
			name: "backoff range",
			values: map[string]string{
				EnvTeamMailBackoffBase: "10s",
				EnvTeamMailBackoffMax:  "5s",
			},
			want: EnvTeamMailBackoffMax,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := LoadFromEnv(func(key string) (string, bool) {
				value, ok := tt.values[key]
				return value, ok
			})
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want field %s", err, tt.want)
			}
		})
	}
}

func TestParseBase64KeyringSupportsRotation(t *testing.T) {
	active := bytes.Repeat([]byte{0x11}, 32)
	old := bytes.Repeat([]byte{0x22}, 32)
	encoded := "active=" + base64.StdEncoding.EncodeToString(active) +
		",old=" + base64.StdEncoding.EncodeToString(old)

	keys, err := ParseBase64Keyring(EnvTeamMailKeyring, encoded)
	if err != nil {
		t.Fatalf("ParseBase64Keyring() error = %v", err)
	}
	if !bytes.Equal(keys["active"], active) || !bytes.Equal(keys["old"], old) {
		t.Fatalf("ParseBase64Keyring() keys do not match decoded rotation keyring")
	}
	active[0] = 0
	if keys["active"][0] != 0x11 {
		t.Fatal("ParseBase64Keyring() did not isolate decoded key bytes")
	}
}

func TestParseBase64KeyringErrorsRedactSecretBytes(t *testing.T) {
	secret := "super-secret-key-material-not-base64!"
	_, err := ParseBase64Keyring(
		EnvTeamCursorKeyring,
		"cursor-v1="+secret,
	)
	if err == nil {
		t.Fatal("ParseBase64Keyring() error = nil, want malformed base64 error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("ParseBase64Keyring() leaked key bytes: %v", err)
	}
	if !strings.Contains(err.Error(), EnvTeamCursorKeyring) ||
		!strings.Contains(err.Error(), "cursor-v1") {
		t.Fatalf("ParseBase64Keyring() error = %v, want safe field and key id", err)
	}
}
