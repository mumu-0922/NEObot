package migration

import (
	"strings"
	"testing"
)

func TestPhase151DConsentExpiryMaterializationSchema(t *testing.T) {
	up := strings.ToLower(readPhase15SQL(t, "009_phase15_consent_expiry_materialization.up.sql"))
	down := strings.ToLower(readPhase15SQL(t, "009_phase15_consent_expiry_materialization.down.sql"))
	assertPhase15Fragments(t, up, "consent expiry must be a materialized time fact",
		"expiry_materialized_at timestamptz",
		"decision = 'granted'",
		"expiry_materialized_at >= expires_at",
		"idx_processing_consents_expiry_due",
	)
	assertPhase15Fragments(t, down, "consent expiry materialization must be reversible",
		"drop index if exists idx_processing_consents_expiry_due",
		"drop column if exists expiry_materialized_at",
	)
}
