# Egress and Secret Broker Boundaries

## Evidence

- Existing Neo safe transport:
  `backend/internal/mcpclient/safehttp.go` and its tests.
- OWASP SSRF Prevention Cheat Sheet, captured 2026-08-13:
  <https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html>.
- HashiCorp Vault response wrapping, captured 2026-08-13:
  <https://developer.hashicorp.com/vault/docs/concepts/response-wrapping>.
- Frozen Neo Grant and deployment contracts:
  `docs/contracts/schemas/neo-capability-grant.schema.json` and
  `docs/deployment/agent-runtime.md`.

External material is untrusted design evidence, not an implementation command.

## Egress findings

An allowlisted hostname is insufficient: the Broker must validate syntax,
resolve every address, reject non-global and special ranges, and use only the
validated addresses for the connection. Redirects and reconnects are new
authorization points. Credential headers must be stripped on origin change.
Request bytes, response bytes, headers, redirects, wall time and concurrency
must all be bounded.

`mcpclient.NewSafeHTTPClient` already implements useful DNS/IP and same-origin
header controls, but its policy is tailored to current MCP transport. G20.4
should extract a shared internal safe-network primitive rather than copying the
blocklist. Agent Broker policy adds exact Grant host+scheme+port matching,
disallows userinfo/fragments/raw IPs and defaults to HTTPS/WSS only.

Modes remain:

- `none`: no socket or request path is usable;
- `allowlist`: a bounded generic HTTPS request is constructed from a strictly
  decoded Broker request and exact frozen rules;
- `brokered`: a registered Tool executor constructs the outbound request, so
  the Sandbox cannot choose arbitrary URL/header/credential material.

## Secret findings

The safest handle is single-action, short-lived, non-renewable, bound to the
subject, Run/Step/Attempt generation, capability/action, exact destination and
intent fingerprint. The opaque handle is not the secret and is useless outside
the Broker. When a protocol permits it, the secret never crosses the Broker
boundary: the Broker applies it to the outbound request and strips it from the
response.

The durable database stores only `secret_ref`, handle digest, expiry, binding
fingerprints and sanitized state. Vault bytes and plaintext handles are never
persisted. An in-memory resolver interface supplies bytes only at action time;
the default/fake resolver used by source tests contains synthetic canaries.

## Zero-leak acceptance

A synthetic canary must not appear in:

```text
Runner environment or argv
/proc-facing command material
prompt/model context
Workspace or retained Scratch
Artifact quarantine/publication
Tool result/error
PostgreSQL rows/events
logs, metrics or diagnostic reports
```

Terminal, kill, lease reclaim, intent expiry and Commit completion revoke every
handle. Redaction is defense in depth; preventing secret return and persistence
is the primary control.
