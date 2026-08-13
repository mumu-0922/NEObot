package agentbroker

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSecretHandleIsSingleUseBoundAndNeverPersistedPlaintext(t *testing.T) {
	const canary = "G204_SECRET_CANARY_DO_NOT_PERSIST"
	repository := &memorySecretRepository{records: map[string]SecretHandleRecord{}}
	broker, err := NewSecretBroker(repository, secretResolverFunc(func(context.Context, string, Subject) ([]byte, error) {
		return []byte(canary), nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC)
	broker.now = func() time.Time { return now }
	intent, registry := testSecretAuthority(t, now)
	binding := testSecretBinding()
	handle, err := broker.Issue(context.Background(), intent, registry,
		"secret_ref_0123456789abcdef", binding, time.Minute)
	if err != nil || strings.Contains(handle, canary) {
		t.Fatalf("Issue = %q, %v", handle, err)
	}
	for _, record := range repository.records {
		if strings.Contains(record.HandleDigest, canary) || strings.Contains(record.BindingFingerprint, canary) ||
			strings.Contains(record.SecretRef, canary) {
			t.Fatalf("durable record leaked canary: %#v", record)
		}
	}
	wrong := binding
	wrong.Action = "list"
	if _, err := broker.Consume(context.Background(), handle, wrong); !errors.Is(err, ErrSecretDenied) {
		t.Fatalf("wrong binding error = %v", err)
	}
	value, err := broker.Consume(context.Background(), handle, binding)
	if err != nil || string(value) != canary {
		t.Fatalf("Consume = %q, %v", value, err)
	}
	clear(value)
	if _, err := broker.Consume(context.Background(), handle, binding); !errors.Is(err, ErrSecretDenied) {
		t.Fatalf("replay error = %v", err)
	}
}

func TestSecretHandleExpiresAndRevokesWithoutResolutionReplay(t *testing.T) {
	repository := &memorySecretRepository{records: map[string]SecretHandleRecord{}}
	resolutions := 0
	broker, _ := NewSecretBroker(repository, secretResolverFunc(func(context.Context, string, Subject) ([]byte, error) {
		resolutions++
		return []byte("synthetic"), nil
	}))
	now := time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC)
	broker.now = func() time.Time { return now }
	intent, registry := testSecretAuthority(t, now)
	binding := testSecretBinding()
	handle, _ := broker.Issue(context.Background(), intent, registry,
		"secret_ref_0123456789abcdef", binding, time.Second)
	now = now.Add(2 * time.Second)
	if _, err := broker.Consume(context.Background(), handle, binding); !errors.Is(err, ErrSecretDenied) {
		t.Fatalf("expired error = %v", err)
	}
	if resolutions != 1 {
		t.Fatalf("secret resolved %d times", resolutions)
	}
	if _, err := broker.RevokeIntent(context.Background(), "intent_0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
}

func TestSecretIssueRejectsAuthorityBeforeResolver(t *testing.T) {
	repository := &memorySecretRepository{records: map[string]SecretHandleRecord{}}
	resolutions := 0
	broker, _ := NewSecretBroker(repository, secretResolverFunc(func(context.Context, string, Subject) ([]byte, error) {
		resolutions++
		return []byte("synthetic"), nil
	}))
	now := time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC)
	broker.now = func() time.Time { return now }
	intent, registry := testSecretAuthority(t, now)
	for _, mutate := range []func(*PreparedIntent, *ToolRegistry, *SecretBinding){
		func(value *PreparedIntent, _ *ToolRegistry, _ *SecretBinding) { value.State = IntentApproved },
		func(_ *PreparedIntent, value *ToolRegistry, _ *SecretBinding) { value.Fingerprint = testPackage },
		func(_ *PreparedIntent, _ *ToolRegistry, value *SecretBinding) { value.Action = "list" },
	} {
		candidateIntent, candidateRegistry, binding := intent, registry, testSecretBinding()
		mutate(&candidateIntent, &candidateRegistry, &binding)
		if _, err := broker.Issue(context.Background(), candidateIntent, candidateRegistry,
			"secret_ref_0123456789abcdef", binding, time.Minute); !errors.Is(err, ErrSecretDenied) {
			t.Fatalf("authority error = %v", err)
		}
	}
	if resolutions != 0 {
		t.Fatalf("denied authority resolved secret %d times", resolutions)
	}
}

func TestGrantRevocationClearsInMemorySecretCanary(t *testing.T) {
	const canary = "G204_REVOKED_GRANT_MEMORY_CANARY"
	repository := &grantSecretRepository{
		memoryRepository:       &memoryRepository{intents: map[string]PreparedIntent{}, requests: map[string]string{}, approvals: map[string]ApprovalInput{}},
		memorySecretRepository: &memorySecretRepository{records: map[string]SecretHandleRecord{}},
		revokedGrants:          map[string]struct{}{},
	}
	broker, _ := NewSecretBroker(repository, secretResolverFunc(func(context.Context, string, Subject) ([]byte, error) {
		return []byte(canary), nil
	}))
	now := time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC)
	broker.now = func() time.Time { return now }
	intent, registry := testSecretAuthority(t, now)
	repository.intents[intent.IntentID] = intent
	binding := testSecretBinding()
	handle, err := broker.Issue(context.Background(), intent, registry,
		"secret_ref_0123456789abcdef", binding, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(repository, nil, broker)
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.RevokeGrant(context.Background(), GrantRevocationInput{
		GrantID: intent.GrantID, GrantFingerprint: testPackage,
		ActorType: "operator", ActorID: "secret-test", ReasonCode: "OPERATOR_REVOKED",
	})
	if err != nil || !created {
		t.Fatalf("RevokeGrant = %v, %v", created, err)
	}
	broker.mu.Lock()
	remaining := len(broker.handles)
	broker.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("revoked Grant retained %d in-memory handles", remaining)
	}
	for _, record := range repository.records {
		if record.State != "revoked" {
			t.Fatalf("revoked Grant retained durable handle state %q", record.State)
		}
	}
	if _, err := broker.Consume(context.Background(), handle, binding); !errors.Is(err, ErrSecretDenied) {
		t.Fatalf("Consume after Grant revocation = %v", err)
	}
	if _, err := broker.Issue(context.Background(), intent, registry,
		"secret_ref_0123456789abcdef", binding, time.Minute); !errors.Is(err, ErrGrantDenied) {
		t.Fatalf("Issue after Grant revocation = %v", err)
	}
}

type grantSecretRepository struct {
	*memoryRepository
	*memorySecretRepository
	revokedGrants map[string]struct{}
}

func (repository *grantSecretRepository) RevokeGrant(ctx context.Context, input GrantRevocationInput) (bool, error) {
	created, err := repository.memoryRepository.RevokeGrant(ctx, input)
	if err != nil {
		return false, err
	}
	repository.memoryRepository.mu.Lock()
	repository.revokedGrants[input.GrantID] = struct{}{}
	repository.memoryRepository.mu.Unlock()
	repository.memorySecretRepository.mu.Lock()
	defer repository.memorySecretRepository.mu.Unlock()
	for digest, record := range repository.records {
		intent, ok := repository.intents[record.IntentID]
		if ok && intent.GrantID == input.GrantID && record.State == "active" {
			record.State = "revoked"
			repository.records[digest] = record
		}
	}
	return created, nil
}

func (repository *grantSecretRepository) CreateSecretHandle(ctx context.Context, record SecretHandleRecord, binding SecretBinding) error {
	repository.memoryRepository.mu.Lock()
	intent, ok := repository.intents[record.IntentID]
	_, revoked := repository.revokedGrants[intent.GrantID]
	repository.memoryRepository.mu.Unlock()
	if !ok || revoked {
		return ErrGrantDenied
	}
	return repository.memorySecretRepository.CreateSecretHandle(ctx, record, binding)
}

func testSecretAuthority(t *testing.T, now time.Time) (PreparedIntent, ToolRegistry) {
	t.Helper()
	grant := testGrantAt(now)
	grant.Secrets = []SecretGrant{{Slot: "workspace", BrokerRef: "secret_ref_0123456789abcdef",
		Actions: []string{"read"}, TTLSeconds: 300}}
	registry, err := BuildRegistry([]ToolDefinition{{Identity: "workspace_read", Capability: "workspace.read",
		Actions: []string{"list", "read"}, Classification: ClassificationRead, Idempotent: true}},
		[]string{"workspace_read"}, grant, now)
	if err != nil {
		t.Fatal(err)
	}
	return PreparedIntent{IntentID: "intent_0123456789abcdef", Subject: grant.Subject,
		RunID: testRun, StepID: testStep, AttemptID: testAttempt, Generation: 1,
		GrantID: grant.GrantID, RegistryFingerprint: registry.Fingerprint, ToolIdentity: "workspace_read",
		Capability: "workspace.read", Action: "read", Resource: "project/a",
		IntentFingerprint: testRuntime, State: IntentCommitting, ExpiresAt: now.Add(10 * time.Minute)}, registry
}

func testSecretBinding() SecretBinding {
	return SecretBinding{Subject: Subject{UserID: testUser, ProjectID: "project_01234567", AssistantID: "assistant_01234567"},
		RunID: testRun, StepID: testStep, AttemptID: testAttempt, Generation: 1,
		Capability: "workspace.read", Action: "read", DestinationFingerprint: testPackage,
		IntentFingerprint: testRuntime}
}

type secretResolverFunc func(context.Context, string, Subject) ([]byte, error)

func (resolver secretResolverFunc) Resolve(ctx context.Context, ref string, subject Subject) ([]byte, error) {
	return resolver(ctx, ref, subject)
}

type memorySecretRepository struct {
	mu      sync.Mutex
	records map[string]SecretHandleRecord
}

func (repository *memorySecretRepository) CreateSecretHandle(_ context.Context, record SecretHandleRecord, _ SecretBinding) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if _, exists := repository.records[record.HandleDigest]; exists {
		return ErrReplayDetected
	}
	repository.records[record.HandleDigest] = record
	return nil
}

func (repository *memorySecretRepository) RevokeSecretHandle(_ context.Context, digest string, _ time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	record, ok := repository.records[digest]
	if !ok || record.State != "active" {
		return ErrSecretDenied
	}
	record.State = "revoked"
	repository.records[digest] = record
	return nil
}

func (repository *memorySecretRepository) ConsumeSecretHandle(_ context.Context, digest, binding string, _ time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	record, ok := repository.records[digest]
	if !ok || record.State != "active" || record.BindingFingerprint != binding {
		return ErrSecretDenied
	}
	record.State = "used"
	repository.records[digest] = record
	return nil
}

func (repository *memorySecretRepository) RevokeIntentHandles(_ context.Context, intentID string, _ time.Time) (int, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	count := 0
	for digest, record := range repository.records {
		if record.IntentID == intentID && record.State == "active" {
			record.State = "revoked"
			repository.records[digest] = record
			count++
		}
	}
	return count, nil
}
