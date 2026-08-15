package main

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"neo-chat/mm-chat/backend/internal/agentactivation"
	"neo-chat/mm-chat/backend/internal/agentorchestrator"
	"neo-chat/mm-chat/backend/internal/agentproductcanary"
	"neo-chat/mm-chat/backend/internal/agentrootcanary"
	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/database"
)

const (
	envEnabled             = "AGENT_PRODUCT_CANARY_ENABLED"
	envRuntimeEnabled      = "AGENT_RUNTIME_ENABLED"
	envSchedulerEnabled    = "AGENT_SCHEDULER_ENABLED"
	envLearningEnabled     = "AGENT_LEARNING_ENABLED"
	envSkillInstallEnabled = "AGENT_SKILL_INSTALL_ENABLED"
	envDatabaseURL         = "AGENT_PRODUCT_CANARY_DATABASE_URL"
	envActivationID        = "AGENT_PRODUCT_CANARY_ACTIVATION_ID"
	envRunnerURL           = "AGENT_PRODUCT_CANARY_RUNNER_URL"
	envRunnerID            = "AGENT_PRODUCT_CANARY_RUNNER_ID"
	envServerName          = "AGENT_PRODUCT_CANARY_SERVER_NAME"
	envCallerIdentity      = "AGENT_PRODUCT_CANARY_CLIENT_IDENTITY"
	envClientCertificate   = "AGENT_PRODUCT_CANARY_CLIENT_CERT_FILE"
	envClientKey           = "AGENT_PRODUCT_CANARY_CLIENT_KEY_FILE"
	envServerCA            = "AGENT_PRODUCT_CANARY_SERVER_CA_FILE"
	envReleaseManifest     = "AGENT_PRODUCT_CANARY_RELEASE_MANIFEST_FILE"
	envProductionPolicy    = "AGENT_PRODUCT_CANARY_PRODUCTION_POLICY_FILE"
	envActivationRecord    = "AGENT_PRODUCT_CANARY_ACTIVATION_FILE"
	envCanaryPlan          = "AGENT_PRODUCT_CANARY_PLAN_FILE"
	envAuthorityPrivateKey = "AGENT_PRODUCT_CANARY_AUTHORITY_PRIVATE_KEY_FILE"
	envAuthorityPublicKey  = "AGENT_PRODUCT_CANARY_AUTHORITY_PUBLIC_KEY_FILE"
	envReleaseCommit       = "AGENT_PRODUCT_CANARY_RELEASE_GIT_COMMIT"
	envClaimOwner          = "AGENT_PRODUCT_CANARY_CLAIM_OWNER"
	envPollInterval        = "AGENT_PRODUCT_CANARY_POLL_INTERVAL"
	envClaimTTL            = "AGENT_PRODUCT_CANARY_CLAIM_TTL"
	envRPCTimeout          = "AGENT_PRODUCT_CANARY_RPC_TIMEOUT"
	envAuthorityTTL        = "AGENT_PRODUCT_CANARY_AUTHORITY_TTL"
	envBatchSize           = "AGENT_PRODUCT_CANARY_BATCH_SIZE"
)

var prerequisiteFlags = []string{
	"AGENT_RUNNER_CONTROL_ENABLED", "AGENT_ROOT_RUN_CANARY_ENABLED",
	"AGENT_BROKER_ARTIFACT_CANARY_ENABLED", "AGENT_PROJECT_MUTATION_CANARY_ENABLED",
	"AGENT_CHILD_CANARY_ENABLED", "AGENT_CRON_WORKER_ENABLED",
	"AGENT_DRAFT_LEARNING_WORKER_ENABLED",
}

var activationIDPattern = regexp.MustCompile(`^activation_[a-z0-9]{16,64}$`)

type workerConfig struct {
	databaseURL, activationID, runnerURL, runnerID, serverName, callerIdentity string
	clientCertificate, clientKey, serverCA, releaseManifest                    string
	productionPolicy, activationRecord, canaryPlan                             string
	authorityPrivateKey, authorityPublicKey, releaseCommit, claimOwner         string
	pollInterval, claimTTL, rpcTimeout, authorityTTL                           time.Duration
	batchSize                                                                  int
}

type evidenceGate struct {
	config agentactivation.ProductCanaryConfig
}

func (gate evidenceGate) Verify(now time.Time) error {
	decision, err := agentactivation.VerifyProductCanary(gate.config, now)
	if err != nil || !decision.Ready {
		return agentproductcanary.ErrUnavailable
	}
	return nil
}

type rootFactory struct {
	config       agentrootcanary.Config
	gate         agentrootcanary.Gate
	orchestrator agentrootcanary.Orchestrator
	authority    agentrootcanary.AuthorityService
	runnerState  agentrootcanary.RunnerRepository
	client       agentrootcanary.RunnerClient
	terminal     agentrootcanary.TerminalRepository
}

func (factory rootFactory) Build(plan agentrootcanary.Plan) (*agentrootcanary.Service, error) {
	return agentrootcanary.NewService(factory.config, plan, factory.gate, factory.orchestrator,
		factory.authority, factory.runnerState, factory.client, factory.terminal)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(os.Args[1:], os.LookupEnv, logger); err != nil {
		logger.Error("agent_product_canary_exit", slog.String("error", sanitizedError(err)))
		os.Exit(1)
	}
}

func run(args []string, lookup func(string) (string, bool), logger *slog.Logger) error {
	if len(args) != 1 || (args[0] != "run" && args[0] != "healthcheck") {
		return errors.New("usage: agent-runtime-product-canary run | agent-runtime-product-canary healthcheck")
	}
	resolved, err := loadWorkerConfig(lookup)
	if err != nil {
		return err
	}
	plan, err := agentproductcanary.LoadPlan(resolved.canaryPlan)
	if err != nil || plan.ActivationID != resolved.activationID {
		return errors.New("Product canary plan invalid")
	}
	privateKey, err := agentrunner.LoadEd25519PrivateKey(resolved.authorityPrivateKey)
	if err != nil {
		return errors.New("Product canary authority key invalid")
	}
	publicKey, err := agentrunner.LoadEd25519PublicKey(resolved.authorityPublicKey)
	if err != nil || !privateKey.Public().(ed25519.PublicKey).Equal(publicKey) {
		return errors.New("Product canary authority key mismatch")
	}
	gate := evidenceGate{config: agentactivation.ProductCanaryConfig{
		Config: agentactivation.Config{PolicyFile: resolved.productionPolicy,
			RecordFile: resolved.activationRecord, ReleaseManifestFile: resolved.releaseManifest,
			ClientCertificateFile: resolved.clientCertificate, ServerCAFile: resolved.serverCA,
			Endpoint: resolved.runnerURL, RunnerID: resolved.runnerID, ServerName: resolved.serverName,
			CallerIdentity: resolved.callerIdentity, ReleaseCommit: resolved.releaseCommit},
		ActivationID: resolved.activationID, CanaryPlanFile: resolved.canaryPlan,
		AuthorityPublicKeyFile: resolved.authorityPublicKey}}
	if err := gate.Verify(time.Now().UTC()); err != nil {
		return errors.New("Product canary activation unavailable")
	}
	openCtx, cancelOpen := context.WithTimeout(context.Background(), 10*time.Second)
	db, err := database.Open(openCtx, config.Config{DatabaseURL: resolved.databaseURL,
		DBMaxOpenConns: 4, DBMaxIdleConns: 2, DBConnMaxLifetime: 30 * time.Minute})
	cancelOpen()
	if err != nil || db == nil || db.SQL() == nil {
		return errors.New("Product canary database unavailable")
	}
	defer db.Close()
	roleCtx, cancelRole := context.WithTimeout(context.Background(), 10*time.Second)
	err = verifyDatabaseRole(roleCtx, db.SQL())
	cancelRole()
	if err != nil {
		return err
	}
	client, err := agentrunner.NewRPCClient(resolved.runnerURL, agentrunner.ClientTLSFiles{
		CertificateFile: resolved.clientCertificate, KeyFile: resolved.clientKey,
		ServerCAFile: resolved.serverCA, ServerName: resolved.serverName,
		ClientIdentity: resolved.callerIdentity,
	}, resolved.rpcTimeout)
	if err != nil {
		return errors.New("Product canary Runner mTLS configuration invalid")
	}
	runnerRepository := agentrunner.NewPostgresControlRepository(db.SQL())
	authority, err := agentrunner.NewControlService(runnerRepository, privateKey)
	if err != nil {
		return errors.New("Product canary authority service invalid")
	}
	factory := rootFactory{config: agentrootcanary.Config{RunnerID: resolved.runnerID,
		CallerIdentity: resolved.callerIdentity, PollInterval: resolved.pollInterval,
		AuthorityTTL: resolved.authorityTTL, BatchSize: resolved.batchSize}, gate: gate,
		orchestrator: agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(db.SQL())),
		authority:    authority, runnerState: runnerRepository, client: client,
		terminal: agentrootcanary.NewPostgresProductTerminalRepository(db.SQL())}
	executor, err := agentproductcanary.NewRootExecutor(plan, factory, resolved.pollInterval)
	if err != nil {
		return errors.New("Product canary executor invalid")
	}
	service, err := agentproductcanary.NewService(agentproductcanary.Config{
		ActivationID: resolved.activationID, ClaimOwner: resolved.claimOwner,
		PollInterval: resolved.pollInterval, ClaimTTL: resolved.claimTTL,
		BatchSize: resolved.batchSize,
	}, plan, gate, agentproductcanary.NewPostgresRepository(db.SQL()), executor)
	if err != nil {
		return errors.New("Product canary service invalid")
	}
	if args[0] == "healthcheck" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return service.Health(ctx)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("agent_product_canary_started", slog.String("stage", agentactivation.StageProductCanary))
	if err := service.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	logger.Info("agent_product_canary_stopped")
	return nil
}

func loadWorkerConfig(lookup func(string) (string, bool)) (workerConfig, error) {
	var result workerConfig
	enabled, err := boolSetting(lookup, envEnabled)
	if err != nil || !enabled {
		return result, fmt.Errorf("%s must be true", envEnabled)
	}
	for _, name := range prerequisiteFlags {
		value, parseErr := boolSetting(lookup, name)
		if parseErr != nil || !value {
			return result, fmt.Errorf("%s must be true", name)
		}
	}
	for _, name := range []string{envRuntimeEnabled, envSchedulerEnabled,
		envLearningEnabled, envSkillInstallEnabled} {
		value, parseErr := boolSetting(lookup, name)
		if parseErr != nil || value {
			return result, fmt.Errorf("%s must be false", name)
		}
	}
	for name, target := range map[string]*string{
		envDatabaseURL: &result.databaseURL, envActivationID: &result.activationID,
		envRunnerURL: &result.runnerURL, envRunnerID: &result.runnerID,
		envServerName: &result.serverName, envCallerIdentity: &result.callerIdentity,
		envClientCertificate: &result.clientCertificate, envClientKey: &result.clientKey,
		envServerCA: &result.serverCA, envReleaseManifest: &result.releaseManifest,
		envProductionPolicy: &result.productionPolicy, envActivationRecord: &result.activationRecord,
		envCanaryPlan: &result.canaryPlan, envAuthorityPrivateKey: &result.authorityPrivateKey,
		envAuthorityPublicKey: &result.authorityPublicKey, envReleaseCommit: &result.releaseCommit,
		envClaimOwner: &result.claimOwner,
	} {
		*target = env(lookup, name, "")
		if *target == "" {
			return result, fmt.Errorf("%s is required", name)
		}
	}
	if result.callerIdentity != agentrootcanary.ProductCallerIdentity {
		return result, fmt.Errorf("%s must be the dedicated product canary identity", envCallerIdentity)
	}
	for name, value := range map[string]string{
		envClientCertificate: result.clientCertificate, envClientKey: result.clientKey,
		envServerCA: result.serverCA, envReleaseManifest: result.releaseManifest,
		envProductionPolicy: result.productionPolicy, envActivationRecord: result.activationRecord,
		envCanaryPlan: result.canaryPlan, envAuthorityPrivateKey: result.authorityPrivateKey,
		envAuthorityPublicKey: result.authorityPublicKey,
	} {
		if !strings.HasPrefix(value, "/") {
			return result, fmt.Errorf("%s must be absolute", name)
		}
	}
	parsed, parseErr := url.Parse(result.databaseURL)
	password, hasPassword := "", false
	if parsed != nil && parsed.User != nil {
		password, hasPassword = parsed.User.Password()
	}
	if parseErr != nil || parsed == nil || !member(parsed.Scheme, "postgres", "postgresql") ||
		parsed.Hostname() == "" || parsed.User == nil || !hasPassword || password == "" {
		return result, fmt.Errorf("%s must be a PostgreSQL URL", envDatabaseURL)
	}
	if len(result.releaseCommit) != 40 || strings.Trim(result.releaseCommit, "0123456789abcdef") != "" ||
		!activationIDPattern.MatchString(result.activationID) || result.claimOwner == "" || len(result.claimOwner) > 128 {
		return result, errors.New("Product canary release or identity binding invalid")
	}
	result.pollInterval, err = durationSetting(lookup, envPollInterval, 5*time.Second, time.Second, time.Minute)
	if err != nil {
		return result, err
	}
	result.claimTTL, err = durationSetting(lookup, envClaimTTL, 5*time.Minute, 31*time.Second, 5*time.Minute)
	if err != nil {
		return result, err
	}
	result.rpcTimeout, err = durationSetting(lookup, envRPCTimeout, 10*time.Second, time.Second, time.Minute)
	if err != nil {
		return result, err
	}
	result.authorityTTL, err = durationSetting(lookup, envAuthorityTTL, 10*time.Second, time.Second, 15*time.Second)
	if err != nil {
		return result, err
	}
	result.batchSize, err = intSetting(lookup, envBatchSize, 20, 1, 20)
	return result, err
}

func verifyDatabaseRole(ctx context.Context, db *sql.DB) error {
	var accepted bool
	err := db.QueryRowContext(ctx, `
WITH RECURSIVE current_login AS (
  SELECT oid,rolcanlogin,rolinherit,rolsuper,rolcreatedb,rolcreaterole,rolreplication,rolbypassrls
  FROM pg_roles WHERE rolname=current_user
), inherited(role_oid) AS (
  SELECT membership.roleid FROM pg_auth_members membership JOIN current_login ON current_login.oid=membership.member
  UNION
  SELECT membership.roleid FROM pg_auth_members membership JOIN inherited ON inherited.role_oid=membership.member
), inherited_names AS (
  SELECT role.rolname FROM inherited JOIN pg_roles role ON role.oid=inherited.role_oid
)
SELECT current_login.rolcanlogin AND current_login.rolinherit
  AND NOT current_login.rolsuper AND NOT current_login.rolcreatedb AND NOT current_login.rolcreaterole
  AND NOT current_login.rolreplication AND NOT current_login.rolbypassrls
  AND (SELECT array_agg(rolname ORDER BY rolname) FROM inherited_names)
      = ARRAY['agent_orchestrator_runtime','agent_product_canary_worker','agent_runner_control']::name[]
FROM current_login`).Scan(&accepted)
	if err != nil || !accepted {
		return errors.New("Product canary database role rejected")
	}
	return nil
}

func env(lookup func(string) (string, bool), name, fallback string) string {
	value, ok := lookup(name)
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return fallback
	}
	return value
}

func boolSetting(lookup func(string) (string, bool), name string) (bool, error) {
	value := env(lookup, name, "false")
	if value == "true" {
		return true, nil
	}
	if value == "false" {
		return false, nil
	}
	return false, fmt.Errorf("%s must be true or false", name)
}

func durationSetting(lookup func(string) (string, bool), name string,
	fallback, minimum, maximum time.Duration,
) (time.Duration, error) {
	parsed, err := time.ParseDuration(env(lookup, name, fallback.String()))
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s is out of range", name)
	}
	return parsed, nil
}

func intSetting(lookup func(string) (string, bool), name string, fallback, minimum, maximum int) (int, error) {
	parsed, err := strconv.Atoi(env(lookup, name, strconv.Itoa(fallback)))
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s is out of range", name)
	}
	return parsed, nil
}

func member(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func sanitizedError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.TrimSpace(err.Error())
	if len(value) > 256 {
		value = value[:256]
	}
	return value
}

var _ agentproductcanary.RootServiceFactory = rootFactory{}
