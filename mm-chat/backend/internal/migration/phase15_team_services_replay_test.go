package migration

import (
	"context"
	"testing"
	"time"
)

func TestPhase151CTeamServicesReplay(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	baseRunner := NewRunner(db, phase151CMigrationFS(t, phase151CBaseMigrationFiles...))
	applied, err := baseRunner.Up(ctx)
	if err != nil {
		t.Fatalf("apply 001-004 migrations: %v", err)
	}
	if len(applied) != len(phase151CBaseMigrationFiles)/2 {
		t.Fatalf("apply 001-004 migrations = %d changes, want %d", len(applied), len(phase151CBaseMigrationFiles)/2)
	}

	fixture := seedPhase151CReplayFixture(t, ctx, db)

	fullRunner := NewRunner(db, phase151CMigrationFS(t, phase151CAllMigrationFiles...))
	applied, err = fullRunner.Up(ctx)
	if err != nil {
		t.Fatalf("apply 005 migration: %v", err)
	}
	if len(applied) != 1 || applied[0].ID() != "005_phase15_team_services" {
		t.Fatalf("apply 005 migration = %#v, want only 005_phase15_team_services", applied)
	}

	assertPhase151CColumnExists(t, ctx, db, "teams", "idempotency_key")
	assertPhase151CColumnExists(t, ctx, db, "team_invites", "idempotency_key")
	assertPhase151CTableExists(t, ctx, db, "identity_mail_outbox")
	assertPhase151CFKDeleteAction(t, ctx, db, "team_memberships", "team_memberships_user_id_fkey", "r")
	assertPhase151CNullColumnValue(t, ctx, db, "teams", "idempotency_key", fixture.teamID)
	assertPhase151CNullColumnValue(t, ctx, db, "team_invites", "idempotency_key", fixture.inviteID)
	assertPhase151CRowCount(t, ctx, db, "team_memberships", 2)
	assertPhase151CRowCount(t, ctx, db, "team_invites", 1)

	mustExecPhase151C(t, ctx, db, `
UPDATE teams
SET idempotency_key = $2
WHERE id = $1
`, fixture.teamID, "team-create-1")
	assertPhase151CUniqueViolation(t, mustExecPhase151CReturnError(ctx, db, `
INSERT INTO teams (id, name, created_by_user_id, idempotency_key)
VALUES ($1, $2, $3, $4)
`, fixture.teamIDConflict, "Replay Duplicate Team", fixture.creatorUserID, "team-create-1"))

	mustExecPhase151C(t, ctx, db, `
UPDATE team_invites
SET idempotency_key = $2
WHERE id = $1
`, fixture.inviteID, "invite-create-1")
	assertPhase151CUniqueViolation(t, mustExecPhase151CReturnError(ctx, db, `
INSERT INTO team_invites (
  id, team_id, invited_by_user_id, token_hash, email, role, expires_at, idempotency_key
) VALUES ($1, $2, $3, $4, $5, 'member', now() + interval '2 hours', $6)
`,
		fixture.inviteIDConflict,
		fixture.teamID,
		fixture.creatorUserID,
		fixture.inviteTokenHashConflict,
		"other-"+fixture.memberUserID+"@example.test",
		"invite-create-1",
	))
	assertPhase151CUniqueViolation(t, mustExecPhase151CReturnError(ctx, db, `
INSERT INTO team_invites (
  id, team_id, invited_by_user_id, token_hash, email, role, expires_at
) VALUES ($1, $2, $3, $4, $5, 'member', now() + interval '2 hours')
`,
		fixture.inviteDuplicatePendingID,
		fixture.teamID,
		fixture.creatorUserID,
		fixture.inviteTokenHashDuplicate,
		fixture.inviteEmail,
	))
	mustExecPhase151C(t, ctx, db, `
INSERT INTO team_invites (
  id, team_id, invited_by_user_id, token_hash, email, role, status, expires_at, revoked_at
) VALUES ($1, $2, $3, $4, $5, 'member', 'revoked', now() + interval '2 hours', now())
`,
		fixture.inviteRevokedID,
		fixture.teamID,
		fixture.creatorUserID,
		fixture.inviteTokenHashRevoked,
		fixture.inviteEmail,
	)

	assertPhase151CForeignKeyViolation(t, mustExecPhase151CReturnError(ctx, db, `
DELETE FROM users
WHERE id = $1
`, fixture.memberUserID))

	mustExecPhase151C(t, ctx, db, `
INSERT INTO identity_mail_outbox (
  id, team_id, invite_id, key_id, payload_version, nonce, ciphertext, message_id
) VALUES ($1, $2, $3, $4, 1, $5, $6, $7)
`,
		fixture.outboxID,
		fixture.teamID,
		fixture.inviteID,
		"mail-key-1",
		[]byte("0123456789ab"),
		[]byte{0x01, 0x02, 0x03},
		"<invite-"+fixture.inviteID+"@example.test>",
	)
	assertPhase151CUniqueViolation(t, mustExecPhase151CReturnError(ctx, db, `
INSERT INTO identity_mail_outbox (
  id, team_id, invite_id, key_id, payload_version, nonce, ciphertext, message_id
) VALUES ($1, $2, $3, $4, 1, $5, $6, $7)
`,
		fixture.outboxDuplicateID,
		fixture.teamID,
		fixture.inviteID,
		"mail-key-2",
		[]byte("fedcba987654"),
		[]byte{0x04, 0x05, 0x06},
		"<invite-dupe-"+fixture.inviteID+"@example.test>",
	))

	rolledBack, err := fullRunner.Down(ctx, false)
	if err != nil {
		t.Fatalf("rollback 005 migration: %v", err)
	}
	if len(rolledBack) != 1 || rolledBack[0].ID() != "005_phase15_team_services" {
		t.Fatalf("rollback 005 migration = %#v, want only 005_phase15_team_services", rolledBack)
	}

	assertPhase151CColumnAbsent(t, ctx, db, "teams", "idempotency_key")
	assertPhase151CColumnAbsent(t, ctx, db, "team_invites", "idempotency_key")
	assertPhase151CTableAbsent(t, ctx, db, "identity_mail_outbox")
	assertPhase151CFKDeleteAction(t, ctx, db, "team_memberships", "team_memberships_user_id_fkey", "c")
	assertPhase151CRowCount(t, ctx, db, "team_memberships", 2)
	assertPhase151CRowCount(t, ctx, db, "team_invites", 2)

	mustExecPhase151C(t, ctx, db, `
INSERT INTO users (id, email, display_name)
VALUES ($1, $2, 'Cascade Replay User')
`, fixture.tempCascadeUserID, "cascade-"+fixture.tempCascadeUserID+"@example.test")
	mustExecPhase151C(t, ctx, db, `
INSERT INTO team_memberships (team_id, user_id, role)
VALUES ($1, $2, 'member')
`, fixture.teamID, fixture.tempCascadeUserID)
	mustExecPhase151C(t, ctx, db, `
DELETE FROM users
WHERE id = $1
`, fixture.tempCascadeUserID)
	assertPhase151CMembershipAbsent(t, ctx, db, fixture.teamID, fixture.tempCascadeUserID)

	applied, err = fullRunner.Up(ctx)
	if err != nil {
		t.Fatalf("reapply 005 migration: %v", err)
	}
	if len(applied) != 1 || applied[0].ID() != "005_phase15_team_services" {
		t.Fatalf("reapply 005 migration = %#v, want only 005_phase15_team_services", applied)
	}

	assertPhase151CColumnExists(t, ctx, db, "teams", "idempotency_key")
	assertPhase151CColumnExists(t, ctx, db, "team_invites", "idempotency_key")
	assertPhase151CTableExists(t, ctx, db, "identity_mail_outbox")
	assertPhase151CFKDeleteAction(t, ctx, db, "team_memberships", "team_memberships_user_id_fkey", "r")
	assertPhase151CRowCount(t, ctx, db, "team_memberships", 2)
	assertPhase151CRowCount(t, ctx, db, "team_invites", 2)
}
