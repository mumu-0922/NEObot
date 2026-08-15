package main

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"neo-chat/mm-chat/backend/internal/agentbrokerrelay"
	"neo-chat/mm-chat/backend/internal/agentrunner"
)

const (
	controlCallerIdentity       = "spiffe://neo-chat/agent-runtime-control"
	rootCanaryCallerIdentity    = "spiffe://neo-chat/agent-runtime-root-canary"
	brokerCanaryCallerIdentity  = "spiffe://neo-chat/agent-runtime-broker-canary"
	brokerRelayIdentity         = "spiffe://neo-chat/neo-runner-broker-relay"
	projectCanaryCallerIdentity = "spiffe://neo-chat/agent-runtime-project-canary"
	projectRelayIdentity        = "spiffe://neo-chat/neo-runner-project-relay"
	childCanaryCallerIdentity   = "spiffe://neo-chat/agent-runtime-child-canary"
	draftLearningCallerIdentity = "spiffe://neo-chat/agent-runtime-draft-learning"
	productCanaryCallerIdentity = "spiffe://neo-chat/agent-runtime-product-canary"
)

func main() {
	if err := run(); err != nil {
		log.Fatal("neo-runnerd startup failed")
	}
}

func run() error {
	if os.Geteuid() == 0 {
		return errors.New("neo-runnerd refuses root execution")
	}
	stateRoot := cleanAbsoluteEnv("NEO_RUNNER_STATE_ROOT")
	listenAddress := strings.TrimSpace(os.Getenv("NEO_RUNNER_LISTEN_ADDR"))
	if stateRoot == "" || !privateAddress(listenAddress) {
		return errors.New("neo-runnerd configuration is invalid")
	}
	manifest, err := agentrunner.LoadReleaseManifest(cleanAbsoluteEnv("NEO_RUNNER_RELEASE_MANIFEST"))
	if err != nil {
		return err
	}
	publicKey, err := agentrunner.LoadEd25519PublicKey(cleanAbsoluteEnv("NEO_RUNNER_AUTHORITY_PUBLIC_KEY_FILE"))
	if err != nil {
		return err
	}
	authority, err := agentrunner.NewSignedAuthorityVerifier(publicKey)
	if err != nil {
		return err
	}
	tlsConfig, err := agentrunner.LoadServerTLS(agentrunner.TLSFiles{CertificateFile: cleanAbsoluteEnv("NEO_RUNNER_TLS_CERT_FILE"), KeyFile: cleanAbsoluteEnv("NEO_RUNNER_TLS_KEY_FILE"), ClientCAFile: cleanAbsoluteEnv("NEO_RUNNER_TLS_CLIENT_CA_FILE")})
	if err != nil {
		return err
	}
	workspaceCatalog, err := agentrunner.NewWorkspaceCatalog(filepath.Join(stateRoot, "workspaces"))
	if err != nil {
		return err
	}
	artifactBroker, err := agentrunner.NewArtifactBroker(filepath.Join(stateRoot, "artifacts"))
	if err != nil {
		return err
	}
	replay, err := agentrunner.NewFileReplayLedger(filepath.Join(stateRoot, "replay", "requests.jsonl"))
	if err != nil {
		return err
	}
	driver, err := agentrunner.NewPodmanDriver(binaryPath(manifest, "podman"), agentrunner.ExecCommandRunner{})
	if err != nil {
		return err
	}
	service, err := agentrunner.NewService(agentrunner.ServiceConfig{RunnerID: manifest.RunnerID, StateRoot: stateRoot,
		SeccompPath: manifest.SeccompProfile.Path, SeccompFingerprint: manifest.SeccompProfile.SHA256,
		ReplayTTL: time.Hour, HeartbeatExtension: 30 * time.Second}, agentrunner.SystemProbe{Manifest: manifest}, authority, replay, driver, workspaceCatalog, artifactBroker)
	if err != nil {
		return err
	}
	// A daemon restart never assumes a persisted lease is still current. Reap
	// every managed Sandbox and orphan staging before accepting fresh inventory
	// from the PostgreSQL-authorized control plane.
	reconcileCtx, reconcileCancel := context.WithTimeout(context.Background(), 30*time.Second)
	_, err = service.Reconcile(reconcileCtx, map[string]agentrunner.SandboxDescriptor{})
	reconcileCancel()
	if err != nil {
		return err
	}
	clientIdentity := strings.TrimSpace(os.Getenv("NEO_RUNNER_CLIENT_IDENTITY"))
	if clientIdentity != controlCallerIdentity {
		return errors.New("neo-runnerd client identity is invalid")
	}
	policies := []agentrunner.CallerPolicy{{Identity: clientIdentity,
		Methods: []string{agentrunner.MethodProbe, agentrunner.MethodList, agentrunner.MethodReconcile}}}
	canaryIdentity := strings.TrimSpace(os.Getenv("NEO_RUNNER_ROOT_CANARY_CLIENT_IDENTITY"))
	if canaryIdentity != "" {
		if canaryIdentity != rootCanaryCallerIdentity || canaryIdentity == clientIdentity {
			return errors.New("neo-runnerd canary client identity is invalid")
		}
		policies = append(policies, agentrunner.CallerPolicy{Identity: canaryIdentity,
			Methods: []string{agentrunner.MethodProbe, agentrunner.MethodList, agentrunner.MethodReconcile,
				agentrunner.MethodLaunch, agentrunner.MethodHeartbeat, agentrunner.MethodCancel}})
	}
	brokerIdentity := strings.TrimSpace(os.Getenv("NEO_RUNNER_BROKER_CANARY_CLIENT_IDENTITY"))
	if brokerIdentity != "" {
		if brokerIdentity != brokerCanaryCallerIdentity || brokerIdentity == clientIdentity ||
			brokerIdentity == canaryIdentity {
			return errors.New("neo-runnerd Broker canary client identity is invalid")
		}
		relayIdentity := strings.TrimSpace(os.Getenv("NEO_RUNNER_BROKER_RELAY_CLIENT_IDENTITY"))
		if relayIdentity != brokerRelayIdentity {
			return errors.New("neo-runnerd Broker relay mTLS configuration is invalid")
		}
		relay, err := agentbrokerrelay.NewClient(strings.TrimSpace(os.Getenv("NEO_RUNNER_BROKER_RELAY_URL")),
			agentrunner.ClientTLSFiles{
				CertificateFile: cleanAbsoluteEnv("NEO_RUNNER_BROKER_RELAY_CLIENT_CERT_FILE"),
				KeyFile:         cleanAbsoluteEnv("NEO_RUNNER_BROKER_RELAY_CLIENT_KEY_FILE"),
				ServerCAFile:    cleanAbsoluteEnv("NEO_RUNNER_BROKER_RELAY_SERVER_CA_FILE"),
				ServerName:      strings.TrimSpace(os.Getenv("NEO_RUNNER_BROKER_RELAY_SERVER_NAME")),
				ClientIdentity:  relayIdentity,
			}, 10*time.Second)
		if err != nil {
			return errors.New("neo-runnerd Broker relay mTLS configuration is invalid")
		}
		if err := service.WithBrokerRelayForCaller(brokerIdentity, relay); err != nil {
			return errors.New("neo-runnerd Broker relay routing configuration is invalid")
		}
		policies = append(policies, agentrunner.CallerPolicy{Identity: brokerIdentity,
			Methods: []string{agentrunner.MethodProbe, agentrunner.MethodList, agentrunner.MethodReconcile,
				agentrunner.MethodLaunch, agentrunner.MethodHeartbeat, agentrunner.MethodCancel,
				agentrunner.MethodPrepare, agentrunner.MethodCommit}})
	} else if brokerRelayConfigured() {
		return errors.New("neo-runnerd Broker relay requires the dedicated caller identity")
	}
	projectIdentity := strings.TrimSpace(os.Getenv("NEO_RUNNER_PROJECT_CANARY_CLIENT_IDENTITY"))
	if projectIdentity != "" {
		if projectIdentity != projectCanaryCallerIdentity || brokerIdentity == "" ||
			projectIdentity == clientIdentity || projectIdentity == canaryIdentity || projectIdentity == brokerIdentity {
			return errors.New("neo-runnerd Project canary client identity is invalid")
		}
		relayIdentity := strings.TrimSpace(os.Getenv("NEO_RUNNER_PROJECT_RELAY_CLIENT_IDENTITY"))
		if relayIdentity != projectRelayIdentity || relayIdentity == brokerRelayIdentity {
			return errors.New("neo-runnerd Project relay mTLS configuration is invalid")
		}
		projectRelayURL := strings.TrimSpace(os.Getenv("NEO_RUNNER_PROJECT_RELAY_URL"))
		if projectRelayURL == strings.TrimSpace(os.Getenv("NEO_RUNNER_BROKER_RELAY_URL")) {
			return errors.New("neo-runnerd Project relay endpoint is not isolated")
		}
		relay, err := agentbrokerrelay.NewClient(projectRelayURL, agentrunner.ClientTLSFiles{
			CertificateFile: cleanAbsoluteEnv("NEO_RUNNER_PROJECT_RELAY_CLIENT_CERT_FILE"),
			KeyFile:         cleanAbsoluteEnv("NEO_RUNNER_PROJECT_RELAY_CLIENT_KEY_FILE"),
			ServerCAFile:    cleanAbsoluteEnv("NEO_RUNNER_PROJECT_RELAY_SERVER_CA_FILE"),
			ServerName:      strings.TrimSpace(os.Getenv("NEO_RUNNER_PROJECT_RELAY_SERVER_NAME")),
			ClientIdentity:  relayIdentity,
		}, 10*time.Second)
		if err != nil {
			return errors.New("neo-runnerd Project relay mTLS configuration is invalid")
		}
		if err := service.WithBrokerRelayForCaller(projectIdentity, relay); err != nil {
			return errors.New("neo-runnerd Project relay routing configuration is invalid")
		}
		policies = append(policies, agentrunner.CallerPolicy{Identity: projectIdentity,
			Methods: []string{agentrunner.MethodProbe, agentrunner.MethodList, agentrunner.MethodReconcile,
				agentrunner.MethodLaunch, agentrunner.MethodHeartbeat, agentrunner.MethodCancel,
				agentrunner.MethodPrepare, agentrunner.MethodCommit}})
	} else if projectRelayConfigured() {
		return errors.New("neo-runnerd Project relay requires the dedicated caller identity")
	}
	childIdentity := strings.TrimSpace(os.Getenv("NEO_RUNNER_CHILD_CANARY_CLIENT_IDENTITY"))
	if childIdentity != "" {
		if childIdentity != childCanaryCallerIdentity || projectIdentity == "" ||
			childIdentity == clientIdentity || childIdentity == canaryIdentity ||
			childIdentity == brokerIdentity || childIdentity == projectIdentity {
			return errors.New("neo-runnerd Child canary client identity is invalid")
		}
		policies = append(policies, agentrunner.CallerPolicy{Identity: childIdentity,
			Methods: []string{agentrunner.MethodProbe, agentrunner.MethodList, agentrunner.MethodReconcile,
				agentrunner.MethodLaunch, agentrunner.MethodHeartbeat, agentrunner.MethodCancel}})
	}
	draftIdentity := strings.TrimSpace(os.Getenv("NEO_RUNNER_DRAFT_LEARNING_CLIENT_IDENTITY"))
	if draftIdentity != "" {
		if draftIdentity != draftLearningCallerIdentity || childIdentity == "" ||
			draftIdentity == clientIdentity || draftIdentity == canaryIdentity ||
			draftIdentity == brokerIdentity || draftIdentity == projectIdentity ||
			draftIdentity == childIdentity {
			return errors.New("neo-runnerd Draft learning client identity is invalid")
		}
		policies = append(policies, agentrunner.CallerPolicy{Identity: draftIdentity,
			Methods: []string{agentrunner.MethodProbe, agentrunner.MethodList, agentrunner.MethodReconcile,
				agentrunner.MethodLaunch, agentrunner.MethodResult, agentrunner.MethodCancel}})
	}
	productIdentity := strings.TrimSpace(os.Getenv("NEO_RUNNER_PRODUCT_CANARY_CLIENT_IDENTITY"))
	if productIdentity != "" {
		if productIdentity != productCanaryCallerIdentity || draftIdentity == "" ||
			productIdentity == clientIdentity || productIdentity == canaryIdentity ||
			productIdentity == brokerIdentity || productIdentity == projectIdentity ||
			productIdentity == childIdentity || productIdentity == draftIdentity {
			return errors.New("neo-runnerd product canary client identity is invalid")
		}
		policies = append(policies, agentrunner.CallerPolicy{Identity: productIdentity,
			Methods: []string{agentrunner.MethodProbe, agentrunner.MethodList, agentrunner.MethodReconcile,
				agentrunner.MethodLaunch, agentrunner.MethodHeartbeat, agentrunner.MethodCancel}})
	}
	handler, err := agentrunner.NewHTTPHandlerWithPolicies(service, 15*time.Second, policies)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: handler, TLSConfig: tlsConfig, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(tls.NewListener(listener, tlsConfig)) }()
	select {
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func brokerRelayConfigured() bool {
	for _, name := range []string{"NEO_RUNNER_BROKER_RELAY_URL", "NEO_RUNNER_BROKER_RELAY_CLIENT_CERT_FILE",
		"NEO_RUNNER_BROKER_RELAY_CLIENT_KEY_FILE", "NEO_RUNNER_BROKER_RELAY_SERVER_CA_FILE",
		"NEO_RUNNER_BROKER_RELAY_SERVER_NAME", "NEO_RUNNER_BROKER_RELAY_CLIENT_IDENTITY"} {
		if strings.TrimSpace(os.Getenv(name)) != "" {
			return true
		}
	}
	return false
}

func projectRelayConfigured() bool {
	for _, name := range []string{"NEO_RUNNER_PROJECT_RELAY_URL", "NEO_RUNNER_PROJECT_RELAY_CLIENT_CERT_FILE",
		"NEO_RUNNER_PROJECT_RELAY_CLIENT_KEY_FILE", "NEO_RUNNER_PROJECT_RELAY_SERVER_CA_FILE",
		"NEO_RUNNER_PROJECT_RELAY_SERVER_NAME", "NEO_RUNNER_PROJECT_RELAY_CLIENT_IDENTITY"} {
		if strings.TrimSpace(os.Getenv(name)) != "" {
			return true
		}
	}
	return false
}

func cleanAbsoluteEnv(name string) string {
	value := filepath.Clean(strings.TrimSpace(os.Getenv(name)))
	if !filepath.IsAbs(value) {
		return ""
	}
	return value
}
func privateAddress(value string) bool {
	host, port, err := net.SplitHostPort(value)
	if err != nil || port == "" {
		return false
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && !ip.IsUnspecified() && (ip.IsLoopback() || ip.IsPrivate())
}
func binaryPath(manifest agentrunner.ReleaseManifest, name string) string {
	for _, binary := range manifest.Binaries {
		if binary.Name == name {
			return binary.Path
		}
	}
	return ""
}
