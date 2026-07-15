package providersmoke

import (
	"errors"
	"strings"
	"testing"
)

func TestAuthorizeDefaultsFailClosed(t *testing.T) {
	cfg := LoadFromEnv(nil)

	err := cfg.Authorize(Target{Kind: KindImageGenerate, ProviderID: "openai", ModelID: "gpt-image-1"})

	assertAuthError(t, err, CodeDisabled)
}

func TestAuthorizeRequiresExactApproval(t *testing.T) {
	cfg := LoadFromEnv(mapGetenv(map[string]string{
		EnvEnabled:  "true",
		EnvApproval: "yes please",
		EnvTargets:  "image.generate:openai:gpt-image-1",
		EnvRunID:    "run-1",
	}))

	err := cfg.Authorize(Target{Kind: KindImageGenerate, ProviderID: "openai", ModelID: "gpt-image-1"})

	assertAuthError(t, err, CodeApprovalMissing)
}

func TestAuthorizeRequiresRunID(t *testing.T) {
	cfg := LoadFromEnv(mapGetenv(map[string]string{
		EnvEnabled:  "true",
		EnvApproval: RequiredApproval,
		EnvTargets:  "image.generate:openai:gpt-image-1",
	}))

	err := cfg.Authorize(Target{Kind: KindImageGenerate, ProviderID: "openai", ModelID: "gpt-image-1"})

	assertAuthError(t, err, CodeRunIDMissing)
}

func TestAuthorizeAllowsOnlyExactConfiguredTarget(t *testing.T) {
	cfg := LoadFromEnv(mapGetenv(map[string]string{
		EnvEnabled:  "true",
		EnvApproval: RequiredApproval,
		EnvTargets:  " image.generate:openai:gpt-image-1 , voice.synthesize:elevenlabs:tts-1 ",
		EnvRunID:    "run-20260715",
	}))

	if err := cfg.Authorize(Target{Kind: KindImageGenerate, ProviderID: "openai", ModelID: "gpt-image-1"}); err != nil {
		t.Fatalf("Authorize() allowed target error = %v", err)
	}
	if err := cfg.Authorize(Target{Kind: KindVoiceSynthesize, ProviderID: "elevenlabs", ModelID: "tts-1"}); err != nil {
		t.Fatalf("Authorize() second allowed target error = %v", err)
	}

	err := cfg.Authorize(Target{Kind: KindImageGenerate, ProviderID: "openai", ModelID: "other-model"})
	assertAuthError(t, err, CodeTargetDenied)
}

func TestAuthorizeRejectsMissingTarget(t *testing.T) {
	cfg := Config{Enabled: true, Approval: RequiredApproval, RunID: "run-1"}

	err := cfg.Authorize(Target{Kind: KindImageGenerate, ProviderID: "openai", ModelID: "gpt-image-1"})

	assertAuthError(t, err, CodeTargetDenied)
}

func TestAuthorizeErrorsDoNotLeakTargetValues(t *testing.T) {
	cfg := LoadFromEnv(mapGetenv(map[string]string{
		EnvEnabled:  "true",
		EnvApproval: RequiredApproval,
		EnvTargets:  "image.generate:openai:gpt-image-1",
		EnvRunID:    "run-1",
	}))

	err := cfg.Authorize(Target{Kind: KindImageGenerate, ProviderID: "sk-secret-provider", ModelID: "private-model"})

	assertAuthError(t, err, CodeTargetDenied)
	if strings.Contains(err.Error(), "sk-secret-provider") || strings.Contains(err.Error(), "private-model") {
		t.Fatalf("authorization error leaked target values: %v", err)
	}
}

func TestParseTargetsDropsMalformedAndUnknownKinds(t *testing.T) {
	targets := ParseTargets("bad,image.generate:openai:gpt-image-1,code.execute:python:runner,voice.transcribe:model:whisper")

	want := []Target{
		{Kind: KindImageGenerate, ProviderID: "openai", ModelID: "gpt-image-1"},
		{Kind: KindVoiceTranscribe, ProviderID: "model", ModelID: "whisper"},
	}
	if len(targets) != len(want) {
		t.Fatalf("targets = %#v, want %#v", targets, want)
	}
	for index := range want {
		if targets[index] != want[index] {
			t.Fatalf("target[%d] = %#v, want %#v", index, targets[index], want[index])
		}
	}
}

func TestLoadFromEnvSanitizesRunID(t *testing.T) {
	cfg := LoadFromEnv(mapGetenv(map[string]string{EnvRunID: " run/secret 1 "}))

	if cfg.RunID != "runsecret1" {
		t.Fatalf("RunID = %q, want runsecret1", cfg.RunID)
	}
}

func assertAuthError(t *testing.T, err error, code string) {
	t.Helper()
	if !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("Authorize() error = %v, want ErrNotAuthorized", err)
	}
	var authErr AuthorizationError
	if !errors.As(err, &authErr) {
		t.Fatalf("Authorize() error = %T, want AuthorizationError", err)
	}
	if authErr.Code != code {
		t.Fatalf("AuthorizationError.Code = %q, want %q", authErr.Code, code)
	}
}

func mapGetenv(values map[string]string) Getenv {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
