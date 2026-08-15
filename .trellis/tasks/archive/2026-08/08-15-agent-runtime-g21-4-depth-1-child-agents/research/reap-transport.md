# Durable Child reap transport research

## Proven foundation

Migration `087` atomically terminalizes live Child Attempts/Steps/Runs,
creates durable reap facts and retries failed reaps. The Go service deliberately
injects a `Reaper`, because G20.5 did not wire a production Runner transport.

## Missing production fact

`agent_delegation_reaps` stores Run, Attempt, generation and lease owner, but
not the Step or Runner Sandbox projection. After cascade terminalizes the
Attempt, `agent_runner_recovery_sandboxes()` no longer returns it because that
function exposes only live Attempts. A restart therefore cannot reliably map a
pending reap to the exact Runner Sandbox. There is also a late-launch window:
an already signed launch ticket can arrive after cascade until its maximum
15-second authority TTL expires.

## Recommended migration 093

Add a function-only bridge without granting runtime table DML:

- `agent_delegation_reap_inventory(integer)` returns each pending/failed reap,
  exact Step/Attempt identity, optional Runner Sandbox descriptor and the last
  launch-authority expiry.
- `agent_delegation_complete_reap` is replaced so successful completion also
  terminalizes the exact matching Runner Sandbox projection in the same
  transaction. Failed attempts retain retryable reap state.
- Only the NOLOGIN function owner receives the narrowly required Runner table
  access. `agent_delegation_control` receives EXECUTE only and keeps no direct
  Runner table access.
- Down is guarded until there are no pending/failed reaps or live depth-1
  Sandbox projections, then restores the migration-087 function.

## Reaper algorithm

1. Load the durable inventory target.
2. Wait until every matching launch authority has expired.
3. List Runner inventory and exclude the exact Child Attempt while retaining
   the live Parent and unrelated expected Sandboxes.
4. Reconcile, then list again and prove the Child is absent.
5. Complete the durable reap and Runner projection atomically.

Crash after Runner reap but before database completion is replay-safe: restart
observes no Child Sandbox and repeats only completion. No new Child is enqueued.

