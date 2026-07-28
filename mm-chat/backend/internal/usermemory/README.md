# usermemory

`usermemory` is the Go authority for optional, durable, per-user Memory in
server mode. It keeps Memory separate from conversation history and derived
conversation summaries.

## Responsibilities

- expose authenticated settings and Memory CRUD at `/v1/memory-settings` and
  `/v1/memories`, plus PR6 Activity polling, Usage inspection, and safe undo;
- persist settings and canonical entries from migration `035`, with the
  additive Project/scope foundation from `053` and provenance/delete authority
  from `055`;
- keep the current v1 API explicitly Global-only while later v2 readers and
  Project APIs remain disabled;
- hydrate and apply explicit current-user `remember|correct|forget` actions
  through migration `057` capabilities without trusting model-supplied user,
  scope, target, or revision authority;
- return current visible Memory details for Activity/Usage links, or a deleted
  marker after deletion, disablement, archive, or generation/epoch drift;
- normalize and validate types, content, importance, tags, and source IDs;
- rank only relevant lexical/CJK matches and return at most five;
- prevent disabled Memory or disabled auto-record from reading/writing entries.

Provider prompt injection remains in `internal/chat`. Durable extraction runs
in `internal/memoryworker`; PR5 reuses only
`NormalizeCandidateForStorage` and persists candidate-wide shadow/Review
proposals through migration `056`. It no longer calls the legacy
`Service.StoreExtracted` canonical write path.

## Usage

```go
repo := usermemory.NewPostgresRepository(db)
service := usermemory.NewService(repo)
handler := usermemory.NewHandler(service)

matches, err := service.SearchRelevant(ctx, "keep this concise", 5)
```

Identity is always read from `auth.UserFromContext` through
`auth.UserOrDevelopment`; API inputs never accept a user ID.

Migration `053` allows the same normalized content in different scopes. The v1
repository remains Global-only. Migration `055` moves manual upsert/update and
delete behind narrow database capabilities: edits append one prior snapshot,
and delete atomically hides the row, writes a targeted tombstone and ID/hash-only
manifest, and enqueues a provider-free purge job. HTTP request and response
shapes remain unchanged.

Migration `057` adds an optional `ActionRepository`. Chat can use it to hydrate
a bounded current Memory context and submit a typed direct-user action. The
service normalizes candidate data, rejects secrets locally, binds generated
IDs, and leaves ownership/scope/revision revalidation to narrow PostgreSQL
capabilities. `GET /v1/memory-activities`,
`POST /v1/memory-activities/{id}/undo`, and
`GET /v1/memory-usages?assistantMessageId=...` are backend-only governance
seams until the PR9 frontend.

## Main API

| Boundary                            | Purpose                                                     |
| ----------------------------------- | ----------------------------------------------------------- |
| `NewPostgresRepository(*sql.DB)`    | Postgres persistence scoped to the current user             |
| `NewService(Repository)`                    | Validation, settings, CRUD, relevance, and optional action authority |
| `NewHandler(*Service)`                      | JSON HTTP routes and bounded error mapping                          |
| `SearchRelevant(ctx, query, limit)`         | Relevant-only Top-5 retrieval                                      |
| `HydrateDirectAction(ctx, input)`           | Bounded current context for the strict direct-user planner          |
| `ExecuteDirectAction(ctx, input)`           | Local validation plus narrow SQL action capability                  |
| `ListActivities` / `ListMessageUsages`      | User-scoped polling and answer provenance                           |
| `UndoActivity(ctx, id, expectedRevision)`   | Revision-safe created/corrected undo                                |
| `NormalizeCandidateForStorage(in)`          | Shared validation/normalization without a write                     |

## Files

```text
handler.go               HTTP contract and DTO mapping
repository_postgres.go   user-scoped canonical writes and deletion capability
action_repository_postgres.go  PR6 action, Activity, Usage, and undo capabilities
action_service.go         direct action validation and application IDs
privacy.go                shared secret/Sensitive classification and redaction
service.go               validation, settings, normalization, relevance
types.go                 repository/service contracts
uuid.go                  application-owned UUID generation
*_test.go                 unit, HTTP, and optional live Postgres coverage
```

See [DESIGN.md](DESIGN.md) and
[`docs/contracts/conversation-context.md`](../../../docs/contracts/conversation-context.md).
