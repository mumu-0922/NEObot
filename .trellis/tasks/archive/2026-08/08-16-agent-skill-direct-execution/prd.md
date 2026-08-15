# Agent Skill direct local execution

## Goal

Replace the mandatory isolated Agent Runner activation path with a Hermes-style
local Skill backend. Installed Agent Skills must become usable from ordinary
Chat without `sudo`, Podman, WSL systemd, a machine restart or a per-Skill
container: the model discovers a Skill, loads its files on demand and executes
commands directly as the existing Backend user in the configured local
workspace.

## Requirements

- Treat `SKILL.md` plus optional `scripts/`, `references/` and `assets/` as the
  executable Skill surface. `neo.runtime.json` and an OCI image are not required
  for a normal installed Skill to be usable by Chat.
- Add progressive disclosure to the native Chat Tool loop:
  - `skills_list` returns bounded metadata for the authenticated user's current
    installations;
  - `skill_view` returns `SKILL.md` or one validated relative package file;
  - `terminal` executes a bounded shell command directly as the Backend process
    user and returns bounded stdout/stderr/exit status.
- Inject only a compact installed-Skill index into the system prompt. Full
  instructions and reference files load only through `skill_view`.
- Materialize only an owner-installed, admitted, content-addressed package.
  Revalidate archive bytes and package fingerprint before publishing an atomic
  immutable local directory. Never trust a caller-supplied object key or path.
- Run terminal commands in the configured local workspace, without Podman,
  rootless namespaces, a second daemon, mTLS, OCI images or Sandbox evidence.
- Use an explicit child environment (`PATH`, locale, `HOME`, workspace and Skill
  roots) rather than copying Backend database/provider/storage credentials into
  normal child environment variables.
- Bound calls, rounds, command duration and combined output. Cancellation must
  terminate the command process group and no command may outlive its Chat Run.
- Keep a non-overridable catastrophic-command blocklist and default smart denial
  for clearly destructive commands. These are accidental-damage guardrails,
  not an isolation claim. The Runtime status and docs must say `local_direct`.
- Enable the direct local backend in the single-server Compose baseline with an
  ordinary UID/GID and a configurable host workspace bind. It must require no
  `sudo`; the existing application container is the execution environment.
- Add the common local Skill toolchain to the Backend runtime image (`bash`,
  Python, Node/npm, Git, curl, jq, ripgrep and archive/file utilities).
- Agent Center must show local direct execution as ready instead of displaying
  `ISOLATION_UNAVAILABLE` when this backend is enabled.
- Remove the WSL2 Podman local-test bootstrap/smoke bundle added solely to make
  isolation mandatory. Preserve protected runtime data and the live env file.
- Keep the old OCI Runner source as disabled optional/history during this pivot;
  it must not gate local Skill discovery or execution.

## Acceptance Criteria

- [x] An installed instruction-only Agent Skill is visible to a tool-capable
      Chat model without a Neo runtime manifest or OCI image.
- [x] `skill_view` reads only files from the exact installed package and rejects
      traversal, missing files, oversized content and fingerprint drift.
- [x] `terminal` runs a safe fixture command in the configured workspace as the
      current non-root Backend user and returns exact bounded result framing.
- [x] Timeout/cancellation kills the full process group; output overflow is
      truncated deterministically; concurrent/call/round budgets fail closed.
- [x] Child commands do not inherit database/provider/storage secret variables.
- [x] Catastrophic and destructive command fixtures are blocked before process
      creation, while normal shell/Python/Node commands execute.
- [x] Chat Tool continuation can perform `skills_list -> skill_view -> terminal
      -> final answer` and emits sanitized Tool process events.
- [x] Single-server configuration enables `local_direct` without Podman, sudo,
      WSL restart, Runner certificates or production isolation evidence.
- [x] Agent Center renders the direct local Runtime as ready and retains an
      explicit warning that commands use the local workspace authority.
- [x] Focused backend/frontend tests, lint/typecheck/vet, existing Skill supply
      gates and full standalone verification pass.

## Definition of Done

- Source, tests, Docker/Compose/example config, user deployment docs, Runtime
  contract and Trellis specs agree on the local-direct model.
- The obsolete WSL Podman local-test setup is removed without touching
  `mm-chat/.env.single-server`, `mm-chat/data/`, `mm-chat/secrets/` or
  `mm-chat/backup/`.
- Work, task archive and journal changes are committed without push or amend.

## Technical Approach

Extend `skillsupply.Service` with a read-only Runtime catalog that resolves only
owner installations, fetches the canonical package object, revalidates it and
atomically materializes immutable package files. Add a small `localskills`
executor for direct commands and adapt it into the existing native Chat Tool
loop alongside MCP/Knowledge/Memory/Web. Add a validated `AgentLocal` config
block, single-server workspace mount and direct-ready Agent Center status.

## Decision (ADR-lite)

**Context:** The original baseline made Rootless OCI isolation a hard launch
gate, which introduced Podman, WSL systemd, `sudo`, restart and promotion
evidence before any useful Skill could run. Hermes supports a simpler local
backend in which Skills are instructions and scripts use ordinary Agent tools.

**Decision:** Make `local_direct` the single-server Skill execution backend and
remove isolation from the required path. Retain bounded execution and approval
guardrails, but do not describe them as a Sandbox.

**Consequences:** Skills become usable on the current machine with no extra
Runner setup. Commands have the authority of the Backend user and configured
workspace; a malicious allowed command is therefore not contained by a
per-Skill security boundary. Operators can disable the feature with one env
switch.

## Out of Scope

- Claiming that local direct execution protects against an adversarial Skill.
- Running as root, copying the Docker socket, or automatically installing host
  OS packages at Chat time.
- Enabling Cron, self-learning promotion, nested Child Agents or the former OCI
  production promotion chain in this task.
- Deleting historical G20/G21 migrations or audit authority.

## Research References

- [`research/hermes-local-runtime.md`](research/hermes-local-runtime.md)

## Technical Notes

- User explicitly replaced the earlier isolation decision and authorized direct
  implementation/commit without additional confirmation.
- No Sub-agent delegation is allowed for this task.
