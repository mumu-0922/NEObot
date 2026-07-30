package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/memorycapture"
	"neo-chat/mm-chat/backend/internal/memoryeval"
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
		"-capture-mode", memorycapture.CaptureModeCalibration,
		"-credential-file", "/secret",
	)
	options, err = parseCommand(live)
	if err != nil || options.credentialPath != "/secret" {
		t.Fatalf("parse live command = %#v/%v", options, err)
	}
	if _, err := parseCommand(append(append([]string{}, base...),
		"-provider-mode", memorycapture.ProviderModeLiveSiliconFlow,
		"-capture-mode", memorycapture.CaptureModeCalibration)); err == nil {
		t.Fatal("live mode omitted credential file")
	}
	if _, err := parseCommand(append(append([]string{}, base...),
		"-provider-mode", memorycapture.ProviderModeLiveSiliconFlow,
		"-credential-file", "/secret")); err == nil {
		t.Fatal("live mode accepted the split-unsafe full regression default")
	}
	cloud := append(append([]string{}, base...),
		"-capture-mode", memorycapture.CaptureModeCloudJudgeDevelopment,
		"-cloud-judge-model", "Pro/test/Memory-Judge",
	)
	options, err = parseCommand(cloud)
	if err != nil || options.judgeModelID != "Pro/test/Memory-Judge" {
		t.Fatalf("parse cloud-judge command = %#v/%v", options, err)
	}
	route := append(append([]string{}, base...),
		"-capture-mode", memorycapture.CaptureModeMemoryToolRouteDevelopment,
		"-memory-tool-route-provider-id", "configured-deepseek",
		"-memory-tool-route-provider-type", "openai_compatible",
		"-memory-tool-route-base-url", "https://api.deepseek.example/v1/",
		"-memory-tool-route-model", "deepseek-chat",
	)
	options, err = parseCommand(route)
	if err != nil || options.routeProviderID != "configured-deepseek" ||
		options.routeCredentialPath != "" {
		t.Fatalf("parse fake Memory Tool-route command = %#v/%v", options, err)
	}
	liveRoute := append(append([]string{}, route...),
		"-provider-mode", memorycapture.ProviderModeLiveSiliconFlow,
		"-credential-file", "/siliconflow-secret",
		"-memory-tool-route-credential-file", "/deepseek-secret",
	)
	options, err = parseCommand(liveRoute)
	if err != nil || options.routeCredentialPath != "/deepseek-secret" {
		t.Fatalf("parse live Memory Tool-route command = %#v/%v", options, err)
	}
	if _, err := parseCommand(append(append([]string{}, route...),
		"-provider-mode", memorycapture.ProviderModeLiveSiliconFlow,
		"-credential-file", "/siliconflow-secret")); err == nil {
		t.Fatal("live Memory Tool-route command omitted its independent credential")
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
	if len(bundle.secrets) != 1 || string(bundle.secrets[0]) != "fixture-siliconflow-key" {
		t.Fatalf("credential bytes = %q", bundle.secrets)
	}
	bundle.clear()
	for _, current := range bundle.secrets[0] {
		if current != 0 {
			t.Fatal("credential bytes were not cleared")
		}
	}
}

func TestBuildProvidersBindsIndependentMemoryToolRouteCredential(t *testing.T) {
	directory := t.TempDir()
	retrievalPath := filepath.Join(directory, "retrieval.key")
	routePath := filepath.Join(directory, "route.key")
	if err := os.WriteFile(retrievalPath, []byte("fixture-siliconflow-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(routePath, []byte("fixture-deepseek-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := commandOptions{
		providerMode:        memorycapture.ProviderModeLiveSiliconFlow,
		captureMode:         memorycapture.CaptureModeMemoryToolRouteDevelopment,
		credentialPath:      retrievalPath,
		routeCredentialPath: routePath,
		routeProviderID:     "configured-deepseek",
		routeProviderType:   "openai_compatible",
		routeBaseURL:        "https://api.deepseek.example/",
		routeModelID:        "deepseek-chat",
	}
	bundle, err := buildProviders(options)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.router == nil || len(bundle.secrets) != 2 ||
		string(bundle.secrets[0]) != "fixture-siliconflow-key" ||
		string(bundle.secrets[1]) != "fixture-deepseek-key" {
		t.Fatalf("Memory Tool-route Provider bundle = %#v", bundle)
	}
	bundle.clear()
	for _, secret := range bundle.secrets {
		for _, current := range secret {
			if current != 0 {
				t.Fatal("a Provider credential byte slice was not cleared")
			}
		}
	}

	options.routeCredentialPath = retrievalPath
	if _, err := buildProviders(options); err == nil {
		t.Fatal("Memory Tool-route accepted the retrieval credential file")
	}
}

func TestBuildMemoryToolRouteAuthorityNormalizesExactEndpoint(t *testing.T) {
	options := commandOptions{
		routeProviderID:   "configured-gpt",
		routeProviderType: "OpenAI-Compatible",
		routeBaseURL:      "https://gateway.example/api/",
		routeModelID:      "gpt-test",
	}
	authority, providerType, baseURL, err := resolveMemoryToolRoute(options)
	if err != nil {
		t.Fatal(err)
	}
	if authority.ProviderID != "configured-gpt" ||
		authority.ProviderType != "openai_compatible" ||
		providerType != "OpenAI Compatible" ||
		baseURL != "https://gateway.example/api/v1" ||
		authority.BaseURLSHA256 != sha256Bytes([]byte(baseURL)) {
		t.Fatalf("Memory Tool-route authority = %#v/%q/%q", authority, providerType, baseURL)
	}
	options.routeBaseURL = "https://user:secret@gateway.example/v1"
	if _, _, _, err := resolveMemoryToolRoute(options); err == nil {
		t.Fatal("Memory Tool-route accepted URL userinfo")
	}
}

func TestLoadLiveAuthorizationRequiresExactModelTargets(t *testing.T) {
	values := map[string]string{
		liveEnabledEnv: "true", liveApprovalEnv: memorycapture.LiveApproval,
		liveRunIDEnv: "run-1", liveProviderIDEnv: "siliconflow",
		liveEmbeddingModelEnv:               ragproviders.SiliconFlowEmbeddingModel,
		liveRerankModelEnv:                  ragproviders.SiliconFlowRerankModel,
		liveCloudJudgeModelEnv:              memorycapture.DefaultSiliconFlowCloudJudgeModelID,
		liveMemoryToolRouteApprovalEnv:      memorycapture.LiveMemoryToolRouteApproval,
		liveMemoryToolRouteProviderIDEnv:    "configured-gpt",
		liveMemoryToolRouteProviderTypeEnv:  "openai_compatible",
		liveMemoryToolRouteBaseURLSHA256Env: strings.Repeat("b", 64),
		liveMemoryToolRouteModelIDEnv:       "gpt-test",
	}
	authorization := loadLiveAuthorization(mapEnvironment(values))
	if err := memorycapture.AuthorizeProviderMode(
		memorycapture.ProviderModeLiveSiliconFlow,
		"run-1",
		authorization,
	); err != nil {
		t.Fatal(err)
	}
	if err := memorycapture.AuthorizeCloudJudgeTarget(
		memorycapture.ProviderModeLiveSiliconFlow,
		memorycapture.DefaultSiliconFlowCloudJudgeModelID,
		authorization,
	); err != nil {
		t.Fatal(err)
	}
	if err := memorycapture.AuthorizeMemoryToolRouteTarget(
		memorycapture.ProviderModeLiveSiliconFlow,
		memorycapture.MemoryToolRouteProfileAuthority{
			ProviderID: "configured-gpt", ProviderType: "openai_compatible",
			BaseURLSHA256: strings.Repeat("b", 64), ModelID: "gpt-test",
		},
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

func TestNewMemoryToolRouteCommandSummaryUsesFirstRoundReportAuthority(t *testing.T) {
	options := commandOptions{
		runID:        "run-1",
		providerMode: memorycapture.ProviderModeFakeProtocol,
		outputDir:    "/private/output/../run-1",
	}
	report := memorycapture.MemoryToolRouteDevelopmentReport{
		CorpusClass:       memoryeval.RegressionCorpusClass,
		AdmissionMode:     memorycapture.MemoryToolFirstRoundDevelopmentAdmissionMode,
		PromotionEligible: false,
		Split:             memorycapture.DevelopmentCalibrationSplit,
		Passed:            false,
	}

	summary := newMemoryToolRouteCommandSummary(options, "capture-1", report)
	if summary.SchemaVersion != "neo-chat.memory-regression-native-summary.v4" ||
		summary.AdmissionMode != memorycapture.MemoryToolFirstRoundDevelopmentAdmissionMode ||
		summary.PromotionEligible || summary.CandidatePassed || summary.PolicySelected {
		t.Fatalf("failed first-round summary = %#v", summary)
	}
	if summary.RunID != options.runID || summary.CaptureID != "capture-1" ||
		summary.CorpusClass != memoryeval.RegressionCorpusClass ||
		summary.ProviderMode != memorycapture.ProviderModeFakeProtocol ||
		summary.CaptureMode != memorycapture.CaptureModeMemoryToolRouteDevelopment ||
		summary.Split != memorycapture.DevelopmentCalibrationSplit ||
		summary.OutputDirectory != filepath.Clean(options.outputDir) {
		t.Fatalf("first-round summary authority = %#v", summary)
	}
}

func mapEnvironment(values map[string]string) environmentLookup {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
