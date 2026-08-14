package main

import (
	"context"
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
	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/agentruntimecontrol"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/database"
)

const (
	envControlEnabled       = "AGENT_RUNNER_CONTROL_ENABLED"
	envRuntimeEnabled       = "AGENT_RUNTIME_ENABLED"
	envSchedulerEnabled     = "AGENT_SCHEDULER_ENABLED"
	envSkillInstallEnabled  = "AGENT_SKILL_INSTALL_ENABLED"
	envLearningEnabled      = "AGENT_LEARNING_ENABLED"
	envDelegationEnabled    = "AGENT_DELEGATION_ENABLED"
	envBrokerReadEnabled    = "AGENT_BROKER_READ_ONLY_ENABLED"
	envBrokerMutableEnabled = "AGENT_BROKER_MUTATION_ENABLED"
	envDatabaseURL          = "AGENT_RUNNER_DATABASE_URL"
	envRunnerURL            = "AGENT_RUNNER_URL"
	envRunnerID             = "AGENT_RUNNER_ID"
	envServerName           = "AGENT_RUNNER_SERVER_NAME"
	envCallerIdentity       = "AGENT_RUNNER_CLIENT_IDENTITY"
	envClientCertificate    = "AGENT_RUNNER_CLIENT_CERT_FILE"
	envClientKey            = "AGENT_RUNNER_CLIENT_KEY_FILE"
	envServerCA             = "AGENT_RUNNER_SERVER_CA_FILE"
	envReleaseManifest      = "AGENT_RUNNER_RELEASE_MANIFEST_FILE"
	envProductionPolicy     = "AGENT_PRODUCTION_POLICY_FILE"
	envActivationRecord     = "AGENT_PRODUCTION_ACTIVATION_FILE"
	envReleaseCommit        = "AGENT_RELEASE_GIT_COMMIT"
	envPollInterval         = "AGENT_RUNNER_POLL_INTERVAL"
	envRPCTimeout           = "AGENT_RUNNER_RPC_TIMEOUT"
	envBatchSize            = "AGENT_RUNNER_RECONCILE_BATCH_SIZE"

	databaseOpenTimeout = 10 * time.Second
	healthcheckTimeout  = 20 * time.Second
)

type workerConfig struct {
	controlEnabled       bool
	runtimeEnabled       bool
	schedulerEnabled     bool
	skillInstallEnabled  bool
	learningEnabled      bool
	delegationEnabled    bool
	brokerReadEnabled    bool
	brokerMutableEnabled bool
	databaseURL          string
	runnerURL            string
	runnerID             string
	serverName           string
	callerIdentity       string
	clientCertificate    string
	clientKey            string
	serverCA             string
	releaseManifest      string
	productionPolicy     string
	activationRecord     string
	releaseCommit        string
	pollInterval         time.Duration
	rpcTimeout           time.Duration
	batchSize            int
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(os.Args[1:], os.LookupEnv, logger); err != nil {
		logger.Error("agent_runtime_control_exit", slog.String("error", sanitizedError(err)))
		os.Exit(1)
	}
}

func run(args []string, lookup func(string) (string, bool), logger *slog.Logger) error {
	if len(args) != 1 || (args[0] != "run" && args[0] != "healthcheck") {
		return errors.New("usage: agent-runtime-control run | agent-runtime-control healthcheck")
	}
	resolved, err := loadWorkerConfig(lookup)
	if err != nil {
		return err
	}
	if logger == nil {
		logger = slog.Default()
	}
	gate := agentruntimecontrol.EvidenceGate{Config: agentactivation.Config{
		PolicyFile: resolved.productionPolicy, RecordFile: resolved.activationRecord,
		ReleaseManifestFile: resolved.releaseManifest, ClientCertificateFile: resolved.clientCertificate,
		ServerCAFile: resolved.serverCA, Endpoint: resolved.runnerURL, RunnerID: resolved.runnerID,
		ServerName: resolved.serverName, CallerIdentity: resolved.callerIdentity,
		ReleaseCommit: resolved.releaseCommit,
	}}
	if err := gate.Verify(time.Now().UTC()); err != nil {
		return errors.New("agent control activation unavailable")
	}
	openCtx, cancelOpen := context.WithTimeout(context.Background(), databaseOpenTimeout)
	db, err := database.Open(openCtx, config.Config{DatabaseURL: resolved.databaseURL,
		DBMaxOpenConns: 4, DBMaxIdleConns: 2, DBConnMaxLifetime: 30 * time.Minute})
	cancelOpen()
	if err != nil || db == nil || db.SQL() == nil {
		return errors.New("agent control database unavailable")
	}
	defer db.Close()
	roleCtx, cancelRole := context.WithTimeout(context.Background(), databaseOpenTimeout)
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
		return errors.New("agent Runner mTLS configuration invalid")
	}
	service, err := agentruntimecontrol.NewService(agentruntimecontrol.Config{
		RunnerID: resolved.runnerID, PollInterval: resolved.pollInterval, BatchSize: resolved.batchSize,
	}, gate, agentrunner.NewPostgresControlRepository(db.SQL()), client)
	if err != nil {
		return errors.New("agent control service configuration invalid")
	}
	if args[0] == "healthcheck" {
		ctx, cancel := context.WithTimeout(context.Background(), healthcheckTimeout)
		defer cancel()
		return service.Health(ctx)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger.Info("agent_runtime_control_started", slog.String("stage", agentactivation.StageControl))
	if err := service.Run(ctx); err != nil {
		return err
	}
	logger.Info("agent_runtime_control_stopped")
	return nil
}

func loadWorkerConfig(lookup func(string) (string, bool)) (workerConfig, error) {
	var result workerConfig
	var err error
	for _, setting := range []struct {
		name   string
		target *bool
	}{
		{envControlEnabled, &result.controlEnabled}, {envRuntimeEnabled, &result.runtimeEnabled},
		{envSchedulerEnabled, &result.schedulerEnabled}, {envSkillInstallEnabled, &result.skillInstallEnabled},
		{envLearningEnabled, &result.learningEnabled}, {envDelegationEnabled, &result.delegationEnabled},
		{envBrokerReadEnabled, &result.brokerReadEnabled}, {envBrokerMutableEnabled, &result.brokerMutableEnabled},
	} {
		*setting.target, err = boolSetting(lookup, setting.name, false)
		if err != nil {
			return workerConfig{}, err
		}
	}
	if !result.controlEnabled {
		return workerConfig{}, fmt.Errorf("%s must be true", envControlEnabled)
	}
	if result.runtimeEnabled || result.schedulerEnabled || result.skillInstallEnabled || result.learningEnabled ||
		result.delegationEnabled || result.brokerReadEnabled || result.brokerMutableEnabled {
		return workerConfig{}, errors.New("G21.0 permits control-plane maintenance only")
	}
	result.databaseURL = env(lookup, envDatabaseURL, "")
	result.runnerURL = env(lookup, envRunnerURL, "")
	result.runnerID = env(lookup, envRunnerID, "")
	result.serverName = env(lookup, envServerName, "")
	result.callerIdentity = env(lookup, envCallerIdentity, "")
	result.clientCertificate = env(lookup, envClientCertificate, "")
	result.clientKey = env(lookup, envClientKey, "")
	result.serverCA = env(lookup, envServerCA, "")
	result.releaseManifest = env(lookup, envReleaseManifest, "")
	result.productionPolicy = env(lookup, envProductionPolicy, "")
	result.activationRecord = env(lookup, envActivationRecord, "")
	result.releaseCommit = env(lookup, envReleaseCommit, "")
	for name, value := range map[string]string{
		envDatabaseURL: result.databaseURL, envRunnerURL: result.runnerURL, envRunnerID: result.runnerID,
		envServerName: result.serverName, envCallerIdentity: result.callerIdentity,
		envClientCertificate: result.clientCertificate, envClientKey: result.clientKey,
		envServerCA: result.serverCA, envReleaseManifest: result.releaseManifest,
		envProductionPolicy: result.productionPolicy, envActivationRecord: result.activationRecord,
		envReleaseCommit: result.releaseCommit,
	} {
		if value == "" {
			return workerConfig{}, fmt.Errorf("%s is required", name)
		}
	}
	if result.callerIdentity != agentactivation.ControlCallerIdentity {
		return workerConfig{}, fmt.Errorf("%s must be the dedicated control identity", envCallerIdentity)
	}
	for name, value := range map[string]string{
		envClientCertificate: result.clientCertificate, envClientKey: result.clientKey,
		envServerCA: result.serverCA, envReleaseManifest: result.releaseManifest,
		envProductionPolicy: result.productionPolicy, envActivationRecord: result.activationRecord,
	} {
		if !strings.HasPrefix(value, "/") {
			return workerConfig{}, fmt.Errorf("%s must be absolute", name)
		}
	}
	if len(result.releaseCommit) != 40 {
		return workerConfig{}, fmt.Errorf("%s must be a lowercase Git commit", envReleaseCommit)
	}
	for _, character := range result.releaseCommit {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return workerConfig{}, fmt.Errorf("%s must be a lowercase Git commit", envReleaseCommit)
		}
	}
	parsedDatabase, parseErr := url.Parse(result.databaseURL)
	password, hasPassword := "", false
	if parsedDatabase != nil && parsedDatabase.User != nil {
		password, hasPassword = parsedDatabase.User.Password()
	}
	if parseErr != nil || parsedDatabase == nil || !member(parsedDatabase.Scheme, "postgres", "postgresql") ||
		parsedDatabase.Hostname() == "" || parsedDatabase.User == nil || !hasPassword || password == "" {
		return workerConfig{}, fmt.Errorf("%s must be a PostgreSQL URL", envDatabaseURL)
	}
	result.pollInterval, err = durationSetting(lookup, envPollInterval, 30*time.Second, 5*time.Second, 5*time.Minute)
	if err != nil {
		return workerConfig{}, err
	}
	result.rpcTimeout, err = durationSetting(lookup, envRPCTimeout, 10*time.Second, time.Second, time.Minute)
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
  SELECT oid,rolcanlogin,rolinherit,rolsuper,rolcreatedb,rolcreaterole,
    rolreplication,rolbypassrls
  FROM pg_roles WHERE rolname=current_user
), inherited(role_oid) AS (
  SELECT membership.roleid FROM pg_auth_members membership
  JOIN current_login ON current_login.oid=membership.member
  UNION
  SELECT membership.roleid FROM pg_auth_members membership
  JOIN inherited ON inherited.role_oid=membership.member
)
SELECT current_login.rolcanlogin AND current_login.rolinherit
  AND NOT current_login.rolsuper AND NOT current_login.rolcreatedb
  AND NOT current_login.rolcreaterole AND NOT current_login.rolreplication
  AND NOT current_login.rolbypassrls
  AND EXISTS (
    SELECT 1 FROM inherited
    JOIN pg_roles inherited_role ON inherited_role.oid=inherited.role_oid
    WHERE inherited_role.rolname='agent_runner_control'
  )
  AND NOT EXISTS (
    SELECT 1 FROM inherited
    JOIN pg_roles inherited_role ON inherited_role.oid=inherited.role_oid
    WHERE inherited_role.rolname<>'agent_runner_control'
  )
FROM current_login`).Scan(&accepted)
	if err != nil || !accepted {
		return errors.New("agent control database role rejected")
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
func boolSetting(lookup func(string) (string, bool), name string, fallback bool) (bool, error) {
	value, ok := lookup(name)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	value = strings.TrimSpace(value)
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
	value := env(lookup, name, strconv.Itoa(fallback))
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s is out of range", name)
	}
	return parsed, nil
}
func member(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
func sanitizedError(err error) string {
	if err == nil {
		return ""
	}
	for _, candidate := range []string{
		"usage:", "must be", "is required", "out of range", "G21.0", "database", "mTLS", "configuration",
		"AGENT_CONTROL_UNAVAILABLE", "AGENT_CONTROL_INVALID",
	} {
		if strings.Contains(err.Error(), candidate) {
			return err.Error()
		}
	}
	return "agent runtime control failed"
}
