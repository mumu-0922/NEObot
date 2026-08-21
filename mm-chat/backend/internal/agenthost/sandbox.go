package agenthost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/localskills"
)

const sandboxProbeTimeout = 10 * time.Second

type sandboxRuntime struct {
	command string
	modes   []PermissionMode
}

func probeSandboxRuntime(command string) (sandboxRuntime, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return sandboxRuntime{modes: []PermissionMode{PermissionFullAccess}}, nil
	}
	command = filepath.Clean(command)
	if !localskills.SecureSandboxCommand(command) {
		return sandboxRuntime{}, errors.New("Host sandbox command is invalid")
	}
	roots := []string{os.TempDir()}
	if runtime.GOOS == "linux" {
		// The supported Windows/WSL topology executes Windows projects through
		// DrvFS. A mode is not advertised unless the live target mount proves it.
		if info, err := os.Stat("/mnt/d"); err == nil && info.IsDir() {
			roots = append(roots, "/mnt/d")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), sandboxProbeTimeout)
	defer cancel()
	for _, root := range roots {
		if err := probeSandboxRoot(ctx, command, root); err != nil {
			return sandboxRuntime{}, err
		}
	}
	return sandboxRuntime{
		command: command,
		modes: []PermissionMode{
			PermissionReadOnly, PermissionWorkspaceWrite, PermissionFullAccess,
		},
	}, nil
}

func probeSandboxRoot(ctx context.Context, command, root string) error {
	base, err := os.MkdirTemp(root, ".neo-chat-sandbox-probe-")
	if err != nil {
		return fmt.Errorf("create Host sandbox probe: %w", err)
	}
	defer os.RemoveAll(base)
	inside := filepath.Join(base, "inside")
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(inside, 0o700); err != nil {
		return err
	}
	if err := os.Mkdir(outside, 0o700); err != nil {
		return err
	}
	if err := runSandboxProbe(ctx, command, PermissionReadOnly, inside, outside); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(inside, "probe")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("read-only Host sandbox permitted a workspace mutation")
	}
	if _, err := os.Stat(filepath.Join(outside, "probe")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("read-only Host sandbox permitted an outside mutation")
	}
	if err := runSandboxProbe(ctx, command, PermissionWorkspaceWrite, inside, outside); err != nil {
		return err
	}
	insideBody, err := os.ReadFile(filepath.Join(inside, "probe"))
	if err != nil || string(insideBody) != "inside" {
		return errors.New("workspace-write Host sandbox denied the workspace mutation")
	}
	if _, err := os.Stat(filepath.Join(outside, "probe")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("workspace-write Host sandbox permitted an outside mutation")
	}
	return nil
}

func runSandboxProbe(
	ctx context.Context,
	command string,
	mode PermissionMode,
	inside string,
	outside string,
) error {
	arguments := localskills.SandboxCommandArguments(string(mode), inside, inside)
	probe := `
if printf outside >"$NEO_CHAT_PROBE_OUTSIDE/probe"; then exit 71; fi
if [ "$NEO_CHAT_PROBE_MODE" = read-only ]; then
  if printf inside >"$NEO_CHAT_PROBE_INSIDE/probe"; then exit 72; fi
else
  printf inside >"$NEO_CHAT_PROBE_INSIDE/probe" || exit 73
fi
`
	arguments = append(arguments, "--", "/bin/sh", "-c", probe)
	process := exec.CommandContext(ctx, command, arguments...)
	process.Env = append(os.Environ(),
		"NEO_CHAT_PROBE_INSIDE="+inside,
		"NEO_CHAT_PROBE_OUTSIDE="+outside,
		"NEO_CHAT_PROBE_MODE="+string(mode),
	)
	if output, err := process.CombinedOutput(); err != nil {
		return fmt.Errorf("Host sandbox probe failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
