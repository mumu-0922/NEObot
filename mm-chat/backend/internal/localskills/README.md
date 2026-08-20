# `localskills`

`localskills` is the Hermes-style local workspace backend for Chat Agent Tools.
It reads and edits workspace files, runs bounded foreground/background commands
as the existing Backend user, and optionally binds admitted installed Skills.
It requires no Podman, WSL systemd, `sudo`, Runner daemon, mTLS certificate, or
OCI image.

The package provides accidental-damage guardrails, not a security Sandbox:

- an explicit child environment that omits Backend credential variables;
- a non-overridable catastrophic-command blocklist;
- `smart` destructive-command denial or explicit operator `off` mode;
- duration, output, concurrency, call and round limits;
- workspace-contained working directories after symlink resolution;
- non-login shell execution, so writable workspace profile files are not
  loaded implicitly;
- process-group termination on timeout or Chat cancellation.

The workspace API uses Go `os.Root` operations and always reduces input to one
workspace-relative path. An optional exact Host-root alias accepts canonical
Linux absolute and WSL UNC representations beneath the same mounted workspace;
it never adds a filesystem root. Other absolute paths, Windows drives,
symlink/traversal escapes, and malformed aliases are rejected. UTF-8 reads,
writes, searches, file counts, and result counts are bounded. Writes and
exact-text edits require a
complete-file `sha256:<hex>` version (`absent` only creates a new file), recheck
that version immediately before a same-directory atomic rename, and `fsync`
the file and parent directory.

`terminal` can reserve the same executor slot for a process-local background
Job. Jobs are scoped to the exact user and conversation, expose bounded output
only through `job_output`, support a blocking wait of at most ten seconds, and
are killed and reaped by `Executor.Close`. They deliberately do not survive a
Backend restart.

Installed package discovery and immutable materialization remain owned by
`internal/skillsupply`. Chat adapts both packages into its native same-model
Tool continuation loop.

When Chat binds a specific installed Skill to `terminal.skill`, the executor
adds `NEO_CHAT_ACTIVE_SKILL_ROOT` to the otherwise explicit child environment.
It does not expose a global Skill-cache variable. Exact workspace/cache/active
paths are replaced with symbolic labels in captured output and never enter
process metadata.
