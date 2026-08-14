# G21.1 canary plan and cleanup research

## Canary plan

The mounted plan must be strict and immutable. It binds a pre-provisioned
synthetic user, exact package/runtime/image/Workspace/seccomp fingerprints,
empty Tool Registry, non-root UID/GID, bounded resources and one fixed argv.
It carries no prompt, Workspace body, Secret reference, credential, Egress
origin or mutable Tool.

The request validator already requires `networkMode=none`, read-only rootfs,
no-new-privileges and an empty capability set. G21.1 additionally requires an
empty Tool list and a canary-only step kind.

## Cleanup and restart

- Cancellation must reap the exact container/process tree and Scratch before
  reporting success.
- Durable Runner projection terminalization and append-only Run terminal
  events remain evidence; they are not immediately pruned.
- Remote post-cancel inventory must contain no canary Sandbox.
- If the worker restarts after lease acquisition, it cannot recover the opaque
  lease token. It must wait for the lease to expire, reconcile the stale
  Sandbox and acquire a new generation. It must never persist or reconstruct
  the token.
- If a restart sees the idempotent Run already terminal, it verifies empty
  remote inventory and enters monitor mode without launching a second Run.

## Out-of-scope effects

The canary has no Broker relay, Artifact publication, Project mutation,
Provider call, Secret handle, Child, Cron, Learning or public/API entrypoint.
