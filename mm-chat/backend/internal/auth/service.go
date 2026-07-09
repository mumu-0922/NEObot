package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/sessioncache"
)

type AuthRepository interface {
	CreateSession(ctx context.Context, input CreateSessionInput) (Session, error)
	RevokeSessionByTokenHash(ctx context.Context, tokenHash string) (Session, error)
}

type CreateSessionInput struct {
	SessionID   string
	UserID      string
	DisplayName string
	TokenHash   string
	UserAgent   string
	ExpiresAt   time.Time
}

type Service struct {
	repo                 AuthRepository
	cache                sessioncache.Store
	bootstrapToken       string
	bootstrapUserID      string
	bootstrapDisplayName string
	sessionTTL           time.Duration
	now                  func() time.Time
	newID                func() (string, error)
	newToken             func() (string, error)
}

type ServiceOption func(*Service)

func WithAuthSessionCache(cache sessioncache.Store) ServiceOption {
	return func(s *Service) {
		s.cache = cache
	}
}

func WithBootstrapToken(token string) ServiceOption {
	return func(s *Service) {
		s.bootstrapToken = strings.TrimSpace(token)
	}
}

func WithBootstrapUser(userID string, displayName string) ServiceOption {
	return func(s *Service) {
		userID = strings.TrimSpace(userID)
		if userID != "" {
			s.bootstrapUserID = userID
		}
		displayName = strings.TrimSpace(displayName)
		if displayName != "" {
			s.bootstrapDisplayName = displayName
		}
	}
}

func WithSessionTTL(ttl time.Duration) ServiceOption {
	return func(s *Service) {
		if ttl > 0 {
			s.sessionTTL = ttl
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
		repo:                 repo,
		bootstrapUserID:      DevelopmentUserID,
		bootstrapDisplayName: "Owner",
		sessionTTL:           7 * 24 * time.Hour,
		now:                  time.Now,
		newID:                newUUID,
		newToken:             GenerateSessionToken,
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
	if service.bootstrapUserID == "" {
		service.bootstrapUserID = DevelopmentUserID
	}
	if service.bootstrapDisplayName == "" {
		service.bootstrapDisplayName = "Owner"
	}
	if service.sessionTTL <= 0 {
		service.sessionTTL = 7 * 24 * time.Hour
	}
	return service
}

func (s *Service) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	if err := s.requireRepository(); err != nil {
		return LoginResult{}, err
	}
	if strings.TrimSpace(s.bootstrapToken) == "" {
		return LoginResult{}, ErrAuthNotConfigured
	}
	if !constantTimeTokenEqual(input.Token, s.bootstrapToken) {
		return LoginResult{}, ErrInvalidCredential
	}

	sessionID, err := s.newID()
	if err != nil {
		return LoginResult{}, err
	}
	if !isUUID(sessionID) {
		return LoginResult{}, errors.New("generated session id must be a UUID")
	}
	rawToken, err := s.newToken()
	if err != nil {
		return LoginResult{}, err
	}
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return LoginResult{}, errors.New("generated session token is blank")
	}
	expiresAt := s.clock().Add(s.sessionTTL).UTC()
	session, err := s.repo.CreateSession(ctx, CreateSessionInput{
		SessionID:   sessionID,
		UserID:      s.bootstrapUserID,
		DisplayName: s.bootstrapDisplayName,
		TokenHash:   HashSessionToken(rawToken),
		UserAgent:   input.UserAgent,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{
		User:      UserFromSession(session),
		Token:     rawToken,
		ExpiresAt: session.ExpiresAt,
	}, nil
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
	if s.cache != nil {
		_ = s.cache.DeleteSession(ctx, tokenHash)
		_ = s.cache.MarkSessionRevoked(ctx, session.ID)
	}
	return nil
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

func constantTimeTokenEqual(got string, want string) bool {
	gotHash := HashSessionToken(got)
	wantHash := HashSessionToken(want)
	return subtle.ConstantTimeCompare([]byte(gotHash), []byte(wantHash)) == 1
}
