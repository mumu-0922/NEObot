package migration

import (
	"context"
	"testing"
	"time"
)

func TestMemoryHybridRelevanceAdmissionLivePostgres(t *testing.T) {
	db := openMemoryLexicalMigrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	base := NewRunner(db, phase15MigrationFSThrough(t, 63))
	if _, err := base.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Up(ctx); err != nil {
		t.Fatalf("apply through 063: %v", err)
	}
	runner := NewRunner(db, phase15MigrationFSThrough(t, 64))
	if _, err := runner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	applied, err := runner.Up(ctx)
	if err != nil || len(applied) != 1 || applied[0].Version != 64 {
		t.Fatalf("apply 064 = %#v/%v", applied, err)
	}
	var apiExecute, workerExecute, publicExecute bool
	if err := db.QueryRowContext(ctx, `
SELECT
  has_function_privilege(
    'go_api_runtime',
    'memory_authorize_hybrid_rerank(uuid,uuid,uuid,text,real[])',
    'EXECUTE'
  ),
  has_function_privilege(
    'memory_worker_runtime',
    'memory_authorize_hybrid_rerank(uuid,uuid,uuid,text,real[])',
    'EXECUTE'
  ),
  EXISTS (
    SELECT 1
    FROM pg_proc function
    CROSS JOIN LATERAL aclexplode(
      coalesce(function.proacl, acldefault('f', function.proowner))
    ) privilege
    WHERE function.oid =
      'memory_authorize_hybrid_rerank(uuid,uuid,uuid,text,real[])'::regprocedure
      AND privilege.grantee = 0
      AND privilege.privilege_type = 'EXECUTE'
  )
`).Scan(&apiExecute, &workerExecute, &publicExecute); err != nil {
		t.Fatal(err)
	}
	if !apiExecute || workerExecute || publicExecute {
		t.Fatalf("064 privileges = api:%t worker:%t public:%t",
			apiExecute, workerExecute, publicExecute)
	}
	down, err := runner.Down(ctx, false)
	if err != nil || len(down) != 1 || down[0].Version != 64 {
		t.Fatalf("064 down = %#v/%v", down, err)
	}
	reapplied, err := runner.Up(ctx)
	if err != nil || len(reapplied) != 1 || reapplied[0].Version != 64 {
		t.Fatalf("064 re-up = %#v/%v", reapplied, err)
	}
}
