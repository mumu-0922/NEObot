# MCP Security and Operations Findings

## Remote trust boundary

- A user-provided MCP URL is an SSRF input, and every redirect, metadata URL,
  authorization server, token endpoint, and reconnect is a new request target.
- Reuse the current plugin remote-dial protections only as a starting point:
  perform public-address validation on every resolution and redirect, pin the
  validated dial result, reject credential forwarding across origins, cap
  response headers/bodies, and use public HTTPS only for private user servers.
- MCP tool descriptions, schemas, annotations, errors, content, resource links,
  and notifications are untrusted data. They never become system instructions
  or authorization metadata without a trusted catalog/manifest override.

## Credential boundary

- User tokens belong in context-bound `providersecrets.Vault` envelopes under
  an MCP-specific AAD context. API responses expose only credential status.
- OAuth state rows store a hash, actor/server binding, redirect target, PKCE
  material envelope/reference, and expiry. The callback atomically consumes
  state before token exchange completion is committed.
- Refresh uses one per-credential singleflight/lock to prevent refresh-token
  races. Revocation removes ciphertext immediately and invalidates pooled
  sessions.
- Manifest secrets use deployment-owned `secretFile` or an allowlisted
  `envRef`; neither raw secret values nor resolved secrets appear in logs,
  metrics, validation output, or process arguments.

## Runner boundary

- One runner container avoids one always-resident container per MCP while its
  child processes remain on-demand and bounded.
- Multiple children in one container share its network/mount/cgroup boundary.
  This is acceptable only for administrator-approved artifacts. User-supplied
  local commands remain forbidden.
- The runner image must contain immutable reviewed runtimes/artifacts. Runtime
  package downloads and Docker socket access would defeat supply-chain and
  isolation controls.
- Put backend and runner control traffic on a dedicated internal network with a
  unique service secret. Give the runner outbound connectivity separately and
  do not join it to PostgreSQL/MinIO private networks.
- Process-group ownership, idle and maximum lifetime timers, queue capacity,
  cancellation escalation, output caps, and crash reaping require direct tests.

## Availability and retry boundary

- Runner or remote transport loss during a write has ambiguous outcome. It is
  never safe to retry automatically without an application-level idempotency
  guarantee from trusted metadata.
- Trusted idempotent reads may retry once only before a result is observed.
  Custom remote annotations default to unknown/write.
- Runner readiness is not a global backend readiness dependency. Backend chat
  remains available while selections that require the failed runner are
  blocked explicitly.
- Circuit breakers, quotas, and tool budgets protect both upstream services
  and Neo Chat from cost/resource amplification.

## Diagnostics and lifecycle

- Structured logs may carry request/run/call correlation IDs but no custom URL,
  arguments, results, headers, or tokens.
- Prometheus labels remain low-cardinality (`transport`, `outcome`,
  `error_class`, `read_write`). User/server/tool identity belongs only in the
  bounded authorized audit view.
- Inline results, MinIO artifacts, audit records, OAuth state, credentials, and
  conversation selection have distinct retention rules and must participate in
  account/conversation deletion, backup, restore, and rollback drills.
- Migrations remain additive during the first MCP release. Rollback uses kill
  switches or prior images and retains MCP tables; the plugin table is removed
  only after the rollback window closes.

