# Agent Runner

`agentrunner` is the private G20.3 host-control boundary for `neo-runnerd`.
It is intentionally not imported by the public API or Chat startup path.

The package provides strict `neo.runner-rpc/v1` request decoding, TLS 1.3 mutual
authentication, Ed25519 authority tickets, a mode-0600 fsync replay ledger,
release-bound host probing, rootless Podman lifecycle validation, immutable
Workspace materialization, bounded tmpfs Scratch and credential-free Artifact
quarantine through a per-Attempt Unix socket.

Production execution is not enabled by this package. A launch still requires an
approved exact release manifest and a ready exact-host probe. On the current
development host, `verify-agent-runner-host.sh` must return
`ISOLATION_UNAVAILABLE`.

`RunLocalTestSmoke` is the only exception to the source-only host baseline. It
runs one synthetic no-network/no-secret workload through the real driver,
inspection, Artifact and reap paths after the separate WSL host manager proves
its pinned local toolchain. It uses ephemeral authority and a StaticProbe,
emits only `neo.agent-runner-local-test-report/v1`, and cannot produce an
approved release, production activation or promotion fact.

## Verification

```bash
bash mm-chat/scripts/verify-agent-runner.sh
bash mm-chat/scripts/verify-agent-runner-postgres17.sh
bash mm-chat/scripts/verify-agent-runner-host.sh # expected nonzero here
bash mm-chat/scripts/verify-agent-runner-local-test.sh
```

See [`DESIGN.md`](./DESIGN.md) and the repository Agent Runtime contracts for
the trust and rollback boundaries.
