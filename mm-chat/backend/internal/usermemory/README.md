# usermemory

`usermemory` is the Go authority for optional, durable, per-user Memory in
server mode. It keeps Memory separate from conversation history and derived
conversation summaries.

## Responsibilities

- expose authenticated settings and Memory CRUD at `/v1/memory-settings` and
  `/v1/memories`;
- persist settings and soft-deletable entries in Postgres migration `035`;
- normalize and validate types, content, importance, tags, and source IDs;
- rank only relevant lexical/CJK matches and return at most five;
- prevent disabled Memory or disabled auto-record from reading/writing entries.

Provider prompt injection and background extraction live in `internal/chat` so
this package does not depend on Provider implementations.

## Usage

```go
repo := usermemory.NewPostgresRepository(db)
service := usermemory.NewService(repo)
handler := usermemory.NewHandler(service)

matches, err := service.SearchRelevant(ctx, "keep this concise", 5)
```

Identity is always read from `auth.UserFromContext` through
`auth.UserOrDevelopment`; API inputs never accept a user ID.

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
