# MCP Stdio Runner Design

## Topology

```text
Go backend -- independent bearer --> private Runner HTTP handler
Runner manager -- MCP stdio --> approved child process group
```

The Runner image is dedicated, immutable, non-root, read-only, capability-free,
resource-bounded, and connected only to internal control plus egress networks.
It receives no browser bearer, database/object credentials, Docker socket, or
arbitrary host mount.

## Manager lifecycle

`Manager.acquire` returns an existing non-expired session or starts a new child
when capacity permits. Each child receives:

- an absolute manifest-approved executable and argv array;
- a mode-`0700` directory beneath `/work` as `HOME`, `TMPDIR`, and cwd;
- a fixed safe `PATH` plus validated manifest environment;
- a dedicated Unix process group.

The default idle timeout is 15 minutes, maximum lifetime is 24 hours, and
process capacity is four. Release updates activity time. Crash or call
cancellation invalidates the session and synchronously kills the process group
before a later request may start a replacement. The reaper handles idle and
lifetime expiry.

## Internal API

- `GET /healthz`
- `POST /internal/v1/tools/list`
- `POST /internal/v1/tools/call`

Every internal Tool route requires constant-time comparison of the independent
Runner bearer. JSON is bounded and rejects unknown fields. Server identity is a
bounded lowercase manifest ID; clients cannot provide argv or environment.
Errors return only bounded codes.

## Failure boundary

Runner unavailability fails only the selected stdio MCP branch. It does not
make the backend globally unready. On shutdown, every managed session is closed,
every process group is terminated, and every work directory is removed.

## Design decisions and tradeoffs

- **One resident Runner, on-demand children** uses less memory than one
  permanent container per server while keeping child lifecycle centralized.
- **Process groups instead of single-PID kill** prevent descendants from
  surviving cancellation, at the cost of a Unix-specific runtime contract.
- **Independent internal bearer** separates Runner compromise from user
  sessions; token rotation requires recreating backend and Runner together.
