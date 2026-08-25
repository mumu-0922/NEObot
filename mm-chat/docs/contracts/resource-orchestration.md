# Conversational Resource Orchestration

## Scope

Neo Chat exposes one server-authorized discovery and install plane for admitted
Skill packages and MCP Marketplace entries. Deterministic slash commands and
the Agent `resource_search` / `resource_request_install` Tools call the same
orchestrator. The orchestrator delegates every write to `skillsupply.Service`
or `mcpclient.Service`; it never installs with Bash, npm, Git, Docker, or an
arbitrary URL.

## HTTP contract

All routes require the existing authenticated user context and return
`Cache-Control: no-store`.

```text
GET  /v1/resources?conversationId=<uuid>
GET  /v1/resources/search?kind=skill|mcp&q=<bounded query>
POST /v1/resources/install
```

The catalog contains a deterministic `sha256:` revision, installed Skills, and
sanitized MCP readiness/selection state. Search returns at most five entries.
Each entry binds `id`, `version`, and `exactRevision`; descriptions,
permissions, and Marketplace metadata are untrusted routing data.

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
```

The current Run snapshot is never mutated in place. A successful mutation
creates a new segment and the trace records the previous/current snapshot
revisions plus the mutation audit ID. The model must not claim the newly
installed resource was used unless it calls a Tool exposed by the fresh
snapshot.

MCP deployments requiring Header Secret, OAuth, Runner environment, custom
endpoint, or failed validation return `RESOURCE_CONFIGURATION_REQUIRED` and
must be completed in the Tools Marketplace. Secret values never enter Agent
Tool schemas or process trace.

## Slash commands

The composer parses commands without an LLM:

```text
/resources                     /reload
/skill                          /skill search <query>
/skill info <name-or-id>        /skill install <candidate-id>
/skill remove <name-or-installation-id>
/skill:<installed-name> [args]
/mcp                            /mcp search <query>
/mcp info <identifier>          /mcp status
/mcp install <identifier>       /mcp enable|disable|remove <name-or-ref>
```

`/skill:<name>` is the canonical deterministic invocation. Historical
`/skill-name` messages remain replay-compatible in the Backend. Running `/reload`
during generation is rejected because it would violate Step consistency.

## Failure contract

| Condition | Result |
| --- | --- |
| malformed/unknown fields | `400 INVALID_RESOURCE_REQUEST` |
| unsupported kind/query | `400 INVALID_RESOURCE_QUERY` |
| candidate version/revision changed | `409 RESOURCE_REVISION_CHANGED` |
| Secret/OAuth/Runner/config required | `409 RESOURCE_CONFIGURATION_REQUIRED` |
| rollout switch disabled | `503 RESOURCE_ORCHESTRATION_DISABLED` |
| audit cannot be persisted after mutation | `503 RESOURCE_AUDIT_UNAVAILABLE`; inspect library/audit before retry |
| source or delegated write unavailable | `503 RESOURCE_*_UNAVAILABLE` / `RESOURCE_INSTALL_FAILED` |
| snapshot refresh fails | Agent Run stops with `RESOURCE_REFRESH_FAILED`; no false continuation |

## Security and rollout

- `RESOURCE_ORCHESTRATION_ENABLED=true` enables Agent discovery/install and
  the unified install route. Set it to `false` to remove Agent resource Tools
  and reject unified writes while keeping existing Skill/MCP management pages.
- Skill Store admission/fingerprint and MCP administrator, endpoint,
  credential, deployment-hash, validation, selection-revision, and ownership
  checks remain authoritative.
- Agent discovery is bounded and deduplicated; a candidate not returned by the
  current Run, a stale revision, a second proposal, or an unavailable runtime
  kind fails closed.
- Agent-initiated installation needs the durable approval runtime. When the
  timeline canary is not eligible, the request fails immediately rather than
  creating an invisible wait.

## Operator diagnostics

```sql
SELECT id, actor_user_id, conversation_id, resource_type, outcome,
       metadata, created_at
FROM audit_logs
WHERE action = 'resource.install'
ORDER BY created_at DESC
LIMIT 50;
```

For `RESOURCE_AUDIT_UNAVAILABLE`, compare the exact candidate revision with the
current Skill library or MCP private-server/selection state before retrying.
For `RESOURCE_REFRESH_FAILED`, the mutation audit is still authoritative, but
the failed Run must not be treated as having used the installed resource.
