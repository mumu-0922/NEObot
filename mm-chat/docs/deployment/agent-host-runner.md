# WSL Agent Host foundation runbook

## Current rollout boundary

The Agent Host is an ordinary-user WSL process that will eventually let Agent
conversations use arbitrary WSL and mounted Windows project directories. The
Host process is deliberately dark: it exposes only capability discovery and
canonical workspace resolution. Migration `102` and the Backend
`/v1/workspaces*` API now persist Workspace settings and execution authority,
but the Docker Backend is not mounted to the Host socket and no existing
execution is routed through it. The bind route therefore returns
`503 HOST_WORKSPACE_UNAVAILABLE` instead of guessing a Host path.

The Host lifecycle remains independently deployable and reversible. Database
rollback is separate: `102.down` refuses once imported settings, a Host binding,
or a Conversation execution snapshot exists.

## Requirements

- WSL2 with Go 1.25, Python 3, and util-linux `setsid` available to the
  operator shell;
- an ordinary non-root WSL user;
- `wslpath` for Windows path interoperability;
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

The wrapper never overwrites an existing token or Runner id and never prints
the token. It refuses root, symlinked state, unsafe modes, wrong ownership,
malformed PID files, and unrelated live PIDs.

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

An execution request whose outcome is unknown will not be retried once
execution routes exist. This foundation currently has no mutation route.

## Stop and rollback

```bash
./scripts/agent-host.sh stop
```

Stopping is sufficient to roll back this dark foundation because nothing in
the live Compose stack consumes it. Revert the source commit if the binary and
protocol should also be removed. Preserve `secrets/agent-host-token` and
`secrets/agent-host-runner-id` for forward recovery unless the owner explicitly
authorizes credential destruction. The disposable `.runtime/agent-host/`
directory may be rebuilt, but never delete or rewrite `data/`, `secrets/`,
`backup/`, or `.env.single-server` as part of rollback.

The later Backend-integration slice must add a read-only bind of the exact
socket runtime directory plus token secret, construct the pinned
`agenthost.Client`, add a health projection and feature flag, and retain
fail-closed routing. It must not switch existing conversations during the
socket rollout.
