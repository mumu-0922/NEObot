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
  `no-new-privileges`, 1 CPU, 768 MiB memory, 256 PIDs, isolated tmpfs, and no
  host port. It joins only internal `mcp-control` plus `mcp-egress`, never the
  PostgreSQL/MinIO private network.
- Production `MCP_RUNNER_IMAGE` is a full immutable digest and has no Compose
  `build:`. Release tooling builds Runner via Dockerfile target `mcp-runner`.
- Runner auth uses a dedicated Docker Secret source: regular non-symlink,
  runtime-owner-owned, mode `0600`, 32..4096 bytes. Never reuse a user bearer or
  provider/RAG credential.
- Manifest schema v1 is strict. Static stdio argv[0] is absolute and not a
  shell; working-directory overrides, unsafe env names, Docker socket, and
  arbitrary host mounts are forbidden. A dynamic Marketplace artifact is the
  only runtime-download exception and is never sourced from public DTO or
  browser command fields.
- Backend, validator, and Runner compile the same strict manifest parser. Any
  new manifest field therefore requires paired Backend and Runner images from
  the same source revision; do not mount a newer manifest into an older Runner
  and assume its current health proves restart compatibility.
- A static Marketplace stdio approval is an exact manifest fingerprint over
  provider, identifier/version, connection/install method, upstream command/
  arguments/package name, and deployment hash; the manifest's separate
  absolute argv remains its execution authority. Otherwise Backend may derive
  a dynamic artifact only from a freshly fetched `npx` deployment and an exact
  npm-registry SemVer. It persists the bounded artifact on the administrator-
  owned Server and sends it only as an AES-GCM envelope bound to Server and
  credential-fingerprint instance IDs.
- Image-bundled Node artifacts remain fixed in
  `backend/mcp-runner-runtime/package-lock.json` and installed at image build by
  `npm ci --omit=dev --ignore-scripts`. Dynamic artifacts use only the Runner-
  owned `/usr/local/bin/npx --yes <package>@<exact-version>` launcher inside a
  per-Server work/cache directory. npm lifecycle code can execute there, which
  is why only the deployment administrator may install and every container,
  network, process, resource, and cleanup fence is mandatory.
- Backup always pairs a full PostgreSQL dump with a full MinIO bucket mirror.
  This includes MCP rows and `mcp-results/`. Restore drills export both
  Knowledge and MCP sample keys and `mc stat` them in a temporary bucket.
- Release order is backup -> validate/build/pull -> explicit migration ->
  backend with MCP off -> optional Runner -> frontend -> enable/smoke.
- Rollback disables MCP/transports or restores prior image digests while
  retaining migrations `074`-`081`. Cleanup remains running; never revive
  Plugins or down-migrate after live traffic. `076.down` must reject while any
  private stdio row exists.
- Marketplace discovery is an optional backend adapter, not another service or
  Runner container. It is disabled by default. Its M2M client secret is read
  from a dedicated Docker Secret file; startup never registers a LobeHub
  identity and Marketplace outage never changes backend readiness.
- Production config bounds Marketplace timeout/cache TTL and permits only the
  configured HTTPS LobeHub-compatible base URL. Operators register/rotate the
  third-party M2M identity explicitly and may disable Marketplace without
  affecting installed private MCP Servers.
- Runner resource values form one release contract across
  `compose.single-server.yml`, `docs/deployment/mcp-runner.md`, and
  `scripts/test-preflight-single-server.sh`. Change all three together; a
  health check does not prove the intended CPU, memory, PID, or tmpfs limits.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| stdio enabled while MCP disabled | preflight rejects |
| Runner image is tag/invalid digest | production preflight rejects |
| Runner URL differs from internal service URL | preflight rejects |
| Token source is symlink/wrong owner/mode/size | preflight rejects without printing content |
| Manifest invalid | validator fails closed; unrelated chat may start with MCP unavailable |
| Manifest contains a field unknown to the running Runner image | Runner restart fails closed; deploy the paired Runner image before declaring the manifest rollout complete |
| Runner health fails | selected stdio runs fail closed; backend global readiness remains independent |
| Restore lacks MCP sample file/object | temporary-bucket drill fails before production restore |
| Partial artifact cleanup fails | retain row/queue entry and retry; never delete DB authority first |
| Marketplace enabled without client ID/valid secret file | preflight rejects before deployment without printing secret content |
| Marketplace unavailable | Marketplace UI fails closed; MCP Server management and chat remain healthy |
| Marketplace stdio is neither an exact manifest artifact nor a bounded exact-version dynamic npm artifact | keep it incompatible/Runner-required; do not install or execute upstream command |
| Approved artifact is removed/changed after install | private stdio validation/selection/execution fails closed |
| `076.down` sees a private stdio row | rollback aborts atomically; preserve row and migration |
| Compose, deployment docs, and preflight disagree on Runner resources | release check fails; synchronize the three authorities before deployment |
| A tail migration is added without advancing both MCP PostgreSQL drills | focused release gate fails before deployment |

### 5. Good / Base / Bad Cases

- **Good**: render four pinned images, validate manifest/token metadata, migrate,
  start backend and optional Runner, then smoke one approved Tool; dynamic npm
  execution uses an exact Server-bound artifact and isolated work directory.
- **Base**: MCP disabled and no Runner profile; cleanup still prunes expired
  durable state and artifacts.
- **Bad**: execute a browser-selected/floating `npx` package, mount Docker
  socket/source, publish 8090, use a mutable tag, or down-migrate after traffic.

### 6. Tests Required

- `go test ./cmd/mcp-validate ./cmd/mcp-runner ./internal/mcprunner`.
- For a manifest schema change, restart the newly built Runner against the
  exact target manifest and require healthy status; a still-running old
  container is not compatibility evidence.
- Build target `mcp-runner`; inspect non-root user and entrypoint.
- Run the pinned image dependency audit against the official npm registry.
  Dynamic coverage must reject Shell/Docker/Git/URL/file/floating package
  specs, accept only exact registry package versions, verify sealed artifact
  binding, and prove per-Server process/workspace cleanup.
- Render example and production Compose with Runner profile; assert no port,
  hardening/resources/networks, digest, and cleared production build.
- `bash scripts/test-preflight-single-server.sh` for toggles, duration bounds,
  token metadata, Runner URL/image, topology, and restore mounts.
- `bash scripts/verify-mcp-postgres17.sh` and
  `bash scripts/verify-mcp-install-credentials-postgres17.sh` for fresh
  `001..081`, tail down/re-up, guarded `076`, targeted repairs, repository, and
  final replay. Every new tail migration must update both scripts.
- `scripts/release-images.sh --dry-run --tag <test>` must print four builds and
  `--target mcp-runner` only for Runner.
- Preflight tests cover Marketplace toggle dependency, HTTPS base URL, bounded
  timeout/cache TTL, and dedicated regular secret-file metadata/content bounds.

### 7. Wrong vs Correct

#### Wrong

```json
{
  "identifier": "example-mcp",
  "version": "latest",
  "command": ["sh", "-c", "npx arbitrary-package"]
}
```

This lets the browser select mutable execution authority.

#### Correct

```json
{
  "version": "2.2.0",
  "deploymentHash": "<backend-issued-exact-hash>",
  "secrets": {}
}
```

Backend re-fetches Marketplace detail, resolves an exact registry version, and
either binds a static manifest executable or sends a Server-bound sealed
dynamic artifact to the Runner. The public request contains no command.

#### Wrong topology

```yaml
mcp-runner:
  image: node:latest
  ports: ["8090:8090"]
  volumes: ["/var/run/docker.sock:/var/run/docker.sock"]
```

#### Correct topology

```yaml
mcp-runner:
  image: registry/runner@sha256:<digest>
  read_only: true
  cap_drop: [ALL]
  security_opt: [no-new-privileges:true]
  networks: [mcp-control, mcp-egress]
```
