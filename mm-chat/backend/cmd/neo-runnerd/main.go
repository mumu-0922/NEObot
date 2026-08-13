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
	"strings"
	"syscall"
	"time"

	"neo-chat/mm-chat/backend/internal/agentrunner"
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
	if stateRoot == "" || !loopbackAddress(listenAddress) {
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
	handler, err := agentrunner.NewHTTPHandler(service, 15*time.Second,
		strings.TrimSpace(os.Getenv("NEO_RUNNER_CLIENT_IDENTITY")))
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

func cleanAbsoluteEnv(name string) string {
	value := filepath.Clean(strings.TrimSpace(os.Getenv(name)))
	if !filepath.IsAbs(value) {
		return ""
	}
	return value
}
func loopbackAddress(value string) bool {
	host, port, err := net.SplitHostPort(value)
	if err != nil || port == "" {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
func binaryPath(manifest agentrunner.ReleaseManifest, name string) string {
	for _, binary := range manifest.Binaries {
		if binary.Name == name {
			return binary.Path
		}
	}
	return ""
}
