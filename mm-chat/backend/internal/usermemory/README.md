# usermemory

`usermemory` is the Go authority for optional, durable, per-user Memory in
server mode. It keeps Memory separate from conversation history and derived
conversation summaries.

## Responsibilities

- expose authenticated settings and Memory CRUD at `/v1/memory-settings` and
  `/v1/memories`;
- persist settings and soft-deletable entries from migration `035`, with the
  additive Project/scope/settings foundation from migration `053`;
- keep the current v1 API explicitly Global-only while later v2 readers and
  Project APIs remain disabled;
- normalize and validate types, content, importance, tags, and source IDs;
- rank only relevant lexical/CJK matches and return at most five;
- prevent disabled Memory or disabled auto-record from reading/writing entries.

Provider prompt injection remains in `internal/chat`. Durable extraction runs
in `internal/memoryworker`, which adapts its lease-fenced candidate writes back
through `Service.StoreExtracted`; this package still has no Provider dependency.

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
repository always inserts `scope_type='global'` and repeats the Global partial
index predicate in its `ON CONFLICT` target. List/update/delete/mark-used also
filter Global rows, so this package cannot mutate future Project or
Conversation Memory before the owning API is introduced.

## Main API

| Boundary                            | Purpose                                                     |
| ----------------------------------- | ----------------------------------------------------------- |
| `NewPostgresRepository(*sql.DB)`    | Postgres persistence scoped to the current user             |
| `NewService(Repository)`            | Validation, settings, CRUD, relevance, extraction admission |
| `NewHandler(*Service)`              | JSON HTTP routes and bounded error mapping                  |
| `SearchRelevant(ctx, query, limit)` | Relevant-only Top-5 retrieval                               |
| `StoreExtracted(ctx, input)`        | Opt-in bounded AI candidate persistence                     |

## Files

```text
handler.go               HTTP contract and DTO mapping
repository_postgres.go   user-scoped SQL and soft deletion
service.go               validation, settings, normalization, relevance
types.go                 repository/service contracts
uuid.go                  application-owned UUID generation
*_test.go                 unit, HTTP, and optional live Postgres coverage
```

See [DESIGN.md](DESIGN.md) and
[`docs/contracts/conversation-context.md`](../../../docs/contracts/conversation-context.md).
