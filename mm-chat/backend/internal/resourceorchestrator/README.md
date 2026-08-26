# Resource Orchestrator

The Resource Orchestrator is the authenticated control plane used by Agent
capability recovery and conversational install requests. It projects installed Skills and
MCP Servers, performs bounded Marketplace discovery, and delegates every write
to the existing `skillsupply.Service` or `mcpclient.Service` authority.

## Responsibilities

- Return a sanitized, deterministic Resource catalog for one conversation.
- Search admitted Skills and approved MCP Marketplace deployments with bounded
  result counts and immutable revision identities.
- Resolve allowlisted LobeHub Skill/MCP links to bounded Marketplace searches,
  then install an exact candidate without accepting arbitrary URLs,
  package commands, or credentials from the model.
- Coordinate MCP Secret/OAuth/Runner configuration through an existing
  provenance-bound private Server draft.
- Keep inventory installation separate from durable conversation selection;
  only the composer pickers persist selections.
- Route management writes through owner and CAS checks, then record
  action-specific mutation audits.
- Signal that the Chat Agent must create a fresh Runtime Resource Snapshot
  before using a changed resource set.

## Dependencies

- `skillsupply.Service`: admitted package detail, install, library, and removal.
- `mcpclient.Service`: Marketplace, private Server, credential readiness,
  validation, and conversation selection authority.
- PostgreSQL `audit_logs`: sanitized mutation outcomes in normal server mode.
- `auth` middleware: the HTTP layer derives the user from request context.

## HTTP usage

```text
GET  /v1/resources?conversationId=<uuid>
GET  /v1/resources/search?kind=skill|mcp&q=<query>
POST /v1/resources/install
POST /v1/resources/mutate
```

Callers must use the exact candidate revision returned by search. All responses
are `Cache-Control: no-store`; response schemas contain no Secret, endpoint,
package body, host path, or raw Tool arguments. A supported LobeHub URL is
parsed only into an identifier and never fetched as installation input.

## Files

- `service.go`: shared contracts plus catalog and search projection.
- `install.go`: exact install and MCP configuration completion.
- `mutation.go`: enable/disable/remove, CAS, and common mutation audit flow.
- `handler.go`: strict authenticated HTTP contract.
- `audit.go`: metadata-only PostgreSQL audit adapter.
- `*_test.go`: authority, drift, configuration, audit, and strict-input tests.

See [DESIGN.md](DESIGN.md) and the product-level
[Resource orchestration contract](../../../docs/contracts/resource-orchestration.md).
