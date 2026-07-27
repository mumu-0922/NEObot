# Operations Development Guidelines

> Executable repository, CI, release, and runtime-state contracts.

## Guidelines index

| Guide                                                       | Scope                                                                                                     |
| ----------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| [Repository root boundary](./repository-root-boundary.md)   | The thin Git root, `mm-chat/` product root, automation paths, runtime protection, and verification gates. |
| [Dependency security](./dependency-security.md)             | Lockfile remediation, official-registry audits, override compatibility, and release verification.         |
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

For Trellis session recording or Git auto-commit changes, read
[`session-auto-commit.md`](./session-auto-commit.md). Preserve the caller-owned
path boundary and pre-existing staged state.

For Trellis initialization, update, platform-hook, or scaffold-tracking
changes, read
[`trellis-scaffold-boundary.md`](./trellis-scaffold-boundary.md). Classify
every generated path before staging and preserve machine-local state.

## Quality check

- Run `bash mm-chat/scripts/verify-standalone.sh --full`.
- Run the frontend, backend, and RAG component checks.
- Render Compose with both the example and active environment files.
- Prove `mm-chat/` tracked source and protected runtime paths were not changed
  by a root-only cleanup.
- Validate GitHub Actions syntax and live health when deployment entrypoints
  change.

**Language**: All documentation should be written in English.
