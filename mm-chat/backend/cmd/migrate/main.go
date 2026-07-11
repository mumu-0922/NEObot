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

func main() {
	log.SetFlags(0)
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usageError()
	}

	command := args[0]
	var downAll bool
	switch command {
	case "up", "baseline":
		if len(args) != 1 {
			return fmt.Errorf("%s does not accept flags or arguments", command)
		}
	case "down":
		flags := flag.NewFlagSet("down", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		flags.BoolVar(&downAll, "all", false, "roll back all applied migrations")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("down accepts only flags")
		}
	default:
		return usageError()
	}

	cfg := config.Load()
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return errors.New("DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), migrateTimeout)
	defer cancel()

	db, err := database.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("database close failed: %v", err)
		}
	}()

	runner := migration.NewRunner(db.SQL(), migrationfiles.FS)
	var changed []migration.Migration
	switch command {
	case "up":
		changed, err = runner.Up(ctx)
	case "down":
		changed, err = runner.Down(ctx, downAll)
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
		log.Printf("%s %s", command, m.ID())
	}

	return nil
}

func usageError() error {
	return errors.New("usage: migrate <up|down|baseline> [--all]")
}
