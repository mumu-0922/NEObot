# Operations Development Guidelines

> Executable repository, CI, release, and runtime-state contracts.

## Guidelines index

| Guide                                                       | Scope                                                                                                     |
| ----------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| [Repository root boundary](./repository-root-boundary.md)   | The thin Git root, `mm-chat/` product root, automation paths, runtime protection, and verification gates. |
| [Dependency security](./dependency-security.md)             | Lockfile remediation, official-registry audits, override compatibility, and release verification.         |
| [Runtime recreate image pinning](./runtime-recreate-image-pinning.md) | Immutable image selection, schema compatibility, and rollback requirements for live Compose recreation. |
| [MCP Runner](./mcp-runner.md)                         | MCP manifest, dedicated Runner image/token/topology, release, backup/restore, retention, and rollback. |
| [Agent Runtime](./agent-runtime.md) | Non-root `neo-runnerd`, rootless OCI, broker/Child/Cron/Draft-learning held boundaries, Kill Switches, backup/restore, release and cutover. |
| [Session auto-commit](./session-auto-commit.md)             | Exact journal/index staging, commit isolation, ignored paths, and regression tests.                       |
| [Trellis scaffold boundary](./trellis-scaffold-boundary.md) | Shared Trellis/Codex scaffold, local state exclusions, explicit staging, and fresh-clone verification.    |

## Pre-development checklist

For repository layout, GitHub automation, build entrypoint, or cleanup changes:

1. Read
   [`repository-root-boundary.md`](./repository-root-boundary.md).
2. Confirm `mm-chat/` remains the only product root.
3. Identify protected runtime paths before changing files.
4. Define backup, rollback, and clean-copy verification before deletion.

For dependency or lockfile changes, read
[`dependency-security.md`](./dependency-security.md) and preserve the frozen
install plus component/full verification gates.

Before recreating a live Compose service, read
[`runtime-recreate-image-pinning.md`](./runtime-recreate-image-pinning.md).
Never assume a mutable local tag still names the image used by the running
container.

For Trellis session recording or Git auto-commit changes, read
[`session-auto-commit.md`](./session-auto-commit.md). Preserve the caller-owned
path boundary and pre-existing staged state.

For Trellis initialization, update, platform-hook, or scaffold-tracking
changes, read
[`trellis-scaffold-boundary.md`](./trellis-scaffold-boundary.md). Classify
every generated path before staging and preserve machine-local state.

For MCP environment, manifest, Runner image/service, preflight, backup/restore,
or release changes, read [`mcp-runner.md`](./mcp-runner.md). Preserve the
dedicated immutable image, independent secret, no-host-port network boundary,
paired Postgres/MinIO backup, and non-destructive rollback.

For Agent Runtime host/runtime, `neo-runnerd`, OCI isolation, Runner mTLS,
Workspace/Scratch/Artifact, Egress/Secret Broker, Kill Switch, backup/restore,
or legacy Skill cutover changes, read
[`agent-runtime.md`](./agent-runtime.md). Require the exact non-root account
capability probe and fingerprint-bound Isolation Acceptance evidence; never
fall back to rootful Docker, privilege, host sockets/mounts/network, or secrets
in Sandbox environment.

## Quality check

- Run `bash mm-chat/scripts/verify-standalone.sh --full`.
- Run the frontend, backend, and RAG component checks.
- Render Compose with both the example and active environment files.
- Prove `mm-chat/` tracked source and protected runtime paths were not changed
  by a root-only cleanup.
- Validate GitHub Actions syntax and live health when deployment entrypoints
  change.
- For Agent Runtime Phase 0, run
  `bash mm-chat/scripts/verify-agent-runtime-phase0.sh`. A production Runtime
  release additionally requires the target-host Isolation Acceptance Suite,
  Kill/reap/restart, Secret/network/filesystem negative proofs, paired backup/
  restore, and the owning G20 promotion gate.

**Language**: All documentation should be written in English.
