# G21.3 Project CAS and crash recovery research

## Question

What authority can prove whether the exact mutable Project effect committed
after an acknowledgement loss?

## Existing repo facts

- `agentbroker.Service` claims Commit exactly once. A possible send calls
  executor `Status`; committed/rejected results terminalize exactly, while an
  unavailable or ambiguous status becomes terminal `outcome_unknown`.
- The existing `DeterministicCASFake` is memory-only and cannot survive restart.
- A multi-file filesystem patch cannot provide a simple atomic receipt across
  process death. A database transaction can atomically CAS content, append a
  stable receipt and expose exact status.

## Compared authorities

### A. PostgreSQL synthetic Project CAS authority — recommended

Migration `092` owns pre-provisioned synthetic canary resources, immutable
mutation receipts and cleanup facts. A function-only role can authorize, CAS,
query status and restore only the exact synthetic resource. The CAS transaction
rechecks the committing effect intent, approval, subject, Run/Attempt/
generation/lease, snapshot, Grant/Registry, Kill Switch, action/resource, base
revision, patch fingerprint and byte bound.

Atomic CAS plus receipt makes every crash class decidable:

- before CAS: no receipt and unchanged base revision means definitely not sent;
- after CAS but before response: status returns the committed receipt;
- status unavailable: Broker terminalizes `outcome_unknown` and never retries;
- exact Commit replay: migration `086` returns the original terminal receipt.

### B. Filesystem file plus sidecar ledger

A single-file rename is atomic, but crash ordering between target bytes and the
ledger needs filesystem-specific recovery and careful path-race handling. It is
useful once a real Project filesystem contract exists, but premature here.

### C. Reuse `agent_effect_receipts` as executor status

The effect receipt is written only after executor completion, so it cannot
answer whether the external action committed during acknowledgement loss. It is
not an independent status authority and is rejected.

## Data and cleanup boundary

- The target is synthetic and explicitly not a user Project store.
- Operator-only provisioning creates the exact baseline resource before the
  canary is enabled; the runtime role cannot create arbitrary resources.
- Commit permits one UTF-8 write, no delete, no traversal and a small exact byte
  bound. Raw synthetic content stays in the dedicated table and never appears
  in effect rows, events, logs or evidence.
- Cleanup restores the exact baseline only after a committed terminal receipt,
  appends a content-free cleanup fact and remains idempotent. The immutable
  mutation/status receipt survives cleanup for replay and incident authority.
- PostgreSQL backup/restore includes the synthetic resource, receipts and
  cleanup facts. Restore runs with all Agent canaries off and requires fresh
  activation afterward.
