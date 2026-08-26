# Conversational Resource Orchestration Contract

## 1. Scope / Trigger

Apply when changing `/v1/resources*`, conversational Skill/MCP acquisition,
`resource_search`, `resource_request_install`, runtime snapshot refresh,
resource mutation audit, or `RESOURCE_ORCHESTRATION_ENABLED`.

This layer is an orchestrator, not a third package authority. Skill writes must
delegate to `skillsupply.Service`; MCP writes must delegate to
`mcpclient.Service`.

## 2. Signatures

```text
GET  /v1/resources?conversationId=<uuid>
GET  /v1/resources/search?kind=skill|mcp&q=<query>
POST /v1/resources/install
POST /v1/resources/mutate

resource_search({kind, query, capability})
resource_request_install({kind, id, version, exactRevision, reason})

RESOURCE_ORCHESTRATION_ENABLED=true|false
```

```go
type InstallRequest struct {
    Kind, ID, Version, ExactRevision string
    UserID, ConversationID, EntryPoint string
}

type InstallResult struct {
    Kind, ID, Name, Revision, Status string
    RefreshRequired bool
    MutationAuditID string
}

type MutationRequest struct {
    Kind, Action, ID string
    ExpectedRevision int64
    UserID, ConversationID, EntryPoint string
}
```

## 3. Contracts

- Catalog/search responses are `no-store`, authenticated, server-sanitized, and
  bounded. Search returns at most five results and exact immutable revision
  material; it never returns credentials, endpoints, package bodies, or paths.
- An explicitly supported discovery HTTPS link may be reduced to one bounded
  identifier only when host, path shape, kind, port, query, fragment, and
  identifier validation pass. LobeHub Skill/MCP links and AIHero
  `/skills-<slug>` Skill links are admitted shapes. AIHero is a discovery alias
  only: the pasted page is never fetched or executed, and installation still
  requires an existing admitted package fingerprint from `skillsupply.Service`.
- A caller must search before an Agent install. The Run retains at most two
  unique search results and one proposal. Candidate metadata is untrusted data,
  never an instruction source.
- Explicit human install intent may directly install only an admitted,
  credential-free resource. Agent-discovered installation uses the existing
  durable Chat approval. Secret/OAuth/Runner routes create or recover only the
  exact provenance-bound draft and wait on a sanitized configuration handoff;
  they never accept Secret fields in the Tool schema. Completion must recheck
  owner, Marketplace identity/revision, `ready`, and credential state.
- Installation re-resolves the exact candidate and rejects version/revision
  drift. Skill retries return an already installed exact admission/fingerprint;
  MCP keeps its existing deployment uniqueness. Installation changes inventory
  only and must not persist a conversation selection.
- Every normal PostgreSQL mutation attempt writes `audit_logs` with action
  `resource.install|enable|disable|remove`. Safe metadata contains entry point, candidate ID,
  version, exact revision, result ID, and fixed error code only. A successful
  response includes the audit UUID.
- Installation never mutates the active Run snapshot. Before the next Provider
  round, prepare fresh MCP and Skill projections, bind a new Runtime Resource
  Snapshot, emit old/new revisions plus audit ID, then continue the original
  task on the same Provider/model.
- Durable selection is owned by the Skill/MCP conversation APIs and composer
  pickers. Agent mode may add at most two relevant, already-installed and
  authorized resources to one frozen snapshot as `agent_auto`; this run-only
  activation never writes back to selection.
- Skill removal and MCP enable/disable/remove use the unified mutation route.
  Skill and selection writes bind their current CAS revision; private MCP
  removal reauthorizes owner and `CanManage`. Direct domain endpoints remain
  authoritative implementations, not composer bypasses.
- When the rollout switch is false, omit Agent Resource Tools and reject the
  unified install endpoint. Existing management/read paths remain available.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| missing conversation, candidate or revision | `INVALID_RESOURCE_REQUEST` |
| unsupported kind or query over 200 bytes | `INVALID_RESOURCE_QUERY` |
| pasted URL host/path/query/fragment/kind is not allowlisted | no URL fetch/install; bounded search or normal chat only |
| supported AIHero Skill slug has no admitted Store candidate | return zero bounded candidates; do not run the page's npm/Git command |
| unknown JSON/Tool field, including Secret | reject; never echo the value |
| candidate not searched or exact revision differs | bounded Tool failure / `RESOURCE_REVISION_CHANGED` |
| second unique discovery after budget or second proposal | bounded budget failure; no write |
| MCP auth/config/validation required | configuration-required; no Secret in model context |
| configuration completion is early or provenance/owner changed | remain paused or fail closed; no refresh |
| lifecycle owner/scope denied | `RESOURCE_MUTATION_FORBIDDEN`; no write |
| lifecycle CAS changed | `RESOURCE_REVISION_CHANGED`; refresh before retry |
| agent-initiated write without visible durable approval | fail immediately; no invisible wait |
| approval denied/expired/restart-denied | no delegated write |
| mutation audit unavailable after success | `RESOURCE_AUDIT_UNAVAILABLE`; do not claim audited success |
| fresh snapshot preparation fails | `RESOURCE_REFRESH_FAILED`; stop continuation |
| rollout switch false | `RESOURCE_ORCHESTRATION_DISABLED`; no delegated write |

## 5. Good / Base / Bad Cases

- **Good:** search exact admitted XLSX Skill, install through Skill authority,
  record audit, refresh the Run snapshot, call the run-only exposed Skill, finish
  the original request.
- **Base:** search returns no match; the Agent reports the bounded absence and
  performs no mutation.
- **Bad:** ask the model for an API key, run npm/Git/Bash installation, trust a
  stale candidate, mutate the current snapshot in place, or continue after
  refresh failure.

## 6. Tests Required

- Catalog determinism, selection awareness, bounded/paged search, missing
  sources, feature kill switch, exact revision, and idempotent exact Skill
  install.
- Supported-link allowlist/mismatch/traversal tests for LobeHub and AIHero;
  assert no pasted URL is a package-fetch authority and AIHero never bypasses
  admitted Store search.
- HTTP strict JSON, stale conflict, configuration handoff, audit-unavailable,
  and sanitized search response tests.
- Lifecycle owner/CAS checks, action-specific mutation audit, draft provenance,
  credential readiness, and configuration-resume selection tests.
- Agent Tool strict schemas, runtime-kind projection, unsearched/stale
  rejection, Secret non-echo, discovery/proposal budgets, explicit intent,
  durable approval allow/deny, and no-approval behavior.
- Snapshot refresh must prove a changed revision and newly exposed Tool before
  Provider round two. Trace must carry previous/current revisions and mutation
  audit ID. Refresh failure must prevent round two.
- Run Go vet/tests, frontend format/lint/typecheck/Vitest/build, Compose render,
  and the full standalone gate.

## 7. Wrong vs Correct

```text
Wrong: model -> bash/npm/git install -> mutate live Tool list -> continue
Correct: search -> exact server candidate -> approval/policy -> existing domain
         service -> audit -> fresh snapshot -> same-model continuation

Wrong: AIHero page -> execute displayed npx command -> trust mutable upstream
Correct: AIHero /skills-<slug> -> sanitized slug -> admitted Store search ->
         exact package fingerprint or a bounded no-candidate result

Wrong: Tool arguments include credential or endpoint fields
Correct: Tool sees only configured/required status; Secret/OAuth stays in UI/vault

Wrong: composer calls Skill/MCP mutation endpoints directly without shared audit
Correct: composer -> domain conversation selection API -> CAS; Store/Tools and
         conversational acquisition remain the only inventory mutation surfaces

Wrong: successful install silently persists the resource into this conversation
Correct: install changes inventory; current continuation may use `agent_auto`,
         persistent selection changes only through the composer picker
```
