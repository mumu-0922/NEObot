package migration

import (
	"strings"
	"testing"
)

func TestModelToolCapabilityCacheSchemaIsBoundedAndReversible(t *testing.T) {
	up := readPhase15SQL(t, "042_model_tool_capability_cache.up.sql")
	down := readPhase15SQL(t, "042_model_tool_capability_cache.down.sql")
	assertPhase15Fragments(t, up,
		"042 cache must bind capability to provider config and model",
		"create table model_tool_capability_cache",
		"primary key ( provider_config_hash , model_id )",
		"provider_config_hash ~ '^[0-9a-f]{64}$'",
		"status in ( 'supported' , 'unsupported' , 'unknown' )",
		"char_length ( category ) <= 64",
		"expires_at > checked_at",
		"create index idx_model_tool_capability_expiry",
		"grant select , insert , update , delete",
		"to go_api_runtime",
	)
	assertPhase15Fragments(t, down,
		"042 rollback must revoke access before dropping the cache",
		"revoke select , insert , update , delete",
		"from go_api_runtime",
		"drop table model_tool_capability_cache",
	)
	assertPhase15Order(t, down,
		"revoke select , insert , update , delete",
		"drop table model_tool_capability_cache",
		"042 rollback must revoke before drop",
	)
	for _, forbidden := range []string{
		"query_text", "prompt", "catalog", "payload", "response", "api_key", "secret",
	} {
		if strings.Contains(up, forbidden) {
			t.Fatalf("042 cache persists forbidden field marker %q", forbidden)
		}
	}
}
