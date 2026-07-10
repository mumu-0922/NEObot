package teams

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

const (
	inviteMailSubject                   = "Neo Chat team invitation"
	defaultInviteMailLeaseDuration      = 30 * time.Second
	defaultInviteMailPollInterval       = 500 * time.Millisecond
	defaultInviteMailBaseBackoff        = 5 * time.Second
	defaultInviteMailMaximumBackoff     = 15 * time.Minute
	defaultInviteMailJitterMultiplier   = 0.2
	defaultInviteMailMaximumStoreErrors = 3

	inviteMailErrorDecryptFailed      = "DECRYPT_FAILED"
	inviteMailErrorLeaseExpired       = "LEASE_EXPIRED"
	inviteMailErrorSMTPDialFailed     = "SMTP_DIAL_FAILED"
	inviteMailErrorSMTPDeadlineFailed = "SMTP_DEADLINE_FAILED"
	inviteMailErrorSMTPDeliveryFailed = "SMTP_DELIVERY_FAILED"
	inviteMailErrorSMTPInvalidMessage = "SMTP_INVALID_MESSAGE"
	inviteMailErrorSMTPInvalidRcpt    = "SMTP_INVALID_RECIPIENT"
)

var errInviteMailWorkerAlreadyRunning = errors.New("invite mail worker is already running")

type InviteMailOutboxWorker struct {
	store      inviteMailOutboxStore
	mailCipher *MailCipher
	transport  inviteMailSMTPTransport
	clock      func() time.Time
	sleep      func(context.Context, time.Duration) error
	random     func() float64

	ownerID            string
	leaseDuration      time.Duration
	pollInterval       time.Duration
	baseBackoff        time.Duration
	maximumBackoff     time.Duration
	jitterMultiplier   float64
	maximumStoreErrors int

	running atomic.Bool
	ready   atomic.Bool
}

type InviteMailOutboxWorkerOption func(*InviteMailOutboxWorker)

type inviteMailSMTPTransport interface {
	Ready() bool
	SendPlaintext(message auth.SMTPPlaintextMessage) error
}

type inviteMailOutboxStore interface {
	FailExpiredProcessingClaims(
		ctx context.Context,
		now time.Time,
		errorCode string,
	) error
	ClaimNext(
		ctx context.Context,
		ownerID string,
		now time.Time,
		leaseExpiresAt time.Time,
	) (*inviteMailOutboxClaim, error)
	ProcessClaim(
		ctx context.Context,
		outboxID string,
		ownerID string,
		now time.Time,
		fn func(inviteMailLockedOutbox) (inviteMailOutboxDecision, error),
	) error
}

type inviteMailOutboxSQLStore struct {
	db *sql.DB
}

type inviteMailOutboxClaim struct {
	ID string
}

type inviteMailLockedOutbox struct {
	ID             string
	TeamID         string
	InviteID       string
	KeyID          string
	PayloadVersion int
	Nonce          []byte
	Ciphertext     []byte
	MessageID      string
	Status         string
	AttemptCount   int
	MaxAttempts    int
	LeaseOwner     string
	LeaseExpiresAt time.Time
}

type inviteMailOutboxAction int

const (
	inviteMailOutboxActionNone inviteMailOutboxAction = iota
	inviteMailOutboxActionRetry
	inviteMailOutboxActionSent
	inviteMailOutboxActionFailed
	inviteMailOutboxActionCancelled
)

type inviteMailOutboxDecision struct {
	Action    inviteMailOutboxAction
	RetryAt   time.Time
	ErrorCode string
}

func NewInviteMailOutboxWorker(
	db *sql.DB,
	cipher *MailCipher,
	transport *auth.SMTPSyncTransport,
	opts ...InviteMailOutboxWorkerOption,
) (*InviteMailOutboxWorker, error) {
	if db == nil {
		return nil, ErrDatabaseRequired
	}
	return newInviteMailOutboxWorker(
		&inviteMailOutboxSQLStore{db: db},
		cipher,
		transport,
		opts...,
	)
}

func newInviteMailOutboxWorker(
	store inviteMailOutboxStore,
	cipher *MailCipher,
	transport inviteMailSMTPTransport,
	opts ...InviteMailOutboxWorkerOption,
) (*InviteMailOutboxWorker, error) {
	ownerID, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("generate invite mail worker owner id: %w", err)
	}

	worker := &InviteMailOutboxWorker{
		store:              store,
		mailCipher:         cipher,
		transport:          transport,
		clock:              time.Now,
		sleep:              sleepInviteMailWorker,
		random:             rand.Float64,
		ownerID:            ownerID,
		leaseDuration:      defaultInviteMailLeaseDuration,
		pollInterval:       defaultInviteMailPollInterval,
		baseBackoff:        defaultInviteMailBaseBackoff,
		maximumBackoff:     defaultInviteMailMaximumBackoff,
		jitterMultiplier:   defaultInviteMailJitterMultiplier,
		maximumStoreErrors: defaultInviteMailMaximumStoreErrors,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(worker)
		}
	}
	if err := worker.validate(); err != nil {
		return nil, err
	}
	return worker, nil
}

func WithInviteMailWorkerClock(
	now func() time.Time,
) InviteMailOutboxWorkerOption {
	return func(worker *InviteMailOutboxWorker) {
		if now != nil {
			worker.clock = now
		}
	}
}

func WithInviteMailWorkerSleep(
	sleep func(context.Context, time.Duration) error,
) InviteMailOutboxWorkerOption {
	return func(worker *InviteMailOutboxWorker) {
		if sleep != nil {
			worker.sleep = sleep
		}
	}
}

func WithInviteMailWorkerRandom(
	random func() float64,
) InviteMailOutboxWorkerOption {
	return func(worker *InviteMailOutboxWorker) {
		if random != nil {
			worker.random = random
		}
	}
}

func WithInviteMailWorkerOwnerID(
	ownerID string,
) InviteMailOutboxWorkerOption {
	return func(worker *InviteMailOutboxWorker) {
		worker.ownerID = strings.TrimSpace(ownerID)
	}
}

func WithInviteMailWorkerLeaseDuration(
	lease time.Duration,
) InviteMailOutboxWorkerOption {
	return func(worker *InviteMailOutboxWorker) {
		worker.leaseDuration = lease
	}
}

func WithInviteMailWorkerPollInterval(
	interval time.Duration,
) InviteMailOutboxWorkerOption {
	return func(worker *InviteMailOutboxWorker) {
		worker.pollInterval = interval
	}
}

func WithInviteMailWorkerBackoff(
	base time.Duration,
	maximum time.Duration,
) InviteMailOutboxWorkerOption {
	return func(worker *InviteMailOutboxWorker) {
		worker.baseBackoff = base
		worker.maximumBackoff = maximum
	}
}

func (w *InviteMailOutboxWorker) AdmitInviteDelivery(context.Context) error {
	if !w.readyForDelivery() || !w.running.Load() || !w.ready.Load() {
		return ErrInviteDeliveryUnavailable
	}
	return nil
}

func (w *InviteMailOutboxWorker) Run(ctx context.Context) error {
	if w == nil {
		return ErrInviteDeliveryUnavailable
	}
	if err := w.validate(); err != nil {
		return err
	}
	if !w.running.CompareAndSwap(false, true) {
		return errInviteMailWorkerAlreadyRunning
	}
	w.ready.Store(false)
	defer func() {
		w.ready.Store(false)
		w.running.Store(false)
	}()

	consecutiveStoreErrors := 0
	for {
		processed, err := w.RunOnce(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			w.ready.Store(false)
			consecutiveStoreErrors++
			if consecutiveStoreErrors >= w.maximumStoreErrors {
				return fmt.Errorf(
					"invite mail outbox store failed %d consecutive times: %w",
					consecutiveStoreErrors,
					err,
				)
			}
			if sleepErr := w.sleep(ctx, w.pollInterval); sleepErr != nil {
				if errors.Is(sleepErr, context.Canceled) ||
					errors.Is(sleepErr, context.DeadlineExceeded) {
					return nil
				}
				return sleepErr
			}
			continue
		}
		consecutiveStoreErrors = 0
		w.ready.Store(true)
		if processed {
			continue
		}
		if err := w.sleep(ctx, w.pollInterval); err != nil {
			if errors.Is(err, context.Canceled) ||
				errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			return err
		}
	}
}

func (w *InviteMailOutboxWorker) RunOnce(
	ctx context.Context,
) (bool, error) {
	if w == nil {
		return false, ErrInviteDeliveryUnavailable
	}
	if err := w.validate(); err != nil {
		return false, err
	}

	now := w.clock().UTC()
	if err := w.store.FailExpiredProcessingClaims(
		ctx,
		now,
		inviteMailErrorLeaseExpired,
	); err != nil {
		return false, err
	}

	claim, err := w.store.ClaimNext(
		ctx,
		w.ownerID,
		now,
		now.Add(w.leaseDuration).UTC(),
	)
	if err != nil {
		return false, err
	}
	if claim == nil {
		return false, nil
	}

	if err := w.store.ProcessClaim(
		ctx,
		claim.ID,
		w.ownerID,
		now,
		func(outbox inviteMailLockedOutbox) (inviteMailOutboxDecision, error) {
			if outbox.Status != InviteDeliveryProcessing ||
				outbox.LeaseOwner != w.ownerID {
				return inviteMailNoopDecision(), nil
			}
			if !outbox.LeaseExpiresAt.After(now) {
				return inviteMailFailedDecision(
					inviteMailErrorLeaseExpired,
				), nil
			}

			payload, err := w.mailCipher.DecryptInvitePayload(
				outbox.ID,
				outbox.InviteID,
				outbox.TeamID,
				EncryptedMailPayload{
					KeyID:      outbox.KeyID,
					Version:    outbox.PayloadVersion,
					Nonce:      append([]byte(nil), outbox.Nonce...),
					Ciphertext: append([]byte(nil), outbox.Ciphertext...),
				},
			)
			if err != nil {
				return inviteMailFailedDecision(
					inviteMailErrorDecryptFailed,
				), nil
			}
			if !payload.ExpiresAt.After(now) {
				return inviteMailCancelledDecision(), nil
			}

			message := buildInviteSMTPMessage(outbox.MessageID, payload)
			err = w.transport.SendPlaintext(message)
			if err == nil {
				return inviteMailSentDecision(), nil
			}

			errorCode, terminal := classifyInviteSMTPError(err)
			if terminal || outbox.AttemptCount >= outbox.MaxAttempts {
				return inviteMailFailedDecision(errorCode), nil
			}
			return inviteMailRetryDecision(
				now.Add(w.retryDelay(outbox.AttemptCount)).UTC(),
				errorCode,
			), nil
		},
	); err != nil {
		return false, err
	}

	return true, nil
}

func buildInviteSMTPMessage(
	messageID string,
	payload InviteMailPayload,
) auth.SMTPPlaintextMessage {
	var body strings.Builder
	inviter := strings.TrimSpace(payload.InvitedByDisplayName)
	if inviter == "" {
		body.WriteString("You were invited to join a Neo Chat team.\r\n\r\n")
	} else {
		body.WriteString(inviter)
		body.WriteString(" invited you to join a Neo Chat team.\r\n\r\n")
	}
	body.WriteString("Use this link to accept the invite:\r\n")
	body.WriteString(payload.AcceptanceURL)
	body.WriteString("\r\n\r\n")
	body.WriteString("Team role: ")
	body.WriteString(payload.TeamRole)
	body.WriteString("\r\n")
	body.WriteString("Invite expires at ")
	body.WriteString(payload.ExpiresAt.UTC().Format(time.RFC3339))
	body.WriteString(".\r\n")

	return auth.SMTPPlaintextMessage{
		To:        payload.Email,
		Subject:   inviteMailSubject,
		MessageID: messageID,
		TextBody:  body.String(),
	}
}

func classifyInviteSMTPError(err error) (string, bool) {
	switch {
	case errors.Is(err, auth.ErrSMTPInvalidRecipient):
		return inviteMailErrorSMTPInvalidRcpt, true
	case errors.Is(err, auth.ErrSMTPInvalidMessage):
		return inviteMailErrorSMTPInvalidMessage, true
	case errors.Is(err, auth.ErrSMTPDialFailed):
		return inviteMailErrorSMTPDialFailed, false
	case errors.Is(err, auth.ErrSMTPDeadlineFailed):
		return inviteMailErrorSMTPDeadlineFailed, false
	default:
		return inviteMailErrorSMTPDeliveryFailed, false
	}
}

func inviteMailNoopDecision() inviteMailOutboxDecision {
	return inviteMailOutboxDecision{Action: inviteMailOutboxActionNone}
}

func inviteMailRetryDecision(
	retryAt time.Time,
	errorCode string,
) inviteMailOutboxDecision {
	return inviteMailOutboxDecision{
		Action:    inviteMailOutboxActionRetry,
		RetryAt:   retryAt.UTC(),
		ErrorCode: strings.TrimSpace(errorCode),
	}
}

func inviteMailSentDecision() inviteMailOutboxDecision {
	return inviteMailOutboxDecision{Action: inviteMailOutboxActionSent}
}

func inviteMailFailedDecision(
	errorCode string,
) inviteMailOutboxDecision {
	return inviteMailOutboxDecision{
		Action:    inviteMailOutboxActionFailed,
		ErrorCode: strings.TrimSpace(errorCode),
	}
}

func inviteMailCancelledDecision() inviteMailOutboxDecision {
	return inviteMailOutboxDecision{Action: inviteMailOutboxActionCancelled}
}

func sleepInviteMailWorker(
	ctx context.Context,
	delay time.Duration,
) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (w *InviteMailOutboxWorker) readyForDelivery() bool {
	return w != nil &&
		w.store != nil &&
		w.mailCipher != nil &&
		w.transport != nil &&
		w.transport.Ready() &&
		isUUID(w.ownerID) &&
		w.leaseDuration > 0 &&
		w.pollInterval > 0 &&
		w.baseBackoff > 0 &&
		w.maximumBackoff >= w.baseBackoff &&
		w.maximumStoreErrors > 0 &&
		w.random != nil &&
		w.clock != nil &&
		w.sleep != nil
}

func (w *InviteMailOutboxWorker) validate() error {
	if w == nil || w.store == nil || w.mailCipher == nil {
		return ErrInviteDeliveryUnavailable
	}
	if w.transport == nil || !w.transport.Ready() {
		return ErrInviteDeliveryUnavailable
	}
	if !isUUID(w.ownerID) {
		return ErrInviteDeliveryUnavailable
	}
	if w.clock == nil || w.sleep == nil || w.random == nil {
		return ErrInviteDeliveryUnavailable
	}
	if w.leaseDuration <= 0 ||
		w.pollInterval <= 0 ||
		w.baseBackoff <= 0 ||
		w.maximumBackoff < w.baseBackoff {
		return ErrInviteDeliveryUnavailable
	}
	if w.jitterMultiplier < 0 {
		return ErrInviteDeliveryUnavailable
	}
	if w.maximumStoreErrors <= 0 {
		return ErrInviteDeliveryUnavailable
	}
	return nil
}

func (w *InviteMailOutboxWorker) retryDelay(
	attemptCount int,
) time.Duration {
	if attemptCount < 1 {
		attemptCount = 1
	}

	delay := w.baseBackoff
	for step := 1; step < attemptCount && delay < w.maximumBackoff; step++ {
		if delay > w.maximumBackoff/2 {
			delay = w.maximumBackoff
			break
		}
		delay *= 2
	}
	if delay > w.maximumBackoff {
		delay = w.maximumBackoff
	}

	jitter := clampInviteMailFloat(w.random())
	if jitter == 0 || w.jitterMultiplier == 0 {
		return delay
	}
	extra := time.Duration(float64(delay) * w.jitterMultiplier * jitter)
	if delay+extra > w.maximumBackoff {
		return w.maximumBackoff
	}
	return delay + extra
}

func clampInviteMailFloat(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}

func (s *inviteMailOutboxSQLStore) FailExpiredProcessingClaims(
	ctx context.Context,
	now time.Time,
	errorCode string,
) error {
	if s == nil || s.db == nil {
		return ErrDatabaseRequired
	}
	if _, err := s.db.ExecContext(ctx, `
UPDATE identity_mail_outbox
SET status = 'failed',
    attempt_count = max_attempts,
    lease_owner = NULL,
    lease_expires_at = NULL,
    retry_at = NULL,
    terminal_at = $1,
    error_code = $2,
    updated_at = $1
WHERE status = 'processing'
  AND lease_expires_at <= $1
  AND attempt_count >= max_attempts
`, now.UTC(), strings.TrimSpace(errorCode)); err != nil {
		return fmt.Errorf("fail expired invite mail processing claims: %w", err)
	}
	return nil
}

func (s *inviteMailOutboxSQLStore) ClaimNext(
	ctx context.Context,
	ownerID string,
	now time.Time,
	leaseExpiresAt time.Time,
) (*inviteMailOutboxClaim, error) {
	if s == nil || s.db == nil {
		return nil, ErrDatabaseRequired
	}

	var claim inviteMailOutboxClaim
	err := s.db.QueryRowContext(ctx, `
WITH candidate AS (
  SELECT id
  FROM identity_mail_outbox
  WHERE (
    status = 'pending'
    AND available_at <= $2
    AND attempt_count < max_attempts
  ) OR (
    status = 'processing'
    AND lease_expires_at <= $2
    AND attempt_count < max_attempts
  )
  ORDER BY
    CASE
      WHEN status = 'processing' THEN lease_expires_at
      ELSE available_at
    END,
    id
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE identity_mail_outbox AS outbox
SET status = 'processing',
    attempt_count = outbox.attempt_count + 1,
    lease_owner = $1::uuid,
    lease_expires_at = $3,
    updated_at = $2
FROM candidate
WHERE outbox.id = candidate.id
RETURNING outbox.id
`, ownerID, now.UTC(), leaseExpiresAt.UTC()).Scan(&claim.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim invite mail outbox: %w", err)
	}
	return &claim, nil
}

func (s *inviteMailOutboxSQLStore) ProcessClaim(
	ctx context.Context,
	outboxID string,
	ownerID string,
	now time.Time,
	fn func(inviteMailLockedOutbox) (inviteMailOutboxDecision, error),
) error {
	if s == nil || s.db == nil {
		return ErrDatabaseRequired
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin invite mail processing transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	outbox, exists, err := lockInviteMailOutbox(ctx, tx, outboxID)
	if err != nil {
		return err
	}
	if !exists {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit invite mail no-op transaction: %w", err)
		}
		return nil
	}
	if outbox.Status != InviteDeliveryProcessing || outbox.LeaseOwner != ownerID {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit invite mail stale-claim transaction: %w", err)
		}
		return nil
	}

	decision, err := fn(outbox)
	if err != nil {
		return err
	}
	if err := applyInviteMailDecision(
		ctx,
		tx,
		outbox,
		ownerID,
		now,
		decision,
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit invite mail outbox decision: %w", err)
	}
	return nil
}

func lockInviteMailOutbox(
	ctx context.Context,
	tx *sql.Tx,
	outboxID string,
) (inviteMailLockedOutbox, bool, error) {
	var (
		outbox         inviteMailLockedOutbox
		leaseOwner     sql.NullString
		leaseExpiresAt sql.NullTime
	)
	err := tx.QueryRowContext(ctx, `
SELECT id,
       team_id,
       invite_id,
       key_id,
       payload_version,
       nonce,
       ciphertext,
       message_id,
       status,
       attempt_count,
       max_attempts,
       lease_owner::text,
       lease_expires_at
FROM identity_mail_outbox
WHERE id = $1
FOR UPDATE
`, outboxID).Scan(
		&outbox.ID,
		&outbox.TeamID,
		&outbox.InviteID,
		&outbox.KeyID,
		&outbox.PayloadVersion,
		&outbox.Nonce,
		&outbox.Ciphertext,
		&outbox.MessageID,
		&outbox.Status,
		&outbox.AttemptCount,
		&outbox.MaxAttempts,
		&leaseOwner,
		&leaseExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return inviteMailLockedOutbox{}, false, nil
	}
	if err != nil {
		return inviteMailLockedOutbox{}, false, fmt.Errorf("lock invite mail outbox: %w", err)
	}
	if leaseOwner.Valid {
		outbox.LeaseOwner = leaseOwner.String
	}
	if leaseExpiresAt.Valid {
		outbox.LeaseExpiresAt = leaseExpiresAt.Time.UTC()
	}
	return outbox, true, nil
}

func applyInviteMailDecision(
	ctx context.Context,
	tx *sql.Tx,
	outbox inviteMailLockedOutbox,
	ownerID string,
	now time.Time,
	decision inviteMailOutboxDecision,
) error {
	switch decision.Action {
	case inviteMailOutboxActionNone:
		return nil
	case inviteMailOutboxActionRetry:
		result, err := tx.ExecContext(ctx, `
UPDATE identity_mail_outbox
SET status = 'pending',
    lease_owner = NULL,
    lease_expires_at = NULL,
    available_at = $3,
    retry_at = $3,
    error_code = $4,
    updated_at = $5
WHERE id = $1
  AND status = 'processing'
  AND lease_owner = $2::uuid
`, outbox.ID, ownerID, decision.RetryAt.UTC(), decision.ErrorCode, now.UTC())
		if err != nil {
			return fmt.Errorf("mark invite mail outbox retry: %w", err)
		}
		return requireInviteMailDecisionRow(result, "retry")
	case inviteMailOutboxActionSent:
		result, err := tx.ExecContext(ctx, `
UPDATE identity_mail_outbox
SET status = 'sent',
    lease_owner = NULL,
    lease_expires_at = NULL,
    retry_at = NULL,
    terminal_at = $3,
    error_code = NULL,
    updated_at = $3
WHERE id = $1
  AND status = 'processing'
  AND lease_owner = $2::uuid
`, outbox.ID, ownerID, now.UTC())
		if err != nil {
			return fmt.Errorf("mark invite mail outbox sent: %w", err)
		}
		return requireInviteMailDecisionRow(result, "sent")
	case inviteMailOutboxActionFailed:
		result, err := tx.ExecContext(ctx, `
UPDATE identity_mail_outbox
SET status = 'failed',
    attempt_count = max_attempts,
    lease_owner = NULL,
    lease_expires_at = NULL,
    retry_at = NULL,
    terminal_at = $3,
    error_code = $4,
    updated_at = $3
WHERE id = $1
  AND status = 'processing'
  AND lease_owner = $2::uuid
`, outbox.ID, ownerID, now.UTC(), decision.ErrorCode)
		if err != nil {
			return fmt.Errorf("mark invite mail outbox failed: %w", err)
		}
		return requireInviteMailDecisionRow(result, "failed")
	case inviteMailOutboxActionCancelled:
		result, err := tx.ExecContext(ctx, `
UPDATE identity_mail_outbox
SET status = 'cancelled',
    lease_owner = NULL,
    lease_expires_at = NULL,
    retry_at = NULL,
    terminal_at = $3,
    error_code = NULL,
    updated_at = $3
WHERE id = $1
  AND status = 'processing'
  AND lease_owner = $2::uuid
`, outbox.ID, ownerID, now.UTC())
		if err != nil {
			return fmt.Errorf("mark invite mail outbox cancelled: %w", err)
		}
		return requireInviteMailDecisionRow(result, "cancelled")
	default:
		return fmt.Errorf("unsupported invite mail outbox action %d", decision.Action)
	}
}

func requireInviteMailDecisionRow(result sql.Result, action string) error {
	if result == nil {
		return fmt.Errorf("mark invite mail outbox %s: missing SQL result", action)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark invite mail outbox %s rows affected: %w", action, err)
	}
	if rows != 1 {
		return fmt.Errorf(
			"mark invite mail outbox %s lost lease fence: affected %d rows",
			action,
			rows,
		)
	}
	return nil
}
