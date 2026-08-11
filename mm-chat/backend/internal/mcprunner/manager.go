package mcprunner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	serverID string
	workDir  string
	command  *exec.Cmd
	session  *protocol.ClientSession
	started  time.Time
	lastUsed time.Time
	active   int
	closing  bool
	changed  bool
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

func (m *Manager) ListTools(ctx context.Context, serverID string) ([]*protocol.Tool, error) {
	managed, release, err := m.acquire(ctx, serverID)
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
) (*protocol.CallToolResult, error) {
	managed, release, err := m.acquire(ctx, serverID)
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
) (*managedSession, func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, nil, ErrUnavailable
	}
	server, approved := m.servers[serverID]
	if !approved {
		return nil, nil, ErrServerNotApproved
	}
	now := time.Now().UTC()
	if managed := m.sessions[serverID]; managed != nil && !managed.closing {
		if now.Sub(managed.started) < server.Command.MaxLifetime {
			managed.active++
			managed.lastUsed = now
			return managed, m.releaseFunc(managed), nil
		}
		managed.closing = true
		delete(m.sessions, serverID)
		go func() { _ = closeManagedSession(managed) }()
	}
	if len(m.sessions) >= m.config.MaxProcesses {
		return nil, nil, ErrCapacity
	}
	managed, err := m.startLocked(ctx, server)
	if err != nil {
		return nil, nil, err
	}
	managed.active = 1
	m.sessions[serverID] = managed
	return managed, m.releaseFunc(managed), nil
}

func (m *Manager) startLocked(ctx context.Context, server mcpclient.Server) (*managedSession, error) {
	workDir := filepath.Join(m.config.WorkRoot, server.Ref.ID)
	if err := os.RemoveAll(workDir); err != nil {
		return nil, ErrUnavailable
	}
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return nil, ErrUnavailable
	}
	command := exec.Command(server.Command.Argv[0], server.Command.Argv[1:]...)
	command.Dir = workDir
	command.Env = []string{"HOME=" + workDir, "TMPDIR=" + workDir, "PATH=/usr/local/bin:/usr/bin:/bin"}
	keys := make([]string, 0, len(server.Command.Env))
	for key := range server.Command.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		command.Env = append(command.Env, key+"="+server.Command.Env[key])
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	client := protocol.NewClient(
		&protocol.Implementation{Name: fallback(m.config.ClientName, "neo-chat-mcp-runner"), Version: fallback(m.config.ClientVersion, "dev")},
		&protocol.ClientOptions{Capabilities: &protocol.ClientCapabilities{}},
	)
	connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
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
		serverID: server.Ref.ID, workDir: workDir, command: command,
		session: session, started: now, lastUsed: now,
	}, nil
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
		server := m.servers[id]
		idleExpired := session.active == 0 && now.Sub(session.lastUsed) >= server.Command.IdleTimeout
		lifetimeExpired := now.Sub(session.started) >= server.Command.MaxLifetime
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
