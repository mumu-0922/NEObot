# Memory v2 storage foundation contracts

## 1. Scope / Trigger

Apply this contract when changing Memory Projects, Conversation Memory policy,
Memory scope columns, Memory settings, the v1 `usermemory` PostgreSQL
repository, or migration `053_memory_project_scope_settings`.

Migration `053` is the additive PR2 foundation. It does not add Project HTTP
routes, start a worker, call a Provider, enable a v2 reader, or authorize
`go_api_runtime` to write `projects`. Published migration bytes are immutable;
all later changes require a new migration.

## 2. Signatures

The database authority added by `053` is:

```text
projects(
  id UUID PK,
  user_id UUID FK users ON DELETE CASCADE,
  name, description,
  lifecycle_status = active | archived,
  revision >= 1,
  scope_generation >= 1,
  timestamps,
  UNIQUE(id, user_id)
)

conversations +=
  project_id UUID NULL,
  memory_scope_generation BIGINT >= 1 DEFAULT 1,
  memory_use_mode = inherit | on | off,
  memory_learn_mode = inherit | on | off,
  UNIQUE(id, user_id),
  FOREIGN KEY(project_id, user_id)
    REFERENCES projects(id, user_id) ON DELETE RESTRICT

user_memory_settings +=
  sensitive_memory_enabled BOOLEAN DEFAULT false,
  l2_mode = inherit | on | off,
  l3_mode = inherit | on | off

user_memories +=
  scope_type = global | project | conversation,
  project_id UUID NULL,
  scope_conversation_id UUID NULL,
  scope_generation BIGINT >= 1 DEFAULT 1
```

Memory scope ownership uses composite `ON DELETE RESTRICT` foreign keys:

```text
(project_id, user_id) -> projects(id, user_id)
(scope_conversation_id, user_id) -> conversations(id, user_id)
```

The existing HTTP signatures remain `/v1/memory-settings` and `/v1/memories`.
They expose the v1 Global view only; PR2 adds no scope field to their payloads.

## 3. Contracts

- Every pre-`053` Memory is explicitly backfilled to `scope_type=global`, null
  scope FKs, and `scope_generation=1`.
- Existing `enabled`, `search_enabled`, and `auto_record_enabled` values are
  preserved. The migration never opts a user into Learn or sensitive egress.
- Scope shape is database-enforced: Global has no scope FK, Project has only a
  Project FK, and Conversation has only a Conversation FK.
- Active exact-content uniqueness is per scope. Global keys use
  `(user_id, normalized_content)`; Project and Conversation keys include their
  owning scope ID. The same normalized content may exist at different scopes.
- The v1 repository explicitly inserts, lists, updates, deletes, marks used,
  and resolves conflicts only for `scope_type='global'`. `source_conversation_id`
  remains provenance and never grants Conversation scope.
- `projects` has no PR2 runtime grant. A later Project API migration must add a
  reviewed capability boundary rather than hot-granting the table.
- Runtime release flags remain unchanged/off. Schema presence is not reader or
  worker activation authority.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| Project or Conversation scope references another user | Composite FK rejects the write. |
| Scope type and nullable scope IDs disagree | `user_memories_scope_shape_check` rejects the write. |
| Scope generation is below one | CHECK rejects the write. |
| Duplicate normalized content in the same active scope | Unique index rejects or the Global v1 create upserts. |
| Same normalized content exists in a different scope | Accept; recall precedence is deferred to a later reader PR. |
| v1 API targets a Project/Conversation Memory ID | Behave as not found; do not mutate scoped data. |
| Down sees any Project row | Fail with `MEMORY_V2_ROLLBACK_REQUIRES_EMPTY_PROJECTS`. |
| Down sees non-Global Memory data | Fail with `MEMORY_V2_ROLLBACK_REQUIRES_GLOBAL_MEMORY_ONLY`. |
| Down sees non-default Conversation policy/generation | Fail with `MEMORY_V2_ROLLBACK_REQUIRES_INHERITED_CONVERSATION_POLICY`. |
| Down sees changed Sensitive/L2/L3 settings | Fail with `MEMORY_V2_ROLLBACK_REQUIRES_DEFAULT_MEMORY_SETTINGS`. |

## 5. Good / Base / Bad Cases

- **Good**: apply `053` over existing settings and Global Memory, preserve all
  old values, add a same-content Project override, and keep the v1 API limited
  to the Global row.
- **Base**: apply `053` to an empty Memory store. All new defaults are
  privacy-safe, all runtime flags stay off, and down/re-up succeeds.
- **Bad**: use `source_conversation_id` as scope, infer ownership from a free
  text tag, let v1 CRUD mutate Project rows, or drop `053` after v2 data exists.

## 6. Tests Required

- SQL contract tests must assert fields, defaults, scope shape, composite
  ownership FKs, `ON DELETE RESTRICT`, three scoped unique indexes, explicit
  Global backfill, and every down guard.
- Disposable PostgreSQL must prove old setting preservation, Global backfill,
  cross-user rejection, same-scope duplicate rejection, cross-scope duplicate
  acceptance, illegal shape rejection, guarded down, clean down, and re-up.
- `internal/usermemory` integration must prove Global creation/upsert and that
  v1 list/update/delete cannot see or mutate a Project-scoped fixture.
- Run `go test -race ./internal/usermemory ./internal/migration ./cmd/migrate`,
  `go test ./...`, and `go vet ./...` from `mm-chat/backend`.

## 7. Wrong vs Correct

### Wrong

```sql
-- Old conflict target after the Global index was replaced.
ON CONFLICT (user_id, normalized_content)
WHERE deleted_at IS NULL DO UPDATE ...
```

This no longer infers a unique index and also fails to define which scope the
v1 writer owns.

### Correct

```sql
INSERT INTO user_memories (..., scope_type)
VALUES (..., 'global')
ON CONFLICT (user_id, normalized_content)
WHERE deleted_at IS NULL AND scope_type = 'global' DO UPDATE ...
```

The code and schema now share the same Global partial-index predicate. Because
the pre-`053` binary still has the wrong target, both upgrade and schema
rollback require an outage boundary. Upgrade by stopping the old backend,
applying `053`, then deploying the new backend. Roll back by stopping the new
backend, downing `053` while guards pass, then deploying the old binary. Never
mix pre-`053` and post-`053` writers after the migration.
