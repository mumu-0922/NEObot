# MCP Runner Operations Contract

## Scenario: Deploy MCP and the optional stdio Runner

### 1. Scope / Trigger

Apply this contract when changing MCP environment/preflight, manifest, Runner
image/service, release tooling, backup/restore, retention, or rollback.
Runtime paths `.env.single-server`, `data/`, `secrets/`, and `backup/` remain
protected.

### 2. Signatures

```bash
mm-chat-mcp-validate --manifest /etc/mm-chat/mcp-manifest.json
scripts/release-images.sh --tag <release> [--push|--load|--dry-run]
scripts/preflight-single-server.sh .env.single-server
scripts/compose-single-server-production.sh .env.single-server \
  --profile app --profile mcp-runner up -d --no-build
```

Images are `BACKEND_IMAGE`, `MCP_RUNNER_IMAGE`, `FRONTEND_IMAGE`, and
`RAG_IMAGE`. Production Runner URL is exactly `http://mcp-runner:8090`.

### 3. Contracts

- `mcp-runner` is one optional resident container; approved stdio children are
  started on demand, at most four, reaped after 15 idle minutes, and capped at
  24 hours.
- The image runs non-root with a read-only root, `cap_drop: ALL`,
  `no-new-privileges`, 0.5 CPU, 256 MiB memory, 128 PIDs, isolated tmpfs, and no
  host port. It joins only internal `mcp-control` plus `mcp-egress`, never the
  PostgreSQL/MinIO private network.
- Production `MCP_RUNNER_IMAGE` is a full immutable digest and has no Compose
  `build:`. Release tooling builds Runner via Dockerfile target `mcp-runner`.
- Runner auth uses a dedicated Docker Secret source: regular non-symlink,
  runtime-owner-owned, mode `0600`, 32..4096 bytes. Never reuse a user bearer or
  provider/RAG credential.
- Manifest schema v1 is strict. Stdio argv[0] is absolute and not a shell;
  runtime downloads, working-directory overrides, unsafe env names, Docker
  socket, and arbitrary host mounts are forbidden.
- Backup always pairs a full PostgreSQL dump with a full MinIO bucket mirror.
  This includes MCP rows and `mcp-results/`. Restore drills export both
  Knowledge and MCP sample keys and `mc stat` them in a temporary bucket.
- Release order is backup -> validate/build/pull -> explicit migration ->
  backend with MCP off -> optional Runner -> frontend -> enable/smoke.
- Rollback disables MCP/transports or restores prior image digests while
  retaining migration `074`. Cleanup remains running; never revive Plugins or
  down-migrate after live traffic.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| stdio enabled while MCP disabled | preflight rejects |
| Runner image is tag/invalid digest | production preflight rejects |
| Runner URL differs from internal service URL | preflight rejects |
| Token source is symlink/wrong owner/mode/size | preflight rejects without printing content |
| Manifest invalid | validator fails closed; unrelated chat may start with MCP unavailable |
| Runner health fails | selected stdio runs fail closed; backend global readiness remains independent |
| Restore lacks MCP sample file/object | temporary-bucket drill fails before production restore |
| Partial artifact cleanup fails | retain row/queue entry and retry; never delete DB authority first |

### 5. Good / Base / Bad Cases

- **Good**: render four pinned images, validate manifest/token metadata, migrate,
  start backend and optional Runner, then smoke one read-only Tool.
- **Base**: MCP disabled and no Runner profile; cleanup still prunes expired
  durable state and artifacts.
- **Bad**: run `npx` in the Runner, mount Docker socket/source, publish 8090, use
  a mutable tag, or run migration `074.down` after traffic.

### 6. Tests Required

- `go test ./cmd/mcp-validate ./cmd/mcp-runner ./internal/mcprunner`.
- Build target `mcp-runner`; inspect non-root user and entrypoint.
- Render example and production Compose with Runner profile; assert no port,
  hardening/resources/networks, digest, and cleared production build.
- `bash scripts/test-preflight-single-server.sh` for toggles, duration bounds,
  token metadata, Runner URL/image, topology, and restore mounts.
- `bash scripts/verify-mcp-postgres17.sh` for migration/repository cleanup.
- `scripts/release-images.sh --dry-run --tag <test>` must print four builds and
  `--target mcp-runner` only for Runner.

### 7. Wrong vs Correct

#### Wrong

```yaml
mcp-runner:
  image: node:latest
  ports: ["8090:8090"]
  volumes: ["/var/run/docker.sock:/var/run/docker.sock"]
```

#### Correct

```yaml
mcp-runner:
  image: registry/runner@sha256:<digest>
  read_only: true
  cap_drop: [ALL]
  security_opt: [no-new-privileges:true]
  networks: [mcp-control, mcp-egress]
```
