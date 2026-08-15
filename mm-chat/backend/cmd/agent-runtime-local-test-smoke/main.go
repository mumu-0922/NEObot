// Command agent-runtime-local-test-smoke runs one explicitly non-production
// synthetic Skill through the real rootless Runner lifecycle.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"neo-chat/mm-chat/backend/internal/agentrunner"
)

func main() {
	config, err := parseConfig(os.Args[1:])
	if err != nil {
		os.Exit(2)
	}
	ctx, cancel := contextWithTimeout(90 * time.Second)
	defer cancel()
	report, err := agentrunner.RunLocalTestSmoke(ctx, config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "LOCAL_SKILL_SMOKE_FAILED")
		os.Exit(1)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		os.Exit(1)
	}
	fmt.Println(string(encoded))
}

func parseConfig(arguments []string) (agentrunner.LocalTestSmokeConfig, error) {
	flags := flag.NewFlagSet("agent-runtime-local-test-smoke", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	podman := flags.String("podman", "", "absolute local-test Podman launcher path")
	workload := flags.String("workload", "", "absolute local-test workload path")
	seccomp := flags.String("seccomp", "", "absolute reviewed seccomp path")
	stateRoot := flags.String("state-root", "", "absolute disposable local-test state root")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 ||
		*podman == "" || *workload == "" || *seccomp == "" || *stateRoot == "" {
		return agentrunner.LocalTestSmokeConfig{}, errors.New("invalid local-test configuration")
	}
	return agentrunner.LocalTestSmokeConfig{
		PodmanPath:   filepath.Clean(*podman),
		WorkloadPath: filepath.Clean(*workload),
		SeccompPath:  filepath.Clean(*seccomp),
		StateRoot:    filepath.Clean(*stateRoot),
	}, nil
}

func contextWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}
