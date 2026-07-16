# Standalone `mm-chat` Cutover Gap

## Frozen End State

`mm-chat/` must become the only independently installable, buildable, testable,
deployable, and restorable project root. The current frontend interface and all
user-visible capabilities remain the product baseline. The final runtime is
server-only: the browser talks to Go, and Go owns access to the private Python
RAG service and external providers.

The original root project is a temporary migration source. It may be deleted
only after an owner-confirmed delete plan and a clean-copy verification that
does not mount, import, build from, or execute any original-root artifact.

## Evidence Baseline

Live source and deployment configuration currently prove:

- `mm-chat/frontend/` contains the complete Next.js application, manifest,
  lockfile, configuration, assets, tests, and standalone Docker image;
- `mm-chat/compose.single-server.yml` builds the frontend in server mode and
  exposes the same-origin `/mm-api` edge to the private Go service;
- the relocated frontend still registers 11 transitional Next.js `/api/*`
  route handlers;
- the Go backend already registers Auth, Chat, Files, Browser Import, Teams,
  Knowledge Collection/Document/Consent, Health, Readiness, and Metrics routes;
- the relocated frontend has working server-mode Chat CRUD/SSE, Files, and Browser
  Import adapters, but production Auth, Teams, Knowledge/RAG, Plugins, Provider
  Settings, Voice, Image, Code Execution, Search, and Agent flows are not all
  wired to Go.

## Completed Migration Spine

| Capability                    | Backend             | Frontend                   | Remaining boundary                            |
| ----------------------------- | ------------------- | -------------------------- | --------------------------------------------- |
| Chat CRUD and SSE             | Go                  | Relocated UI wired         | Remove local production path                  |
| Files and attachments         | Go + object storage | Relocated UI wired         | G9.5 fenced local persist, OPFS, and chat-message IndexedDB writes |
| Browser data import           | Go                  | Relocated UI wired         | Keep explicit one-time import                 |
| Auth and sessions             | Go                  | Partial/legacy UI          | Wire server-only auth lifecycle               |
| Teams and membership          | Go                  | Not fully wired            | Add UI adapters and screens in existing theme |
| Knowledge control plane       | Go                  | Local knowledge UI remains | Wire Collections/Documents/Consent to Go      |
| Offline parser/RAG foundation | Python dark-run     | Not production-visible     | Complete Phase 15.2 gates and Go handlers     |

## Legacy Next.js API Route Disposition

The following routes already have a Go replacement spine and must be removed
from the final frontend after parity verification:

```text
/api/access/verify    -> /v1/auth/login
/api/chat             -> /v1/chat/conversations/*
/api/chat/generate    -> /v1/chat/conversations/{id}/stream
/api/health           -> /health + /ready + /v1/version
```

The following RAG/doc-parse routes were retired in G9.2 after the Go/RAG
server-mode path passed:

```text
/api/chat/rag-queries
/api/doc-parse
/api/doc-parse/jobs/{id}
/api/rag/delete
/api/rag/query
/api/rag/upsert
```

The following config/provider/BYOK routes were retired in G9.3 after the Go
API-client adapters passed:

```text
/api/byok/public-key
/api/config
/api/providers/models
```

The following plugin/agent routes were retired in G9.4 after the Go API-client
adapters passed:

```text
/api/agents
/api/agents/{identifier}
/api/plugins/execute
/api/plugins/install
/api/plugins/list
```

The remaining 7 root handlers still require a Go/RAG replacement or an
equivalent server-owned static/catalog implementation:

```text
/api/chat/execute-code
/api/chat/generate-image
/api/chat/generate-title
/api/chat/related-questions
/api/search
/api/voice/synthesize
/api/voice/transcribe
```

## Remaining Cutover Work

1. Wire the existing Go Auth, Teams, Knowledge, Files, Chat, and Import APIs
   through typed frontend adapters; keep the current visible UI and theme.
2. Replace the 11 remaining handlers with Go/RAG capabilities, one domain
   at a time, using change-based tests instead of repeated full-suite runs.
3. Remove production `local|server` branching, IndexedDB/OPFS authority, and
   legacy Next API routes only after their Go-backed parity tests pass. G9.5a
   has made Zustand local persistence no-op in server mode, and G9.5b has made
   OPFS write/delete helpers throw in server mode. G9.5c replaced direct chat
   message `appDb.setItem/removeItem` calls with runtime helpers that also throw
   in server mode; explicit browser import/export reads remain available.
4. Centralize migrated design tokens/components as the mandatory UI extension
   surface for later features.
5. Run one final full gate in a clean copy containing only `mm-chat/`, then
   prepare a separate exact original-root deletion plan for owner confirmation.

## Verification Policy

- During migration: test the changed adapter, handler, store, route, or Compose
  edge and its nearest callers only.
- At a domain cutover: run that domain's integration and browser smoke.
- At standalone-project closure: run the complete frontend, Go, RAG, migration,
  Docker, security, backup/restore, visual-regression, and clean-copy gates once.
