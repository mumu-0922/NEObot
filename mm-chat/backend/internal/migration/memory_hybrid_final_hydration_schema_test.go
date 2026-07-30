package migration

import (
	"strings"
	"testing"
)

func TestMemoryHybridFinalHydrationMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "065_memory_hybrid_final_hydration.up.sql")
	down := readPhase15SQL(t, "065_memory_hybrid_final_hydration.down.sql")
	assertPhase15Fragments(t, up,
		"065 must hydrate only a completed current hybrid final set",
		"create function memory_hydrate_hybrid_final",
		"security definer",
		"observation.status = 'completed'",
		"observation.result_code = 'ok'",
		"observation.rerank_status = 'applied'",
		"result.lane = 'final'",
		"v_returned <> v_expected",
		"memory_hybrid_final_hydration_authority_stale")
	assertPhase15Fragments(t, up,
		"065 must repeat source, policy, projection, revision, scope, and sensitivity authority",
		"source.status = 'completed'",
		"v_observation.query_sha256",
		"state.active_projection_generation = v_observation.projection_generation",
		"settings.enabled",
		"settings.search_enabled",
		"memory.revision = projection.memory_revision",
		"memory.content_hash = projection.content_hash",
		"memory.visibility_epoch = projection.visibility_epoch",
		"memory.scope_generation = projection.scope_generation",
		"memory.sensitivity = 'normal' or settings.sensitive_memory_enabled",
		"project.lifecycle_status = 'active'",
		"scoped_conversation.memory_scope_generation = projection.scope_generation")
	assertPhase15Fragments(t, up,
		"065 must expose only the narrow API capability",
		"revoke all on function memory_hydrate_hybrid_final",
		"from public",
		"grant execute on function memory_hydrate_hybrid_final",
		"to go_api_runtime")
	for _, forbidden := range []string{
		"p_memory_id", "p_memory_ids", "query_text", "rerank_score", "raw_score",
		"insert into message_memory_hybrid_shadow", "update message_memory_hybrid_shadow",
	} {
		if strings.Contains(up, forbidden) {
			t.Fatalf("065 exposes or persists forbidden hydration input %q", forbidden)
		}
	}
	assertPhase15Fragments(t, down,
		"065 rollback must remove only the additive capability",
		"revoke all on function memory_hydrate_hybrid_final",
		"from go_api_runtime",
		"drop function memory_hydrate_hybrid_final")
}
