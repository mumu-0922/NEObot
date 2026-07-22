package migration

import (
	"strings"
	"testing"
)

func TestRAGSourceGatewayGoAPIGrantMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "040_rag_source_gateway_go_api_grant.up.sql")
	down := readPhase15SQL(t, "040_rag_source_gateway_go_api_grant.down.sql")

	assertPhase15Fragments(t, up,
		"040 must grant only the hardened source metadata gateway",
		"quote_ident ( current_schema ( ) ) || ' , pg_catalog , pg_temp'",
		"grant execute on function knowledge_fetch_parse_source_metadata",
		"uuid , uuid , uuid , uuid , uuid",
		"to go_api_runtime",
	)
	assertPhase15Fragments(t, down,
		"040 down must revoke only the source metadata gateway",
		"revoke execute on function knowledge_fetch_parse_source_metadata",
		"from go_api_runtime",
	)

	for _, sql := range []string{up, down} {
		for _, forbidden := range []string{
			"grant select",
			"grant insert",
			"grant update",
			"grant delete",
			"drop table",
			"drop function",
		} {
			if strings.Contains(sql, forbidden) {
				t.Fatalf("040 crossed its function-grant boundary: found %q", forbidden)
			}
		}
	}
}
