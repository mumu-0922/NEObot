package agentbroker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

type SecretBinding struct {
	Subject                Subject `json:"subject"`
	RunID                  string  `json:"runId"`
	StepID                 string  `json:"stepId"`
	AttemptID              string  `json:"attemptId"`
	Generation             int64   `json:"generation"`
	Capability             string  `json:"capability"`
	Action                 string  `json:"action"`
	DestinationFingerprint string  `json:"destinationFingerprint"`
	IntentFingerprint      string  `json:"intentFingerprint"`
}

type SecretHandleRecord struct {
	HandleDigest       string
	IntentID           string
	SecretRef          string
	BindingFingerprint string
	State              string
	ExpiresAt          time.Time
}

type SecretHandleRepository interface {
	CreateSecretHandle(context.Context, SecretHandleRecord, SecretBinding) error
	ConsumeSecretHandle(context.Context, string, string, time.Time) error
	RevokeSecretHandle(context.Context, string, time.Time) error
	RevokeIntentHandles(context.Context, string, time.Time) (int, error)
}

type SecretResolver interface {
	Resolve(context.Context, string, Subject) ([]byte, error)
}

type SecretBroker struct {
	repository SecretHandleRepository
	resolver   SecretResolver
	now        func() time.Time
	opMu       sync.RWMutex
	mu         sync.Mutex
	handles    map[string]secretHandle
}

type secretHandle struct {
	IntentID           string
	GrantID            string
	BindingFingerprint string
	SecretRef          string
	Value              []byte
	ExpiresAt          time.Time
}

func NewSecretBroker(repository SecretHandleRepository, resolver SecretResolver) (*SecretBroker, error) {
	if repository == nil || resolver == nil {
		return nil, ErrInvalidInput
	}
	return &SecretBroker{repository: repository, resolver: resolver, now: time.Now,
		handles: map[string]secretHandle{}}, nil
}

func (broker *SecretBroker) Issue(ctx context.Context, intent PreparedIntent, registry ToolRegistry,
	secretRef string, binding SecretBinding, ttl time.Duration) (string, error) {
	if broker == nil || broker.repository == nil || broker.resolver == nil ||
		!secretAuthorityAllows(intent, registry, secretRef, binding, ttl, broker.now().UTC()) {
		return "", ErrSecretDenied
	}
	broker.opMu.RLock()
	defer broker.opMu.RUnlock()
	now := broker.now().UTC()
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", ErrExecutorUnavailable
	}
	handle := "secret_handle_" + base64.RawURLEncoding.EncodeToString(raw)
	handleDigest := digestSecretHandle(handle)
	bindingFingerprint := secretBindingFingerprint(binding)
	expiresAt := now.Add(ttl)
	if err := broker.repository.CreateSecretHandle(ctx, SecretHandleRecord{HandleDigest: handleDigest,
		IntentID: intent.IntentID, SecretRef: secretRef, BindingFingerprint: bindingFingerprint,
		State: "active", ExpiresAt: expiresAt}, binding); err != nil {
		return "", err
	}
	value, err := broker.resolver.Resolve(ctx, secretRef, binding.Subject)
	if err != nil || len(value) == 0 || len(value) > 64<<10 {
		if value != nil {
			clear(value)
		}
		_ = broker.repository.RevokeSecretHandle(context.WithoutCancel(ctx), handleDigest, broker.now().UTC())
		return "", ErrSecretDenied
	}
	defer clear(value)
	broker.mu.Lock()
	broker.handles[handleDigest] = secretHandle{IntentID: intent.IntentID, GrantID: intent.GrantID, SecretRef: secretRef,
		BindingFingerprint: bindingFingerprint, Value: append([]byte(nil), value...), ExpiresAt: expiresAt}
	broker.mu.Unlock()
	return handle, nil
}

func secretAuthorityAllows(intent PreparedIntent, registry ToolRegistry, secretRef string,
	binding SecretBinding, ttl time.Duration, now time.Time) bool {
	if intent.State != IntentCommitting || !now.Before(intent.ExpiresAt) ||
		!secretRefPattern.MatchString(secretRef) || !validSecretBinding(binding) ||
		ttl < time.Second || ttl > time.Hour || now.Add(ttl).After(intent.ExpiresAt) ||
		registry.Fingerprint != intent.RegistryFingerprint || registry.Subject != intent.Subject ||
		registry.RunID != intent.RunID || registry.GrantID != intent.GrantID ||
		binding.Subject != intent.Subject || binding.RunID != intent.RunID || binding.StepID != intent.StepID ||
		binding.AttemptID != intent.AttemptID || binding.Generation != intent.Generation ||
		binding.Capability != intent.Capability || binding.Action != intent.Action ||
		binding.IntentFingerprint != intent.IntentFingerprint {
		return false
	}
	tool, err := RegistryToolFor(registry, intent.ToolIdentity, intent.Action, intent.Resource)
	if err != nil || tool.Capability != intent.Capability {
		return false
	}
	for _, secret := range registry.Secrets {
		if secret.BrokerRef == secretRef && ttl <= time.Duration(secret.TTLSeconds)*time.Second {
			for _, action := range secret.Actions {
				if action == intent.Action {
					return true
				}
			}
		}
	}
	return false
}

func (broker *SecretBroker) Consume(ctx context.Context, handle string, binding SecretBinding) ([]byte, error) {
	if broker == nil || !validSecretBinding(binding) {
		return nil, ErrSecretDenied
	}
	digest := digestSecretHandle(handle)
	broker.mu.Lock()
	stored, ok := broker.handles[digest]
	if !ok || stored.BindingFingerprint != secretBindingFingerprint(binding) {
		broker.mu.Unlock()
		return nil, ErrSecretDenied
	}
	if !broker.now().UTC().Before(stored.ExpiresAt) {
		delete(broker.handles, digest)
		clear(stored.Value)
		broker.mu.Unlock()
		return nil, ErrSecretDenied
	}
	delete(broker.handles, digest)
	value := append([]byte(nil), stored.Value...)
	clear(stored.Value)
	broker.mu.Unlock()
	if err := broker.repository.ConsumeSecretHandle(ctx, digest, stored.BindingFingerprint, broker.now().UTC()); err != nil {
		clear(value)
		return nil, err
	}
	return value, nil
}

func (broker *SecretBroker) RevokeIntent(ctx context.Context, intentID string) (int, error) {
	if broker == nil || !validID(intentID, "intent") {
		return 0, ErrSecretDenied
	}
	broker.opMu.Lock()
	defer broker.opMu.Unlock()
	broker.mu.Lock()
	broker.clearIntentLocked(intentID)
	broker.mu.Unlock()
	return broker.repository.RevokeIntentHandles(ctx, intentID, broker.now().UTC())
}

func (broker *SecretBroker) revokeGrant(grantID string) int {
	broker.opMu.Lock()
	defer broker.opMu.Unlock()
	broker.mu.Lock()
	defer broker.mu.Unlock()
	changed := 0
	for digest, stored := range broker.handles {
		if stored.GrantID == grantID {
			clear(stored.Value)
			delete(broker.handles, digest)
			changed++
		}
	}
	return changed
}

func (broker *SecretBroker) clearIntent(intentID string) int {
	broker.opMu.Lock()
	defer broker.opMu.Unlock()
	broker.mu.Lock()
	defer broker.mu.Unlock()
	return broker.clearIntentLocked(intentID)
}

func (broker *SecretBroker) clearIntentLocked(intentID string) int {
	changed := 0
	for digest, stored := range broker.handles {
		if stored.IntentID == intentID {
			clear(stored.Value)
			delete(broker.handles, digest)
			changed++
		}
	}
	return changed
}

func (broker *SecretBroker) clearExpired(cutoff time.Time) int {
	broker.opMu.Lock()
	defer broker.opMu.Unlock()
	broker.mu.Lock()
	defer broker.mu.Unlock()
	changed := 0
	for digest, stored := range broker.handles {
		if !stored.ExpiresAt.After(cutoff) {
			clear(stored.Value)
			delete(broker.handles, digest)
			changed++
		}
	}
	return changed
}

func validSecretBinding(binding SecretBinding) bool {
	return validateSubject(binding.Subject) && validID(binding.RunID, "run") && validID(binding.StepID, "step") &&
		validID(binding.AttemptID, "attempt") && binding.Generation >= 1 && validIdentifier(binding.Capability) &&
		validIdentifier(binding.Action) && validFingerprint(binding.DestinationFingerprint) &&
		validFingerprint(binding.IntentFingerprint)
}

func secretBindingFingerprint(binding SecretBinding) string {
	encoded, _ := json.Marshal(binding)
	return fingerprint("neo-secret-binding-v1", encoded)
}

func digestSecretHandle(handle string) string {
	digest := sha256.Sum256([]byte(handle))
	return hex.EncodeToString(digest[:])
}
