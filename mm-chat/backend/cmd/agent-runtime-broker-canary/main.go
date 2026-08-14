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
	"neo-chat/mm-chat/backend/internal/agentbrokercanary"
	"neo-chat/mm-chat/backend/internal/agentbrokerrelay"
	"neo-chat/mm-chat/backend/internal/agentorchestrator"
	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/database"
	"neo-chat/mm-chat/backend/internal/mcpclient"
	"neo-chat/mm-chat/backend/internal/storage"
)

const (
	envCanaryEnabled       = "AGENT_BROKER_ARTIFACT_CANARY_ENABLED"
	envRuntimeEnabled      = "AGENT_RUNTIME_ENABLED"
	envSchedulerEnabled    = "AGENT_SCHEDULER_ENABLED"
	envSkillInstallEnabled = "AGENT_SKILL_INSTALL_ENABLED"
	envLearningEnabled     = "AGENT_LEARNING_ENABLED"
	envDelegationEnabled   = "AGENT_DELEGATION_ENABLED"
	envBrokerReadEnabled   = "AGENT_BROKER_READ_ONLY_ENABLED"
	envBrokerWriteEnabled  = "AGENT_BROKER_MUTATION_ENABLED"

	envDatabaseURL         = "AGENT_BROKER_CANARY_DATABASE_URL"
	envRunnerURL           = "AGENT_BROKER_CANARY_RUNNER_URL"
	envRunnerID            = "AGENT_BROKER_CANARY_RUNNER_ID"
	envRunnerServerName    = "AGENT_BROKER_CANARY_RUNNER_SERVER_NAME"
	envCallerIdentity      = "AGENT_BROKER_CANARY_CLIENT_IDENTITY"
	envRunnerClientCert    = "AGENT_BROKER_CANARY_CLIENT_CERT_FILE"
	envRunnerClientKey     = "AGENT_BROKER_CANARY_CLIENT_KEY_FILE"
	envRunnerServerCA      = "AGENT_BROKER_CANARY_SERVER_CA_FILE"
	envReleaseManifest     = "AGENT_BROKER_CANARY_RELEASE_MANIFEST_FILE"
	envProductionPolicy    = "AGENT_BROKER_CANARY_PRODUCTION_POLICY_FILE"
	envActivationRecord    = "AGENT_BROKER_CANARY_ACTIVATION_FILE"
	envCanaryPlan          = "AGENT_BROKER_CANARY_PLAN_FILE"
	envAuthorityPrivateKey = "AGENT_BROKER_CANARY_AUTHORITY_PRIVATE_KEY_FILE"
	envAuthorityPublicKey  = "AGENT_BROKER_CANARY_AUTHORITY_PUBLIC_KEY_FILE"
	envReleaseCommit       = "AGENT_BROKER_CANARY_RELEASE_GIT_COMMIT"

	envRelayListen         = "AGENT_BROKER_CANARY_RELAY_LISTEN_ADDR"
	envRelayEndpoint       = "AGENT_BROKER_CANARY_RELAY_ENDPOINT"
	envRelayServerCert     = "AGENT_BROKER_CANARY_RELAY_TLS_CERT_FILE"
	envRelayServerKey      = "AGENT_BROKER_CANARY_RELAY_TLS_KEY_FILE"
	envRelayClientCA       = "AGENT_BROKER_CANARY_RELAY_TLS_CLIENT_CA_FILE"
	envRunnerRelayIdentity = "AGENT_BROKER_CANARY_RUNNER_RELAY_IDENTITY"
	envQuarantineRoot      = "AGENT_BROKER_CANARY_QUARANTINE_ROOT"
	envMCPRunnerURL        = "AGENT_BROKER_CANARY_MCP_RUNNER_URL"
	envMCPRunnerTokenFile  = "AGENT_BROKER_CANARY_MCP_RUNNER_TOKEN_FILE"
	envPollInterval        = "AGENT_BROKER_CANARY_POLL_INTERVAL"
	envRPCTimeout          = "AGENT_BROKER_CANARY_RPC_TIMEOUT"
	envAuthorityTTL        = "AGENT_BROKER_CANARY_AUTHORITY_TTL"
	envBatchSize           = "AGENT_BROKER_CANARY_RECONCILE_BATCH_SIZE"
)

type workerConfig struct {
	canaryEnabled, runtimeEnabled, schedulerEnabled, skillInstallEnabled      bool
	learningEnabled, delegationEnabled, brokerReadEnabled, brokerWriteEnabled bool
	databaseURL, runnerURL, runnerID, runnerServerName, callerIdentity        string
	runnerClientCert, runnerClientKey, runnerServerCA                         string
	releaseManifest, productionPolicy, activationRecord, canaryPlan           string
	authorityPrivateKey, authorityPublicKey, releaseCommit                    string
	relayListen, relayEndpoint, relayServerCert, relayServerKey               string
	relayClientCA, runnerRelayIdentity, quarantineRoot                        string
	mcpRunnerURL, mcpRunnerTokenFile                                          string
	storageBackend, s3Endpoint, s3Bucket, s3Region, s3AccessKey, s3SecretKey  string
	s3UseSSL, s3ForcePathStyle                                                bool
	pollInterval, rpcTimeout, authorityTTL                                    time.Duration
	batchSize                                                                 int
}

type evidenceGate struct {
	config agentactivation.BrokerCanaryConfig
}

func (gate evidenceGate) Verify(now time.Time) error {
	decision, err := agentactivation.VerifyBrokerCanary(gate.config, now)
	if err != nil || !decision.Ready {
		return agentbrokercanary.ErrUnavailable
	}
	return nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(os.Args[1:], os.LookupEnv, logger); err != nil {
		logger.Error("agent_broker_canary_exit", slog.String("error", sanitizedError(err)))
		os.Exit(1)
	}
}

func run(args []string, lookup func(string) (string, bool), logger *slog.Logger) error {
	if len(args) != 1 || (args[0] != "run" && args[0] != "healthcheck") {
		return errors.New("usage: agent-runtime-broker-canary run | agent-runtime-broker-canary healthcheck")
	}
	resolved, err := loadWorkerConfig(lookup)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	plan, bindings, err := agentbrokercanary.LoadPlan(resolved.canaryPlan, resolved.runnerID, now)
	if err != nil {
		return errors.New("Broker canary plan invalid")
	}
	privateKey, err := agentrunner.LoadEd25519PrivateKey(resolved.authorityPrivateKey)
	if err != nil {
		return errors.New("Broker canary authority key invalid")
	}
	publicKey, err := agentrunner.LoadEd25519PublicKey(resolved.authorityPublicKey)
	if err != nil || !privateKey.Public().(ed25519.PublicKey).Equal(publicKey) {
		return errors.New("Broker canary authority key mismatch")
	}
	gate := evidenceGate{config: agentactivation.BrokerCanaryConfig{Config: agentactivation.Config{
		PolicyFile: resolved.productionPolicy, RecordFile: resolved.activationRecord,
		ReleaseManifestFile: resolved.releaseManifest, ClientCertificateFile: resolved.runnerClientCert,
		ServerCAFile: resolved.runnerServerCA, Endpoint: resolved.runnerURL, RunnerID: resolved.runnerID,
		ServerName: resolved.runnerServerName, CallerIdentity: resolved.callerIdentity,
		ReleaseCommit: resolved.releaseCommit}, CanaryPlanFile: resolved.canaryPlan,
		AuthorityPublicKeyFile: resolved.authorityPublicKey, RelayEndpoint: resolved.relayEndpoint,
		RelayServerCertificateFile: resolved.relayServerCert, RelayClientCAFile: resolved.relayClientCA,
		RunnerRelayIdentity: resolved.runnerRelayIdentity}}
	if err := gate.Verify(now); err != nil {
		return errors.New("Broker canary activation unavailable")
	}
	openCtx, cancelOpen := context.WithTimeout(context.Background(), 10*time.Second)
	db, err := database.Open(openCtx, config.Config{DatabaseURL: resolved.databaseURL,
		DBMaxOpenConns: 8, DBMaxIdleConns: 4, DBConnMaxLifetime: 30 * time.Minute})
	cancelOpen()
	if err != nil || db == nil || db.SQL() == nil {
		return errors.New("Broker canary database unavailable")
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
		return errors.New("Broker canary Runner mTLS configuration invalid")
	}
	mcpRunnerToken, err := readMCPRunnerToken(resolved.mcpRunnerTokenFile)
	if err != nil {
		return errors.New("Broker canary MCP Runner token invalid")
	}
	mcpConnector, err := mcpclient.NewRunnerConnector(resolved.mcpRunnerURL, mcpRunnerToken)
	if err != nil {
		return errors.New("Broker canary MCP Runner configuration invalid")
	}
	objectStore, err := storage.NewS3Store(storage.S3Config{Endpoint: resolved.s3Endpoint,
		Bucket: resolved.s3Bucket, Region: resolved.s3Region, AccessKeyID: resolved.s3AccessKey,
		SecretAccessKey: resolved.s3SecretKey, UseSSL: resolved.s3UseSSL,
		ForcePathStyle: resolved.s3ForcePathStyle || resolved.storageBackend == "minio"})
	if err != nil {
		return errors.New("Broker canary object store configuration invalid")
	}
	readyCtx, cancelReady := context.WithTimeout(context.Background(), 10*time.Second)
	err = objectStore.CheckReady(readyCtx)
	cancelReady()
	if err != nil {
		return errors.New("Broker canary object store unavailable")
	}
	quarantine, err := agentbrokercanary.NewDirectoryQuarantine(resolved.quarantineRoot)
	if err != nil {
		return errors.New("Broker canary quarantine invalid")
	}
	artifactPublisher, err := agentbroker.NewArtifactPublisher(quarantine, objectStore,
		agentbroker.NewPostgresArtifactRepository(db.SQL()), agentbrokercanary.CanaryArtifactScanner{})
	if err != nil {
		return errors.New("Broker canary Artifact publisher invalid")
	}
	readers := map[string]agentbroker.ReadOnlyExecutor{}
	effects := map[string]agentbroker.EffectExecutor{}
	for _, binding := range bindings.All() {
		switch binding.Plan.ID {
		case agentbrokercanary.ActionProjectRead, agentbrokercanary.ActionWorkspaceRead:
			readers[binding.Plan.ToolIdentity], err = agentbrokercanary.NewExactFileReader(binding.Plan)
		case agentbrokercanary.ActionMCPRead:
			readers[binding.Plan.ToolIdentity], err = agentbrokercanary.NewMCPReadExecutor(binding.Plan, mcpConnector)
		case agentbrokercanary.ActionPossibleSend:
			readers[binding.Plan.ToolIdentity], err = agentbrokercanary.NewPossibleSendExecutor(binding.Plan)
		case agentbrokercanary.ActionArtifact:
			effects[binding.Plan.ToolIdentity], err = agentbrokercanary.NewArtifactExecutor(
				binding.Plan, plan.ArtifactPolicy, artifactPublisher)
		default:
			err = agentbrokercanary.ErrInvalidPlan
		}
		if err != nil {
			return errors.New("Broker canary executor configuration invalid")
		}
	}
	broker, err := agentbroker.NewServiceWithReadOnly(agentbroker.NewPostgresRepository(db.SQL()), effects, readers)
	if err != nil {
		return errors.New("Broker canary service configuration invalid")
	}
	target, err := agentbrokercanary.NewRelayTarget(broker, bindings)
	if err != nil {
		return errors.New("Broker canary relay target invalid")
	}
	verifier, err := agentrunner.NewSignedAuthorityVerifier(publicKey)
	if err != nil {
		return errors.New("Broker canary authority verifier invalid")
	}
	relayHandler, err := agentbrokerrelay.NewHandler(target, verifier, resolved.runnerRelayIdentity,
		resolved.callerIdentity, resolved.runnerID)
	if err != nil {
		return errors.New("Broker canary relay handler invalid")
	}
	relayTLS, err := agentrunner.LoadServerTLS(agentrunner.TLSFiles{CertificateFile: resolved.relayServerCert,
		KeyFile: resolved.relayServerKey, ClientCAFile: resolved.relayClientCA})
	if err != nil {
		return errors.New("Broker canary relay TLS configuration invalid")
	}
	runnerRepository := agentrunner.NewPostgresControlRepository(db.SQL())
	authority, err := agentrunner.NewControlService(runnerRepository, privateKey)
	if err != nil {
		return errors.New("Broker canary authority service invalid")
	}
	service, err := agentbrokercanary.NewService(agentbrokercanary.Config{RunnerID: resolved.runnerID,
		CallerIdentity: resolved.callerIdentity, PollInterval: resolved.pollInterval,
		AuthorityTTL: resolved.authorityTTL, BatchSize: resolved.batchSize}, plan, bindings, gate,
		agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(db.SQL())), authority,
		runnerRepository, runnerClient)
	if err != nil {
		return errors.New("Broker canary controller configuration invalid")
	}
	if args[0] == "healthcheck" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return service.Health(ctx)
	}
	listener, err := net.Listen("tcp", resolved.relayListen)
	if err != nil {
		return errors.New("Broker canary relay listener unavailable")
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
	logger.Info("agent_broker_canary_started", slog.String("stage", agentactivation.StageBrokerCanary))
	select {
	case <-ctx.Done():
	case serveErr := <-serverErrors:
		if !errors.Is(serveErr, http.ErrServerClosed) {
			return errors.New("Broker canary relay stopped")
		}
	case workerErr := <-workerErrors:
		if workerErr != nil && !errors.Is(workerErr, context.Canceled) {
			return workerErr
		}
	}
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := relayServer.Shutdown(shutdownCtx); err != nil {
		return errors.New("Broker canary relay shutdown failed")
	}
	logger.Info("agent_broker_canary_stopped")
	return nil
}

func sanitizedError(err error) string {
	if err == nil {
		return ""
	}
	for _, allowed := range []string{"usage:", "must be", "is required", "out of range", "G21.2",
		"canary", "database", "mTLS", "configuration", "BROKER_CANARY"} {
		if strings.Contains(err.Error(), allowed) {
			return err.Error()
		}
	}
	return "agent Broker canary failed"
}
