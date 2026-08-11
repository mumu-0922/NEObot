package migration

import (
	"strings"
	"testing"
)

func TestMemoryWorkerHealthResolutionMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "071_memory_worker_health_resolutions.up.sql")
	down := readPhase15SQL(t, "071_memory_worker_health_resolutions.down.sql")
	resolutionDDL := phase15TableDDL(t, up, "memory_job_health_resolutions")
	assertPhase15Fragments(t, resolutionDDL,
		"071 resolution evidence must be bounded, owner-bound, and content-free",
		"job_id uuid primary key",
		"user_id uuid not null",
		"error_code text not null",
		"resolution_code text not null",
		"resolved_by_user_id uuid not null",
		"resolved_by_user_id = user_id",
		"source_no_longer_current",
		"historical_failure_accepted",
		"references memory_jobs ( job_id , user_id ) on delete restrict")
	for _, forbidden := range []string{
		"content", "query", "provider", "secret", "raw_error", "note",
	} {
		if strings.Contains(resolutionDDL, forbidden) {
			t.Fatalf("071 resolution persists forbidden field %q", forbidden)
		}
	}

	assertPhase15Fragments(t, up,
		"071 must exclude maintenance, preserve unresolved failures, and expose only a narrow acknowledgement",
		"memory_worker_health_resolution_requires_070",
		"add constraint memory_jobs_job_id_user_unique unique ( job_id , user_id )",
		"create function memory_acknowledge_job_health",
		"job.stage = 'extract'",
		"not exists ( select 1 from memory_job_health_resolutions",
		"memory_job_health_resolution_append_only",
		"memory_job_health_resolution_not_historical",
		"memory_job_health_resolution_source_current",
		"or not exists ( select 1 from conversations conversation",
		"conversation.deleted_at is not null",
		"v_job.error_code <> p_expected_error_code",
		"clock_timestamp ( ) - interval '24 hours'",
		"revoke all on memory_job_health_resolutions from public , go_api_runtime , memory_worker_runtime",
		"to go_api_runtime")
	assertPhase15Fragments(t, down,
		"071 rollback must preserve acknowledgements and restore exact 070 aggregation",
		"lock table memory_job_health_resolutions in access exclusive mode",
		"memory_job_health_resolution_rollback_requires_empty",
		"where job.user_id = p_user_id",
		"drop function memory_acknowledge_job_health",
		"drop trigger memory_job_health_resolutions_append_only",
		"drop table memory_job_health_resolutions",
		"drop constraint memory_jobs_job_id_user_unique")
	if strings.Contains(normalizePhase15SQL(down), "job.stage = 'extract'") ||
		strings.Contains(normalizePhase15SQL(down), "memory_job_health_resolutions resolution") {
		t.Fatal("071 down did not restore the 070 all-job capture aggregation")
	}
}
