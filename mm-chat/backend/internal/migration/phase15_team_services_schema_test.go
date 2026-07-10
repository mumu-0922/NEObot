package migration

import (
	"regexp"
	"testing"
)

const (
	phase151CUpPath   = "005_phase15_team_services.up.sql"
	phase151CDownPath = "005_phase15_team_services.down.sql"
)

func TestPhase151CTeamServicesSchemaContract(t *testing.T) {
	up := readPhase15SQL(t, phase151CUpPath)
	down := readPhase15SQL(t, phase151CDownPath)

	t.Run("adds bounded nullable idempotency keys with scoped partial uniqueness", func(t *testing.T) {
		for _, fragment := range []string{
			"alter table teams add column idempotency_key text",
			"alter table team_invites add column idempotency_key text",
			"octet_length ( idempotency_key ) between 1 and 128",
			"length ( trim ( idempotency_key ) ) > 0",
		} {
			assertPhase15Fragments(t, up,
				"phase 15.1c must add bounded nullable idempotency keys",
				fragment)
		}

		assertPhase151CPartialUniqueIndex(t, up,
			"teams idempotency must be scoped to the creator",
			"idx_teams_created_by_idempotency",
			"teams",
			[]string{"created_by_user_id", "idempotency_key"},
			[]string{"idempotency_key is not null"},
		)
		assertPhase151CPartialUniqueIndex(t, up,
			"team invite idempotency must be scoped to the team and inviter",
			"idx_team_invites_team_inviter_idempotency",
			"team_invites",
			[]string{"team_id", "invited_by_user_id", "idempotency_key"},
			[]string{"idempotency_key is not null"},
		)
	})

	t.Run("permits only one pending invite per team email", func(t *testing.T) {
		assertPhase151CPartialUniqueIndex(t, up,
			"each team/email pair must have at most one pending invite",
			"idx_team_invites_pending_team_email",
			"team_invites",
			[]string{"team_id", "email"},
			[]string{"status = 'pending'"},
		)
	})

	t.Run("replaces memberships user delete behavior with same-name restrict and restores cascade on down", func(t *testing.T) {
		assertPhase15Fragments(t, up,
			"phase 15.1c up must drop the old memberships user fk before recreating it",
			"alter table team_memberships drop constraint team_memberships_user_id_fkey",
			"add constraint team_memberships_user_id_fkey foreign key ( user_id ) references users ( id ) on delete restrict",
		)
		assertPhase15Fragments(t, down,
			"phase 15.1c down must restore the original memberships user fk action",
			"alter table team_memberships drop constraint team_memberships_user_id_fkey",
			"add constraint team_memberships_user_id_fkey foreign key ( user_id ) references users ( id ) on delete cascade",
		)
		assertPhase15Order(t, up,
			"drop constraint team_memberships_user_id_fkey",
			"add constraint team_memberships_user_id_fkey",
			"the runner transaction must not leave the migration without recreating the memberships user fk",
		)
	})

	t.Run("creates encrypted identity mail outbox without plaintext token password or session columns", func(t *testing.T) {
		outbox := mustPhase15TableBody(t, up, "identity_mail_outbox")
		assertPhase15Columns(t, outbox, "identity_mail_outbox",
			"id", "team_id", "invite_id", "key_id", "payload_version", "nonce", "ciphertext",
			"message_id", "status", "attempt_count", "max_attempts",
			"available_at", "lease_owner", "lease_expires_at", "retry_at", "terminal_at",
			"error_code", "created_at", "updated_at")
		assertPhase15Fragments(t, outbox,
			"identity mail outbox must use one durable row per invite with stable ids",
			"id uuid primary key",
			"invite_id uuid not null unique",
			"message_id text not null unique")
		assertPhase15Fragments(t, outbox,
			"identity mail outbox must persist only encrypted invite delivery material",
			"octet_length ( nonce ) = 12",
			"unique ( key_id , nonce )",
			"payload_version between 1 and 32767",
			"octet_length ( ciphertext ) > 0",
			"error_code is null or error_code ~ '^[a-z0-9_]{1 , 64}$'",
		)
		assertPhase15Fragments(t, outbox,
			"identity mail outbox statuses must fence lease retry and terminal timestamps",
			"status in ( 'pending' , 'processing' , 'sent' , 'failed' , 'cancelled' )",
			"status = 'pending'",
			"lease_expires_at is null",
			"status = 'processing'",
			"lease_owner is not null",
			"lease_expires_at is not null",
			"status = 'sent'",
			"terminal_at is not null",
			"status = 'failed'",
			"error_code is not null",
			"status = 'cancelled'")
		assertPhase15Fragments(t, outbox,
			"identity mail outbox must bound retry attempts",
			"attempt_count >= 0",
			"max_attempts between 1 and 32",
			"attempt_count <= max_attempts",
			"retry_at is null or retry_at = available_at")
		assertPhase15Fragments(t,
			phase15TableDDL(t, up, "identity_mail_outbox"),
			"identity mail outbox must bind team and invite identity with a cascade fk",
			"foreign key ( team_id , invite_id ) references team_invites ( team_id , id ) on delete cascade",
		)
		if regexp.MustCompile(`\b(?:raw_)?token\b|\bpassword\b|\bsession\b|\bplaintext\b|\bacceptance_url\b|\bemail\b`).MatchString(outbox) {
			t.Fatal("identity_mail_outbox must not expose plaintext email, token, password, session, or acceptance URL columns")
		}

		assertPhase151CPartialIndex(t, up,
			"pending mail delivery claims must scan by available_at and id",
			"idx_identity_mail_outbox_pending",
			"identity_mail_outbox",
			[]string{"available_at", "id"},
			[]string{"status = 'pending'"},
		)
		assertPhase151CPartialIndex(t, up,
			"processing mail delivery claims must reclaim expired leases by lease_expires_at and id",
			"idx_identity_mail_outbox_processing",
			"identity_mail_outbox",
			[]string{"lease_expires_at", "id"},
			[]string{"status = 'processing'"},
		)
	})

	t.Run("down removes phase 15.1c objects before restoring cascade", func(t *testing.T) {
		createdTables := phase15CreatedTables(up)
		for _, table := range createdTables {
			if !phase15DropsTable(down, table) {
				t.Errorf("Phase 15.1C down migration does not drop up-created table %q", table)
			}
		}

		createdIndexes := phase15CreatedIndexes(up)
		for _, index := range createdIndexes {
			if !phase15DropsIndex(down, index) {
				t.Errorf("Phase 15.1C down migration does not drop up-created index %q", index)
			}
		}

		assertPhase15Order(t, down,
			"drop table if exists identity_mail_outbox",
			"drop constraint if exists team_invites_team_id_id_unique",
			"mail outbox must disappear before its supporting invite identity uniqueness is removed",
		)
		assertPhase15Order(t, down,
			"drop table if exists identity_mail_outbox",
			"drop column if exists idempotency_key",
			"mail outbox must be removed before phase 15.1c columns are rolled back",
		)
		assertPhase15Order(t, down,
			"drop column if exists idempotency_key",
			"add constraint team_memberships_user_id_fkey foreign key ( user_id ) references users ( id ) on delete cascade",
			"down must remove phase 15.1c columns before restoring the original memberships fk action",
		)
	})
}
