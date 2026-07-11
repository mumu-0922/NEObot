package migration

import (
	"strings"
	"testing"
)

func TestPhase151DGovernanceProfilesAreImmutable(t *testing.T) {
	up := strings.ToLower(readPhase15SQL(t, "008_phase15_governance_immutability.up.sql"))
	down := strings.ToLower(readPhase15SQL(t, "008_phase15_governance_immutability.down.sql"))
	assertPhase15Fragments(t, up, "governance profiles must reject mutation",
		"create function reject_processor_governance_profile_mutation",
		"before update or delete on processor_governance_profiles",
		"raise exception 'processor governance profiles are immutable'",
	)
	assertPhase15Fragments(t, down, "governance immutability must be reversible",
		"drop trigger if exists processor_governance_profiles_immutable",
		"drop function if exists reject_processor_governance_profile_mutation",
	)
}
