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

type DirectSkillInstallRequest struct {
    URL, Identifier, UserID, ConversationID, EntryPoint string
}

type SupportedResourceLink struct {
    Kind, Identifier, URL string
    DirectInstall bool
}

func SingleSupportedResourceLink(text string) (SupportedResourceLink, bool)

skillsupply.Service.InstallDirectSkillLink(
    context.Context, userID, url, expectedName string,
) (skillsupply.Installation, error)
```

## 3. Contracts

- Catalog/search responses are `no-store`, authenticated, server-sanitized, and
  bounded. Search returns at most five results and exact immutable revision
  material; it never returns credentials, endpoints, package bodies, or paths.
- An explicitly supported discovery HTTPS link may be reduced to one bounded
  identifier only when host, path shape, kind, port, query, fragment, and
  identifier validation pass. LobeHub Skill/MCP links, exact GitHub Skill tree
  or `SKILL.md` blob paths, and AIHero `/skills-<slug>` links are admitted
  shapes. LobeHub remains a bounded Store/Marketplace alias. Exact GitHub and
  legacy AIHero Skill links enter only the fixed owner-private direct adapter.
- The direct adapter accepts only one unambiguous GitHub Skill directory. It
  rejects repository roots, query/fragment/userinfo/non-default ports, encoded
  paths and traversal; resolves mutable refs to one 40-character commit; and
  downloads the fixed `codeload.github.com` ZIP. The legacy AIHero adapter may
  fetch only the canonical page and parse one unique restricted
  `npx skills@latest add owner/repository --skill=<slug>` coordinate as data.
  Neither path executes npm, Git, Shell, page code, hooks, tests, scripts, or
  package entrypoints.
- Direct packages reuse `ValidateArchive`, canonical ZIP, SBOM,
  content-addressed objects, and runtime revalidation. Their candidate remains
  `validated`, has no reviewer, is scoped by `owner_user_id`, is excluded from
  Store, and is installable only by that owner. This is structural validation,
  not content review.
- A selected GitHub Skill subdirectory is the extraction authority. ZIP entry
  paths are still validated across the archive, but file type/content checks
  apply only inside the selected prefix; an unrelated repository-root symlink
  must not invalidate a safe Skill, while any symlink inside the selected Skill
  remains forbidden.
- Authenticated owner IDs use canonical UUID syntax and may include the fixed
  development/bootstrap UUID. Do not apply versioned resource-object UUID
  validation to an authenticated principal ID.
- When the current human text has explicit install intent and contains exactly
  one supported discovery link, Chat enters the server-owned deterministic
  route before any Provider request. Exact GitHub or legacy AIHero Skills go
  straight to the direct adapter with zero Store searches; other supported
  links use bounded `resource_search`. The scanner is bounded to
  16 KiB, stops a URL at non-ASCII/user-text boundaries, and rejects multiple
  URLs, query, fragment, userinfo, non-443 port, wrong host/path/kind, traversal,
  or an unsupported scheme. It does not create a second search authority.
- The non-direct deterministic path may install only one candidate whose ID or package
  name exactly matches the parsed identifier. Zero or ambiguous exact matches
  finish the Assistant normally with a truthful no-install answer and the
  durable Resource search trace. Search/install failure stays fail-closed and
  never falls through to an arbitrary Provider-generated installer.
- A successful deterministic install uses the existing exact revision,
  owner, admission, credential, audit, and mutation service. Because this
  install-only turn has no later Provider Step, it reports availability on the
  next Agent task; a later ordinary Tool-loop install still refreshes the
  frozen snapshot before its next Provider Step. Neither path changes the
  Conversation selection.
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
- If an install write returns an error after commit, perform one bounded
  authoritative Library read-back. Report success only when the exact
  candidate/fingerprint is present; otherwise preserve the original error.
- Every normal PostgreSQL mutation attempt writes `audit_logs` with action
  `resource.install|enable|disable|remove`. Safe metadata contains entry point, candidate ID,
  version, exact revision, result ID, and fixed error code only. A successful
  response includes the audit UUID. Encode audit metadata as text with an
  explicit `::jsonb` cast; passing raw `[]byte` through pgx is not a valid JSONB
  contract. Audit failures log only a bounded class/SQLSTATE, never metadata.
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
| explicit install text contains zero, multiple, oversized, or unsupported URLs | do not enter deterministic installation; ordinary Agent behavior |
| supported explicit GitHub Skill tree/blob link | bypass Store; owner-private pinned-source install before Provider |
| GitHub link is a repository root, encoded, ambiguous, or does not identify one Skill directory | direct install fails; zero package mutation/Provider calls |
| supported explicit AIHero Skill link | bypass Store; owner-private pinned-source install before Provider |
| AIHero page command is absent, ambiguous, or names another Skill | direct install fails; zero package mutation/Provider calls |
| GitHub commit cannot be pinned, ZIP exceeds bounds, or zero/multiple matching Skills validate | direct install fails closed |
| another owner addresses a private candidate | installation denied even with exact candidate UUID/fingerprint |
| deterministic search returns zero or no unique exact identifier match | completed no-install answer; zero Provider/install calls |
| deterministic search returns one exact admitted credential-free candidate | install once through existing domain authority; next Agent task sees inventory |
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
- Supported-link allowlist/mismatch/traversal tests for LobeHub, GitHub, and AIHero.
  GitHub fixtures must cover exact tree/blob paths, repository-root rejection,
  branch-to-commit pinning, and zero Provider/Store calls.
  AIHero fixtures must prove bounded page parsing, unique restricted command,
  exact GitHub commit, one matching package, no command execution, and no Store
  publication.
- Embedded-CJK/trailing-punctuation parsing, multiple-link denial, and
  Provider-zero-call tests for deterministic explicit handling. Cover zero,
  unique exact, and ambiguous exact Store candidates. Separately prove an
  explicit GitHub or AIHero link performs one direct mutation with zero Store
  searches.
- PostgreSQL migration/runtime tests must prove same-owner private installation,
  cross-owner denial, Store exclusion, idempotent retry, Library listing,
  conversation selection, runtime materialization, uninstall, guarded down,
  and clean re-up.
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
Correct: AIHero /skills-<slug> -> parse one restricted coordinate as data ->
         exact GitHub commit -> existing validation/SBOM -> owner-private library

Wrong: GitHub repository URL -> guess a Skill -> install mutable branch bytes
Correct: exact GitHub Skill tree/blob path -> resolve ref -> exact commit ->
         existing validation/SBOM -> owner-private library

Wrong: explicit supported link -> Provider must call resource_search -> 502 blocks discovery
Correct: explicit intent + one supported link -> Backend resource_search ->
         unique exact candidate or completed no-install answer

Wrong: Tool arguments include credential or endpoint fields
Correct: Tool sees only configured/required status; Secret/OAuth stays in UI/vault

Wrong: composer calls Skill/MCP mutation endpoints directly without shared audit
Correct: composer -> domain conversation selection API -> CAS; Store/Tools and
         conversational acquisition remain the only inventory mutation surfaces

Wrong: successful install silently persists the resource into this conversation
Correct: install changes inventory; current continuation may use `agent_auto`,
         persistent selection changes only through the composer picker
```
