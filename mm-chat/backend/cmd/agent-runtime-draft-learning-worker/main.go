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
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"neo-chat/mm-chat/backend/internal/agentactivation"
	"neo-chat/mm-chat/backend/internal/agentlearning"
	"neo-chat/mm-chat/backend/internal/agentlearningworker"
	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/database"
	"neo-chat/mm-chat/backend/internal/storage"
)

const (
	envEnabled              = "AGENT_DRAFT_LEARNING_WORKER_ENABLED"
	envRuntimeEnabled       = "AGENT_RUNTIME_ENABLED"
	envSchedulerEnabled     = "AGENT_SCHEDULER_ENABLED"
	envLearningEnabled      = "AGENT_LEARNING_ENABLED"
	envSkillInstallEnabled  = "AGENT_SKILL_INSTALL_ENABLED"
	envBrokerReadEnabled    = "AGENT_BROKER_READ_ONLY_ENABLED"
	envBrokerWriteEnabled   = "AGENT_BROKER_MUTATION_ENABLED"
	envDelegationEnabled    = "AGENT_DELEGATION_ENABLED"
	envDatabaseURL          = "AGENT_DRAFT_LEARNING_WORKER_DATABASE_URL"
	envPlanFile             = "AGENT_DRAFT_LEARNING_WORKER_PLAN_FILE"
	envRunnerURL            = "AGENT_DRAFT_LEARNING_WORKER_RUNNER_URL"
	envRunnerID             = "AGENT_DRAFT_LEARNING_WORKER_RUNNER_ID"
	envServerName           = "AGENT_DRAFT_LEARNING_WORKER_SERVER_NAME"
	envCallerIdentity       = "AGENT_DRAFT_LEARNING_WORKER_CLIENT_IDENTITY"
	envClientCertificate    = "AGENT_DRAFT_LEARNING_WORKER_CLIENT_CERT_FILE"
	envClientKey            = "AGENT_DRAFT_LEARNING_WORKER_CLIENT_KEY_FILE"
	envServerCA             = "AGENT_DRAFT_LEARNING_WORKER_SERVER_CA_FILE"
	envAuthorityPrivateKey  = "AGENT_DRAFT_LEARNING_WORKER_AUTHORITY_PRIVATE_KEY_FILE"
	envAuthorityPublicKey   = "AGENT_DRAFT_LEARNING_WORKER_AUTHORITY_PUBLIC_KEY_FILE"
	envOwner                = "AGENT_DRAFT_LEARNING_WORKER_OWNER"
	envPollInterval         = "AGENT_DRAFT_LEARNING_WORKER_POLL_INTERVAL"
	envRPCTimeout           = "AGENT_DRAFT_LEARNING_WORKER_RPC_TIMEOUT"
	envLeaseDuration        = "AGENT_DRAFT_LEARNING_WORKER_LEASE_DURATION"
	envBatchSize            = "AGENT_DRAFT_LEARNING_WORKER_BATCH_SIZE"
	envRetention            = "AGENT_DRAFT_LEARNING_WORKER_RETENTION"
	envMaintenanceEvery     = "AGENT_DRAFT_LEARNING_WORKER_MAINTENANCE_EVERY"
	envReleaseManifest      = "AGENT_DRAFT_LEARNING_WORKER_RELEASE_MANIFEST_FILE"
	envProductionPolicy     = "AGENT_DRAFT_LEARNING_WORKER_PRODUCTION_POLICY_FILE"
	envActivationRecord     = "AGENT_DRAFT_LEARNING_WORKER_ACTIVATION_FILE"
	envReleaseCommit        = "AGENT_DRAFT_LEARNING_WORKER_RELEASE_GIT_COMMIT"
	envObjectCredentialMeta = "AGENT_DRAFT_LEARNING_WORKER_OBJECT_CREDENTIAL_META_FILE"
)

type workerConfig struct {
	enabled, runtimeEnabled, schedulerEnabled, learningEnabled bool
	skillInstallEnabled, brokerReadEnabled, brokerWriteEnabled bool
	delegationEnabled                                          bool
	databaseURL, planFile, runnerURL, runnerID, serverName     string
	callerIdentity, clientCertificate, clientKey, serverCA     string
	authorityPrivateKey, authorityPublicKey, owner             string
	releaseManifest, productionPolicy, activationRecord        string
	releaseCommit, objectCredentialMeta                        string
	prerequisites                                              map[string]string
	storageBackend, s3Endpoint, s3Bucket, s3Region             string
	s3AccessKey, s3SecretKey                                   string
	s3UseSSL, s3ForcePathStyle                                 bool
	pollInterval, rpcTimeout, leaseDuration, retention         time.Duration
	batchSize, maintenanceEvery                                int
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(os.Args[1:], os.LookupEnv, logger); err != nil {
		logger.Error("agent_draft_learning_worker_exit", slog.String("error", sanitize(err)))
		os.Exit(1)
	}
}

func run(args []string, lookup func(string) (string, bool), logger *slog.Logger) error {
	if len(args) != 1 || (args[0] != "run" && args[0] != "healthcheck") {
		return errors.New("usage: agent-runtime-draft-learning-worker run | agent-runtime-draft-learning-worker healthcheck")
	}
	resolved, err := loadWorkerConfig(lookup)
	if err != nil {
		return err
	}
	plan, err := agentlearningworker.LoadPlan(resolved.planFile)
	if err != nil || plan.RunnerID != resolved.runnerID || plan.CallerIdentity != resolved.callerIdentity {
		return errors.New("Draft learning worker plan invalid")
	}
	privateKey, err := agentrunner.LoadEd25519PrivateKey(resolved.authorityPrivateKey)
	if err != nil {
		return errors.New("Draft learning worker authority key invalid")
	}
	publicKey, err := agentrunner.LoadEd25519PublicKey(resolved.authorityPublicKey)
	if err != nil || !privateKey.Public().(ed25519.PublicKey).Equal(publicKey) {
		return errors.New("Draft learning worker authority key mismatch")
	}
	decision, err := agentactivation.VerifyDraftLearningWorker(agentactivation.DraftLearningWorkerConfig{
		WorkerConfig: agentactivation.WorkerConfig{PolicyFile: resolved.productionPolicy,
			RecordFile: resolved.activationRecord, PlanFile: resolved.planFile,
			ReleaseCommit: resolved.releaseCommit, PrerequisiteFiles: resolved.prerequisites},
		ReleaseManifestFile:   resolved.releaseManifest,
		ClientCertificateFile: resolved.clientCertificate, ServerCAFile: resolved.serverCA,
		AuthorityPublicKeyFile:   resolved.authorityPublicKey,
		ObjectCredentialMetaFile: resolved.objectCredentialMeta,
		Endpoint:                 resolved.runnerURL, RunnerID: resolved.runnerID, ServerName: resolved.serverName,
		CallerIdentity: resolved.callerIdentity,
	}, time.Now().UTC())
	if err != nil || !decision.Ready {
		return errors.New("Draft learning worker activation unavailable")
	}
	openCtx, cancelOpen := context.WithTimeout(context.Background(), 10*time.Second)
	db, err := database.Open(openCtx, config.Config{DatabaseURL: resolved.databaseURL,
		DBMaxOpenConns: 4, DBMaxIdleConns: 2, DBConnMaxLifetime: 30 * time.Minute})
	cancelOpen()
	if err != nil || db == nil || db.SQL() == nil {
		return errors.New("Draft learning worker database unavailable")
	}
	defer db.Close()
	verifyCtx, cancelVerify := context.WithTimeout(context.Background(), 10*time.Second)
	err = verifyDatabaseRole(verifyCtx, db.SQL())
	repository := agentlearningworker.NewPostgresRepository(db.SQL(), plan.ActivationID)
	if err == nil {
		err = repository.VerifyTarget(verifyCtx, plan)
	}
	cancelVerify()
	if err != nil {
		return errors.New("Draft learning worker activation target rejected")
	}
	runnerClient, err := agentrunner.NewRPCClient(resolved.runnerURL, agentrunner.ClientTLSFiles{
		CertificateFile: resolved.clientCertificate, KeyFile: resolved.clientKey,
		ServerCAFile: resolved.serverCA, ServerName: resolved.serverName,
		ClientIdentity: resolved.callerIdentity}, resolved.rpcTimeout)
	if err != nil {
		return errors.New("Draft learning worker Runner mTLS configuration invalid")
	}
	objectStore, err := storage.NewS3Store(storage.S3Config{Endpoint: resolved.s3Endpoint,
		Bucket: resolved.s3Bucket, Region: resolved.s3Region, AccessKeyID: resolved.s3AccessKey,
		SecretAccessKey: resolved.s3SecretKey, UseSSL: resolved.s3UseSSL,
		ForcePathStyle: resolved.s3ForcePathStyle || resolved.storageBackend == "minio"})
	if err != nil {
		return errors.New("Draft learning worker object store configuration invalid")
	}
	readyCtx, cancelReady := context.WithTimeout(context.Background(), 10*time.Second)
	err = objectStore.CheckReady(readyCtx)
	cancelReady()
	if err != nil {
		return errors.New("Draft learning worker object store unavailable")
	}
	authority, err := agentrunner.NewControlService(repository, privateKey)
	if err != nil {
		return errors.New("Draft learning worker authority service invalid")
	}
	runtime, err := agentlearningworker.NewRuntime(plan, repository, authority, runnerClient)
	if err != nil {
		return errors.New("Draft learning worker Runner configuration invalid")
	}
	learningRepository := agentlearning.NewActivationPostgresRepository(db.SQL(), plan.ActivationID)
	learning := agentlearning.NewService(agentlearning.WithRepository(learningRepository),
		agentlearning.WithObjectStore(objectStore), agentlearning.WithLearningEnabled(true),
		agentlearning.WithCheckers(agentlearning.StaticChecker{}, runtime.IsolationChecker(),
			runtime.EvaluationChecker()))
	service, err := agentlearningworker.NewService(agentlearningworker.Config{Owner: resolved.owner,
		PollInterval: resolved.pollInterval, LeaseDuration: resolved.leaseDuration,
		BatchSize: resolved.batchSize, Retention: resolved.retention,
		MaintenanceEvery: resolved.maintenanceEvery}, learning, runtime)
	if err != nil {
		return errors.New("Draft learning worker configuration invalid")
	}
	if args[0] == "healthcheck" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return service.Health(ctx)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("agent_draft_learning_worker_started", slog.String("activation_id", plan.ActivationID))
	if err := service.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	logger.Info("agent_draft_learning_worker_stopped")
	return nil
}

func loadWorkerConfig(lookup func(string) (string, bool)) (workerConfig, error) {
	var result workerConfig
	var err error
	for _, setting := range []struct {
		name   string
		target *bool
	}{
		{envEnabled, &result.enabled}, {envRuntimeEnabled, &result.runtimeEnabled},
		{envSchedulerEnabled, &result.schedulerEnabled}, {envLearningEnabled, &result.learningEnabled},
		{envSkillInstallEnabled, &result.skillInstallEnabled},
		{envBrokerReadEnabled, &result.brokerReadEnabled}, {envBrokerWriteEnabled, &result.brokerWriteEnabled},
		{envDelegationEnabled, &result.delegationEnabled},
	} {
		*setting.target, err = boolSetting(lookup, setting.name, false)
		if err != nil {
			return workerConfig{}, err
		}
	}
	if !result.enabled {
		return workerConfig{}, fmt.Errorf("%s must be true", envEnabled)
	}
	if result.runtimeEnabled || result.schedulerEnabled || result.learningEnabled ||
		result.skillInstallEnabled || result.brokerReadEnabled || result.brokerWriteEnabled ||
		result.delegationEnabled {
		return workerConfig{}, errors.New("Draft learning worker requires broad Agent Runtime flags to remain false")
	}
	for name, target := range map[string]*string{
		envDatabaseURL: &result.databaseURL, envPlanFile: &result.planFile,
		envRunnerURL: &result.runnerURL, envRunnerID: &result.runnerID,
		envServerName: &result.serverName, envCallerIdentity: &result.callerIdentity,
		envClientCertificate: &result.clientCertificate, envClientKey: &result.clientKey,
		envServerCA: &result.serverCA, envAuthorityPrivateKey: &result.authorityPrivateKey,
		envAuthorityPublicKey: &result.authorityPublicKey, envOwner: &result.owner,
		envReleaseManifest: &result.releaseManifest, envProductionPolicy: &result.productionPolicy,
		envActivationRecord: &result.activationRecord, envReleaseCommit: &result.releaseCommit,
		envObjectCredentialMeta: &result.objectCredentialMeta,
		"STORAGE_BACKEND":       &result.storageBackend, "S3_ENDPOINT": &result.s3Endpoint,
		"S3_BUCKET": &result.s3Bucket, "S3_REGION": &result.s3Region,
		"S3_ACCESS_KEY_ID": &result.s3AccessKey, "S3_SECRET_ACCESS_KEY": &result.s3SecretKey,
	} {
		*target = env(lookup, name, "")
		if *target == "" {
			return workerConfig{}, fmt.Errorf("%s is required", name)
		}
	}
	if result.callerIdentity != agentlearningworker.CallerIdentity {
		return workerConfig{}, fmt.Errorf("%s must be the dedicated Draft learning identity", envCallerIdentity)
	}
	for name, value := range map[string]string{
		envPlanFile: result.planFile, envClientCertificate: result.clientCertificate,
		envClientKey: result.clientKey, envServerCA: result.serverCA,
		envAuthorityPrivateKey: result.authorityPrivateKey,
		envAuthorityPublicKey:  result.authorityPublicKey,
		envReleaseManifest:     result.releaseManifest, envProductionPolicy: result.productionPolicy,
		envActivationRecord:     result.activationRecord,
		envObjectCredentialMeta: result.objectCredentialMeta,
	} {
		if !filepath.IsAbs(value) {
			return workerConfig{}, fmt.Errorf("%s must be absolute", name)
		}
	}
	if len(result.releaseCommit) != 40 || strings.Trim(result.releaseCommit, "0123456789abcdef") != "" {
		return workerConfig{}, fmt.Errorf("%s must be a lowercase Git commit", envReleaseCommit)
	}
	result.prerequisites = workerPrerequisiteFiles(lookup, "AGENT_DRAFT_LEARNING_WORKER")
	for _, path := range result.prerequisites {
		if !filepath.IsAbs(path) {
			return workerConfig{}, errors.New("Draft learning worker prerequisite activation files are required")
		}
	}
	parsed, parseErr := url.Parse(result.databaseURL)
	password, hasPassword := "", false
	if parsed != nil && parsed.User != nil {
		password, hasPassword = parsed.User.Password()
	}
	if parseErr != nil || parsed == nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") ||
		parsed.Hostname() == "" || parsed.User == nil || !hasPassword || password == "" {
		return workerConfig{}, fmt.Errorf("%s must be a PostgreSQL URL", envDatabaseURL)
	}
	result.storageBackend = strings.ToLower(result.storageBackend)
	if result.storageBackend != "minio" && result.storageBackend != "s3" {
		return workerConfig{}, errors.New("Draft learning worker requires an S3-compatible object store")
	}
	result.s3UseSSL, err = boolSetting(lookup, "S3_USE_SSL", false)
	if err != nil {
		return workerConfig{}, err
	}
	result.s3ForcePathStyle, err = boolSetting(lookup, "S3_FORCE_PATH_STYLE", result.storageBackend == "minio")
	if err != nil {
		return workerConfig{}, err
	}
	if autoCreate, autoErr := boolSetting(lookup, "S3_BUCKET_AUTO_CREATE", false); autoErr != nil || autoCreate {
		return workerConfig{}, errors.New("Draft learning worker object bucket auto-create must be false")
	}
	result.pollInterval, err = durationSetting(lookup, envPollInterval, 10*time.Second, time.Second, time.Minute)
	if err != nil {
		return workerConfig{}, err
	}
	result.rpcTimeout, err = durationSetting(lookup, envRPCTimeout, 10*time.Second, time.Second, time.Minute)
	if err != nil {
		return workerConfig{}, err
	}
	result.leaseDuration, err = durationSetting(lookup, envLeaseDuration, 2*time.Minute, 30*time.Second, 5*time.Minute)
	if err != nil {
		return workerConfig{}, err
	}
	result.retention, err = durationSetting(lookup, envRetention, 30*24*time.Hour, time.Hour, 365*24*time.Hour)
	if err != nil {
		return workerConfig{}, err
	}
	result.batchSize, err = intSetting(lookup, envBatchSize, 10, 1, 1000)
	if err != nil {
		return workerConfig{}, err
	}
	result.maintenanceEvery, err = intSetting(lookup, envMaintenanceEvery, 60, 1, 10_000)
	if err != nil {
		return workerConfig{}, err
	}
	return result, nil
}

func workerPrerequisiteFiles(lookup func(string) (string, bool), prefix string) map[string]string {
	return map[string]string{
		"control_plane":           env(lookup, prefix+"_CONTROL_ACTIVATION_FILE", ""),
		"root_run_canary":         env(lookup, prefix+"_ROOT_ACTIVATION_FILE", ""),
		"broker_artifact_canary":  env(lookup, prefix+"_BROKER_ACTIVATION_FILE", ""),
		"project_mutation_canary": env(lookup, prefix+"_PROJECT_ACTIVATION_FILE", ""),
		"child_run_canary":        env(lookup, prefix+"_CHILD_ACTIVATION_FILE", ""),
	}
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
  AND NOT current_login.rolsuper AND NOT current_login.rolcreatedb
  AND NOT current_login.rolcreaterole AND NOT current_login.rolreplication
  AND NOT current_login.rolbypassrls
  AND (SELECT array_agg(rolname ORDER BY rolname) FROM inherited_names)
      = ARRAY['agent_learning_worker']::name[]
  AND NOT has_table_privilege(current_user,'agent_learning_drafts','SELECT,INSERT,UPDATE,DELETE')
  AND NOT has_table_privilege(current_user,'agent_learning_runner_attempts','SELECT,INSERT,UPDATE,DELETE')
  AND NOT has_function_privilege(current_user,'agent_learning_promote(text,uuid,bigint,text,text,uuid,text,text,text,text,jsonb,text)','EXECUTE')
FROM current_login`).Scan(&accepted)
	if err != nil || !accepted {
		return errors.New("Draft learning worker database role rejected")
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
	value := env(lookup, name, strconv.FormatBool(fallback))
	if value == "true" {
		return true, nil
	}
	if value == "false" {
		return false, nil
	}
	return false, fmt.Errorf("%s must be true or false", name)
}

func durationSetting(lookup func(string) (string, bool), name string, fallback, minimum,
	maximum time.Duration,
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

func sanitize(err error) string {
	if err == nil {
		return ""
	}
	for _, allowed := range []string{"usage:", "must be", "is required", "out of range", "Draft learning", "database", "mTLS", "configuration"} {
		if strings.Contains(err.Error(), allowed) {
			return err.Error()
		}
	}
	return "agent Draft learning worker failed"
}
