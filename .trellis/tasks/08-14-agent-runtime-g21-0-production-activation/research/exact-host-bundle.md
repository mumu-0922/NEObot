# Exact-host deployment bundle

## Existing material

The repository already contains the Runner daemon/probe sources, exact release
manifest template, seccomp profile, systemd unit and environment template. The
deployment directory is explicitly only a review artifact; there is no
content-addressed build output proving which binary and configuration files
belong to one release.

## Recommended bundle

Add an offline bundle builder that compiles static `neo-runnerd` and
`neo-runner-probe` binaries, copies only the reviewed release/config/systemd
artifacts, and writes a canonical file inventory containing path, mode, size
and SHA-256. A verifier rejects symlinks, unexpected/missing files, mode or byte
drift, placeholder production bindings and an unapproved production release
manifest.

The builder writes only to an explicit derived-artifact directory and never
installs packages, creates accounts, edits subuid/subgid/cgroups/systemd,
provisions certificates or starts services. The current development host can
build and verify a `template` bundle while exact-host production activation
continues to fail closed.

## Host topology

Allow the daemon to bind either loopback or one literal RFC1918/private address;
wildcard, hostname and public binds remain forbidden. Separate-host deployment
therefore needs no public reverse proxy or rootful container fallback. The
systemd unit runs the exact probe before start and retains the dedicated
non-root/cgroup/rootless-OCI boundary.
