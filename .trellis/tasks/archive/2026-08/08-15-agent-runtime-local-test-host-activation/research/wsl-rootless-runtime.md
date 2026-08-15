# WSL2 Rootless Runtime Research

## Question

How can the current Ubuntu 22.04 WSL2 development machine run a real bounded
Agent Skill without weakening the production Runner gates or using rootful
Docker as an execution fallback?

## Host facts

- The host is Ubuntu 22.04 on WSL2, `amd64`, with WSL `2.7.11.0` and kernel
  `6.18.33.2-microsoft-standard-WSL2`.
- PID 1 is currently `/init`; `/etc/wsl.conf` does not enable systemd.
- systemd and systemd-sysv are installed, but the user manager is offline and
  the current process has no delegated cgroup v2 subtree.
- cgroup v2, the `cpu`, `memory`, and `pids` controllers, user namespaces, and
  one 65,536-entry subordinate UID/GID range for `mumu` are present.
- `uidmap`, Podman, crun, conmon, slirp4netns, and fuse-overlayfs are absent.
- The user can run Docker, but the Agent Runtime contract explicitly forbids a
  rootful Docker fallback.
- `sudo` requires the operator's password. Automation must never request,
  receive, pipe, log, or store that password.

## Trusted upstream findings

### WSL systemd

Microsoft's WSL documentation requires WSL 0.67.6 or newer, a
`[boot] systemd=true` setting in `/etc/wsl.conf`, and a WSL shutdown/restart.
The installed WSL version satisfies the version requirement. Enabling systemd
is the narrow path that supplies the user session and delegated cgroup subtree
required by the existing Runner inspection contract.

Source:
<https://learn.microsoft.com/en-us/windows/wsl/systemd>

### Rootless Podman prerequisites

Podman's official rootless tutorial requires `newuidmap`/`newgidmap` and
non-overlapping `/etc/subuid` and `/etc/subgid` ranges. Rootless storage and
network helpers are normally installed by the distribution. The project uses
`networkMode=none`, but installing the distribution's rootless helpers avoids
an incomplete Podman host while no helper is treated as proof of production
readiness.

Sources:

- <https://podman.io/docs/installation>
- <https://github.com/podman-container-tools/podman/blob/v6.1.0/docs/tutorials/rootless_tutorial.md>

### Ubuntu 22.04 version gap

The Ubuntu 22.04 repositories provide Podman `3.4.4`, crun `0.17`, and conmon
`2.0.25`. The checked-in Runner template names Podman `6.1.0`, crun `1.29.1`,
and conmon `2.2.1`. The old distribution Podman cannot satisfy the existing
exact-version host contract. The old OBS libcontainers repositories are also
insufficient: the observed Ubuntu 22.04 unstable index tops out at Podman
`4.6.2`, crun `1.14.4`, and conmon `2.1.13`.

Podman 6.1.0 publishes Linux remote clients, not a static Linux engine. Its
engine must therefore be built from the exact upstream source or obtained from
another independently reviewed package producer. Building from the exact
source is the narrower trust path for this local-test profile.

### Pinned upstream inputs

The following upstream files were downloaded and independently hashed during
research. A checked-in local-test lock should bind these values and fail closed
on any drift:

| Component | Upstream identity | SHA-256 |
| --- | --- | --- |
| Podman source | tag `v6.1.0`, commit `cade97a52ebdf9dbf9e81de8009015776837a074` | `e086183db2f852476a7fa2580d0276cef32086b4cf17ae7020948f06eb613e0d` |
| Go toolchain | `go1.25.9.linux-amd64.tar.gz` | `00859d7bd6defe8bf84d9db9e57b9a4467b2887c18cd93ae7460e713db774bc1` |
| crun | `crun-1.29.1-linux-amd64` | `0a5ea25cafe618bbfbf1c747871155063619f18025ccdd8ad648c97633f35d57` |
| conmon | `conmon.amd64` from `v2.2.1` | `1d97294c14c43d477e0a0826e9cd0f2a2af373ddfafe6f10252e8a3c43f32be6` |

The standard crun asset reports `+SYSTEMD`, while the separately published
`disable-systemd` asset does not. The standard asset is required for this
systemd/cgroup profile.

Upstream release references:

- <https://github.com/podman-container-tools/podman/releases/tag/v6.1.0>
- <https://github.com/containers/crun/releases/tag/1.29.1>
- <https://github.com/containers/conmon/releases/tag/v2.2.1>
- <https://go.dev/dl/>

## Feasible approaches

### A. Native WSL local-test profile (recommended)

- Preserve the current distro and enable WSL systemd.
- Install only the Ubuntu build/rootless prerequisites through one explicit
  operator-run `sudo` command.
- Build Podman from pinned source as the normal user and install pinned crun
  and conmon into a local-test-only root.
- Keep generated state, evidence, and containers under an explicitly named
  local-test root and watermark every report as `local_test`.
- Run a no-network, no-secret, no-user-data bounded smoke and verify actual
  post-create/post-start isolation.

Trade-off: one WSL restart and one interactive sudo step are unavoidable.

### B. Separate Podman machine/distro

Use Podman's Windows installer and a separate managed WSL machine. This gives
stronger operational separation but creates another machine-like environment,
does not match the user's single-machine preference, and still needs separate
Runner wiring.

### C. Reuse Docker or Ubuntu's Podman 3.4

This is faster but violates the existing rootless Runner contract or creates a
large unreviewed compatibility fork. It is rejected.

## Decision

Use approach A. Keep production `verify-agent-runner-host.sh`, release
manifests, activation schemas, and promotion evaluators unchanged. Local-test
readiness can unlock only local bounded smoke; it must never emit
`ACTIVATION_READY`, `PROMOTION_READY`, production evidence, or an approved
production manifest.

## Rollback requirements

- Back up `/etc/wsl.conf` before a guarded edit and restore only when the
  current file still matches the bootstrap's recorded post-edit hash.
- Record which apt packages were absent before installation; rollback may
  propose removing only that set and must not silently run apt autoremove.
- Remove only the dedicated local-test toolchain/state roots after proving
  their canonical paths and ownership. Never touch project runtime state.
- A rollback of `wsl.conf` also requires a WSL shutdown/restart to take effect.
