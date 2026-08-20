package config

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultAddr                         = ":8080"
	DefaultVersion                      = "dev"
	DefaultDBMaxOpenConns               = 10
	DefaultDBMaxIdleConns               = 5
	DefaultDBConnMaxLifetime            = 30 * time.Minute
	DefaultRedisKeyPrefix               = "mm-chat"
	DefaultRedisRunCancelTTL            = 10 * time.Minute
	DefaultRedisSessionCacheTTL         = 5 * time.Minute
	DefaultRedisRateLimitEnabled        = false
	DefaultRedisRateLimitRequests       = 120
	DefaultRedisRateLimitWindow         = time.Minute
	DefaultMemoryLexicalShadowEnabled   = false
	DefaultMemoryHybridShadowEnabled    = false
	DefaultMemoryToolLoopEnabled        = false
	DefaultMemoryL2SceneShadowEnabled   = false
	DefaultMemoryL2SceneReaderEnabled   = false
	DefaultMemoryL3PersonaShadowEnabled = false
	DefaultMemoryL3PersonaReaderEnabled = false
	DefaultProviderTimeout              = 2 * time.Minute
	DefaultProviderName                 = "Server Default"
	DefaultStorageBackend               = "local"
	DefaultLocalStorageDir              = "./data/files"
	DefaultS3Region                     = "us-east-1"
	DefaultMaxUploadBytes               = int64(25 << 20)
	AuthModeDevelopment                 = "development"
	AuthModeRequired                    = "required"
	DefaultAuthMode                     = AuthModeDevelopment
	DefaultAuthBootstrapUserID          = "00000000-0000-0000-0000-000000000001"
	DefaultAuthBootstrapUserName        = "Owner"
	DefaultAuthSessionTTL               = 7 * 24 * time.Hour
	DefaultAuthRecoveryTTL              = 30 * time.Minute
	DefaultAuthSMTPQueueSize            = 100
	DefaultAuthSMTPTimeout              = 10 * time.Second
	DefaultTeamMailWorkerLease          = 30 * time.Second
	DefaultTeamMailWorkerPoll           = 500 * time.Millisecond
	DefaultTeamMailBackoffBase          = 5 * time.Second
	DefaultTeamMailBackoffMax           = 15 * time.Minute
	DefaultMCPEnabled                   = false
	DefaultMCPRemoteEnabled             = true
	DefaultMCPStdioEnabled              = false
	DefaultMCPRunnerURL                 = "http://mcp-runner:8090"
	DefaultMCPRunnerTokenFile           = "/run/secrets/mm_chat_mcp_runner_token"
	DefaultMCPMarketplaceEnabled        = false
	DefaultMCPMarketplaceBaseURL        = "https://market.lobehub.com"
	DefaultMCPMarketplaceSecretFile     = "/run/secrets/mm_chat_mcp_marketplace_client_secret"
	DefaultMCPMarketplaceTimeout        = 8 * time.Second
	DefaultMCPMarketplaceCacheTTL       = 5 * time.Minute
	DefaultMCPPrivateServerLimit        = 20
	DefaultMCPConversationLimit         = 8
	DefaultMCPMaxExposedTools           = 32
	DefaultMCPMaxCallsPerRun            = 32
	DefaultMCPMaxRoundsPerRun           = 8
	DefaultMCPMaxConcurrentPerUser      = 4
	DefaultMCPMaxOAuthFlows             = 5
	DefaultMCPCallTimeout               = 30 * time.Second
	DefaultMCPRunTimeout                = 120 * time.Second
	DefaultMCPAuditRetention            = 90 * 24 * time.Hour
	DefaultMCPCleanupInterval           = time.Hour
	DefaultAgentLocalEnabled            = false
	DefaultAgentLocalRuntimeRoot        = "/var/lib/mm-chat/agent-skills"
	DefaultAgentLocalWorkspaceRoot      = "/workspace"
	DefaultAgentLocalShell              = "/bin/bash"
	DefaultAgentLocalApprovalMode       = "smart"
	DefaultAgentLocalCallTimeout        = 30 * time.Second
	DefaultAgentLocalRunTimeout         = 5 * time.Minute
	DefaultAgentLocalMaxOutputBytes     = int64(1 << 20)
	DefaultAgentLocalMaxCalls           = 32
	DefaultAgentLocalMaxRounds          = 8
	DefaultAgentLocalMaxConcurrent      = 2
	maximumAuthSMTPQueueSize            = 10_000

	EnvAddr                     = "MM_CHAT_ADDR"
	EnvVersion                  = "MM_CHAT_VERSION"
	EnvDatabaseURL              = "DATABASE_URL"
	EnvDBMaxOpenConns           = "DB_MAX_OPEN_CONNS"
	EnvDBMaxIdleConns           = "DB_MAX_IDLE_CONNS"
	EnvDBConnMaxLifetime        = "DB_CONN_MAX_LIFETIME"
	EnvRedisURL                 = "REDIS_URL"
	EnvRedisKeyPrefix           = "REDIS_KEY_PREFIX"
	EnvRedisRunCancelTTL        = "REDIS_RUN_CANCEL_TTL"
	EnvRedisSessionCacheTTL     = "REDIS_SESSION_CACHE_TTL"
	EnvRedisRateLimitEnabled    = "REDIS_RATE_LIMIT_ENABLED"
	EnvRedisRateLimitRequests   = "REDIS_RATE_LIMIT_REQUESTS"
	EnvRedisRateLimitWindow     = "REDIS_RATE_LIMIT_WINDOW"
	EnvProviderTimeout          = "PROVIDER_TIMEOUT"
	EnvProviderSecretKeyring    = "PROVIDER_SECRET_KEYRING_FILE"
	EnvBYOKPrivateKeyPEM        = "BYOK_PRIVATE_KEY_PEM"
	EnvBYOKKeyID                = "BYOK_KEY_ID"
	EnvBYOKAllowEphemeralKey    = "BYOK_ALLOW_EPHEMERAL_KEY"
	EnvStorageBackend           = "STORAGE_BACKEND"
	EnvLocalStorageDir          = "LOCAL_STORAGE_DIR"
	EnvS3Endpoint               = "S3_ENDPOINT"
	EnvS3Bucket                 = "S3_BUCKET"
	EnvS3Region                 = "S3_REGION"
	EnvS3AccessKeyID            = "S3_ACCESS_KEY_ID"
	EnvS3SecretAccessKey        = "S3_SECRET_ACCESS_KEY"
	EnvS3UseSSL                 = "S3_USE_SSL"
	EnvS3ForcePathStyle         = "S3_FORCE_PATH_STYLE"
	EnvS3BucketAutoCreate       = "S3_BUCKET_AUTO_CREATE"
	EnvMaxUploadBytes           = "MAX_UPLOAD_BYTES"
	EnvRAGSourceGatewayToken    = "RAG_SOURCE_GATEWAY_TOKEN"
	EnvAuthMode                 = "AUTH_MODE"
	EnvAuthBootstrapUserID      = "AUTH_BOOTSTRAP_USER_ID"
	EnvAuthBootstrapUserName    = "AUTH_BOOTSTRAP_DISPLAY_NAME"
	EnvAuthSessionTTL           = "AUTH_SESSION_TTL"
	EnvAuthRecoveryTTL          = "AUTH_RECOVERY_TTL"
	EnvAuthSMTPAddr             = "AUTH_SMTP_ADDR"
	EnvAuthSMTPUsername         = "AUTH_SMTP_USERNAME"
	EnvAuthSMTPPassword         = "AUTH_SMTP_PASSWORD"
	EnvAuthSMTPFrom             = "AUTH_SMTP_FROM"
	EnvAuthSMTPQueueSize        = "AUTH_SMTP_QUEUE_SIZE"
	EnvAuthSMTPTimeout          = "AUTH_SMTP_TIMEOUT"
	EnvTeamCursorActiveKeyID    = "TEAM_CURSOR_ACTIVE_KEY_ID"
	EnvTeamCursorKeyring        = "TEAM_CURSOR_KEYRING"
	EnvTeamMailActiveKeyID      = "TEAM_MAIL_ACTIVE_KEY_ID"
	EnvTeamMailKeyring          = "TEAM_MAIL_KEYRING"
	EnvTeamInviteAcceptURL      = "TEAM_INVITE_ACCEPT_URL_BASE"
	EnvTeamMailWorkerLease      = "TEAM_MAIL_WORKER_LEASE_DURATION"
	EnvTeamMailWorkerPoll       = "TEAM_MAIL_WORKER_POLL_INTERVAL"
	EnvTeamMailBackoffBase      = "TEAM_MAIL_WORKER_BACKOFF_BASE"
	EnvTeamMailBackoffMax       = "TEAM_MAIL_WORKER_BACKOFF_MAX"
	EnvMemoryLexicalShadow      = "MEMORY_LEXICAL_SHADOW_ENABLED"
	EnvMemoryHybridShadow       = "MEMORY_HYBRID_SHADOW_ENABLED"
	EnvMemoryToolLoop           = "MEMORY_TOOL_LOOP_ENABLED"
	EnvMemoryToolLoopCanary     = "MEMORY_TOOL_LOOP_CANARY_USER_IDS"
	EnvMemoryL2SceneShadow      = "MEMORY_L2_SCENE_SHADOW_ENABLED"
	EnvMemoryL2SceneReader      = "MEMORY_L2_SCENE_READER_ENABLED"
	EnvMemoryL3PersonaShadow    = "MEMORY_L3_PERSONA_SHADOW_ENABLED"
	EnvMemoryL3PersonaReader    = "MEMORY_L3_PERSONA_READER_ENABLED"
	EnvMCPEnabled               = "MCP_ENABLED"
	EnvMCPRemoteEnabled         = "MCP_REMOTE_ENABLED"
	EnvMCPStdioEnabled          = "MCP_STDIO_ENABLED"
	EnvMCPManifestFile          = "MCP_MANIFEST_FILE"
	EnvMCPRunnerURL             = "MCP_RUNNER_URL"
	EnvMCPRunnerTokenFile       = "MCP_RUNNER_TOKEN_FILE"
	EnvMCPOAuthCallbackURL      = "MCP_OAUTH_CALLBACK_URL"
	EnvMCPPrivateServerLimit    = "MCP_PRIVATE_SERVER_LIMIT"
	EnvMCPConversationLimit     = "MCP_CONVERSATION_SERVER_LIMIT"
	EnvMCPMaxExposedTools       = "MCP_MAX_EXPOSED_TOOLS"
	EnvMCPMaxCallsPerRun        = "MCP_MAX_CALLS_PER_RUN"
	EnvMCPMaxRoundsPerRun       = "MCP_MAX_ROUNDS_PER_RUN"
	EnvMCPMaxConcurrent         = "MCP_MAX_CONCURRENT_PER_USER"
	EnvMCPMaxOAuthFlows         = "MCP_MAX_PENDING_OAUTH_FLOWS"
	EnvMCPCallTimeout           = "MCP_CALL_TIMEOUT"
	EnvMCPRunTimeout            = "MCP_RUN_TIMEOUT"
	EnvMCPAuditRetention        = "MCP_AUDIT_RETENTION"
	EnvMCPCleanupInterval       = "MCP_CLEANUP_INTERVAL"
	EnvMCPMarketplaceEnabled    = "MCP_MARKETPLACE_ENABLED"
	EnvMCPMarketplaceBaseURL    = "MCP_MARKETPLACE_BASE_URL"
	EnvMCPMarketplaceClientID   = "MCP_MARKETPLACE_CLIENT_ID"
	EnvMCPMarketplaceSecretFile = "MCP_MARKETPLACE_CLIENT_SECRET_FILE"
	EnvMCPMarketplaceTimeout    = "MCP_MARKETPLACE_TIMEOUT"
	EnvMCPMarketplaceCacheTTL   = "MCP_MARKETPLACE_CACHE_TTL"
	EnvAgentLocalEnabled        = "AGENT_LOCAL_RUNTIME_ENABLED"
	EnvAgentLocalRuntimeRoot    = "AGENT_LOCAL_RUNTIME_ROOT"
	EnvAgentLocalWorkspaceRoot  = "AGENT_LOCAL_WORKSPACE_ROOT"
	EnvAgentLocalHostRoot       = "AGENT_LOCAL_WORKSPACE_HOST_ROOT"
	EnvAgentLocalShell          = "AGENT_LOCAL_SHELL"
	EnvAgentLocalApprovalMode   = "AGENT_LOCAL_APPROVAL_MODE"
	EnvAgentLocalCallTimeout    = "AGENT_LOCAL_CALL_TIMEOUT"
	EnvAgentLocalRunTimeout     = "AGENT_LOCAL_RUN_TIMEOUT"
	EnvAgentLocalMaxOutputBytes = "AGENT_LOCAL_MAX_OUTPUT_BYTES"
	EnvAgentLocalMaxCalls       = "AGENT_LOCAL_MAX_CALLS_PER_RUN"
	EnvAgentLocalMaxRounds      = "AGENT_LOCAL_MAX_ROUNDS_PER_RUN"
	EnvAgentLocalMaxConcurrent  = "AGENT_LOCAL_MAX_CONCURRENT"
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

	Provider        ProviderConfig
	ProviderSecrets ProviderSecretConfig
	BYOK            BYOKConfig
	Storage         StorageConfig
	RAG             RAGConfig
	Memory          MemoryConfig
	Auth            AuthConfig
	Team            TeamConfig
	MCP             MCPConfig
	AgentLocal      AgentLocalConfig
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

// ProviderConfig contains non-secret model-provider transport limits. Provider
// endpoint metadata and credentials are Postgres/vault-owned.
type ProviderConfig struct {
	Timeout time.Duration
}

// ProviderSecretConfig points to the read-only Docker Secret keyring used for
// restart-stable provider credential encryption at rest. Key material is read
// from the file and must never be copied into process environment values.
type ProviderSecretConfig struct {
	KeyringFile string
}

// BYOKConfig contains browser-encryption key settings. Private key material
// must never be logged or serialized into API responses.
type BYOKConfig struct {
	PrivateKeyPEM     string
	KeyID             string
	AllowEphemeralKey bool
}

// StorageConfig contains file-byte storage settings. Object-store secrets must
// never be logged or serialized into API responses.
type StorageConfig struct {
	Backend        string
	LocalDir       string
	S3             S3Config
	MaxUploadBytes int64
}

// RAGConfig contains infrastructure-only settings for the private worker-to-Go
// boundary. Provider credentials are resolved from Postgres/vault at runtime.
type RAGConfig struct {
	SourceGatewayToken string
}

// MemoryConfig contains default-off Memory reader rollout switches.
type MemoryConfig struct {
	LexicalShadowEnabled   bool
	HybridShadowEnabled    bool
	ToolLoopEnabled        bool
	ToolLoopCanaryUserIDs  []string
	L2SceneShadowEnabled   bool
	L2SceneReaderEnabled   bool
	L3PersonaShadowEnabled bool
	L3PersonaReaderEnabled bool
	invalidCanaryUserIDs   bool
}

type MCPConfig struct {
	Enabled               bool
	RemoteEnabled         bool
	StdioEnabled          bool
	MarketplaceEnabled    bool
	MarketplaceBaseURL    string
	MarketplaceClientID   string
	MarketplaceSecretFile string
	MarketplaceTimeout    time.Duration
	MarketplaceCacheTTL   time.Duration
	ManifestFile          string
	RunnerURL             string
	RunnerTokenFile       string
	OAuthCallbackURL      string
	PrivateServerLimit    int
	ConversationLimit     int
	MaxExposedTools       int
	MaxCallsPerRun        int
	MaxRoundsPerRun       int
	MaxConcurrentPerUser  int
	MaxOAuthFlows         int
	CallTimeout           time.Duration
	RunTimeout            time.Duration
	AuditRetention        time.Duration
	CleanupInterval       time.Duration
}

// AgentLocalConfig enables Hermes-style local_direct Skill execution as the
// ordinary Backend user. Its limits are guardrails, not a Sandbox boundary.
type AgentLocalConfig struct {
	Enabled           bool
	RuntimeRoot       string
	WorkspaceRoot     string
	WorkspaceHostRoot string
	Shell             string
	ApprovalMode      string
	CallTimeout       time.Duration
	RunTimeout        time.Duration
	MaxOutputBytes    int64
	MaxCalls          int
	MaxRounds         int
	MaxConcurrent     int
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
	if cfg.Memory.invalidCanaryUserIDs {
		return fmt.Errorf("%s must be a comma-separated list of UUIDs", EnvMemoryToolLoopCanary)
	}
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
	if err := validateMCPConfig(cfg.MCP); err != nil {
		return err
	}
	if err := validateAgentLocalConfig(cfg.AgentLocal); err != nil {
		return err
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
	memoryCanaryUserIDs, invalidMemoryCanaryUserIDs :=
		loadMemoryToolLoopCanaryUserIDs(lookup)
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
			Timeout: durationEnvOrDefault(lookup, EnvProviderTimeout, DefaultProviderTimeout),
		},
		ProviderSecrets: ProviderSecretConfig{
			KeyringFile: optionalEnv(lookup, EnvProviderSecretKeyring),
		},

		BYOK: BYOKConfig{
			PrivateKeyPEM:     optionalEnv(lookup, EnvBYOKPrivateKeyPEM),
			KeyID:             optionalEnv(lookup, EnvBYOKKeyID),
			AllowEphemeralKey: boolEnvOrDefault(lookup, EnvBYOKAllowEphemeralKey, false),
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

		RAG: RAGConfig{
			SourceGatewayToken: optionalEnv(lookup, EnvRAGSourceGatewayToken),
		},
		Memory: MemoryConfig{
			LexicalShadowEnabled: boolEnvOrDefault(
				lookup,
				EnvMemoryLexicalShadow,
				DefaultMemoryLexicalShadowEnabled,
			),
			HybridShadowEnabled: boolEnvOrDefault(
				lookup,
				EnvMemoryHybridShadow,
				DefaultMemoryHybridShadowEnabled,
			),
			ToolLoopEnabled: boolEnvOrDefault(
				lookup,
				EnvMemoryToolLoop,
				DefaultMemoryToolLoopEnabled,
			),
			ToolLoopCanaryUserIDs: memoryCanaryUserIDs,
			L2SceneShadowEnabled: boolEnvOrDefault(
				lookup,
				EnvMemoryL2SceneShadow,
				DefaultMemoryL2SceneShadowEnabled,
			),
			L2SceneReaderEnabled: boolEnvOrDefault(
				lookup,
				EnvMemoryL2SceneReader,
				DefaultMemoryL2SceneReaderEnabled,
			),
			L3PersonaShadowEnabled: boolEnvOrDefault(
				lookup,
				EnvMemoryL3PersonaShadow,
				DefaultMemoryL3PersonaShadowEnabled,
			),
			L3PersonaReaderEnabled: boolEnvOrDefault(
				lookup,
				EnvMemoryL3PersonaReader,
				DefaultMemoryL3PersonaReaderEnabled,
			),
			invalidCanaryUserIDs: invalidMemoryCanaryUserIDs,
		},
		MCP: MCPConfig{
			Enabled:               boolEnvOrDefault(lookup, EnvMCPEnabled, DefaultMCPEnabled),
			RemoteEnabled:         boolEnvOrDefault(lookup, EnvMCPRemoteEnabled, DefaultMCPRemoteEnabled),
			StdioEnabled:          boolEnvOrDefault(lookup, EnvMCPStdioEnabled, DefaultMCPStdioEnabled),
			MarketplaceEnabled:    boolEnvOrDefault(lookup, EnvMCPMarketplaceEnabled, DefaultMCPMarketplaceEnabled),
			MarketplaceBaseURL:    envOrDefault(lookup, EnvMCPMarketplaceBaseURL, DefaultMCPMarketplaceBaseURL),
			MarketplaceClientID:   optionalEnv(lookup, EnvMCPMarketplaceClientID),
			MarketplaceSecretFile: envOrDefault(lookup, EnvMCPMarketplaceSecretFile, DefaultMCPMarketplaceSecretFile),
			MarketplaceTimeout:    durationEnvOrDefault(lookup, EnvMCPMarketplaceTimeout, DefaultMCPMarketplaceTimeout),
			MarketplaceCacheTTL:   durationEnvOrDefault(lookup, EnvMCPMarketplaceCacheTTL, DefaultMCPMarketplaceCacheTTL),
			ManifestFile:          optionalEnv(lookup, EnvMCPManifestFile),
			RunnerURL:             envOrDefault(lookup, EnvMCPRunnerURL, DefaultMCPRunnerURL),
			RunnerTokenFile:       envOrDefault(lookup, EnvMCPRunnerTokenFile, DefaultMCPRunnerTokenFile),
			OAuthCallbackURL:      optionalEnv(lookup, EnvMCPOAuthCallbackURL),
			PrivateServerLimit:    intEnvOrDefault(lookup, EnvMCPPrivateServerLimit, DefaultMCPPrivateServerLimit),
			ConversationLimit:     intEnvOrDefault(lookup, EnvMCPConversationLimit, DefaultMCPConversationLimit),
			MaxExposedTools:       intEnvOrDefault(lookup, EnvMCPMaxExposedTools, DefaultMCPMaxExposedTools),
			MaxCallsPerRun:        intEnvOrDefault(lookup, EnvMCPMaxCallsPerRun, DefaultMCPMaxCallsPerRun),
			MaxRoundsPerRun:       intEnvOrDefault(lookup, EnvMCPMaxRoundsPerRun, DefaultMCPMaxRoundsPerRun),
			MaxConcurrentPerUser:  intEnvOrDefault(lookup, EnvMCPMaxConcurrent, DefaultMCPMaxConcurrentPerUser),
			MaxOAuthFlows:         intEnvOrDefault(lookup, EnvMCPMaxOAuthFlows, DefaultMCPMaxOAuthFlows),
			CallTimeout:           durationEnvOrDefault(lookup, EnvMCPCallTimeout, DefaultMCPCallTimeout),
			RunTimeout:            durationEnvOrDefault(lookup, EnvMCPRunTimeout, DefaultMCPRunTimeout),
			AuditRetention:        durationEnvOrDefault(lookup, EnvMCPAuditRetention, DefaultMCPAuditRetention),
			CleanupInterval:       durationEnvOrDefault(lookup, EnvMCPCleanupInterval, DefaultMCPCleanupInterval),
		},
		AgentLocal: AgentLocalConfig{
			Enabled:           boolEnvOrDefault(lookup, EnvAgentLocalEnabled, DefaultAgentLocalEnabled),
			RuntimeRoot:       envOrDefault(lookup, EnvAgentLocalRuntimeRoot, DefaultAgentLocalRuntimeRoot),
			WorkspaceRoot:     envOrDefault(lookup, EnvAgentLocalWorkspaceRoot, DefaultAgentLocalWorkspaceRoot),
			WorkspaceHostRoot: optionalEnv(lookup, EnvAgentLocalHostRoot),
			Shell:             envOrDefault(lookup, EnvAgentLocalShell, DefaultAgentLocalShell),
			ApprovalMode:      strings.ToLower(envOrDefault(lookup, EnvAgentLocalApprovalMode, DefaultAgentLocalApprovalMode)),
			CallTimeout:       durationEnvOrDefault(lookup, EnvAgentLocalCallTimeout, DefaultAgentLocalCallTimeout),
			RunTimeout:        durationEnvOrDefault(lookup, EnvAgentLocalRunTimeout, DefaultAgentLocalRunTimeout),
			MaxOutputBytes:    int64EnvOrDefault(lookup, EnvAgentLocalMaxOutputBytes, DefaultAgentLocalMaxOutputBytes),
			MaxCalls:          intEnvOrDefault(lookup, EnvAgentLocalMaxCalls, DefaultAgentLocalMaxCalls),
			MaxRounds:         intEnvOrDefault(lookup, EnvAgentLocalMaxRounds, DefaultAgentLocalMaxRounds),
			MaxConcurrent:     intEnvOrDefault(lookup, EnvAgentLocalMaxConcurrent, DefaultAgentLocalMaxConcurrent),
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

func validateAgentLocalConfig(config AgentLocalConfig) error {
	if !config.Enabled {
		return nil
	}
	for _, root := range []struct {
		name  string
		value string
	}{
		{name: EnvAgentLocalRuntimeRoot, value: config.RuntimeRoot},
		{name: EnvAgentLocalWorkspaceRoot, value: config.WorkspaceRoot},
	} {
		if !filepath.IsAbs(root.value) || filepath.Clean(root.value) != root.value ||
			root.value == string(filepath.Separator) {
			return fmt.Errorf("%s must be a clean absolute non-root path", root.name)
		}
	}
	if config.WorkspaceHostRoot != "" &&
		(!filepath.IsAbs(config.WorkspaceHostRoot) ||
			filepath.Clean(config.WorkspaceHostRoot) != config.WorkspaceHostRoot ||
			config.WorkspaceHostRoot == string(filepath.Separator)) {
		return fmt.Errorf(
			"%s must be empty or a clean absolute non-root path",
			EnvAgentLocalHostRoot,
		)
	}
	if !filepath.IsAbs(config.Shell) || filepath.Clean(config.Shell) != config.Shell {
		return fmt.Errorf("%s must be a clean absolute path", EnvAgentLocalShell)
	}
	if config.ApprovalMode != "smart" && config.ApprovalMode != "off" {
		return fmt.Errorf("%s must be smart or off", EnvAgentLocalApprovalMode)
	}
	if config.CallTimeout < time.Second || config.CallTimeout > 10*time.Minute {
		return fmt.Errorf("%s must be between 1s and 10m", EnvAgentLocalCallTimeout)
	}
	if config.RunTimeout < config.CallTimeout || config.RunTimeout > 30*time.Minute {
		return fmt.Errorf("%s must be at least %s and at most 30m", EnvAgentLocalRunTimeout, EnvAgentLocalCallTimeout)
	}
	if config.MaxOutputBytes < 1024 || config.MaxOutputBytes > 8<<20 {
		return fmt.Errorf("%s must be between 1024 and 8388608", EnvAgentLocalMaxOutputBytes)
	}
	if config.MaxCalls < 1 || config.MaxCalls > 128 {
		return fmt.Errorf("%s must be between 1 and 128", EnvAgentLocalMaxCalls)
	}
	if config.MaxRounds < 1 || config.MaxRounds > 32 {
		return fmt.Errorf("%s must be between 1 and 32", EnvAgentLocalMaxRounds)
	}
	if config.MaxConcurrent < 1 || config.MaxConcurrent > 32 {
		return fmt.Errorf("%s must be between 1 and 32", EnvAgentLocalMaxConcurrent)
	}
	return nil
}

func validateMCPConfig(config MCPConfig) error {
	if config.AuditRetention == 0 {
		config.AuditRetention = DefaultMCPAuditRetention
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = DefaultMCPCleanupInterval
	}
	if config.AuditRetention < 24*time.Hour || config.AuditRetention > 365*24*time.Hour {
		return fmt.Errorf("%s must be between 24h and 8760h", EnvMCPAuditRetention)
	}
	if config.CleanupInterval < time.Minute || config.CleanupInterval > 24*time.Hour {
		return fmt.Errorf("%s must be between 1m and 24h", EnvMCPCleanupInterval)
	}
	if config.MarketplaceEnabled && !config.Enabled {
		return fmt.Errorf("%s requires %s", EnvMCPMarketplaceEnabled, EnvMCPEnabled)
	}
	if !config.Enabled {
		return nil
	}
	if config.MarketplaceEnabled {
		if !config.RemoteEnabled {
			return fmt.Errorf("%s requires %s", EnvMCPMarketplaceEnabled, EnvMCPRemoteEnabled)
		}
		marketplaceURL, err := url.Parse(strings.TrimSpace(config.MarketplaceBaseURL))
		if err != nil || marketplaceURL.Scheme != "https" || marketplaceURL.Host == "" ||
			marketplaceURL.User != nil || marketplaceURL.RawQuery != "" || marketplaceURL.Fragment != "" {
			return fmt.Errorf("%s must be an HTTPS URL", EnvMCPMarketplaceBaseURL)
		}
		if strings.TrimSpace(config.MarketplaceClientID) == "" || len(config.MarketplaceClientID) > 2048 {
			return fmt.Errorf("%s is required when Marketplace is enabled", EnvMCPMarketplaceClientID)
		}
		if !strings.HasPrefix(strings.TrimSpace(config.MarketplaceSecretFile), "/run/secrets/") {
			return fmt.Errorf("%s must be under /run/secrets", EnvMCPMarketplaceSecretFile)
		}
		if config.MarketplaceTimeout < time.Second || config.MarketplaceTimeout > 30*time.Second {
			return fmt.Errorf("%s must be between 1s and 30s", EnvMCPMarketplaceTimeout)
		}
		if config.MarketplaceCacheTTL < 10*time.Second || config.MarketplaceCacheTTL > time.Hour {
			return fmt.Errorf("%s must be between 10s and 1h", EnvMCPMarketplaceCacheTTL)
		}
	}
	limits := []struct {
		name    string
		value   int
		minimum int
		maximum int
	}{
		{EnvMCPPrivateServerLimit, config.PrivateServerLimit, 1, 100},
		{EnvMCPConversationLimit, config.ConversationLimit, 1, 32},
		{EnvMCPMaxExposedTools, config.MaxExposedTools, 2, 128},
		{EnvMCPMaxCallsPerRun, config.MaxCallsPerRun, 1, 128},
		{EnvMCPMaxRoundsPerRun, config.MaxRoundsPerRun, 1, 32},
		{EnvMCPMaxConcurrent, config.MaxConcurrentPerUser, 1, 32},
		{EnvMCPMaxOAuthFlows, config.MaxOAuthFlows, 1, 20},
	}
	for _, limit := range limits {
		if limit.value < limit.minimum || limit.value > limit.maximum {
			return fmt.Errorf("%s must be between %d and %d", limit.name, limit.minimum, limit.maximum)
		}
	}
	if config.CallTimeout <= 0 || config.CallTimeout > 2*time.Minute {
		return fmt.Errorf("%s must be between 1ns and 2m", EnvMCPCallTimeout)
	}
	if config.RunTimeout < config.CallTimeout || config.RunTimeout > 10*time.Minute {
		return fmt.Errorf("%s must be at least %s and at most 10m", EnvMCPRunTimeout, EnvMCPCallTimeout)
	}
	if config.StdioEnabled {
		runner, err := url.Parse(strings.TrimSpace(config.RunnerURL))
		if err != nil || runner.Scheme != "http" || runner.Host == "" || runner.User != nil ||
			runner.RawQuery != "" || runner.Fragment != "" {
			return fmt.Errorf("%s must be an internal HTTP URL", EnvMCPRunnerURL)
		}
		if !strings.HasPrefix(strings.TrimSpace(config.RunnerTokenFile), "/run/secrets/") {
			return fmt.Errorf("%s must be under /run/secrets", EnvMCPRunnerTokenFile)
		}
	}
	if callback := strings.TrimSpace(config.OAuthCallbackURL); callback != "" {
		parsed, err := url.Parse(callback)
		if err != nil || parsed == nil {
			return fmt.Errorf("%s must be a public HTTPS or exact loopback HTTP URL", EnvMCPOAuthCallbackURL)
		}
		host := strings.ToLower(parsed.Hostname())
		loopbackHTTP := parsed.Scheme == "http" &&
			(host == "localhost" || host == "127.0.0.1" || host == "::1")
		if (parsed.Scheme != "https" && !loopbackHTTP) || parsed.Host == "" ||
			parsed.User != nil || parsed.Fragment != "" {
			return fmt.Errorf("%s must be a public HTTPS or exact loopback HTTP URL", EnvMCPOAuthCallbackURL)
		}
	}
	return nil
}

var canonicalUUIDRE = regexp.MustCompile(
	`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`,
)

func loadMemoryToolLoopCanaryUserIDs(
	lookup func(string) (string, bool),
) ([]string, bool) {
	value, configured := optionalLookup(lookup, EnvMemoryToolLoopCanary)
	if !configured {
		return nil, false
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		userID := strings.ToLower(strings.TrimSpace(part))
		if !canonicalUUIDRE.MatchString(userID) {
			return nil, true
		}
		if _, duplicate := seen[userID]; duplicate {
			return nil, true
		}
		seen[userID] = struct{}{}
		result = append(result, userID)
	}
	return result, false
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
