package teams

import (
	"context"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

const (
	TeamRoleAdmin  = "admin"
	TeamRoleMember = "member"

	MembershipStatusActive  = "active"
	MembershipStatusRemoved = "removed"

	InviteStatusPending  = "pending"
	InviteStatusAccepted = "accepted"
	InviteStatusRevoked  = "revoked"
	InviteStatusExpired  = "expired"

	InviteDeliveryPending    = "pending"
	InviteDeliveryProcessing = "processing"
	InviteDeliverySent       = "sent"
	InviteDeliveryFailed     = "failed"
	InviteDeliveryCancelled  = "cancelled"
)

type Repository interface {
	CreateTeam(ctx context.Context, input CreateTeamRepositoryInput) (Team, error)
	ListTeams(ctx context.Context, input ListTeamsRepositoryInput) (TeamPageResult, error)
	GetTeam(ctx context.Context, input TeamLookupInput) (Team, error)
	RenameTeam(ctx context.Context, input RenameTeamRepositoryInput) (Team, error)
	ListMembers(ctx context.Context, input ListMembersRepositoryInput) (TeamMemberPageResult, error)
	UpdateMemberRole(ctx context.Context, input UpdateMemberRepositoryInput) (TeamMember, error)
	RemoveMember(ctx context.Context, input RemoveMemberRepositoryInput) error
	LeaveTeam(ctx context.Context, input LeaveTeamRepositoryInput) error
	CreateInvite(ctx context.Context, input CreateInviteRepositoryInput) (TeamInviteRecord, error)
	ListInvites(ctx context.Context, input ListInvitesRepositoryInput) (TeamInvitePageResult, error)
	RevokeInvite(ctx context.Context, input RevokeInviteRepositoryInput) error
}

type InviteDeliveryGate interface {
	AdmitInviteDelivery(ctx context.Context) error
}

type AccountDisabler interface {
	DisableAccount(ctx context.Context, userID string) ([]auth.RevokedSession, error)
}

type ApiPage[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type TeamMembership struct {
	TeamRole  string    `json:"teamRole"`
	Status    string    `json:"status"`
	JoinedAt  time.Time `json:"joinedAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Team struct {
	ID                 string         `json:"id"`
	Name               string         `json:"name"`
	MembershipRevision int64          `json:"membershipRevision"`
	MyMembership       TeamMembership `json:"myMembership"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
}

type TeamMember struct {
	UserID      string    `json:"userId"`
	DisplayName string    `json:"displayName"`
	TeamRole    string    `json:"teamRole"`
	Status      string    `json:"status"`
	JoinedAt    time.Time `json:"joinedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type TeamInvite struct {
	ID             string    `json:"id"`
	TeamID         string    `json:"teamId"`
	MaskedEmail    string    `json:"maskedEmail"`
	TeamRole       string    `json:"teamRole"`
	Status         string    `json:"status"`
	DeliveryStatus string    `json:"deliveryStatus"`
	ExpiresAt      time.Time `json:"expiresAt"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type TeamInviteRecord struct {
	ID             string
	TeamID         string
	Email          string
	TeamRole       string
	Status         string
	DeliveryStatus string
	ExpiresAt      time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateTeamInput struct {
	Name           string `json:"name"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type ListTeamsInput struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type RenameTeamInput struct {
	Name string `json:"name"`
}

type ListTeamMembersInput struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type UpdateTeamMemberInput struct {
	TeamRole string `json:"teamRole"`
}

type CreateTeamInviteInput struct {
	Email          string `json:"email"`
	TeamRole       string `json:"teamRole"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type ListTeamInvitesInput struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type CreateTeamRepositoryInput struct {
	ID              string
	Name            string
	CreatedByUserID string
	IdempotencyKey  string
}

type TeamLookupInput struct {
	TeamID      string
	ActorUserID string
}

type RenameTeamRepositoryInput struct {
	TeamID      string
	ActorUserID string
	Name        string
}

type TeamPageCursor struct {
	CreatedAt time.Time
	ID        string
}

type TeamMemberPageCursor struct {
	JoinedAt time.Time
	UserID   string
}

type TeamInvitePageCursor struct {
	CreatedAt time.Time
	ID        string
}

type ListTeamsRepositoryInput struct {
	ActorUserID string
	Limit       int
	After       *TeamPageCursor
}

type TeamPageResult struct {
	Items   []Team
	HasMore bool
}

type ListMembersRepositoryInput struct {
	TeamID      string
	ActorUserID string
	Limit       int
	After       *TeamMemberPageCursor
}

type TeamMemberPageResult struct {
	Items   []TeamMember
	HasMore bool
}

type UpdateMemberRepositoryInput struct {
	TeamID       string
	ActorUserID  string
	TargetUserID string
	TeamRole     string
}

type RemoveMemberRepositoryInput struct {
	TeamID       string
	ActorUserID  string
	TargetUserID string
}

type LeaveTeamRepositoryInput struct {
	TeamID      string
	ActorUserID string
}

type InviteMailPayload struct {
	Version              int       `json:"version"`
	Email                string    `json:"email"`
	InviteToken          string    `json:"inviteToken"`
	AcceptanceURL        string    `json:"acceptanceUrl"`
	TeamID               string    `json:"teamId"`
	InvitedByUserID      string    `json:"invitedByUserId"`
	InvitedByDisplayName string    `json:"invitedByDisplayName,omitempty"`
	TeamRole             string    `json:"teamRole"`
	ExpiresAt            time.Time `json:"expiresAt"`
}

type IdentityMailOutboxInput struct {
	ID         string
	MessageID  string
	KeyID      string
	Version    int
	Nonce      []byte
	Ciphertext []byte
}

type CreateInviteRepositoryInput struct {
	ID              string
	TeamID          string
	InvitedByUserID string
	Email           string
	TeamRole        string
	IdempotencyKey  string
	TokenHash       string
	ExpiresAt       time.Time
	MailOutbox      IdentityMailOutboxInput
}

type ListInvitesRepositoryInput struct {
	TeamID      string
	ActorUserID string
	Limit       int
	After       *TeamInvitePageCursor
}

type TeamInvitePageResult struct {
	Items   []TeamInviteRecord
	HasMore bool
}

type RevokeInviteRepositoryInput struct {
	TeamID      string
	ActorUserID string
	InviteID    string
}
