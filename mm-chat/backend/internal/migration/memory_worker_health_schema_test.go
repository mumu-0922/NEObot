package migration

import (
	"strings"
	"testing"
)

func TestMemoryWorkerHealthMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "070_memory_worker_health.up.sql")
	down := readPhase15SQL(t, "070_memory_worker_health.down.sql")
	heartbeatDDL := phase15TableDDL(t, up, "memory_worker_heartbeats")
	assertPhase15Fragments(t, heartbeatDDL,
		"070 heartbeat must remain content-free, expiring, and worker-scoped",
		"worker_id uuid primary key",
		"embedding_enabled boolean not null",
		"heartbeat_at timestamptz not null",
		"expires_at timestamptz not null",
		"expires_at > heartbeat_at")
	for _, forbidden := range []string{
		"content", "query", "provider", "secret", "model_id", "memory_id",
	} {
		if strings.Contains(heartbeatDDL, forbidden) {
			t.Fatalf("070 heartbeat persists forbidden field %q", forbidden)
		}
	}
	assertPhase15Fragments(t, up,
		"070 must expose narrow heartbeat and user health capabilities",
		"create function memory_worker_heartbeat",
		"create function memory_worker_retire",
		"create function memory_user_health",
		"security definer",
		"p_worker_id is null",
		"p_ttl_seconds is null",
		"p_embedding_enabled is null",
		"p_ttl_seconds < 5",
		"p_ttl_seconds > 120",
		"heartbeat.expires_at > now ( )",
		"where job.user_id = p_user_id",
		"where memory.user_id = p_user_id",
		"settings.enabled",
		"settings.search_enabled",
		"to memory_worker_runtime",
		"to go_api_runtime",
		"revoke all on memory_worker_heartbeats from public , go_api_runtime , memory_worker_runtime")
	assertPhase15Fragments(t, down,
		"070 rollback must reject active workers and restore the prior readiness contract",
		"memory_health_rollback_requires_stopped_workers",
		"drop function memory_user_health",
		"drop function memory_worker_retire",
		"drop function memory_worker_heartbeat",
		"select true",
		"drop table memory_worker_heartbeats")
}
