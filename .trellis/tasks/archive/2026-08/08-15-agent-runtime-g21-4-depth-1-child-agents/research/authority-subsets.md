# Child authority subset research

## Existing enforcement layers

`agentdelegation.Service` and migration `087` already enforce:

- only depth `0 -> 1`;
- authenticated user separate from proposed Grant subject;
- same model, package and runtime bundle;
- narrower Grant capabilities, actions, selectors, approvals, Egress, Secrets,
  expiry and four-dimensional budget;
- exact Parent Attempt, generation, owner, lease-token digest, snapshot,
  Grant/Registry and Kill Switch authority;
- atomic Parent budget reservation and terminal settlement;
- literal prefix containment rather than SQL wildcard matching.

`agentbroker.BuildRegistry` removes `delegate_task`, `cron_manage`,
`grant_manage`, `secret_manage` and `runtime_manage` for depth one before
fingerprinting. Runner launch validation repeats the physical forbidden-Tool
check and rejects depth two.

## Recommended synthetic plan

- One depth-zero Parent with exactly the `delegate_task/create` capability.
- One depth-one Child using the same synthetic subject/model/package/runtime,
  a strict budget subset, no Egress and no Secrets.
- The Child requests `delegate_task`, but its derived Registry must be empty;
  this turns physical removal into an observable invariant without giving the
  Child any executable effect.
- Both Sandboxes use `networkMode=none`, read-only rootfs, empty capabilities
  and bounded resources. No Provider, MCP, Project, Artifact or Secret action
  runs.
- Unit/PostgreSQL/Runner gates attempt model/package/subject/Grant/Registry/
  budget widening, aliasing, stale Parent and depth two before launch.

## Future boundary

G21.4 proves control-plane depth and lifecycle only. Product cohort delegation
and package-selected child prompts remain reserved for G21.6.

