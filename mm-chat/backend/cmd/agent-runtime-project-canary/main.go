package main

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"neo-chat/mm-chat/backend/internal/agentactivation"
	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentbrokerrelay"
	"neo-chat/mm-chat/backend/internal/agentorchestrator"
	"neo-chat/mm-chat/backend/internal/agentprojectcanary"
	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/database"
)

const (
	envCanaryEnabled       = "AGENT_PROJECT_MUTATION_CANARY_ENABLED"
	envControlEnabled      = "AGENT_RUNNER_CONTROL_ENABLED"
	envRootCanaryEnabled   = "AGENT_ROOT_RUN_CANARY_ENABLED"
	envBrokerCanaryEnabled = "AGENT_BROKER_ARTIFACT_CANARY_ENABLED"
	envRuntimeEnabled      = "AGENT_RUNTIME_ENABLED"
	envSchedulerEnabled    = "AGENT_SCHEDULER_ENABLED"
	envSkillInstallEnabled = "AGENT_SKILL_INSTALL_ENABLED"
	envLearningEnabled     = "AGENT_LEARNING_ENABLED"
	envDelegationEnabled   = "AGENT_DELEGATION_ENABLED"
	envBrokerReadEnabled   = "AGENT_BROKER_READ_ONLY_ENABLED"
	envBrokerWriteEnabled  = "AGENT_BROKER_MUTATION_ENABLED"

	envDatabaseURL         = "AGENT_PROJECT_CANARY_DATABASE_URL"
	envRunnerURL           = "AGENT_PROJECT_CANARY_RUNNER_URL"
	envRunnerID            = "AGENT_PROJECT_CANARY_RUNNER_ID"
	envRunnerServerName    = "AGENT_PROJECT_CANARY_RUNNER_SERVER_NAME"
	envCallerIdentity      = "AGENT_PROJECT_CANARY_CLIENT_IDENTITY"
	envRunnerClientCert    = "AGENT_PROJECT_CANARY_CLIENT_CERT_FILE"
	envRunnerClientKey     = "AGENT_PROJECT_CANARY_CLIENT_KEY_FILE"
	envRunnerServerCA      = "AGENT_PROJECT_CANARY_SERVER_CA_FILE"
	envReleaseManifest     = "AGENT_PROJECT_CANARY_RELEASE_MANIFEST_FILE"
	envProductionPolicy    = "AGENT_PROJECT_CANARY_PRODUCTION_POLICY_FILE"
	envActivationRecord    = "AGENT_PROJECT_CANARY_ACTIVATION_FILE"
	envCanaryPlan          = "AGENT_PROJECT_CANARY_PLAN_FILE"
	envAuthorityPrivateKey = "AGENT_PROJECT_CANARY_AUTHORITY_PRIVATE_KEY_FILE"
	envAuthorityPublicKey  = "AGENT_PROJECT_CANARY_AUTHORITY_PUBLIC_KEY_FILE"
	envApprovalDocument    = "AGENT_PROJECT_CANARY_APPROVAL_DOCUMENT_FILE"
	envApprovalPublicKey   = "AGENT_PROJECT_CANARY_APPROVAL_PUBLIC_KEY_FILE"
	envReleaseCommit       = "AGENT_PROJECT_CANARY_RELEASE_GIT_COMMIT"

	envRelayListen         = "AGENT_PROJECT_CANARY_RELAY_LISTEN_ADDR"
	envRelayEndpoint       = "AGENT_PROJECT_CANARY_RELAY_ENDPOINT"
	envRelayServerCert     = "AGENT_PROJECT_CANARY_RELAY_TLS_CERT_FILE"
	envRelayServerKey      = "AGENT_PROJECT_CANARY_RELAY_TLS_KEY_FILE"
	envRelayClientCA       = "AGENT_PROJECT_CANARY_RELAY_TLS_CLIENT_CA_FILE"
	envRunnerRelayIdentity = "AGENT_PROJECT_CANARY_RUNNER_RELAY_IDENTITY"
	envPollInterval        = "AGENT_PROJECT_CANARY_POLL_INTERVAL"
	envRPCTimeout          = "AGENT_PROJECT_CANARY_RPC_TIMEOUT"
	envAuthorityTTL        = "AGENT_PROJECT_CANARY_AUTHORITY_TTL"
	envBatchSize           = "AGENT_PROJECT_CANARY_RECONCILE_BATCH_SIZE"
)

type workerConfig struct {
	canaryEnabled, controlEnabled, rootCanaryEnabled, brokerCanaryEnabled        bool
	runtimeEnabled, schedulerEnabled, skillInstallEnabled, learningEnabled       bool
	delegationEnabled, brokerReadEnabled, brokerWriteEnabled                     bool
	databaseURL, runnerURL, runnerID, runnerServerName, callerIdentity           string
	runnerClientCert, runnerClientKey, runnerServerCA                            string
	releaseManifest, productionPolicy, activationRecord, canaryPlan              string
	authorityPrivateKey, authorityPublicKey, approvalDocument, approvalPublicKey string
	releaseCommit, relayListen, relayEndpoint, relayServerCert, relayServerKey   string
	relayClientCA, runnerRelayIdentity                                           string
	pollInterval, rpcTimeout, authorityTTL                                       time.Duration
	batchSize                                                                    int
}

type evidenceGate struct {
	config agentactivation.ProjectCanaryConfig
}

func (gate evidenceGate) Verify(now time.Time) error {
	decision, err := agentactivation.VerifyProjectCanary(gate.config, now)
	if err != nil || !decision.Ready {
		return agentprojectcanary.ErrUnavailable
	}
	return nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(os.Args[1:], os.LookupEnv, logger); err != nil {
		logger.Error("agent_project_canary_exit", slog.String("error", sanitizedError(err)))
		os.Exit(1)
	}
}

func run(args []string, lookup func(string) (string, bool), logger *slog.Logger) error {
	if len(args) != 1 || (args[0] != "run" && args[0] != "healthcheck") {
		return errors.New("usage: agent-runtime-project-canary run | agent-runtime-project-canary healthcheck")
	}
	resolved, err := loadWorkerConfig(lookup)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	plan, bindings, planFingerprint, err := agentprojectcanary.LoadPlan(resolved.canaryPlan, resolved.runnerID, now)
	if err != nil {
		return errors.New("Project canary plan invalid")
	}
	binding, ok := bindings.Action()
	if !ok {
		return errors.New("Project canary action binding invalid")
	}
	privateKey, err := agentrunner.LoadEd25519PrivateKey(resolved.authorityPrivateKey)
	if err != nil {
		return errors.New("Project canary authority key invalid")
	}
	publicKey, err := agentrunner.LoadEd25519PublicKey(resolved.authorityPublicKey)
	if err != nil || !privateKey.Public().(ed25519.PublicKey).Equal(publicKey) {
		return errors.New("Project canary authority key mismatch")
	}
	approvalPublicKey, err := agentrunner.LoadEd25519PublicKey(resolved.approvalPublicKey)
	if err != nil || publicKey.Equal(approvalPublicKey) {
		return errors.New("Project canary approval authority is invalid or reused")
	}
	gate := evidenceGate{config: agentactivation.ProjectCanaryConfig{Config: agentactivation.Config{
		PolicyFile: resolved.productionPolicy, RecordFile: resolved.activationRecord,
		ReleaseManifestFile: resolved.releaseManifest, ClientCertificateFile: resolved.runnerClientCert,
		ServerCAFile: resolved.runnerServerCA, Endpoint: resolved.runnerURL, RunnerID: resolved.runnerID,
		ServerName: resolved.runnerServerName, CallerIdentity: resolved.callerIdentity,
		ReleaseCommit: resolved.releaseCommit}, CanaryPlanFile: resolved.canaryPlan,
		AuthorityPublicKeyFile: resolved.authorityPublicKey, ApprovalDocumentFile: resolved.approvalDocument,
		ApprovalPublicKeyFile: resolved.approvalPublicKey, TargetFingerprint: plan.TargetFingerprint,
		RelayEndpoint: resolved.relayEndpoint, RelayServerCertificateFile: resolved.relayServerCert,
		RelayClientCAFile: resolved.relayClientCA, RunnerRelayIdentity: resolved.runnerRelayIdentity}}
	decision, err := agentactivation.VerifyProjectCanary(gate.config, now)
	if err != nil || !decision.Ready {
		return errors.New("Project canary activation unavailable")
	}
	approval, err := agentprojectcanary.LoadApproval(resolved.approvalDocument, resolved.approvalPublicKey,
		agentprojectcanary.ApprovalBinding{ReleaseCommit: resolved.releaseCommit,
			TargetFingerprint: plan.TargetFingerprint, RunnerID: resolved.runnerID,
			ActivationFingerprint: agentprojectcanary.ActivationBindingFingerprint(resolved.releaseCommit,
				plan.TargetFingerprint, resolved.runnerID, planFingerprint, resolved.callerIdentity,
				resolved.relayEndpoint), PlanFingerprint: planFingerprint,
			CallerIdentity: resolved.callerIdentity, RequestIdentity: binding.Plan.RequestIdentity,
			IdempotencyKey: binding.Plan.IdempotencyKey, Action: binding}, now)
	if err != nil {
		return errors.New("Project canary signed approval invalid")
	}
	openCtx, cancelOpen := context.WithTimeout(context.Background(), 10*time.Second)
	db, err := database.Open(openCtx, config.Config{DatabaseURL: resolved.databaseURL,
		DBMaxOpenConns: 8, DBMaxIdleConns: 4, DBConnMaxLifetime: 30 * time.Minute})
	cancelOpen()
	if err != nil || db == nil || db.SQL() == nil {
		return errors.New("Project canary database unavailable")
	}
	defer db.Close()
	roleCtx, cancelRole := context.WithTimeout(context.Background(), 10*time.Second)
	err = verifyDatabaseRole(roleCtx, db.SQL())
	cancelRole()
	if err != nil {
		return err
	}
	runnerClient, err := agentrunner.NewRPCClient(resolved.runnerURL, agentrunner.ClientTLSFiles{
		CertificateFile: resolved.runnerClientCert, KeyFile: resolved.runnerClientKey,
		ServerCAFile: resolved.runnerServerCA, ServerName: resolved.runnerServerName,
		ClientIdentity: resolved.callerIdentity}, resolved.rpcTimeout)
	if err != nil {
		return errors.New("Project canary Runner mTLS configuration invalid")
	}
	projectExecutor, err := agentbroker.NewProjectEffectExecutor(
		agentbroker.NewPostgresProjectMutationRepository(db.SQL()), binding.MutationAuthority())
	if err != nil {
		return errors.New("Project canary executor configuration invalid")
	}
	broker, err := agentbroker.NewService(agentbroker.NewPostgresRepository(db.SQL()),
		map[string]agentbroker.EffectExecutor{"project.patch": projectExecutor})
	if err != nil {
		return errors.New("Project canary Broker service configuration invalid")
	}
	target, err := agentprojectcanary.NewRelayTarget(broker, bindings)
	if err != nil {
		return errors.New("Project canary relay target invalid")
	}
	verifier, err := agentrunner.NewSignedAuthorityVerifier(publicKey)
	if err != nil {
		return errors.New("Project canary authority verifier invalid")
	}
	relayHandler, err := agentbrokerrelay.NewHandler(target, verifier, resolved.runnerRelayIdentity,
		resolved.callerIdentity, resolved.runnerID)
	if err != nil {
		return errors.New("Project canary relay handler invalid")
	}
	relayTLS, err := agentrunner.LoadServerTLS(agentrunner.TLSFiles{CertificateFile: resolved.relayServerCert,
		KeyFile: resolved.relayServerKey, ClientCAFile: resolved.relayClientCA})
	if err != nil {
		return errors.New("Project canary relay TLS configuration invalid")
	}
	runnerRepository := agentrunner.NewPostgresControlRepository(db.SQL())
	authority, err := agentrunner.NewControlService(runnerRepository, privateKey)
	if err != nil {
		return errors.New("Project canary authority service invalid")
	}
	service, err := agentprojectcanary.NewService(agentprojectcanary.Config{RunnerID: resolved.runnerID,
		CallerIdentity: resolved.callerIdentity, PollInterval: resolved.pollInterval,
		AuthorityTTL: resolved.authorityTTL, BatchSize: resolved.batchSize}, plan, bindings, gate,
		agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(db.SQL())), authority,
		runnerRepository, runnerClient, broker, approval, projectExecutor)
	if err != nil {
		return errors.New("Project canary controller configuration invalid")
	}
	if args[0] == "healthcheck" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return service.Health(ctx)
	}
	listener, err := net.Listen("tcp", resolved.relayListen)
	if err != nil {
		return errors.New("Project canary relay listener unavailable")
	}
	relayServer := &http.Server{Handler: relayHandler, TLSConfig: relayTLS,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- relayServer.Serve(tls.NewListener(listener, relayTLS)) }()
	workerErrors := make(chan error, 1)
	go func() { workerErrors <- service.Run(ctx) }()
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("agent_project_canary_started", slog.String("stage", agentactivation.StageProjectCanary))
	select {
	case <-ctx.Done():
	case serveErr := <-serverErrors:
		if !errors.Is(serveErr, http.ErrServerClosed) {
			return errors.New("Project canary relay stopped")
		}
	case workerErr := <-workerErrors:
		if workerErr != nil && !errors.Is(workerErr, context.Canceled) {
			return workerErr
		}
	}
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := relayServer.Shutdown(shutdownCtx); err != nil {
		return errors.New("Project canary relay shutdown failed")
	}
	logger.Info("agent_project_canary_stopped")
	return nil
}

func sanitizedError(err error) string {
	if err == nil {
		return ""
	}
	for _, allowed := range []string{"usage:", "must be", "is required", "out of range", "G21.3",
		"canary", "database", "mTLS", "configuration", "PROJECT_CANARY"} {
		if strings.Contains(err.Error(), allowed) {
			return err.Error()
		}
	}
	return "agent Project canary failed"
}
