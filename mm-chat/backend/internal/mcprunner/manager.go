package mcprunner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	protocol "github.com/modelcontextprotocol/go-sdk/mcp"

	"neo-chat/mm-chat/backend/internal/mcpclient"
)

var (
	ErrServerNotApproved = errors.New("mcp runner server is not approved")
	ErrCapacity          = errors.New("mcp runner process capacity exhausted")
	ErrUnavailable       = errors.New("mcp runner server is unavailable")
)

const runnerConnectTimeout = 2 * time.Minute

type Config struct {
	MaxProcesses  int
	WorkRoot      string
	ReapInterval  time.Duration
	ClientName    string
	ClientVersion string
}

type Manager struct {
	mu       sync.Mutex
	config   Config
	servers  map[string]mcpclient.Server
	sessions map[string]*managedSession
	closed   bool
}

type managedSession struct {
	serverID               string
	workDir                string
	command                *exec.Cmd
	session                *protocol.ClientSession
	started                time.Time
	lastUsed               time.Time
	active                 int
	closing                bool
	changed                bool
	environmentFingerprint string
	idleTimeout            time.Duration
	maxLifetime            time.Duration
}

func NewManager(config Config, servers []mcpclient.Server) (*Manager, error) {
	if config.MaxProcesses <= 0 {
		config.MaxProcesses = 4
	}
	if config.MaxProcesses > 4 {
		return nil, ErrCapacity
	}
	if config.ReapInterval <= 0 {
		config.ReapInterval = time.Minute
	}
	if config.WorkRoot == "" {
		config.WorkRoot = "/work"
	}
	root, err := filepath.Abs(config.WorkRoot)
	if err != nil {
		return nil, ErrUnavailable
	}
	config.WorkRoot = root
	manager := &Manager{
		config: config, servers: map[string]mcpclient.Server{},
		sessions: map[string]*managedSession{},
	}
	for _, server := range servers {
		if server.Ref.Source != mcpclient.SourceManifest ||
			server.Transport != mcpclient.TransportStdio || server.Command == nil ||
			len(server.Command.Argv) == 0 {
			continue
		}
		if _, duplicate := manager.servers[server.Ref.ID]; duplicate {
			return nil, ErrServerNotApproved
		}
		manager.servers[server.Ref.ID] = server
	}
	return manager, nil
}

func (m *Manager) RunReaper(ctx context.Context) {
	ticker := time.NewTicker(m.config.ReapInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = m.Close()
			return
		case now := <-ticker.C:
			m.reap(now.UTC())
		}
	}
}

func (m *Manager) ListTools(ctx context.Context, serverID string, options ...instanceOptions) ([]*protocol.Tool, error) {
	instanceID, environment, artifact := runnerInstanceOptions(serverID, options)
	managed, release, err := m.acquire(ctx, serverID, instanceID, environment, artifact)
	if err != nil {
		return nil, err
	}
	defer release()
	tools := []*protocol.Tool{}
	for tool, err := range managed.session.Tools(ctx, nil) {
		if err != nil {
			m.invalidate(managed)
			return nil, ErrUnavailable
		}
		tools = append(tools, tool)
	}
	sort.SliceStable(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools, nil
}

func (m *Manager) CallTool(
	ctx context.Context,
	serverID string,
	name string,
	arguments map[string]any,
	options ...instanceOptions,
) (*protocol.CallToolResult, error) {
	instanceID, environment, artifact := runnerInstanceOptions(serverID, options)
	managed, release, err := m.acquire(ctx, serverID, instanceID, environment, artifact)
	if err != nil {
		return nil, err
	}
	defer release()
	result, err := managed.session.CallTool(ctx, &protocol.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		m.invalidate(managed)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrUnavailable
	}
	return result, nil
}

type instanceOptions struct {
	InstanceID  string
	Environment map[string]string
	Artifact    mcpclient.DynamicRunnerArtifact
}

func runnerInstanceOptions(serverID string, options []instanceOptions) (string, map[string]string, mcpclient.DynamicRunnerArtifact) {
	if len(options) == 1 && options[0].InstanceID != "" {
		return options[0].InstanceID, options[0].Environment, options[0].Artifact
	}
	return serverID, map[string]string{}, mcpclient.DynamicRunnerArtifact{}
}

func (m *Manager) Health() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.config.MaxProcesses <= 0 || len(m.sessions) > m.config.MaxProcesses {
		return ErrUnavailable
	}
	return nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	sessions := make([]*managedSession, 0, len(m.sessions))
	for id, session := range m.sessions {
		session.closing = true
		sessions = append(sessions, session)
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	var closeErrors []error
	for _, session := range sessions {
		if err := closeManagedSession(session); err != nil {
			closeErrors = append(closeErrors, err)
		}
	}
	return errors.Join(closeErrors...)
}

func (m *Manager) acquire(
	ctx context.Context,
	serverID string,
	instanceID string,
	environment map[string]string,
	artifact mcpclient.DynamicRunnerArtifact,
) (*managedSession, func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, nil, ErrUnavailable
	}
	server, approved := m.servers[serverID]
	if !approved {
		var err error
		server, err = dynamicServer(serverID, artifact)
		if err != nil {
			return nil, nil, err
		}
	}
	if err := validateInstanceEnvironment(server, environment); err != nil {
		return nil, nil, err
	}
	key := instanceID
	now := time.Now().UTC()
	if managed := m.sessions[key]; managed != nil && !managed.closing {
		if now.Sub(managed.started) < server.Command.MaxLifetime &&
			managed.environmentFingerprint == environmentFingerprint(environment) {
			managed.active++
			managed.lastUsed = now
			return managed, m.releaseFunc(managed), nil
		}
		managed.closing = true
		delete(m.sessions, key)
		// Finish removing the old instance before recreating its workspace. An
		// asynchronous close can delete the replacement's npm cache/workdir.
		_ = closeManagedSession(managed)
	}
	if len(m.sessions) >= m.config.MaxProcesses {
		return nil, nil, ErrCapacity
	}
	managed, err := m.startLocked(ctx, server, key, environment)
	if err != nil {
		return nil, nil, err
	}
	managed.active = 1
	m.sessions[key] = managed
	return managed, m.releaseFunc(managed), nil
}

func dynamicServer(serverID string, artifact mcpclient.DynamicRunnerArtifact) (mcpclient.Server, error) {
	if artifact.ID != serverID || !validDynamicPackageSpec(artifact.PackageSpec) ||
		len(artifact.Args) > 64 || len(artifact.SecretEnv) > 8 ||
		artifact.IdleSeconds < 60 || artifact.IdleSeconds > 3600 ||
		artifact.LifetimeSeconds < artifact.IdleSeconds || artifact.LifetimeSeconds > 86400 {
		return mcpclient.Server{}, ErrServerNotApproved
	}
	for _, value := range artifact.Args {
		if value == "" || len(value) > 4096 || strings.ContainsAny(value, "\x00\r\n") {
			return mcpclient.Server{}, ErrServerNotApproved
		}
	}
	for _, name := range artifact.SecretEnv {
		if !validEnvironmentName(name) {
			return mcpclient.Server{}, ErrServerNotApproved
		}
	}
	return mcpclient.Server{
		Ref:       mcpclient.ServerRef{Source: mcpclient.SourceManifest, ID: serverID},
		Transport: mcpclient.TransportStdio,
		Command: &mcpclient.Command{
			Argv: append([]string{"/usr/local/bin/npx", "--yes", artifact.PackageSpec}, artifact.Args...),
			Env:  map[string]string{}, UserSecretEnv: append([]string(nil), artifact.SecretEnv...),
			IdleTimeout: time.Duration(artifact.IdleSeconds) * time.Second,
			MaxLifetime: time.Duration(artifact.LifetimeSeconds) * time.Second,
		},
	}, nil
}

func validDynamicPackageSpec(value string) bool {
	if value == "" || len(value) > 512 || strings.ContainsAny(value, "\\:#?%\x00\r\n\t") {
		return false
	}
	lastAt := strings.LastIndexByte(value, '@')
	if lastAt <= 0 || lastAt == len(value)-1 {
		return false
	}
	name, version := value[:lastAt], value[lastAt+1:]
	if strings.HasPrefix(value, "@") {
		lastAt = strings.LastIndexByte(value[1:], '@') + 1
		if lastAt <= strings.IndexByte(value, '/') || lastAt == len(value)-1 {
			return false
		}
		name, version = value[:lastAt], value[lastAt+1:]
	}
	if strings.Contains(name, "..") || strings.Count(name, "/") > 1 ||
		(strings.HasPrefix(name, "@") && !strings.Contains(name, "/")) {
		return false
	}
	if version == "latest" || version == "next" || version == "beta" || version == "dev" {
		return false
	}
	for _, char := range strings.ToLower(name + version) {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || strings.ContainsRune("@/._+-", char) {
			continue
		}
		return false
	}
	return true
}

func validEnvironmentName(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for index, char := range value {
		if char == '_' || (char >= 'A' && char <= 'Z') || (index > 0 && char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	return true
}

func (m *Manager) startLocked(ctx context.Context, server mcpclient.Server, instanceID string, environment map[string]string) (*managedSession, error) {
	workDir := filepath.Join(m.config.WorkRoot, instanceID)
	if err := os.RemoveAll(workDir); err != nil {
		return nil, ErrUnavailable
	}
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return nil, ErrUnavailable
	}
	command := exec.Command(server.Command.Argv[0], server.Command.Argv[1:]...)
	command.Dir = workDir
	cacheDir := filepath.Join(workDir, ".npm")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		_ = os.RemoveAll(workDir)
		return nil, ErrUnavailable
	}
	command.Env = []string{
		"HOME=" + workDir, "TMPDIR=" + workDir, "PATH=/usr/local/bin:/usr/bin:/bin",
		"PLAYWRIGHT_BROWSERS_PATH=/ms-playwright",
		"npm_config_cache=" + cacheDir, "npm_config_update_notifier=false",
		"npm_config_audit=false", "npm_config_fund=false",
	}
	keys := make([]string, 0, len(server.Command.Env))
	for key := range server.Command.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		command.Env = append(command.Env, key+"="+server.Command.Env[key])
	}
	secretKeys := make([]string, 0, len(environment))
	for key := range environment {
		secretKeys = append(secretKeys, key)
	}
	sort.Strings(secretKeys)
	for _, key := range secretKeys {
		command.Env = append(command.Env, key+"="+environment[key])
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	client := protocol.NewClient(
		&protocol.Implementation{Name: fallback(m.config.ClientName, "neo-chat-mcp-runner"), Version: fallback(m.config.ClientVersion, "dev")},
		&protocol.ClientOptions{Capabilities: &protocol.ClientCapabilities{}},
	)
	// Cold npx artifacts install into the isolated per-Server cache during
	// Connect. Bound that path independently from the shorter steady-state call
	// timeout so a legitimate first install is not killed halfway through.
	connectCtx, cancel := context.WithTimeout(ctx, runnerConnectTimeout)
	defer cancel()
	session, err := client.Connect(connectCtx, &protocol.CommandTransport{
		Command: command, TerminateDuration: 3 * time.Second,
	}, nil)
	if err != nil {
		_ = terminateProcessGroup(command)
		_ = os.RemoveAll(workDir)
		return nil, fmt.Errorf("start approved mcp server: %w", ErrUnavailable)
	}
	now := time.Now().UTC()
	return &managedSession{
		serverID: instanceID, workDir: workDir, command: command,
		session: session, started: now, lastUsed: now,
		environmentFingerprint: environmentFingerprint(environment),
		idleTimeout:            server.Command.IdleTimeout, maxLifetime: server.Command.MaxLifetime,
	}, nil
}

func environmentFingerprint(environment map[string]string) string {
	encoded, err := json.Marshal(environment)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func validateInstanceEnvironment(server mcpclient.Server, environment map[string]string) error {
	if server.Command == nil || len(environment) != len(server.Command.UserSecretEnv) {
		if len(environment) == 0 && server.Command != nil && len(server.Command.UserSecretEnv) == 0 {
			return nil
		}
		return ErrServerNotApproved
	}
	for _, name := range server.Command.UserSecretEnv {
		value, ok := environment[name]
		if !ok || value == "" || len(value) > 16<<10 || strings.ContainsAny(value, "\x00\r\n") {
			return ErrServerNotApproved
		}
	}
	return nil
}

func (m *Manager) releaseFunc(managed *managedSession) func() {
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if managed.active > 0 {
			managed.active--
		}
		managed.lastUsed = time.Now().UTC()
	}
}

// invalidate evicts a failed session before killing its entire process group.
// A subsequent request may start a clean approved process, while descendants
// of a crashed or canceled server cannot outlive the failed control request.
func (m *Manager) invalidate(managed *managedSession) {
	if managed == nil {
		return
	}
	m.mu.Lock()
	if current := m.sessions[managed.serverID]; current != managed {
		m.mu.Unlock()
		return
	}
	managed.closing = true
	delete(m.sessions, managed.serverID)
	m.mu.Unlock()
	_ = forceCloseManagedSession(managed)
}

func (m *Manager) reap(now time.Time) {
	m.mu.Lock()
	closing := []*managedSession{}
	for id, session := range m.sessions {
		idleExpired := session.active == 0 && now.Sub(session.lastUsed) >= session.idleTimeout
		lifetimeExpired := now.Sub(session.started) >= session.maxLifetime
		if !idleExpired && !lifetimeExpired {
			continue
		}
		session.closing = true
		delete(m.sessions, id)
		closing = append(closing, session)
	}
	m.mu.Unlock()
	for _, session := range closing {
		_ = closeManagedSession(session)
	}
}

func closeManagedSession(session *managedSession) error {
	if session == nil {
		return nil
	}
	var closeErr error
	if session.session != nil {
		closeErr = session.session.Close()
	}
	_ = terminateProcessGroup(session.command)
	_ = os.RemoveAll(session.workDir)
	return closeErr
}

func forceCloseManagedSession(session *managedSession) error {
	if session == nil {
		return nil
	}
	killErr := terminateProcessGroup(session.command)
	var closeErr error
	if session.session != nil {
		closeErr = session.session.Close()
	}
	_ = os.RemoveAll(session.workDir)
	return errors.Join(killErr, closeErr)
}

func terminateProcessGroup(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return nil
	}
	if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func fallback(value, fallbackValue string) string {
	if value == "" {
		return fallbackValue
	}
	return value
}
