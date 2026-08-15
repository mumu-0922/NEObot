package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"neo-chat/mm-chat/backend/internal/agentactivation"
	"neo-chat/mm-chat/backend/internal/agentcron"
	"neo-chat/mm-chat/backend/internal/agentcronworker"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/database"
)

const (
	envEnabled             = "AGENT_CRON_WORKER_ENABLED"
	envRuntimeEnabled      = "AGENT_RUNTIME_ENABLED"
	envSchedulerEnabled    = "AGENT_SCHEDULER_ENABLED"
	envLearningEnabled     = "AGENT_LEARNING_ENABLED"
	envSkillInstallEnabled = "AGENT_SKILL_INSTALL_ENABLED"
	envBrokerReadEnabled   = "AGENT_BROKER_READ_ONLY_ENABLED"
	envBrokerWriteEnabled  = "AGENT_BROKER_MUTATION_ENABLED"
	envDelegationEnabled   = "AGENT_DELEGATION_ENABLED"
	envDatabaseURL         = "AGENT_CRON_WORKER_DATABASE_URL"
	envPlanFile            = "AGENT_CRON_WORKER_PLAN_FILE"
	envProductionPolicy    = "AGENT_CRON_WORKER_PRODUCTION_POLICY_FILE"
	envActivationRecord    = "AGENT_CRON_WORKER_ACTIVATION_FILE"
	envReleaseCommit       = "AGENT_CRON_WORKER_RELEASE_GIT_COMMIT"
)

type workerConfig struct {
	enabled, runtimeEnabled, schedulerEnabled, learningEnabled bool
	skillInstallEnabled, brokerReadEnabled, brokerWriteEnabled bool
	delegationEnabled                                          bool
	databaseURL, planFile, productionPolicy, activationRecord  string
	releaseCommit                                              string
	prerequisites                                              map[string]string
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(os.Args[1:], os.LookupEnv, logger); err != nil {
		logger.Error("agent_cron_worker_exit", slog.String("error", sanitize(err)))
		os.Exit(1)
	}
}

func run(args []string, lookup func(string) (string, bool), logger *slog.Logger) error {
	if len(args) != 1 || (args[0] != "run" && args[0] != "healthcheck") {
		return errors.New("usage: agent-runtime-cron-worker run | agent-runtime-cron-worker healthcheck")
	}
	resolved, err := loadWorkerConfig(lookup)
	if err != nil {
		return err
	}
	plan, err := agentcronworker.LoadPlan(resolved.planFile)
	if err != nil {
		return errors.New("Cron worker plan invalid")
	}
	decision, err := agentactivation.VerifyCronWorker(agentactivation.WorkerConfig{
		PolicyFile: resolved.productionPolicy, RecordFile: resolved.activationRecord,
		PlanFile: resolved.planFile, ReleaseCommit: resolved.releaseCommit,
		PrerequisiteFiles: resolved.prerequisites,
	}, time.Now().UTC())
	if err != nil || !decision.Ready {
		return errors.New("Cron worker activation unavailable")
	}
	openCtx, cancelOpen := context.WithTimeout(context.Background(), 10*time.Second)
	db, err := database.Open(openCtx, config.Config{DatabaseURL: resolved.databaseURL,
		DBMaxOpenConns: 4, DBMaxIdleConns: 2, DBConnMaxLifetime: 30 * time.Minute})
	cancelOpen()
	if err != nil || db == nil || db.SQL() == nil {
		return errors.New("Cron worker database unavailable")
	}
	defer db.Close()
	verifyCtx, cancelVerify := context.WithTimeout(context.Background(), 10*time.Second)
	err = verifyDatabaseRole(verifyCtx, db.SQL())
	if err == nil {
		err = agentcronworker.VerifyPostgresTarget(verifyCtx, db.SQL(), plan)
	}
	cancelVerify()
	if err != nil {
		return errors.New("Cron worker activation target rejected")
	}
	if args[0] == "healthcheck" {
		return nil
	}
	repository := agentcron.NewActivationPostgresRepository(db.SQL(), plan.ActivationID)
	worker, err := agentcronworker.NewService(plan.Config(), agentcron.NewService(repository))
	if err != nil {
		return errors.New("Cron worker configuration invalid")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("agent_cron_worker_started", slog.String("activation_id", plan.ActivationID))
	if err := worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	logger.Info("agent_cron_worker_stopped")
	return nil
}

func loadWorkerConfig(lookup func(string) (string, bool)) (workerConfig, error) {
	var result workerConfig
	var err error
	for _, item := range []struct {
		name   string
		target *bool
	}{
		{envEnabled, &result.enabled}, {envRuntimeEnabled, &result.runtimeEnabled},
		{envSchedulerEnabled, &result.schedulerEnabled}, {envLearningEnabled, &result.learningEnabled},
		{envSkillInstallEnabled, &result.skillInstallEnabled},
		{envBrokerReadEnabled, &result.brokerReadEnabled},
		{envBrokerWriteEnabled, &result.brokerWriteEnabled}, {envDelegationEnabled, &result.delegationEnabled},
	} {
		*item.target, err = boolSetting(lookup, item.name)
		if err != nil {
			return workerConfig{}, err
		}
	}
	if !result.enabled {
		return workerConfig{}, errors.New("Cron worker is disabled")
	}
	if result.runtimeEnabled || result.schedulerEnabled || result.learningEnabled ||
		result.skillInstallEnabled || result.brokerReadEnabled || result.brokerWriteEnabled ||
		result.delegationEnabled {
		return workerConfig{}, errors.New("Cron worker requires broad Agent Runtime flags to remain false")
	}
	result.databaseURL = env(lookup, envDatabaseURL)
	result.planFile = env(lookup, envPlanFile)
	result.productionPolicy = env(lookup, envProductionPolicy)
	result.activationRecord = env(lookup, envActivationRecord)
	result.releaseCommit = env(lookup, envReleaseCommit)
	result.prerequisites = workerPrerequisiteFiles(lookup, "AGENT_CRON_WORKER")
	if result.databaseURL == "" || !strings.HasPrefix(result.planFile, "/") ||
		!strings.HasPrefix(result.productionPolicy, "/") ||
		!strings.HasPrefix(result.activationRecord, "/") ||
		len(result.releaseCommit) != 40 || strings.Trim(result.releaseCommit, "0123456789abcdef") != "" {
		return workerConfig{}, errors.New("Cron worker database and plan are required")
	}
	for _, path := range result.prerequisites {
		if !strings.HasPrefix(path, "/") {
			return workerConfig{}, errors.New("Cron worker prerequisite activation files are required")
		}
	}
	return result, nil
}

func workerPrerequisiteFiles(lookup func(string) (string, bool), prefix string) map[string]string {
	return map[string]string{
		"control_plane":           env(lookup, prefix+"_CONTROL_ACTIVATION_FILE"),
		"root_run_canary":         env(lookup, prefix+"_ROOT_ACTIVATION_FILE"),
		"broker_artifact_canary":  env(lookup, prefix+"_BROKER_ACTIVATION_FILE"),
		"project_mutation_canary": env(lookup, prefix+"_PROJECT_ACTIVATION_FILE"),
		"child_run_canary":        env(lookup, prefix+"_CHILD_ACTIVATION_FILE"),
	}
}

func verifyDatabaseRole(ctx context.Context, db *sql.DB) error {
	var accepted bool
	err := db.QueryRowContext(ctx, `
WITH RECURSIVE current_login AS (
  SELECT oid,rolcanlogin,rolinherit,rolsuper,rolcreatedb,rolcreaterole,rolreplication,rolbypassrls
  FROM pg_roles WHERE rolname=current_user
), inherited(role_oid) AS (
  SELECT membership.roleid FROM pg_auth_members membership
    JOIN current_login ON current_login.oid=membership.member
  UNION
  SELECT membership.roleid FROM pg_auth_members membership
    JOIN inherited ON inherited.role_oid=membership.member
), inherited_names AS (
  SELECT role.rolname FROM inherited JOIN pg_roles role ON role.oid=inherited.role_oid
)
SELECT current_login.rolcanlogin AND current_login.rolinherit
  AND NOT current_login.rolsuper AND NOT current_login.rolcreatedb
  AND NOT current_login.rolcreaterole AND NOT current_login.rolreplication
  AND NOT current_login.rolbypassrls
  AND (SELECT array_agg(rolname ORDER BY rolname) FROM inherited_names)
      = ARRAY['agent_cron_worker']::name[]
FROM current_login`).Scan(&accepted)
	if err != nil || !accepted {
		return errors.New("Cron worker database role rejected")
	}
	return nil
}

func boolSetting(lookup func(string) (string, bool), name string) (bool, error) {
	value := env(lookup, name)
	if value == "true" {
		return true, nil
	}
	if value == "false" || value == "" {
		return false, nil
	}
	return false, fmt.Errorf("%s must be true or false", name)
}

func env(lookup func(string) (string, bool), name string) string {
	value, _ := lookup(name)
	return strings.TrimSpace(value)
}

func sanitize(err error) string {
	if err == nil {
		return ""
	}
	value := err.Error()
	if len(value) > 256 {
		value = value[:256]
	}
	return value
}
