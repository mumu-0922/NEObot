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
- Route an exact GitHub Skill tree or `SKILL.md` blob link to the `skillsupply`
  owner-private direct adapter without Store review or publication.
- Route an explicit AIHero Skill link to the `skillsupply` owner-private direct
  adapter as a backward-compatible path.
- Parse exactly one allowlisted link from an explicit human install request so
  Chat can perform the same bounded search without depending on model uptime.
- Coordinate MCP Secret/OAuth/Runner configuration through an existing
  provenance-bound private Server draft.
- Keep inventory installation separate from durable conversation selection;
  only the composer pickers persist selections.
- Route management writes through owner and CAS checks, then record
  action-specific mutation audits.
- Signal that the Chat Agent must create a fresh Runtime Resource Snapshot
  before using a changed resource set.

## Dependencies

- `skillsupply.Service`: curated catalog, admitted compatibility detail,
  owner-private direct install, library, and removal.
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
parsed only into a Marketplace identifier. An exact GitHub Skill directory or
`SKILL.md` URL enters only the fixed direct-source adapter. The legacy AIHero
URL adapter parses but never executes its command. Both pin the GitHub package
to an exact commit.
Multiple, query-bearing, fragment-bearing, or unsupported links do not enter
the deterministic install path.

## Files

- `service.go`: shared contracts plus catalog and search projection.
- `resource_link.go`: bounded exact-link parsing and discovery aliases.
- `install.go`: exact install and MCP configuration completion.
- `mutation.go`: enable/disable/remove, CAS, and common mutation audit flow.
- `handler.go`: strict authenticated HTTP contract.
- `audit.go`: metadata-only PostgreSQL audit adapter.
- `*_test.go`: authority, drift, configuration, audit, and strict-input tests.

See [DESIGN.md](DESIGN.md) and the product-level
[Resource orchestration contract](../../../docs/contracts/resource-orchestration.md).
