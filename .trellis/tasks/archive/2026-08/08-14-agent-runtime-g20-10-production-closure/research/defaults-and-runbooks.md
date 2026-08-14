# Conservative operations defaults

## Capacity and budgets

For the first single-server production cohort, use conservative defaults below
the already enforced Grant maxima:

- two concurrent root Runs and four total Sandboxes;
- queue depth 32 and one concurrent canary;
- root Run: 300 wall seconds, 20,000 model tokens, 32 Tool calls and 16 MiB
  published Artifact bytes;
- Child/Cron: 120 wall seconds, 8,000 model tokens, 8 Tool calls and 4 MiB
  Artifact bytes;
- no Egress or Secrets in the first canary; any widening creates a new policy
  revision and repeats the owning gate.

These are defaults, not a bypass around a narrower frozen Grant. Saturation
queues or denies new work; it never widens budgets or falls back to another
executor.

## Retention

- terminal Run/event/snapshot authority: 30 days after all Artifact and effect
  cleanup dependencies are resolved;
- completed Runner replay/Sandbox projection: 7 days after exact terminal
  cleanup;
- published canary Artifacts and rejected/promoted Draft quarantine: 7 days,
  object before row;
- Cron trigger history: 30 days; immutable Cron/Draft/Kill-Switch audit: 365
  days;
- content-free promotion records and incident facts: 400 days;
- unresolved `outcome_unknown`, object drift, failed reap or failed cleanup:
  retain until explicit operator resolution; never age-prune to make health
  green.

Pruning is bounded (100 rows per pass) and stays active while execution is off.

## Monitoring boundary

Metrics use bounded labels only: outcome, reason code, scope class, mode,
runtime class and operation. IDs, users, fingerprints, URLs, object keys,
arguments/results and content are forbidden as labels. Page immediately for
Runtime enabled with readiness false, Secret canary leakage, unresolved
`outcome_unknown`, Kill Switch enforcement failure or orphan residue after two
reconcile passes. Ticket sustained queue/capacity/cleanup pressure.

## Incident principle

First append the narrowest durable Kill Switch that safely stops new authority,
then preserve evidence, reconcile, repair forward, rerun exact acceptance and
append a new inactive switch revision. Never delete the incident record,
manually rewrite tables, retry an ambiguous Commit with a new key or enable a
rootful/browser/API fallback.
