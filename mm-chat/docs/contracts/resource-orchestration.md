# Conversational Resource Orchestration

## Scope

Neo Chat exposes one server-authorized discovery and install plane for curated,
direct-source, or legacy admitted Skill packages and MCP Marketplace entries. The Agent
`resource_search` / `resource_request_install` Tools call the orchestrator;
Resource lifecycle Slash commands are not a product surface. The orchestrator delegates every write to `skillsupply.Service`
or `mcpclient.Service`; it never installs with Bash, npm, Git, Docker, or an
arbitrary URL.

## HTTP contract

All routes require the existing authenticated user context and return
`Cache-Control: no-store`.

```text
GET  /v1/resources?conversationId=<uuid>
GET  /v1/resources/search?kind=skill|mcp&q=<bounded query>
POST /v1/resources/install
POST /v1/resources/mutate
```

The catalog contains a deterministic `sha256:` revision, installed Skills, and
sanitized MCP readiness/selection state. Search returns at most five entries.
Each entry binds `id`, `version`, and `exactRevision`; descriptions,
permissions, and Marketplace metadata are untrusted routing data. An allowlisted
LobeHub Skill/MCP link is a search alias. An exact GitHub Skill tree link, or a
blob link ending in `SKILL.md`, enters the server-owned direct Skill adapter.
The adapter requires one unambiguous Skill directory, pins mutable refs to a
40-character commit, and passes that directory through the existing canonical
ZIP/SBOM validator. The legacy AIHero `/skills-<slug>` adapter remains as a
compatibility path; its bounded page is parsed as data and its command is never
executed. Unknown hosts, repository roots, query strings, fragments, traversal,
and kind mismatches stay untrusted chat/search text.

When one human message has explicit install intent and exactly one supported
link, the Backend acts before any model request. The scanner accepts at most
16 KiB of text and rejects multiple URLs. Exact GitHub Skill links and legacy
AIHero Skill links install directly into the current owner's private library
without Store search, review, or publication. Other supported links retain
bounded `resource_search` and exact admitted-candidate installation. Neither
path invokes Shell, npm, Git, Docker, or a fallback Provider-generated
installer.

```json
{
  "kind": "skill",
  "id": "candidate-id",
  "version": "1.0.0",
  "exactRevision": "sha256:...",
  "conversationId": "conversation-uuid"
}
```

A successful install returns `refreshRequired=true` and, in the normal
PostgreSQL runtime, a `mutationAuditId`. The audit row records user,
conversation, entry point, immutable candidate identity, safe outcome, and the
installed resource ID. It never stores credentials, endpoint secrets, package
content, Tool arguments, or host/cache paths.

Lifecycle mutations use the same plane instead of calling domain endpoints
from slash-command code:

```json
{
  "kind": "mcp",
  "action": "enable",
  "id": "catalog:deepwiki",
  "expectedRevision": 3,
  "conversationId": "conversation-uuid"
}
```

Skill removal binds the installation revision. MCP enable/disable binds the
conversation selection revision. Private MCP removal reauthorizes the owner
and management capability. Every outcome records `resource.remove`,
`resource.enable`, or `resource.disable` with metadata-only audit fields.

## Agent state machine

```text
resource_search (max 2 unique queries, max 5 entries each)
  -> exact candidate retained only for this Run
  -> resource_request_install (max 1)
  -> explicit user install: existing policy may proceed directly
     agent-discovered install: durable Chat Agent approval required
  -> Skill/MCP authority revalidates admission, auth, revision, owner and CAS
  -> mutation audit
  -> fresh Runtime Resource Snapshot
  -> same Provider/model continues the original task

explicit human install + exactly one supported link
  -> GitHub Skill: exact directory -> exact GitHub commit -> private validate/install
  -> AIHero Skill: bounded page parse -> same GitHub direct path
  -> other link: Backend resource_search -> one exact admitted candidate
  -> next Agent task receives the changed inventory
```

For an approved Marketplace deployment that requires Header Secret, OAuth, or
Runner environment configuration, the install authority first creates or
recovers the exact provenance-bound private Server draft. The Agent emits an
open configuration card and waits on the durable approval channel. The card
opens and targets the provenance-bound draft in the existing Tools installed
view; secrets still travel directly from that UI to the Backend vault. When the
user selects **Configured, continue**, the
Backend re-fetches the exact Marketplace revision, checks draft ownership and
provenance metadata, requires `ready` plus credential presence, records a
second mutation audit, and only then creates
the fresh Runtime Resource Snapshot.

The current Run snapshot is never mutated in place. A successful mutation
creates a new segment and the trace records the previous/current snapshot
revisions plus the mutation audit ID. The model must not claim the newly
installed resource was used unless it calls a Tool exposed by the fresh
snapshot.

MCP deployments requiring Header Secret, OAuth, or Runner environment use the
configuration handoff above. A custom endpoint, unsupported deployment, failed
validation, incomplete setup, or expired/denied handoff fails closed. Secret
values never enter Agent Tool schemas, process trace, mutation audit, or model
context.

## Inventory, conversation selection, and Run activation

- Skill Store and Tools own inventory installation, configuration, health and
  removal.
- Two compact composer pickers own durable per-conversation Skill and MCP
  selection. Both use Backend revision/CAS authority and reload on conversation
  switch or browser refresh.
- Agent mode may add at most two relevant, already-installed and authorized
  resources for one Run. These entries are tagged `agent_auto` in the frozen
  snapshot and never write back to the conversation selection.
- A successful conversational install changes inventory only. The refreshed
  continuation may use it run-only; persistent reuse requires the user to select
  it in the composer.
- Historical Skill invocation messages remain replay-compatible, but lifecycle
  Slash commands are absent from the new-input palette.

## Failure contract

| Condition | Result |
| --- | --- |
| malformed/unknown fields | `400 INVALID_RESOURCE_REQUEST` |
| lifecycle owner/scope denied | `403 RESOURCE_MUTATION_FORBIDDEN` |
| unsupported kind/query | `400 INVALID_RESOURCE_QUERY` |
| candidate version/revision changed | `409 RESOURCE_REVISION_CHANGED` |
| Secret/OAuth/Runner/config required | `409 RESOURCE_CONFIGURATION_REQUIRED` |
| rollout switch disabled | `503 RESOURCE_ORCHESTRATION_DISABLED` |
| audit cannot be persisted after mutation | `503 RESOURCE_AUDIT_UNAVAILABLE`; inspect library/audit before retry |
| source or delegated write unavailable | `503 RESOURCE_*_UNAVAILABLE` / `RESOURCE_INSTALL_FAILED` |
| snapshot refresh fails | Agent Run stops with `RESOURCE_REFRESH_FAILED`; no false continuation |
| supported explicit link has no exact admitted candidate | completed no-install answer; zero Provider/install calls |
| GitHub URL is a repository root, malformed, ambiguous, mutable after pinning, or invalid | completed direct-install failure; zero Provider/package execution |
| AIHero command/source/name is absent, ambiguous, mutable, or invalid | completed direct-install failure; zero Provider/package execution |
| explicit text has multiple/unsafe/unsupported links | no deterministic mutation; ordinary Agent handling |

## Security and rollout

- `RESOURCE_ORCHESTRATION_ENABLED=true` enables Agent discovery/install and
  the unified install route. Set it to `false` to remove Agent resource Tools
  and reject unified writes while keeping existing Skill/MCP management pages.
- Curated catalog commit/fingerprint, legacy Skill Store admission/fingerprint,
  owner-private direct-source fingerprint,
  and MCP administrator, endpoint,
  credential, deployment-hash, validation, selection-revision, and ownership
  checks remain authoritative.
- Agent discovery is bounded and deduplicated; a candidate not returned by the
  current Run, a stale revision, a second proposal, or an unavailable runtime
  kind fails closed.
- Agent-initiated installation needs the durable approval runtime. When the
  timeline canary is not eligible, the request fails immediately rather than
  creating an invisible wait.
- Provider availability is not part of deterministic explicit-link authority.
  Provider failure cannot broaden Store admission, private ownership, or URL
  authority.

## Operator diagnostics

```sql
SELECT id, actor_user_id, conversation_id, resource_type, outcome,
       metadata, created_at
FROM audit_logs
WHERE action IN ('resource.install', 'resource.enable', 'resource.disable', 'resource.remove')
ORDER BY created_at DESC
LIMIT 50;
```

For `RESOURCE_AUDIT_UNAVAILABLE`, compare the exact candidate revision with the
current Skill library or MCP private-server/selection state before retrying.
For `RESOURCE_REFRESH_FAILED`, the mutation audit is still authoritative, but
the failed Run must not be treated as having used the installed resource.
