package agenthost

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf16"
)

var (
	ErrWorkspacePathInvalid       = errors.New("workspace path is invalid")
	ErrWorkspacePathUnavailable   = errors.New("workspace path is unavailable")
	ErrWindowsInteropUnavailable  = errors.New("Windows path interop is unavailable")
	ErrDirectoryBrowseUnavailable = errors.New("directory browsing is unavailable")
	ErrNativePickerUnavailable    = errors.New("native directory picker is unavailable")
	ErrDirectoryPickerCancelled   = errors.New("native directory picker was cancelled")

	windowsDrivePathPattern = regexp.MustCompile(`^[A-Za-z]:[\\/]`)
	windowsMountPattern     = regexp.MustCompile(`^/mnt/[A-Za-z](?:/|$)`)
)

type WorkspaceResolver interface {
	ResolveWorkspace(context.Context, string) (WorkspaceDescriptor, error)
	WindowsPathInterop() bool
}

type DirectoryBrowser interface {
	BrowseDirectories(context.Context, string) (DirectoryBrowseResponse, error)
	DirectoryBrowseAvailable() bool
}

type NativeDirectoryPicker interface {
	PickNativeDirectory(context.Context) (WorkspaceDescriptor, error)
	NativeDirectoryPickerAvailable() bool
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
	Picker    *ExecWindowsDirectoryPicker
}

func NewLocalWorkspaceResolver(runnerID string) (*LocalWorkspaceResolver, error) {
	if err := validateRunnerID(runnerID); err != nil {
		return nil, err
	}
	return &LocalWorkspaceResolver{
		RunnerID: runnerID, Converter: NewExecWindowsPathConverter(),
		Picker: NewExecWindowsDirectoryPicker(),
	}, nil
}

func (resolver *LocalWorkspaceResolver) WindowsPathInterop() bool {
	converter, ok := resolver.Converter.(*ExecWindowsPathConverter)
	return ok && converter.Available()
}

func (resolver *LocalWorkspaceResolver) DirectoryBrowseAvailable() bool {
	return resolver != nil
}

func (resolver *LocalWorkspaceResolver) NativeDirectoryPickerAvailable() bool {
	return resolver != nil && resolver.Picker != nil && resolver.Picker.Available()
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

func (resolver *LocalWorkspaceResolver) BrowseDirectories(
	ctx context.Context,
	input string,
) (DirectoryBrowseResponse, error) {
	if resolver == nil || validateRunnerID(resolver.RunnerID) != nil {
		return DirectoryBrowseResponse{}, ErrDirectoryBrowseUnavailable
	}
	if strings.TrimSpace(input) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return DirectoryBrowseResponse{}, ErrDirectoryBrowseUnavailable
		}
		input = home
	}
	current, err := resolver.ResolveWorkspace(ctx, input)
	if err != nil {
		return DirectoryBrowseResponse{}, err
	}
	items, err := os.ReadDir(current.CanonicalPath)
	if err != nil {
		return DirectoryBrowseResponse{}, ErrWorkspacePathUnavailable
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name()) < strings.ToLower(items[j].Name())
	})
	entries := make([]DirectoryEntry, 0, min(len(items), 256))
	for _, item := range items {
		if len(entries) >= 256 {
			break
		}
		if !validDirectoryName(item.Name()) {
			continue
		}
		candidate := filepath.Join(current.CanonicalPath, item.Name())
		info, statErr := os.Stat(candidate)
		if statErr != nil || !info.IsDir() {
			continue
		}
		pathKind := "wsl"
		if windowsMountPattern.MatchString(candidate) {
			pathKind = "windows-mounted"
		}
		entries = append(entries, DirectoryEntry{
			Name: item.Name(), Path: candidate, DisplayPath: candidate, PathKind: pathKind,
		})
	}
	parent := filepath.Dir(current.CanonicalPath)
	if parent == current.CanonicalPath {
		parent = ""
	}
	return DirectoryBrowseResponse{
		ProtocolVersion: ProtocolVersion,
		RunnerID:        resolver.RunnerID,
		Path:            current.CanonicalPath,
		DisplayPath:     current.DisplayPath,
		PathKind:        current.PathKind,
		ParentPath:      parent,
		Entries:         entries,
	}, nil
}

func validDirectoryName(value string) bool {
	return value != "" && value != "." && value != ".." && validWorkspacePathInput(value) &&
		!strings.ContainsAny(value, `/\\`)
}

func (resolver *LocalWorkspaceResolver) PickNativeDirectory(
	ctx context.Context,
) (WorkspaceDescriptor, error) {
	if !resolver.NativeDirectoryPickerAvailable() {
		return WorkspaceDescriptor{}, ErrNativePickerUnavailable
	}
	selected, err := resolver.Picker.Pick(ctx)
	if err != nil {
		return WorkspaceDescriptor{}, err
	}
	return resolver.ResolveWorkspace(ctx, selected)
}

type ExecWindowsDirectoryPicker struct {
	Executable string
	Timeout    time.Duration
}

func NewExecWindowsDirectoryPicker() *ExecWindowsDirectoryPicker {
	executable, _ := exec.LookPath("powershell.exe")
	return &ExecWindowsDirectoryPicker{Executable: executable, Timeout: 5 * time.Minute}
}

func (picker *ExecWindowsDirectoryPicker) Available() bool {
	return picker != nil && filepath.IsAbs(strings.TrimSpace(picker.Executable))
}

func (picker *ExecWindowsDirectoryPicker) Pick(ctx context.Context) (string, error) {
	if !picker.Available() {
		return "", ErrNativePickerUnavailable
	}
	timeout := picker.Timeout
	if timeout <= 0 || timeout > 10*time.Minute {
		timeout = 5 * time.Minute
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	const script = `$ErrorActionPreference='Stop'; Add-Type -AssemblyName System.Windows.Forms; $d=New-Object System.Windows.Forms.FolderBrowserDialog; $d.Description='Select a project folder'; $d.ShowNewFolderButton=$false; if($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK){[Console]::OutputEncoding=New-Object System.Text.UTF8Encoding($false); Write-Output $d.SelectedPath; exit 0}; exit 2`
	encodedScript := encodePowerShellCommand(script)
	command := exec.CommandContext(
		commandCtx, picker.Executable,
		"-NoLogo", "-NoProfile", "-STA", "-NonInteractive", "-EncodedCommand", encodedScript,
	)
	command.Env = os.Environ()
	output, err := command.Output()
	if commandCtx.Err() != nil {
		return "", ErrNativePickerUnavailable
	}
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) && exitError.ExitCode() == 2 {
			return "", ErrDirectoryPickerCancelled
		}
		return "", ErrNativePickerUnavailable
	}
	if len(output) > maxWorkspacePathBytes+2 {
		return "", ErrWorkspacePathInvalid
	}
	selected := strings.TrimSpace(string(output))
	if !isWindowsPath(selected) || !validWorkspacePathInput(selected) {
		return "", ErrWorkspacePathInvalid
	}
	return selected, nil
}

func encodePowerShellCommand(value string) string {
	encoded := utf16.Encode([]rune(value))
	data := make([]byte, len(encoded)*2)
	for index, char := range encoded {
		data[index*2] = byte(char)
		data[index*2+1] = byte(char >> 8)
	}
	return base64.StdEncoding.EncodeToString(data)
}

func isWindowsPath(value string) bool {
	return windowsDrivePathPattern.MatchString(value) || strings.HasPrefix(value, `\\`)
}
