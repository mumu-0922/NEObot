# Phase 15.1 Knowledge Control Plane Plan

## Goal

Build the authoritative identity, Team, Knowledge ACL, Consent, and Outbox
control plane in Go/Postgres before Python parsing or retrieval is allowed to
process user data. The existing Next.js/React UI remains unchanged; frontend
work is limited to later service-adapter wiring.

## Boundaries

- Go owns public APIs, sessions, authorization, file binding, revisions,
  consent decisions, outbox events, and citation reauthorization.
- Postgres is authoritative. Redis and the future vector index are rebuildable
  acceleration layers and never decide access.
- Python receives only short-lived, workload-authenticated jobs after Go has
  checked the relevant Governance and Consent matrix.
- Team Admin authority never grants access to another user's Personal
  Knowledge.
- Outbox `BIGSERIAL` IDs are allocation order, not transaction commit order.
  Replay checkpoints advance only across a contiguous applied prefix; workers
  also rescan claimable rows below the checkpoint and deduplicate by `event_id`.

## Execution Order

- [x] **15.1A — Schema foundation:** add reversible identity, Team,
      Membership, Invite, Collection, logical Document/Version, Governance,
      Consent, and Outbox schema. Active Documents pin an Active current Version;
      Active Governance Heads pin an Approved Profile. Verify 001→004 Up, real
      constraint failures, 004-only Down, and catalog cleanup on PostgreSQL 16.
- [x] **15.1B — Identity services:** follow
      [`phase-15-1b-identity-services-plan.md`](./phase-15-1b-identity-services-plan.md)
      to add Argon2id credentials, mailbox invite acceptance, recovery, independent
      login, session revocation, and audit-safe token handling.
- [x] **15.1C — Team services:** follow
      [`phase-15-1c-team-services-plan.md`](./phase-15-1c-team-services-plan.md)
      to add Team/Membership repositories and APIs, mailbox Invite delivery,
      versioned Outbox-backed Membership changes, and transactional last-Admin
      protection.
- [ ] **15.1D — Knowledge services:** follow
      [`phase-15-1d-collection-document-consent-plan.md`](./phase-15-1d-collection-document-consent-plan.md)
      to add Collection and Document/Version CRUD, owner/Admin authorization,
      locked File binding, Governance/Consent updates, durable Processing Jobs,
      and transactional Outbox writes.
- [ ] **15.1E — Isolation gate:** pass two-user/two-team Personal/Team tests,
      cross-scope `404` behavior, Consent-purpose tests, revision fencing,
      deletion, idempotency, and Outbox producer/source-recovery prerequisites.
      Consumer replay and projection reconstruction remain gated on the real
      Python RAG worker and checkpoint schema.

## Promotion and Rollback

Each slice requires focused tests, `go test ./...`, an independent xhigh review,
and tracking evidence before its checkbox is marked complete. Migrations move
forward in production; the `004` Down path is for pre-release/local rollback
only. Python RAG implementation begins only after 15.1E passes.
