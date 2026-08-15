# Hermes local Skill runtime comparison

## Upstream facts checked on 2026-08-16

- Hermes Agent's README describes seven terminal backends: `local`, Docker,
  SSH, Singularity, Modal, Daytona and Vercel Sandbox. Isolation is optional;
  the local backend executes with the Hermes process user's host authority.
- The upstream Skills documentation describes Skills as progressive-disclosure
  instruction directories under `~/.hermes/skills/`: `SKILL.md` is required
  and `scripts/`, `references/` and `assets/` are optional. The agent loads a
  Skill on demand, then uses its normal tools (including `terminal`) to follow
  it. A Skill does not need its own container or runtime manifest.
- The upstream security documentation keeps a hard command blocklist and a
  configurable dangerous-command approval layer for host-reaching backends.
  It explicitly states these guards reduce accidental damage but are not a
  sandbox against an adversarial process.

Sources:

- <https://github.com/NousResearch/hermes-agent>
- <https://hermes-agent.nousresearch.com/docs/user-guide/features/skills>
- <https://hermes-agent.nousresearch.com/docs/user-guide/security>

## Neo Chat fit

- Neo Chat already stores validated Agent Skills packages and owner-bound
  installations through `internal/skillsupply`, but Chat does not currently
  discover or load them.
- The Chat native Tool loop already supports server-owned Tools and multi-round
  continuation. A local Skill runtime can join this loop without inventing a
  second model protocol.
- The single-server backend already runs as the configured ordinary UID/GID.
  Direct execution can therefore run as that same user inside the existing
  application environment; it does not need Podman, rootless user namespaces,
  WSL systemd, `sudo`, mTLS Runner certificates or a second per-Skill Sandbox.
- The existing Backend image is intentionally minimal. A useful local backend
  needs common interpreters and a host workspace bind mount. Child process
  environment must remain explicit so database/provider credentials are not
  copied into ordinary command environment variables by default.

## Chosen direction

Implement a Hermes-style `local` backend as the single-server default:

1. Inject a compact index of the current user's installed Skills into Chat.
2. Expose `skills_list` and `skill_view` for progressive disclosure.
3. Materialize the already-validated immutable package on demand.
4. Expose a bounded `terminal` Tool that runs directly as the Backend user in
   the configured workspace, with no per-Skill container.
5. Retain a non-overridable catastrophic-command blocklist and default smart
   denial for destructive commands. These are guardrails, not isolation.
6. Retire the WSL2 Podman local-test activation path from the normal setup.

The former OCI Runner implementation remains historical/optional code during
this change so the pivot is reversible; it is not a startup or enablement
dependency for local Skills.
