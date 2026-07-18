package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"
	"strings"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/database"
	"neo-chat/mm-chat/backend/internal/providersecrets"
	"neo-chat/mm-chat/backend/internal/runtimeconfig"
)

var sha256HexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

const providerSecretRewriteOutputFormat = "provider secret rewrite mode=%s " +
	"total_rows=%d secret_rows=%d changed_rows=%d legacy_rows=%d " +
	"env_rows=%d rotated_rows=%d current_rows=%d empty_rows=%d " +
	"blocked_rows=%d plan_sha256=%s\n"

type providerSecretRewriteCommandOptions struct {
	rewrite               runtimeconfig.ProviderSecretRewriteOptions
	confirmedBackupSHA256 string
}

func runProviderSecretsRewrite(args []string, stdout io.Writer) error {
	options, err := parseProviderSecretRewriteArgs(args)
	if err != nil {
		return err
	}
	rewriter, closeDatabase, err := openProviderSecretRewriter()
	if err != nil {
		return err
	}
	defer closeDatabase()
	ctx, cancel := context.WithTimeout(context.Background(), adminCommandTimeout)
	defer cancel()
	result, err := rewriter.Rewrite(ctx, options.rewrite)
	if err != nil {
		return err
	}
	return writeProviderSecretRewriteResult(stdout, result)
}

func parseProviderSecretRewriteArgs(
	args []string,
) (providerSecretRewriteCommandOptions, error) {
	flags := flag.NewFlagSet("provider-secrets-rewrite", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var execute bool
	var expectedPlan, backupSHA256 string
	flags.BoolVar(&execute, "execute", false, "execute the exact dry-run plan")
	flags.StringVar(&expectedPlan, "expected-plan-sha256", "", "exact dry-run plan digest")
	flags.StringVar(&backupSHA256, "confirmed-backup-sha256", "", "verified pre-rewrite Postgres backup digest")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return providerSecretRewriteCommandOptions{}, usageError()
	}
	expectedPlan = strings.TrimSpace(expectedPlan)
	backupSHA256 = strings.TrimSpace(backupSHA256)
	if execute {
		if !sha256HexPattern.MatchString(expectedPlan) ||
			!sha256HexPattern.MatchString(backupSHA256) {
			return providerSecretRewriteCommandOptions{}, usageError()
		}
	} else if expectedPlan != "" || backupSHA256 != "" {
		return providerSecretRewriteCommandOptions{}, usageError()
	}
	return providerSecretRewriteCommandOptions{
		rewrite: runtimeconfig.ProviderSecretRewriteOptions{
			Execute: execute, ExpectedPlanSHA256: expectedPlan,
		},
		confirmedBackupSHA256: backupSHA256,
	}, nil
}

func openProviderSecretRewriter() (
	*runtimeconfig.PostgresProviderSecretRewriter,
	func(),
	error,
) {
	cfg := config.Load()
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return nil, func() {}, errors.New("DATABASE_URL is required")
	}
	vault, err := providersecrets.LoadVaultFile(cfg.ProviderSecrets.KeyringFile)
	if err != nil {
		return nil, func() {}, runtimeconfig.ErrProviderSecretRewriteUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), adminCommandTimeout)
	defer cancel()
	db, err := database.Open(ctx, cfg)
	if err != nil {
		return nil, func() {}, err
	}
	if db == nil || db.SQL() == nil {
		if db != nil {
			_ = db.Close()
		}
		return nil, func() {}, runtimeconfig.ErrDatabaseRequired
	}
	return runtimeconfig.NewPostgresProviderSecretRewriter(
		db.SQL(), cfg, vault,
	), func() { _ = db.Close() }, nil
}

func writeProviderSecretRewriteResult(
	stdout io.Writer,
	result runtimeconfig.ProviderSecretRewriteResult,
) error {
	mode := "dry-run"
	if result.Executed {
		mode = "executed"
	}
	_, err := fmt.Fprintf(
		stdout,
		providerSecretRewriteOutputFormat,
		mode,
		result.TotalRows,
		result.SecretRows,
		result.ChangedRows,
		result.LegacyRows,
		result.EnvRows,
		result.RotatedRows,
		result.CurrentRows,
		result.EmptyRows,
		result.BlockedRows,
		result.PlanSHA256,
	)
	return err
}
