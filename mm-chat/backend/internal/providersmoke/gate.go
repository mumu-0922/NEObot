package providersmoke

import (
	"errors"
	"fmt"
	"strings"
)

const (
	EnvEnabled  = "MM_CHAT_PROVIDER_LIVE_SMOKE_ENABLED"
	EnvApproval = "MM_CHAT_PROVIDER_LIVE_SMOKE_APPROVAL"
	EnvTargets  = "MM_CHAT_PROVIDER_LIVE_SMOKE_TARGETS"
	EnvRunID    = "MM_CHAT_PROVIDER_LIVE_SMOKE_RUN_ID"

	RequiredApproval = "I_UNDERSTAND_THIS_USES_REAL_PROVIDER_QUOTA"
)

type Kind string

const (
	KindVoiceTranscribe Kind = "voice.transcribe"
	KindVoiceSynthesize Kind = "voice.synthesize"
	KindImageGenerate   Kind = "image.generate"
)

const (
	CodeDisabled        = "PROVIDER_LIVE_SMOKE_DISABLED"
	CodeApprovalMissing = "PROVIDER_LIVE_SMOKE_APPROVAL_REQUIRED"
	CodeTargetMissing   = "PROVIDER_LIVE_SMOKE_TARGET_REQUIRED"
	CodeTargetDenied    = "PROVIDER_LIVE_SMOKE_TARGET_NOT_AUTHORIZED"
	CodeRunIDMissing    = "PROVIDER_LIVE_SMOKE_RUN_ID_REQUIRED"
)

var ErrNotAuthorized = errors.New("provider live smoke is not authorized")

type Config struct {
	Enabled  bool
	Approval string
	Targets  []Target
	RunID    string
}

type Target struct {
	Kind       Kind
	ProviderID string
	ModelID    string
}

type AuthorizationError struct {
	Code string
}

func (e AuthorizationError) Error() string {
	if e.Code == "" {
		return ErrNotAuthorized.Error()
	}
	return fmt.Sprintf("%s: %s", ErrNotAuthorized, e.Code)
}

func (e AuthorizationError) Unwrap() error {
	return ErrNotAuthorized
}

type Getenv func(string) (string, bool)

func LoadFromEnv(getenv Getenv) Config {
	if getenv == nil {
		getenv = func(string) (string, bool) { return "", false }
	}
	enabled, _ := getenv(EnvEnabled)
	approval, _ := getenv(EnvApproval)
	targets, _ := getenv(EnvTargets)
	runID, _ := getenv(EnvRunID)
	return Config{
		Enabled:  parseBool(enabled),
		Approval: strings.TrimSpace(approval),
		Targets:  ParseTargets(targets),
		RunID:    sanitizeRunID(runID),
	}
}

func (cfg Config) Authorize(target Target) error {
	target = NormalizeTarget(target)
	if !cfg.Enabled {
		return AuthorizationError{Code: CodeDisabled}
	}
	if cfg.Approval != RequiredApproval {
		return AuthorizationError{Code: CodeApprovalMissing}
	}
	if cfg.RunID == "" {
		return AuthorizationError{Code: CodeRunIDMissing}
	}
	if target == (Target{}) {
		return AuthorizationError{Code: CodeTargetMissing}
	}
	for _, allowed := range cfg.Targets {
		if NormalizeTarget(allowed) == target {
			return nil
		}
	}
	return AuthorizationError{Code: CodeTargetDenied}
}

func ParseTargets(value string) []Target {
	parts := strings.Split(value, ",")
	targets := make([]Target, 0, len(parts))
	for _, part := range parts {
		fields := strings.Split(strings.TrimSpace(part), ":")
		if len(fields) != 3 {
			continue
		}
		target := NormalizeTarget(Target{
			Kind:       Kind(fields[0]),
			ProviderID: fields[1],
			ModelID:    fields[2],
		})
		if target != (Target{}) {
			targets = append(targets, target)
		}
	}
	return targets
}

func NormalizeTarget(target Target) Target {
	target.Kind = Kind(strings.ToLower(strings.TrimSpace(string(target.Kind))))
	target.ProviderID = strings.TrimSpace(target.ProviderID)
	target.ModelID = strings.TrimSpace(target.ModelID)
	if !isKnownKind(target.Kind) || target.ProviderID == "" || target.ModelID == "" {
		return Target{}
	}
	return target
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func isKnownKind(kind Kind) bool {
	switch kind {
	case KindVoiceTranscribe, KindVoiceSynthesize, KindImageGenerate:
		return true
	default:
		return false
	}
}

func sanitizeRunID(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	for _, current := range value {
		if current >= 'a' && current <= 'z' ||
			current >= 'A' && current <= 'Z' ||
			current >= '0' && current <= '9' ||
			current == '-' || current == '_' || current == '.' {
			builder.WriteRune(current)
		}
	}
	return builder.String()
}
