-- The repaired gateways and corrected Version failure states remain valid for
-- every earlier schema containing durable Knowledge Jobs. Rolling back the
-- ledger entry intentionally keeps the known-good behavior and backfill.
DO $rag_failure_state_projection_down$
BEGIN
  NULL;
END
$rag_failure_state_projection_down$;
