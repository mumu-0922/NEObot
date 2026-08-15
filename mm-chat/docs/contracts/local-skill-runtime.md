# Local Skill Runtime Contract

## Scope

`local_direct` is the current single-server Agent Skill execution backend. It
follows the Hermes Agent model: an installed Skill is an instruction directory,
and the normal Chat Tool Loop loads its files and runs commands when needed.

```text
owner installation -> admitted canonical package -> immutable local materialization
  -> compact Skill index -> skills_list -> skill_view -> terminal -> same-model answer
```

An ordinary Skill needs `SKILL.md`. `scripts/`, `references/`, and `assets/`
are optional. `neo.runtime.json`, OCI images, Podman, WSL systemd, `sudo`,
Runner mTLS, and production isolation evidence are not prerequisites.

## Package authority

- Only the authenticated owner's current installation is eligible.
- The server resolves the admitted package and canonical object key; callers
  cannot supply an object key or materialization path.
- Canonical ZIP size, package name, installation fingerprint, archive
  validation, and content fingerprint are rechecked before publication.
- Publication uses a same-filesystem temporary directory and atomic rename.
- Reads accept only exact inventory paths and reject absolute paths, traversal,
  missing files, symlinks, oversized files, and content drift.
- A `terminal.skill` binding rehashes the complete exact inventory again before
  exposing the selected root to the child command.
- Chat may view only `SKILL.md` or exact files below `scripts/`, `references/`,
  and `assets/`.

## Native Tools

| Tool | Contract |
| --- | --- |
| `skills_list({})` | Returns bounded name, version, description, and file-count metadata for current installations. |
| `skill_view({name,path?})` | Defaults to `SKILL.md`; returns UTF-8 or base64 content from one exact installed package file. |
| `terminal({command,skill?,workingDir?,timeoutSeconds?})` | Runs one bounded shell command as the Backend user in the configured workspace. `skill` exposes that installed package only through `NEO_CHAT_ACTIVE_SKILL_ROOT`. |

The system prompt contains only a bounded installed-Skill index. Full Skill
instructions and files enter model context only through `skill_view`. Every
Tool result is marked as untrusted and cannot override system/developer
instructions.

## Direct execution and limits

`terminal` is direct local execution, not a Sandbox. A permitted command has
the filesystem and network authority of the Backend process and configured
workspace. Agent Center and deployment docs must state this explicitly.

The executor:

- supplies an explicit child environment rather than inheriting database,
  Provider, storage, or secret environment variables;
- uses a non-login shell, so writable workspace profile files are not executed
  implicitly, and exposes only the selected Skill root rather than a global
  package-cache variable;
- confines the command working directory to the configured workspace,
  including symlink resolution;
- enforces per-call timeout, per-Run timeout, combined stdout/stderr bytes,
  calls, rounds, and global concurrency;
- starts a process group and kills the complete group on timeout or Chat Run
  cancellation;
- always blocks catastrophic command patterns and, in default `smart` mode,
  returns `approval_required` for destructive patterns;
- records only Tool name, round, `local_direct`, classification, optional
  timeout, duration, and failure category in process events. Command text and
  output are never process-trace or SSE metadata.

These guards reduce accidental damage. They are not protection against an
adversarial allowed process.

## Failure and rollback

- A model without native Tool support fails before assistant creation with
  `SKILL_MODEL_UNSUPPORTED`.
- Package preparation failure returns `SKILL_RUNTIME_UNAVAILABLE` without
  exposing storage or filesystem details.
- Run deadline returns `LOCAL_SKILL_BUDGET_EXHAUSTED`; cancellation terminates
  the process group and finalizes the Chat Run as cancelled.
- Set `AGENT_LOCAL_RUNTIME_ENABLED=false` and recreate the Backend to roll back
  immediately. Installed packages remain stored and no OCI Runtime is enabled.
