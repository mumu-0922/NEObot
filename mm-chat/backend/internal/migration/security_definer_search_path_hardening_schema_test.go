package migration

import (
	"strings"
	"testing"
)

func TestSecurityDefinerSearchPathHardeningMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "041_security_definer_search_path_hardening.up.sql")
	down := readPhase15SQL(t, "041_security_definer_search_path_hardening.down.sql")

	for _, sql := range []string{up, down} {
		assertPhase15Fragments(t, sql,
			"041 must harden every current-schema SECURITY DEFINER function",
			"quote_ident ( current_schema ( ) ) || ' , pg_catalog , pg_temp'",
			"from pg_proc function",
			"join pg_namespace namespace",
			"namespace.nspname = schema_name",
			"function.prosecdef",
			"pg_get_function_identity_arguments ( function.oid )",
			"alter function %i.%i ( %s ) set search_path to %i , pg_catalog , pg_temp",
		)
		for _, forbidden := range []string{
			"reset search_path",
			"'$user , public'",
			"grant execute",
			"grant select",
			"drop function",
		} {
			if strings.Contains(sql, forbidden) {
				t.Fatalf("041 must not reopen or alter function authority: found %q", forbidden)
			}
		}
	}

	assertPhase15Fragments(t, up,
		"041 up must fail unless every function is hardened",
		"function.proconfig is distinct from array",
		"message = 'security_definer_search_path_hardening_incomplete'",
	)
}
