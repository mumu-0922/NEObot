package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/sessioncache"
)

const (
	DevelopmentUserID      = "00000000-0000-0000-0000-000000000001"
	DevelopmentDisplayName = "Development User"

	defaultUserRole = "user"
)

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session expired")
	ErrSessionRevoked  = errors.New("session revoked")

	ErrDatabaseRequired          = errors.New("database is required")
	ErrInvalidCredential         = errors.New("invalid credentials")
	ErrInvalidIdentityInput      = errors.New("identity input is invalid")
	ErrInviteNotActive           = errors.New("invite is not active")
	ErrBootstrapAlreadyCompleted = errors.New("bootstrap identity already exists")
)

// Session is the canonical application view of a Postgres session row joined to
// browser-safe user metadata. It never contains raw bearer tokens or token
// hashes.
type Session struct {
	ID          string
	UserID      string
	DisplayName string
	Role        string
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type LoginInput struct {
	Email     string
	Password  string
	UserAgent string
}

type LoginResult struct {
	User      User
	Token     string
	ExpiresAt time.Time
}

type AcceptInviteInput struct {
	Token     string
	Password  string
	UserAgent string
}

type RecoveryRequestInput struct {
	Email string
}

type RecoveryCompleteInput struct {
	Token       string
	NewPassword string
}

type LoginCredential struct {
	UserID             string
	Email              string
	DisplayName        string
	PasswordHash       string
	CredentialRevision int64
}

type RevokedSession struct {
	ID        string
	TokenHash string
}

type User struct {
	ID          string
	DisplayName string
	Role        string
}

type contextKey string

const userContextKey contextKey = "auth.user"

func DevelopmentUser() User {
	return User{
		ID:          DevelopmentUserID,
		DisplayName: DevelopmentDisplayName,
		Role:        defaultUserRole,
	}
}

func WithUser(ctx context.Context, user User) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	user = normalizeUser(user)
	if user.ID == "" {
		user = DevelopmentUser()
	}

	return context.WithValue(ctx, userContextKey, user)
}

func UserFromContext(ctx context.Context) (User, bool) {
	if ctx == nil {
		return User{}, false
	}
	user, ok := ctx.Value(userContextKey).(User)
	if !ok {
		return User{}, false
	}
	user = normalizeUser(user)
	if user.ID == "" {
		return User{}, false
	}

	return user, true
}

func UserOrDevelopment(ctx context.Context) User {
	if user, ok := UserFromContext(ctx); ok {
		return user
	}

	return DevelopmentUser()
}

func UserFromSession(session Session) User {
	return normalizeUser(User{
		ID:          session.UserID,
		DisplayName: session.DisplayName,
		Role:        session.Role,
	})
}

func (s Session) Snapshot() sessioncache.Snapshot {
	role := s.Role
	if role == "" {
		role = defaultUserRole
	}

	return sessioncache.Snapshot{
		SessionID:   s.ID,
		UserID:      s.UserID,
		DisplayName: s.DisplayName,
		Role:        role,
		ExpiresAt:   s.ExpiresAt,
	}
}

func normalizeUser(user User) User {
	user.ID = strings.TrimSpace(user.ID)
	user.DisplayName = strings.TrimSpace(user.DisplayName)
	user.Role = strings.TrimSpace(user.Role)
	if user.Role == "" {
		user.Role = defaultUserRole
	}
	if user.ID == DevelopmentUserID && user.DisplayName == "" {
		user.DisplayName = DevelopmentDisplayName
	}

	return user
}

func sessionFromSnapshot(snapshot sessioncache.Snapshot) Session {
	role := snapshot.Role
	if role == "" {
		role = defaultUserRole
	}

	return Session{
		ID:          snapshot.SessionID,
		UserID:      snapshot.UserID,
		DisplayName: snapshot.DisplayName,
		Role:        role,
		ExpiresAt:   snapshot.ExpiresAt,
	}
}
