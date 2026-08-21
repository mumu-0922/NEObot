package agenthost

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	ErrWorkspacePathInvalid      = errors.New("workspace path is invalid")
	ErrWorkspacePathUnavailable  = errors.New("workspace path is unavailable")
	ErrWindowsInteropUnavailable = errors.New("Windows path interop is unavailable")

	windowsDrivePathPattern = regexp.MustCompile(`^[A-Za-z]:[\\/]`)
	windowsMountPattern     = regexp.MustCompile(`^/mnt/[A-Za-z](?:/|$)`)
)

type WorkspaceResolver interface {
	ResolveWorkspace(context.Context, string) (WorkspaceDescriptor, error)
	WindowsPathInterop() bool
}

type WindowsPathConverter interface {
	ToWSL(context.Context, string) (string, error)
}

type ExecWindowsPathConverter struct {
	Executable string
	Timeout    time.Duration
}

func NewExecWindowsPathConverter() *ExecWindowsPathConverter {
	executable, _ := exec.LookPath("wslpath")
	return &ExecWindowsPathConverter{Executable: executable, Timeout: 5 * time.Second}
}

func (converter *ExecWindowsPathConverter) Available() bool {
	return converter != nil && filepath.IsAbs(strings.TrimSpace(converter.Executable))
}

func (converter *ExecWindowsPathConverter) ToWSL(ctx context.Context, value string) (string, error) {
	if !converter.Available() {
		return "", ErrWindowsInteropUnavailable
	}
	timeout := converter.Timeout
	if timeout <= 0 || timeout > 10*time.Second {
		timeout = 5 * time.Second
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(commandCtx, converter.Executable, "-u", "--", value)
	command.Env = []string{"PATH=/usr/bin:/bin"}
	output, err := command.Output()
	if err != nil || commandCtx.Err() != nil || len(output) > maxWorkspacePathBytes+1 {
		return "", ErrWorkspacePathInvalid
	}
	converted := strings.TrimSuffix(string(output), "\n")
	converted = strings.TrimSuffix(converted, "\r")
	if !validWorkspacePathInput(converted) || !filepath.IsAbs(converted) {
		return "", ErrWorkspacePathInvalid
	}
	return converted, nil
}

type LocalWorkspaceResolver struct {
	RunnerID  string
	Converter WindowsPathConverter
}

func NewLocalWorkspaceResolver(runnerID string) (*LocalWorkspaceResolver, error) {
	if err := validateRunnerID(runnerID); err != nil {
		return nil, err
	}
	return &LocalWorkspaceResolver{
		RunnerID: runnerID, Converter: NewExecWindowsPathConverter(),
	}, nil
}

func (resolver *LocalWorkspaceResolver) WindowsPathInterop() bool {
	converter, ok := resolver.Converter.(*ExecWindowsPathConverter)
	return ok && converter.Available()
}

func (resolver *LocalWorkspaceResolver) ResolveWorkspace(
	ctx context.Context,
	input string,
) (WorkspaceDescriptor, error) {
	if resolver == nil || validateRunnerID(resolver.RunnerID) != nil ||
		!validWorkspacePathInput(input) {
		return WorkspaceDescriptor{}, ErrWorkspacePathInvalid
	}
	originalWindows := isWindowsPath(input)
	resolvedInput := input
	if originalWindows {
		if resolver.Converter == nil {
			return WorkspaceDescriptor{}, ErrWindowsInteropUnavailable
		}
		converted, err := resolver.Converter.ToWSL(ctx, input)
		if err != nil {
			return WorkspaceDescriptor{}, err
		}
		resolvedInput = converted
	}
	if !filepath.IsAbs(resolvedInput) {
		return WorkspaceDescriptor{}, ErrWorkspacePathInvalid
	}
	canonical, err := filepath.EvalSymlinks(filepath.Clean(resolvedInput))
	if err != nil || !filepath.IsAbs(canonical) || !validWorkspacePathInput(canonical) {
		return WorkspaceDescriptor{}, ErrWorkspacePathUnavailable
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return WorkspaceDescriptor{}, ErrWorkspacePathUnavailable
	}
	pathKind := "wsl"
	if originalWindows || windowsMountPattern.MatchString(canonical) {
		pathKind = "windows-mounted"
	}
	fingerprint := sha256.Sum256([]byte(resolver.RunnerID + "\x00" + canonical))
	displayPath := canonical
	if originalWindows {
		displayPath = input
	}
	return WorkspaceDescriptor{
		CanonicalPath:        canonical,
		DisplayPath:          displayPath,
		PathKind:             pathKind,
		DirectoryFingerprint: fmt.Sprintf("sha256:%x", fingerprint),
	}, nil
}

func isWindowsPath(value string) bool {
	return windowsDrivePathPattern.MatchString(value) || strings.HasPrefix(value, `\\`)
}
