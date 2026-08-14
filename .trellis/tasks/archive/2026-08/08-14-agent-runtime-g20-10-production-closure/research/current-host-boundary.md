# Current exact-host boundary

The repository's checked-in Runner release manifest is an example with
`approved=false`, placeholder SHA-256 values and paths that are not an installed
release. `verify-agent-runner-host.sh` delegates to `neo-runner-probe`, which
requires the exact non-root identity, immutable binaries, rootless Podman/crun,
cgroup v2 delegation, subordinate IDs, seccomp and a fresh approved isolation
report.

Therefore G20.10 cannot honestly produce production-ready evidence on this
host. The committed fixture must be explicitly `template`, carry
`isolation_unavailable`, and the aggregate gate must exit nonzero for production
promotion. Offline schema/self-tests may pass while the production decision
remains held.

No code in this slice may install host packages, create service accounts,
change subuid/subgid/cgroups, write mTLS material, start a Runner, access the
live env, or touch `data/`, `secrets/` or `backup/`.
