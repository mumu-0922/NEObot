package teams

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestPostgresInviteMailEndToEndRequiresSentThenAcceptsOnce(t *testing.T) {
	db := openInviteMailWorkerPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)
	inviterID := mustPostgresInviteMailUUID(t)
	inviterEmail := "e2e-inviter-" + strings.ReplaceAll(inviterID, "-", "") + "@example.test"
	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, email, display_name, account_status)
VALUES ($1, $2, 'End-to-end Inviter', 'active')
`, inviterID, inviterEmail); err != nil {
		t.Fatalf("insert end-to-end inviter: %v", err)
	}

	rawInviteToken := strings.Repeat("b", inviteTokenBytes*2)
	inviteEmail := "e2e-invitee-" + strings.ReplaceAll(mustPostgresInviteMailUUID(t), "-", "") + "@example.test"
	acceptanceURL := "https://app.example.test/auth/invites/accept#token=" + rawInviteToken
	cipher := newPostgresInviteMailCipher(t)
	teamService := NewService(
		NewPostgresRepository(db),
		WithMailCipher(cipher),
		WithInviteDeliveryGate(postgresInviteMailReadyGate{}),
		WithTeamServiceClock(func() time.Time { return now }),
		WithInviteTokenGenerator(func() (string, error) { return rawInviteToken, nil }),
		WithInviteURLBuilder(func(token string) (string, error) {
			return "https://app.example.test/auth/invites/accept#token=" + url.QueryEscape(token), nil
		}),
	)
	actorCtx := auth.WithUser(ctx, auth.User{
		ID:          inviterID,
		DisplayName: "End-to-end Inviter",
		Role:        "user",
	})
	team, err := teamService.CreateTeam(actorCtx, CreateTeamInput{
		Name:           "Invite Mail End-to-end Team",
		IdempotencyKey: "mail-e2e-team",
	})
	if err != nil {
		t.Fatalf("Team Service CreateTeam() error = %v", err)
	}
	invite, err := teamService.CreateInvite(actorCtx, team.ID, CreateTeamInviteInput{
		Email:          inviteEmail,
		TeamRole:       TeamRoleMember,
		IdempotencyKey: "mail-e2e-invite",
	})
	if err != nil {
		t.Fatalf("Team Service CreateInvite() error = %v", err)
	}
	if invite.Status != InviteStatusPending || invite.DeliveryStatus != InviteDeliveryPending {
		t.Fatalf("created invite status/delivery = %q/%q, want pending/pending", invite.Status, invite.DeliveryStatus)
	}

	var (
		outboxID         string
		outboxStatus     string
		outboxMessageID  string
		outboxCiphertext []byte
		storedTokenHash  string
	)
	if err := db.QueryRowContext(ctx, `
SELECT o.id, o.status, o.message_id, o.ciphertext, i.token_hash
FROM team_invites i
JOIN identity_mail_outbox o
  ON o.team_id = i.team_id
 AND o.invite_id = i.id
WHERE i.id = $1
`, invite.ID).Scan(
		&outboxID,
		&outboxStatus,
		&outboxMessageID,
		&outboxCiphertext,
		&storedTokenHash,
	); err != nil {
		t.Fatalf("query atomically-created invite/outbox: %v", err)
	}
	if outboxStatus != InviteDeliveryPending || len(outboxCiphertext) == 0 {
		t.Fatalf("created outbox status/ciphertext bytes = %q/%d", outboxStatus, len(outboxCiphertext))
	}
	if storedTokenHash != HashInviteToken(rawInviteToken) {
		t.Fatalf("stored token hash = %q, want SHA-256 of generated token", storedTokenHash)
	}
	if bytes.Contains(outboxCiphertext, []byte(rawInviteToken)) ||
		bytes.Contains(outboxCiphertext, []byte(acceptanceURL)) {
		t.Fatal("encrypted outbox contains plaintext invite token or acceptance URL")
	}

	authService := auth.NewService(
		auth.NewPostgresSessionRepository(db),
		auth.WithServiceClock(func() time.Time { return now }),
	)
	password := "end-to-end invite password"
	_, err = authService.AcceptInvite(ctx, auth.AcceptInviteInput{
		Token:     rawInviteToken,
		Password:  password,
		UserAgent: "mail-e2e-unsent",
	})
	if !errors.Is(err, auth.ErrInviteNotActive) {
		t.Fatalf("unsent Auth Service AcceptInvite() error = %v, want ErrInviteNotActive", err)
	}
	assertPostgresInviteMailAcceptanceNotMutated(t, ctx, db, invite.ID, inviteEmail, team.ID)

	transport := &postgresInviteMailTransport{
		ready:    true,
		db:       db,
		outboxID: outboxID,
	}
	// CreateInvite persists available_at with the database clock. Advance the
	// deterministic worker clock slightly so the just-committed row is due even
	// when the database clock is ahead of the process by a few milliseconds.
	sendNow := now.Add(time.Second)
	worker := newPostgresInviteMailWorker(
		t,
		db,
		cipher,
		transport,
		mustPostgresInviteMailUUID(t),
		func() time.Time { return sendNow },
	)
	processed, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("worker RunOnce() error = %v", err)
	}
	if !processed {
		t.Fatal("worker RunOnce() processed = false")
	}

	messages, observations, observationErr := transport.snapshot()
	if observationErr != nil {
		t.Fatalf("observe end-to-end send: %v", observationErr)
	}
	if len(messages) != 1 || len(observations) != 1 {
		t.Fatalf("end-to-end transport messages/observations = %d/%d, want 1/1", len(messages), len(observations))
	}
	capturedURL, capturedToken := extractPostgresInviteMailAcceptance(t, messages[0].TextBody)
	if capturedURL != acceptanceURL || capturedToken != rawInviteToken {
		t.Fatalf(
			"captured acceptance URL/token = %q/%q, want %q/%q",
			capturedURL,
			capturedToken,
			acceptanceURL,
			rawInviteToken,
		)
	}
	if messages[0].MessageID != outboxMessageID {
		t.Fatalf("sent Message-ID = %q, want persisted %q", messages[0].MessageID, outboxMessageID)
	}
	afterSend := loadPostgresInviteMailOutbox(t, ctx, db, outboxID)
	if afterSend.status != InviteDeliverySent ||
		!bytes.Equal(afterSend.ciphertext, outboxCiphertext) {
		t.Fatalf(
			"outbox after send status/ciphertext unchanged = %q/%t",
			afterSend.status,
			bytes.Equal(afterSend.ciphertext, outboxCiphertext),
		)
	}

	accepted, err := authService.AcceptInvite(ctx, auth.AcceptInviteInput{
		Token:     capturedToken,
		Password:  password,
		UserAgent: "mail-e2e-sent",
	})
	if err != nil {
		t.Fatalf("sent Auth Service AcceptInvite() error = %v", err)
	}
	if accepted.User.ID == "" || accepted.Token == "" || !accepted.ExpiresAt.After(now) {
		t.Fatalf("accepted login result = %#v", accepted)
	}
	assertPostgresInviteMailAcceptedState(
		t,
		ctx,
		db,
		invite.ID,
		team.ID,
		accepted.User.ID,
		outboxID,
	)

	_, err = authService.AcceptInvite(ctx, auth.AcceptInviteInput{
		Token:     capturedToken,
		Password:  password,
		UserAgent: "mail-e2e-replay",
	})
	if !errors.Is(err, auth.ErrInviteNotActive) {
		t.Fatalf("replayed Auth Service AcceptInvite() error = %v, want ErrInviteNotActive", err)
	}
	var sessionCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*)
FROM sessions
WHERE user_id = $1
`, accepted.User.ID).Scan(&sessionCount); err != nil {
		t.Fatalf("count accepted sessions: %v", err)
	}
	if sessionCount != 1 {
		t.Fatalf("accepted session count = %d, want 1", sessionCount)
	}
}

type postgresInviteMailReadyGate struct{}

func (postgresInviteMailReadyGate) AdmitInviteDelivery(context.Context) error {
	return nil
}

func extractPostgresInviteMailAcceptance(
	t *testing.T,
	body string,
) (string, string) {
	t.Helper()
	acceptanceURL, token, ok := parsePostgresInviteMailAcceptance(body)
	if !ok {
		t.Fatalf("invite message did not contain an HTTPS acceptance URL: %q", body)
	}
	return acceptanceURL, token
}

func parsePostgresInviteMailAcceptance(body string) (string, string, bool) {
	for _, line := range strings.Split(body, "\r\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parsed, err := url.Parse(line)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			continue
		}
		fragment, err := url.ParseQuery(parsed.Fragment)
		if err != nil || len(fragment) != 1 || len(fragment["token"]) != 1 {
			continue
		}
		token := fragment.Get("token")
		if _, err := NormalizeInviteToken(token); err != nil {
			continue
		}
		queryHasTokenKey := false
		for key := range parsed.Query() {
			if strings.EqualFold(strings.TrimSpace(key), "token") {
				queryHasTokenKey = true
				break
			}
		}
		if queryHasTokenKey {
			continue
		}
		if _, err := normalizeInviteAcceptanceURL(line, token); err != nil {
			continue
		}
		return line, token, true
	}
	return "", "", false
}

func TestPostgresInviteMailAcceptanceExtractorRequiresFragmentOnly(t *testing.T) {
	token := strings.Repeat("a", inviteTokenBytes*2)
	for _, testCase := range []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "fragment only",
			url:  "https://app.example.test/invites/accept#token=" + token,
			want: true,
		},
		{
			name: "safe base query plus fragment",
			url: "https://app.example.test/invites/accept?source=email#token=" +
				token,
			want: true,
		},
		{
			name: "token query key",
			url: "https://app.example.test/invites/accept?Token=other#token=" +
				token,
		},
		{
			name: "raw token in another query value",
			url: "https://app.example.test/invites/accept?value=" + token +
				"#token=" + token,
		},
		{
			name: "raw token in path",
			url: "https://app.example.test/invites/" + token +
				"/accept#token=" + token,
		},
		{
			name: "duplicate fragment token",
			url: "https://app.example.test/invites/accept#token=" + token +
				"&token=" + token,
		},
		{
			name: "extra fragment field",
			url: "https://app.example.test/invites/accept#token=" + token +
				"&source=email",
		},
		{
			name: "http is not accepted by hosted e2e",
			url:  "http://app.example.test/invites/accept#token=" + token,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			acceptanceURL, extractedToken, ok :=
				parsePostgresInviteMailAcceptance(testCase.url + "\r\n")
			if ok != testCase.want {
				t.Fatalf("parsePostgresInviteMailAcceptance() ok = %v, want %v", ok, testCase.want)
			}
			if testCase.want &&
				(acceptanceURL != testCase.url || extractedToken != token) {
				t.Fatalf(
					"parsePostgresInviteMailAcceptance() = %q/%q",
					acceptanceURL,
					extractedToken,
				)
			}
		})
	}
}

func assertPostgresInviteMailAcceptanceNotMutated(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	inviteID string,
	inviteEmail string,
	teamID string,
) {
	t.Helper()

	var inviteStatus, deliveryStatus string
	var membershipRevision int64
	if err := db.QueryRowContext(ctx, `
SELECT i.status, o.status, t.membership_revision
FROM team_invites i
JOIN identity_mail_outbox o ON o.invite_id = i.id
JOIN teams t ON t.id = i.team_id
WHERE i.id = $1
`, inviteID).Scan(&inviteStatus, &deliveryStatus, &membershipRevision); err != nil {
		t.Fatalf("query state after unsent acceptance: %v", err)
	}
	if inviteStatus != InviteStatusPending ||
		deliveryStatus != InviteDeliveryPending ||
		membershipRevision != 1 {
		t.Fatalf(
			"state after unsent acceptance = %q/%q/revision %d, want pending/pending/1",
			inviteStatus,
			deliveryStatus,
			membershipRevision,
		)
	}

	var identityCount, membershipCount int
	if err := db.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM users WHERE email = $1),
  (SELECT count(*)
   FROM team_memberships m
   JOIN users u ON u.id = m.user_id
   WHERE m.team_id = $2 AND u.email = $1)
`, inviteEmail, teamID).Scan(&identityCount, &membershipCount); err != nil {
		t.Fatalf("query unsent acceptance side effects: %v", err)
	}
	if identityCount != 0 || membershipCount != 0 {
		t.Fatalf(
			"unsent acceptance identity/membership count = %d/%d, want 0/0",
			identityCount,
			membershipCount,
		)
	}
}

func assertPostgresInviteMailAcceptedState(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	inviteID string,
	teamID string,
	userID string,
	outboxID string,
) {
	t.Helper()

	var (
		inviteStatus       string
		deliveryStatus     string
		membershipRole     string
		membershipStatus   string
		membershipRevision int64
		credentialCount    int
		sessionCount       int
	)
	if err := db.QueryRowContext(ctx, `
SELECT i.status,
       o.status,
       m.role,
       m.status,
       t.membership_revision,
       (SELECT count(*) FROM user_credentials c WHERE c.user_id = $3),
       (SELECT count(*) FROM sessions s WHERE s.user_id = $3)
FROM team_invites i
JOIN identity_mail_outbox o ON o.id = $4 AND o.invite_id = i.id
JOIN teams t ON t.id = i.team_id
JOIN team_memberships m ON m.team_id = t.id AND m.user_id = $3
WHERE i.id = $1
  AND i.team_id = $2
`, inviteID, teamID, userID, outboxID).Scan(
		&inviteStatus,
		&deliveryStatus,
		&membershipRole,
		&membershipStatus,
		&membershipRevision,
		&credentialCount,
		&sessionCount,
	); err != nil {
		t.Fatalf("query accepted invite state: %v", err)
	}
	if inviteStatus != InviteStatusAccepted ||
		deliveryStatus != InviteDeliverySent ||
		membershipRole != TeamRoleMember ||
		membershipStatus != MembershipStatusActive ||
		membershipRevision != 2 ||
		credentialCount != 1 ||
		sessionCount != 1 {
		t.Fatalf(
			"accepted state invite/delivery/role/status/revision/credential/session = %q/%q/%q/%q/%d/%d/%d",
			inviteStatus,
			deliveryStatus,
			membershipRole,
			membershipStatus,
			membershipRevision,
			credentialCount,
			sessionCount,
		)
	}
}

func TestPostgresInviteMailWorkerPendingToSentPreservesCiphertextAndMessageID(t *testing.T) {
	db := openInviteMailWorkerPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)
	cipher := newPostgresInviteMailCipher(t)
	fixture := insertPostgresInviteMailFixture(
		t,
		ctx,
		db,
		cipher,
		now,
		now.Add(time.Hour),
	)
	before := loadPostgresInviteMailOutbox(t, ctx, db, fixture.outboxID)
	transport := &postgresInviteMailTransport{
		ready:    true,
		db:       db,
		outboxID: fixture.outboxID,
	}
	ownerID := mustPostgresInviteMailUUID(t)
	worker := newPostgresInviteMailWorker(
		t,
		db,
		cipher,
		transport,
		ownerID,
		func() time.Time { return now },
	)

	processed, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if !processed {
		t.Fatal("RunOnce() processed = false")
	}

	messages, observations, observationErr := transport.snapshot()
	if observationErr != nil {
		t.Fatalf("observe outbox during bounded send: %v", observationErr)
	}
	if len(messages) != 1 || len(observations) != 1 {
		t.Fatalf("transport messages/observations = %d/%d, want 1/1", len(messages), len(observations))
	}
	observation := observations[0]
	if observation.status != InviteDeliveryProcessing ||
		observation.attemptCount != 1 ||
		!observation.leaseOwner.Valid ||
		observation.leaseOwner.String != ownerID {
		t.Fatalf("outbox observed during send = %#v", observation)
	}

	after := loadPostgresInviteMailOutbox(t, ctx, db, fixture.outboxID)
	if after.status != InviteDeliverySent || after.attemptCount != 1 {
		t.Fatalf("outbox status/attempts = %q/%d, want sent/1", after.status, after.attemptCount)
	}
	if after.leaseOwner.Valid || after.leaseExpiresAt.Valid || after.retryAt.Valid {
		t.Fatalf("sent outbox retained lease/retry state = %#v", after)
	}
	if !after.terminalAt.Valid || !after.terminalAt.Time.Equal(now) || after.errorCode.Valid {
		t.Fatalf("sent outbox terminal/error state = %#v", after)
	}
	if messages[0].MessageID != fixture.messageID || after.messageID != fixture.messageID {
		t.Fatalf(
			"Message-ID transport/row = %q/%q, want %q",
			messages[0].MessageID,
			after.messageID,
			fixture.messageID,
		)
	}
	if !bytes.Equal(before.ciphertext, fixture.ciphertext) ||
		!bytes.Equal(after.ciphertext, fixture.ciphertext) {
		t.Fatal("worker mutated the encrypted invite ciphertext")
	}
	if bytes.Equal(fixture.ciphertext, []byte(fixture.inviteToken)) {
		t.Fatal("outbox ciphertext unexpectedly equals the plaintext invite token")
	}
	if !strings.Contains(messages[0].TextBody, fixture.acceptanceURL) {
		t.Fatalf("invite message body does not contain acceptance URL: %q", messages[0].TextBody)
	}
}

func TestPostgresInviteMailWorkerRetryThenSuccess(t *testing.T) {
	db := openInviteMailWorkerPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)
	workerNow := now
	cipher := newPostgresInviteMailCipher(t)
	fixture := insertPostgresInviteMailFixture(
		t,
		ctx,
		db,
		cipher,
		now,
		now.Add(time.Hour),
	)
	transport := &postgresInviteMailTransport{
		ready:    true,
		db:       db,
		outboxID: fixture.outboxID,
		sendErrors: []error{
			fmt.Errorf("smtp relay password=do-not-persist: %w", auth.ErrSMTPDialFailed),
			nil,
		},
	}
	worker := newPostgresInviteMailWorker(
		t,
		db,
		cipher,
		transport,
		mustPostgresInviteMailUUID(t),
		func() time.Time { return workerNow },
	)

	processed, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("first RunOnce() error = %v", err)
	}
	if !processed {
		t.Fatal("first RunOnce() processed = false")
	}

	retrying := loadPostgresInviteMailOutbox(t, ctx, db, fixture.outboxID)
	wantRetryAt := now.Add(time.Second)
	if retrying.status != InviteDeliveryPending || retrying.attemptCount != 1 {
		t.Fatalf("retry outbox status/attempts = %q/%d, want pending/1", retrying.status, retrying.attemptCount)
	}
	if !retrying.retryAt.Valid || !retrying.retryAt.Time.Equal(wantRetryAt) ||
		!retrying.availableAt.Equal(wantRetryAt) {
		t.Fatalf(
			"retry_at/available_at = %v/%s, want %s",
			retrying.retryAt,
			retrying.availableAt,
			wantRetryAt,
		)
	}
	if !retrying.errorCode.Valid || retrying.errorCode.String != inviteMailErrorSMTPDialFailed {
		t.Fatalf("retry error_code = %v, want %q", retrying.errorCode, inviteMailErrorSMTPDialFailed)
	}
	if strings.Contains(retrying.errorCode.String, "password") ||
		strings.Contains(retrying.errorCode.String, "do-not-persist") {
		t.Fatalf("retry error_code persisted relay detail: %q", retrying.errorCode.String)
	}
	if retrying.leaseOwner.Valid || retrying.leaseExpiresAt.Valid || retrying.terminalAt.Valid {
		t.Fatalf("retry outbox retained lease/terminal state = %#v", retrying)
	}
	if !bytes.Equal(retrying.ciphertext, fixture.ciphertext) {
		t.Fatal("retry scheduling mutated the encrypted invite ciphertext")
	}

	processed, err = worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("early retry RunOnce() error = %v", err)
	}
	if processed {
		t.Fatal("RunOnce() processed retry before retry_at")
	}

	workerNow = wantRetryAt
	processed, err = worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("successful retry RunOnce() error = %v", err)
	}
	if !processed {
		t.Fatal("successful retry RunOnce() processed = false")
	}

	after := loadPostgresInviteMailOutbox(t, ctx, db, fixture.outboxID)
	if after.status != InviteDeliverySent || after.attemptCount != 2 || after.errorCode.Valid {
		t.Fatalf("outbox after successful retry = %#v", after)
	}
	if !after.terminalAt.Valid || !after.terminalAt.Time.Equal(wantRetryAt) || after.retryAt.Valid {
		t.Fatalf("successful retry terminal/retry state = %#v", after)
	}
	if !bytes.Equal(after.ciphertext, fixture.ciphertext) {
		t.Fatal("successful retry mutated the encrypted invite ciphertext")
	}

	messages, observations, observationErr := transport.snapshot()
	if observationErr != nil {
		t.Fatalf("observe retry sends: %v", observationErr)
	}
	if len(messages) != 2 || len(observations) != 2 {
		t.Fatalf("transport messages/observations = %d/%d, want 2/2", len(messages), len(observations))
	}
	for i, message := range messages {
		if message.MessageID != fixture.messageID {
			t.Fatalf("attempt %d Message-ID = %q, want %q", i+1, message.MessageID, fixture.messageID)
		}
	}
}

func TestPostgresInviteMailStoreReclaimsExpiredLeaseAndFencesOldOwner(t *testing.T) {
	db := openInviteMailWorkerPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)
	cipher := newPostgresInviteMailCipher(t)
	fixture := insertPostgresInviteMailFixture(
		t,
		ctx,
		db,
		cipher,
		now,
		now.Add(time.Hour),
	)
	oldOwnerID := mustPostgresInviteMailUUID(t)
	newOwnerID := mustPostgresInviteMailUUID(t)
	if _, err := db.ExecContext(ctx, `
UPDATE identity_mail_outbox
SET status = 'processing',
    attempt_count = 1,
    lease_owner = $2::uuid,
    lease_expires_at = $3,
    updated_at = $3
WHERE id = $1
`, fixture.outboxID, oldOwnerID, now.Add(-time.Minute)); err != nil {
		t.Fatalf("seed expired processing lease: %v", err)
	}

	store := &inviteMailOutboxSQLStore{db: db}
	claim, err := store.ClaimNext(ctx, newOwnerID, now, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("ClaimNext() reclaim error = %v", err)
	}
	if claim == nil || claim.ID != fixture.outboxID {
		t.Fatalf("ClaimNext() reclaim = %#v, want outbox %s", claim, fixture.outboxID)
	}

	reclaimed := loadPostgresInviteMailOutbox(t, ctx, db, fixture.outboxID)
	if reclaimed.status != InviteDeliveryProcessing ||
		reclaimed.attemptCount != 2 ||
		!reclaimed.leaseOwner.Valid ||
		reclaimed.leaseOwner.String != newOwnerID ||
		!reclaimed.leaseExpiresAt.Valid ||
		!reclaimed.leaseExpiresAt.Time.Equal(now.Add(time.Minute)) {
		t.Fatalf("reclaimed outbox = %#v", reclaimed)
	}

	var ownerObservedByStaleCommit string
	err = store.ProcessClaim(
		ctx,
		fixture.outboxID,
		oldOwnerID,
		now,
		func(outbox inviteMailLockedOutbox) (inviteMailOutboxDecision, error) {
			ownerObservedByStaleCommit = outbox.LeaseOwner
			return inviteMailFailedDecision(inviteMailErrorSMTPDeliveryFailed), nil
		},
	)
	if err != nil {
		t.Fatalf("old owner ProcessClaim() error = %v", err)
	}
	if ownerObservedByStaleCommit != "" {
		t.Fatalf("old owner callback ran after losing lease; observed owner %q", ownerObservedByStaleCommit)
	}

	afterStaleCommit := loadPostgresInviteMailOutbox(t, ctx, db, fixture.outboxID)
	if afterStaleCommit.status != InviteDeliveryProcessing ||
		afterStaleCommit.attemptCount != 2 ||
		!afterStaleCommit.leaseOwner.Valid ||
		afterStaleCommit.leaseOwner.String != newOwnerID ||
		afterStaleCommit.terminalAt.Valid ||
		afterStaleCommit.errorCode.Valid {
		t.Fatalf("old owner changed reclaimed outbox = %#v", afterStaleCommit)
	}

	err = store.ProcessClaim(
		ctx,
		fixture.outboxID,
		newOwnerID,
		now,
		func(outbox inviteMailLockedOutbox) (inviteMailOutboxDecision, error) {
			if outbox.Status != InviteDeliveryProcessing || outbox.LeaseOwner != newOwnerID {
				t.Fatalf("new owner locked outbox = %#v", outbox)
			}
			return inviteMailSentDecision(), nil
		},
	)
	if err != nil {
		t.Fatalf("new owner ProcessClaim() error = %v", err)
	}

	afterNewOwner := loadPostgresInviteMailOutbox(t, ctx, db, fixture.outboxID)
	if afterNewOwner.status != InviteDeliverySent ||
		afterNewOwner.attemptCount != 2 ||
		afterNewOwner.leaseOwner.Valid ||
		!afterNewOwner.terminalAt.Valid ||
		!afterNewOwner.terminalAt.Time.Equal(now) {
		t.Fatalf("new owner final outbox = %#v", afterNewOwner)
	}
}

func TestPostgresInviteMailWorkerCancelsExpiredPayloadBeforeSend(t *testing.T) {
	db := openInviteMailWorkerPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)
	cipher := newPostgresInviteMailCipher(t)
	fixture := insertPostgresInviteMailFixture(
		t,
		ctx,
		db,
		cipher,
		now,
		now.Add(-time.Minute),
	)
	transport := &postgresInviteMailTransport{
		ready:    true,
		db:       db,
		outboxID: fixture.outboxID,
	}
	worker := newPostgresInviteMailWorker(
		t,
		db,
		cipher,
		transport,
		mustPostgresInviteMailUUID(t),
		func() time.Time { return now },
	)

	processed, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if !processed {
		t.Fatal("RunOnce() processed = false")
	}

	messages, observations, observationErr := transport.snapshot()
	if observationErr != nil {
		t.Fatalf("unexpected transport observation error: %v", observationErr)
	}
	if len(messages) != 0 || len(observations) != 0 {
		t.Fatalf("expired invite reached transport: messages/observations = %d/%d", len(messages), len(observations))
	}
	after := loadPostgresInviteMailOutbox(t, ctx, db, fixture.outboxID)
	if after.status != InviteDeliveryCancelled ||
		after.attemptCount != 1 ||
		after.leaseOwner.Valid ||
		after.leaseExpiresAt.Valid ||
		after.retryAt.Valid ||
		after.errorCode.Valid ||
		!after.terminalAt.Valid ||
		!after.terminalAt.Time.Equal(now) {
		t.Fatalf("cancelled expired outbox = %#v", after)
	}
	if !bytes.Equal(after.ciphertext, fixture.ciphertext) {
		t.Fatal("expiry cancellation mutated the encrypted invite ciphertext")
	}
}

func TestPostgresInviteMailWorkersConcurrentClaimDoesNotDoubleSend(t *testing.T) {
	db := openInviteMailWorkerPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)
	cipher := newPostgresInviteMailCipher(t)
	fixture := insertPostgresInviteMailFixture(
		t,
		ctx,
		db,
		cipher,
		now,
		now.Add(time.Hour),
	)
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	transport := &postgresInviteMailTransport{
		ready:    true,
		db:       db,
		outboxID: fixture.outboxID,
		started:  started,
		release:  release,
	}
	workerA := newPostgresInviteMailWorker(
		t,
		db,
		cipher,
		transport,
		mustPostgresInviteMailUUID(t),
		func() time.Time { return now },
	)
	workerB := newPostgresInviteMailWorker(
		t,
		db,
		cipher,
		transport,
		mustPostgresInviteMailUUID(t),
		func() time.Time { return now },
	)

	type runResult struct {
		processed bool
		err       error
	}
	start := make(chan struct{})
	results := make(chan runResult, 2)
	for _, worker := range []*InviteMailOutboxWorker{workerA, workerB} {
		go func(worker *InviteMailOutboxWorker) {
			<-start
			processed, err := worker.RunOnce(ctx)
			results <- runResult{processed: processed, err: err}
		}(worker)
	}
	close(start)

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the claimed row to reach bounded send")
	}

	var loser runResult
	select {
	case loser = <-results:
	case <-time.After(5 * time.Second):
		t.Fatal("second worker did not skip the locked/leased due row")
	}
	if loser.err != nil || loser.processed {
		t.Fatalf("non-owner worker result = %#v, want processed=false without error", loser)
	}

	releaseOnce.Do(func() { close(release) })
	var winner runResult
	select {
	case winner = <-results:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the claiming worker")
	}
	if winner.err != nil || !winner.processed {
		t.Fatalf("owner worker result = %#v, want processed=true without error", winner)
	}

	messages, observations, observationErr := transport.snapshot()
	if observationErr != nil {
		t.Fatalf("observe concurrent send: %v", observationErr)
	}
	if len(messages) != 1 || len(observations) != 1 {
		t.Fatalf("concurrent transport messages/observations = %d/%d, want 1/1", len(messages), len(observations))
	}
	if observations[0].status != InviteDeliveryProcessing || observations[0].attemptCount != 1 {
		t.Fatalf("outbox during concurrent send = %#v", observations[0])
	}
	after := loadPostgresInviteMailOutbox(t, ctx, db, fixture.outboxID)
	if after.status != InviteDeliverySent || after.attemptCount != 1 {
		t.Fatalf("outbox after concurrent workers = %#v", after)
	}
}

type postgresInviteMailFixture struct {
	outboxID      string
	inviteToken   string
	messageID     string
	acceptanceURL string
	ciphertext    []byte
}

type postgresInviteMailOutboxState struct {
	status         string
	attemptCount   int
	availableAt    time.Time
	leaseOwner     sql.NullString
	leaseExpiresAt sql.NullTime
	retryAt        sql.NullTime
	terminalAt     sql.NullTime
	errorCode      sql.NullString
	messageID      string
	ciphertext     []byte
}

type postgresInviteMailSendObservation struct {
	status       string
	attemptCount int
	leaseOwner   sql.NullString
}

type postgresInviteMailTransport struct {
	ready      bool
	db         *sql.DB
	outboxID   string
	sendErrors []error
	started    chan struct{}
	release    <-chan struct{}

	startedOnce    sync.Once
	mu             sync.Mutex
	messages       []auth.SMTPPlaintextMessage
	observations   []postgresInviteMailSendObservation
	observationErr error
}

func (t *postgresInviteMailTransport) Ready() bool {
	return t != nil && t.ready
}

func (t *postgresInviteMailTransport) SendPlaintext(message auth.SMTPPlaintextMessage) error {
	var (
		observation postgresInviteMailSendObservation
		observeErr  error
	)
	if t.db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		observeErr = t.db.QueryRowContext(ctx, `
SELECT status, attempt_count, lease_owner::text
FROM identity_mail_outbox
WHERE id = $1
`, t.outboxID).Scan(
			&observation.status,
			&observation.attemptCount,
			&observation.leaseOwner,
		)
	}

	t.mu.Lock()
	call := len(t.messages)
	t.messages = append(t.messages, message)
	t.observations = append(t.observations, observation)
	if observeErr != nil && t.observationErr == nil {
		t.observationErr = observeErr
	}
	var sendErr error
	if call < len(t.sendErrors) {
		sendErr = t.sendErrors[call]
	}
	t.mu.Unlock()

	if t.started != nil {
		t.startedOnce.Do(func() { close(t.started) })
	}
	if t.release != nil {
		<-t.release
	}
	return sendErr
}

func (t *postgresInviteMailTransport) snapshot() (
	[]auth.SMTPPlaintextMessage,
	[]postgresInviteMailSendObservation,
	error,
) {
	t.mu.Lock()
	defer t.mu.Unlock()

	messages := append([]auth.SMTPPlaintextMessage(nil), t.messages...)
	observations := append([]postgresInviteMailSendObservation(nil), t.observations...)
	return messages, observations, t.observationErr
}

func newPostgresInviteMailWorker(
	t *testing.T,
	db *sql.DB,
	cipher *MailCipher,
	transport inviteMailSMTPTransport,
	ownerID string,
	now func() time.Time,
) *InviteMailOutboxWorker {
	t.Helper()

	worker, err := newInviteMailOutboxWorker(
		&inviteMailOutboxSQLStore{db: db},
		cipher,
		transport,
		WithInviteMailWorkerClock(now),
		WithInviteMailWorkerOwnerID(ownerID),
		WithInviteMailWorkerLeaseDuration(time.Minute),
		WithInviteMailWorkerBackoff(time.Second, time.Minute),
		WithInviteMailWorkerRandom(func() float64 { return 0 }),
	)
	if err != nil {
		t.Fatalf("newInviteMailOutboxWorker() error = %v", err)
	}
	return worker
}

func newPostgresInviteMailCipher(t *testing.T) *MailCipher {
	t.Helper()

	cipher, err := NewMailCipher(MailKeyring{
		ActiveKeyID: "postgres-test-key",
		Keys: map[string][]byte{
			"postgres-test-key": []byte("0123456789abcdef0123456789abcdef"),
		},
	})
	if err != nil {
		t.Fatalf("NewMailCipher() error = %v", err)
	}
	return cipher
}

func insertPostgresInviteMailFixture(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	cipher *MailCipher,
	now time.Time,
	expiresAt time.Time,
) postgresInviteMailFixture {
	t.Helper()

	inviterID := mustPostgresInviteMailUUID(t)
	teamID := mustPostgresInviteMailUUID(t)
	inviteID := mustPostgresInviteMailUUID(t)
	outboxID := mustPostgresInviteMailUUID(t)
	inviteToken := strings.Repeat("a", inviteTokenBytes*2)
	email := "invitee-" + strings.ReplaceAll(inviteID, "-", "") + "@example.test"
	acceptanceURL := "https://app.example.test/invites/accept#token=" + inviteToken
	messageID := inviteMessageID(outboxID)
	createdAt := now.Add(-2 * time.Hour)

	encrypted, err := cipher.EncryptInvitePayload(
		outboxID,
		inviteID,
		teamID,
		InviteMailPayload{
			Email:                email,
			InviteToken:          inviteToken,
			AcceptanceURL:        acceptanceURL,
			TeamID:               teamID,
			InvitedByUserID:      inviterID,
			InvitedByDisplayName: "Postgres Inviter",
			TeamRole:             TeamRoleMember,
			ExpiresAt:            expiresAt,
		},
	)
	if err != nil {
		t.Fatalf("EncryptInvitePayload() error = %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, email, display_name, account_status, created_at, updated_at)
VALUES ($1, $2, 'Postgres Inviter', 'active', $3, $3)
`, inviterID, "inviter-"+strings.ReplaceAll(inviterID, "-", "")+"@example.test", createdAt); err != nil {
		t.Fatalf("insert mail worker user fixture: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO teams (id, name, created_by_user_id, created_at, updated_at)
VALUES ($1, 'Postgres Mail Worker Team', $2, $3, $3)
`, teamID, inviterID, createdAt); err != nil {
		t.Fatalf("insert mail worker team fixture: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO team_invites (
  id,
  team_id,
  invited_by_user_id,
  token_hash,
  email,
  role,
  status,
  expires_at,
  created_at,
  updated_at
) VALUES ($1, $2, $3, $4, $5, 'member', 'pending', $6, $7, $7)
`, inviteID, teamID, inviterID, HashInviteToken(inviteToken), email, expiresAt, createdAt); err != nil {
		t.Fatalf("insert mail worker invite fixture: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO identity_mail_outbox (
  id,
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
  available_at,
  created_at,
  updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending', 0, 8, $9, $10, $10)
`,
		outboxID,
		teamID,
		inviteID,
		encrypted.KeyID,
		encrypted.Version,
		encrypted.Nonce,
		encrypted.Ciphertext,
		messageID,
		now.Add(-time.Minute),
		createdAt,
	); err != nil {
		t.Fatalf("insert mail worker outbox fixture: %v", err)
	}

	return postgresInviteMailFixture{
		outboxID:      outboxID,
		inviteToken:   inviteToken,
		messageID:     messageID,
		acceptanceURL: acceptanceURL,
		ciphertext:    append([]byte(nil), encrypted.Ciphertext...),
	}
}

func loadPostgresInviteMailOutbox(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	outboxID string,
) postgresInviteMailOutboxState {
	t.Helper()

	var state postgresInviteMailOutboxState
	if err := db.QueryRowContext(ctx, `
SELECT status,
       attempt_count,
       available_at,
       lease_owner::text,
       lease_expires_at,
       retry_at,
       terminal_at,
       error_code,
       message_id,
       ciphertext
FROM identity_mail_outbox
WHERE id = $1
`, outboxID).Scan(
		&state.status,
		&state.attemptCount,
		&state.availableAt,
		&state.leaseOwner,
		&state.leaseExpiresAt,
		&state.retryAt,
		&state.terminalAt,
		&state.errorCode,
		&state.messageID,
		&state.ciphertext,
	); err != nil {
		t.Fatalf("query invite mail outbox %s: %v", outboxID, err)
	}
	state.availableAt = state.availableAt.UTC()
	if state.leaseExpiresAt.Valid {
		state.leaseExpiresAt.Time = state.leaseExpiresAt.Time.UTC()
	}
	if state.retryAt.Valid {
		state.retryAt.Time = state.retryAt.Time.UTC()
	}
	if state.terminalAt.Valid {
		state.terminalAt.Time = state.terminalAt.Time.UTC()
	}
	return state
}

func openInviteMailWorkerPostgresIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()

	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set MM_CHAT_TEST_DATABASE_URL to run Postgres integration tests")
	}

	adminConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse MM_CHAT_TEST_DATABASE_URL: %v", err)
	}
	adminConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	adminDB := stdlib.OpenDB(*adminConfig)
	t.Cleanup(func() { _ = adminDB.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := adminDB.PingContext(ctx); err != nil {
		t.Fatalf("ping integration database: %v", err)
	}

	schemaName := "teams_mail_worker_" + strings.ReplaceAll(mustPostgresInviteMailUUID(t), "-", "")
	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf(`CREATE SCHEMA "%s"`, schemaName)); err != nil {
		t.Fatalf("create isolated integration schema %s: %v", schemaName, err)
	}
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		if _, err := adminDB.ExecContext(
			dropCtx,
			fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, schemaName),
		); err != nil {
			t.Errorf("drop isolated integration schema %s: %v", schemaName, err)
		}
	})

	testConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse MM_CHAT_TEST_DATABASE_URL for isolated schema: %v", err)
	}
	testConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	if testConfig.RuntimeParams == nil {
		testConfig.RuntimeParams = make(map[string]string)
	}
	testConfig.RuntimeParams["search_path"] = schemaName
	db := stdlib.OpenDB(*testConfig)
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	t.Cleanup(func() { _ = db.Close() })

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping isolated schema integration database: %v", err)
	}
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatalf("apply migrations in isolated schema: %v", err)
	}
	return db
}

func mustPostgresInviteMailUUID(t *testing.T) string {
	t.Helper()

	id, err := newUUID()
	if err != nil {
		t.Fatalf("newUUID() error = %v", err)
	}
	return id
}
