package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/database"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const memoryHealthAcknowledgeApproval = "I_ACKNOWLEDGE_ONE_HISTORICAL_MEMORY_CAPTURE_HEALTH_FAILURE"

var (
	memoryHealthErrorCodePattern            = regexp.MustCompile(`^[A-Z0-9_]{1,64}$`)
	errMemoryHealthAcknowledgeNotAuthorized = errors.New(
		"MEMORY_HEALTH_ACKNOWLEDGE_NOT_AUTHORIZED",
	)
)

type memoryHealthAcknowledgeOptions struct {
	jobID             string
	expectedErrorCode string
	resolutionCode    string
}

type memoryHealthAcknowledger interface {
	AcknowledgeMemoryJobHealth(
		context.Context,
		usermemory.MemoryJobHealthResolutionInput,
	) (bool, error)
}

func runMemoryHealthAcknowledge(args []string, stdout io.Writer) error {
	options, err := parseMemoryHealthAcknowledgeArgs(args)
	if err != nil {
		return err
	}
	cfg := config.Load()
	userID := strings.TrimSpace(cfg.Auth.BootstrapUserID)
	if !canonicalUUID(userID) || strings.TrimSpace(cfg.DatabaseURL) == "" {
		return errors.New("MEMORY_HEALTH_ACKNOWLEDGE_AUTHORITY_UNAVAILABLE")
	}

	ctx, cancel := context.WithTimeout(
		auth.WithUser(context.Background(), auth.User{
			ID: userID, DisplayName: cfg.Auth.BootstrapDisplayName, Role: "user",
		}),
		adminCommandTimeout,
	)
	defer cancel()
	db, err := database.Open(ctx, cfg)
	if err != nil || db == nil || db.SQL() == nil {
		if db != nil {
			_ = db.Close()
		}
		return errors.New("MEMORY_HEALTH_ACKNOWLEDGE_AUTHORITY_UNAVAILABLE")
	}
	defer func() { _ = db.Close() }()

	created, err := acknowledgeMemoryJobHealth(
		ctx,
		usermemory.NewPostgresRepository(db.SQL()),
		options,
	)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		stdout,
		"memory health acknowledged job_id=%s resolution=%s created=%t\n",
		options.jobID,
		options.resolutionCode,
		created,
	)
	return err
}

func parseMemoryHealthAcknowledgeArgs(
	args []string,
) (memoryHealthAcknowledgeOptions, error) {
	flags := flag.NewFlagSet("memory-health-acknowledge", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var jobID, expectedErrorCode, resolutionCode, approval string
	flags.StringVar(&jobID, "job-id", "", "historical extract job UUID")
	flags.StringVar(&expectedErrorCode, "expected-error-code", "", "pinned terminal error code")
	flags.StringVar(&resolutionCode, "resolution-code", "", "bounded resolution code")
	flags.StringVar(&approval, "approval", "", "exact one-job acknowledgement")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 ||
		flagCount(args, "job-id") != 1 ||
		flagCount(args, "expected-error-code") != 1 ||
		flagCount(args, "resolution-code") != 1 ||
		flagCount(args, "approval") != 1 {
		return memoryHealthAcknowledgeOptions{}, usageError()
	}
	if approval != memoryHealthAcknowledgeApproval {
		return memoryHealthAcknowledgeOptions{}, errMemoryHealthAcknowledgeNotAuthorized
	}
	jobID = strings.TrimSpace(jobID)
	expectedErrorCode = strings.TrimSpace(expectedErrorCode)
	resolutionCode = strings.TrimSpace(resolutionCode)
	if !canonicalUUID(jobID) ||
		!memoryHealthErrorCodePattern.MatchString(expectedErrorCode) ||
		!validMemoryHealthResolutionCode(resolutionCode) {
		return memoryHealthAcknowledgeOptions{}, usageError()
	}
	return memoryHealthAcknowledgeOptions{
		jobID:             jobID,
		expectedErrorCode: expectedErrorCode,
		resolutionCode:    resolutionCode,
	}, nil
}

func acknowledgeMemoryJobHealth(
	ctx context.Context,
	repository memoryHealthAcknowledger,
	options memoryHealthAcknowledgeOptions,
) (bool, error) {
	if repository == nil {
		return false, errors.New("MEMORY_HEALTH_ACKNOWLEDGE_AUTHORITY_UNAVAILABLE")
	}
	return repository.AcknowledgeMemoryJobHealth(
		ctx,
		usermemory.MemoryJobHealthResolutionInput{
			JobID:             options.jobID,
			ExpectedErrorCode: options.expectedErrorCode,
			ResolutionCode:    options.resolutionCode,
		},
	)
}

func validMemoryHealthResolutionCode(value string) bool {
	return value == usermemory.MemoryHealthResolutionSourceNoLongerCurrent ||
		value == usermemory.MemoryHealthResolutionHistoricalFailureAccepted
}

func canonicalUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}
