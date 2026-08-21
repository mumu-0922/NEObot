# Operations Development Guidelines

> Executable repository, CI, release, and runtime-state contracts.

## Guidelines index

| Guide                                                       | Scope                                                                                                     |
| ----------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| [Repository root boundary](./repository-root-boundary.md)   | The thin Git root, `mm-chat/` product root, automation paths, runtime protection, and verification gates. |
| [Dependency security](./dependency-security.md)             | Lockfile remediation, official-registry audits, override compatibility, and release verification.         |
| [Runtime recreate image pinning](./runtime-recreate-image-pinning.md) | Immutable image selection, schema compatibility, and rollback requirements for live Compose recreation. |
| [MCP Runner](./mcp-runner.md)                         | MCP manifest, dedicated Runner image/token/topology, release, backup/restore, retention, and rollback. |
| [Agent Runtime](./agent-runtime.md) | Current no-sudo `local_direct` Backend/Compose wiring plus the dark non-systemd WSL Agent Host lifecycle and migration-`098` legacy retirement boundary. |
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

For current `local_direct` Skill configuration, Backend image, Compose mounts,
workspace or rollback changes, read [`agent-runtime.md`](./agent-runtime.md).
Keep the ordinary runtime UID/GID, explicit mounts and environment, no-sudo
setup, non-isolation warning, process limits and switch-off rollback. Never
mount a container socket or bind unrelated secret/runtime paths.

## Quality check

- Run `bash mm-chat/scripts/verify-standalone.sh --full`.
- Run the frontend, backend, and RAG component checks.
- Render Compose with both the example and active environment files.
- Prove `mm-chat/` tracked source and protected runtime paths were not changed
  by a root-only cleanup.
- Validate GitHub Actions syntax and live health when deployment entrypoints
  change.
- For Chat Agent Runtime, run
  `bash mm-chat/scripts/verify-agent-local-runtime.sh` and the migration-`098`
  PostgreSQL 17 cleanup drill. Verify the two Backend mounts, ordinary UID/GID,
  process bounds, paired backup/restore rollback, and absence of retired
  services/networks.

**Language**: All documentation should be written in English.
