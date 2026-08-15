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
	"strconv"
	"strings"
	"syscall"
	"time"

	"neo-chat/mm-chat/backend/internal/agentactivation"
	"neo-chat/mm-chat/backend/internal/agentchildcanary"
	"neo-chat/mm-chat/backend/internal/agentdelegation"
	"neo-chat/mm-chat/backend/internal/agentorchestrator"
	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/database"
)

const (
	envCanaryEnabled        = "AGENT_CHILD_CANARY_ENABLED"
	envControlEnabled       = "AGENT_RUNNER_CONTROL_ENABLED"
	envRootCanaryEnabled    = "AGENT_ROOT_RUN_CANARY_ENABLED"
	envBrokerCanaryEnabled  = "AGENT_BROKER_ARTIFACT_CANARY_ENABLED"
	envProjectCanaryEnabled = "AGENT_PROJECT_MUTATION_CANARY_ENABLED"
	envRuntimeEnabled       = "AGENT_RUNTIME_ENABLED"
	envSchedulerEnabled     = "AGENT_SCHEDULER_ENABLED"
	envSkillInstallEnabled  = "AGENT_SKILL_INSTALL_ENABLED"
	envLearningEnabled      = "AGENT_LEARNING_ENABLED"
	envDelegationEnabled    = "AGENT_DELEGATION_ENABLED"
	envBrokerReadEnabled    = "AGENT_BROKER_READ_ONLY_ENABLED"
	envBrokerWriteEnabled   = "AGENT_BROKER_MUTATION_ENABLED"
	envDatabaseURL          = "AGENT_CHILD_CANARY_DATABASE_URL"
	envRunnerURL            = "AGENT_CHILD_CANARY_RUNNER_URL"
	envRunnerID             = "AGENT_CHILD_CANARY_RUNNER_ID"
	envServerName           = "AGENT_CHILD_CANARY_SERVER_NAME"
	envCallerIdentity       = "AGENT_CHILD_CANARY_CLIENT_IDENTITY"
	envClientCertificate    = "AGENT_CHILD_CANARY_CLIENT_CERT_FILE"
	envClientKey            = "AGENT_CHILD_CANARY_CLIENT_KEY_FILE"
	envServerCA             = "AGENT_CHILD_CANARY_SERVER_CA_FILE"
	envReleaseManifest      = "AGENT_CHILD_CANARY_RELEASE_MANIFEST_FILE"
	envProductionPolicy     = "AGENT_CHILD_CANARY_PRODUCTION_POLICY_FILE"
	envActivationRecord     = "AGENT_CHILD_CANARY_ACTIVATION_FILE"
	envCanaryPlan           = "AGENT_CHILD_CANARY_PLAN_FILE"
	envAuthorityPrivateKey  = "AGENT_CHILD_CANARY_AUTHORITY_PRIVATE_KEY_FILE"
	envAuthorityPublicKey   = "AGENT_CHILD_CANARY_AUTHORITY_PUBLIC_KEY_FILE"
	envReleaseCommit        = "AGENT_CHILD_CANARY_RELEASE_GIT_COMMIT"
	envPollInterval         = "AGENT_CHILD_CANARY_POLL_INTERVAL"
	envRPCTimeout           = "AGENT_CHILD_CANARY_RPC_TIMEOUT"
	envAuthorityTTL         = "AGENT_CHILD_CANARY_AUTHORITY_TTL"
	envBatchSize            = "AGENT_CHILD_CANARY_RECONCILE_BATCH_SIZE"
)

type workerConfig struct {
	canaryEnabled, controlEnabled, rootCanaryEnabled, brokerCanaryEnabled       bool
	projectCanaryEnabled, runtimeEnabled, schedulerEnabled, skillInstallEnabled bool
	learningEnabled, delegationEnabled, brokerReadEnabled, brokerWriteEnabled   bool
	databaseURL, runnerURL, runnerID, serverName, callerIdentity                string
	clientCertificate, clientKey, serverCA, releaseManifest                     string
	productionPolicy, activationRecord, canaryPlan                              string
	authorityPrivateKey, authorityPublicKey, releaseCommit                      string
	pollInterval, rpcTimeout, authorityTTL                                      time.Duration
	batchSize                                                                   int
}

type evidenceGate struct {
	config agentactivation.ChildCanaryConfig
}

func (gate evidenceGate) Verify(now time.Time) error {
	decision, err := agentactivation.VerifyChildCanary(gate.config, now)
	if err != nil || !decision.Ready {
		return agentchildcanary.ErrUnavailable
	}
	return nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(os.Args[1:], os.LookupEnv, logger); err != nil {
		logger.Error("agent_child_canary_exit", slog.String("error", sanitizedError(err)))
		os.Exit(1)
	}
}

func run(args []string, lookup func(string) (string, bool), logger *slog.Logger) error {
	if len(args) != 1 || (args[0] != "run" && args[0] != "healthcheck") {
		return errors.New("usage: agent-runtime-child-canary run | agent-runtime-child-canary healthcheck")
	}
	resolved, err := loadWorkerConfig(lookup)
	if err != nil {
		return err
	}
	plan, err := agentchildcanary.LoadPlan(resolved.canaryPlan)
	if err != nil {
		return errors.New("Child canary plan invalid")
	}
	privateKey, err := agentrunner.LoadEd25519PrivateKey(resolved.authorityPrivateKey)
	if err != nil {
		return errors.New("Child canary authority key invalid")
	}
	publicKey, err := agentrunner.LoadEd25519PublicKey(resolved.authorityPublicKey)
	if err != nil || !privateKey.Public().(ed25519.PublicKey).Equal(publicKey) {
		return errors.New("Child canary authority key mismatch")
	}
	gate := evidenceGate{config: agentactivation.ChildCanaryConfig{Config: agentactivation.Config{
		PolicyFile: resolved.productionPolicy, RecordFile: resolved.activationRecord,
		ReleaseManifestFile: resolved.releaseManifest, ClientCertificateFile: resolved.clientCertificate,
		ServerCAFile: resolved.serverCA, Endpoint: resolved.runnerURL, RunnerID: resolved.runnerID,
		ServerName: resolved.serverName, CallerIdentity: resolved.callerIdentity, ReleaseCommit: resolved.releaseCommit,
	}, CanaryPlanFile: resolved.canaryPlan, AuthorityPublicKeyFile: resolved.authorityPublicKey}}
	if err := gate.Verify(time.Now().UTC()); err != nil {
		return errors.New("Child canary activation unavailable")
	}
	openCtx, cancelOpen := context.WithTimeout(context.Background(), 10*time.Second)
	db, err := database.Open(openCtx, config.Config{DatabaseURL: resolved.databaseURL,
		DBMaxOpenConns: 4, DBMaxIdleConns: 2, DBConnMaxLifetime: 30 * time.Minute})
	cancelOpen()
	if err != nil || db == nil || db.SQL() == nil {
		return errors.New("Child canary database unavailable")
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
		return errors.New("Child canary Runner mTLS configuration invalid")
	}
	runnerRepository := agentrunner.NewPostgresControlRepository(db.SQL())
	authority, err := agentrunner.NewControlService(runnerRepository, privateKey)
	if err != nil {
		return errors.New("Child canary authority service invalid")
	}
	reaper, err := agentchildcanary.NewRunnerReaper(resolved.runnerID, client)
	if err != nil {
		return errors.New("Child canary Reaper configuration invalid")
	}
	delegationService := agentdelegation.NewService(
		agentdelegation.NewPostgresRepository(db.SQL()), reaper)
	service, err := agentchildcanary.NewService(agentchildcanary.Config{RunnerID: resolved.runnerID,
		CallerIdentity: resolved.callerIdentity, PollInterval: resolved.pollInterval,
		AuthorityTTL: resolved.authorityTTL, BatchSize: resolved.batchSize}, plan, gate,
		agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(db.SQL())), delegationService,
		authority, runnerRepository, client, agentchildcanary.NewPostgresParentTerminalRepository(db.SQL()),
		agentchildcanary.NewPostgresStateRepository(db.SQL()))
	if err != nil {
		return errors.New("Child canary service configuration invalid")
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
	logger.Info("agent_child_canary_started", slog.String("stage", agentactivation.StageChildCanary))
	if err := service.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	logger.Info("agent_child_canary_stopped")
	return nil
}

func loadWorkerConfig(lookup func(string) (string, bool)) (workerConfig, error) {
	var result workerConfig
	var err error
	for _, setting := range []struct {
		name   string
		target *bool
	}{
		{envCanaryEnabled, &result.canaryEnabled}, {envRuntimeEnabled, &result.runtimeEnabled},
		{envControlEnabled, &result.controlEnabled}, {envRootCanaryEnabled, &result.rootCanaryEnabled},
		{envBrokerCanaryEnabled, &result.brokerCanaryEnabled}, {envProjectCanaryEnabled, &result.projectCanaryEnabled},
		{envSchedulerEnabled, &result.schedulerEnabled}, {envSkillInstallEnabled, &result.skillInstallEnabled},
		{envLearningEnabled, &result.learningEnabled}, {envDelegationEnabled, &result.delegationEnabled},
		{envBrokerReadEnabled, &result.brokerReadEnabled}, {envBrokerWriteEnabled, &result.brokerWriteEnabled},
	} {
		*setting.target, err = boolSetting(lookup, setting.name)
		if err != nil {
			return workerConfig{}, err
		}
	}
	if !result.canaryEnabled || !result.controlEnabled || !result.rootCanaryEnabled ||
		!result.brokerCanaryEnabled || !result.projectCanaryEnabled || !result.delegationEnabled {
		return workerConfig{}, errors.New("G21.4 requires G21.0-G21.3 and delegation readiness")
	}
	if result.runtimeEnabled || result.schedulerEnabled || result.skillInstallEnabled || result.learningEnabled ||
		result.brokerReadEnabled || result.brokerWriteEnabled {
		return workerConfig{}, errors.New("G21.4 permits the synthetic Child canary only")
	}
	for name, target := range map[string]*string{
		envDatabaseURL: &result.databaseURL, envRunnerURL: &result.runnerURL, envRunnerID: &result.runnerID,
		envServerName: &result.serverName, envCallerIdentity: &result.callerIdentity,
		envClientCertificate: &result.clientCertificate, envClientKey: &result.clientKey, envServerCA: &result.serverCA,
		envReleaseManifest: &result.releaseManifest, envProductionPolicy: &result.productionPolicy,
		envActivationRecord: &result.activationRecord, envCanaryPlan: &result.canaryPlan,
		envAuthorityPrivateKey: &result.authorityPrivateKey, envAuthorityPublicKey: &result.authorityPublicKey,
		envReleaseCommit: &result.releaseCommit,
	} {
		*target = env(lookup, name, "")
		if *target == "" {
			return workerConfig{}, fmt.Errorf("%s is required", name)
		}
	}
	if result.callerIdentity != agentactivation.ChildCanaryCallerIdentity {
		return workerConfig{}, fmt.Errorf("%s must be the dedicated Child canary identity", envCallerIdentity)
	}
	for name, value := range map[string]string{
		envClientCertificate: result.clientCertificate, envClientKey: result.clientKey, envServerCA: result.serverCA,
		envReleaseManifest: result.releaseManifest, envProductionPolicy: result.productionPolicy,
		envActivationRecord: result.activationRecord, envCanaryPlan: result.canaryPlan,
		envAuthorityPrivateKey: result.authorityPrivateKey, envAuthorityPublicKey: result.authorityPublicKey,
	} {
		if !strings.HasPrefix(value, "/") {
			return workerConfig{}, fmt.Errorf("%s must be absolute", name)
		}
	}
	parsed, parseErr := url.Parse(result.databaseURL)
	password, hasPassword := "", false
	if parsed != nil && parsed.User != nil {
		password, hasPassword = parsed.User.Password()
	}
	if parseErr != nil || parsed == nil || !member(parsed.Scheme, "postgres", "postgresql") ||
		parsed.Hostname() == "" || parsed.User == nil || !hasPassword || password == "" {
		return workerConfig{}, fmt.Errorf("%s must be a PostgreSQL URL", envDatabaseURL)
	}
	if len(result.releaseCommit) != 40 || strings.Trim(result.releaseCommit, "0123456789abcdef") != "" {
		return workerConfig{}, fmt.Errorf("%s must be a lowercase Git commit", envReleaseCommit)
	}
	result.pollInterval, err = durationSetting(lookup, envPollInterval, 10*time.Second, time.Second, time.Minute)
	if err != nil {
		return workerConfig{}, err
	}
	result.rpcTimeout, err = durationSetting(lookup, envRPCTimeout, 10*time.Second, time.Second, time.Minute)
	if err != nil {
		return workerConfig{}, err
	}
	result.authorityTTL, err = durationSetting(lookup, envAuthorityTTL, 10*time.Second, time.Second, 15*time.Second)
	if err != nil {
		return workerConfig{}, err
	}
	result.batchSize, err = intSetting(lookup, envBatchSize, 100, 1, 1000)
	if err != nil {
		return workerConfig{}, err
	}
	return result, nil
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
      = ARRAY['agent_delegation_control','agent_orchestrator_runtime','agent_runner_control']::name[]
FROM current_login`).Scan(&accepted)
	if err != nil || !accepted {
		return errors.New("Child canary database role rejected")
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
func durationSetting(lookup func(string) (string, bool), name string, fallback, minimum, maximum time.Duration) (time.Duration, error) {
	value := env(lookup, name, fallback.String())
	parsed, err := time.ParseDuration(value)
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
	for _, allowed := range []string{"usage:", "must be", "is required", "out of range", "G21.4", "canary", "database", "mTLS", "configuration", "CHILD_CANARY"} {
		if strings.Contains(err.Error(), allowed) {
			return err.Error()
		}
	}
	return "agent Child canary failed"
}
