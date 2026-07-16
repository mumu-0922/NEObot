package main

import (
	"bytes"
	"context"
	"encoding/json"
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
	"neo-chat/mm-chat/backend/internal/knowledge"
	"neo-chat/mm-chat/backend/internal/teams"
)

const adminCommandTimeout = 45 * time.Second

const (
	providerWireContractFrozen             = false
	providerWireContractNotFrozenErrorCode = "PROVIDER_WIRE_CONTRACT_NOT_FROZEN"
	ragProviderProfileEnv                  = "RAG_PROVIDER_PROFILE"
	ragProviderProfileDraftAcceptedEnv     = "RAG_PROVIDER_PROFILE_DRAFT_WIRE_ACCEPTED"
	ragDraftAcceptedProviderProfile        = "mineru_jina_postgres_v1"
)

var errProviderWireContractNotFrozen = errors.New(
	providerWireContractNotFrozenErrorCode,
)

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
	case "governance-apply":
		return runGovernanceApply(args[1:], stdin, stdout)
	case "governance-disable":
		return runGovernanceDisable(args[1:], stdout)
	default:
		return usageError()
	}
}

func runGovernanceApply(args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("governance-apply", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var manifestStdin bool
	flags.BoolVar(&manifestStdin, "manifest-stdin", false, "read strict governance manifest JSON from stdin")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || !manifestStdin {
		return usageError()
	}
	// Governance profiles must not be persisted until the Provider Registry and
	// its contract hash binding are frozen, except for the explicit G7 live-smoke
	// profile where the operator has accepted the still-draft MinerU/Jina wire
	// risk via environment. Keep this gate ahead of stdin reads, database setup,
	// and GovernanceService.Apply so no syntactically valid placeholder manifest
	// can become active in the interim.
	if !providerWireContractApplyAllowed() {
		return errProviderWireContractNotFrozen
	}
	manifest, err := readGovernanceManifest(stdin)
	if err != nil {
		return err
	}
	service, closeDatabase, err := openGovernanceService()
	if err != nil {
		return err
	}
	defer closeDatabase()
	ctx, cancel := context.WithTimeout(context.Background(), adminCommandTimeout)
	defer cancel()
	head, err := service.Apply(ctx, manifest)
	if err != nil {
		return err
	}
	return writeGovernanceApplyResult(stdout, head)
}

func providerWireContractApplyAllowed() bool {
	if providerWireContractFrozen {
		return true
	}
	return strings.TrimSpace(os.Getenv(ragProviderProfileEnv)) == ragDraftAcceptedProviderProfile &&
		strings.EqualFold(strings.TrimSpace(os.Getenv(ragProviderProfileDraftAcceptedEnv)), "true")
}

func runGovernanceDisable(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("governance-disable", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var processor, endpointID, modelID string
	flags.StringVar(&processor, "processor", "", "processor alias")
	flags.StringVar(&endpointID, "endpoint-id", "", "processor endpoint identifier")
	flags.StringVar(&modelID, "model-id", "", "processor model identifier")
	if err := flags.Parse(args); err != nil {
		return usageError()
	}
	modelIDSet := false
	flags.Visit(func(value *flag.Flag) {
		modelIDSet = modelIDSet || value.Name == "model-id"
	})
	if flags.NArg() != 0 || strings.TrimSpace(processor) == "" ||
		strings.TrimSpace(endpointID) == "" ||
		(modelIDSet && strings.TrimSpace(modelID) == "") {
		return usageError()
	}
	service, closeDatabase, err := openGovernanceService()
	if err != nil {
		return err
	}
	defer closeDatabase()
	ctx, cancel := context.WithTimeout(context.Background(), adminCommandTimeout)
	defer cancel()
	head, err := disableGovernance(ctx, service, processor, endpointID, modelID)
	if err != nil {
		return err
	}
	return writeGovernanceDisableResult(stdout, head)
}

func writeGovernanceApplyResult(
	stdout io.Writer,
	head knowledge.ProcessorGovernanceHead,
) error {
	_, err := fmt.Fprintf(stdout, "governance active processor=%s endpoint=%s model=%s profile=%s governance_revision=%d head_revision=%d\n",
		head.Processor, head.EndpointID, head.ModelID, head.ActiveProfileID,
		head.ActiveGovernanceRevision, head.HeadRevision)
	return err
}

func writeGovernanceDisableResult(
	stdout io.Writer,
	head knowledge.ProcessorGovernanceHead,
) error {
	_, err := fmt.Fprintf(stdout, "governance disabled processor=%s endpoint=%s model=%s head_revision=%d\n",
		head.Processor, head.EndpointID, head.ModelID, head.HeadRevision)
	return err
}

type governanceDisableService interface {
	Disable(context.Context, string, string) (knowledge.ProcessorGovernanceHead, error)
	DisableModel(context.Context, string, string, string) (knowledge.ProcessorGovernanceHead, error)
}

func disableGovernance(
	ctx context.Context,
	service governanceDisableService,
	processor, endpointID, modelID string,
) (knowledge.ProcessorGovernanceHead, error) {
	if modelID == "" {
		return service.Disable(ctx, processor, endpointID)
	}
	return service.DisableModel(ctx, processor, endpointID, modelID)
}

func readGovernanceManifest(reader io.Reader) (knowledge.GovernanceManifest, error) {
	var manifest knowledge.GovernanceManifest
	if reader == nil {
		return manifest, errors.New("governance manifest stdin is required")
	}
	payload, err := io.ReadAll(io.LimitReader(reader, 64<<10+1))
	if err != nil {
		return manifest, errors.New("read governance manifest")
	}
	if len(payload) > 64<<10 {
		return manifest, errors.New("governance manifest is too large")
	}
	if err := validateGovernanceManifestKeys(payload); err != nil {
		return manifest, err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return manifest, errors.New("decode governance manifest")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return manifest, errors.New("governance manifest must contain one JSON object")
	}
	return manifest, nil
}

func validateGovernanceManifestKeys(payload []byte) error {
	allowed := map[string]struct{}{
		"processor": {}, "endpointId": {}, "modelId": {}, "modelApiVersion": {},
		"allowedPurposes": {}, "allowedDataTypes": {}, "region": {},
		"retentionPolicy": {}, "deletionContract": {}, "trainingUse": {},
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("governance manifest must be a JSON object")
	}
	seen := make(map[string]struct{}, len(allowed))
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return errors.New("decode governance manifest key")
		}
		key, ok := token.(string)
		_, permitted := allowed[key]
		_, duplicate := seen[key]
		if !ok || !permitted || duplicate {
			return errors.New("governance manifest contains an unknown or duplicate field")
		}
		seen[key] = struct{}{}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return errors.New("decode governance manifest value")
		}
	}
	return nil
}

func openGovernanceService() (*knowledge.GovernanceService, func(), error) {
	cfg := config.Load()
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return nil, func() {}, errors.New("DATABASE_URL is required")
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
		return nil, func() {}, knowledge.ErrDatabaseRequired
	}
	return knowledge.NewGovernanceService(knowledge.NewPostgresRepository(db.SQL())), func() { _ = db.Close() }, nil
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
	return errors.New("usage: admin bootstrap-identity --email <mailbox> --password-stdin [--user-id <uuid>] [--display-name <name>] | admin disable-account --user-id <uuid> | admin governance-apply --manifest-stdin | admin governance-disable --processor <alias> --endpoint-id <id> [--model-id <id>]")
}
