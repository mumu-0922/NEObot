package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/memorycapture"
	"neo-chat/mm-chat/backend/internal/ragproviders"
)

func TestParseCommandSeparatesFakeAndLiveCredentialBoundaries(t *testing.T) {
	base := []string{
		"-root", "/protected/regression", "-output-dir", "/private/output",
		"-cost-basis", "/private/cost.json", "-run-id", "run-1",
	}
	options, err := parseCommand(base)
	if err != nil || options.providerMode != memorycapture.ProviderModeFakeProtocol {
		t.Fatalf("parse fake command = %#v/%v", options, err)
	}
	if _, err := parseCommand(append(append([]string{}, base...), "-credential-file", "/secret")); err == nil {
		t.Fatal("fake protocol accepted a credential file")
	}
	live := append(append([]string{}, base...),
		"-provider-mode", memorycapture.ProviderModeLiveSiliconFlow,
		"-credential-file", "/secret",
	)
	options, err = parseCommand(live)
	if err != nil || options.credentialPath != "/secret" {
		t.Fatalf("parse live command = %#v/%v", options, err)
	}
	if _, err := parseCommand(append(append([]string{}, base...),
		"-provider-mode", memorycapture.ProviderModeLiveSiliconFlow)); err == nil {
		t.Fatal("live mode omitted credential file")
	}
}

func TestLoadDatabaseConfigsRejectsLiveOrUnprivilegedTopologyBeforeConnect(t *testing.T) {
	valid := map[string]string{
		adminDatabaseURLEnv:   "postgres://admin:secret@database:5432/mm_chat_memory_regression_run1?sslmode=disable",
		runtimeDatabaseURLEnv: "postgres://admin:secret@database:5432/mm_chat_memory_regression_run1?sslmode=disable&role=go_api_runtime",
	}
	admin, runtime, err := loadDatabaseConfigs(mapEnvironment(valid))
	if err != nil || admin.Database != runtime.Database || runtime.RuntimeParams["role"] != "go_api_runtime" {
		t.Fatalf("valid database configs = %#v/%#v/%v", admin, runtime, err)
	}

	invalid := []map[string]string{
		{
			adminDatabaseURLEnv:   "postgres://admin:secret@database:5432/mm_chat?sslmode=disable",
			runtimeDatabaseURLEnv: "postgres://admin:secret@database:5432/mm_chat?sslmode=disable&role=go_api_runtime",
		},
		{
			adminDatabaseURLEnv:   valid[adminDatabaseURLEnv],
			runtimeDatabaseURLEnv: "postgres://admin:secret@other:5432/mm_chat_memory_regression_run1?sslmode=disable&role=go_api_runtime",
		},
		{
			adminDatabaseURLEnv:   valid[adminDatabaseURLEnv],
			runtimeDatabaseURLEnv: "postgres://admin:secret@database:5432/mm_chat_memory_regression_run1?sslmode=disable",
		},
	}
	for index, values := range invalid {
		if _, _, err := loadDatabaseConfigs(mapEnvironment(values)); err == nil {
			t.Fatalf("invalid database topology %d was accepted", index)
		}
	}
}

func TestBuildProvidersRequiresPrivateCredentialFileAndClearsBytes(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "credential")
	if err := os.WriteFile(path, []byte("fixture-siliconflow-key\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	options := commandOptions{
		providerMode:   memorycapture.ProviderModeLiveSiliconFlow,
		credentialPath: path,
	}
	if _, err := buildProviders(options); err == nil {
		t.Fatal("world-readable credential was accepted")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	bundle, err := buildProviders(options)
	if err != nil {
		t.Fatal(err)
	}
	if string(bundle.secret) != "fixture-siliconflow-key" {
		t.Fatalf("credential bytes = %q", bundle.secret)
	}
	bundle.clear()
	for _, current := range bundle.secret {
		if current != 0 {
			t.Fatal("credential bytes were not cleared")
		}
	}
}

func TestLoadLiveAuthorizationRequiresExactModelTargets(t *testing.T) {
	values := map[string]string{
		liveEnabledEnv: "true", liveApprovalEnv: memorycapture.LiveApproval,
		liveRunIDEnv: "run-1", liveProviderIDEnv: "siliconflow",
		liveEmbeddingModelEnv: ragproviders.SiliconFlowEmbeddingModel,
		liveRerankModelEnv:    ragproviders.SiliconFlowRerankModel,
	}
	authorization := loadLiveAuthorization(mapEnvironment(values))
	if err := memorycapture.AuthorizeProviderMode(
		memorycapture.ProviderModeLiveSiliconFlow,
		"run-1",
		authorization,
	); err != nil {
		t.Fatal(err)
	}
	values[liveRerankModelEnv] = "other"
	err := memorycapture.AuthorizeProviderMode(
		memorycapture.ProviderModeLiveSiliconFlow,
		"run-1",
		loadLiveAuthorization(mapEnvironment(values)),
	)
	if !errors.Is(err, memorycapture.ErrLiveNotAuthorized) || strings.Contains(err.Error(), "other") {
		t.Fatalf("target denial error = %v", err)
	}
}

func mapEnvironment(values map[string]string) environmentLookup {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
