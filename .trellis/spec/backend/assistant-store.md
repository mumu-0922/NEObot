# Assistant Store Backend Contract

## Scenario: Server-owned lightweight Assistant presets

### 1. Scope / Trigger

Use this contract for `internal/agents`, migration `082`, `/v1/assistants/*`,
LobeHub discovery/admission, or Assistant-library deployment changes.
Assistants are bounded System Prompt presets, not independent Agent runtimes.

### 2. Signatures

- DB: `assistant_library_entries` and `assistant_market_admissions`.
- Library: `GET|POST /v1/assistants/library`,
  `GET|PUT|DELETE /v1/assistants/library/{id}`.
- Actions: `POST .../{id}/copy`, `POST .../{id}/update`.
- Store: `GET /v1/assistants/market?page=N&pageSize=20`,
  `GET /v1/assistants/market/items/{identifier}`,
  `POST .../{identifier}/install|review`.
- Preferred LobeHub source: the configured Market M2M adapter calls
  `/api/v1/agents` and `/api/v1/agents/detail/{id}` with bounded JSON and the
  same token/cache authority used by MCP Marketplace. Public `.data` routes
  remain the administrator-only fallback: English `/agent.data` and
  `/agent/{id}.data`; Chinese `/zh/...`; Japanese `/ja/...`. English never adds
  `/en`.

### 3. Contracts

- Library rows are owned by `user_id`; only Store rows are unique by
  `(user_id, source, source_identifier)`. Multiple custom rows must coexist.
- Installation accepts only an admitted snapshot whose 64-character canonical
  fingerprint matches the confirmed detail.
- Custom edits, deletion, uninstall, Store-to-custom copy, and Store updates use
  revision/CAS.
- Live detail and live list apply identical provenance before fingerprinting.
  If live detail fails, administrator detail and review both use legacy detail
  and therefore the same fingerprint.
- Store categories follow LobeHub's Assistant UI allowlist. Do not expose raw
  `engineering` or `uncategorized` aggregate buckets, because the public list
  currently cannot enumerate them. Category labels/counts must not imply that
  LobeHub's full aggregate corpus is individually browseable.
- Store installation persists display metadata plus the complete bounded
  System Prompt and normalized display-only `required_tools`. Tool declarations
  participate in the fingerprint but never install or enable Models,
  credentials, MCP, Tools, plugins, Knowledge Bases, or permissions.
- Starting a chat copies the prompt into Conversation state; later library
  mutation cannot change history.

### 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Missing repository/database | `ASSISTANT_LIBRARY_UNAVAILABLE` / 503 |
| Ordinary user requests unadmitted item | `ASSISTANT_NOT_ADMITTED` / 403 |
| Fingerprint changed or mismatched | `ASSISTANT_MARKET_CHANGED` / 409; no row |
| Stale library revision | `ASSISTANT_REVISION_CONFLICT` / 409 |
| Duplicate Store install | `ASSISTANT_ALREADY_INSTALLED` / 409 |
| Invalid/missing System Prompt | fail closed; no admission/install row |
| Live schema/transport failure | administrator-only legacy fallback |

### 5. Good / Base / Bad Cases

- Good: administrator opens detail, reviews the exact fingerprint, admits it,
  then a user installs that immutable snapshot.
- Base: an admitted snapshot remains usable while upstream is unavailable.
- Bad: trusting an upstream list card, synthesizing a prompt, or importing
  prompt-mentioned capabilities.

### 6. Tests Required

- Focused `internal/agents` tests for admission, matching fingerprints,
  live/legacy detail parity, English URL paths, multi-custom rows, ownership,
  and revision conflicts.
- Migration tests must assert partial Store uniqueness and reject a
  `NULLS NOT DISTINCT` constraint that limits custom rows.
- Run `scripts/verify-assistant-store-postgres17.sh` to prove PostgreSQL 17
  replay, runtime grants, ownership, multi-custom rows, Store uniqueness,
  `required_tools`, and revision conflicts.
- Before release: build the Backend `runtime` target, apply the exact additive
  migration from a pinned candidate, verify schema head, then recreate only the
  intended service with retained rollback images.

### 7. Wrong vs Correct

#### Wrong

```text
https://lobehub.com/en/agent.data
UNIQUE NULLS NOT DISTINCT (user_id, source, source_identifier)
Store card fingerprint -> different detail provenance -> admit anyway
```

#### Correct

```text
https://lobehub.com/agent.data
UNIQUE INDEX ... WHERE source = 'lobehub'
list/detail/review -> same normalized snapshot -> same fingerprint
```
