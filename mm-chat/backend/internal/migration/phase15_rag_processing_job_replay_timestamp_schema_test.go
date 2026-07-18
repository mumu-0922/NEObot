package migration

import (
	"strings"
	"testing"
)

func TestProcessingJobReplayUsesOneTimestamp(t *testing.T) {
	up := readPhase15SQL(t, "030_rag_processing_job_replay_timestamp_fix.up.sql")
	down := readPhase15SQL(t, "030_rag_processing_job_replay_timestamp_fix.down.sql")
	for _, fragment := range []string{
		"replayed_at timestamptz := clock_timestamp ( )",
		"failed_job.max_attempts , replayed_at , null",
		"replayed_at , replayed_at , null",
		"p_operator_id , trim ( p_reason ) , replayed_at",
	} {
		if !strings.Contains(up, fragment) {
			t.Fatalf("030 migration missing %q", fragment)
		}
	}
	if strings.Contains(down, "replayed_at timestamptz :=") {
		t.Fatal("030 rollback must restore the prior multi-clock replay body")
	}
}
