# G15 Server-Owned Task Model Settings Plan

Status: complete. This slice moves the six default automation-model
selections from browser-local preference storage to Go/Postgres authority.

## Contract

- Persist title generation, related questions, context compression, prompt
  optimization, RAG query, and memory model references per owner in Postgres.
- Expose the authoritative values through `GET /v1/config` and an administrator
  `GET/PATCH /v1/admin/task-models` API.
- Accept only bounded `providerId:modelId` references that belong to an enabled,
  backend-stored provider and one of its configured models.
- Let the settings page save a selection immediately, show a compact saving or
  failure state, and roll back the optimistic choice if the request fails.
- On first cutover, import the existing valid browser selections once when no
  Postgres row exists; after that, Postgres always wins.
- Stop persisting `defaultModels` in browser preference storage. Browser state
  remains only a runtime projection of the server response.

## Verification

- Go service/handler/repository and migration tests.
- Frontend API/store/component regression tests plus full quality gates.
- Live Postgres proof: change a row in the page, reload, restart frontend and
  backend, and verify the same server value returns without browser authority.
- Clean the disposable proof state or restore the owner's original selections.

## Rollback

The migration has a paired down script. The frontend/API changes are isolated
from provider secrets and chat data. Rollback restores the previous browser
projection behavior without altering provider records or conversations.

## Executable contract

### 1. Scope / trigger

This contract applies whenever an automation task model is loaded, changed, or
executed in server mode. The browser store is a runtime projection only.

### 2. Signatures

```text
GET   /v1/config
GET   /v1/admin/task-models
PATCH /v1/admin/task-models

task_model_settings(user_id PK, six model-ref columns, created_at, updated_at)
```

### 3. Request, response, and persistence

- PATCH accepts any non-empty subset of `titleGeneration`,
  `relatedQuestions`, `contextCompression`, `promptOptimization`, `ragQuery`,
  and `memory`.
- Each non-blank value uses `providerId:modelId` and is at most 512 bytes.
- The response returns all six values, `configured`, and `updatedAt`.
- `defaultModelsConfigured=true` in `/v1/config` means the frontend must replace
  browser runtime state with the returned server values.

### 4. Validation and error matrix

```text
empty PATCH / malformed ref       -> 400 TASK_MODEL_SETTINGS_INVALID
unknown/disabled provider/model   -> 409 TASK_MODEL_UNAVAILABLE
missing Postgres repository       -> 503 DATABASE_REQUIRED
unknown JSON field / trailing JSON -> 400 INVALID_REQUEST
```

### 5. Good, base, and bad cases

- Good: `SERVER_DEFAULT:gpt-5.6-luna` from an enabled, attested provider.
- Base: no Postgres row returns `configured=false`; the app performs one
  bounded import of valid legacy selections.
- Bad: a browser-only or deleted provider reference is rejected and the UI
  restores its previous selection.

### 6. Required tests

- Migration up/down fragments and repository reload.
- Service validation, handler status/error mapping, and public-config authority.
- Client GET/PATCH shape, store browser-persistence exclusion, bootstrap, UI
  autosave/rollback, full frontend/backend gates, and live restart proof.

### 7. Wrong vs correct

```text
Wrong:  dropdown -> Zustand -> localStorage -> task request
Correct: dropdown -> PATCH Go -> Postgres -> /v1/config -> Zustand projection
```
