// Package localskills executes installed Agent Skill commands directly as the
// current Backend user. It deliberately provides guardrails, not isolation.
package localskills

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	ApprovalSmart = "smart"
	ApprovalOff   = "off"

	defaultPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
)

var (
	ErrInvalidConfig    = errors.New("local Skill runtime configuration is invalid")
	ErrInvalidCommand   = errors.New("local Skill command is invalid")
	ErrCommandBlocked   = errors.New("local Skill command is blocked")
	ErrApprovalRequired = errors.New("local Skill command requires approval")
	ErrRuntimeBusy      = errors.New("local Skill runtime is busy")
	ErrRuntimeFailed    = errors.New("local Skill runtime failed")
)

type Config struct {
	Enabled       bool
	RuntimeRoot   string
	WorkspaceRoot string
	ShellPath     string
	ApprovalMode  string
	CallTimeout   time.Duration
	RunTimeout    time.Duration
	MaxOutput     int64
	MaxCalls      int
	MaxRounds     int
	MaxConcurrent int
}

type Request struct {
	Command         string
	WorkingDir      string
	TimeoutSeconds  int
	SkillsRoot      string
	ActiveSkillRoot string
}

type Result struct {
	ExitCode       int    `json:"exitCode"`
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
	TimedOut       bool   `json:"timedOut"`
	Truncated      bool   `json:"truncated"`
	DurationMillis int64  `json:"durationMillis"`
}

type Executor struct {
	config           Config
	slots            chan struct{}
	workspaceWriteMu sync.Mutex
	lifecycleCtx     context.Context
	lifecycleCancel  context.CancelFunc
	closeOnce        sync.Once
	jobMu            sync.Mutex
	jobs             map[string]*backgroundJob
	jobNotices       map[string][]JobNotice
	closed           bool
}

func NewExecutor(config Config) (*Executor, error) {
	config.RuntimeRoot = filepath.Clean(strings.TrimSpace(config.RuntimeRoot))
	config.WorkspaceRoot = filepath.Clean(strings.TrimSpace(config.WorkspaceRoot))
	config.ShellPath = filepath.Clean(strings.TrimSpace(config.ShellPath))
	config.ApprovalMode = strings.TrimSpace(config.ApprovalMode)
	if config.Enabled && (!secureRoot(config.RuntimeRoot) || !secureRoot(config.WorkspaceRoot) ||
		!filepath.IsAbs(config.ShellPath) ||
		(config.ApprovalMode != ApprovalSmart && config.ApprovalMode != ApprovalOff) ||
		config.CallTimeout < time.Second || config.CallTimeout > 10*time.Minute ||
		config.RunTimeout < config.CallTimeout || config.RunTimeout > 30*time.Minute ||
		config.MaxOutput < 1024 || config.MaxOutput > 8<<20 ||
		config.MaxCalls < 1 || config.MaxCalls > 128 || config.MaxRounds < 1 || config.MaxRounds > 32 ||
		config.MaxConcurrent < 1 || config.MaxConcurrent > 32) {
		return nil, ErrInvalidConfig
	}
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	concurrency := config.MaxConcurrent
	if !config.Enabled {
		concurrency = 1
	}
	return &Executor{
		config: config, slots: make(chan struct{}, concurrency),
		lifecycleCtx: lifecycleCtx, lifecycleCancel: lifecycleCancel,
		jobs: make(map[string]*backgroundJob), jobNotices: make(map[string][]JobNotice),
	}, nil
}

func (executor *Executor) Config() Config {
	if executor == nil {
		return Config{}
	}
	return executor.config
}

func (executor *Executor) Enabled() bool {
	return executor != nil && executor.config.Enabled
}

func (executor *Executor) Execute(ctx context.Context, request Request) (Result, error) {
	if !executor.Enabled() {
		return Result{}, ErrRuntimeFailed
	}
	workingDir, timeout, err := executor.prepareRequest(&request, executor.config.CallTimeout)
	if err != nil {
		return Result{}, err
	}
	if !executor.acquireSlot() {
		return Result{}, ErrRuntimeBusy
	}
	defer executor.releaseSlot()
	return executor.executeReserved(ctx, request, workingDir, timeout)
}

func (executor *Executor) prepareRequest(
	request *Request,
	maximumTimeout time.Duration,
) (string, time.Duration, error) {
	if request == nil {
		return "", 0, ErrInvalidCommand
	}
	request.Command = strings.TrimSpace(request.Command)
	if !validCommand(request.Command) {
		return "", 0, ErrInvalidCommand
	}
	if hardBlockedCommand(request.Command) {
		return "", 0, ErrCommandBlocked
	}
	if executor.config.ApprovalMode == ApprovalSmart && destructiveCommand(request.Command) {
		return "", 0, ErrApprovalRequired
	}
	workingDir, err := executor.resolveWorkingDirectory(request.WorkingDir)
	if err != nil {
		return "", 0, err
	}
	timeout := maximumTimeout
	if request.TimeoutSeconds > 0 {
		requested := time.Duration(request.TimeoutSeconds) * time.Second
		if requested < time.Second || requested > maximumTimeout {
			return "", 0, ErrInvalidCommand
		}
		timeout = requested
	}
	return workingDir, timeout, nil
}

func (executor *Executor) acquireSlot() bool {
	select {
	case executor.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (executor *Executor) releaseSlot() {
	<-executor.slots
}

func (executor *Executor) executeReserved(
	ctx context.Context,
	request Request,
	workingDir string,
	timeout time.Duration,
) (Result, error) {
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	started := time.Now()
	// Do not start a login shell. HOME is the writable workspace, so -l would
	// implicitly execute a Skill-created .profile/.bash_profile before the
	// validated command on every later Tool call.
	command := exec.Command(executor.config.ShellPath, "-c", request.Command)
	command.Dir = workingDir
	command.Env = explicitEnvironment(
		executor.config.WorkspaceRoot,
		request.SkillsRoot,
		request.ActiveSkillRoot,
	)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	capture := newBoundedCapture(executor.config.MaxOutput)
	command.Stdout = capture.writer(true)
	command.Stderr = capture.writer(false)
	// Close may cancel a queued background Job after it has reserved a slot but
	// before its goroutine reaches process creation. Never start a process once
	// the owning lifecycle or request context is already canceled.
	if err := commandCtx.Err(); err != nil {
		return Result{}, err
	}
	if err := command.Start(); err != nil {
		return Result{}, ErrRuntimeFailed
	}
	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()
	var waitErr error
	timedOut := false
	select {
	case waitErr = <-waited:
	case <-commandCtx.Done():
		// A call timeout is a bounded Tool result. Cancellation or a deadline
		// inherited from the owning Chat Run is fatal to the whole Run and must
		// not be converted into an ordinary non-zero command result.
		timedOut = ctx.Err() == nil && errors.Is(commandCtx.Err(), context.DeadlineExceeded)
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		waitErr = <-waited
	}
	stdout, stderr, truncated := capture.result()
	stdout = redactExecutionPaths(
		stdout,
		executor.config.WorkspaceRoot,
		request.SkillsRoot,
		request.ActiveSkillRoot,
	)
	stderr = redactExecutionPaths(
		stderr,
		executor.config.WorkspaceRoot,
		request.SkillsRoot,
		request.ActiveSkillRoot,
	)
	stdout, stderr, redactionTruncated := boundOutput(
		stdout,
		stderr,
		executor.config.MaxOutput,
	)
	truncated = truncated || redactionTruncated
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	exitCode := 0
	if waitErr != nil {
		var exitError *exec.ExitError
		if errors.As(waitErr, &exitError) {
			exitCode = exitError.ExitCode()
		} else {
			return Result{}, ErrRuntimeFailed
		}
	}
	if timedOut {
		exitCode = 124
	}
	return Result{
		ExitCode: exitCode, Stdout: stdout, Stderr: stderr, TimedOut: timedOut,
		Truncated: truncated, DurationMillis: max(time.Since(started).Milliseconds(), 0),
	}, nil
}

func (executor *Executor) resolveWorkingDirectory(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = executor.config.WorkspaceRoot
	} else if !filepath.IsAbs(value) {
		value = filepath.Join(executor.config.WorkspaceRoot, value)
	}
	value = filepath.Clean(value)
	if strings.ContainsRune(value, '\x00') {
		return "", ErrInvalidCommand
	}
	info, err := os.Stat(value)
	if err != nil || !info.IsDir() {
		return "", ErrInvalidCommand
	}
	resolvedRoot, err := filepath.EvalSymlinks(executor.config.WorkspaceRoot)
	if err != nil {
		return "", ErrRuntimeFailed
	}
	resolvedValue, err := filepath.EvalSymlinks(value)
	if err != nil || !pathWithin(resolvedRoot, resolvedValue) {
		return "", ErrInvalidCommand
	}
	return resolvedValue, nil
}

func pathWithin(root, target string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	return err == nil && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func explicitEnvironment(workspace, skillsRoot, activeSkillRoot string) []string {
	environment := []string{
		"HOME=" + workspace,
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"NEO_CHAT_LOCAL_DIRECT=1",
		"NEO_CHAT_WORKSPACE=" + workspace,
		"PATH=" + defaultPath,
	}
	if secureRoot(filepath.Clean(strings.TrimSpace(skillsRoot))) {
		skillsRoot = filepath.Clean(skillsRoot)
		activeSkillRoot = filepath.Clean(strings.TrimSpace(activeSkillRoot))
		if secureRoot(activeSkillRoot) && pathWithin(skillsRoot, activeSkillRoot) {
			environment = append(environment, "NEO_CHAT_ACTIVE_SKILL_ROOT="+activeSkillRoot)
		}
	}
	return environment
}

func redactExecutionPaths(value, workspace, skillsRoot, activeSkillRoot string) string {
	replacements := []struct {
		path  string
		label string
	}{
		{path: activeSkillRoot, label: "$NEO_CHAT_ACTIVE_SKILL_ROOT"},
		{path: skillsRoot, label: "<skill-cache>"},
		{path: workspace, label: "$NEO_CHAT_WORKSPACE"},
	}
	for _, replacement := range replacements {
		path := filepath.Clean(strings.TrimSpace(replacement.path))
		if secureRoot(path) {
			value = strings.ReplaceAll(value, path, replacement.label)
		}
	}
	return value
}

func boundOutput(stdout, stderr string, limit int64) (string, string, bool) {
	capture := newBoundedCapture(limit)
	_, _ = capture.writer(true).Write([]byte(stdout))
	_, _ = capture.writer(false).Write([]byte(stderr))
	return capture.result()
}

func secureRoot(value string) bool {
	return filepath.IsAbs(value) && filepath.Clean(value) == value &&
		value != string(filepath.Separator) && !strings.ContainsRune(value, '\x00')
}

func validCommand(value string) bool {
	if value == "" || len(value) > 64<<10 || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) && character != '\n' && character != '\r' && character != '\t' {
			return false
		}
	}
	return true
}

var hardBlockedPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(^|[;&|]\s*)(sudo\s+)?(shutdown|reboot|poweroff|halt)(\s|$)`),
	regexp.MustCompile(`(?i)(^|[;&|]\s*)(sudo\s+)?(mkfs(?:\.[a-z0-9]+)?|wipefs)(\s|$)`),
	regexp.MustCompile(`(?i)\bdd\s+[^\n;&|]*\bof=/dev/(?:sd|hd|vd|nvme|mmcblk)`),
	regexp.MustCompile(`(?i)\brm\s+(?:[^\n;&|]*\s)?--no-preserve-root(?:\s|=)[^\n;&|]*\s/\s*(?:$|[;&|])`),
	regexp.MustCompile(`(?i)\brm\s+-[a-z]*r[a-z]*f[a-z]*\s+/\s*(?:$|[;&|])`),
	regexp.MustCompile(`(?i)(/run/secrets|/proc/(?:1|self)/environ|provider-keyring|mm_chat_provider_keyring)`),
	regexp.MustCompile(`(?i)>\s*/proc/sysrq-trigger`),
	regexp.MustCompile(`(?i)\bkill\s+-9\s+-1\b`),
}

var destructivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(^|[;&|]\s*)sudo(?:\s|$)`),
	regexp.MustCompile(`(?i)\brm\s+-[a-z]*r[a-z]*f[a-z]*(?:\s|$)`),
	regexp.MustCompile(`(?i)\bgit\s+(?:reset\s+--hard|clean\s+-[a-z]*f|push\s+[^\n;&|]*(?:--force|-f(?:\s|$)))`),
	regexp.MustCompile(`(?i)\b(?:docker|podman)\s+(?:system\s+prune|stop|kill|restart|rm)(?:\s|$)`),
	regexp.MustCompile(`(?i)\bdocker\s+compose\s+(?:down|stop|kill|restart)(?:\s|$)`),
	regexp.MustCompile(`(?i)\b(?:chmod|chown)\s+-R(?:\s|$)`),
	regexp.MustCompile(`(?i)(?:curl|wget)[^\n;&|]*\|\s*(?:ba|z|fi)?sh(?:\s|$)`),
	regexp.MustCompile(`(?i)(^|[;&|]\s*)systemctl\s+(?:stop|disable|mask|restart)(?:\s|$)`),
}

func hardBlockedCommand(command string) bool {
	compact := strings.Join(strings.Fields(command), " ")
	if strings.Contains(compact, ":(){:|:&};:") || strings.Contains(compact, ":(){ :|:& };:") {
		return true
	}
	return matchesAny(command, hardBlockedPatterns)
}

func destructiveCommand(command string) bool { return matchesAny(command, destructivePatterns) }

func matchesAny(value string, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

type boundedCapture struct {
	mu        sync.Mutex
	remaining int64
	stdout    strings.Builder
	stderr    strings.Builder
	truncated bool
}

type captureWriter struct {
	capture *boundedCapture
	stdout  bool
}

func newBoundedCapture(limit int64) *boundedCapture {
	return &boundedCapture{remaining: limit}
}

func (capture *boundedCapture) writer(stdout bool) io.Writer {
	return captureWriter{capture: capture, stdout: stdout}
}

func (writer captureWriter) Write(value []byte) (int, error) {
	written := len(value)
	writer.capture.mu.Lock()
	defer writer.capture.mu.Unlock()
	keep := int64(len(value))
	if keep > writer.capture.remaining {
		keep = writer.capture.remaining
		writer.capture.truncated = true
	}
	if keep > 0 {
		if writer.stdout {
			_, _ = writer.capture.stdout.Write(value[:keep])
		} else {
			_, _ = writer.capture.stderr.Write(value[:keep])
		}
		writer.capture.remaining -= keep
	}
	return written, nil
}

func (capture *boundedCapture) result() (string, string, bool) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.stdout.String(), capture.stderr.String(), capture.truncated
}

func (config Config) String() string {
	return fmt.Sprintf("enabled=%t mode=local_direct approval=%s", config.Enabled, config.ApprovalMode)
}
