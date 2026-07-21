# G15 Server-Owned Task Model Settings Process

## 2026-07-21 — Runtime trace and design lock

The settings dropdown called `updateDefaultModels` directly. Zustand persisted
the six values under `neo-chat-core-settings` through browser `localStorage`,
and no Go write API existed. The Go public-config DTO already contained a
`defaultModels` field but returned an empty map, so it was not authoritative.

The selected design adds a dedicated `task_model_settings` table and bounded
administrator API while reusing the existing authenticated runtime-config
service and `/v1/config` bootstrap. Existing browser choices are eligible only
for a one-time bootstrap when the server explicitly reports that no row exists.
Once configured, the server response replaces browser state.

## 2026-07-21 — G15.1 implementation and live closure

Migration `036` adds a per-owner `task_model_settings` table with bounded
columns and a paired rollback. The runtime-config service now exposes
`GET/PATCH /v1/admin/task-models`, rejects malformed or unavailable provider
model references, and publishes the authoritative values plus an explicit
configured flag through `/v1/config`.

The frontend removes `defaultModels` from the persisted core-settings snapshot.
App startup imports valid legacy selections only when the server explicitly
reports no row; afterward the server replaces runtime state. The settings page
loads the server record, optimistically applies each selection, immediately
PATCHes it, shows saving/saved state, and restores the prior value on failure.

The first Chrome restart pass exposed a real lifecycle bug: a provider-list
update aborted the settings GET while a sticky `bootstrapStartedRef` prevented
retry, leaving every dropdown disabled. The guard was removed; the fetch is now
abortable and safely re-entrant. A second deployed pass proved the controls
become enabled after reload.

Verification:

```text
backend gofmt / go vet / go test ./...              passed
frontend lint / typecheck / format / build          passed
frontend full tests                                 191 files / 911 tests
focused task-model tests                            6 files / 24 tests
change / quality / security automated gates         passed / passed / passed
migration                                            up 036_task_model_settings
deployed backend / frontend                          healthy / healthy
initial owner mapping                                Luna title; Sol other five
temporary UI change                                  Luna -> Terra
PATCH response / saved status                        200 / visible
reload selection                                     GPT-5.6 Terra
browser localStorage defaultModels                   absent
backend + frontend restart selection                 GPT-5.6 Terra
owner mapping restoration                            Terra -> Luna; 200; reload OK
GET /v1/config cache policy                          Cache-Control: no-store
legacy bootstrap setup                               delete owner row; seed Luna/Sol browser state
legacy bootstrap result                              PATCH 200; row recreated; local copy removed
final authoritative state                            Luna title; Sol other five; configured=true
```

The proof changed only the title task model temporarily. It made no provider
call, consumed no model quota, and restored the owner's six original choices.
The final live `/v1/config` read after restoration returned
`defaultModelsConfigured=true` with Luna for title generation and Sol for the
other five tasks. Its response carried `Cache-Control: no-store`, so reloads do
not reuse a stale task-model projection.

The cutover path was also exercised against a disposable baseline: the owner's
Postgres row was temporarily removed, the prior Luna/Sol browser snapshot was
seeded, and app startup recreated the row through a successful PATCH. The
browser's `defaultModels` field was then removed. The database was left in the
same configured state that existed before the proof.
