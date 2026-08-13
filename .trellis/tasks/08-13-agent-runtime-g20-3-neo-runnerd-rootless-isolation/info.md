# G20.3 `neo-runnerd` — Technical Design

## Decision

Create an isolated `agentrunner` bounded context and host daemon. Migration
`085` owns durable request replay and Sandbox lifecycle projection. The daemon
uses strict mTLS/RPC admission, revalidates G20.2 lease/snapshot/Kill-Switch
authority, requires an exact-release capability probe, then calls a narrow OCI
driver. No API/Chat startup path imports it.

## Data flow

```text
Backend client certificate + neo.runner-rpc/v1 envelope
  -> strict transport/JSON/time/version/identity validation
  -> PostgreSQL replay claim + exact G20.2 authority fence + signed ticket
  -> Runner local fsync replay claim (no Runner DB credential)
  -> release-bound probe
  -> verified Workspace + new Scratch/broker boundary
  -> Podman create -> strict inspect -> start
  -> durable Sandbox projection
  -> heartbeat/list/cancel/reconcile
  -> exact kill/reap -> remove -> filesystem cleanup
  -> durable sanitized replay response
```

## Privilege model

```text
agent_runner_owner   NOLOGIN; owns Runner tables/functions
agent_runner_control NOLOGIN; exact function EXECUTE only for Backend control plane
neo-runnerd OS user  non-root; no DB/object/provider/vault credentials
```

The Backend supplies a short-lived exact Attempt authority proof over mTLS; the
Runner gains no database access in production topology. PostgreSQL stores the
Control Plane claim; the host-local fsync ledger gives the daemon its own
restart-stable replay fence. Disposable PostgreSQL tests exercise the control
repository under the dedicated role. Broader API/runtime roles gain no Runner
DML.

## Driver rule

`PodmanDriver` is the production implementation. `FakeDriver` exists only in
tests and reports a non-production identity. Readiness can become true only
when the exact probe reads approved release artifacts and proves kernel/userns/
cgroup/seccomp/storage/network/reap features. Driver calls use direct argv,
bounded env and absolute approved binaries; never a shell.

## Rollback

- Keep migration `085` inert with Runtime globally unavailable when rolling
  back the daemon/backend artifact.
- Stop new launches, reconcile/kill exact live Sandboxes, remove Scratch and
  retain replay/lifecycle evidence.
- Down migration refuses while any replay/Sandbox authority exists.
- Host release rollback is an exact previous manifest/artifact unit; never
  substitute an unpinned distro runtime or rootful Docker.
