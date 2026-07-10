package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/database"
	"neo-chat/mm-chat/backend/internal/teams"
)

const adminCommandTimeout = 45 * time.Second

func main() {
	log.SetFlags(0)
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return usageError()
	}
	switch args[0] {
	case "bootstrap-identity":
		return runBootstrapIdentity(args[1:], stdin, stdout)
	case "disable-account":
		return runDisableAccount(args[1:], stdout)
	default:
		return usageError()
	}
}

func runBootstrapIdentity(args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("bootstrap-identity", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var email, userID, displayName string
	var passwordStdin bool
	flags.StringVar(&email, "email", "", "verified owner email")
	flags.StringVar(&userID, "user-id", "", "owner UUID (optional)")
	flags.StringVar(&displayName, "display-name", "", "owner display name (optional)")
	flags.BoolVar(&passwordStdin, "password-stdin", false, "read the password from standard input")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return usageError()
	}
	if strings.TrimSpace(email) == "" || !passwordStdin {
		return usageError()
	}

	password, err := readPasswordLine(stdin)
	if err != nil {
		return err
	}
	cfg := config.Load()
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return errors.New("DATABASE_URL is required")
	}
	if strings.TrimSpace(userID) == "" {
		userID = cfg.Auth.BootstrapUserID
	}
	if strings.TrimSpace(displayName) == "" {
		displayName = cfg.Auth.BootstrapDisplayName
	}

	ctx, cancel := context.WithTimeout(context.Background(), adminCommandTimeout)
	defer cancel()
	db, err := database.Open(ctx, cfg)
	if err != nil {
		return err
	}
	if db == nil || db.SQL() == nil {
		return auth.ErrDatabaseRequired
	}
	defer func() { _ = db.Close() }()

	repo := auth.NewPostgresSessionRepository(db.SQL())
	service := auth.NewService(repo)
	if err := service.BootstrapIdentity(ctx, userID, email, displayName, password); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, "bootstrap identity created")
	return err
}

func runDisableAccount(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("disable-account", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var userID string
	flags.StringVar(&userID, "user-id", "", "user UUID to disable")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 ||
		strings.TrimSpace(userID) == "" {
		return usageError()
	}

	cfg := config.Load()
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return errors.New("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), adminCommandTimeout)
	defer cancel()
	db, err := database.Open(ctx, cfg)
	if err != nil {
		return err
	}
	if db == nil || db.SQL() == nil {
		return auth.ErrDatabaseRequired
	}
	defer func() { _ = db.Close() }()

	revoked, err := teams.NewService(
		teams.NewPostgresRepository(db.SQL()),
	).DisableAccount(
		ctx,
		strings.TrimSpace(userID),
	)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		stdout,
		"account disable transaction completed; revoked_sessions=%d\n",
		len(revoked),
	)
	return err
}

func readPasswordLine(reader io.Reader) (string, error) {
	if reader == nil {
		return "", errors.New("password stdin is required")
	}
	payload, err := io.ReadAll(io.LimitReader(reader, 1025))
	if err != nil {
		return "", errors.New("read password from stdin")
	}
	if len(payload) > 1024 {
		return "", errors.New("password stdin is too large")
	}
	payload = bytes.TrimSuffix(payload, []byte{'\n'})
	payload = bytes.TrimSuffix(payload, []byte{'\r'})
	if bytes.ContainsAny(payload, "\r\n") {
		return "", errors.New("password stdin must contain exactly one line")
	}
	password := string(payload)
	if password == "" {
		return "", errors.New("password stdin is empty")
	}
	return password, nil
}

func usageError() error {
	return errors.New("usage: admin bootstrap-identity --email <mailbox> --password-stdin [--user-id <uuid>] [--display-name <name>] | admin disable-account --user-id <uuid>")
}
