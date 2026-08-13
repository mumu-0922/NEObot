# Workspace, Artifact and Sandbox lifecycle boundary

## Workspace input

The Runner must never receive an arbitrary Project host path. G20.3 accepts one
content-addressed snapshot archive already delivered into the Runner-owned
inbox by a trusted deployment channel. The launch envelope carries only the
snapshot ID/fingerprint; a Runner-owned catalog resolves it.

Materialization rules:

- bounded tar stream and bounded file/tree counts;
- canonical relative UTF-8 paths only;
- reject absolute paths, `..`, NUL, duplicate/case-fold/Unicode collisions;
- reject symlink, hardlink, device, FIFO and socket entries;
- create regular files/directories without following links;
- verify per-file bytes and final tree fingerprint before launch;
- materialized Workspace becomes read-only in the Sandbox;
- no source checkout, Git resolution or package installer runs here.

The catalog owns a fresh staging directory and atomic promotion. A caller
cannot supply a host source/destination path in Runner RPC.

## Scratch

- One Runner-owned mode-0700 Scratch directory per exact Attempt.
- The OCI launch has exactly two non-system mounts: Workspace read-only and
  Scratch read/write. Package/runtime bytes are inside the immutable digest
  image/bundle, not arbitrary binds.
- Scratch quota is represented in the frozen launch envelope and enforced by
  the approved storage/cgroup mechanism; lack of enforceable quota makes the
  probe/launch fail closed.
- Terminal, kill, stale/orphan reconcile and failed launch paths all remove
  Scratch through a path-contained, no-symlink cleanup helper.

## Artifact egress

G20.3 implements a local Artifact Broker intake boundary, not object-store
publication:

- Sandbox receives a narrow per-Attempt Unix socket/pipe, never S3/MinIO
  credentials.
- One bounded upload frame contains requested logical name/type/size and bytes.
- Broker streams into a Runner-owned quarantine file, computes SHA-256, rejects
  unsafe path/name/type/size and records only content-free metadata.
- Acceptance is still provisional. G20.4/5 Backend authority owns malware/
  secret scan, owner binding and object-store publication.
- Stale lease/generation cannot finalize an Artifact. Partial files are removed
  on disconnect, terminal or orphan cleanup.

The G20.3 external RPC surface does not expose an object-storage credential or
arbitrary filesystem upload/download route.

## Exact lifecycle

```text
validated launch envelope
  -> reserve deterministic Attempt/Sandbox record
  -> materialize/verify read-only Workspace
  -> create mode-0700 Scratch + broker socket
  -> podman create (never `run`)
  -> inspect generated OCI/security/resource config
  -> start
  -> bounded heartbeat/list/cancel
  -> stop/kill exact ID
  -> wait until no descendants/cgroup remain
  -> remove container + broker partials + Scratch + staging
```

No substring/prefix process killing is allowed. The driver uses the exact
container ID returned by create, validates labels that bind Run/Attempt/
generation/snapshot, and fails closed if inspection disagrees.
