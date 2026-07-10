package teams

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

type repositoryTeamMembershipChangedPayload struct {
	TeamID             string `json:"teamId"`
	UserID             string `json:"userId"`
	Operation          string `json:"operation"`
	TeamRole           string `json:"teamRole"`
	Status             string `json:"status"`
	MembershipRevision int64  `json:"membershipRevision"`
}

func TestPostgresRepositoryDisableAccountRejectsLastUsableAdmin(t *testing.T) {
	db := openTeamsPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userID := mustTeamsTestUUID(t)
	teamID := mustTeamsTestUUID(t)
	sessionID := mustTeamsTestUUID(t)
	insertTeamsUserFixture(t, ctx, db, userID, "last-admin@example.test", "Last Admin", "active")
	insertTeamsSessionFixture(t, ctx, db, sessionID, userID, strings.Repeat("a", 64))
	insertTeamsTeamFixture(t, ctx, db, teamID, userID, "Solo Team")
	insertTeamsMembershipFixture(t, ctx, db, teamID, userID, TeamRoleAdmin)

	_, err := repo.DisableAccount(ctx, userID)
	if !errors.Is(err, ErrLastTeamAdmin) {
		t.Fatalf("DisableAccount() error = %v, want ErrLastTeamAdmin", err)
	}

	if got := loadTeamsUserAccountStatus(t, ctx, db, userID); got != "active" {
		t.Fatalf("user account_status = %q, want active", got)
	}
	if revoked := loadTeamsSessionRevocationCount(t, ctx, db, userID); revoked != 0 {
		t.Fatalf("revoked session count = %d, want 0", revoked)
	}
	if revision := loadTeamsMembershipRevision(t, ctx, db, teamID); revision != 1 {
		t.Fatalf("team membership_revision = %d, want 1", revision)
	}
	if payloads := loadTeamsMembershipChangedPayloads(t, ctx, db, teamID); len(payloads) != 0 {
		t.Fatalf("membership payloads = %#v, want empty", payloads)
	}
}

func TestPostgresRepositoryDisableAccountHandlesMultipleTeamsAtomically(t *testing.T) {
	db := openTeamsPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	targetUserID := mustTeamsTestUUID(t)
	teamAdminAID := mustTeamsTestUUID(t)
	teamAdminBID := mustTeamsTestUUID(t)
	sessionAID := mustTeamsTestUUID(t)
	sessionBID := mustTeamsTestUUID(t)
	teamAID := "00000000-0000-0000-0000-000000000201"
	teamBID := "00000000-0000-0000-0000-000000000202"

	insertTeamsUserFixture(t, ctx, db, targetUserID, "target@example.test", "Target User", "active")
	insertTeamsUserFixture(t, ctx, db, teamAdminAID, "admin-a@example.test", "Team Admin A", "active")
	insertTeamsUserFixture(t, ctx, db, teamAdminBID, "admin-b@example.test", "Team Admin B", "active")
	insertTeamsSessionFixture(t, ctx, db, sessionAID, targetUserID, strings.Repeat("b", 64))
	insertTeamsSessionFixture(t, ctx, db, sessionBID, targetUserID, strings.Repeat("c", 64))
	insertTeamsTeamFixture(t, ctx, db, teamAID, teamAdminAID, "Alpha Team")
	insertTeamsTeamFixture(t, ctx, db, teamBID, teamAdminBID, "Beta Team")
	insertTeamsMembershipFixture(t, ctx, db, teamAID, teamAdminAID, TeamRoleAdmin)
	insertTeamsMembershipFixture(t, ctx, db, teamAID, targetUserID, TeamRoleMember)
	insertTeamsMembershipFixture(t, ctx, db, teamBID, teamAdminBID, TeamRoleAdmin)
	insertTeamsMembershipFixture(t, ctx, db, teamBID, targetUserID, TeamRoleAdmin)

	revoked, err := repo.DisableAccount(ctx, targetUserID)
	if err != nil {
		t.Fatalf("DisableAccount() error = %v", err)
	}
	assertRevokedSessionsEqual(t, revoked, []auth.RevokedSession{
		{ID: sessionAID, TokenHash: strings.Repeat("b", 64)},
		{ID: sessionBID, TokenHash: strings.Repeat("c", 64)},
	})

	if got := loadTeamsUserAccountStatus(t, ctx, db, targetUserID); got != "disabled" {
		t.Fatalf("user account_status = %q, want disabled", got)
	}
	if revokedCount := loadTeamsSessionRevocationCount(t, ctx, db, targetUserID); revokedCount != 2 {
		t.Fatalf("revoked session count = %d, want 2", revokedCount)
	}
	if status := loadTeamsMembershipStatus(t, ctx, db, teamAID, targetUserID); status != MembershipStatusActive {
		t.Fatalf("team A membership status = %q, want active", status)
	}
	if status := loadTeamsMembershipStatus(t, ctx, db, teamBID, targetUserID); status != MembershipStatusActive {
		t.Fatalf("team B membership status = %q, want active", status)
	}
	if revision := loadTeamsMembershipRevision(t, ctx, db, teamAID); revision != 2 {
		t.Fatalf("team A membership_revision = %d, want 2", revision)
	}
	if revision := loadTeamsMembershipRevision(t, ctx, db, teamBID); revision != 2 {
		t.Fatalf("team B membership_revision = %d, want 2", revision)
	}

	payloadsA := loadTeamsMembershipChangedPayloads(t, ctx, db, teamAID)
	if len(payloadsA) != 1 {
		t.Fatalf("team A payloads = %#v", payloadsA)
	}
	if payloadsA[0].UserID != targetUserID ||
		payloadsA[0].Operation != membershipOperationDisabled ||
		payloadsA[0].TeamRole != TeamRoleMember ||
		payloadsA[0].Status != membershipEffectiveStatusDisabled ||
		payloadsA[0].MembershipRevision != 2 {
		t.Fatalf("team A payload = %#v", payloadsA[0])
	}
	payloadsB := loadTeamsMembershipChangedPayloads(t, ctx, db, teamBID)
	if len(payloadsB) != 1 {
		t.Fatalf("team B payloads = %#v", payloadsB)
	}
	if payloadsB[0].UserID != targetUserID ||
		payloadsB[0].Operation != membershipOperationDisabled ||
		payloadsB[0].TeamRole != TeamRoleAdmin ||
		payloadsB[0].Status != membershipEffectiveStatusDisabled ||
		payloadsB[0].MembershipRevision != 2 {
		t.Fatalf("team B payload = %#v", payloadsB[0])
	}
}

func TestPostgresRepositoryDisableAccountRollsBackOnOutboxFailure(t *testing.T) {
	db := openTeamsPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	repo.newEventID = func() (string, error) { return "not-a-uuid", nil }
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	targetUserID := mustTeamsTestUUID(t)
	teamAdminID := mustTeamsTestUUID(t)
	teamID := mustTeamsTestUUID(t)
	sessionID := mustTeamsTestUUID(t)
	insertTeamsUserFixture(t, ctx, db, targetUserID, "rollback-target@example.test", "Rollback Target", "active")
	insertTeamsUserFixture(t, ctx, db, teamAdminID, "rollback-admin@example.test", "Rollback Admin", "active")
	insertTeamsSessionFixture(t, ctx, db, sessionID, targetUserID, strings.Repeat("d", 64))
	insertTeamsTeamFixture(t, ctx, db, teamID, teamAdminID, "Rollback Team")
	insertTeamsMembershipFixture(t, ctx, db, teamID, teamAdminID, TeamRoleAdmin)
	insertTeamsMembershipFixture(t, ctx, db, teamID, targetUserID, TeamRoleMember)

	_, err := repo.DisableAccount(ctx, targetUserID)
	if err == nil || !strings.Contains(err.Error(), "membership outbox event id must be a UUID") {
		t.Fatalf("DisableAccount() error = %v, want invalid outbox id", err)
	}

	if got := loadTeamsUserAccountStatus(t, ctx, db, targetUserID); got != "active" {
		t.Fatalf("user account_status = %q, want active", got)
	}
	if revokedCount := loadTeamsSessionRevocationCount(t, ctx, db, targetUserID); revokedCount != 0 {
		t.Fatalf("revoked session count = %d, want 0", revokedCount)
	}
	if revision := loadTeamsMembershipRevision(t, ctx, db, teamID); revision != 1 {
		t.Fatalf("team membership_revision = %d, want 1", revision)
	}
	if payloads := loadTeamsMembershipChangedPayloads(t, ctx, db, teamID); len(payloads) != 0 {
		t.Fatalf("membership payloads = %#v, want empty", payloads)
	}
}

func TestPostgresRepositoryDisableAccountWithoutMembershipsSkipsTeamEffects(t *testing.T) {
	db := openTeamsPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userID := mustTeamsTestUUID(t)
	sessionID := mustTeamsTestUUID(t)
	insertTeamsUserFixture(t, ctx, db, userID, "no-memberships@example.test", "No Memberships", "active")
	insertTeamsSessionFixture(t, ctx, db, sessionID, userID, strings.Repeat("e", 64))

	revoked, err := repo.DisableAccount(ctx, userID)
	if err != nil {
		t.Fatalf("DisableAccount() error = %v", err)
	}
	assertRevokedSessionsEqual(t, revoked, []auth.RevokedSession{{
		ID:        sessionID,
		TokenHash: strings.Repeat("e", 64),
	}})
	if got := loadTeamsUserAccountStatus(t, ctx, db, userID); got != "disabled" {
		t.Fatalf("user account_status = %q, want disabled", got)
	}
	if payloadCount := countTeamsMembershipChangedPayloadsForUser(t, ctx, db, userID); payloadCount != 0 {
		t.Fatalf("membership payload count = %d, want 0", payloadCount)
	}
}

func TestPostgresRepositoryDisableAccountAlreadyDisabledIsNoop(t *testing.T) {
	db := openTeamsPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userID := mustTeamsTestUUID(t)
	insertTeamsUserFixture(t, ctx, db, userID, "already-disabled@example.test", "Already Disabled", "disabled")

	revoked, err := repo.DisableAccount(ctx, userID)
	if err != nil {
		t.Fatalf("DisableAccount() error = %v", err)
	}
	if len(revoked) != 0 {
		t.Fatalf("revoked sessions = %#v, want empty", revoked)
	}
	if got := loadTeamsUserAccountStatus(t, ctx, db, userID); got != "disabled" {
		t.Fatalf("user account_status = %q, want disabled", got)
	}
	if payloadCount := countTeamsMembershipChangedPayloadsForUser(t, ctx, db, userID); payloadCount != 0 {
		t.Fatalf("membership payload count = %d, want 0", payloadCount)
	}
}

func TestPostgresRepositoryRemoveMemberRejectsSelfRemoval(t *testing.T) {
	db := openTeamsPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userID := mustTeamsTestUUID(t)
	teamID := mustTeamsTestUUID(t)
	insertTeamsUserFixture(t, ctx, db, userID, "self-remove@example.test", "Self Remove", "active")
	insertTeamsTeamFixture(t, ctx, db, teamID, userID, "Self Remove Team")
	insertTeamsMembershipFixture(t, ctx, db, teamID, userID, TeamRoleAdmin)

	err := repo.RemoveMember(ctx, RemoveMemberRepositoryInput{
		TeamID:       teamID,
		ActorUserID:  userID,
		TargetUserID: userID,
	})
	var validationErr ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("RemoveMember() error = %v, want ValidationError", err)
	}
	if validationErr.Code != ErrorCodeInvalidMembershipPayload {
		t.Fatalf("RemoveMember() code = %q, want %q", validationErr.Code, ErrorCodeInvalidMembershipPayload)
	}
	if !strings.Contains(validationErr.Message, "self-leave") {
		t.Fatalf("RemoveMember() message = %q, want self-leave hint", validationErr.Message)
	}
	if status := loadTeamsMembershipStatus(t, ctx, db, teamID, userID); status != MembershipStatusActive {
		t.Fatalf("membership status = %q, want active", status)
	}
	if revision := loadTeamsMembershipRevision(t, ctx, db, teamID); revision != 1 {
		t.Fatalf("team membership_revision = %d, want 1", revision)
	}
	if payloads := loadTeamsMembershipChangedPayloads(t, ctx, db, teamID); len(payloads) != 0 {
		t.Fatalf("membership payloads = %#v, want empty", payloads)
	}
}

func openTeamsPostgresIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()

	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set MM_CHAT_TEST_DATABASE_URL to run Postgres integration tests")
	}

	pgxConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse MM_CHAT_TEST_DATABASE_URL: %v", err)
	}
	pgxConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	db := stdlib.OpenDB(*pgxConfig)
	t.Cleanup(func() {
		_ = db.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping integration database: %v", err)
	}
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	return db
}

func mustTeamsTestUUID(t *testing.T) string {
	t.Helper()

	id, err := newUUID()
	if err != nil {
		t.Fatalf("newUUID() error = %v", err)
	}
	return id
}

func insertTeamsUserFixture(
	t *testing.T,
	ctx context.Context,
	db interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	},
	userID string,
	email string,
	displayName string,
	accountStatus string,
) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, email, display_name, account_status)
VALUES ($1, $2, $3, $4)
`, userID, email, displayName, accountStatus); err != nil {
		t.Fatalf("insert user fixture: %v", err)
	}
}

func insertTeamsSessionFixture(
	t *testing.T,
	ctx context.Context,
	db interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	},
	sessionID string,
	userID string,
	tokenHash string,
) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
INSERT INTO sessions (id, user_id, token_hash, user_agent, expires_at)
VALUES ($1, $2, $3, 'teams-test', now() + interval '1 hour')
`, sessionID, userID, tokenHash); err != nil {
		t.Fatalf("insert session fixture: %v", err)
	}
}

func insertTeamsTeamFixture(
	t *testing.T,
	ctx context.Context,
	db interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	},
	teamID string,
	creatorUserID string,
	name string,
) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
INSERT INTO teams (id, name, created_by_user_id)
VALUES ($1, $2, $3)
`, teamID, name, creatorUserID); err != nil {
		t.Fatalf("insert team fixture: %v", err)
	}
}

func insertTeamsMembershipFixture(
	t *testing.T,
	ctx context.Context,
	db interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	},
	teamID string,
	userID string,
	role string,
) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
INSERT INTO team_memberships (team_id, user_id, role)
VALUES ($1, $2, $3)
`, teamID, userID, role); err != nil {
		t.Fatalf("insert membership fixture: %v", err)
	}
}

func loadTeamsUserAccountStatus(
	t *testing.T,
	ctx context.Context,
	db interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	userID string,
) string {
	t.Helper()

	var status string
	if err := db.QueryRowContext(ctx, `
SELECT account_status
FROM users
WHERE id = $1
`, userID).Scan(&status); err != nil {
		t.Fatalf("query user account_status: %v", err)
	}
	return status
}

func loadTeamsSessionRevocationCount(
	t *testing.T,
	ctx context.Context,
	db interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	userID string,
) int {
	t.Helper()

	var count int
	if err := db.QueryRowContext(ctx, `
SELECT count(*)
FROM sessions
WHERE user_id = $1
  AND revoked_at IS NOT NULL
`, userID).Scan(&count); err != nil {
		t.Fatalf("query revoked session count: %v", err)
	}
	return count
}

func loadTeamsMembershipRevision(
	t *testing.T,
	ctx context.Context,
	db interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	teamID string,
) int64 {
	t.Helper()

	var revision int64
	if err := db.QueryRowContext(ctx, `
SELECT membership_revision
FROM teams
WHERE id = $1
`, teamID).Scan(&revision); err != nil {
		t.Fatalf("query membership revision: %v", err)
	}
	return revision
}

func loadTeamsMembershipStatus(
	t *testing.T,
	ctx context.Context,
	db interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	teamID string,
	userID string,
) string {
	t.Helper()

	var status string
	if err := db.QueryRowContext(ctx, `
SELECT status
FROM team_memberships
WHERE team_id = $1
  AND user_id = $2
`, teamID, userID).Scan(&status); err != nil {
		t.Fatalf("query membership status: %v", err)
	}
	return status
}

func loadTeamsMembershipChangedPayloads(
	t *testing.T,
	ctx context.Context,
	db interface {
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	},
	teamID string,
) []repositoryTeamMembershipChangedPayload {
	t.Helper()

	rows, err := db.QueryContext(ctx, `
SELECT payload
FROM knowledge_outbox
WHERE aggregate_type = 'team'
  AND aggregate_key = $1
  AND event_type = $2
ORDER BY id ASC
`, teamID, teamMembershipChangedEventType)
	if err != nil {
		t.Fatalf("query membership payloads: %v", err)
	}
	defer rows.Close()

	var payloads []repositoryTeamMembershipChangedPayload
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("scan membership payload: %v", err)
		}
		var payload repositoryTeamMembershipChangedPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("unmarshal membership payload: %v", err)
		}
		payloads = append(payloads, payload)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate membership payloads: %v", err)
	}
	return payloads
}

func countTeamsMembershipChangedPayloadsForUser(
	t *testing.T,
	ctx context.Context,
	db interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	userID string,
) int {
	t.Helper()

	var count int
	if err := db.QueryRowContext(ctx, `
SELECT count(*)
FROM knowledge_outbox
WHERE aggregate_type = 'team'
  AND event_type = $1
  AND payload->>'userId' = $2
`, teamMembershipChangedEventType, userID).Scan(&count); err != nil {
		t.Fatalf("count membership payloads by user: %v", err)
	}
	return count
}

func assertRevokedSessionsEqual(t *testing.T, got []auth.RevokedSession, want []auth.RevokedSession) {
	t.Helper()

	sort.Slice(got, func(i, j int) bool { return got[i].ID < got[j].ID })
	sort.Slice(want, func(i, j int) bool { return want[i].ID < want[j].ID })
	if len(got) != len(want) {
		t.Fatalf("revoked sessions = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("revoked sessions = %#v, want %#v", got, want)
		}
	}
}
