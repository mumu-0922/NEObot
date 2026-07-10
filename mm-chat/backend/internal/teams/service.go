package teams

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"neo-chat/mm-chat/backend/internal/auth"
)

const (
	defaultPageLimit      = 50
	maximumPageLimit      = 100
	maximumTeamNameRunes  = 100
	maximumTeamNameBytes  = 256
	maximumIdempotencyLen = 128
	defaultInviteTTL      = 72 * time.Hour
)

type Service struct {
	repo           Repository
	cursorCodec    *CursorCodec
	mailCipher     *MailCipher
	deliveryGate   InviteDeliveryGate
	now            func() time.Time
	newID          func() (string, error)
	newInviteToken func() (string, error)
	inviteTTL      time.Duration
	buildInviteURL func(token string) (string, error)
}

type ServiceOption func(*Service)

func WithCursorCodec(codec *CursorCodec) ServiceOption {
	return func(s *Service) {
		s.cursorCodec = codec
	}
}

func WithMailCipher(cipher *MailCipher) ServiceOption {
	return func(s *Service) {
		s.mailCipher = cipher
	}
}

func WithInviteDeliveryGate(gate InviteDeliveryGate) ServiceOption {
	return func(s *Service) {
		s.deliveryGate = gate
	}
}

func WithTeamServiceClock(now func() time.Time) ServiceOption {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func WithTeamIDGenerator(newID func() (string, error)) ServiceOption {
	return func(s *Service) {
		if newID != nil {
			s.newID = newID
		}
	}
}

func WithInviteTokenGenerator(newToken func() (string, error)) ServiceOption {
	return func(s *Service) {
		if newToken != nil {
			s.newInviteToken = newToken
		}
	}
}

func WithInviteTTL(ttl time.Duration) ServiceOption {
	return func(s *Service) {
		if ttl > 0 {
			s.inviteTTL = ttl
		}
	}
}

func WithInviteURLBuilder(builder func(token string) (string, error)) ServiceOption {
	return func(s *Service) {
		s.buildInviteURL = builder
	}
}

func NewService(repo Repository, opts ...ServiceOption) *Service {
	service := &Service{
		repo:           repo,
		now:            time.Now,
		newID:          newUUID,
		newInviteToken: GenerateInviteToken,
		inviteTTL:      defaultInviteTTL,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	if service.now == nil {
		service.now = time.Now
	}
	if service.newID == nil {
		service.newID = newUUID
	}
	if service.newInviteToken == nil {
		service.newInviteToken = GenerateInviteToken
	}
	if service.inviteTTL <= 0 {
		service.inviteTTL = defaultInviteTTL
	}
	return service
}

func (s *Service) CreateTeam(ctx context.Context, input CreateTeamInput) (Team, error) {
	if err := s.requireRepository(); err != nil {
		return Team{}, err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return Team{}, err
	}
	name, err := normalizeTeamName(input.Name)
	if err != nil {
		return Team{}, err
	}
	idempotencyKey, err := normalizeIdempotencyKey(
		input.IdempotencyKey,
		ErrorCodeInvalidTeamPayload,
	)
	if err != nil {
		return Team{}, err
	}
	teamID, err := s.generateID("team")
	if err != nil {
		return Team{}, err
	}

	return s.repo.CreateTeam(ctx, CreateTeamRepositoryInput{
		ID:              teamID,
		Name:            name,
		CreatedByUserID: actor.ID,
		IdempotencyKey:  idempotencyKey,
	})
}

func (s *Service) ListTeams(
	ctx context.Context,
	input ListTeamsInput,
) (ApiPage[Team], error) {
	if err := s.requireRepository(); err != nil {
		return ApiPage[Team]{}, err
	}
	if err := s.requireCursorCodec(); err != nil {
		return ApiPage[Team]{}, err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return ApiPage[Team]{}, err
	}
	limit, err := normalizePageLimit(input.Limit)
	if err != nil {
		return ApiPage[Team]{}, err
	}
	after, err := s.decodeTeamCursor(input.Cursor, actor.ID)
	if err != nil {
		return ApiPage[Team]{}, err
	}

	result, err := s.repo.ListTeams(ctx, ListTeamsRepositoryInput{
		ActorUserID: actor.ID,
		Limit:       limit,
		After:       after,
	})
	if err != nil {
		return ApiPage[Team]{}, err
	}
	page := ApiPage[Team]{Items: result.Items}
	if result.HasMore && len(result.Items) > 0 {
		nextCursor, err := s.cursorCodec.Encode(Cursor{
			Resource: cursorResourceTeams,
			UserID:   actor.ID,
			Sort:     cursorSortCreatedDesc,
			Values: []string{
				result.Items[len(result.Items)-1].CreatedAt.UTC().Format(time.RFC3339Nano),
				result.Items[len(result.Items)-1].ID,
			},
		})
		if err != nil {
			return ApiPage[Team]{}, fmt.Errorf("encode teams cursor: %w", err)
		}
		page.NextCursor = nextCursor
	}
	return page, nil
}

func (s *Service) GetTeam(ctx context.Context, teamID string) (Team, error) {
	if err := s.requireRepository(); err != nil {
		return Team{}, err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return Team{}, err
	}
	teamID, err = normalizeUUID(teamID, "team id")
	if err != nil {
		return Team{}, err
	}
	return s.repo.GetTeam(ctx, TeamLookupInput{
		TeamID:      teamID,
		ActorUserID: actor.ID,
	})
}

func (s *Service) RenameTeam(
	ctx context.Context,
	teamID string,
	input RenameTeamInput,
) (Team, error) {
	if err := s.requireRepository(); err != nil {
		return Team{}, err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return Team{}, err
	}
	teamID, err = normalizeUUID(teamID, "team id")
	if err != nil {
		return Team{}, err
	}
	name, err := normalizeTeamName(input.Name)
	if err != nil {
		return Team{}, err
	}
	return s.repo.RenameTeam(ctx, RenameTeamRepositoryInput{
		TeamID:      teamID,
		ActorUserID: actor.ID,
		Name:        name,
	})
}

func (s *Service) ListMembers(
	ctx context.Context,
	teamID string,
	input ListTeamMembersInput,
) (ApiPage[TeamMember], error) {
	if err := s.requireRepository(); err != nil {
		return ApiPage[TeamMember]{}, err
	}
	if err := s.requireCursorCodec(); err != nil {
		return ApiPage[TeamMember]{}, err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return ApiPage[TeamMember]{}, err
	}
	teamID, err = normalizeUUID(teamID, "team id")
	if err != nil {
		return ApiPage[TeamMember]{}, err
	}
	limit, err := normalizePageLimit(input.Limit)
	if err != nil {
		return ApiPage[TeamMember]{}, err
	}
	after, err := s.decodeMemberCursor(input.Cursor, actor.ID, teamID)
	if err != nil {
		return ApiPage[TeamMember]{}, err
	}

	result, err := s.repo.ListMembers(ctx, ListMembersRepositoryInput{
		TeamID:      teamID,
		ActorUserID: actor.ID,
		Limit:       limit,
		After:       after,
	})
	if err != nil {
		return ApiPage[TeamMember]{}, err
	}
	page := ApiPage[TeamMember]{Items: result.Items}
	if result.HasMore && len(result.Items) > 0 {
		nextCursor, err := s.cursorCodec.Encode(Cursor{
			Resource: cursorResourceTeamMembers,
			UserID:   actor.ID,
			TeamID:   teamID,
			Sort:     cursorSortMemberAsc,
			Values: []string{
				result.Items[len(result.Items)-1].JoinedAt.UTC().Format(time.RFC3339Nano),
				result.Items[len(result.Items)-1].UserID,
			},
		})
		if err != nil {
			return ApiPage[TeamMember]{}, fmt.Errorf("encode team members cursor: %w", err)
		}
		page.NextCursor = nextCursor
	}
	return page, nil
}

func (s *Service) UpdateMember(
	ctx context.Context,
	teamID string,
	userID string,
	input UpdateTeamMemberInput,
) (TeamMember, error) {
	if err := s.requireRepository(); err != nil {
		return TeamMember{}, err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return TeamMember{}, err
	}
	teamID, err = normalizeUUID(teamID, "team id")
	if err != nil {
		return TeamMember{}, err
	}
	userID, err = normalizeUUID(userID, "user id")
	if err != nil {
		return TeamMember{}, err
	}
	role, err := normalizeTeamRole(input.TeamRole, ErrorCodeInvalidMembershipPayload)
	if err != nil {
		return TeamMember{}, err
	}
	return s.repo.UpdateMemberRole(ctx, UpdateMemberRepositoryInput{
		TeamID:       teamID,
		ActorUserID:  actor.ID,
		TargetUserID: userID,
		TeamRole:     role,
	})
}

func (s *Service) RemoveMember(ctx context.Context, teamID string, userID string) error {
	if err := s.requireRepository(); err != nil {
		return err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return err
	}
	teamID, err = normalizeUUID(teamID, "team id")
	if err != nil {
		return err
	}
	userID, err = normalizeUUID(userID, "user id")
	if err != nil {
		return err
	}
	return s.repo.RemoveMember(ctx, RemoveMemberRepositoryInput{
		TeamID:       teamID,
		ActorUserID:  actor.ID,
		TargetUserID: userID,
	})
}

func (s *Service) LeaveTeam(ctx context.Context, teamID string) error {
	if err := s.requireRepository(); err != nil {
		return err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return err
	}
	teamID, err = normalizeUUID(teamID, "team id")
	if err != nil {
		return err
	}
	return s.repo.LeaveTeam(ctx, LeaveTeamRepositoryInput{
		TeamID:      teamID,
		ActorUserID: actor.ID,
	})
}

func (s *Service) CreateInvite(
	ctx context.Context,
	teamID string,
	input CreateTeamInviteInput,
) (TeamInvite, error) {
	if err := s.requireRepository(); err != nil {
		return TeamInvite{}, err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return TeamInvite{}, err
	}
	teamID, err = normalizeUUID(teamID, "team id")
	if err != nil {
		return TeamInvite{}, err
	}
	email, err := canonicalizeEmail(input.Email)
	if err != nil {
		return TeamInvite{}, err
	}
	role, err := normalizeTeamRole(input.TeamRole, ErrorCodeInvalidInvitePayload)
	if err != nil {
		return TeamInvite{}, err
	}
	idempotencyKey, err := normalizeIdempotencyKey(
		input.IdempotencyKey,
		ErrorCodeInvalidInvitePayload,
	)
	if err != nil {
		return TeamInvite{}, err
	}
	team, err := s.repo.GetTeam(ctx, TeamLookupInput{
		TeamID:      teamID,
		ActorUserID: actor.ID,
	})
	if err != nil {
		return TeamInvite{}, err
	}
	if strings.TrimSpace(team.MyMembership.TeamRole) != TeamRoleAdmin {
		return TeamInvite{}, ErrTeamAdminRequired
	}
	if err := s.requireInviteDeliveryReady(ctx); err != nil {
		return TeamInvite{}, err
	}

	inviteID, err := s.generateID("invite")
	if err != nil {
		return TeamInvite{}, err
	}
	outboxID, err := s.generateID("invite mail outbox")
	if err != nil {
		return TeamInvite{}, err
	}
	rawToken, err := s.generateInviteToken()
	if err != nil {
		return TeamInvite{}, err
	}
	acceptanceURL, err := s.buildAcceptanceURL(rawToken)
	if err != nil {
		return TeamInvite{}, err
	}
	expiresAt := s.now().UTC().Add(s.inviteTTL)
	encrypted, err := s.mailCipher.EncryptInvitePayload(
		outboxID,
		inviteID,
		teamID,
		InviteMailPayload{
			Email:                email,
			InviteToken:          rawToken,
			AcceptanceURL:        acceptanceURL,
			TeamID:               teamID,
			InvitedByUserID:      actor.ID,
			InvitedByDisplayName: strings.TrimSpace(actor.DisplayName),
			TeamRole:             role,
			ExpiresAt:            expiresAt,
		},
	)
	if err != nil {
		return TeamInvite{}, ErrInviteDeliveryUnavailable
	}

	record, err := s.repo.CreateInvite(ctx, CreateInviteRepositoryInput{
		ID:              inviteID,
		TeamID:          teamID,
		InvitedByUserID: actor.ID,
		Email:           email,
		TeamRole:        role,
		IdempotencyKey:  idempotencyKey,
		TokenHash:       HashInviteToken(rawToken),
		ExpiresAt:       expiresAt,
		MailOutbox: IdentityMailOutboxInput{
			ID:         outboxID,
			MessageID:  inviteMessageID(outboxID),
			KeyID:      encrypted.KeyID,
			Version:    encrypted.Version,
			Nonce:      append([]byte(nil), encrypted.Nonce...),
			Ciphertext: append([]byte(nil), encrypted.Ciphertext...),
		},
	})
	if err != nil {
		return TeamInvite{}, err
	}
	return safeInvite(record)
}

func inviteMessageID(outboxID string) string {
	return "<invite-" + strings.ToLower(strings.TrimSpace(outboxID)) + "@mm-chat.invalid>"
}

func (s *Service) ListInvites(
	ctx context.Context,
	teamID string,
	input ListTeamInvitesInput,
) (ApiPage[TeamInvite], error) {
	if err := s.requireRepository(); err != nil {
		return ApiPage[TeamInvite]{}, err
	}
	if err := s.requireCursorCodec(); err != nil {
		return ApiPage[TeamInvite]{}, err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return ApiPage[TeamInvite]{}, err
	}
	teamID, err = normalizeUUID(teamID, "team id")
	if err != nil {
		return ApiPage[TeamInvite]{}, err
	}
	limit, err := normalizePageLimit(input.Limit)
	if err != nil {
		return ApiPage[TeamInvite]{}, err
	}
	after, err := s.decodeInviteCursor(input.Cursor, actor.ID, teamID)
	if err != nil {
		return ApiPage[TeamInvite]{}, err
	}

	result, err := s.repo.ListInvites(ctx, ListInvitesRepositoryInput{
		TeamID:      teamID,
		ActorUserID: actor.ID,
		Limit:       limit,
		After:       after,
	})
	if err != nil {
		return ApiPage[TeamInvite]{}, err
	}

	items := make([]TeamInvite, 0, len(result.Items))
	for _, record := range result.Items {
		item, err := safeInvite(record)
		if err != nil {
			return ApiPage[TeamInvite]{}, err
		}
		items = append(items, item)
	}
	page := ApiPage[TeamInvite]{Items: items}
	if result.HasMore && len(result.Items) > 0 {
		last := result.Items[len(result.Items)-1]
		nextCursor, err := s.cursorCodec.Encode(Cursor{
			Resource: cursorResourceTeamInvites,
			UserID:   actor.ID,
			TeamID:   teamID,
			Sort:     cursorSortCreatedDesc,
			Values: []string{
				last.CreatedAt.UTC().Format(time.RFC3339Nano),
				last.ID,
			},
		})
		if err != nil {
			return ApiPage[TeamInvite]{}, fmt.Errorf("encode team invites cursor: %w", err)
		}
		page.NextCursor = nextCursor
	}
	return page, nil
}

func (s *Service) RevokeInvite(ctx context.Context, teamID string, inviteID string) error {
	if err := s.requireRepository(); err != nil {
		return err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return err
	}
	teamID, err = normalizeUUID(teamID, "team id")
	if err != nil {
		return err
	}
	inviteID, err = normalizeUUID(inviteID, "invite id")
	if err != nil {
		return err
	}
	return s.repo.RevokeInvite(ctx, RevokeInviteRepositoryInput{
		TeamID:      teamID,
		ActorUserID: actor.ID,
		InviteID:    inviteID,
	})
}

// DisableAccount is the operator maintenance boundary for account disablement.
// It intentionally has no HTTP route and delegates only to a repository that
// implements the Team/User lock and revision transaction.
func (s *Service) DisableAccount(
	ctx context.Context,
	userID string,
) ([]auth.RevokedSession, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	disabler, ok := s.repo.(AccountDisabler)
	if !ok {
		return nil, ErrDatabaseRequired
	}
	userID, err := normalizeUUID(userID, "user id")
	if err != nil {
		return nil, err
	}
	return disabler.DisableAccount(ctx, userID)
}

func (s *Service) requireRepository() error {
	if s == nil || s.repo == nil {
		return ErrDatabaseRequired
	}
	return nil
}

func (s *Service) requireCursorCodec() error {
	if s == nil || s.cursorCodec == nil {
		return ErrCursorCodecRequired
	}
	return nil
}

func (s *Service) requireInviteDeliveryReady(ctx context.Context) error {
	if s == nil || s.mailCipher == nil || s.deliveryGate == nil || s.buildInviteURL == nil {
		return ErrInviteDeliveryUnavailable
	}
	if err := s.deliveryGate.AdmitInviteDelivery(ctx); err != nil {
		return ErrInviteDeliveryUnavailable
	}
	return nil
}

func (s *Service) generateID(kind string) (string, error) {
	newID := s.newID
	if newID == nil {
		newID = newUUID
	}
	id, err := newID()
	if err != nil {
		return "", fmt.Errorf("generate %s id: %w", kind, err)
	}
	if !isUUID(id) {
		return "", fmt.Errorf("generate %s id: generator returned non-uuid %q", kind, id)
	}
	return id, nil
}

func (s *Service) generateInviteToken() (string, error) {
	newToken := s.newInviteToken
	if newToken == nil {
		newToken = GenerateInviteToken
	}
	token, err := newToken()
	if err != nil {
		return "", fmt.Errorf("generate invite token: %w", err)
	}
	token, err = NormalizeInviteToken(token)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) buildAcceptanceURL(token string) (string, error) {
	if s == nil || s.buildInviteURL == nil {
		return "", ErrInviteDeliveryUnavailable
	}
	url, err := s.buildInviteURL(token)
	if err != nil {
		return "", ErrInviteDeliveryUnavailable
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return "", ErrInviteDeliveryUnavailable
	}
	return url, nil
}

func (s *Service) decodeTeamCursor(encoded string, userID string) (*TeamPageCursor, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, nil
	}
	cursor, err := s.cursorCodec.Decode(encoded, CursorContext{
		Resource: cursorResourceTeams,
		UserID:   userID,
		Sort:     cursorSortCreatedDesc,
	})
	if err != nil {
		return nil, invalidTeamPayload("cursor is invalid")
	}
	if len(cursor.Values) != 2 {
		return nil, invalidTeamPayload("cursor is invalid")
	}
	createdAt, err := parseCursorTime(cursor.Values[0])
	if err != nil {
		return nil, invalidTeamPayload("cursor is invalid")
	}
	id, err := normalizeUUID(cursor.Values[1], "cursor id")
	if err != nil {
		return nil, invalidTeamPayload("cursor is invalid")
	}
	return &TeamPageCursor{CreatedAt: createdAt, ID: id}, nil
}

func (s *Service) decodeMemberCursor(
	encoded string,
	userID string,
	teamID string,
) (*TeamMemberPageCursor, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, nil
	}
	cursor, err := s.cursorCodec.Decode(encoded, CursorContext{
		Resource: cursorResourceTeamMembers,
		UserID:   userID,
		TeamID:   teamID,
		Sort:     cursorSortMemberAsc,
	})
	if err != nil {
		return nil, invalidTeamPayload("cursor is invalid")
	}
	if len(cursor.Values) != 2 {
		return nil, invalidTeamPayload("cursor is invalid")
	}
	joinedAt, err := parseCursorTime(cursor.Values[0])
	if err != nil {
		return nil, invalidTeamPayload("cursor is invalid")
	}
	memberUserID, err := normalizeUUID(cursor.Values[1], "cursor user id")
	if err != nil {
		return nil, invalidTeamPayload("cursor is invalid")
	}
	return &TeamMemberPageCursor{JoinedAt: joinedAt, UserID: memberUserID}, nil
}

func (s *Service) decodeInviteCursor(
	encoded string,
	userID string,
	teamID string,
) (*TeamInvitePageCursor, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, nil
	}
	cursor, err := s.cursorCodec.Decode(encoded, CursorContext{
		Resource: cursorResourceTeamInvites,
		UserID:   userID,
		TeamID:   teamID,
		Sort:     cursorSortCreatedDesc,
	})
	if err != nil {
		return nil, invalidTeamPayload("cursor is invalid")
	}
	if len(cursor.Values) != 2 {
		return nil, invalidTeamPayload("cursor is invalid")
	}
	createdAt, err := parseCursorTime(cursor.Values[0])
	if err != nil {
		return nil, invalidTeamPayload("cursor is invalid")
	}
	id, err := normalizeUUID(cursor.Values[1], "cursor id")
	if err != nil {
		return nil, invalidTeamPayload("cursor is invalid")
	}
	return &TeamInvitePageCursor{CreatedAt: createdAt, ID: id}, nil
}

func safeInvite(record TeamInviteRecord) (TeamInvite, error) {
	maskedEmail, err := MaskEmail(record.Email)
	if err != nil {
		return TeamInvite{}, fmt.Errorf("mask invite email: %w", err)
	}
	return TeamInvite{
		ID:             strings.TrimSpace(record.ID),
		TeamID:         strings.TrimSpace(record.TeamID),
		MaskedEmail:    maskedEmail,
		TeamRole:       strings.TrimSpace(record.TeamRole),
		Status:         strings.TrimSpace(record.Status),
		DeliveryStatus: strings.TrimSpace(record.DeliveryStatus),
		ExpiresAt:      record.ExpiresAt,
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      record.UpdatedAt,
	}, nil
}

func requireActor(ctx context.Context) (auth.User, error) {
	user, ok := auth.UserFromContext(ctx)
	if !ok || !isUUID(strings.TrimSpace(user.ID)) {
		return auth.User{}, ErrUnauthenticated
	}
	user.ID = strings.TrimSpace(user.ID)
	user.DisplayName = strings.TrimSpace(user.DisplayName)
	return user, nil
}

func normalizeUUID(value string, label string) (string, error) {
	value = strings.TrimSpace(value)
	if !isUUID(value) {
		return "", invalidTeamPayload(label + " must be a UUID")
	}
	return value, nil
}

func normalizeTeamName(name string) (string, error) {
	if !utf8.ValidString(name) {
		return "", invalidTeamPayload("team name must be valid UTF-8")
	}
	if containsControlOrFormatRune(name) {
		return "", invalidTeamPayload("team name must not contain control or format characters")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", invalidTeamPayload("team name is required")
	}
	if len(name) > maximumTeamNameBytes {
		return "", invalidTeamPayload("team name must not exceed 256 bytes")
	}
	if utf8.RuneCountInString(name) > maximumTeamNameRunes {
		return "", invalidTeamPayload("team name must not exceed 100 characters")
	}
	return name, nil
}

func normalizeIdempotencyKey(value string, code string) (string, error) {
	if !utf8.ValidString(value) {
		return "", validationByCode(code, "idempotencyKey must be valid UTF-8")
	}
	if containsControlOrFormatRune(value) {
		return "", validationByCode(code, "idempotencyKey must not contain control or format characters")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", validationByCode(code, "idempotencyKey is required")
	}
	if len(value) > maximumIdempotencyLen {
		return "", validationByCode(code, "idempotencyKey must not exceed 128 bytes")
	}
	return value, nil
}

func normalizePageLimit(limit int) (int, error) {
	if limit == 0 {
		return defaultPageLimit, nil
	}
	if limit < 1 || limit > maximumPageLimit {
		return 0, invalidTeamPayload("limit must be between 1 and 100")
	}
	return limit, nil
}

func normalizeTeamRole(value string, code string) (string, error) {
	role, err := normalizeTeamRoleValue(value)
	if err != nil {
		return "", validationByCode(code, "teamRole must be admin or member")
	}
	return role, nil
}

func normalizeTeamRoleValue(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case TeamRoleAdmin, TeamRoleMember:
		return value, nil
	default:
		return "", errors.New("team role must be admin or member")
	}
}

func parseCursorTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New("cursor time is required")
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func validationByCode(code string, message string) error {
	switch strings.TrimSpace(code) {
	case ErrorCodeInvalidInvitePayload:
		return invalidInvitePayload(message)
	case ErrorCodeInvalidMembershipPayload:
		return invalidMembershipPayload(message)
	default:
		return invalidTeamPayload(message)
	}
}
