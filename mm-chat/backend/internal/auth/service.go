package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/sessioncache"
)

const defaultDummyPasswordHash = "$argon2id$v=19$m=65536,t=3,p=2$bW0tY2hhdC1kdW1teS0wMQ$2b5AUH5MBsXMTnEU5j9gVEAOUxi6VsrvFfNMGejrEjU"

type AuthRepository interface {
	LookupLoginCredential(ctx context.Context, canonicalEmail string) (LoginCredential, error)
	LookupInviteAcceptanceSnapshot(ctx context.Context, inviteTokenHash string) (InviteAcceptanceSnapshot, error)
	CreateCredentialSession(ctx context.Context, input CreateCredentialSessionInput) (Session, error)
	AcceptInvite(ctx context.Context, input AcceptInviteRepositoryInput) (Session, error)
	CreateRecoveryToken(ctx context.Context, input CreateRecoveryTokenInput) (RecoveryTarget, bool, error)
	CompleteRecovery(ctx context.Context, input CompleteRecoveryRepositoryInput) ([]RevokedSession, error)
	RevokeSessionByTokenHash(ctx context.Context, tokenHash string) (Session, error)
	RevokeSessionsByUserID(ctx context.Context, userID string) ([]RevokedSession, error)
	BootstrapIdentity(ctx context.Context, input BootstrapIdentityInput) error
}

// CreateSessionInput remains available for trusted integration setup and
// migration tooling. Public Email/Password login uses the revision-fenced
// CreateCredentialSession path instead.
type CreateSessionInput struct {
	SessionID   string
	UserID      string
	DisplayName string
	TokenHash   string
	UserAgent   string
	ExpiresAt   time.Time
}

type CreateCredentialSessionInput struct {
	SessionID          string
	UserID             string
	TokenHash          string
	UserAgent          string
	ExpiresAt          time.Time
	CredentialRevision int64
}

type AcceptInviteRepositoryInput struct {
	InviteTokenHash    string
	InviteTeamID       string
	InviteEmail        string
	PasswordHash       string
	UserID             string
	CredentialRevision int64
	SessionID          string
	SessionTokenHash   string
	UserAgent          string
	SessionExpiresAt   time.Time
}

type InviteAcceptanceSnapshot struct {
	TeamID string
	Email  string
}

type CreateRecoveryTokenInput struct {
	CanonicalEmail string
	TokenID        string
	TokenHash      string
	TTL            time.Duration
}

type RecoveryTarget struct {
	Email     string
	ExpiresAt time.Time
}

type CompleteRecoveryRepositoryInput struct {
	TokenHash    string
	PasswordHash string
}

type BootstrapIdentityInput struct {
	UserID       string
	Email        string
	DisplayName  string
	PasswordHash string
}

type Service struct {
	repo              AuthRepository
	cache             sessioncache.Store
	recoveryDelivery  RecoveryDelivery
	sessionTTL        time.Duration
	recoveryTTL       time.Duration
	now               func() time.Time
	newID             func() (string, error)
	newToken          func() (string, error)
	dummyPasswordHash string
}

type ServiceOption func(*Service)

func WithAuthSessionCache(cache sessioncache.Store) ServiceOption {
	return func(s *Service) {
		s.cache = cache
	}
}

func WithRecoveryDelivery(delivery RecoveryDelivery) ServiceOption {
	return func(s *Service) {
		s.recoveryDelivery = delivery
	}
}

func WithSessionTTL(ttl time.Duration) ServiceOption {
	return func(s *Service) {
		if ttl > 0 {
			s.sessionTTL = ttl
		}
	}
}

func WithRecoveryTTL(ttl time.Duration) ServiceOption {
	return func(s *Service) {
		if ttl > 0 {
			s.recoveryTTL = ttl
		}
	}
}

func WithServiceClock(now func() time.Time) ServiceOption {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func NewService(repo AuthRepository, opts ...ServiceOption) *Service {
	service := &Service{
		repo:              repo,
		sessionTTL:        7 * 24 * time.Hour,
		recoveryTTL:       30 * time.Minute,
		now:               time.Now,
		newID:             newUUID,
		newToken:          GenerateSessionToken,
		dummyPasswordHash: defaultDummyPasswordHash,
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
	if service.newToken == nil {
		service.newToken = GenerateSessionToken
	}
	if service.sessionTTL <= 0 {
		service.sessionTTL = 7 * 24 * time.Hour
	}
	if service.recoveryTTL <= 0 {
		service.recoveryTTL = 30 * time.Minute
	}
	if service.dummyPasswordHash == "" {
		service.dummyPasswordHash = defaultDummyPasswordHash
	}
	return service
}

func (s *Service) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	if err := s.requireRepository(); err != nil {
		return LoginResult{}, err
	}
	email, err := canonicalizeEmail(input.Email)
	if err != nil {
		return LoginResult{}, ErrInvalidIdentityInput
	}
	if err := validatePassword(input.Password); err != nil {
		return LoginResult{}, ErrInvalidIdentityInput
	}

	credential, err := s.repo.LookupLoginCredential(ctx, email)
	if errors.Is(err, ErrInvalidCredential) {
		_, verifyErr := verifyPassword(ctx, input.Password, s.dummyPasswordHash)
		if verifyErr != nil {
			return LoginResult{}, fmt.Errorf("verify dummy credential: %w", verifyErr)
		}
		return LoginResult{}, ErrInvalidCredential
	}
	if err != nil {
		return LoginResult{}, err
	}

	valid, err := verifyPassword(ctx, input.Password, credential.PasswordHash)
	if err != nil {
		return LoginResult{}, fmt.Errorf("verify credential: %w", err)
	}
	if !valid {
		return LoginResult{}, ErrInvalidCredential
	}

	material, err := s.newSessionMaterial(input.UserAgent)
	if err != nil {
		return LoginResult{}, err
	}
	session, err := s.repo.CreateCredentialSession(ctx, CreateCredentialSessionInput{
		SessionID:          material.sessionID,
		UserID:             credential.UserID,
		TokenHash:          material.tokenHash,
		UserAgent:          material.userAgent,
		ExpiresAt:          material.expiresAt,
		CredentialRevision: credential.CredentialRevision,
	})
	if errors.Is(err, ErrInvalidCredential) {
		return LoginResult{}, ErrInvalidCredential
	}
	if err != nil {
		return LoginResult{}, err
	}
	return loginResult(session, material.rawToken), nil
}

func (s *Service) AcceptInvite(ctx context.Context, input AcceptInviteInput) (LoginResult, error) {
	if err := s.requireRepository(); err != nil {
		return LoginResult{}, err
	}
	token, err := normalizeOneTimeToken(input.Token)
	if err != nil {
		return LoginResult{}, ErrInviteNotActive
	}
	if err := validatePassword(input.Password); err != nil {
		return LoginResult{}, err
	}
	inviteTokenHash := HashSessionToken(token)
	snapshot, err := s.repo.LookupInviteAcceptanceSnapshot(ctx, inviteTokenHash)
	if errors.Is(err, ErrInviteNotActive) {
		return LoginResult{}, ErrInviteNotActive
	}
	if err != nil {
		return LoginResult{}, err
	}

	repoInput := AcceptInviteRepositoryInput{
		InviteTokenHash: inviteTokenHash,
		InviteTeamID:    snapshot.TeamID,
		InviteEmail:     snapshot.Email,
	}

	credential, err := s.repo.LookupLoginCredential(ctx, snapshot.Email)
	if err == nil {
		credentialEmail, emailErr := canonicalizeEmail(credential.Email)
		if emailErr != nil || credentialEmail != snapshot.Email ||
			!isUUID(credential.UserID) ||
			credential.CredentialRevision < 1 ||
			credential.PasswordHash == "" {
			err = ErrInvalidCredential
		}
	}
	switch {
	case err == nil:
		valid, verifyErr := verifyPassword(ctx, input.Password, credential.PasswordHash)
		if verifyErr != nil {
			return LoginResult{}, fmt.Errorf("verify invite credential: %w", verifyErr)
		}
		if !valid {
			return LoginResult{}, ErrInviteNotActive
		}
		repoInput.UserID = credential.UserID
		repoInput.CredentialRevision = credential.CredentialRevision
	case errors.Is(err, ErrInvalidCredential):
		passwordHash, hashErr := hashPassword(ctx, input.Password)
		if hashErr != nil {
			if errors.Is(hashErr, ErrInvalidIdentityInput) {
				return LoginResult{}, hashErr
			}
			return LoginResult{}, fmt.Errorf("hash invite password: %w", hashErr)
		}
		userID, idErr := s.newUUID("user")
		if idErr != nil {
			return LoginResult{}, idErr
		}
		repoInput.UserID = userID
		repoInput.PasswordHash = passwordHash
	default:
		return LoginResult{}, err
	}

	material, err := s.newSessionMaterial(input.UserAgent)
	if err != nil {
		return LoginResult{}, err
	}
	repoInput.SessionID = material.sessionID
	repoInput.SessionTokenHash = material.tokenHash
	repoInput.UserAgent = material.userAgent
	repoInput.SessionExpiresAt = material.expiresAt

	session, err := s.repo.AcceptInvite(ctx, repoInput)
	if errors.Is(err, ErrInviteNotActive) {
		return LoginResult{}, ErrInviteNotActive
	}
	if err != nil {
		return LoginResult{}, err
	}
	return loginResult(session, material.rawToken), nil
}

func (s *Service) RequestRecovery(ctx context.Context, input RecoveryRequestInput) error {
	if err := s.requireRepository(); err != nil {
		return err
	}
	email, err := canonicalizeEmail(input.Email)
	if err != nil {
		return ErrInvalidIdentityInput
	}
	tokenID, err := s.newUUID("recovery token")
	if err != nil {
		return err
	}
	rawToken, err := s.newOpaqueToken()
	if err != nil {
		return err
	}
	target, deliver, err := s.repo.CreateRecoveryToken(ctx, CreateRecoveryTokenInput{
		CanonicalEmail: email,
		TokenID:        tokenID,
		TokenHash:      HashSessionToken(rawToken),
		TTL:            s.recoveryTTL,
	})
	if err != nil {
		return err
	}
	if deliver && s.recoveryDelivery != nil {
		_ = s.recoveryDelivery.EnqueueRecovery(RecoveryMessage{
			Email:     target.Email,
			Token:     rawToken,
			ExpiresAt: target.ExpiresAt,
		})
	}
	return nil
}

func (s *Service) CompleteRecovery(ctx context.Context, input RecoveryCompleteInput) error {
	if err := s.requireRepository(); err != nil {
		return err
	}
	token, err := normalizeOneTimeToken(input.Token)
	if err != nil {
		return ErrInvalidCredential
	}
	passwordHash, err := hashPassword(ctx, input.NewPassword)
	if err != nil {
		if errors.Is(err, ErrInvalidIdentityInput) {
			return err
		}
		return fmt.Errorf("hash recovery password: %w", err)
	}
	revoked, err := s.repo.CompleteRecovery(ctx, CompleteRecoveryRepositoryInput{
		TokenHash:    HashSessionToken(token),
		PasswordHash: passwordHash,
	})
	if errors.Is(err, ErrInvalidCredential) {
		return ErrInvalidCredential
	}
	if err != nil {
		return err
	}
	s.invalidateSessions(ctx, revoked)
	return nil
}

func (s *Service) Logout(ctx context.Context, rawToken string) error {
	if err := s.requireRepository(); err != nil {
		return err
	}
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return ErrSessionNotFound
	}
	tokenHash := HashSessionToken(rawToken)
	session, err := s.repo.RevokeSessionByTokenHash(ctx, tokenHash)
	if errors.Is(err, ErrSessionNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	s.invalidateSessions(ctx, []RevokedSession{{ID: session.ID, TokenHash: tokenHash}})
	return nil
}

func (s *Service) RevokeAllSessions(ctx context.Context) error {
	if err := s.requireRepository(); err != nil {
		return err
	}
	user, ok := UserFromContext(ctx)
	if !ok || !isUUID(user.ID) {
		return ErrSessionNotFound
	}
	revoked, err := s.repo.RevokeSessionsByUserID(ctx, user.ID)
	if err != nil {
		return err
	}
	s.invalidateSessions(ctx, revoked)
	return nil
}

// BootstrapIdentity creates the first Email/Password credential through an
// operator-only command. The repository refuses the operation once any
// credential exists, so this is not a password-reset or break-glass path.
func (s *Service) BootstrapIdentity(
	ctx context.Context,
	userID string,
	email string,
	displayName string,
	password string,
) error {
	if err := s.requireRepository(); err != nil {
		return err
	}
	canonicalEmail, err := canonicalizeEmail(email)
	if err != nil {
		return ErrInvalidIdentityInput
	}
	passwordHash, err := hashPassword(ctx, password)
	if err != nil {
		return err
	}
	return s.repo.BootstrapIdentity(ctx, BootstrapIdentityInput{
		UserID:       strings.TrimSpace(userID),
		Email:        canonicalEmail,
		DisplayName:  strings.TrimSpace(displayName),
		PasswordHash: passwordHash,
	})
}

func (s *Service) CurrentUser(ctx context.Context) User {
	return UserOrDevelopment(ctx)
}

func (s *Service) requireRepository() error {
	if s == nil || s.repo == nil {
		return ErrDatabaseRequired
	}
	return nil
}

func (s *Service) clock() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

type sessionMaterial struct {
	sessionID string
	rawToken  string
	tokenHash string
	userAgent string
	expiresAt time.Time
}

func (s *Service) newSessionMaterial(userAgent string) (sessionMaterial, error) {
	sessionID, err := s.newUUID("session")
	if err != nil {
		return sessionMaterial{}, err
	}
	rawToken, err := s.newOpaqueToken()
	if err != nil {
		return sessionMaterial{}, err
	}
	return sessionMaterial{
		sessionID: sessionID,
		rawToken:  rawToken,
		tokenHash: HashSessionToken(rawToken),
		userAgent: strings.TrimSpace(userAgent),
		expiresAt: s.clock().Add(s.sessionTTL).UTC(),
	}, nil
}

func (s *Service) newUUID(kind string) (string, error) {
	id, err := s.newID()
	if err != nil {
		return "", err
	}
	if !isUUID(id) {
		return "", fmt.Errorf("generated %s id must be a UUID", kind)
	}
	return id, nil
}

func (s *Service) newOpaqueToken() (string, error) {
	token, err := s.newToken()
	if err != nil {
		return "", err
	}
	token, err = normalizeOneTimeToken(token)
	if err != nil {
		return "", errors.New("generated token must be 32-byte lowercase hex")
	}
	return token, nil
}

func (s *Service) invalidateSessions(ctx context.Context, sessions []RevokedSession) {
	if s == nil || s.cache == nil {
		return
	}
	for _, session := range sessions {
		if session.TokenHash != "" {
			_ = s.cache.DeleteSession(ctx, session.TokenHash)
		}
		if session.ID != "" {
			_ = s.cache.MarkSessionRevoked(ctx, session.ID)
		}
	}
}

func loginResult(session Session, rawToken string) LoginResult {
	return LoginResult{
		User:      UserFromSession(session),
		Token:     rawToken,
		ExpiresAt: session.ExpiresAt,
	}
}
