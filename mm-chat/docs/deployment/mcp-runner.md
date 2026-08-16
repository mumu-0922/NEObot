# MCP Tools and Runner Operations

## Default state

MCP is off by default. Remote MCP runs inside the Go backend. Only the exact
`AUTH_BOOTSTRAP_USER_ID` may install or manage definitions. Its ready Servers
are shared with authenticated users, who may select/use Tools but cannot
install, configure, validate, or delete them. npm-backed stdio servers use one
optional hardened `mcp-runner` Compose service; do not run one permanent
container per Tool server.

```text
MCP_ENABLED=false
MCP_REMOTE_ENABLED=true
MCP_STDIO_ENABLED=false
MCP_AUDIT_RETENTION=2160h
MCP_CLEANUP_INTERVAL=1h
MCP_MARKETPLACE_ENABLED=false
```

Cleanup continues while the global switch is off so expired calls, OAuth
state, run snapshots, account-deletion queue entries, and MinIO artifacts do
not become permanent.

## Optional LobeHub Marketplace

The Marketplace is a Go backend adapter; it does not add a resident container
or enable the stdio Runner. Register the Neo Chat M2M identity explicitly with
LobeHub, then store only its client secret in a dedicated mode-`0600`,
runtime-owner-owned file:

```text
MCP_MARKETPLACE_ENABLED=true
MCP_MARKETPLACE_BASE_URL=https://market.lobehub.com
MCP_MARKETPLACE_CLIENT_ID=<registered-client-id>
MCP_MARKETPLACE_CLIENT_SECRET_SOURCE=./mcp/marketplace-client-secret
MCP_MARKETPLACE_TIMEOUT=8s
MCP_MARKETPLACE_CACHE_TTL=5m
```

The file is mounted as
`/run/secrets/mm_chat_mcp_marketplace_client_secret`. Do not place its contents
in `.env`, logs, images, or Git. Startup never registers a third-party identity.
Disabling Marketplace or an upstream outage affects only search/install;
already installed Servers and chat remain available. Search/detail cache
contains bounded public metadata only. Rotate by replacing the secret file and
recreating the backend.

## Manifest workflow

The checked-in default is `mcp/manifest.json` with schema version `1`. It may
contain hidden Marketplace artifacts as well as administrator-visible shared
Servers. Treat the manifest as reviewed configuration, not user data. Changes
take effect only after restart.

Validate before release or restart:

```bash
docker run --rm \
  --entrypoint /usr/local/bin/mm-chat-mcp-validate \
  -v "$PWD/mcp/manifest.json:/etc/mm-chat/mcp-manifest.json:ro" \
  mm-chat/backend:<release> \
  --manifest /etc/mm-chat/mcp-manifest.json
```

The command prints counts only. It rejects unknown fields, duplicate IDs,
relative executables, shell executables, working-directory overrides, unsafe
environment names, inline auth secrets, invalid grants, unsafe endpoints, and
out-of-range lifetimes.

A reviewed manifest Server may add a non-empty `allowedTools` array. When
present, both discovery and direct Tool invocation fail closed to those exact
upstream names; every `toolPolicy` key must be inside the allowlist. A stdio
Server may additionally set `instanceScope: "run"`. Backend then derives a
non-reversible instance ID from the exact Server, user, Conversation, and Chat
Run instead of reusing one process identity across Runs.

Private stdio installations that rebind to an exact reviewed manifest artifact
inherit its allowlist for both Runner listing and calling. Stored private
metadata cannot widen or replace the reviewed Tool surface.

Backend, validator, and Runner compile the same strict manifest parser. When a
release adds a manifest field, build and roll the paired Backend and Runner
images from that same source revision. A healthy old Runner has not re-read the
mounted file and is not proof that it can restart with the new schema.

Minimal remote entry:

```json
{
  "version": 1,
  "servers": [
    {
      "id": "example",
      "name": "Example",
      "transport": "streamable_http",
      "endpointUrl": "https://mcp.example/v1",
      "grants": [{ "scopeType": "global", "defaultEnabled": false }],
      "toolPolicy": { "read_item": "read", "write_item": "write" }
    }
  ]
}
```

Minimal stdio entry:

```json
{
  "version": 1,
  "servers": [
    {
      "id": "local-example",
      "name": "Local Example",
      "transport": "stdio",
      "instanceScope": "run",
      "command": {
        "argv": ["/usr/local/bin/example-mcp", "--stdio"],
        "idleTimeout": "15m",
        "maxLifetime": "24h"
      },
      "allowedTools": ["read_item", "write_item"],
      "toolPolicy": { "read_item": "read", "write_item": "write" },
      "grants": [{ "scopeType": "global", "defaultEnabled": false }]
    }
  ]
}
```

Static manifest entries still require every executable and dependency to exist
in the immutable Runner image. They may not use `sh -c`, `uvx`, a Docker
socket, or host source mounts. Dynamic Marketplace artifacts are the sole
runtime-download exception: Backend re-fetches the item and accepts only
`command=npx`, an exact registry package/version, bounded argv, and declared
secret environment names. Shell, Docker, Git, URL/file package specifications,
and floating npm tags remain forbidden. Secret values are encrypted in the
Backend vault and sealed separately from the artifact on the internal control
plane; they are never written into the manifest or public DTO.

### Reviewed Playwright Browser

`playwright-browser-0.0.79` is the checked-in Browser artifact. The Runner
dependency and official base image are both pinned to `@playwright/mcp`
`0.0.79`. It starts headless Chromium with an in-memory isolated profile,
service workers blocked, image responses omitted, code generation disabled,
and bounded action/navigation timeouts. `instanceScope: "run"` prevents page,
Cookie, and in-process browser state reuse across Chat Runs.

The upstream package currently advertises 24 Tools, including the RCE-
equivalent `browser_run_code_unsafe`. Neo Chat exposes only the 17 exact names
in the manifest allowlist. It also excludes `browser_evaluate`, file upload,
network request/body inspection, screenshot capture, and storage-state
mutation. Connector call paths re-check the allowlist even after Tool
discovery, so an upstream update cannot invoke a newly added name through a
stale or forged call.

Only the upstream read-only console, find, and snapshot Tools use Neo Chat's
parallel `read` policy. `browser_wait_for` is upstream non-read-only and stays
an ordered `write` barrier, despite its passive-looking name.

Enable both `MCP_ENABLED=true` and `MCP_STDIO_ENABLED=true`, start the existing
`mcp-runner` profile, then explicitly select `Browser (Playwright)` for the
Conversation. This does not move `local_direct` Skills into the Runner or add
any sudo/new-machine requirement; it only activates the existing optional MCP
stdio service for the Browser package.

### Marketplace npm artifacts

A Marketplace stdio option is installable only after Backend refreshes its
authoritative detail and binds provider, identifier, Marketplace version,
connection/install method, package, argv, environment field names, and the
derived deployment hash. Browser requests choose only an issued deployment
hash and transient values for declared fields; they cannot choose a command or
package.

If a checked-in manifest entry has the same approval fingerprint, its immutable
absolute executable remains authoritative. Otherwise Backend derives a dynamic
artifact containing only an exact npm package specification, bounded extra
arguments, declared environment names, and idle/lifetime limits. It persists
that artifact only on the administrator-owned Server and seals it before each
Runner request. The Runner validates the definition again and invokes
`/usr/local/bin/npx --yes <package>@<exact-version>` with a fixed executable and
argv rather than a browser-selected shell command. npm may internally use its
own fixed lifecycle launcher. The Server-specific work directory is its cache. npm install
scripts can execute inside this container; this is why only the administrator
may install and the Runner must retain every isolation fence below.

An installed administrator-owned stdio row stores `runner://<approved-id>`,
bounded provenance, and (for dynamic npm) the sealed-control-plane artifact
source. The public API hides all of these. Backend reconstructs and validates
the binding before validation, selection, and execution. Ordinary users share
the definition but execution resolves the administrator's vault credential;
no credential row is copied to the user.

## Runner token and image

Generate a dedicated high-entropy token without printing it:

```bash
install -d -m 700 mcp
umask 077
openssl rand -base64 48 > mcp/runner-token
chmod 600 mcp/runner-token
```

Set `MCP_RUNNER_TOKEN_SOURCE=./mcp/runner-token`. The source must be a regular
non-symlink file, owned by `MM_CHAT_RUNTIME_UID`, mode `0600`, and between 32
and 4096 bytes. Do not reuse a browser bearer, provider key, or RAG token.

Production requires a dedicated immutable Runner digest:

```text
MCP_RUNNER_IMAGE=registry.example/neo-chat-mcp-runner@sha256:<64-hex>
MCP_RUNNER_URL=http://mcp-runner:8090
```

Build/publish all four release images with `scripts/release-images.sh`; the
Runner build uses Dockerfile target `mcp-runner`.

Browser-backed stdio packages use the exact Chromium binary baked into the
pinned official Playwright MCP base image and exposed at Playwright's expected
`/opt/google/chrome/chrome` path.
Do not repair a deployment with runtime `playwright install`: the root
filesystem is read-only and runtime browser downloads would be ephemeral,
unreviewed version drift.

## Compose topology

Run production preflight first:

```bash
./scripts/preflight-single-server.sh .env.single-server
```

When stdio is enabled, include the profile:

```bash
scripts/compose-single-server-production.sh .env.single-server \
  --profile app --profile mcp-runner up -d --no-build backend mcp-runner frontend
```

Expected Runner fences:

- non-root runtime UID/GID and non-root image user;
- read-only root filesystem;
- `cap_drop: ALL` and `no-new-privileges`;
- 1 CPU, 768 MiB memory, and 256 PID limit;
- isolated 512 MiB `exec,nosuid,nodev` `/work` and 64 MiB `noexec` `/tmp`
  tmpfs mounts (`/work` requires `exec` only for the exact downloaded package
  bin; Docker otherwise defaults tmpfs to `noexec`);
- no host port;
- only `mcp-control` (internal) and `mcp-egress` networks;
- no PostgreSQL, Redis, MinIO, provider-vault, or Docker-socket access.

The backend joins `mcp-control` but a Runner outage does not fail global backend
readiness. Only sends selecting a stdio server fail closed.

## Process lifecycle

The Runner accepts only bearer-authenticated internal calls to
`/internal/v1/tools/list` and `/internal/v1/tools/call`. Static IDs must exist in
the validated manifest. A dynamic ID must carry an AES-GCM-sealed artifact
bound to the exact Server and instance IDs; required environment values travel
in a separate sealed envelope. Runner revalidates both before it starts a child.
It uses an isolated mode-`0700` work directory, dedicated HOME/TMP/npm cache, a
minimal environment, direct argv execution, and a separate process group.

At most four child servers are retained. Default idle reap is 15 minutes and
maximum lifetime is 24 hours; an artifact may set lower manifest bounds. Each
installed or run-scoped Server is keyed separately, and a credential
fingerprint change replaces the old child. At capacity, the least-recent idle
child is reaped before a new instance starts; an active call is never evicted.
Crash, cancellation, expiry, and Runner shutdown terminate the whole process
group and remove the work directory. A later call may start a clean approved
child.

A cold dynamic npm child has a bounded two-minute download/initialize window.
The internal HTTP server carries a small envelope beyond that child-start bound;
steady-state Tool calls retain the normal shorter MCP call timeout. Installation
that cannot initialize inside the cold-start bound stays recoverable and never
becomes selectable.

## Release order

1. Create and verify a paired PostgreSQL/MinIO `pre-deploy` backup.
2. Validate the target manifest and Runner token metadata.
3. Build/pull backend, Runner, frontend, and RAG images; record all digests.
4. Run migrations `074`-`081` explicitly while old application writers are
   stopped.
5. Start the backend with MCP kill switches still off and verify `/ready`.
6. If needed, start the Runner and verify its container health internally.
7. Start the frontend.
8. Enable `MCP_ENABLED`; enable remote and/or stdio transport separately.
9. Smoke one approved read-only server, selection, native continuation, and
   persisted timeline. Do not use a write Tool as the first live smoke.

## Backup, deletion, and retention

The PostgreSQL dump contains all MCP rows. The MinIO mirror contains
`mcp-results/` artifacts. They must be captured and restored as one set. The
restore drill exports sample keys from `mcp_tool_results.object_keys` and
requires `mc stat` to succeed in the temporary bucket.

Conversation deletion removes MCP objects before marking conversation data
deleted. Account deletion writes object coordinates to
`mcp_artifact_cleanup_queue` before FK cascade; the cleanup-only worker later
removes bytes and acknowledges the queue. Never manually delete queue rows to
make cleanup appear healthy.

`MCP_AUDIT_RETENTION` defaults to `2160h` and accepts 24h through 8760h.
`MCP_CLEANUP_INTERVAL` defaults to `1h` and accepts 1m through 24h. On partial
object-store failure, durable rows remain for an idempotent retry.

## Rollback

Fast behavioral rollback:

```text
MCP_ENABLED=false
MCP_REMOTE_ENABLED=false   # optional narrower isolation
MCP_STDIO_ENABLED=false    # optional narrower isolation
```

Recreate only the backend and stop the optional Runner when stdio is disabled.
The cleanup-only worker must remain available through the backend process.

For an image rollback, restore the previous backend/frontend/Runner digests but
retain migrations `074`-`081`, their runtime grants, and all MCP rows. MCP
credential and stdio downs refuse while dependent rows exist; do not delete
installed Servers merely to force them. The old
Plugin runtime remains removed; MCP switches never reactivate it. Do not run
`074.down` after any live MCP selection, credential, call, result, or artifact
exists. Prefer a forward fix.

## Safe diagnostics

Safe evidence: service health, low-cardinality metrics, counts grouped by
transport/outcome/error class, call IDs, run IDs, bounded error codes, and
Runner process counts.

Never log or paste: Tool arguments/results, tokens, OAuth codes/state, custom
endpoint paths, credential headers, manifest secret values, object bytes, or
high-cardinality user/server/tool labels.
