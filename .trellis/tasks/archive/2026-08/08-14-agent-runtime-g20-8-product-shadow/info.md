# G20.8 technical design index

## Delivery slices

1. **Control/read API**
   - Reuse `/v1/skills/*` for package Store/library/admission.
   - Add an `internal/agentcontrol` facade for authenticated Run process,
     approval, Child, Cron and Draft review DTOs/actions.
   - Extend owning repositories with bounded user/admin projections rather than
     exposing generic table readers.

2. **Shadow authority**
   - Add migration `090` and `internal/agentshadow` only if persistence is
     needed after final repository mapping.
   - Default off; PostgreSQL policy/opt-in/generation/observation authority;
     injected synthetic/read-only adapter; no startup executable worker.

3. **Agent Center frontend**
   - Add a top-level panel and URL state, typed server API client, Zod DTO
     validation, domain components and tests.
   - Preserve the existing `SkillMarket` as visibly legacy through G20.8.

4. **Legacy cutover preparation**
   - Add local content-free inventory, explicit local backup and dry-run reset
     plan; no deletion until G20.9.

5. **Verification and documentation**
   - Focused backend/frontend/accessibility tests, PostgreSQL drill if `090` is
     used, clean restart/backup proof, Phase 0/full standalone and synchronized
     architecture/contracts/deployment/tracking/Trellis specs.

## Hard boundaries

- No frontend direct authority and no direct table DML from handlers.
- No Assistant/MCP identity reuse for package Skills.
- No raw prompt/output/Tool/Secret/Workspace body in Agent/Shadow diagnostics.
- No package execution in API/browser process.
- No destructive legacy state mutation in G20.8.
- Exact-host promotion remains held while `verify-agent-runner-host.sh` returns
  `ISOLATION_UNAVAILABLE`.

## Likely affected areas

- Backend: `internal/httpserver`, `cmd/api`, `internal/skillsupply`,
  `internal/agentorchestrator`, `internal/agentbroker`,
  `internal/agentdelegation`, `internal/agentcron`, `internal/agentlearning`,
  new control/shadow packages and optional migration `090`.
- Frontend: `components/app/ChatApp.tsx`, `lib/chat/panelUrlState.ts`, new
  `components/agent/`, `services/api/client/{types,server}`, runtime schemas,
  locale bundles, storage inventory utilities and `src/__tests__/`.
- Operations/contracts: Agent Runtime docs/specs, Phase 0 and new focused gates.

## Rollout order

Land source and migrations default-off, verify API authorization and UI held
states, exercise synthetic adapter in disposable tests, run inventory dry-run,
then require exact deployment/backup/canary evidence before any production flag
activation. G20.8 does not promote itself merely because repository CI is green.
