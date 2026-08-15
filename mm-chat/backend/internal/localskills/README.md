# `localskills`

`localskills` is the Hermes-style local terminal backend for installed Agent
Skills. It runs bounded commands directly as the existing Backend user in the
configured workspace. It requires no Podman, WSL systemd, `sudo`, Runner daemon,
mTLS certificate or OCI image.

The package provides accidental-damage guardrails, not a security Sandbox:

- an explicit child environment that omits Backend credential variables;
- a non-overridable catastrophic-command blocklist;
- `smart` destructive-command denial or explicit operator `off` mode;
- duration, output, concurrency, call and round limits;
- workspace-contained working directories after symlink resolution;
- non-login shell execution, so writable workspace profile files are not
  loaded implicitly;
- process-group termination on timeout or Chat cancellation.

Installed package discovery and immutable materialization remain owned by
`internal/skillsupply`. Chat adapts both packages into its native same-model
Tool continuation loop.

When Chat binds a specific installed Skill to `terminal.skill`, the executor
adds `NEO_CHAT_ACTIVE_SKILL_ROOT` to the otherwise explicit child environment.
It does not expose a global Skill-cache variable. Exact workspace/cache/active
paths are replaced with symbolic labels in captured output and never enter
process metadata.
