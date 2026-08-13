package agentorchestrator

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"neo-chat/mm-chat/backend/internal/strictjson"
)

const (
	maxSnapshotBytes = 64 << 10
	maxSteps         = 256
	maxDetailBytes   = 4096
)

var (
	prefixedIDPattern     = regexp.MustCompile(`^(run|snapshot|step|attempt|event|switch)_[a-z0-9]{16,64}$`)
	reasonPattern         = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	actorTypePattern      = regexp.MustCompile(`^(user|orchestrator|runner|scheduler|operator)$`)
	lowCardinalityPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:-]{0,127}$`)
	fingerprintPattern    = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	scopeTypes            = set("global", "scheduler", "runner", "source", "admission", "skill", "tool", "capability", "action", "egress", "secret", "project", "user", "run")
	detailKeys            = set("observationCode", "classification", "count", "durationMs", "byteCount", "fingerprint", "outcome", "conflictState")
)

type Service struct {
	repository Repository
	newID      func(string) string
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{
		repository: repository,
		newID: func(prefix string) string {
			return prefix + "_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		},
		now: time.Now,
	}
}

func (service *Service) EnqueueRun(ctx context.Context, input EnqueueInput) (EnqueueResult, error) {
	if service == nil || service.repository == nil {
		return EnqueueResult{}, ErrDatabaseRequired
	}
	prepared, err := service.prepareEnqueue(input)
	if err != nil {
		return EnqueueResult{}, err
	}
	runID, created, err := service.repository.EnqueueRun(ctx, prepared)
	if err != nil {
		return EnqueueResult{}, err
	}
	run, err := service.repository.GetRun(ctx, input.UserID, runID)
	return EnqueueResult{Run: run, Created: created}, err
}

func (service *Service) GetRun(ctx context.Context, userID, runID string) (Run, error) {
	if service == nil || service.repository == nil {
		return Run{}, ErrDatabaseRequired
	}
	if !validUUID(userID) || !validID(runID, "run") {
		return Run{}, ErrInvalidInput
	}
	return service.repository.GetRun(ctx, userID, runID)
}

func (service *Service) TransitionRun(ctx context.Context, input TransitionInput) error {
	if service == nil || service.repository == nil {
		return ErrDatabaseRequired
	}
	if err := validateTransitionInput(input, "run"); err != nil || !ValidRunTransition(input.Expected, input.To) {
		return ErrInvalidTransition
	}
	input.EventID = service.newID("event")
	return service.repository.TransitionRun(ctx, input)
}

func (service *Service) TransitionStep(ctx context.Context, input TransitionInput) error {
	if service == nil || service.repository == nil {
		return ErrDatabaseRequired
	}
	if err := validateTransitionInput(input, "step"); err != nil || !ValidStepTransition(input.Expected, input.To) {
		return ErrInvalidTransition
	}
	input.EventID = service.newID("event")
	return service.repository.TransitionStep(ctx, input)
}

func (service *Service) TransitionAttempt(ctx context.Context, input TransitionInput) error {
	if service == nil || service.repository == nil {
		return ErrDatabaseRequired
	}
	if err := validateTransitionInput(input, "attempt"); err != nil || !ValidAttemptTransition(input.Expected, input.To) {
		return ErrInvalidTransition
	}
	input.EventID = service.newID("event")
	return service.repository.TransitionAttempt(ctx, input)
}

func (service *Service) ObserveTerminalConflict(ctx context.Context, input TransitionInput) error {
	if service == nil || service.repository == nil {
		return ErrDatabaseRequired
	}
	if err := validateTransitionInput(input, "observation"); err != nil || TerminalRank(input.To) == 0 {
		return ErrInvalidInput
	}
	input.EventID = service.newID("event")
	return service.repository.ObserveTerminalConflict(ctx, input)
}

func (service *Service) AcquireStep(ctx context.Context, input AcquireInput) (Lease, error) {
	if service == nil || service.repository == nil {
		return Lease{}, ErrDatabaseRequired
	}
	if !validUUID(input.UserID) || !validID(input.RunID, "run") || !validID(input.StepID, "step") ||
		!validActor(input.Actor) || !validReason(input.ReasonCode) ||
		strings.TrimSpace(input.LeaseOwner) != input.LeaseOwner || len(input.LeaseOwner) < 1 || len(input.LeaseOwner) > 128 ||
		input.LeaseDuration < 5*time.Second || input.LeaseDuration > time.Hour {
		return Lease{}, ErrInvalidInput
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return Lease{}, fmt.Errorf("create lease credential: %w", err)
	}
	// The protocol-visible credential is namespaced so it cannot be confused
	// with another opaque bearer value. Only its digest reaches PostgreSQL.
	rawToken := "lease_" + base64.RawURLEncoding.EncodeToString(token)
	digest := sha256.Sum256([]byte(rawToken))
	prepared := preparedLease{
		AttemptID: service.newID("attempt"), Token: rawToken, TokenHash: hex.EncodeToString(digest[:]),
		EventIDs: []string{service.newID("event"), service.newID("event"), service.newID("event"), service.newID("event")},
	}
	return service.repository.AcquireStep(ctx, input, prepared)
}

func (service *Service) HeartbeatAttempt(ctx context.Context, input HeartbeatInput) (time.Time, error) {
	if service == nil || service.repository == nil {
		return time.Time{}, ErrDatabaseRequired
	}
	if !validUUID(input.UserID) || !validID(input.RunID, "run") || !validID(input.StepID, "step") ||
		!validID(input.AttemptID, "attempt") || input.Generation < 1 || !validActor(input.Actor) ||
		!validReason(input.ReasonCode) || input.LeaseToken == "" || input.LeaseOwner == "" ||
		input.LeaseDuration < 5*time.Second || input.LeaseDuration > time.Hour {
		return time.Time{}, ErrInvalidInput
	}
	digest := sha256.Sum256([]byte(input.LeaseToken))
	input.EventID = service.newID("event")
	return service.repository.HeartbeatAttempt(ctx, input, hex.EncodeToString(digest[:]))
}

func (service *Service) ListRecoveryRuns(ctx context.Context, limit int) ([]RecoveryRun, error) {
	if service == nil || service.repository == nil {
		return nil, ErrDatabaseRequired
	}
	if limit < 1 || limit > 1000 {
		return nil, ErrInvalidInput
	}
	return service.repository.ListRecoveryRuns(ctx, limit)
}

func (service *Service) RebuildProjection(ctx context.Context, userID, runID string) error {
	if service == nil || service.repository == nil {
		return ErrDatabaseRequired
	}
	if !validUUID(userID) || !validID(runID, "run") {
		return ErrInvalidInput
	}
	return service.repository.RebuildProjection(ctx, userID, runID)
}

func (service *Service) AppendKillSwitch(ctx context.Context, input KillSwitchInput) (KillSwitch, error) {
	if service == nil || service.repository == nil {
		return KillSwitch{}, ErrDatabaseRequired
	}
	if input.ID == "" {
		input.ID = service.newID("switch")
	}
	if !validID(input.ID, "switch") || !validScope(input.ScopeType, input.ScopeValue) ||
		(input.Mode != KillDenyNew && input.Mode != KillCancel && input.Mode != KillKill) ||
		input.ExpectedRevision < 0 || !validActor(input.Actor) || !validReason(input.ReasonCode) {
		return KillSwitch{}, ErrInvalidInput
	}
	return service.repository.AppendKillSwitch(ctx, input)
}

func (service *Service) ResolveKillSwitch(ctx context.Context, userID, runID string) (KillResolution, error) {
	if service == nil || service.repository == nil {
		return KillResolution{}, ErrDatabaseRequired
	}
	if !validUUID(userID) || !validID(runID, "run") {
		return KillResolution{}, ErrInvalidInput
	}
	return service.repository.ResolveKillSwitch(ctx, userID, runID)
}

func (service *Service) PruneTerminalRuns(ctx context.Context, cutoff time.Time, limit int) (int, error) {
	if service == nil || service.repository == nil {
		return 0, ErrDatabaseRequired
	}
	if cutoff.IsZero() || cutoff.After(service.now().Add(time.Minute)) || limit < 1 || limit > 1000 {
		return 0, ErrInvalidInput
	}
	return service.repository.PruneTerminalRuns(ctx, cutoff, limit)
}

func (service *Service) prepareEnqueue(input EnqueueInput) (preparedEnqueue, error) {
	if !validUUID(input.UserID) || strings.TrimSpace(input.IdempotencyKey) != input.IdempotencyKey ||
		len(input.IdempotencyKey) < 1 || len(input.IdempotencyKey) > 256 || len(input.Steps) < 1 || len(input.Steps) > maxSteps {
		return preparedEnqueue{}, ErrInvalidInput
	}
	canonical, err := canonicalSnapshot(input.Snapshot)
	if err != nil {
		return preparedEnqueue{}, err
	}
	input.Steps = append([]StepPlan(nil), input.Steps...)
	requestSteps := make([]string, len(input.Steps))
	requestScopes := make([]string, 0, len(input.ScopeBindings))
	seenSteps := make(map[string]struct{}, len(input.Steps))
	for index := range input.Steps {
		requestSteps[index] = input.Steps[index].Kind
		if input.Steps[index].ID == "" {
			input.Steps[index].ID = service.newID("step")
		}
		step := input.Steps[index]
		if !validID(step.ID, "step") || strings.TrimSpace(step.Kind) != step.Kind || len(step.Kind) < 1 || len(step.Kind) > 64 {
			return preparedEnqueue{}, ErrInvalidInput
		}
		if _, duplicate := seenSteps[step.ID]; duplicate {
			return preparedEnqueue{}, ErrInvalidInput
		}
		seenSteps[step.ID] = struct{}{}
	}
	runID := service.newID("run")
	scopeKeys := []string{"global:*", "scheduler:*", "user:" + input.UserID, "run:" + runID}
	seenScopes := map[string]struct{}{"global:*": {}, "scheduler:*": {}, "user:" + input.UserID: {}, "run:" + runID: {}}
	for _, binding := range input.ScopeBindings {
		if binding.Type == "global" || binding.Type == "scheduler" || binding.Type == "user" || binding.Type == "run" || !validScope(binding.Type, binding.Value) {
			return preparedEnqueue{}, ErrInvalidInput
		}
		key := binding.Type + ":" + binding.Value
		requestScopes = append(requestScopes, key)
		if _, duplicate := seenScopes[key]; !duplicate {
			scopeKeys = append(scopeKeys, key)
			seenScopes[key] = struct{}{}
		}
	}
	sort.Strings(scopeKeys)
	sort.Strings(requestScopes)
	stepsJSON, _ := json.Marshal(input.Steps)
	snapshotDigest := sha256.Sum256(append([]byte("neo-agent-snapshot-v1\x00"), canonical...))
	requestBody, _ := json.Marshal(struct {
		Snapshot json.RawMessage `json:"snapshot"`
		Steps    []string        `json:"steps"`
		Scopes   []string        `json:"scopes"`
	}{canonical, requestSteps, requestScopes})
	requestDigest := sha256.Sum256(append([]byte("neo-agent-enqueue-v1\x00"), requestBody...))
	eventIDs := make([]string, 3+2*len(input.Steps))
	for index := range eventIDs {
		eventIDs[index] = service.newID("event")
	}
	return preparedEnqueue{
		Input: input, RunID: runID, SnapshotID: service.newID("snapshot"),
		SnapshotFingerprint: "sha256:" + hex.EncodeToString(snapshotDigest[:]),
		RequestFingerprint:  "sha256:" + hex.EncodeToString(requestDigest[:]),
		CanonicalSnapshot:   canonical, StepsJSON: stepsJSON, ScopeKeys: scopeKeys, EventIDs: eventIDs,
	}, nil
}

func canonicalSnapshot(body []byte) ([]byte, error) {
	var value map[string]any
	if err := strictjson.Decode(body, maxSnapshotBytes, &value); err != nil || value == nil || containsForbiddenSnapshotKey(value) {
		return nil, ErrInvalidInput
	}
	canonical, err := json.Marshal(value)
	if err != nil || len(canonical) > maxSnapshotBytes {
		return nil, ErrInvalidInput
	}
	return canonical, nil
}

func containsForbiddenSnapshotKey(value any) bool {
	forbidden := set("prompt", "systemprompt", "skillbody", "workspacecontent", "toolarguments", "toolresults", "stdout", "stderr", "secretvalue", "leasetoken", "authorization", "apikey", "password", "accesstoken", "refreshtoken", "credential")
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalizedKey := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
			_, denied := forbidden[normalizedKey]
			if denied || strings.Contains(normalizedKey, "password") ||
				strings.Contains(normalizedKey, "authorization") || containsForbiddenSnapshotKey(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsForbiddenSnapshotKey(child) {
				return true
			}
		}
	}
	return false
}

func validateTransitionInput(input TransitionInput, entity string) error {
	if !validUUID(input.UserID) || !validID(input.RunID, "run") || !validActor(input.Actor) || !validReason(input.ReasonCode) ||
		input.Expected == "" || input.To == "" || validateDetail(input.Detail) != nil {
		return ErrInvalidInput
	}
	if entity == "step" && !validID(input.StepID, "step") {
		return ErrInvalidInput
	}
	if entity == "step" && input.Expected == StepRunning &&
		(!validID(input.AttemptID, "attempt") || input.Generation < 1 ||
			input.LeaseOwner == "" || input.LeaseToken == "") {
		return ErrInvalidInput
	}
	if entity == "attempt" && (!validID(input.StepID, "step") || !validID(input.AttemptID, "attempt") || input.Generation < 1 || input.LeaseOwner == "" || input.LeaseToken == "") {
		return ErrInvalidInput
	}
	if entity == "observation" && input.StepID != "" && !validID(input.StepID, "step") {
		return ErrInvalidInput
	}
	return nil
}

func validateDetail(detail map[string]any) error {
	if len(detail) > 8 {
		return ErrInvalidInput
	}
	for key, value := range detail {
		if _, ok := detailKeys[key]; !ok {
			return ErrInvalidInput
		}
		switch typed := value.(type) {
		case nil, bool:
		case float64:
			if typed < 0 || typed > 1<<53 {
				return ErrInvalidInput
			}
		case int:
			if typed < 0 {
				return ErrInvalidInput
			}
		case int64:
			if typed < 0 {
				return ErrInvalidInput
			}
		case string:
			if key == "fingerprint" {
				if !fingerprintPattern.MatchString(typed) {
					return ErrInvalidInput
				}
			} else if !lowCardinalityPattern.MatchString(typed) {
				return ErrInvalidInput
			}
		default:
			return ErrInvalidInput
		}
	}
	encoded, err := json.Marshal(detail)
	if err != nil || len(encoded) > maxDetailBytes {
		return ErrInvalidInput
	}
	return nil
}

func validID(value, prefix string) bool {
	return strings.HasPrefix(value, prefix+"_") && prefixedIDPattern.MatchString(value)
}

func validUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == strings.ToLower(value)
}

func validActor(actor Actor) bool {
	return actorTypePattern.MatchString(actor.Type) && strings.TrimSpace(actor.ID) == actor.ID && len(actor.ID) >= 1 && len(actor.ID) <= 128
}

func validReason(value string) bool { return reasonPattern.MatchString(value) }

func validScope(scopeType, scopeValue string) bool {
	_, ok := scopeTypes[scopeType]
	return ok && strings.TrimSpace(scopeValue) == scopeValue && len(scopeValue) >= 1 && len(scopeValue) <= 512 && !strings.ContainsRune(scopeValue, '\x00')
}
