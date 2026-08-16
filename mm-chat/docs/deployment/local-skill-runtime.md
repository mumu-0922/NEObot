# Local Skill Runtime

The single-server stack enables Hermes-style `local_direct` Skill execution in
the existing Backend container. It uses the configured ordinary
`MM_CHAT_RUNTIME_UID:GID`; no Podman, WSL systemd, `sudo`, machine restart,
Runner certificate, or per-Skill container is required.

## First boot

From `mm-chat/`, create the two default bind sources as your normal user:

```bash
mkdir -p data/agent-skills data/agent-workspace
chmod 700 data/agent-skills data/agent-workspace
docker compose --env-file .env.single-server --profile app up -d --build
```

No command above needs `sudo`. The Backend image includes Bash, Python/pip,
Node/npm, Git, curl, jq, ripgrep, zip/unzip, and `file` for common Skills.
Compose deliberately refuses to auto-create these bind sources, preventing the
Docker daemon from leaving root-owned workspace directories behind.

The default workspace is `./data/agent-workspace`. To let Skills work on a
different host directory, set an absolute or project-relative
`AGENT_LOCAL_WORKSPACE_SOURCE` in `.env.single-server`, ensure the current UID
can read/write it, and recreate the Backend. That deliberately widens Skill
authority to that directory.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `AGENT_LOCAL_RUNTIME_ENABLED` | `true` in single-server Compose | Immediate enable/rollback switch. |
| `AGENT_LOCAL_RUNTIME_SOURCE` | `./data/agent-skills` | Host cache bind source. |
| `AGENT_LOCAL_RUNTIME_ROOT` | `/var/lib/mm-chat/agent-skills` | Container materialization root. |
| `AGENT_LOCAL_WORKSPACE_SOURCE` | `./data/agent-workspace` | Host workspace bind source. |
| `AGENT_LOCAL_WORKSPACE_ROOT` | `/workspace` | Container command workspace. |
| `AGENT_LOCAL_SHELL` | `/bin/bash` | Absolute shell used with `-c`; workspace login/profile files are not loaded. |
| `AGENT_LOCAL_APPROVAL_MODE` | `smart` | Deny destructive patterns; `off` disables only this soft denial. |
| `AGENT_LOCAL_CALL_TIMEOUT` | `30s` | Maximum one command duration. |
| `AGENT_LOCAL_RUN_TIMEOUT` | `5m` | Maximum Tool-loop wall time. |
| `AGENT_LOCAL_MAX_OUTPUT_BYTES` | `1048576` | Combined stdout/stderr budget per command. |
| `AGENT_LOCAL_MAX_CALLS_PER_RUN` | `32` | Local Tool call budget. |
| `AGENT_LOCAL_MAX_ROUNDS_PER_RUN` | `8` | Provider Tool-round budget. |
| `AGENT_LOCAL_MAX_CONCURRENT` | `2` | Concurrent commands per Backend process. |

`smart` approval failures are returned to the model as
`approval_required`; they do not silently execute. Catastrophic patterns stay
blocked even when approval mode is `off`.

## Operational truth

Agent Center reports:

```text
state=local_ready
reasonCode=LOCAL_DIRECT_EXECUTION
executable=true
```

This status means Skills can execute in ordinary Chat. It does **not** mean the
commands are isolated. Commands have the Backend user's authority inside the
container, the mounted workspace, and reachable networks. Keep secrets and
unrelated personal files outside the configured workspace.

## Chat smoke test

1. Install one Skill from **Store** and keep `AGENT_LOCAL_RUNTIME_ENABLED=true`.
2. Start a new Chat with a Tool-capable model and ask for a task that names the
   Skill or clearly matches its Store description.
3. The process view must show `skill` before `terminal` or any other task Tool;
   new requests must not advertise `skills_list` or `skill_view`.
4. Send `/skill-name <task>` using the exact installed name. The first Provider
   round already contains the loaded instructions and must not load the same
   Skill again.
5. Uninstall the last Skill and start another Turn. The Backend publishes an
   empty catalog tombstone and exposes no local Skill Tools.
6. Complete one `skill -> terminal -> answer` Turn, refresh the page, and open
   the process panel. The Tool order and terminal statuses must match the live
   view; history now prefers migration-`096` Agent events over legacy Message
   metadata.
7. For restart recovery, start a bounded long-running local command, restart
   only the Backend, then reload the Conversation. The unfinished Tool/Step
   must show `interrupted`; an answer that had already completed before restart
   must remain completed. This uses the same ordinary Backend user and requires
   no `sudo`, Runner, or per-Skill isolation.

The local regression command is:

```bash
cd mm-chat/backend
go test ./internal/chat -run TestLocalSkill -count=1
bash ../scripts/verify-chat-agent-event-log-postgres17.sh
```

## Rollback

Set `AGENT_LOCAL_RUNTIME_ENABLED=false` and recreate only Backend:

```bash
docker compose --env-file .env.single-server --profile app up -d --force-recreate backend
```

This stops new local Skill Tools without uninstalling packages or deleting the
workspace/cache. The retained G20/G21 OCI Runner profiles remain independently
disabled and are not a fallback.
