# Current Delegation Seams

## Evidence inspected

- `internal/agentorchestrator` and migration `084` own generic sanitized
  snapshots, Run/Step/Attempt state, leases, events and Kill Switches.
- `internal/agentrunner` and migration `085` own replay-fenced Runner authority;
  launch already rejects depth outside `0..1` and forbidden depth-1 Tool names.
- `internal/agentbroker` and migration `086` own canonical Grants, Tool Registry,
  effects and budget call counters. Registry construction already removes the
  five forbidden Tool identities for depth 1.
- The current Orchestrator snapshot is deliberately generic and contains no
  typed delegation authority; no production Agent route or worker imports these
  packages.

## Reuse decision

- Reuse Broker Grant/Registry types and fingerprint builder rather than create a
  parallel authorization format.
- Reuse Orchestrator enqueue/event/lease SQL functions inside migration `087`;
  do not copy the state machine.
- Extend Runner launch lineage projection and validation; do not give Runner a
  database or Parent-authority credential.
- Use a new `agentdelegation` package to coordinate Broker and Orchestrator
  concepts without introducing an `agentorchestrator -> agentbroker` test import
  cycle.

## Held boundaries

- Registration/delegation/launch admission are internal APIs only.
- Production worker, mTLS control routing and host execution remain absent.
- Exact-host evidence is still separately required.
