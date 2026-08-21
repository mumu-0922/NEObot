# Agent Host foundation design

## Goal and boundary

The browser and Docker Backend cannot attach arbitrary Host directories to an
already-running container. The Agent Host therefore runs as the ordinary WSL
user that owns the projects and will become the only authority that probes and
executes against Host paths.

The current slice extends the rollback-safe control plane with execution:

```text
future Docker Backend
      |
      | HTTP/1.1 + bearer token + expected runnerId
      v
private Unix socket (0600, parent 0700)
      |
      v
WSL Agent Host (ordinary user)
      |
      +-- capability facts
      +-- canonical WSL workspace resolution
      +-- Windows path -> fixed wslpath argv -> canonical WSL path
      +-- Workspace-pinned File / Terminal / Job execution
```

The Host reports `execution=true` only when its ExecutionManager starts. It
still reports an empty `permissionModes` array because advertising Read Only,
Workspace Write, or Full access before enforcement probes would be false.

## Trust boundary and threat model

Inputs from the Backend are untrusted even though the deployment is
single-user. Relevant threats are:

- a process that discovers the socket tries to call the Host without the
  independent token;
- a stale or replaced socket connects the Backend to the wrong process;
- a stale PID is reused by an unrelated process;
- a token/socket path is replaced with a symlink or a file owned by another
  user;
- JSON bodies or responses consume unbounded memory or smuggle unknown fields;
- a Windows path injects shell syntax into path conversion;
- error text discloses a private Host path or token;
- a symlink alias creates duplicate durable workspace identities;
- Runner state changes while the Backend still trusts an old workspace.
- a Host-bound Tool silently executes in Docker `/workspace` after Runner loss;
- an active Skill relative path escapes the canonical materialization root.

The current controls are:

- a dedicated 32-to-4096-byte token, stored in an absolute, owner-matched,
  regular, non-symlink, exact-mode-`0600` file;
- a stable validated Runner id persisted independently from the token;
- Runner-id pinning on every successful client response;
- no TCP listener and no reuse of the MCP Runner token;
- an owner-matched canonical socket directory with exact mode `0700` and a
  socket with mode `0600`;
- identity rechecks before stale-socket removal and before socket unlink on
  close;
- strict JSON decoding, one-document framing, protocol versions, fixed byte
  limits, HTTP timeouts, and stable sanitized error codes;
- a fixed absolute `wslpath` executable invoked with `exec.CommandContext`,
  `-u`, `--`, and a separate argument. No shell is involved;
- `filepath.EvalSymlinks` followed by an absolute-directory probe;
- fingerprints derived from `SHA-256(runnerId || NUL || canonicalPath)`.
- per-call canonical Workspace/fingerprint revalidation and one Host-rooted
  `localskills.Executor` per exact authority;
- separate bounded control and execution envelopes, process-group cancellation,
  strict error allowlists, contained non-symlink active Skill roots, and no
  Host-to-Docker fallback.

The fingerprint is a deduplication key, not a secret, MAC, filesystem inode
identity, or proof that a directory has not been replaced later. Every future
execution request must re-resolve and re-authorize the durable workspace.

## Socket and process lifecycle

`ListenUnix` refuses relative paths, long Unix paths, symlinked directories,
wrong modes, wrong ownership, regular files, and active sockets. An owned
socket is removed as stale only after a failed bounded dial and a same-file
recheck. `UnixListener.Close` removes only the exact socket inode it created.

`scripts/agent-host.sh` is the non-systemd supervisor for the first release.
It creates the token and Runner id only when absent, builds through a temporary
file, detaches through `setsid`, uses a private PID file, verifies the exact executable path before
signalling, and proves protocol plus Runner identity before reporting healthy.
It refuses to clean up a PID that belongs to an unrelated live process.

There remains a narrow same-UID race between the final stale-socket recheck and
unlink. The containing directory is exact-mode `0700`, so this race is limited
to another process already running with the same OS identity. A future daemon
supervisor may replace the wrapper but must preserve the same-file and
identity-fencing behavior.

## Failure and rollback behavior

- Host errors are stable and path-free; internal filesystem errors are not
  forwarded.
- A missing, malformed, mismatched, or oversized response fails as a protocol
  error.
- Transport loss fails as unavailable. There is no retry for operations with
  unknown outcomes and no Docker `local_direct` fallback for future Host-bound
  conversations.
- Stopping the Host makes every bound Agent Tool fail closed. Only ungrouped
  legacy Conversations retain Docker `local_direct`.
- Rollback is `./scripts/agent-host.sh stop` plus reverting the source commit.
  Runtime identity files may remain for forward recovery; rollback must not
  delete `data/`, `secrets/`, `backup/`, or `.env.single-server`.

## Known limitations

- Only WSL is implemented; Windows-drive projects use WSL tools through
  `/mnt/<drive>`.
- There is no native Windows Runner, permission Sandbox, or enforced permission
  preset yet. Foreground execution is one bounded buffered response rather than
  per-chunk NDJSON; the final durable transcript remains authoritative.
- Canonical string identity does not detect a directory deleted and recreated
  at the same path. Execution admission must revalidate future durable state.
- Unix peer credentials are not yet captured. Token, socket ownership, and
  Runner-id pinning are the current admission controls.
- The Host binary refuses root, but the operating-system user remains the
  maximum authority boundary for the future Full access mode.

## Decision record

| Date       | Decision | Consequence |
| ---------- | -------- | ----------- |
| 2026-08-21 | Use HTTP/1.1 over a private Unix socket | Reuses bounded Go HTTP machinery without opening a Host TCP port. |
| 2026-08-21 | Use one ordinary-user WSL Runner first | WSL and mounted Windows paths share one protocol; native Windows can be added through another `runnerId`. |
| 2026-08-21 | Advertise execution separately from permission modes | Bound Tools may use the Host while all unenforced presets stay unavailable. |
| 2026-08-21 | Reuse the guarded local executor at a Host root | File CAS, symlink checks, approvals, process groups, limits, and Jobs retain one implementation. |
| 2026-08-21 | Use a wrapper rather than systemd | The verified target WSL environment has no active user systemd manager. |
