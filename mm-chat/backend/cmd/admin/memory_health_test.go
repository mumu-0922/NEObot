package main

import (
	"context"
	"errors"
	"testing"

	"neo-chat/mm-chat/backend/internal/usermemory"
)

const testMemoryHealthJobID = "71000000-0000-4000-8000-000000000001"

type fakeMemoryHealthAcknowledger struct {
	input   usermemory.MemoryJobHealthResolutionInput
	created bool
	err     error
}

func (f *fakeMemoryHealthAcknowledger) AcknowledgeMemoryJobHealth(
	_ context.Context,
	input usermemory.MemoryJobHealthResolutionInput,
) (bool, error) {
	f.input = input
	return f.created, f.err
}

func TestParseMemoryHealthAcknowledgeArgsRequiresExactBindings(t *testing.T) {
	base := []string{
		"--job-id", testMemoryHealthJobID,
		"--expected-error-code", "EXTRACTION_INVALID",
		"--resolution-code", usermemory.MemoryHealthResolutionHistoricalFailureAccepted,
		"--approval", memoryHealthAcknowledgeApproval,
	}
	options, err := parseMemoryHealthAcknowledgeArgs(base)
	if err != nil || options.jobID != testMemoryHealthJobID ||
		options.expectedErrorCode != "EXTRACTION_INVALID" ||
		options.resolutionCode != usermemory.MemoryHealthResolutionHistoricalFailureAccepted {
		t.Fatalf("parseMemoryHealthAcknowledgeArgs() = %#v/%v", options, err)
	}

	for name, args := range map[string][]string{
		"missing approval": base[:6],
		"wrong approval":   append(append([]string{}, base[:7]...), "WRONG"),
		"duplicate job":    append(append([]string{}, base...), "--job-id", testMemoryHealthJobID),
		"noncanonical job": append([]string{
			"--job-id", "71000000-0000-4000-8000-00000000000A",
		}, base[2:]...),
		"dynamic error": append([]string{
			"--job-id", testMemoryHealthJobID,
			"--expected-error-code", "provider timeout",
		}, base[4:]...),
		"unknown resolution": append([]string{
			"--job-id", testMemoryHealthJobID,
			"--expected-error-code", "EXTRACTION_INVALID",
			"--resolution-code", "ignore_everything",
		}, base[6:]...),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseMemoryHealthAcknowledgeArgs(args); err == nil {
				t.Fatalf("parseMemoryHealthAcknowledgeArgs(%v) error = nil", args)
			}
		})
	}
}

func TestAcknowledgeMemoryJobHealthForwardsContentFreeBinding(t *testing.T) {
	repository := &fakeMemoryHealthAcknowledger{created: true}
	created, err := acknowledgeMemoryJobHealth(
		context.Background(),
		repository,
		memoryHealthAcknowledgeOptions{
			jobID:             testMemoryHealthJobID,
			expectedErrorCode: "SOURCE_DRIFT",
			resolutionCode:    usermemory.MemoryHealthResolutionSourceNoLongerCurrent,
		},
	)
	if err != nil || !created || repository.input != (usermemory.MemoryJobHealthResolutionInput{
		JobID:             testMemoryHealthJobID,
		ExpectedErrorCode: "SOURCE_DRIFT",
		ResolutionCode:    usermemory.MemoryHealthResolutionSourceNoLongerCurrent,
	}) {
		t.Fatalf("acknowledgeMemoryJobHealth() = %t/%v input=%#v", created, err, repository.input)
	}

	wantErr := errors.New("fixture")
	repository.err = wantErr
	if _, err := acknowledgeMemoryJobHealth(
		context.Background(), repository, memoryHealthAcknowledgeOptions{},
	); !errors.Is(err, wantErr) {
		t.Fatalf("acknowledgeMemoryJobHealth() error = %v", err)
	}
	if _, err := acknowledgeMemoryJobHealth(
		context.Background(), nil, memoryHealthAcknowledgeOptions{},
	); err == nil {
		t.Fatal("nil repository accepted")
	}
}
