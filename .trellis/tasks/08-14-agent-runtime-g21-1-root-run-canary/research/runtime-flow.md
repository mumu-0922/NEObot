# G21.1 runtime flow research

## Existing seams

- `internal/agentorchestrator` already provides immutable enqueue snapshots,
  Step leasing, lease generations, heartbeat, append-only transitions and
  recovery inventory through migration `084`.
- `internal/agentrunner` already provides PostgreSQL-issued Ed25519 launch
  authority, mTLS Runner RPC, launch/heartbeat/cancel, expected-Sandbox state
  and recovery inventory through migration `085`.
- `agent_runner_control` can issue/complete authority and maintain expected
  Sandbox projections. `agent_orchestrator_runtime` can enqueue, lease,
  heartbeat and transition Runs without direct table DML.
- A dedicated LOGIN inheriting exactly those two NOLOGIN roles is sufficient;
  a new migration is unnecessary for the canary worker.

## Decisive gaps

- No production process composes the Orchestrator and Runner seams.
- The current Runner accepts only the G21.0 control mTLS identity and has no
  per-identity method policy. Adding the canary identity must not let the
  control identity call launch/heartbeat/cancel.
- Runner authority creation needs request ID/nonce before signing, while the
  current request helper does not expose a safe way to bind a ticket back to
  the same envelope.
- Separate Attempt, Step and Run terminal calls create an unrecoverable crash
  window after the opaque lease token is lost. The canary terminal append and
  expected-Sandbox transition should therefore execute in one database
  transaction using the existing SECURITY DEFINER functions.

## Recommended flow

1. Revalidate a dedicated `root_run_canary` activation record.
2. Probe the exact Runner and reconcile stale inventory before claiming work.
3. Idempotently enqueue one synthetic, content-free Root Run.
4. Acquire its only Step and transition the Attempt to `starting`.
5. Build the exact no-network/no-Secret/no-Tool launch request, bind a freshly
   issued signed authority ticket to the same request ID/nonce and launch.
6. Persist expected/running Sandbox state and transition the Attempt to
   `running`.
7. Exercise Runner and PostgreSQL heartbeats.
8. Issue signed cooperative cancellation, then atomically append terminal
   Attempt/Step/Run events and terminalize the expected Sandbox projection.
9. Prove the Runner inventory is empty. A restart replays the idempotent Run;
   live tokenless Attempts wait for expiry, reconcile and reclaim rather than
   inventing authority.
