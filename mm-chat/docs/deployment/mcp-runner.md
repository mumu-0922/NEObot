# MCP Tools and Runner Operations

## Default state

MCP is off by default. Remote MCP runs inside the Go backend. Administrator-
approved stdio servers use the optional `mcp-runner` Compose profile; do not
run one permanent container per Tool server.

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
      "command": {
        "argv": ["/usr/local/bin/example-mcp", "--stdio"],
        "idleTimeout": "15m",
        "maxLifetime": "24h"
      },
      "grants": [{ "scopeType": "global", "defaultEnabled": false }]
    }
  ]
}
```

Every executable and runtime dependency must already exist in the immutable
Runner image. Runtime `npx`, `uvx`, package downloads, `sh -c`, Docker socket
access, and host source mounts are forbidden. Environment values must come from
one exact literal for non-secrets, `/run/secrets/...`, or an allowlisted
`MCP_SECRET_*` reference; auth secrets may not be inline.

### Approved Marketplace artifacts

A Marketplace stdio option is installable only when one manifest entry carries
an exact `marketplace` projection matching provider, identifier, Marketplace
version, connection/install method, command, arguments, package name, and the
derived deployment hash. This projection is an approval fingerprint only. The
Runner executes the entry's separate absolute `command.argv`; it never executes
the Marketplace command.

The initial reviewed artifact maps LobeHub `upstash-context7@2.2.0`
(`npx ctx7`) to the image-bundled
`@upstash/context7-mcp@3.2.5` executable. The package and transitive artifacts
are pinned by `backend/mcp-runner-runtime/package-lock.json`, installed during
image build with `npm ci --omit=dev --ignore-scripts`, and audited before
release. Adding a store item requires reviewing and rebuilding this lock plus
the manifest; it never performs an install in a running container.

An installed user-private stdio row stores `runner://<approved-id>` and bounded
provenance. The public API hides both. The backend rechecks the current manifest
binding before validation, selection, and execution, so removing or changing an
artifact fails closed without trusting stale database metadata.

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
- 0.5 CPU, 256 MiB memory, and 128 PID limit;
- isolated `/work` and `/tmp` tmpfs mounts;
- no host port;
- only `mcp-control` (internal) and `mcp-egress` networks;
- no PostgreSQL, Redis, MinIO, provider-vault, or Docker-socket access.

The backend joins `mcp-control` but a Runner outage does not fail global backend
readiness. Only sends selecting a stdio server fail closed.

## Process lifecycle

The Runner accepts only independent bearer-authenticated internal calls to
`/internal/v1/tools/list` and `/internal/v1/tools/call`. Server IDs must already
exist in the validated manifest. It starts a child on first use, uses an
isolated mode-`0700` work directory, sets a minimal environment, and creates a
separate process group.

At most four child servers are active. Default idle reap is 15 minutes and
maximum lifetime is 24 hours. Crash, cancellation, expiry, and Runner shutdown
terminate the whole process group and remove the work directory. A later call
may start a clean approved child.

## Release order

1. Create and verify a paired PostgreSQL/MinIO `pre-deploy` backup.
2. Validate the target manifest and Runner token metadata.
3. Build/pull backend, Runner, frontend, and RAG images; record all digests.
4. Run migrations `074`-`076` explicitly while old application writers are
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
retain migrations `074`-`076`, their runtime grants, and all MCP rows. Migration
`076.down` refuses while stdio rows exist; do not delete installed Servers to
force it. The old
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
