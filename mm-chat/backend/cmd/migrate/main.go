package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/database"
	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

const migrateTimeout = 5 * time.Minute

const migrationDatabaseURLEnv = "MIGRATION_DATABASE_URL"

func main() {
	log.SetFlags(0)
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	options, err := parseCommand(args)
	if err != nil {
		return err
	}
	cfg, err := loadMigrationConfig(os.LookupEnv)
	if err != nil {
		return err
	}
	var governanceMapping *migration.Phase15GovernanceMapping
	if options.governanceMapPath != "" {
		mappingData, readErr := os.ReadFile(options.governanceMapPath)
		if readErr != nil {
			return fmt.Errorf("read Phase 15 governance mapping file: %w", readErr)
		}
		mapping, parseErr := migration.ParsePhase15GovernanceMapping(mappingData)
		if parseErr != nil {
			return parseErr
		}
		governanceMapping = &mapping
	}

	ctx, cancel := context.WithTimeout(context.Background(), migrateTimeout)
	defer cancel()

	db, err := database.Open(ctx, cfg)
	if err != nil {
		// database.Open errors can contain connection metadata. The migration
		// credential is intentionally never reflected through the CLI error path.
		return errors.New("open migration database failed")
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("database close failed: %v", err)
		}
	}()

	runner := migration.NewRunner(db.SQL(), migrationfiles.FS)
	if governanceMapping != nil {
		if _, configureErr := runner.WithPhase15GovernanceMapping(*governanceMapping); configureErr != nil {
			return configureErr
		}
	}
	var changed []migration.Migration
	switch options.command {
	case "up":
		changed, err = runner.Up(ctx)
	case "down":
		changed, err = runner.Down(ctx, options.downAll)
	case "baseline":
		changed, err = runner.BaselineLegacyChecksums(ctx)
	}
	if err != nil {
		return err
	}

	if len(changed) == 0 {
		log.Printf("no migrations changed")
		return nil
	}
	for _, m := range changed {
		log.Printf("%s %s", options.command, m.ID())
	}

	return nil
}

func loadMigrationConfig(
	lookup func(string) (string, bool),
) (config.Config, error) {
	databaseURL, exists := lookup(migrationDatabaseURLEnv)
	databaseURL = strings.TrimSpace(databaseURL)
	if !exists || databaseURL == "" {
		return config.Config{}, errors.New("MIGRATION_DATABASE_URL is required")
	}

	return config.Config{
		DatabaseURL:       databaseURL,
		DBMaxOpenConns:    config.DefaultDBMaxOpenConns,
		DBMaxIdleConns:    config.DefaultDBMaxIdleConns,
		DBConnMaxLifetime: config.DefaultDBConnMaxLifetime,
	}, nil
}

type commandOptions struct {
	command           string
	downAll           bool
	governanceMapPath string
}

func parseCommand(args []string) (commandOptions, error) {
	if len(args) == 0 {
		return commandOptions{}, usageError()
	}

	options := commandOptions{command: args[0]}
	switch options.command {
	case "up":
		flags := flag.NewFlagSet("up", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		flags.StringVar(
			&options.governanceMapPath,
			"phase15-governance-map",
			"",
			"path to Phase 15 governance mapping JSON",
		)
		if err := flags.Parse(args[1:]); err != nil {
			return commandOptions{}, err
		}
		if flags.NArg() != 0 {
			return commandOptions{}, errors.New("up accepts only flags")
		}
		if strings.TrimSpace(options.governanceMapPath) != options.governanceMapPath {
			return commandOptions{}, errors.New("phase15 governance mapping path must not have surrounding whitespace")
		}
	case "baseline":
		if len(args) != 1 {
			return commandOptions{}, errors.New("baseline does not accept flags or arguments")
		}
	case "down":
		flags := flag.NewFlagSet("down", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		flags.BoolVar(&options.downAll, "all", false, "roll back all applied migrations")
		if err := flags.Parse(args[1:]); err != nil {
			return commandOptions{}, err
		}
		if flags.NArg() != 0 {
			return commandOptions{}, errors.New("down accepts only flags")
		}
	default:
		return commandOptions{}, usageError()
	}
	return options, nil
}

func usageError() error {
	return errors.New(
		"usage: migrate up [--phase15-governance-map=FILE] | migrate down [--all] | migrate baseline",
	)
}
