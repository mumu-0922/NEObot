# Rootless stack and target-host evidence

## Sources inspected

- Podman rootless tutorial (live upstream `main`, inspected 2026-08-13):
  <https://github.com/containers/podman/blob/main/docs/tutorials/rootless_tutorial.md>
- Podman create contract (live latest manual, inspected 2026-08-13):
  <https://docs.podman.io/en/latest/markdown/podman-create.1.html>
- OCI Runtime Specification Linux config (live upstream `main`, inspected
  2026-08-13):
  <https://github.com/opencontainers/runtime-spec/blob/main/config-linux.md>

## Recommended release tuple

Use a release-manifest-owned, exact tuple rather than accepting whatever is on
`PATH`:

```text
Podman 6.1.0
crun 1.29.1
conmon 2.2.1
netavark 2.1.0
aardvark-dns 2.1.0
pasta/passt 2026_07_28.f8df3f1
containers/storage 1.59.1 contract
OCI Runtime Specification 1.3.0 contract
```

These were the latest non-prerelease upstream tags visible on 2026-08-13.
The tuple is a project approval target, not permission to download latest at
Runner startup. Installation must provide exact binaries/configuration and the
Runner probe must compare them with a checked-in release manifest.

## Why Podman + crun

- Podman has a daemonless rootless execution path and exposes the required
  read-only rootfs, nonzero user, user namespace, seccomp, capability, network,
  PID, CPU and memory controls.
- crun is an OCI runtime and makes the runtime identity explicit.
- `--network=none` is the only G20.3 production launch mode. The brokered
  network path is deferred to G20.4 and must not silently become ordinary
  rootless networking.
- `--userns=auto:size=65536` provides per-container namespace allocation, but
  requires the Runner account to have unique subordinate UID/GID ranges and
  working `newuidmap`/`newgidmap`.
- OCI config inspection is required after create and before start. CLI flags
  are intent, not enforcement evidence.

## Exact host observation

Observed from the development host on 2026-08-13:

```text
user: mumu (uid/gid 1000; not the future neo-runner account)
kernel: Linux 6.18.33.2-microsoft-standard-WSL2
distribution: Ubuntu 22.04.4 LTS
cgroup: cgroup v2; cpu/io/memory/pids controllers present
user.max_user_namespaces: 63475
subuid/subgid: mumu:100000:65536
unprivileged user namespace: functional
pidfd API: available
Docker: 29.7.2 (not acceptable Runner evidence)
Podman/crun/conmon/netavark/aardvark-dns/pasta/slirp4netns/
fuse-overlayfs/newuidmap/newgidmap: absent
user systemd/cgroup delegation: unavailable (`offline` / no user bus)
current process seccomp: disabled
```

Ubuntu Jammy repositories offer Podman 3.4.4 and crun 0.17, which do not match
the approved release tuple. Installing those packages would not make readiness
true. No host package or account change is made by G20.3 source implementation.

## Probe consequences

- Readiness is false on this host and must return `ISOLATION_UNAVAILABLE`.
- Docker group membership and rootful Docker cannot satisfy the probe.
- A unit/fake-driver acceptance lane can prove validation, replay, lifecycle,
  filesystem and OCI-intent logic, but is not production isolation evidence.
- The full host acceptance command must exit nonzero until run as the dedicated
  service account with the exact tuple, cgroup delegation and seccomp profile.
