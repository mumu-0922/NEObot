# WSL Agent Host foundation runbook

## Current rollout boundary

The Agent Host is an ordinary-user WSL process for arbitrary WSL and mounted
Windows project directories. It exposes capability discovery, canonical
Workspace resolution, WSL directory browsing, and a native Windows folder
picker. Migration `102` and the Backend `/v1/workspaces*` API persist Workspace
settings and execution authority. Bound Agent Conversations route File,
Terminal, Job, Skill-script, and artifact-read operations through the exact
socket; ungrouped legacy Conversations retain Docker `local_direct`.

The Host lifecycle remains independently deployable and reversible. Database
rollback is separate: `102.down` refuses once imported settings, a Host binding,
or a Conversation execution snapshot exists.

## Requirements

- WSL2 with Go 1.25, Python 3, and util-linux `setsid` available to the
  operator shell;
- an ordinary non-root WSL user;
- `wslpath` for Windows path interoperability;
- Windows PowerShell interop for the native folder picker;
- no systemd requirement;
- repository runtime paths must not be symlinked.

## Build and start

From `mm-chat/`:

```bash
./scripts/agent-host.sh prepare
./scripts/agent-host.sh status
```

`prepare` builds `backend/cmd/agent-host`, creates private runtime state only
when missing, starts it with `nohup`, and waits for an authenticated capability
response carrying the persisted Runner id. Re-running `prepare` is safe: the
binary is replaced atomically and a healthy matching Runner is reused only when
its bounded source fingerprint also matches; changed code restarts the managed
process.

The wrapper uses `setsid` in addition to `nohup` so the Host is not tied to the
deployment shell's process group. The recorded PID remains the Go process PID;
the wrapper does not introduce a long-running supervisor process.

Defaults:

| Path | Mode | Purpose |
| ---- | ---- | ------- |
| `.runtime/agent-host/` | `0700` | Private binary, Unix socket, PID, and log state. |
| `secrets/agent-host-runner-id` | `0600` | Stable identity used to pin Backend responses. |
| `.runtime/agent-host/agent-host.sock` | `0600` | No-TCP control channel. |
| `secrets/agent-host-token` | `0600` | Independent bearer token; generated only when absent. |
| `data/agent-skills/` | `0700` | Host-visible source of Backend-materialized Skill packages. |

The wrapper never overwrites an existing token or Runner id and never prints
the token. It refuses root, symlinked state, unsafe modes, wrong ownership,
malformed PID files, and unrelated live PIDs.

`AGENT_HOST_SKILLS_ROOT` may override the Skill root for tests. It must be an
existing absolute canonical directory with no symlink component. Active Skill
roots cross the socket only as contained relative names and are revalidated by
the Host before process creation.

## Connect the Compose Backend

Run `prepare` first, then set the active environment without changing any
project-directory mounts:

```dotenv
AGENT_HOST_ENABLED=true
AGENT_HOST_STATE_SOURCE=./.runtime/agent-host
AGENT_HOST_TOKEN_SOURCE=./secrets/agent-host-token
AGENT_HOST_RUNNER_ID_SOURCE=./secrets/agent-host-runner-id
AGENT_HOST_TIMEOUT=15s
```

Compose mounts the state directory read-only at `/run/mm-chat/agent-host` and
the two identity files as `/run/secrets/*`. The Backend runs as the same
ordinary UID/GID, validates the Docker-secret projections, pins the Runner id,
and exposes sanitized status at `GET /v1/workspaces/host-status`. Never mount
`$HOME`, a project parent, unrelated secrets, or a container socket.

## Focused verification

```bash
cd backend
go test ./internal/agenthost ./cmd/agent-host
go vet ./internal/agenthost ./cmd/agent-host
cd ..
bash scripts/test-agent-host.sh
```

The lifecycle smoke uses a temporary state tree and proves build, start,
health, idempotent start, restart with stable identity/token, and stop. It does
not alter the default runtime tree.

To inspect the deployed foundation without exposing credentials:

```bash
./scripts/agent-host.sh status
tail -n 50 .runtime/agent-host/agent-host.log
```

Do not paste the token, socket request headers, private project paths, or log
contents into issue trackers.

## Failure handling

- `unavailable`: inspect the private log, then run `restart`.
- unsafe PID error: inspect the PID and `/proc/<pid>/cmdline`; do not delete or
  signal it until identity is understood.
- unsafe token/socket path: restore exact owner/mode and remove symlink
  components. Do not replace the token merely to bypass validation.
- Windows path interop unavailable: install/fix `wslpath`; do not accept a
  guessed `/mnt/<drive>` conversion from the Backend.
- Runner identity mismatch: stop the unexpected process and restore the
  persisted Runner id expected by future durable workspace records. Do not
  silently adopt another Runner.
- `HOST_EXECUTION_UNAVAILABLE`: restore the same pinned Runner and run
  `prepare`; never switch a bound Conversation to Docker `/workspace`.

An execution request whose outcome is unknown is not retried. Workspace
binding mutates only Backend registration; directory browse/pick/resolve never
mutate project files.

## Stop and rollback

```bash
./scripts/agent-host.sh stop
```

Set `AGENT_HOST_ENABLED=false` and recreate Backend to disconnect the socket;
existing Workspace records remain readable, while every Host-bound Agent Turn
fails closed. Ungrouped legacy Conversations may still use `local_direct`.
Then stop the Host if desired. Preserve `secrets/agent-host-token` and
`secrets/agent-host-runner-id` for forward recovery unless the owner explicitly
authorizes credential destruction. The disposable `.runtime/agent-host/`
directory may be rebuilt, but never delete or rewrite `data/`, `secrets/`,
`backup/`, or `.env.single-server` as part of rollback.

Rollback never deletes Workspace records or project directories and never
rewrites protected runtime trees merely to reach an older schema head.
