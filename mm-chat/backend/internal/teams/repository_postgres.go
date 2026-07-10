package teams

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"neo-chat/mm-chat/backend/internal/auth"
)

const (
	teamMembershipChangedEventType = "team.membership.changed"

	membershipOperationAdded       = "added"
	membershipOperationRoleChanged = "role_changed"
	membershipOperationRemoved     = "removed"
	membershipOperationLeft        = "left"
	membershipOperationDisabled    = "disabled"

	membershipEffectiveStatusDisabled = "disabled"
)

type PostgresRepository struct {
	db         *sql.DB
	newEventID func() (string, error)
}

var _ Repository = (*PostgresRepository)(nil)

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db, newEventID: newUUID}
}

type rowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type queryExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type teamRow struct {
	ID                 string
	Name               string
	MembershipRevision int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type membershipRow struct {
	TeamID      string
	UserID      string
	DisplayName string
	Role        string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type actorAccess struct {
	Role      string
	JoinedAt  time.Time
	UpdatedAt time.Time
}

type inviteRow struct {
	ID        string
	Email     string
	Role      string
	Status    string
	ExpiresAt time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

type disableUserState struct {
	Exists         bool
	AccountStatus  string
	DeletedAtValid bool
}

func (r *PostgresRepository) CreateTeam(
	ctx context.Context,
	input CreateTeamRepositoryInput,
) (Team, error) {
	if err := r.requireDB(); err != nil {
		return Team{}, err
	}
	if !isUUID(strings.TrimSpace(input.ID)) || !isUUID(strings.TrimSpace(input.CreatedByUserID)) {
		return Team{}, errors.New("team and creator ids must be UUIDs")
	}
	input.Name = strings.TrimSpace(input.Name)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.Name == "" || input.IdempotencyKey == "" {
		return Team{}, errors.New("team name and idempotency key are required")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Team{}, fmt.Errorf("begin create team: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := lockActiveUser(ctx, tx, input.CreatedByUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Team{}, ErrUnauthenticated
		}
		return Team{}, fmt.Errorf("lock team creator: %w", err)
	}
	if err := ensureTeamIdempotencyAvailable(ctx, tx, input.CreatedByUserID, input.IdempotencyKey); err != nil {
		return Team{}, err
	}

	teamRecord, err := insertTeamRow(ctx, tx, input)
	if err != nil {
		if isConstraintViolation(err, "idx_teams_created_by_idempotency") {
			return Team{}, ErrIdempotencyConflict
		}
		return Team{}, err
	}
	membership, err := insertActiveMembership(ctx, tx, input.ID, input.CreatedByUserID, TeamRoleAdmin)
	if err != nil {
		return Team{}, err
	}
	if err := r.insertMembershipOutbox(
		ctx,
		tx,
		input.ID,
		input.CreatedByUserID,
		TeamRoleAdmin,
		MembershipStatusActive,
		membershipOperationAdded,
		1,
	); err != nil {
		return Team{}, err
	}

	if err := tx.Commit(); err != nil {
		if isConstraintViolation(err, "idx_teams_created_by_idempotency") {
			return Team{}, ErrIdempotencyConflict
		}
		return Team{}, fmt.Errorf("commit create team: %w", err)
	}

	return Team{
		ID:                 teamRecord.ID,
		Name:               teamRecord.Name,
		MembershipRevision: teamRecord.MembershipRevision,
		MyMembership: TeamMembership{
			TeamRole:  TeamRoleAdmin,
			Status:    MembershipStatusActive,
			JoinedAt:  membership.CreatedAt.UTC(),
			UpdatedAt: membership.UpdatedAt.UTC(),
		},
		CreatedAt: teamRecord.CreatedAt.UTC(),
		UpdatedAt: teamRecord.UpdatedAt.UTC(),
	}, nil
}

func (r *PostgresRepository) ListTeams(
	ctx context.Context,
	input ListTeamsRepositoryInput,
) (TeamPageResult, error) {
	if err := r.requireDB(); err != nil {
		return TeamPageResult{}, err
	}
	limit := repositoryPageLimit(input.Limit)
	query := `
SELECT
  t.id,
  t.name,
  t.membership_revision,
  m.role,
  m.status,
  m.created_at,
  m.updated_at,
  t.created_at,
  t.updated_at
FROM team_memberships m
JOIN teams t ON t.id = m.team_id
JOIN users actor ON actor.id = m.user_id
WHERE m.user_id = $1
  AND m.status = 'active'
  AND actor.account_status = 'active'
  AND actor.deleted_at IS NULL
  AND t.deleted_at IS NULL
`
	args := []any{input.ActorUserID}
	if input.After != nil {
		query += fmt.Sprintf(`
  AND (
    t.created_at < $%d
    OR (t.created_at = $%d AND t.id < $%d)
  )
`, len(args)+1, len(args)+1, len(args)+2)
		args = append(args, input.After.CreatedAt.UTC(), input.After.ID)
	}
	query += fmt.Sprintf(`
ORDER BY t.created_at DESC, t.id DESC
LIMIT $%d
`, len(args)+1)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return TeamPageResult{}, fmt.Errorf("list teams: %w", err)
	}
	defer rows.Close()

	items := make([]Team, 0, limit)
	for rows.Next() {
		var team Team
		if err := rows.Scan(
			&team.ID,
			&team.Name,
			&team.MembershipRevision,
			&team.MyMembership.TeamRole,
			&team.MyMembership.Status,
			&team.MyMembership.JoinedAt,
			&team.MyMembership.UpdatedAt,
			&team.CreatedAt,
			&team.UpdatedAt,
		); err != nil {
			return TeamPageResult{}, fmt.Errorf("scan team row: %w", err)
		}
		if len(items) == limit {
			return TeamPageResult{Items: items, HasMore: true}, nil
		}
		team.MyMembership.JoinedAt = team.MyMembership.JoinedAt.UTC()
		team.MyMembership.UpdatedAt = team.MyMembership.UpdatedAt.UTC()
		team.CreatedAt = team.CreatedAt.UTC()
		team.UpdatedAt = team.UpdatedAt.UTC()
		items = append(items, team)
	}
	if err := rows.Err(); err != nil {
		return TeamPageResult{}, fmt.Errorf("iterate teams: %w", err)
	}
	return TeamPageResult{Items: items}, nil
}

func (r *PostgresRepository) GetTeam(
	ctx context.Context,
	input TeamLookupInput,
) (Team, error) {
	if err := r.requireDB(); err != nil {
		return Team{}, err
	}
	var team Team
	err := r.db.QueryRowContext(ctx, `
SELECT
  t.id,
  t.name,
  t.membership_revision,
  m.role,
  m.status,
  m.created_at,
  m.updated_at,
  t.created_at,
  t.updated_at
FROM teams t
JOIN team_memberships m ON m.team_id = t.id
JOIN users actor ON actor.id = m.user_id
WHERE t.id = $1
  AND t.deleted_at IS NULL
  AND m.user_id = $2
  AND m.status = 'active'
  AND actor.account_status = 'active'
  AND actor.deleted_at IS NULL
`, input.TeamID, input.ActorUserID).Scan(
		&team.ID,
		&team.Name,
		&team.MembershipRevision,
		&team.MyMembership.TeamRole,
		&team.MyMembership.Status,
		&team.MyMembership.JoinedAt,
		&team.MyMembership.UpdatedAt,
		&team.CreatedAt,
		&team.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Team{}, ErrTeamNotFound
	}
	if err != nil {
		return Team{}, fmt.Errorf("get team: %w", err)
	}
	team.MyMembership.JoinedAt = team.MyMembership.JoinedAt.UTC()
	team.MyMembership.UpdatedAt = team.MyMembership.UpdatedAt.UTC()
	team.CreatedAt = team.CreatedAt.UTC()
	team.UpdatedAt = team.UpdatedAt.UTC()
	return team, nil
}

func (r *PostgresRepository) RenameTeam(
	ctx context.Context,
	input RenameTeamRepositoryInput,
) (Team, error) {
	if err := r.requireDB(); err != nil {
		return Team{}, err
	}
	if err := r.requireAdminAccess(ctx, input.TeamID, input.ActorUserID); err != nil {
		return Team{}, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Team{}, fmt.Errorf("begin rename team: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	teamRecord, err := lockTeam(ctx, tx, input.TeamID)
	if err != nil {
		return Team{}, err
	}
	access, err := lockActorAccess(ctx, tx, input.TeamID, input.ActorUserID)
	if err != nil {
		return Team{}, err
	}
	if access.Role != TeamRoleAdmin {
		return Team{}, ErrTeamAdminRequired
	}
	if teamRecord.Name != input.Name {
		teamRecord, err = updateTeamName(ctx, tx, input.TeamID, input.Name)
		if err != nil {
			return Team{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Team{}, fmt.Errorf("commit rename team: %w", err)
	}

	return Team{
		ID:                 teamRecord.ID,
		Name:               teamRecord.Name,
		MembershipRevision: teamRecord.MembershipRevision,
		MyMembership: TeamMembership{
			TeamRole:  access.Role,
			Status:    MembershipStatusActive,
			JoinedAt:  access.JoinedAt.UTC(),
			UpdatedAt: access.UpdatedAt.UTC(),
		},
		CreatedAt: teamRecord.CreatedAt.UTC(),
		UpdatedAt: teamRecord.UpdatedAt.UTC(),
	}, nil
}

func (r *PostgresRepository) ListMembers(
	ctx context.Context,
	input ListMembersRepositoryInput,
) (TeamMemberPageResult, error) {
	if err := r.requireDB(); err != nil {
		return TeamMemberPageResult{}, err
	}
	if _, err := r.lookupActorAccess(ctx, input.TeamID, input.ActorUserID); err != nil {
		return TeamMemberPageResult{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return TeamMemberPageResult{}, fmt.Errorf("begin list team members: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := lockTeam(ctx, tx, input.TeamID); err != nil {
		return TeamMemberPageResult{}, err
	}
	if _, err := lockActorAccess(ctx, tx, input.TeamID, input.ActorUserID); err != nil {
		return TeamMemberPageResult{}, err
	}

	limit := repositoryPageLimit(input.Limit)
	query := `
SELECT
  m.user_id,
  COALESCE(u.display_name, ''),
  m.role,
  m.status,
  m.created_at,
  m.updated_at
FROM team_memberships m
JOIN users u ON u.id = m.user_id
WHERE m.team_id = $1
  AND m.status = 'active'
	AND u.account_status = 'active'
	AND u.deleted_at IS NULL
`
	args := []any{input.TeamID}
	if input.After != nil {
		query += fmt.Sprintf(`
  AND (
    m.created_at > $%d
    OR (m.created_at = $%d AND m.user_id > $%d)
  )
`, len(args)+1, len(args)+1, len(args)+2)
		args = append(args, input.After.JoinedAt.UTC(), input.After.UserID)
	}
	query += fmt.Sprintf(`
ORDER BY m.created_at ASC, m.user_id ASC
LIMIT $%d
`, len(args)+1)
	args = append(args, limit+1)

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return TeamMemberPageResult{}, fmt.Errorf("list team members: %w", err)
	}
	defer rows.Close()

	items := make([]TeamMember, 0, limit)
	hasMore := false
	for rows.Next() {
		var member TeamMember
		if err := rows.Scan(
			&member.UserID,
			&member.DisplayName,
			&member.TeamRole,
			&member.Status,
			&member.JoinedAt,
			&member.UpdatedAt,
		); err != nil {
			return TeamMemberPageResult{}, fmt.Errorf("scan team member row: %w", err)
		}
		if len(items) == limit {
			hasMore = true
			break
		}
		member.JoinedAt = member.JoinedAt.UTC()
		member.UpdatedAt = member.UpdatedAt.UTC()
		items = append(items, member)
	}
	if err := rows.Err(); err != nil {
		return TeamMemberPageResult{}, fmt.Errorf("iterate team members: %w", err)
	}
	if err := rows.Close(); err != nil {
		return TeamMemberPageResult{}, fmt.Errorf("close team member rows: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return TeamMemberPageResult{}, fmt.Errorf("commit list team members: %w", err)
	}
	return TeamMemberPageResult{Items: items, HasMore: hasMore}, nil
}

func (r *PostgresRepository) UpdateMemberRole(
	ctx context.Context,
	input UpdateMemberRepositoryInput,
) (TeamMember, error) {
	if err := r.requireDB(); err != nil {
		return TeamMember{}, err
	}
	if err := r.requireAdminAccess(ctx, input.TeamID, input.ActorUserID); err != nil {
		return TeamMember{}, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return TeamMember{}, fmt.Errorf("begin update member role: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	targetUser, targetUserMissing, err := lockTargetUser(ctx, tx, input.TargetUserID)
	if err != nil {
		return TeamMember{}, err
	}
	if _, err := lockTeam(ctx, tx, input.TeamID); err != nil {
		return TeamMember{}, err
	}
	actor, err := lockActorAccess(ctx, tx, input.TeamID, input.ActorUserID)
	if err != nil {
		return TeamMember{}, err
	}
	if actor.Role != TeamRoleAdmin {
		return TeamMember{}, ErrTeamAdminRequired
	}
	if targetUserMissing {
		return TeamMember{}, ErrTeamMemberNotFound
	}

	membership, err := lockMembership(ctx, tx, input.TeamID, input.TargetUserID)
	if err != nil {
		return TeamMember{}, err
	}
	if membership.Role == input.TeamRole {
		if err := tx.Commit(); err != nil {
			return TeamMember{}, fmt.Errorf("commit noop update member role: %w", err)
		}
		return TeamMember{
			UserID:      input.TargetUserID,
			DisplayName: targetUser.DisplayName,
			TeamRole:    membership.Role,
			Status:      membership.Status,
			JoinedAt:    membership.CreatedAt.UTC(),
			UpdatedAt:   membership.UpdatedAt.UTC(),
		}, nil
	}
	if membership.Role == TeamRoleAdmin {
		ok, err := hasOtherUsableAdmin(ctx, tx, input.TeamID, input.TargetUserID)
		if err != nil {
			return TeamMember{}, err
		}
		if !ok {
			return TeamMember{}, ErrLastTeamAdmin
		}
	}

	updatedMembership, err := updateMembershipRoleRow(ctx, tx, input.TeamID, input.TargetUserID, input.TeamRole)
	if err != nil {
		return TeamMember{}, err
	}
	revision, err := advanceMembershipRevision(ctx, tx, input.TeamID)
	if err != nil {
		return TeamMember{}, err
	}
	if err := r.insertMembershipOutbox(
		ctx,
		tx,
		input.TeamID,
		input.TargetUserID,
		updatedMembership.Role,
		updatedMembership.Status,
		membershipOperationRoleChanged,
		revision,
	); err != nil {
		return TeamMember{}, err
	}
	if err := tx.Commit(); err != nil {
		return TeamMember{}, fmt.Errorf("commit update member role: %w", err)
	}

	return TeamMember{
		UserID:      input.TargetUserID,
		DisplayName: targetUser.DisplayName,
		TeamRole:    updatedMembership.Role,
		Status:      updatedMembership.Status,
		JoinedAt:    updatedMembership.CreatedAt.UTC(),
		UpdatedAt:   updatedMembership.UpdatedAt.UTC(),
	}, nil
}

func (r *PostgresRepository) RemoveMember(
	ctx context.Context,
	input RemoveMemberRepositoryInput,
) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	if err := r.requireAdminAccess(ctx, input.TeamID, input.ActorUserID); err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin remove member: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, targetUserMissing, err := lockTargetUser(ctx, tx, input.TargetUserID)
	if err != nil {
		return err
	}
	if _, err := lockTeam(ctx, tx, input.TeamID); err != nil {
		return err
	}
	actor, err := lockActorAccess(ctx, tx, input.TeamID, input.ActorUserID)
	if err != nil {
		return err
	}
	if actor.Role != TeamRoleAdmin {
		return ErrTeamAdminRequired
	}
	if strings.EqualFold(
		strings.TrimSpace(input.ActorUserID),
		strings.TrimSpace(input.TargetUserID),
	) {
		return invalidMembershipPayload(
			"cannot remove your own membership; use the self-leave endpoint",
		)
	}
	if targetUserMissing {
		return ErrTeamMemberNotFound
	}

	membership, err := lockMembership(ctx, tx, input.TeamID, input.TargetUserID)
	if err != nil {
		return err
	}
	if membership.Role == TeamRoleAdmin {
		ok, err := hasOtherUsableAdmin(ctx, tx, input.TeamID, input.TargetUserID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrLastTeamAdmin
		}
	}
	updatedMembership, err := markMembershipRemoved(ctx, tx, input.TeamID, input.TargetUserID)
	if err != nil {
		return err
	}
	revision, err := advanceMembershipRevision(ctx, tx, input.TeamID)
	if err != nil {
		return err
	}
	if err := r.insertMembershipOutbox(
		ctx,
		tx,
		input.TeamID,
		input.TargetUserID,
		membership.Role,
		updatedMembership.Status,
		membershipOperationRemoved,
		revision,
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit remove member: %w", err)
	}
	return nil
}

func (r *PostgresRepository) LeaveTeam(
	ctx context.Context,
	input LeaveTeamRepositoryInput,
) error {
	if err := r.requireDB(); err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin leave team: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, actorMissing, err := lockTargetUser(ctx, tx, input.ActorUserID)
	if err != nil {
		return err
	}
	if _, err := lockTeam(ctx, tx, input.TeamID); err != nil {
		return err
	}
	if actorMissing {
		return ErrTeamNotFound
	}
	membership, err := lockActorAccess(ctx, tx, input.TeamID, input.ActorUserID)
	if err != nil {
		return err
	}
	if membership.Role == TeamRoleAdmin {
		ok, err := hasOtherUsableAdmin(ctx, tx, input.TeamID, input.ActorUserID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrLastTeamAdmin
		}
	}
	updatedMembership, err := markMembershipRemoved(ctx, tx, input.TeamID, input.ActorUserID)
	if err != nil {
		return err
	}
	revision, err := advanceMembershipRevision(ctx, tx, input.TeamID)
	if err != nil {
		return err
	}
	if err := r.insertMembershipOutbox(
		ctx,
		tx,
		input.TeamID,
		input.ActorUserID,
		membership.Role,
		updatedMembership.Status,
		membershipOperationLeft,
		revision,
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit leave team: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CreateInvite(
	ctx context.Context,
	input CreateInviteRepositoryInput,
) (TeamInviteRecord, error) {
	if err := r.requireDB(); err != nil {
		return TeamInviteRecord{}, err
	}
	if err := r.requireAdminAccess(ctx, input.TeamID, input.InvitedByUserID); err != nil {
		return TeamInviteRecord{}, err
	}
	if !isUUID(strings.TrimSpace(input.ID)) || !isUUID(strings.TrimSpace(input.TeamID)) || !isUUID(strings.TrimSpace(input.InvitedByUserID)) || !isUUID(strings.TrimSpace(input.MailOutbox.ID)) {
		return TeamInviteRecord{}, errors.New("invite, team, inviter, and outbox ids must be UUIDs")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return TeamInviteRecord{}, fmt.Errorf("begin create invite: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := lockTeam(ctx, tx, input.TeamID); err != nil {
		return TeamInviteRecord{}, err
	}
	actor, err := lockActorAccess(ctx, tx, input.TeamID, input.InvitedByUserID)
	if err != nil {
		return TeamInviteRecord{}, err
	}
	if actor.Role != TeamRoleAdmin {
		return TeamInviteRecord{}, ErrTeamAdminRequired
	}

	if err := revokeExpiredPendingInvites(ctx, tx, input.TeamID, input.Email); err != nil {
		return TeamInviteRecord{}, err
	}
	if err := ensureInviteIdempotencyAvailable(ctx, tx, input.TeamID, input.InvitedByUserID, input.IdempotencyKey); err != nil {
		return TeamInviteRecord{}, err
	}
	if exists, err := activeMembershipExistsByEmail(ctx, tx, input.TeamID, input.Email); err != nil {
		return TeamInviteRecord{}, err
	} else if exists {
		return TeamInviteRecord{}, ErrInviteConflict
	}
	if exists, err := activePendingInviteExists(ctx, tx, input.TeamID, input.Email); err != nil {
		return TeamInviteRecord{}, err
	} else if exists {
		return TeamInviteRecord{}, ErrInviteConflict
	}

	invite, err := insertInvite(ctx, tx, input)
	if err != nil {
		if isConstraintViolation(err, "idx_team_invites_team_inviter_idempotency") {
			return TeamInviteRecord{}, ErrIdempotencyConflict
		}
		if isConstraintViolation(err, "idx_team_invites_pending_team_email") {
			return TeamInviteRecord{}, ErrInviteConflict
		}
		return TeamInviteRecord{}, err
	}
	if err := insertIdentityMailOutbox(ctx, tx, input.TeamID, input.ID, input.MailOutbox); err != nil {
		return TeamInviteRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		if isConstraintViolation(err, "idx_team_invites_team_inviter_idempotency") {
			return TeamInviteRecord{}, ErrIdempotencyConflict
		}
		if isConstraintViolation(err, "idx_team_invites_pending_team_email") {
			return TeamInviteRecord{}, ErrInviteConflict
		}
		return TeamInviteRecord{}, fmt.Errorf("commit create invite: %w", err)
	}
	return TeamInviteRecord{
		ID:             invite.ID,
		TeamID:         input.TeamID,
		Email:          invite.Email,
		TeamRole:       invite.Role,
		Status:         invite.Status,
		DeliveryStatus: InviteDeliveryPending,
		ExpiresAt:      invite.ExpiresAt.UTC(),
		CreatedAt:      invite.CreatedAt.UTC(),
		UpdatedAt:      invite.UpdatedAt.UTC(),
	}, nil
}

func (r *PostgresRepository) ListInvites(
	ctx context.Context,
	input ListInvitesRepositoryInput,
) (TeamInvitePageResult, error) {
	if err := r.requireDB(); err != nil {
		return TeamInvitePageResult{}, err
	}
	if err := r.requireAdminAccess(ctx, input.TeamID, input.ActorUserID); err != nil {
		return TeamInvitePageResult{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return TeamInvitePageResult{}, fmt.Errorf("begin list team invites: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := lockTeam(ctx, tx, input.TeamID); err != nil {
		return TeamInvitePageResult{}, err
	}
	access, err := lockActorAccess(ctx, tx, input.TeamID, input.ActorUserID)
	if err != nil {
		return TeamInvitePageResult{}, err
	}
	if access.Role != TeamRoleAdmin {
		return TeamInvitePageResult{}, ErrTeamAdminRequired
	}

	limit := repositoryPageLimit(input.Limit)
	query := `
SELECT
  i.id,
  i.team_id,
  i.email,
  i.role,
  CASE
    WHEN i.status = 'pending' AND i.expires_at <= now() THEN 'expired'
    ELSE i.status
  END AS api_status,
  COALESCE(o.status, 'pending') AS delivery_status,
  i.expires_at,
  i.created_at,
  i.updated_at
FROM team_invites i
LEFT JOIN identity_mail_outbox o ON o.invite_id = i.id
WHERE i.team_id = $1
`
	args := []any{input.TeamID}
	if input.After != nil {
		query += fmt.Sprintf(`
  AND (
    i.created_at < $%d
    OR (i.created_at = $%d AND i.id < $%d)
  )
`, len(args)+1, len(args)+1, len(args)+2)
		args = append(args, input.After.CreatedAt.UTC(), input.After.ID)
	}
	query += fmt.Sprintf(`
ORDER BY i.created_at DESC, i.id DESC
LIMIT $%d
`, len(args)+1)
	args = append(args, limit+1)

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return TeamInvitePageResult{}, fmt.Errorf("list invites: %w", err)
	}
	defer rows.Close()

	items := make([]TeamInviteRecord, 0, limit)
	hasMore := false
	for rows.Next() {
		var invite TeamInviteRecord
		if err := rows.Scan(
			&invite.ID,
			&invite.TeamID,
			&invite.Email,
			&invite.TeamRole,
			&invite.Status,
			&invite.DeliveryStatus,
			&invite.ExpiresAt,
			&invite.CreatedAt,
			&invite.UpdatedAt,
		); err != nil {
			return TeamInvitePageResult{}, fmt.Errorf("scan invite row: %w", err)
		}
		if len(items) == limit {
			hasMore = true
			break
		}
		invite.ExpiresAt = invite.ExpiresAt.UTC()
		invite.CreatedAt = invite.CreatedAt.UTC()
		invite.UpdatedAt = invite.UpdatedAt.UTC()
		items = append(items, invite)
	}
	if err := rows.Err(); err != nil {
		return TeamInvitePageResult{}, fmt.Errorf("iterate invites: %w", err)
	}
	if err := rows.Close(); err != nil {
		return TeamInvitePageResult{}, fmt.Errorf("close invite rows: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return TeamInvitePageResult{}, fmt.Errorf("commit list invites: %w", err)
	}
	return TeamInvitePageResult{Items: items, HasMore: hasMore}, nil
}

func (r *PostgresRepository) RevokeInvite(
	ctx context.Context,
	input RevokeInviteRepositoryInput,
) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	if err := r.requireAdminAccess(ctx, input.TeamID, input.ActorUserID); err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin revoke invite: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := lockTeam(ctx, tx, input.TeamID); err != nil {
		return err
	}
	actor, err := lockActorAccess(ctx, tx, input.TeamID, input.ActorUserID)
	if err != nil {
		return err
	}
	if actor.Role != TeamRoleAdmin {
		return ErrTeamAdminRequired
	}
	invite, err := lockInvite(ctx, tx, input.TeamID, input.InviteID)
	if err != nil {
		return err
	}
	if invite.Status == InviteStatusRevoked {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit repeated revoke invite: %w", err)
		}
		return nil
	}
	if invite.Status == InviteStatusAccepted || invite.ExpiresAt.UTC().Before(time.Now().UTC()) || invite.ExpiresAt.UTC().Equal(time.Now().UTC()) {
		return ErrInviteNotActive
	}
	if err := markInviteRevoked(ctx, tx, input.TeamID, input.InviteID); err != nil {
		return err
	}
	if err := cancelPendingMailOutbox(ctx, tx, input.InviteID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit revoke invite: %w", err)
	}
	return nil
}

func (r *PostgresRepository) DisableAccount(
	ctx context.Context,
	userID string,
) ([]auth.RevokedSession, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	userID = strings.TrimSpace(userID)
	if !isUUID(userID) {
		return nil, errors.New("user id must be a UUID")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin disable account: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	state, err := lockDisableUser(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	if !state.Exists || state.DeletedAtValid || !strings.EqualFold(state.AccountStatus, "active") {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit disable account noop: %w", err)
		}
		return nil, nil
	}

	teamIDs, err := listActiveMembershipTeamIDs(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	if err := lockTeamsByID(ctx, tx, teamIDs); err != nil {
		return nil, err
	}

	memberships, err := lockUserActiveMemberships(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	for _, membership := range memberships {
		if membership.Role != TeamRoleAdmin {
			continue
		}
		ok, err := hasOtherUsableAdmin(ctx, tx, membership.TeamID, userID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrLastTeamAdmin
		}
	}

	if err := disableUserRow(ctx, tx, userID); err != nil {
		return nil, err
	}
	revoked, err := revokeUserSessions(ctx, tx, userID)
	if err != nil {
		return nil, err
	}

	for _, membership := range memberships {
		revision, err := advanceMembershipRevision(ctx, tx, membership.TeamID)
		if err != nil {
			return nil, err
		}
		if err := r.insertMembershipOutbox(
			ctx,
			tx,
			membership.TeamID,
			userID,
			membership.Role,
			membershipEffectiveStatusDisabled,
			membershipOperationDisabled,
			revision,
		); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit disable account: %w", err)
	}
	return revoked, nil
}

func (r *PostgresRepository) requireDB() error {
	if r == nil || r.db == nil {
		return ErrDatabaseRequired
	}
	if r.newEventID == nil {
		r.newEventID = newUUID
	}
	return nil
}

func (r *PostgresRepository) lookupActorAccess(
	ctx context.Context,
	teamID string,
	actorUserID string,
) (actorAccess, error) {
	var access actorAccess
	err := r.db.QueryRowContext(ctx, `
SELECT m.role, m.created_at, m.updated_at
FROM team_memberships m
JOIN teams t ON t.id = m.team_id
JOIN users actor ON actor.id = m.user_id
WHERE m.team_id = $1
  AND m.user_id = $2
  AND m.status = 'active'
  AND t.deleted_at IS NULL
  AND actor.account_status = 'active'
  AND actor.deleted_at IS NULL
`, teamID, actorUserID).Scan(&access.Role, &access.JoinedAt, &access.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return actorAccess{}, ErrTeamNotFound
	}
	if err != nil {
		return actorAccess{}, fmt.Errorf("lookup actor team access: %w", err)
	}
	access.JoinedAt = access.JoinedAt.UTC()
	access.UpdatedAt = access.UpdatedAt.UTC()
	return access, nil
}

func (r *PostgresRepository) requireAdminAccess(
	ctx context.Context,
	teamID string,
	actorUserID string,
) error {
	access, err := r.lookupActorAccess(ctx, teamID, actorUserID)
	if err != nil {
		return err
	}
	if access.Role != TeamRoleAdmin {
		return ErrTeamAdminRequired
	}
	return nil
}

func lockActiveUser(ctx context.Context, tx *sql.Tx, userID string) error {
	return tx.QueryRowContext(ctx, `
SELECT id
FROM users
WHERE id = $1
  AND account_status = 'active'
  AND deleted_at IS NULL
FOR UPDATE
`, userID).Scan(new(string))
}

func insertTeamRow(
	ctx context.Context,
	tx *sql.Tx,
	input CreateTeamRepositoryInput,
) (teamRow, error) {
	var team teamRow
	err := tx.QueryRowContext(ctx, `
INSERT INTO teams (
  id,
  name,
  created_by_user_id,
  membership_revision,
  idempotency_key
) VALUES ($1, $2, $3, 1, $4)
RETURNING id, name, membership_revision, created_at, updated_at
`, input.ID, input.Name, input.CreatedByUserID, input.IdempotencyKey).Scan(
		&team.ID,
		&team.Name,
		&team.MembershipRevision,
		&team.CreatedAt,
		&team.UpdatedAt,
	)
	if err != nil {
		return teamRow{}, fmt.Errorf("insert team: %w", err)
	}
	return team, nil
}

func insertActiveMembership(
	ctx context.Context,
	tx *sql.Tx,
	teamID string,
	userID string,
	role string,
) (membershipRow, error) {
	var membership membershipRow
	err := tx.QueryRowContext(ctx, `
INSERT INTO team_memberships (team_id, user_id, role, status)
VALUES ($1, $2, $3, 'active')
RETURNING user_id, role, status, created_at, updated_at
`, teamID, userID, role).Scan(
		&membership.UserID,
		&membership.Role,
		&membership.Status,
		&membership.CreatedAt,
		&membership.UpdatedAt,
	)
	if err != nil {
		return membershipRow{}, fmt.Errorf("insert active membership: %w", err)
	}
	return membership, nil
}

func ensureTeamIdempotencyAvailable(
	ctx context.Context,
	query rowQuerier,
	creatorUserID string,
	idempotencyKey string,
) error {
	var existing string
	err := query.QueryRowContext(ctx, `
SELECT id
FROM teams
WHERE created_by_user_id = $1
  AND idempotency_key = $2
LIMIT 1
`, creatorUserID, idempotencyKey).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check team idempotency: %w", err)
	}
	return ErrIdempotencyConflict
}

func lockTeam(ctx context.Context, tx *sql.Tx, teamID string) (teamRow, error) {
	var team teamRow
	err := tx.QueryRowContext(ctx, `
SELECT id, name, membership_revision, created_at, updated_at
FROM teams
WHERE id = $1
  AND deleted_at IS NULL
FOR UPDATE
`, teamID).Scan(
		&team.ID,
		&team.Name,
		&team.MembershipRevision,
		&team.CreatedAt,
		&team.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return teamRow{}, ErrTeamNotFound
	}
	if err != nil {
		return teamRow{}, fmt.Errorf("lock team: %w", err)
	}
	return team, nil
}

func lockActorAccess(
	ctx context.Context,
	tx *sql.Tx,
	teamID string,
	actorUserID string,
) (actorAccess, error) {
	var access actorAccess
	err := tx.QueryRowContext(ctx, `
SELECT m.role, m.created_at, m.updated_at
FROM team_memberships m
JOIN users actor ON actor.id = m.user_id
WHERE m.team_id = $1
  AND m.user_id = $2
  AND m.status = 'active'
  AND actor.account_status = 'active'
  AND actor.deleted_at IS NULL
FOR UPDATE OF m
`, teamID, actorUserID).Scan(&access.Role, &access.JoinedAt, &access.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return actorAccess{}, ErrTeamNotFound
	}
	if err != nil {
		return actorAccess{}, fmt.Errorf("lock actor membership: %w", err)
	}
	return access, nil
}

func updateTeamName(ctx context.Context, tx *sql.Tx, teamID string, name string) (teamRow, error) {
	var team teamRow
	err := tx.QueryRowContext(ctx, `
UPDATE teams
SET name = $2,
    updated_at = now()
WHERE id = $1
  AND deleted_at IS NULL
RETURNING id, name, membership_revision, created_at, updated_at
`, teamID, name).Scan(
		&team.ID,
		&team.Name,
		&team.MembershipRevision,
		&team.CreatedAt,
		&team.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return teamRow{}, ErrTeamNotFound
	}
	if err != nil {
		return teamRow{}, fmt.Errorf("rename team: %w", err)
	}
	return team, nil
}

func lockTargetUser(ctx context.Context, tx *sql.Tx, userID string) (membershipRow, bool, error) {
	var user membershipRow
	err := tx.QueryRowContext(ctx, `
SELECT id, COALESCE(display_name, '')
FROM users
WHERE id = $1
FOR UPDATE
`, userID).Scan(&user.UserID, &user.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return membershipRow{}, true, nil
	}
	if err != nil {
		return membershipRow{}, false, fmt.Errorf("lock target user: %w", err)
	}
	return user, false, nil
}

func lockMembership(
	ctx context.Context,
	tx *sql.Tx,
	teamID string,
	userID string,
) (membershipRow, error) {
	var membership membershipRow
	err := tx.QueryRowContext(ctx, `
SELECT user_id, role, status, created_at, updated_at
FROM team_memberships
WHERE team_id = $1
  AND user_id = $2
  AND status = 'active'
FOR UPDATE
`, teamID, userID).Scan(
		&membership.UserID,
		&membership.Role,
		&membership.Status,
		&membership.CreatedAt,
		&membership.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return membershipRow{}, ErrTeamMemberNotFound
	}
	if err != nil {
		return membershipRow{}, fmt.Errorf("lock team membership: %w", err)
	}
	return membership, nil
}

func hasOtherUsableAdmin(
	ctx context.Context,
	query rowQuerier,
	teamID string,
	excludeUserID string,
) (bool, error) {
	var exists bool
	err := query.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM team_memberships m
  JOIN users u ON u.id = m.user_id
  WHERE m.team_id = $1
    AND m.status = 'active'
    AND m.role = 'admin'
    AND m.user_id <> $2
    AND u.account_status = 'active'
    AND u.deleted_at IS NULL
)
`, teamID, excludeUserID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check usable team admins: %w", err)
	}
	return exists, nil
}

func updateMembershipRoleRow(
	ctx context.Context,
	tx *sql.Tx,
	teamID string,
	userID string,
	role string,
) (membershipRow, error) {
	var membership membershipRow
	err := tx.QueryRowContext(ctx, `
UPDATE team_memberships
SET role = $3,
    updated_at = now()
WHERE team_id = $1
  AND user_id = $2
  AND status = 'active'
RETURNING user_id, role, status, created_at, updated_at
`, teamID, userID, role).Scan(
		&membership.UserID,
		&membership.Role,
		&membership.Status,
		&membership.CreatedAt,
		&membership.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return membershipRow{}, ErrTeamMemberNotFound
	}
	if err != nil {
		return membershipRow{}, fmt.Errorf("update member role: %w", err)
	}
	return membership, nil
}

func markMembershipRemoved(
	ctx context.Context,
	tx *sql.Tx,
	teamID string,
	userID string,
) (membershipRow, error) {
	var membership membershipRow
	err := tx.QueryRowContext(ctx, `
UPDATE team_memberships
SET status = 'removed',
    removed_at = now(),
    updated_at = now()
WHERE team_id = $1
  AND user_id = $2
  AND status = 'active'
RETURNING user_id, role, status, created_at, updated_at
`, teamID, userID).Scan(
		&membership.UserID,
		&membership.Role,
		&membership.Status,
		&membership.CreatedAt,
		&membership.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return membershipRow{}, ErrTeamMemberNotFound
	}
	if err != nil {
		return membershipRow{}, fmt.Errorf("remove team membership: %w", err)
	}
	return membership, nil
}

func advanceMembershipRevision(ctx context.Context, tx *sql.Tx, teamID string) (int64, error) {
	var revision int64
	err := tx.QueryRowContext(ctx, `
UPDATE teams
SET membership_revision = membership_revision + 1,
    updated_at = now()
WHERE id = $1
  AND deleted_at IS NULL
RETURNING membership_revision
`, teamID).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrTeamNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("advance membership revision: %w", err)
	}
	return revision, nil
}

func (r *PostgresRepository) insertMembershipOutbox(
	ctx context.Context,
	tx *sql.Tx,
	teamID string,
	userID string,
	role string,
	status string,
	operation string,
	revision int64,
) error {
	eventID, err := r.newEventID()
	if err != nil {
		return fmt.Errorf("generate membership outbox event id: %w", err)
	}
	eventID = strings.TrimSpace(eventID)
	if !isUUID(eventID) {
		return errors.New("membership outbox event id must be a UUID")
	}
	payload, err := json.Marshal(map[string]any{
		"teamId":             teamID,
		"userId":             userID,
		"operation":          operation,
		"teamRole":           role,
		"status":             status,
		"membershipRevision": revision,
	})
	if err != nil {
		return fmt.Errorf("marshal membership outbox payload: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO knowledge_outbox (
  event_id,
  aggregate_type,
  aggregate_key,
  event_type,
  payload
) VALUES ($1, 'team', $2, $3, $4::jsonb)
`, eventID, teamID, teamMembershipChangedEventType, string(payload)); err != nil {
		return fmt.Errorf("insert membership outbox: %w", err)
	}
	return nil
}

func ensureInviteIdempotencyAvailable(
	ctx context.Context,
	query rowQuerier,
	teamID string,
	inviterUserID string,
	idempotencyKey string,
) error {
	var existing string
	err := query.QueryRowContext(ctx, `
SELECT id
FROM team_invites
WHERE team_id = $1
  AND invited_by_user_id = $2
  AND idempotency_key = $3
LIMIT 1
`, teamID, inviterUserID, idempotencyKey).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check invite idempotency: %w", err)
	}
	return ErrIdempotencyConflict
}

func revokeExpiredPendingInvites(
	ctx context.Context,
	tx *sql.Tx,
	teamID string,
	email string,
) error {
	rows, err := tx.QueryContext(ctx, `
UPDATE team_invites
SET status = 'revoked',
    revoked_at = now(),
    updated_at = now()
WHERE team_id = $1
  AND email = $2
  AND status = 'pending'
  AND expires_at <= now()
RETURNING id
`, teamID, email)
	if err != nil {
		return fmt.Errorf("revoke expired pending invites: %w", err)
	}
	defer rows.Close()

	var inviteIDs []string
	for rows.Next() {
		var inviteID string
		if err := rows.Scan(&inviteID); err != nil {
			return fmt.Errorf("scan revoked expired invite id: %w", err)
		}
		inviteIDs = append(inviteIDs, inviteID)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate revoked expired invite ids: %w", err)
	}
	for _, inviteID := range inviteIDs {
		if err := cancelPendingMailOutbox(ctx, tx, inviteID); err != nil {
			return err
		}
	}
	return nil
}

func activeMembershipExistsByEmail(
	ctx context.Context,
	query rowQuerier,
	teamID string,
	email string,
) (bool, error) {
	var exists bool
	err := query.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM team_memberships m
  JOIN users u ON u.id = m.user_id
  WHERE m.team_id = $1
    AND m.status = 'active'
    AND lower(u.email) = $2
)
`, teamID, email).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check team membership email conflict: %w", err)
	}
	return exists, nil
}

func activePendingInviteExists(
	ctx context.Context,
	query rowQuerier,
	teamID string,
	email string,
) (bool, error) {
	var exists bool
	err := query.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM team_invites
  WHERE team_id = $1
    AND email = $2
    AND status = 'pending'
    AND expires_at > now()
)
`, teamID, email).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check pending invite conflict: %w", err)
	}
	return exists, nil
}

func insertInvite(
	ctx context.Context,
	tx *sql.Tx,
	input CreateInviteRepositoryInput,
) (inviteRow, error) {
	var invite inviteRow
	err := tx.QueryRowContext(ctx, `
INSERT INTO team_invites (
  id,
  team_id,
  invited_by_user_id,
  token_hash,
  email,
  role,
  expires_at,
  idempotency_key
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, email, role, status, expires_at, created_at, updated_at
`, input.ID, input.TeamID, input.InvitedByUserID, input.TokenHash, input.Email, input.TeamRole, input.ExpiresAt.UTC(), input.IdempotencyKey).Scan(
		&invite.ID,
		&invite.Email,
		&invite.Role,
		&invite.Status,
		&invite.ExpiresAt,
		&invite.CreatedAt,
		&invite.UpdatedAt,
	)
	if err != nil {
		return inviteRow{}, fmt.Errorf("insert invite: %w", err)
	}
	return invite, nil
}

func insertIdentityMailOutbox(
	ctx context.Context,
	tx *sql.Tx,
	teamID string,
	inviteID string,
	outbox IdentityMailOutboxInput,
) error {
	if _, err := tx.ExecContext(ctx, `
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
  error_code
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8,
  'pending', 0, 8, now(), NULL
)
`, outbox.ID, teamID, inviteID, outbox.KeyID, outbox.Version, outbox.Nonce, outbox.Ciphertext, outbox.MessageID); err != nil {
		return fmt.Errorf("insert identity mail outbox: %w", err)
	}
	return nil
}

func lockInvite(
	ctx context.Context,
	tx *sql.Tx,
	teamID string,
	inviteID string,
) (inviteRow, error) {
	var invite inviteRow
	err := tx.QueryRowContext(ctx, `
SELECT id, email, role, status, expires_at, created_at, updated_at
FROM team_invites
WHERE team_id = $1
  AND id = $2
FOR UPDATE
`, teamID, inviteID).Scan(
		&invite.ID,
		&invite.Email,
		&invite.Role,
		&invite.Status,
		&invite.ExpiresAt,
		&invite.CreatedAt,
		&invite.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return inviteRow{}, ErrInviteNotFound
	}
	if err != nil {
		return inviteRow{}, fmt.Errorf("lock invite: %w", err)
	}
	return invite, nil
}

func markInviteRevoked(
	ctx context.Context,
	tx *sql.Tx,
	teamID string,
	inviteID string,
) error {
	result, err := tx.ExecContext(ctx, `
UPDATE team_invites
SET status = 'revoked',
    revoked_at = now(),
    updated_at = now()
WHERE team_id = $1
  AND id = $2
  AND status = 'pending'
  AND expires_at > now()
`, teamID, inviteID)
	if err != nil {
		return fmt.Errorf("revoke invite: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("revoke invite rows affected: %w", err)
	}
	if rows != 1 {
		return ErrInviteNotActive
	}
	return nil
}

func cancelPendingMailOutbox(ctx context.Context, tx *sql.Tx, inviteID string) error {
	if _, err := tx.ExecContext(ctx, `
UPDATE identity_mail_outbox
SET status = 'cancelled',
    lease_owner = NULL,
    lease_expires_at = NULL,
    retry_at = NULL,
    terminal_at = now(),
    error_code = NULL,
    updated_at = now()
WHERE invite_id = $1
  AND status IN ('pending', 'processing')
`, inviteID); err != nil {
		return fmt.Errorf("cancel invite mail outbox: %w", err)
	}
	return nil
}

func lockDisableUser(ctx context.Context, tx *sql.Tx, userID string) (disableUserState, error) {
	var state disableUserState
	var deletedAt sql.NullTime
	err := tx.QueryRowContext(ctx, `
SELECT account_status, deleted_at
FROM users
WHERE id = $1
FOR UPDATE
`, userID).Scan(&state.AccountStatus, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return disableUserState{}, nil
	}
	if err != nil {
		return disableUserState{}, fmt.Errorf("lock disable user: %w", err)
	}
	state.Exists = true
	state.DeletedAtValid = deletedAt.Valid
	return state, nil
}

func listActiveMembershipTeamIDs(
	ctx context.Context,
	query queryExecer,
	userID string,
) ([]string, error) {
	rows, err := query.QueryContext(ctx, `
SELECT team_id
FROM team_memberships
WHERE user_id = $1
  AND status = 'active'
ORDER BY team_id ASC
`, userID)
	if err != nil {
		return nil, fmt.Errorf("list disable-account team ids: %w", err)
	}
	defer rows.Close()

	var teamIDs []string
	for rows.Next() {
		var teamID string
		if err := rows.Scan(&teamID); err != nil {
			return nil, fmt.Errorf("scan disable-account team id: %w", err)
		}
		teamIDs = append(teamIDs, teamID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate disable-account team ids: %w", err)
	}
	return teamIDs, nil
}

func lockUserActiveMemberships(
	ctx context.Context,
	query queryExecer,
	userID string,
) ([]membershipRow, error) {
	rows, err := query.QueryContext(ctx, `
SELECT team_id, role, status, created_at, updated_at
FROM team_memberships
WHERE user_id = $1
  AND status = 'active'
ORDER BY team_id ASC
FOR UPDATE
`, userID)
	if err != nil {
		return nil, fmt.Errorf("lock disable-account memberships: %w", err)
	}
	defer rows.Close()

	var memberships []membershipRow
	for rows.Next() {
		var membership membershipRow
		if err := rows.Scan(
			&membership.TeamID,
			&membership.Role,
			&membership.Status,
			&membership.CreatedAt,
			&membership.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan disable-account membership: %w", err)
		}
		memberships = append(memberships, membership)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate disable-account memberships: %w", err)
	}
	return memberships, nil
}

func lockTeamsByID(ctx context.Context, tx *sql.Tx, teamIDs []string) error {
	if len(teamIDs) == 0 {
		return nil
	}

	ordered := append([]string(nil), teamIDs...)
	sort.Strings(ordered)
	for _, teamID := range ordered {
		if _, err := lockTeam(ctx, tx, teamID); err != nil {
			return err
		}
	}
	return nil
}

func disableUserRow(ctx context.Context, tx *sql.Tx, userID string) error {
	result, err := tx.ExecContext(ctx, `
UPDATE users
SET account_status = 'disabled',
    updated_at = now()
WHERE id = $1
  AND account_status = 'active'
  AND deleted_at IS NULL
`, userID)
	if err != nil {
		return fmt.Errorf("disable user account: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("disable user account rows affected: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("disable user account lost user fence: affected %d rows", rows)
	}
	return nil
}

func revokeUserSessions(
	ctx context.Context,
	query queryExecer,
	userID string,
) ([]auth.RevokedSession, error) {
	rows, err := query.QueryContext(ctx, `
UPDATE sessions
SET revoked_at = COALESCE(revoked_at, now()),
    updated_at = now()
WHERE user_id = $1
  AND revoked_at IS NULL
RETURNING id, token_hash
`, userID)
	if err != nil {
		return nil, fmt.Errorf("revoke disabled-user sessions: %w", err)
	}
	defer rows.Close()

	var revoked []auth.RevokedSession
	for rows.Next() {
		var session auth.RevokedSession
		if err := rows.Scan(&session.ID, &session.TokenHash); err != nil {
			return nil, fmt.Errorf("scan revoked disabled-user session: %w", err)
		}
		revoked = append(revoked, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate revoked disabled-user sessions: %w", err)
	}
	return revoked, nil
}

func repositoryPageLimit(limit int) int {
	if limit <= 0 {
		return defaultPageLimit
	}
	if limit > maximumPageLimit {
		return maximumPageLimit
	}
	return limit
}

func isConstraintViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(pgErr.ConstraintName), strings.TrimSpace(constraint))
}
