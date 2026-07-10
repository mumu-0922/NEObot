package teams

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

func TestServiceCreateInviteEncryptsPayloadAndReturnsMaskedEmail(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	repo := &fakeTeamsRepository{}
	mailCipher, err := NewMailCipher(MailKeyring{
		ActiveKeyID: "mail-active",
		Keys: map[string][]byte{
			"mail-active": repeatedKey('k'),
		},
	})
	if err != nil {
		t.Fatalf("NewMailCipher() error = %v", err)
	}

	service := NewService(
		repo,
		WithMailCipher(mailCipher),
		WithInviteDeliveryGate(&fakeInviteDeliveryGate{}),
		WithTeamServiceClock(func() time.Time { return now }),
		WithInviteTokenGenerator(func() (string, error) {
			return testInviteToken('a'), nil
		}),
		WithTeamIDGenerator(testIDGenerator(
			"22222222-2222-4222-8222-222222222222",
			"33333333-3333-4333-8333-333333333333",
		)),
		WithInviteURLBuilder(func(token string) (string, error) {
			return "https://example.test/accept#token=" + token, nil
		}),
	)
	ctx := auth.WithUser(context.Background(), auth.User{
		ID:          "11111111-1111-4111-8111-111111111111",
		DisplayName: "Team Admin",
	})

	invite, err := service.CreateInvite(ctx, "44444444-4444-4444-8444-444444444444", CreateTeamInviteInput{
		Email:          " Owner@Example.Test ",
		TeamRole:       " Admin ",
		IdempotencyKey: " invite-key ",
	})
	if err != nil {
		t.Fatalf("CreateInvite() error = %v", err)
	}
	if invite.MaskedEmail != "o***@e***.test" || invite.TeamRole != TeamRoleAdmin {
		t.Fatalf("CreateInvite() invite = %#v", invite)
	}
	if repo.createInviteInput.Email != "owner@example.test" ||
		repo.createInviteInput.TeamRole != TeamRoleAdmin ||
		repo.createInviteInput.TokenHash != HashInviteToken(testInviteToken('a')) ||
		repo.createInviteInput.IdempotencyKey != "invite-key" {
		t.Fatalf("CreateInvite() repository input = %#v", repo.createInviteInput)
	}
	if repo.createInviteInput.MailOutbox.MessageID !=
		"<invite-33333333-3333-4333-8333-333333333333@mm-chat.invalid>" {
		t.Fatalf("CreateInvite() message id = %q", repo.createInviteInput.MailOutbox.MessageID)
	}

	payload, err := mailCipher.DecryptInvitePayload(
		repo.createInviteInput.MailOutbox.ID,
		repo.createInviteInput.ID,
		repo.createInviteInput.TeamID,
		EncryptedMailPayload{
			KeyID:      repo.createInviteInput.MailOutbox.KeyID,
			Version:    repo.createInviteInput.MailOutbox.Version,
			Nonce:      repo.createInviteInput.MailOutbox.Nonce,
			Ciphertext: repo.createInviteInput.MailOutbox.Ciphertext,
		},
	)
	if err != nil {
		t.Fatalf("DecryptInvitePayload() error = %v", err)
	}
	if payload.Email != "owner@example.test" ||
		payload.InviteToken != testInviteToken('a') ||
		!strings.Contains(payload.AcceptanceURL, testInviteToken('a')) ||
		payload.InvitedByUserID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("mail payload = %#v", payload)
	}
}

func TestServiceCreateInviteFailsClosedWhenDeliveryUnavailable(t *testing.T) {
	repo := &fakeTeamsRepository{}
	service := NewService(repo)
	ctx := auth.WithUser(context.Background(), auth.User{
		ID: "11111111-1111-4111-8111-111111111111",
	})

	_, err := service.CreateInvite(ctx, "44444444-4444-4444-8444-444444444444", CreateTeamInviteInput{
		Email:          "owner@example.test",
		TeamRole:       TeamRoleMember,
		IdempotencyKey: "invite-key",
	})
	if !errors.Is(err, ErrInviteDeliveryUnavailable) {
		t.Fatalf("CreateInvite() error = %v, want ErrInviteDeliveryUnavailable", err)
	}
	if repo.createInviteCalled {
		t.Fatal("CreateInvite() wrote invite despite unavailable delivery")
	}
}

func TestServiceCreateInvitePreflightsTeamVisibilityBeforeDeliveryGate(t *testing.T) {
	repo := &fakeTeamsRepository{getTeamErr: ErrTeamNotFound}
	delivery := &fakeInviteDeliveryGate{err: ErrInviteDeliveryUnavailable}
	mailCipher, err := NewMailCipher(MailKeyring{
		ActiveKeyID: "mail-active",
		Keys: map[string][]byte{
			"mail-active": repeatedKey('m'),
		},
	})
	if err != nil {
		t.Fatalf("NewMailCipher() error = %v", err)
	}
	tokenCalls := 0
	service := NewService(
		repo,
		WithMailCipher(mailCipher),
		WithInviteDeliveryGate(delivery),
		WithInviteTokenGenerator(func() (string, error) {
			tokenCalls++
			return testInviteToken('b'), nil
		}),
		WithInviteURLBuilder(func(token string) (string, error) {
			return "https://example.test/accept#token=" + token, nil
		}),
	)
	ctx := auth.WithUser(context.Background(), auth.User{
		ID: "11111111-1111-4111-8111-111111111111",
	})

	_, err = service.CreateInvite(ctx, "44444444-4444-4444-8444-444444444444", CreateTeamInviteInput{
		Email:          "owner@example.test",
		TeamRole:       TeamRoleMember,
		IdempotencyKey: "invite-key",
	})
	if !errors.Is(err, ErrTeamNotFound) {
		t.Fatalf("CreateInvite() error = %v, want ErrTeamNotFound", err)
	}
	if delivery.calls != 0 {
		t.Fatalf("delivery gate calls = %d, want 0", delivery.calls)
	}
	if tokenCalls != 0 {
		t.Fatalf("invite token calls = %d, want 0", tokenCalls)
	}
	if repo.createInviteCalled {
		t.Fatal("CreateInvite() should not reach repository create")
	}
}

func TestServiceCreateInviteRequiresAdminBeforeDeliveryGate(t *testing.T) {
	repo := &fakeTeamsRepository{
		getTeamResult: Team{
			ID: "44444444-4444-4444-8444-444444444444",
			MyMembership: TeamMembership{
				TeamRole: TeamRoleMember,
				Status:   MembershipStatusActive,
			},
		},
	}
	delivery := &fakeInviteDeliveryGate{err: ErrInviteDeliveryUnavailable}
	mailCipher, err := NewMailCipher(MailKeyring{
		ActiveKeyID: "mail-active",
		Keys: map[string][]byte{
			"mail-active": repeatedKey('n'),
		},
	})
	if err != nil {
		t.Fatalf("NewMailCipher() error = %v", err)
	}
	tokenCalls := 0
	service := NewService(
		repo,
		WithMailCipher(mailCipher),
		WithInviteDeliveryGate(delivery),
		WithInviteTokenGenerator(func() (string, error) {
			tokenCalls++
			return testInviteToken('c'), nil
		}),
		WithInviteURLBuilder(func(token string) (string, error) {
			return "https://example.test/accept#token=" + token, nil
		}),
	)
	ctx := auth.WithUser(context.Background(), auth.User{
		ID: "11111111-1111-4111-8111-111111111111",
	})

	_, err = service.CreateInvite(ctx, "44444444-4444-4444-8444-444444444444", CreateTeamInviteInput{
		Email:          "owner@example.test",
		TeamRole:       TeamRoleMember,
		IdempotencyKey: "invite-key",
	})
	if !errors.Is(err, ErrTeamAdminRequired) {
		t.Fatalf("CreateInvite() error = %v, want ErrTeamAdminRequired", err)
	}
	if delivery.calls != 0 {
		t.Fatalf("delivery gate calls = %d, want 0", delivery.calls)
	}
	if tokenCalls != 0 {
		t.Fatalf("invite token calls = %d, want 0", tokenCalls)
	}
	if repo.createInviteCalled {
		t.Fatal("CreateInvite() should not reach repository create")
	}
}

func TestServiceListTeamsUsesDefaultLimitAndCursorBinding(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	repo := &fakeTeamsRepository{
		teamPageResult: TeamPageResult{
			Items: []Team{{
				ID:   "22222222-2222-4222-8222-222222222222",
				Name: "Research",
				MyMembership: TeamMembership{
					TeamRole:  TeamRoleAdmin,
					Status:    MembershipStatusActive,
					JoinedAt:  now,
					UpdatedAt: now,
				},
				MembershipRevision: 1,
				CreatedAt:          now,
				UpdatedAt:          now,
			}},
			HasMore: true,
		},
	}
	codec, err := NewCursorCodec(CursorKeyring{
		ActiveKeyID: "cursor-active",
		Keys: map[string][]byte{
			"cursor-active": repeatedKey('q'),
		},
	})
	if err != nil {
		t.Fatalf("NewCursorCodec() error = %v", err)
	}

	service := NewService(repo, WithCursorCodec(codec))
	ctx := auth.WithUser(context.Background(), auth.User{
		ID: "11111111-1111-4111-8111-111111111111",
	})

	page, err := service.ListTeams(ctx, ListTeamsInput{})
	if err != nil {
		t.Fatalf("ListTeams() first page error = %v", err)
	}
	if repo.teamPageInput.Limit != defaultPageLimit {
		t.Fatalf("ListTeams() default limit = %d, want %d", repo.teamPageInput.Limit, defaultPageLimit)
	}
	if page.NextCursor == "" {
		t.Fatal("ListTeams() nextCursor = empty, want cursor")
	}

	repo.teamPageResult = TeamPageResult{}
	if _, err := service.ListTeams(ctx, ListTeamsInput{
		Cursor: page.NextCursor,
		Limit:  10,
	}); err != nil {
		t.Fatalf("ListTeams() second page error = %v", err)
	}
	if repo.teamPageInput.After == nil ||
		repo.teamPageInput.After.ID != "22222222-2222-4222-8222-222222222222" ||
		!repo.teamPageInput.After.CreatedAt.Equal(now) {
		t.Fatalf("ListTeams() decoded cursor = %#v", repo.teamPageInput.After)
	}
}

func TestServiceValidationErrorsUseContractCodes(t *testing.T) {
	codec, err := NewCursorCodec(CursorKeyring{
		ActiveKeyID: "cursor-active",
		Keys: map[string][]byte{
			"cursor-active": repeatedKey('z'),
		},
	})
	if err != nil {
		t.Fatalf("NewCursorCodec() error = %v", err)
	}
	service := NewService(&fakeTeamsRepository{}, WithCursorCodec(codec))
	ctx := auth.WithUser(context.Background(), auth.User{
		ID: "11111111-1111-4111-8111-111111111111",
	})

	assertValidationCode(t, func() error {
		_, err := service.CreateTeam(ctx, CreateTeamInput{
			Name:           " ",
			IdempotencyKey: "team-key",
		})
		return err
	}(), ErrorCodeInvalidTeamPayload)

	assertValidationCode(t, func() error {
		_, err := service.CreateInvite(ctx, "44444444-4444-4444-8444-444444444444", CreateTeamInviteInput{
			Email:          "invalid",
			TeamRole:       TeamRoleAdmin,
			IdempotencyKey: "invite-key",
		})
		return err
	}(), ErrorCodeInvalidInvitePayload)

	assertValidationCode(t, func() error {
		_, err := service.UpdateMember(
			ctx,
			"44444444-4444-4444-8444-444444444444",
			"55555555-5555-4555-8555-555555555555",
			UpdateTeamMemberInput{TeamRole: "owner"},
		)
		return err
	}(), ErrorCodeInvalidMembershipPayload)

	assertValidationCode(t, func() error {
		_, err := service.ListTeams(ctx, ListTeamsInput{Limit: 101})
		return err
	}(), ErrorCodeInvalidTeamPayload)
}

func TestServiceRemoveMemberDelegatesSelfRemovalAuthorizationToRepository(t *testing.T) {
	repo := &fakeTeamsRepository{}
	service := NewService(repo)
	ctx := auth.WithUser(context.Background(), auth.User{
		ID: "11111111-1111-4111-8111-111111111111",
	})

	err := service.RemoveMember(
		ctx,
		"44444444-4444-4444-8444-444444444444",
		"11111111-1111-4111-8111-111111111111",
	)
	if err != nil {
		t.Fatalf("RemoveMember() error = %v", err)
	}
	if !repo.removeMemberCalled ||
		repo.removeMemberInput.ActorUserID != "11111111-1111-4111-8111-111111111111" ||
		repo.removeMemberInput.TargetUserID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("RemoveMember() repository input = %#v", repo.removeMemberInput)
	}
}

func TestServiceDisableAccountUsesFencedRepositoryBoundary(t *testing.T) {
	repo := &fakeTeamsRepository{}
	service := NewService(repo)
	userID := "11111111-1111-4111-8111-111111111111"

	revoked, err := service.DisableAccount(context.Background(), userID)
	if err != nil {
		t.Fatalf("DisableAccount() error = %v", err)
	}
	if repo.disableAccountUserID != userID || len(revoked) != 1 {
		t.Fatalf("DisableAccount() = (%#v, repo user %q)", revoked, repo.disableAccountUserID)
	}
	if _, err := service.DisableAccount(context.Background(), "not-a-uuid"); err == nil {
		t.Fatal("DisableAccount() invalid user id error = nil")
	}
}

type fakeTeamsRepository struct {
	createTeamInput      CreateTeamRepositoryInput
	teamPageInput        ListTeamsRepositoryInput
	getTeamInput         TeamLookupInput
	getTeamResult        Team
	getTeamErr           error
	renameTeamInput      RenameTeamRepositoryInput
	listMembersInput     ListMembersRepositoryInput
	updateMemberInput    UpdateMemberRepositoryInput
	removeMemberInput    RemoveMemberRepositoryInput
	leaveTeamInput       LeaveTeamRepositoryInput
	createInviteInput    CreateInviteRepositoryInput
	listInvitesInput     ListInvitesRepositoryInput
	revokeInviteInput    RevokeInviteRepositoryInput
	teamPageResult       TeamPageResult
	memberPageResult     TeamMemberPageResult
	invitePageResult     TeamInvitePageResult
	removeMemberCalled   bool
	createInviteCalled   bool
	disableAccountUserID string
}

func (r *fakeTeamsRepository) CreateTeam(_ context.Context, input CreateTeamRepositoryInput) (Team, error) {
	r.createTeamInput = input
	return Team{ID: input.ID, Name: input.Name}, nil
}

func (r *fakeTeamsRepository) ListTeams(_ context.Context, input ListTeamsRepositoryInput) (TeamPageResult, error) {
	r.teamPageInput = input
	return r.teamPageResult, nil
}

func (r *fakeTeamsRepository) GetTeam(_ context.Context, input TeamLookupInput) (Team, error) {
	r.getTeamInput = input
	if r.getTeamErr != nil {
		return Team{}, r.getTeamErr
	}
	if r.getTeamResult.ID != "" {
		return r.getTeamResult, nil
	}
	return Team{
		ID: input.TeamID,
		MyMembership: TeamMembership{
			TeamRole: TeamRoleAdmin,
			Status:   MembershipStatusActive,
		},
	}, nil
}

func (r *fakeTeamsRepository) RenameTeam(_ context.Context, input RenameTeamRepositoryInput) (Team, error) {
	r.renameTeamInput = input
	return Team{ID: input.TeamID, Name: input.Name}, nil
}

func (r *fakeTeamsRepository) ListMembers(_ context.Context, input ListMembersRepositoryInput) (TeamMemberPageResult, error) {
	r.listMembersInput = input
	return r.memberPageResult, nil
}

func (r *fakeTeamsRepository) UpdateMemberRole(_ context.Context, input UpdateMemberRepositoryInput) (TeamMember, error) {
	r.updateMemberInput = input
	return TeamMember{UserID: input.TargetUserID, TeamRole: input.TeamRole}, nil
}

func (r *fakeTeamsRepository) RemoveMember(_ context.Context, input RemoveMemberRepositoryInput) error {
	r.removeMemberCalled = true
	r.removeMemberInput = input
	return nil
}

func (r *fakeTeamsRepository) LeaveTeam(_ context.Context, input LeaveTeamRepositoryInput) error {
	r.leaveTeamInput = input
	return nil
}

func (r *fakeTeamsRepository) CreateInvite(_ context.Context, input CreateInviteRepositoryInput) (TeamInviteRecord, error) {
	r.createInviteCalled = true
	r.createInviteInput = input
	return TeamInviteRecord{
		ID:             input.ID,
		TeamID:         input.TeamID,
		Email:          input.Email,
		TeamRole:       input.TeamRole,
		Status:         InviteStatusPending,
		DeliveryStatus: InviteDeliveryPending,
		ExpiresAt:      input.ExpiresAt,
		CreatedAt:      input.ExpiresAt.Add(-defaultInviteTTL),
		UpdatedAt:      input.ExpiresAt.Add(-defaultInviteTTL),
	}, nil
}

func (r *fakeTeamsRepository) ListInvites(_ context.Context, input ListInvitesRepositoryInput) (TeamInvitePageResult, error) {
	r.listInvitesInput = input
	return r.invitePageResult, nil
}

func (r *fakeTeamsRepository) RevokeInvite(_ context.Context, input RevokeInviteRepositoryInput) error {
	r.revokeInviteInput = input
	return nil
}

func (r *fakeTeamsRepository) DisableAccount(
	_ context.Context,
	userID string,
) ([]auth.RevokedSession, error) {
	r.disableAccountUserID = userID
	return []auth.RevokedSession{{ID: "session-1"}}, nil
}

type fakeInviteDeliveryGate struct {
	err   error
	calls int
}

func (g *fakeInviteDeliveryGate) AdmitInviteDelivery(context.Context) error {
	g.calls++
	return g.err
}

func assertValidationCode(t *testing.T, err error, wantCode string) {
	t.Helper()
	var validationErr ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ValidationError(%s)", err, wantCode)
	}
	if validationErr.Code != wantCode {
		t.Fatalf("validation code = %q, want %q", validationErr.Code, wantCode)
	}
}

func repeatedKey(value byte) []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = value
	}
	return key
}

func testInviteToken(value byte) string {
	return strings.Repeat(string(value), inviteTokenBytes*2)
}

func testIDGenerator(ids ...string) func() (string, error) {
	index := 0
	return func() (string, error) {
		if index >= len(ids) {
			return "", fmt.Errorf("unexpected id request %d", index)
		}
		id := ids[index]
		index++
		return id, nil
	}
}
