package teams

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

func TestPhase15CPostgresCreateTeamIsAtomicAndIdempotent(t *testing.T) {
	fixture := newPhase15CPostgresFixture(t)
	repo := NewPostgresRepository(fixture.db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	creatorID := fixture.insertUser(ctx, "creator", "active")
	teamID := fixture.newTeamID()
	idempotencyKey := phase15CUniqueValue(t, "create-team")

	team, err := repo.CreateTeam(ctx, CreateTeamRepositoryInput{
		ID:              teamID,
		Name:            "Phase 15C Atomic Team",
		CreatedByUserID: creatorID,
		IdempotencyKey:  idempotencyKey,
	})
	if err != nil {
		t.Fatalf("CreateTeam() error = %v", err)
	}
	if team.ID != teamID || team.MembershipRevision != 1 {
		t.Fatalf("CreateTeam() team = %#v", team)
	}
	if team.MyMembership.TeamRole != TeamRoleAdmin ||
		team.MyMembership.Status != MembershipStatusActive {
		t.Fatalf("CreateTeam() membership = %#v", team.MyMembership)
	}

	var revision int64
	if err := fixture.db.QueryRowContext(ctx, `
SELECT membership_revision
FROM teams
WHERE id = $1
`, teamID).Scan(&revision); err != nil {
		t.Fatalf("query created team: %v", err)
	}
	if revision != 1 {
		t.Fatalf("membership_revision = %d, want 1", revision)
	}

	var role, status string
	if err := fixture.db.QueryRowContext(ctx, `
SELECT role, status
FROM team_memberships
WHERE team_id = $1
  AND user_id = $2
`, teamID, creatorID).Scan(&role, &status); err != nil {
		t.Fatalf("query creator membership: %v", err)
	}
	if role != TeamRoleAdmin || status != MembershipStatusActive {
		t.Fatalf("creator membership = (%q, %q), want (admin, active)", role, status)
	}

	payloads := loadTeamsMembershipChangedPayloads(t, ctx, fixture.db, teamID)
	if len(payloads) != 1 {
		t.Fatalf("create-team outbox payloads = %#v, want one", payloads)
	}
	assertPhase15CMembershipPayload(t, payloads[0], repositoryTeamMembershipChangedPayload{
		TeamID:             teamID,
		UserID:             creatorID,
		Operation:          membershipOperationAdded,
		TeamRole:           TeamRoleAdmin,
		Status:             MembershipStatusActive,
		MembershipRevision: 1,
	})

	conflictingTeamID := fixture.newTeamID()
	_, err = repo.CreateTeam(ctx, CreateTeamRepositoryInput{
		ID:              conflictingTeamID,
		Name:            "Must Not Be Created",
		CreatedByUserID: creatorID,
		IdempotencyKey:  idempotencyKey,
	})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("duplicate CreateTeam() error = %v, want ErrIdempotencyConflict", err)
	}
	if got := phase15CCount(t, ctx, fixture.db, `
SELECT count(*)
FROM teams
WHERE created_by_user_id = $1
  AND idempotency_key = $2
`, creatorID, idempotencyKey); got != 1 {
		t.Fatalf("teams for creator/idempotency = %d, want 1", got)
	}
	if got := phase15CCount(t, ctx, fixture.db, `
SELECT count(*) FROM teams WHERE id = $1
`, conflictingTeamID); got != 0 {
		t.Fatalf("conflicting team row count = %d, want 0", got)
	}

	rollbackTeamID := fixture.newTeamID()
	rollbackKey := phase15CUniqueValue(t, "create-team-rollback")
	rollbackRepo := NewPostgresRepository(fixture.db)
	rollbackRepo.newEventID = func() (string, error) { return "not-a-uuid", nil }
	_, err = rollbackRepo.CreateTeam(ctx, CreateTeamRepositoryInput{
		ID:              rollbackTeamID,
		Name:            "Rolled Back Team",
		CreatedByUserID: creatorID,
		IdempotencyKey:  rollbackKey,
	})
	if err == nil || !strings.Contains(err.Error(), "membership outbox event id must be a UUID") {
		t.Fatalf("rollback CreateTeam() error = %v, want invalid outbox id", err)
	}
	if got := phase15CCount(t, ctx, fixture.db, `
SELECT count(*) FROM teams WHERE id = $1
`, rollbackTeamID); got != 0 {
		t.Fatalf("rolled-back team row count = %d, want 0", got)
	}
	if got := phase15CCount(t, ctx, fixture.db, `
SELECT count(*) FROM team_memberships WHERE team_id = $1
`, rollbackTeamID); got != 0 {
		t.Fatalf("rolled-back membership count = %d, want 0", got)
	}
	if got := phase15CCount(t, ctx, fixture.db, `
SELECT count(*)
FROM knowledge_outbox
WHERE aggregate_type = 'team'
  AND aggregate_key = $1
`, rollbackTeamID); got != 0 {
		t.Fatalf("rolled-back outbox count = %d, want 0", got)
	}
}

func TestPhase15CPostgresVisibilityRolesLeaveAndRevisionSemantics(t *testing.T) {
	fixture := newPhase15CPostgresFixture(t)
	repo := NewPostgresRepository(fixture.db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adminID := fixture.insertUser(ctx, "admin", "active")
	memberID := fixture.insertUser(ctx, "member", "active")
	outsiderID := fixture.insertUser(ctx, "outsider", "active")
	disabledAdminID := fixture.insertUser(ctx, "disabled-admin", "disabled")
	teamID := fixture.insertTeam(ctx, adminID, "Phase 15C Visibility Team")
	insertTeamsMembershipFixture(t, ctx, fixture.db, teamID, adminID, TeamRoleAdmin)
	insertTeamsMembershipFixture(t, ctx, fixture.db, teamID, memberID, TeamRoleMember)

	adminTeam, err := repo.GetTeam(ctx, TeamLookupInput{TeamID: teamID, ActorUserID: adminID})
	if err != nil || adminTeam.MyMembership.TeamRole != TeamRoleAdmin {
		t.Fatalf("admin GetTeam() = (%#v, %v)", adminTeam, err)
	}
	memberTeam, err := repo.GetTeam(ctx, TeamLookupInput{TeamID: teamID, ActorUserID: memberID})
	if err != nil || memberTeam.MyMembership.TeamRole != TeamRoleMember {
		t.Fatalf("member GetTeam() = (%#v, %v)", memberTeam, err)
	}
	if _, err := repo.GetTeam(ctx, TeamLookupInput{
		TeamID: teamID, ActorUserID: outsiderID,
	}); !errors.Is(err, ErrTeamNotFound) {
		t.Fatalf("outsider GetTeam() error = %v, want ErrTeamNotFound", err)
	}

	for label, actorID := range map[string]string{"admin": adminID, "member": memberID} {
		page, err := repo.ListTeams(ctx, ListTeamsRepositoryInput{ActorUserID: actorID, Limit: 10})
		if err != nil {
			t.Fatalf("%s ListTeams() error = %v", label, err)
		}
		if len(page.Items) != 1 || page.Items[0].ID != teamID {
			t.Fatalf("%s ListTeams() items = %#v", label, page.Items)
		}
	}
	outsiderTeams, err := repo.ListTeams(ctx, ListTeamsRepositoryInput{
		ActorUserID: outsiderID,
		Limit:       10,
	})
	if err != nil || len(outsiderTeams.Items) != 0 {
		t.Fatalf("outsider ListTeams() = (%#v, %v), want empty", outsiderTeams, err)
	}

	members, err := repo.ListMembers(ctx, ListMembersRepositoryInput{
		TeamID:      teamID,
		ActorUserID: memberID,
		Limit:       10,
	})
	if err != nil || len(members.Items) != 2 {
		t.Fatalf("member ListMembers() = (%#v, %v)", members, err)
	}
	if _, err := repo.ListMembers(ctx, ListMembersRepositoryInput{
		TeamID: teamID, ActorUserID: outsiderID, Limit: 10,
	}); !errors.Is(err, ErrTeamNotFound) {
		t.Fatalf("outsider ListMembers() error = %v, want ErrTeamNotFound", err)
	}

	if _, err := repo.RenameTeam(ctx, RenameTeamRepositoryInput{
		TeamID: teamID, ActorUserID: memberID, Name: "Member Rename",
	}); !errors.Is(err, ErrTeamAdminRequired) {
		t.Fatalf("member RenameTeam() error = %v, want ErrTeamAdminRequired", err)
	}
	if _, err := repo.RenameTeam(ctx, RenameTeamRepositoryInput{
		TeamID: teamID, ActorUserID: outsiderID, Name: "Outsider Rename",
	}); !errors.Is(err, ErrTeamNotFound) {
		t.Fatalf("outsider RenameTeam() error = %v, want ErrTeamNotFound", err)
	}
	renamed, err := repo.RenameTeam(ctx, RenameTeamRepositoryInput{
		TeamID: teamID, ActorUserID: adminID, Name: "Admin Renamed Team",
	})
	if err != nil || renamed.Name != "Admin Renamed Team" {
		t.Fatalf("admin RenameTeam() = (%#v, %v)", renamed, err)
	}
	if revision := loadTeamsMembershipRevision(t, ctx, fixture.db, teamID); revision != 1 {
		t.Fatalf("revision after rename = %d, want 1", revision)
	}

	if _, err := repo.UpdateMemberRole(ctx, UpdateMemberRepositoryInput{
		TeamID:       teamID,
		ActorUserID:  memberID,
		TargetUserID: memberID,
		TeamRole:     TeamRoleAdmin,
	}); !errors.Is(err, ErrTeamAdminRequired) {
		t.Fatalf("member UpdateMemberRole() error = %v, want ErrTeamAdminRequired", err)
	}
	if _, err := repo.UpdateMemberRole(ctx, UpdateMemberRepositoryInput{
		TeamID:       teamID,
		ActorUserID:  outsiderID,
		TargetUserID: memberID,
		TeamRole:     TeamRoleAdmin,
	}); !errors.Is(err, ErrTeamNotFound) {
		t.Fatalf("outsider UpdateMemberRole() error = %v, want ErrTeamNotFound", err)
	}
	if err := repo.RemoveMember(ctx, RemoveMemberRepositoryInput{
		TeamID:       teamID,
		ActorUserID:  memberID,
		TargetUserID: memberID,
	}); !errors.Is(err, ErrTeamAdminRequired) {
		t.Fatalf("member self RemoveMember() error = %v, want ErrTeamAdminRequired", err)
	}
	if err := repo.RemoveMember(ctx, RemoveMemberRepositoryInput{
		TeamID:       teamID,
		ActorUserID:  outsiderID,
		TargetUserID: outsiderID,
	}); !errors.Is(err, ErrTeamNotFound) {
		t.Fatalf("outsider self RemoveMember() error = %v, want ErrTeamNotFound", err)
	}
	if err := repo.RemoveMember(ctx, RemoveMemberRepositoryInput{
		TeamID:       teamID,
		ActorUserID:  adminID,
		TargetUserID: adminID,
	}); err == nil {
		t.Fatal("admin self RemoveMember() error = nil, want self-leave validation")
	} else {
		var validationErr ValidationError
		if !errors.As(err, &validationErr) ||
			validationErr.Code != ErrorCodeInvalidMembershipPayload {
			t.Fatalf("admin self RemoveMember() error = %v, want INVALID_MEMBERSHIP_PAYLOAD", err)
		}
	}

	noop, err := repo.UpdateMemberRole(ctx, UpdateMemberRepositoryInput{
		TeamID:       teamID,
		ActorUserID:  adminID,
		TargetUserID: memberID,
		TeamRole:     TeamRoleMember,
	})
	if err != nil || noop.TeamRole != TeamRoleMember {
		t.Fatalf("no-op UpdateMemberRole() = (%#v, %v)", noop, err)
	}
	if revision := loadTeamsMembershipRevision(t, ctx, fixture.db, teamID); revision != 1 {
		t.Fatalf("revision after role no-op = %d, want 1", revision)
	}
	if payloads := loadTeamsMembershipChangedPayloads(t, ctx, fixture.db, teamID); len(payloads) != 0 {
		t.Fatalf("payloads after rename/no-op/rejections = %#v, want empty", payloads)
	}

	promoted, err := repo.UpdateMemberRole(ctx, UpdateMemberRepositoryInput{
		TeamID:       teamID,
		ActorUserID:  adminID,
		TargetUserID: memberID,
		TeamRole:     TeamRoleAdmin,
	})
	if err != nil || promoted.TeamRole != TeamRoleAdmin {
		t.Fatalf("promote UpdateMemberRole() = (%#v, %v)", promoted, err)
	}
	if revision := loadTeamsMembershipRevision(t, ctx, fixture.db, teamID); revision != 2 {
		t.Fatalf("revision after promotion = %d, want 2", revision)
	}

	insertTeamsMembershipFixture(t, ctx, fixture.db, teamID, disabledAdminID, TeamRoleAdmin)
	members, err = repo.ListMembers(ctx, ListMembersRepositoryInput{
		TeamID: teamID, ActorUserID: adminID, Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListMembers() with disabled membership error = %v", err)
	}
	for _, listed := range members.Items {
		if listed.UserID == disabledAdminID {
			t.Fatalf("ListMembers() exposed disabled account as active: %#v", listed)
		}
	}
	if err := repo.LeaveTeam(ctx, LeaveTeamRepositoryInput{
		TeamID: teamID, ActorUserID: memberID,
	}); err != nil {
		t.Fatalf("promoted admin LeaveTeam() error = %v", err)
	}
	if status := loadTeamsMembershipStatus(t, ctx, fixture.db, teamID, memberID); status != MembershipStatusRemoved {
		t.Fatalf("self-left membership status = %q, want removed", status)
	}
	if revision := loadTeamsMembershipRevision(t, ctx, fixture.db, teamID); revision != 3 {
		t.Fatalf("revision after self-leave = %d, want 3", revision)
	}

	if _, err := repo.UpdateMemberRole(ctx, UpdateMemberRepositoryInput{
		TeamID:       teamID,
		ActorUserID:  adminID,
		TargetUserID: adminID,
		TeamRole:     TeamRoleMember,
	}); !errors.Is(err, ErrLastTeamAdmin) {
		t.Fatalf("last usable admin demotion error = %v, want ErrLastTeamAdmin", err)
	}
	if err := repo.LeaveTeam(ctx, LeaveTeamRepositoryInput{
		TeamID: teamID, ActorUserID: adminID,
	}); !errors.Is(err, ErrLastTeamAdmin) {
		t.Fatalf("last usable admin LeaveTeam() error = %v, want ErrLastTeamAdmin", err)
	}

	payloads := loadTeamsMembershipChangedPayloads(t, ctx, fixture.db, teamID)
	if len(payloads) != 2 {
		t.Fatalf("membership payloads = %#v, want two", payloads)
	}
	assertPhase15CMembershipPayload(t, payloads[0], repositoryTeamMembershipChangedPayload{
		TeamID:             teamID,
		UserID:             memberID,
		Operation:          membershipOperationRoleChanged,
		TeamRole:           TeamRoleAdmin,
		Status:             MembershipStatusActive,
		MembershipRevision: 2,
	})
	assertPhase15CMembershipPayload(t, payloads[1], repositoryTeamMembershipChangedPayload{
		TeamID:             teamID,
		UserID:             memberID,
		Operation:          membershipOperationLeft,
		TeamRole:           TeamRoleAdmin,
		Status:             MembershipStatusRemoved,
		MembershipRevision: 3,
	})
	if revision := loadTeamsMembershipRevision(t, ctx, fixture.db, teamID); revision != payloads[len(payloads)-1].MembershipRevision {
		t.Fatalf(
			"team revision = %d, last outbox revision = %d",
			revision,
			payloads[len(payloads)-1].MembershipRevision,
		)
	}
}

func TestPhase15CPostgresConcurrentLastAdminMutationsKeepUsableAdmin(t *testing.T) {
	testCases := []struct {
		name      string
		operation string
		run       func(context.Context, *PostgresRepository, string, string, string) error
	}{
		{
			name:      "demote",
			operation: membershipOperationRoleChanged,
			run: func(ctx context.Context, repo *PostgresRepository, teamID, actorID, _ string) error {
				_, err := repo.UpdateMemberRole(ctx, UpdateMemberRepositoryInput{
					TeamID:       teamID,
					ActorUserID:  actorID,
					TargetUserID: actorID,
					TeamRole:     TeamRoleMember,
				})
				return err
			},
		},
		{
			name:      "remove",
			operation: membershipOperationRemoved,
			run: func(ctx context.Context, repo *PostgresRepository, teamID, actorID, otherID string) error {
				return repo.RemoveMember(ctx, RemoveMemberRepositoryInput{
					TeamID:       teamID,
					ActorUserID:  actorID,
					TargetUserID: otherID,
				})
			},
		},
		{
			name:      "leave",
			operation: membershipOperationLeft,
			run: func(ctx context.Context, repo *PostgresRepository, teamID, actorID, _ string) error {
				return repo.LeaveTeam(ctx, LeaveTeamRepositoryInput{
					TeamID: teamID, ActorUserID: actorID,
				})
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newPhase15CPostgresFixture(t)
			repo := NewPostgresRepository(fixture.db)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			adminAID := fixture.insertUser(ctx, "concurrent-admin-a", "active")
			adminBID := fixture.insertUser(ctx, "concurrent-admin-b", "active")
			teamID := fixture.insertTeam(ctx, adminAID, "Concurrent Last Admin Team")
			insertTeamsMembershipFixture(t, ctx, fixture.db, teamID, adminAID, TeamRoleAdmin)
			insertTeamsMembershipFixture(t, ctx, fixture.db, teamID, adminBID, TeamRoleAdmin)

			start := make(chan struct{})
			results := make(chan error, 2)
			go func() {
				<-start
				results <- testCase.run(ctx, repo, teamID, adminAID, adminBID)
			}()
			go func() {
				<-start
				results <- testCase.run(ctx, repo, teamID, adminBID, adminAID)
			}()
			close(start)

			successes := 0
			for i := 0; i < 2; i++ {
				err := <-results
				if err == nil {
					successes++
					continue
				}
				if !errors.Is(err, ErrLastTeamAdmin) &&
					!errors.Is(err, ErrTeamNotFound) &&
					!errors.Is(err, ErrTeamAdminRequired) {
					t.Fatalf("concurrent %s error = %v", testCase.name, err)
				}
			}
			if successes > 1 {
				t.Fatalf("concurrent %s successes = %d, want at most 1", testCase.name, successes)
			}
			if successes != 1 {
				t.Fatalf("concurrent %s successes = %d, want 1", testCase.name, successes)
			}

			usableAdmins := phase15CCount(t, ctx, fixture.db, `
SELECT count(*)
FROM team_memberships m
JOIN users u ON u.id = m.user_id
WHERE m.team_id = $1
  AND m.status = 'active'
  AND m.role = 'admin'
  AND u.account_status = 'active'
  AND u.deleted_at IS NULL
`, teamID)
			if usableAdmins < 1 {
				t.Fatalf("usable admin count after concurrent %s = %d", testCase.name, usableAdmins)
			}
			if revision := loadTeamsMembershipRevision(t, ctx, fixture.db, teamID); revision != 2 {
				t.Fatalf("revision after concurrent %s = %d, want 2", testCase.name, revision)
			}
			payloads := loadTeamsMembershipChangedPayloads(t, ctx, fixture.db, teamID)
			if len(payloads) != 1 {
				t.Fatalf("payloads after concurrent %s = %#v, want one", testCase.name, payloads)
			}
			if payloads[0].TeamID != teamID ||
				payloads[0].Operation != testCase.operation ||
				payloads[0].MembershipRevision != 2 {
				t.Fatalf("payload after concurrent %s = %#v", testCase.name, payloads[0])
			}
		})
	}
}

func TestPhase15CPostgresConcurrentSameRoleNoopDoesNotAdvanceRevision(t *testing.T) {
	fixture := newPhase15CPostgresFixture(t)
	repo := NewPostgresRepository(fixture.db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adminID := fixture.insertUser(ctx, "noop-admin", "active")
	memberID := fixture.insertUser(ctx, "noop-member", "active")
	teamID := fixture.insertTeam(ctx, adminID, "Concurrent No-op Team")
	insertTeamsMembershipFixture(t, ctx, fixture.db, teamID, adminID, TeamRoleAdmin)
	insertTeamsMembershipFixture(t, ctx, fixture.db, teamID, memberID, TeamRoleMember)

	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			_, err := repo.UpdateMemberRole(ctx, UpdateMemberRepositoryInput{
				TeamID:       teamID,
				ActorUserID:  adminID,
				TargetUserID: memberID,
				TeamRole:     TeamRoleMember,
			})
			results <- err
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatalf("concurrent no-op UpdateMemberRole() error = %v", err)
		}
	}

	if revision := loadTeamsMembershipRevision(t, ctx, fixture.db, teamID); revision != 1 {
		t.Fatalf("revision after concurrent role no-ops = %d, want 1", revision)
	}
	if payloads := loadTeamsMembershipChangedPayloads(t, ctx, fixture.db, teamID); len(payloads) != 0 {
		t.Fatalf("payloads after concurrent role no-ops = %#v, want empty", payloads)
	}
}

func TestPhase15CPostgresCreateInviteStoresHashAndEncryptedOutbox(t *testing.T) {
	fixture := newPhase15CPostgresFixture(t)
	repo := NewPostgresRepository(fixture.db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adminID := fixture.insertUser(ctx, "invite-admin", "active")
	teamID := fixture.insertTeam(ctx, adminID, "Encrypted Invite Team")
	insertTeamsMembershipFixture(t, ctx, fixture.db, teamID, adminID, TeamRoleAdmin)
	actorCtx := auth.WithUser(ctx, auth.User{ID: adminID, DisplayName: "Invite Admin"})
	mailCipher := phase15CNewMailCipher(t)

	email := phase15CUniqueEmail(t, "invitee")
	idempotencyKey := phase15CUniqueValue(t, "invite")
	attempt := newPhase15CInviteAttempt(t, repo, mailCipher)
	invite, err := attempt.service.CreateInvite(actorCtx, teamID, CreateTeamInviteInput{
		Email:          email,
		TeamRole:       TeamRoleMember,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		t.Fatalf("CreateInvite() error = %v", err)
	}
	wantMaskedEmail, err := MaskEmail(email)
	if err != nil {
		t.Fatalf("MaskEmail() error = %v", err)
	}
	if invite.ID != attempt.inviteID || invite.MaskedEmail != wantMaskedEmail ||
		invite.DeliveryStatus != InviteDeliveryPending {
		t.Fatalf("CreateInvite() invite = %#v", invite)
	}

	var storedTokenHash, storedEmail, storedIdempotency string
	var outboxID, keyID, messageID, deliveryStatus string
	var payloadVersion int
	var nonce, ciphertext []byte
	if err := fixture.db.QueryRowContext(ctx, `
SELECT
  i.token_hash,
  i.email,
  i.idempotency_key,
  o.id,
  o.key_id,
  o.payload_version,
  o.nonce,
  o.ciphertext,
  o.message_id,
  o.status
FROM team_invites i
JOIN identity_mail_outbox o ON o.invite_id = i.id
WHERE i.id = $1
`, attempt.inviteID).Scan(
		&storedTokenHash,
		&storedEmail,
		&storedIdempotency,
		&outboxID,
		&keyID,
		&payloadVersion,
		&nonce,
		&ciphertext,
		&messageID,
		&deliveryStatus,
	); err != nil {
		t.Fatalf("query stored invite/outbox: %v", err)
	}
	if storedTokenHash != HashInviteToken(attempt.rawToken) || storedTokenHash == attempt.rawToken {
		t.Fatalf("stored token hash = %q, raw token was persisted or hash mismatched", storedTokenHash)
	}
	if storedEmail != email || storedIdempotency != idempotencyKey {
		t.Fatalf("stored invite identity = (%q, %q)", storedEmail, storedIdempotency)
	}
	if outboxID != attempt.outboxID || deliveryStatus != InviteDeliveryPending || messageID == "" {
		t.Fatalf(
			"stored outbox = (id=%q, status=%q, messageID=%q)",
			outboxID,
			deliveryStatus,
			messageID,
		)
	}
	if bytes.Contains(ciphertext, []byte(attempt.rawToken)) {
		t.Fatal("identity_mail_outbox ciphertext contains the plaintext invite token")
	}
	if bytes.Contains(ciphertext, []byte(email)) {
		t.Fatal("identity_mail_outbox ciphertext contains the plaintext email")
	}

	payload, err := mailCipher.DecryptInvitePayload(
		outboxID,
		attempt.inviteID,
		teamID,
		EncryptedMailPayload{
			KeyID: keyID, Version: payloadVersion, Nonce: nonce, Ciphertext: ciphertext,
		},
	)
	if err != nil {
		t.Fatalf("DecryptInvitePayload() error = %v", err)
	}
	if payload.Email != email || payload.InviteToken != attempt.rawToken ||
		!strings.Contains(payload.AcceptanceURL, attempt.rawToken) {
		t.Fatalf("decrypted invite payload = %#v", payload)
	}
	encodedInvite, err := json.Marshal(invite)
	if err != nil {
		t.Fatalf("marshal safe invite response: %v", err)
	}
	if bytes.Contains(encodedInvite, []byte(attempt.rawToken)) || bytes.Contains(encodedInvite, []byte(email)) {
		t.Fatalf("safe invite response leaks plaintext: %s", encodedInvite)
	}

	idempotencyConflict := newPhase15CInviteAttempt(t, repo, mailCipher)
	_, err = idempotencyConflict.service.CreateInvite(actorCtx, teamID, CreateTeamInviteInput{
		Email:          phase15CUniqueEmail(t, "idempotency-conflict"),
		TeamRole:       TeamRoleAdmin,
		IdempotencyKey: idempotencyKey,
	})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("duplicate invite idempotency error = %v, want ErrIdempotencyConflict", err)
	}

	pendingConflict := newPhase15CInviteAttempt(t, repo, mailCipher)
	_, err = pendingConflict.service.CreateInvite(actorCtx, teamID, CreateTeamInviteInput{
		Email:          email,
		TeamRole:       TeamRoleAdmin,
		IdempotencyKey: phase15CUniqueValue(t, "pending-conflict"),
	})
	if !errors.Is(err, ErrInviteConflict) {
		t.Fatalf("duplicate pending invite error = %v, want ErrInviteConflict", err)
	}
	if got := phase15CCount(t, ctx, fixture.db, `
SELECT count(*) FROM team_invites WHERE team_id = $1
`, teamID); got != 1 {
		t.Fatalf("team invite count after conflicts = %d, want 1", got)
	}
	if got := phase15CCount(t, ctx, fixture.db, `
SELECT count(*) FROM identity_mail_outbox WHERE team_id = $1
`, teamID); got != 1 {
		t.Fatalf("mail outbox count after conflicts = %d, want 1", got)
	}
	if revision := loadTeamsMembershipRevision(t, ctx, fixture.db, teamID); revision != 1 {
		t.Fatalf("revision after invite creation/conflicts = %d, want 1", revision)
	}
}

func TestPhase15CPostgresRevokeInviteCancelsOutboxAtomically(t *testing.T) {
	fixture := newPhase15CPostgresFixture(t)
	repo := NewPostgresRepository(fixture.db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adminID := fixture.insertUser(ctx, "revoke-admin", "active")
	teamID := fixture.insertTeam(ctx, adminID, "Atomic Revoke Team")
	insertTeamsMembershipFixture(t, ctx, fixture.db, teamID, adminID, TeamRoleAdmin)
	mailCipher := phase15CNewMailCipher(t)
	attempt := newPhase15CInviteAttempt(t, repo, mailCipher)
	actorCtx := auth.WithUser(ctx, auth.User{ID: adminID, DisplayName: "Revoke Admin"})
	_, err := attempt.service.CreateInvite(actorCtx, teamID, CreateTeamInviteInput{
		Email:          phase15CUniqueEmail(t, "revoke-invitee"),
		TeamRole:       TeamRoleMember,
		IdempotencyKey: phase15CUniqueValue(t, "revoke-invite"),
	})
	if err != nil {
		t.Fatalf("CreateInvite() error = %v", err)
	}

	if _, err := fixture.db.ExecContext(ctx, `
UPDATE identity_mail_outbox
SET created_at = now() + interval '1 hour',
    available_at = now() + interval '1 hour',
    updated_at = now() + interval '1 hour'
WHERE id = $1
`, attempt.outboxID); err != nil {
		t.Fatalf("poison outbox timestamps: %v", err)
	}
	err = repo.RevokeInvite(ctx, RevokeInviteRepositoryInput{
		TeamID: teamID, ActorUserID: adminID, InviteID: attempt.inviteID,
	})
	if err == nil {
		t.Fatal("RevokeInvite() succeeded despite forced outbox cancellation failure")
	}
	assertPhase15CInviteAndOutboxStatus(
		t,
		ctx,
		fixture.db,
		attempt.inviteID,
		InviteStatusPending,
		InviteDeliveryPending,
		false,
		false,
	)

	if _, err := fixture.db.ExecContext(ctx, `
UPDATE identity_mail_outbox
SET created_at = now() - interval '1 minute',
    available_at = now(),
    updated_at = now()
WHERE id = $1
`, attempt.outboxID); err != nil {
		t.Fatalf("restore outbox timestamps: %v", err)
	}
	if err := repo.RevokeInvite(ctx, RevokeInviteRepositoryInput{
		TeamID: teamID, ActorUserID: adminID, InviteID: attempt.inviteID,
	}); err != nil {
		t.Fatalf("RevokeInvite() error = %v", err)
	}
	assertPhase15CInviteAndOutboxStatus(
		t,
		ctx,
		fixture.db,
		attempt.inviteID,
		InviteStatusRevoked,
		InviteDeliveryCancelled,
		true,
		true,
	)
	if revision := loadTeamsMembershipRevision(t, ctx, fixture.db, teamID); revision != 1 {
		t.Fatalf("revision after invite revoke = %d, want 1", revision)
	}
}

func TestPhase15CPostgresCreateInviteRollsBackOnOutboxInsertFailure(t *testing.T) {
	fixture := newPhase15CPostgresFixture(t)
	repo := NewPostgresRepository(fixture.db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adminID := fixture.insertUser(ctx, "outbox-rollback-admin", "active")
	teamID := fixture.insertTeam(ctx, adminID, "Outbox Rollback Team")
	insertTeamsMembershipFixture(t, ctx, fixture.db, teamID, adminID, TeamRoleAdmin)
	inviteID := mustTeamsTestUUID(t)
	outboxID := mustTeamsTestUUID(t)
	rawToken := phase15CInviteToken(t)
	idempotencyKey := phase15CUniqueValue(t, "outbox-rollback")

	_, err := repo.CreateInvite(ctx, CreateInviteRepositoryInput{
		ID:              inviteID,
		TeamID:          teamID,
		InvitedByUserID: adminID,
		Email:           phase15CUniqueEmail(t, "outbox-rollback-invitee"),
		TeamRole:        TeamRoleMember,
		IdempotencyKey:  idempotencyKey,
		TokenHash:       HashInviteToken(rawToken),
		ExpiresAt:       time.Now().UTC().Add(time.Hour),
		MailOutbox: IdentityMailOutboxInput{
			ID:         outboxID,
			MessageID:  fmt.Sprintf("<phase15c-%s@mm-chat.invalid>", outboxID),
			KeyID:      "phase15c-rollback",
			Version:    1,
			Nonce:      phase15CRandomBytes(t, 11),
			Ciphertext: phase15CRandomBytes(t, 64),
		},
	})
	if err == nil {
		t.Fatal("CreateInvite() succeeded with an invalid outbox nonce")
	}
	if got := phase15CCount(t, ctx, fixture.db, `
SELECT count(*)
FROM team_invites
WHERE id = $1
   OR (team_id = $2 AND invited_by_user_id = $3 AND idempotency_key = $4)
`, inviteID, teamID, adminID, idempotencyKey); got != 0 {
		t.Fatalf("rolled-back invite count = %d, want 0", got)
	}
	if got := phase15CCount(t, ctx, fixture.db, `
SELECT count(*) FROM identity_mail_outbox WHERE id = $1 OR invite_id = $2
`, outboxID, inviteID); got != 0 {
		t.Fatalf("rolled-back identity outbox count = %d, want 0", got)
	}
	if revision := loadTeamsMembershipRevision(t, ctx, fixture.db, teamID); revision != 1 {
		t.Fatalf("revision after rolled-back invite = %d, want 1", revision)
	}
	if payloads := loadTeamsMembershipChangedPayloads(t, ctx, fixture.db, teamID); len(payloads) != 0 {
		t.Fatalf("membership payloads after rolled-back invite = %#v, want empty", payloads)
	}
}

type phase15CPostgresFixture struct {
	t       *testing.T
	db      *sql.DB
	userIDs []string
	teamIDs []string
}

func newPhase15CPostgresFixture(t *testing.T) *phase15CPostgresFixture {
	t.Helper()

	fixture := &phase15CPostgresFixture{
		t:  t,
		db: openTeamsPostgresIntegrationDB(t),
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		for _, teamID := range fixture.teamIDs {
			if _, err := fixture.db.ExecContext(ctx, `
DELETE FROM knowledge_outbox
WHERE aggregate_type = 'team'
  AND aggregate_key = $1
`, teamID); err != nil {
				t.Logf("cleanup knowledge outbox for team %s: %v", teamID, err)
			}
			if _, err := fixture.db.ExecContext(ctx, `DELETE FROM teams WHERE id = $1`, teamID); err != nil {
				t.Logf("cleanup team %s: %v", teamID, err)
			}
		}
		for _, userID := range fixture.userIDs {
			if _, err := fixture.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
				t.Logf("cleanup user %s: %v", userID, err)
			}
		}
	})
	return fixture
}

func (f *phase15CPostgresFixture) insertUser(
	ctx context.Context,
	prefix string,
	accountStatus string,
) string {
	f.t.Helper()

	userID := mustTeamsTestUUID(f.t)
	insertTeamsUserFixture(
		f.t,
		ctx,
		f.db,
		userID,
		phase15CUniqueEmail(f.t, prefix),
		prefix+" "+phase15CUniqueValue(f.t, "display"),
		accountStatus,
	)
	f.userIDs = append(f.userIDs, userID)
	return userID
}

func (f *phase15CPostgresFixture) newTeamID() string {
	f.t.Helper()

	teamID := mustTeamsTestUUID(f.t)
	f.teamIDs = append(f.teamIDs, teamID)
	return teamID
}

func (f *phase15CPostgresFixture) insertTeam(
	ctx context.Context,
	creatorUserID string,
	name string,
) string {
	f.t.Helper()

	teamID := f.newTeamID()
	insertTeamsTeamFixture(f.t, ctx, f.db, teamID, creatorUserID, name)
	return teamID
}

type phase15CInviteAttempt struct {
	service  *Service
	inviteID string
	outboxID string
	rawToken string
}

func newPhase15CInviteAttempt(
	t *testing.T,
	repo Repository,
	mailCipher *MailCipher,
) phase15CInviteAttempt {
	t.Helper()

	inviteID := mustTeamsTestUUID(t)
	outboxID := mustTeamsTestUUID(t)
	rawToken := phase15CInviteToken(t)
	ids := []string{inviteID, outboxID}
	index := 0
	service := NewService(
		repo,
		WithMailCipher(mailCipher),
		WithInviteDeliveryGate(phase15CReadyDeliveryGate{}),
		WithTeamIDGenerator(func() (string, error) {
			if index >= len(ids) {
				return "", errors.New("phase15c id generator exhausted")
			}
			id := ids[index]
			index++
			return id, nil
		}),
		WithInviteTokenGenerator(func() (string, error) { return rawToken, nil }),
		WithInviteURLBuilder(func(token string) (string, error) {
			return "https://phase15c.example.test/invites/accept#token=" + token, nil
		}),
	)
	return phase15CInviteAttempt{
		service: service, inviteID: inviteID, outboxID: outboxID, rawToken: rawToken,
	}
}

type phase15CReadyDeliveryGate struct{}

func (phase15CReadyDeliveryGate) AdmitInviteDelivery(context.Context) error { return nil }

func phase15CNewMailCipher(t *testing.T) *MailCipher {
	t.Helper()

	keyID := phase15CUniqueValue(t, "mail-key")
	cipher, err := NewMailCipher(MailKeyring{
		ActiveKeyID: keyID,
		Keys: map[string][]byte{
			keyID: phase15CRandomBytes(t, 32),
		},
	})
	if err != nil {
		t.Fatalf("NewMailCipher() error = %v", err)
	}
	return cipher
}

func phase15CInviteToken(t *testing.T) string {
	t.Helper()

	token, err := GenerateInviteToken()
	if err != nil {
		t.Fatalf("GenerateInviteToken() error = %v", err)
	}
	return token
}

func phase15CRandomBytes(t *testing.T, size int) []byte {
	t.Helper()

	value := make([]byte, size)
	if _, err := cryptorand.Read(value); err != nil {
		t.Fatalf("crypto/rand.Read() error = %v", err)
	}
	return value
}

func phase15CUniqueValue(t *testing.T, prefix string) string {
	t.Helper()

	return prefix + "-" + strings.ReplaceAll(mustTeamsTestUUID(t), "-", "")
}

func phase15CUniqueEmail(t *testing.T, prefix string) string {
	t.Helper()

	return phase15CUniqueValue(t, prefix) + "@phase15c.example.test"
}

func phase15CCount(
	t *testing.T,
	ctx context.Context,
	db interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	query string,
	args ...any,
) int {
	t.Helper()

	var count int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		t.Fatalf("count phase15c rows: %v", err)
	}
	return count
}

func assertPhase15CMembershipPayload(
	t *testing.T,
	got repositoryTeamMembershipChangedPayload,
	want repositoryTeamMembershipChangedPayload,
) {
	t.Helper()

	if got != want {
		t.Fatalf("membership payload = %#v, want %#v", got, want)
	}
}

func assertPhase15CInviteAndOutboxStatus(
	t *testing.T,
	ctx context.Context,
	db interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	inviteID string,
	wantInviteStatus string,
	wantDeliveryStatus string,
	wantRevokedAt bool,
	wantTerminalAt bool,
) {
	t.Helper()

	var inviteStatus, deliveryStatus string
	var revokedAt, terminalAt bool
	if err := db.QueryRowContext(ctx, `
SELECT
  i.status,
  i.revoked_at IS NOT NULL,
  o.status,
  o.terminal_at IS NOT NULL
FROM team_invites i
JOIN identity_mail_outbox o ON o.invite_id = i.id
WHERE i.id = $1
`, inviteID).Scan(&inviteStatus, &revokedAt, &deliveryStatus, &terminalAt); err != nil {
		t.Fatalf("query invite/outbox status: %v", err)
	}
	if inviteStatus != wantInviteStatus || deliveryStatus != wantDeliveryStatus ||
		revokedAt != wantRevokedAt || terminalAt != wantTerminalAt {
		t.Fatalf(
			"invite/outbox status = (%q, %t, %q, %t), want (%q, %t, %q, %t)",
			inviteStatus,
			revokedAt,
			deliveryStatus,
			terminalAt,
			wantInviteStatus,
			wantRevokedAt,
			wantDeliveryStatus,
			wantTerminalAt,
		)
	}
}
