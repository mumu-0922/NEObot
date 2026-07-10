package teams

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

func TestInviteMailOutboxWorkerRunOncePendingToSent(t *testing.T) {
	now := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	worker, store, transport := newTestInviteMailOutboxWorker(
		t,
		now,
		newPendingInviteMailState(t, now, now.Add(time.Hour)),
		nil,
	)

	processed, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if !processed {
		t.Fatal("RunOnce() processed = false")
	}
	if !store.processingObserved {
		t.Fatal("worker did not move the outbox through processing before SMTP")
	}
	if !transport.sendObservedLock {
		t.Fatal("SMTP send did not happen while the outbox lock was held")
	}
	if len(transport.messages) != 1 {
		t.Fatalf("SMTP messages = %d, want 1", len(transport.messages))
	}

	state := store.row
	if state.Status != InviteDeliverySent ||
		state.AttemptCount != 1 ||
		state.LeaseOwner != "" ||
		!state.TerminalAt.Equal(now) ||
		state.ErrorCode != "" ||
		!state.RetryAt.IsZero() {
		t.Fatalf("outbox state after send = %#v", state)
	}
	if transport.messages[0].MessageID != state.MessageID {
		t.Fatalf("Message-ID = %q, want %q", transport.messages[0].MessageID, state.MessageID)
	}
	if !strings.Contains(transport.messages[0].TextBody, state.AcceptanceURL) {
		t.Fatalf("invite body missing acceptance URL: %q", transport.messages[0].TextBody)
	}
}

func TestInviteMailOutboxWorkerRunOnceRetriesOnSMTPFailure(t *testing.T) {
	now := time.Date(2026, 7, 11, 11, 0, 0, 0, time.UTC)
	worker, store, transport := newTestInviteMailOutboxWorker(
		t,
		now,
		newPendingInviteMailState(t, now, now.Add(2*time.Hour)),
		auth.ErrSMTPDeliveryFailed,
	)

	processed, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if !processed {
		t.Fatal("RunOnce() processed = false")
	}
	if len(transport.messages) != 1 {
		t.Fatalf("SMTP messages = %d, want 1", len(transport.messages))
	}

	state := store.row
	if state.Status != InviteDeliveryPending ||
		state.AttemptCount != 1 ||
		state.ErrorCode != inviteMailErrorSMTPDeliveryFailed {
		t.Fatalf("outbox state after retry scheduling = %#v", state)
	}
	wantRetryAt := now.Add(defaultInviteMailBaseBackoff)
	if !state.AvailableAt.Equal(wantRetryAt) || !state.RetryAt.Equal(wantRetryAt) {
		t.Fatalf("retry schedule = %s/%s, want %s", state.AvailableAt, state.RetryAt, wantRetryAt)
	}
	if state.LeaseOwner != "" || !state.TerminalAt.IsZero() {
		t.Fatalf("lease/terminal state after retry = %#v", state)
	}
}

func TestInviteMailOutboxWorkerRunOnceReclaimsExpiredLease(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	state := newPendingInviteMailState(t, now, now.Add(time.Hour))
	state.Status = InviteDeliveryProcessing
	state.AttemptCount = 1
	state.LeaseOwner = "00000000-0000-0000-0000-0000000000ff"
	state.LeaseExpiresAt = now.Add(-time.Minute)

	worker, store, transport := newTestInviteMailOutboxWorker(
		t,
		now,
		state,
		nil,
	)

	processed, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if !processed {
		t.Fatal("RunOnce() processed = false")
	}
	if !store.processingObserved || !transport.sendObservedLock {
		t.Fatalf("processing/lock observation = %t/%t", store.processingObserved, transport.sendObservedLock)
	}

	if store.row.Status != InviteDeliverySent || store.row.AttemptCount != 2 {
		t.Fatalf("reclaimed outbox state = %#v", store.row)
	}
}

func TestInviteMailOutboxWorkerRunOnceCancelsExpiredInvite(t *testing.T) {
	now := time.Date(2026, 7, 11, 13, 0, 0, 0, time.UTC)
	worker, store, transport := newTestInviteMailOutboxWorker(
		t,
		now,
		newPendingInviteMailState(t, now, now.Add(-time.Second)),
		nil,
	)

	processed, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if !processed {
		t.Fatal("RunOnce() processed = false")
	}
	if len(transport.messages) != 0 {
		t.Fatalf("SMTP messages = %d, want 0", len(transport.messages))
	}
	if store.row.Status != InviteDeliveryCancelled ||
		store.row.AttemptCount != 1 ||
		!store.row.TerminalAt.Equal(now) {
		t.Fatalf("cancelled outbox state = %#v", store.row)
	}
}

func TestInviteMailOutboxWorkerRunOnceFailsDecryptError(t *testing.T) {
	now := time.Date(2026, 7, 11, 14, 0, 0, 0, time.UTC)
	state := newPendingInviteMailState(t, now, now.Add(time.Hour))
	state.Ciphertext[0] ^= 0xff

	worker, store, transport := newTestInviteMailOutboxWorker(
		t,
		now,
		state,
		nil,
	)

	processed, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if !processed {
		t.Fatal("RunOnce() processed = false")
	}
	if len(transport.messages) != 0 {
		t.Fatalf("SMTP messages = %d, want 0", len(transport.messages))
	}
	if store.row.Status != InviteDeliveryFailed ||
		store.row.AttemptCount != store.row.MaxAttempts ||
		store.row.ErrorCode != inviteMailErrorDecryptFailed ||
		!store.row.TerminalAt.Equal(now) {
		t.Fatalf("failed outbox state = %#v", store.row)
	}
}

func TestInviteMailOutboxWorkerGateFailsClosedUntilRunning(t *testing.T) {
	now := time.Date(2026, 7, 11, 15, 0, 0, 0, time.UTC)
	worker, _, _ := newTestInviteMailOutboxWorker(
		t,
		now,
		newPendingInviteMailState(t, now, now.Add(time.Hour)),
		nil,
	)

	if err := worker.AdmitInviteDelivery(context.Background()); !errors.Is(err, ErrInviteDeliveryUnavailable) {
		t.Fatalf("AdmitInviteDelivery() before Run = %v, want ErrInviteDeliveryUnavailable", err)
	}

	started := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	worker.sleep = func(ctx context.Context, delay time.Duration) error {
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return ctx.Err()
	}

	done := make(chan error, 1)
	go func() {
		done <- worker.Run(ctx)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker to start")
	}
	if err := worker.AdmitInviteDelivery(context.Background()); err != nil {
		t.Fatalf("AdmitInviteDelivery() while running = %v", err)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker shutdown")
	}
}

func TestInviteMailOutboxWorkerStopsAfterPersistentStoreErrors(t *testing.T) {
	now := time.Date(2026, 7, 11, 16, 0, 0, 0, time.UTC)
	worker, store, _ := newTestInviteMailOutboxWorker(
		t,
		now,
		newPendingInviteMailState(t, now, now.Add(time.Hour)),
		nil,
	)
	store.failExpiredErr = errors.New("database unavailable")
	worker.maximumStoreErrors = 3
	worker.sleep = func(context.Context, time.Duration) error { return nil }

	err := worker.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "3 consecutive times") {
		t.Fatalf("Run() persistent store error = %v", err)
	}
	if store.failExpiredCalls != 3 {
		t.Fatalf("FailExpiredProcessingClaims() calls = %d, want 3", store.failExpiredCalls)
	}
	if err := worker.AdmitInviteDelivery(context.Background()); !errors.Is(err, ErrInviteDeliveryUnavailable) {
		t.Fatalf("AdmitInviteDelivery() after worker failure = %v", err)
	}
}

func TestRequireInviteMailDecisionRowFencesLostLease(t *testing.T) {
	if err := requireInviteMailDecisionRow(fixedSQLResult(1), "sent"); err != nil {
		t.Fatalf("requireInviteMailDecisionRow(1) error = %v", err)
	}
	err := requireInviteMailDecisionRow(fixedSQLResult(0), "sent")
	if err == nil || !strings.Contains(err.Error(), "lost lease fence") {
		t.Fatalf("requireInviteMailDecisionRow(0) error = %v", err)
	}
}

func newTestInviteMailOutboxWorker(
	t *testing.T,
	now time.Time,
	state fakeInviteMailOutboxState,
	transportErr error,
) (*InviteMailOutboxWorker, *fakeInviteMailOutboxStore, *fakeInviteMailSMTPTransport) {
	t.Helper()

	store := &fakeInviteMailOutboxStore{row: state}
	transport := &fakeInviteMailSMTPTransport{
		ready: true,
		err:   transportErr,
		lockHeld: func() bool {
			return store.lockHeld
		},
	}
	cipher := newTestInviteMailCipher(t)
	worker, err := newInviteMailOutboxWorker(
		store,
		cipher,
		transport,
		WithInviteMailWorkerClock(func() time.Time { return now }),
		WithInviteMailWorkerOwnerID("00000000-0000-0000-0000-0000000000aa"),
		WithInviteMailWorkerRandom(func() float64 { return 0 }),
	)
	if err != nil {
		t.Fatalf("newInviteMailOutboxWorker() error = %v", err)
	}
	return worker, store, transport
}

func newTestInviteMailCipher(t *testing.T) *MailCipher {
	t.Helper()

	cipher, err := NewMailCipher(MailKeyring{
		ActiveKeyID: "active",
		Keys: map[string][]byte{
			"active": []byte("0123456789abcdef0123456789abcdef"),
		},
	})
	if err != nil {
		t.Fatalf("NewMailCipher() error = %v", err)
	}
	return cipher
}

type fakeInviteMailOutboxState struct {
	inviteMailLockedOutbox
	AvailableAt   time.Time
	RetryAt       time.Time
	TerminalAt    time.Time
	ErrorCode     string
	AcceptanceURL string
}

func newPendingInviteMailState(
	t *testing.T,
	now time.Time,
	expiresAt time.Time,
) fakeInviteMailOutboxState {
	t.Helper()

	cipher := newTestInviteMailCipher(t)
	state := fakeInviteMailOutboxState{
		inviteMailLockedOutbox: inviteMailLockedOutbox{
			ID:           "00000000-0000-0000-0000-000000000401",
			TeamID:       "00000000-0000-0000-0000-000000000201",
			InviteID:     "00000000-0000-0000-0000-000000000301",
			MessageID:    inviteMessageID("00000000-0000-0000-0000-000000000401"),
			Status:       InviteDeliveryPending,
			AttemptCount: 0,
			MaxAttempts:  8,
		},
		AvailableAt:   now.Add(-time.Minute),
		AcceptanceURL: "https://app.example.test/invite/accept#token=" + testInviteToken('a'),
	}
	encrypted, err := cipher.EncryptInvitePayload(
		state.ID,
		state.InviteID,
		state.TeamID,
		InviteMailPayload{
			Email:                "invitee@example.test",
			InviteToken:          strings.Repeat("a", 64),
			AcceptanceURL:        state.AcceptanceURL,
			TeamID:               state.TeamID,
			InvitedByUserID:      "00000000-0000-0000-0000-000000000101",
			InvitedByDisplayName: "Inviter",
			TeamRole:             TeamRoleMember,
			ExpiresAt:            expiresAt,
		},
	)
	if err != nil {
		t.Fatalf("EncryptInvitePayload() error = %v", err)
	}
	state.KeyID = encrypted.KeyID
	state.PayloadVersion = encrypted.Version
	state.Nonce = append([]byte(nil), encrypted.Nonce...)
	state.Ciphertext = append([]byte(nil), encrypted.Ciphertext...)
	return state
}

type fakeInviteMailOutboxStore struct {
	row                fakeInviteMailOutboxState
	lockHeld           bool
	processingObserved bool
	failExpiredErr     error
	failExpiredCalls   int
}

func (s *fakeInviteMailOutboxStore) FailExpiredProcessingClaims(
	context.Context,
	time.Time,
	string,
) error {
	s.failExpiredCalls++
	return s.failExpiredErr
}

func (s *fakeInviteMailOutboxStore) ClaimNext(
	_ context.Context,
	ownerID string,
	now time.Time,
	leaseExpiresAt time.Time,
) (*inviteMailOutboxClaim, error) {
	switch {
	case s.row.Status == InviteDeliveryPending &&
		s.row.AttemptCount < s.row.MaxAttempts &&
		!s.row.AvailableAt.After(now):
	case s.row.Status == InviteDeliveryProcessing &&
		s.row.AttemptCount < s.row.MaxAttempts &&
		!s.row.LeaseExpiresAt.After(now):
	default:
		return nil, nil
	}

	s.row.Status = InviteDeliveryProcessing
	s.row.AttemptCount++
	s.row.LeaseOwner = ownerID
	s.row.LeaseExpiresAt = leaseExpiresAt
	s.processingObserved = true
	return &inviteMailOutboxClaim{ID: s.row.ID}, nil
}

func (s *fakeInviteMailOutboxStore) ProcessClaim(
	_ context.Context,
	outboxID string,
	_ string,
	now time.Time,
	fn func(inviteMailLockedOutbox) (inviteMailOutboxDecision, error),
) error {
	if s.row.ID != outboxID {
		return nil
	}
	s.lockHeld = true
	decision, err := fn(s.row.inviteMailLockedOutbox)
	s.lockHeld = false
	if err != nil {
		return err
	}

	switch decision.Action {
	case inviteMailOutboxActionNone:
		return nil
	case inviteMailOutboxActionRetry:
		s.row.Status = InviteDeliveryPending
		s.row.LeaseOwner = ""
		s.row.LeaseExpiresAt = time.Time{}
		s.row.AvailableAt = decision.RetryAt
		s.row.RetryAt = decision.RetryAt
		s.row.TerminalAt = time.Time{}
		s.row.ErrorCode = decision.ErrorCode
	case inviteMailOutboxActionSent:
		s.row.Status = InviteDeliverySent
		s.row.LeaseOwner = ""
		s.row.LeaseExpiresAt = time.Time{}
		s.row.RetryAt = time.Time{}
		s.row.TerminalAt = now
		s.row.ErrorCode = ""
	case inviteMailOutboxActionFailed:
		s.row.Status = InviteDeliveryFailed
		s.row.AttemptCount = s.row.MaxAttempts
		s.row.LeaseOwner = ""
		s.row.LeaseExpiresAt = time.Time{}
		s.row.RetryAt = time.Time{}
		s.row.TerminalAt = now
		s.row.ErrorCode = decision.ErrorCode
	case inviteMailOutboxActionCancelled:
		s.row.Status = InviteDeliveryCancelled
		s.row.LeaseOwner = ""
		s.row.LeaseExpiresAt = time.Time{}
		s.row.RetryAt = time.Time{}
		s.row.TerminalAt = now
		s.row.ErrorCode = ""
	default:
		return errors.New("unexpected outbox action")
	}
	return nil
}

type fakeInviteMailSMTPTransport struct {
	ready            bool
	err              error
	lockHeld         func() bool
	sendObservedLock bool
	messages         []auth.SMTPPlaintextMessage
}

type fixedSQLResult int64

func (r fixedSQLResult) LastInsertId() (int64, error) { return 0, nil }
func (r fixedSQLResult) RowsAffected() (int64, error) { return int64(r), nil }

func (t *fakeInviteMailSMTPTransport) Ready() bool {
	return t != nil && t.ready
}

func (t *fakeInviteMailSMTPTransport) SendPlaintext(
	message auth.SMTPPlaintextMessage,
) error {
	t.messages = append(t.messages, message)
	if t.lockHeld != nil {
		t.sendObservedLock = t.lockHeld()
	}
	return t.err
}
