package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "memory-benchmark-author:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New(
			"subcommand is required: generate, review, status, verify, freeze, holdout-begin, " +
				"regression-generate, regression-status, regression-verify, regression-v3-generate, " +
				"regression-v3-status, regression-v3-verify, regression-v4-generate, " +
				"regression-v4-status, or regression-v4-verify",
		)
	}
	repositoryRoot, err := findRepositoryRoot()
	if err != nil {
		return err
	}
	defaultRoot := filepath.Join(repositoryRoot, "mm-chat", "data", "memory-benchmark", "v1")
	defaultRegressionRoot := filepath.Join(repositoryRoot, "mm-chat", "data", "memory-benchmark", "v2-regression")
	defaultRegressionV3Root := filepath.Join(repositoryRoot, "mm-chat", "data", "memory-benchmark", "v3-regression")
	defaultRegressionV4Root := filepath.Join(repositoryRoot, "mm-chat", "data", "memory-benchmark", "v4-regression")
	switch args[0] {
	case "regression-generate":
		return runRegressionGenerate(
			args[0], args[1:], repositoryRoot, defaultRegressionRoot, "",
			memoryauthor.GenerateRegression, stdout,
		)
	case "regression-v3-generate":
		return runRegressionGenerate(
			args[0], args[1:], repositoryRoot, defaultRegressionV3Root,
			memoryauthor.RegressionRepairedProfileID, memoryauthor.GenerateRegressionV3, stdout,
		)
	case "regression-v4-generate":
		return runRegressionGenerate(
			args[0], args[1:], repositoryRoot, defaultRegressionV4Root,
			memoryauthor.RegressionSemanticProfileID, memoryauthor.GenerateRegressionV4, stdout,
		)
	case "regression-status", "regression-verify":
		return runRegressionInspect(
			args[0], args[1:], repositoryRoot, defaultRegressionRoot, "", stdout,
		)
	case "regression-v3-status", "regression-v3-verify":
		return runRegressionInspect(
			args[0], args[1:], repositoryRoot, defaultRegressionV3Root,
			memoryauthor.RegressionRepairedProfileID, stdout,
		)
	case "regression-v4-status", "regression-v4-verify":
		return runRegressionInspect(
			args[0], args[1:], repositoryRoot, defaultRegressionV4Root,
			memoryauthor.RegressionSemanticProfileID, stdout,
		)
	case "generate":
		flags := newFlags("generate")
		root := flags.String("root", defaultRoot, "new protected authoring root")
		if err := parseFlags(flags, args[1:]); err != nil {
			return err
		}
		validatedRoot, err := memoryauthor.ValidateFormalRoot(*root, repositoryRoot)
		if err != nil {
			return err
		}
		pool, err := memoryauthor.Generate()
		if err != nil {
			return err
		}
		if err := memoryauthor.PublishPool(validatedRoot, pool); err != nil {
			return err
		}
		status, err := memoryauthor.Verify(validatedRoot)
		if err != nil {
			return err
		}
		return encodeJSON(stdout, status)
	case "review":
		flags := newFlags("review")
		root := flags.String("root", defaultRoot, "protected authoring root")
		reviewer := flags.String("reviewer", "", "explicit human reviewer UUID")
		if err := parseFlags(flags, args[1:]); err != nil {
			return err
		}
		validatedRoot, err := memoryauthor.ValidateFormalRoot(*root, repositoryRoot)
		if err != nil {
			return err
		}
		server, err := memoryauthor.StartReviewServer(memoryauthor.ReviewServerOptions{
			Root: validatedRoot, ReviewerID: strings.TrimSpace(*reviewer),
		})
		if err != nil {
			return err
		}
		if err := encodeJSON(stdout, struct {
			SchemaVersion string `json:"schemaVersion"`
			URL           string `json:"url"`
		}{SchemaVersion: "neo-chat.memory-benchmark-review-session.v1", URL: server.URL()}); err != nil {
			_ = server.Close(context.Background())
			return err
		}
		select {
		case <-ctx.Done():
		case serveErr := <-server.Done():
			if serveErr != nil {
				return fmt.Errorf("review server stopped: %w", serveErr)
			}
		}
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Close(shutdown)
	case "status", "verify":
		flags := newFlags(args[0])
		root := flags.String("root", defaultRoot, "protected authoring root")
		if err := parseFlags(flags, args[1:]); err != nil {
			return err
		}
		validatedRoot, err := memoryauthor.ValidateFormalRoot(*root, repositoryRoot)
		if err != nil {
			return err
		}
		var status memoryauthor.Status
		if args[0] == "verify" {
			status, err = memoryauthor.Verify(validatedRoot)
		} else {
			status, err = memoryauthor.CurrentStatus(validatedRoot)
		}
		if err != nil {
			return err
		}
		return encodeJSON(stdout, status)
	case "freeze":
		flags := newFlags("freeze")
		root := flags.String("root", defaultRoot, "protected authoring root")
		holdoutRunID := flags.String("holdout-run-id", "", "precommitted Holdout UUID")
		if err := parseFlags(flags, args[1:]); err != nil {
			return err
		}
		validatedRoot, err := memoryauthor.ValidateFormalRoot(*root, repositoryRoot)
		if err != nil {
			return err
		}
		frozen, err := memoryauthor.Freeze(validatedRoot, memoryauthor.FreezeInput{
			HoldoutRunID: strings.TrimSpace(*holdoutRunID),
		})
		if err != nil {
			return err
		}
		return encodeJSON(stdout, struct {
			SchemaVersion       string `json:"schemaVersion"`
			GoldenContentSHA256 string `json:"goldenContentSha256"`
			FixtureRawSHA256    string `json:"fixtureRawSha256"`
			HoldoutRunID        string `json:"holdoutRunId"`
			CaseCount           int    `json:"caseCount"`
		}{
			SchemaVersion:       "neo-chat.memory-benchmark-freeze-output.v1",
			GoldenContentSHA256: frozen.Manifest.GoldenContentSHA256,
			FixtureRawSHA256:    frozen.Manifest.FixtureRawSHA256,
			HoldoutRunID:        frozen.Manifest.HoldoutRunID,
			CaseCount:           len(frozen.Golden.Cases),
		})
	case "holdout-begin":
		flags := newFlags("holdout-begin")
		root := flags.String("root", defaultRoot, "protected authoring root")
		holdoutRunID := flags.String("holdout-run-id", "", "precommitted Holdout UUID")
		output := flags.String("output", "", "new protected Holdout bundle path")
		if err := parseFlags(flags, args[1:]); err != nil {
			return err
		}
		validatedRoot, err := memoryauthor.ValidateFormalRoot(*root, repositoryRoot)
		if err != nil {
			return err
		}
		use, err := memoryauthor.BeginHoldout(validatedRoot, memoryauthor.HoldoutInput{
			HoldoutRunID: strings.TrimSpace(*holdoutRunID), OutputPath: strings.TrimSpace(*output),
		})
		if err != nil {
			return err
		}
		return encodeJSON(stdout, use)
	default:
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func runRegressionGenerate(
	command string,
	args []string,
	repositoryRoot string,
	defaultRoot string,
	expectedProfile string,
	generate func() (memoryauthor.RegressionPool, error),
	stdout io.Writer,
) error {
	flags := newFlags(command)
	root := flags.String("root", defaultRoot, "new protected regression root")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	validatedRoot, err := memoryauthor.ValidateRegressionRoot(*root, repositoryRoot)
	if err != nil {
		return err
	}
	pool, err := generate()
	if err != nil {
		return err
	}
	if err := memoryauthor.PublishRegression(validatedRoot, pool); err != nil {
		return err
	}
	status, err := memoryauthor.VerifyRegression(validatedRoot)
	if err != nil {
		return err
	}
	if err := requireRegressionProfile(status, expectedProfile); err != nil {
		return err
	}
	return encodeJSON(stdout, status)
}

func runRegressionInspect(
	command string,
	args []string,
	repositoryRoot string,
	defaultRoot string,
	expectedProfile string,
	stdout io.Writer,
) error {
	flags := newFlags(command)
	root := flags.String("root", defaultRoot, "protected regression root")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	validatedRoot, err := memoryauthor.ValidateRegressionRoot(*root, repositoryRoot)
	if err != nil {
		return err
	}
	var status memoryauthor.RegressionStatus
	if strings.HasSuffix(command, "-verify") {
		status, err = memoryauthor.VerifyRegression(validatedRoot)
	} else {
		status, err = memoryauthor.CurrentRegressionStatus(validatedRoot)
	}
	if err != nil {
		return err
	}
	if err := requireRegressionProfile(status, expectedProfile); err != nil {
		return err
	}
	return encodeJSON(stdout, status)
}

func requireRegressionProfile(status memoryauthor.RegressionStatus, expected string) error {
	if expected != "" && status.Profile != expected {
		return fmt.Errorf("regression profile %q does not match command profile %q", status.Profile, expected)
	}
	return nil
}

func newFlags(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

func parseFlags(flags *flag.FlagSet, args []string) error {
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("invalid arguments")
	}
	return nil
}

func encodeJSON(output io.Writer, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return errors.New("encode command output")
	}
	if _, err := output.Write(append(body, '\n')); err != nil {
		return errors.New("write command output")
	}
	return nil
}

func findRepositoryRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", errors.New("resolve working directory")
	}
	for {
		if _, err := os.Lstat(filepath.Join(current, ".git")); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("repository root was not found")
		}
		current = parent
	}
}
