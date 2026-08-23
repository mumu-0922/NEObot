## Bug Analysis: Exact URL unreadable and partial Web success misreported

### 1. Root Cause Category

- **Category**: B/D/E — Cross-layer contract, test coverage gap, and implicit
  egress assumption.
- **Specific Cause**: The Agent exposed keyword Search but no exact-URL Tool.
  Turn diagnostics treated any failed Web call as total degradation even when
  other calls had already produced sources. Development commands also appeared
  able to read Linux.do because they inherited a host proxy, while the hardened
  Backend intentionally disabled ambient proxies and could not reach that host
  directly.

### 2. Why Fixes Failed

1. Keyword Search: it finds related indexed pages but is not an exact document
   reader and one upstream call reached the 30-second timeout.
2. Direct safe reader alone: fixtures passed, but the real Backend egress path
   timed out because host proxy access was mistaken for direct reachability.
3. Global proxy enablement was rejected: it would let proxy-side DNS resolution
   escape the Backend's checked-address binding and reopen SSRF risk.

### 3. Prevention Mechanisms

| Priority | Mechanism | Specific Action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Add `read_web_url` with selected-provider Extract and safe direct fallback | DONE |
| P0 | Runtime security | Keep ambient proxy disabled; validate public URL before either path | DONE |
| P0 | Test coverage | Add Discourse, HTML, blocked URL, provider Extract, persistence, and partial-result tests | DONE |
| P1 | Diagnostics | Derive `partial` from cumulative sources plus any failed call | DONE |
| P1 | Documentation | Record host-vs-container egress proof and exact fallback contract | DONE |

### 4. Systematic Expansion

- **Similar Issues**: Any provider, webhook, MCP, media, or crawler smoke can
  pass from a developer shell through a proxy while failing inside the actual
  container.
- **Design Improvement**: Treat keyword search, exact extraction, and browser
  automation as distinct capabilities with distinct authority.
- **Process Improvement**: Every new outbound path needs a no-proxy proof from
  the deployed caller, not just a host-shell `curl`.

### 5. Knowledge Capture

- [x] Updated backend Tool Loop and source-fusion specs.
- [x] Updated the cross-layer egress checklist.
- [x] Updated Web Search module README/DESIGN and public contract.
- [x] Added deterministic regression tests and a deployed-path acceptance step.

