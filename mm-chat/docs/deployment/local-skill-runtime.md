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
can read/write it, and recreate the Backend. Set
`AGENT_LOCAL_WORKSPACE_HOST_ROOT` to the same canonical absolute Host path when
users should be able to paste its Linux or WSL UNC representation. That value
is only an input alias; the bind remains the single filesystem authority.
This deliberately widens Skill authority to that one directory, so never use a
Home or parent directory containing unrelated projects, live env, Secrets, or
backups.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `AGENT_LOCAL_RUNTIME_ENABLED` | `true` in single-server Compose | Immediate enable/rollback switch. |
| `AGENT_LOCAL_RUNTIME_SOURCE` | `./data/agent-skills` | Host cache bind source. |
| `AGENT_LOCAL_RUNTIME_ROOT` | `/var/lib/mm-chat/agent-skills` | Container materialization root. |
| `AGENT_LOCAL_WORKSPACE_SOURCE` | `./data/agent-workspace` | Host workspace bind source. |
| `AGENT_LOCAL_WORKSPACE_ROOT` | `/workspace` | Container command workspace. |
| `AGENT_LOCAL_WORKSPACE_HOST_ROOT` | empty | Optional clean absolute Host alias for the exact workspace source; accepts Linux/WSL pasted paths only below that root. |
| `AGENT_LOCAL_SHELL` | `/bin/bash` | Absolute shell used with `-c`; workspace login/profile files are not loaded. |
| `AGENT_LOCAL_APPROVAL_MODE` | `smart` | Deny destructive patterns; `off` disables only this soft denial. |
| `AGENT_LOCAL_CALL_TIMEOUT` | `30s` | Foreground command limit and maximum explicit `terminal.timeoutSeconds`. |
| `AGENT_LOCAL_RUN_TIMEOUT` | `5m` | Whole local Tool-loop limit and background Job default when `timeoutSeconds=null`. |
| `AGENT_LOCAL_MAX_OUTPUT_BYTES` | `1048576` | Combined stdout/stderr budget per command. |
| `AGENT_LOCAL_MAX_CALLS_PER_RUN` | `32` | Local Tool call budget. |
| `AGENT_LOCAL_MAX_ROUNDS_PER_RUN` | `8` | Provider Tool-round budget. |
| `AGENT_LOCAL_MAX_CONCURRENT` | `2` | Concurrent commands per Backend process. |

In `smart` mode, destructive Terminal calls enter the durable Chat Agent
approval flow when the event/approval repository is available. The same Tool
call waits at most five minutes and executes only after `Allow once` or an exact
conversation grant. Without durable authority the bounded result remains
`approval_required`. Catastrophic patterns stay blocked and cannot be bypassed
even when approval mode is `off`.

The Terminal schema is intentionally capped at the foreground Call timeout.
For a command that may exceed it, the Agent must use
`runInBackground=true, timeoutSeconds=null`, then read the exact Job with
`job_output(wait=true)`. Do not raise the explicit argument to the Run timeout;
that recreates a schema/executor mismatch. The runtime prompt also directs
Python commands to `python3` after availability checks because a `python` alias
is not portable across Host Workspaces.

Example for one explicitly authorized WSL project:

```dotenv
AGENT_LOCAL_WORKSPACE_SOURCE=/home/example/projects/authorized-project
AGENT_LOCAL_WORKSPACE_HOST_ROOT=/home/example/projects/authorized-project
```

`/home/example/projects/authorized-project/README.md` and
`\\wsl.localhost\Ubuntu\home\example\projects\authorized-project\README.md`
then resolve to the same internal `README.md`. Other project roots remain
unavailable.

## Operational truth

Backend startup and the local Runtime verification gate report:

```text
state=local_ready
reasonCode=LOCAL_DIRECT_EXECUTION
executable=true
```

This status means Agent-mode turns can execute installed Skills through the
ordinary Chat runtime. It does **not** mean the commands are isolated. Commands
have the Backend user's authority inside the container, the mounted workspace,
and reachable networks. Keep secrets and unrelated personal files outside the
configured workspace. The retired Runner/Canary control plane is not involved.

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
   metadata. Migration `097` additionally keeps one current Goal for the
   Conversation.
7. For restart recovery, start a bounded long-running local command, restart
   only the Backend, then reload the Conversation. The unfinished Tool/Step
   must show `interrupted`; an answer that had already completed before restart
   must remain completed. This uses the same ordinary Backend user and requires
   no `sudo`, Runner, or per-Skill isolation.
8. Ask for a clearly multi-step objective. The process view should show
   `create_goal`, then automatic Goal Rounds without requiring repeated
   “继续”. After any successful `terminal` mutation, the Agent must run a real
   check and call `verify_completion` before it may mark the Goal complete.
9. Restart Backend while an active Goal exists, reload the Conversation, and
   say “继续”. The restored Goal is disarmed until that direct human request;
   it must not continue work merely because the service restarted.

The local regression command is:

```bash
cd mm-chat/backend
go test ./internal/chat -run TestLocalSkill -count=1
bash ../scripts/verify-chat-agent-event-log-postgres17.sh
bash ../scripts/verify-chat-agent-goals-postgres17.sh
```

## Rollback

Set `AGENT_LOCAL_RUNTIME_ENABLED=false` and recreate only Backend:

```bash
docker compose --env-file .env.single-server --profile app up -d --force-recreate backend
```

This stops new local Skill Tools without uninstalling packages or deleting the
workspace/cache. There is no OCI Runner fallback or second Agent control plane.
