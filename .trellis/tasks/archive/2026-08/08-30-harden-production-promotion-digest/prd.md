# Harden Production Promotion and Digest Release

## Goal

Turn the currently healthy local retained-tag deployment into a reproducible
production-promotion path that passes the repository's strict preflight,
selects immutable image digests, preserves the active runtime state, and keeps
an exact rollback route.

## What I already know

- The live stack is healthy on Backend version `skill-cleanup-ae221181`.
- The active `.env.single-server` predates newer promotion-only fields and the
  strict preflight stops first at missing `POSTGRES_DATA_DIR`.
- The current release uses a local retained Backend tag rather than a registry
  digest.
- The previous task produced a checksum-verified PostgreSQL/MinIO recovery set
  and retained the old Backend image.
- Real secrets, runtime data, user files, and the active env must remain outside
  Git.

## Confirmed constraints

- Repository scripts and `.env.single-server.example` define the desired
  production contract and should remain the authority.
- Configuration hardening will be implemented and verified locally without
  pushing images or changing the live deployment.
- Registry publication and live digest promotion remain a later, separately
  authorized operation requiring reviewed registry coordinates, credentials,
  and production-only values.

## Requirements (evolving)

- Inventory every strict-preflight field missing from the active env without
  printing secret values.
- Close the executable release gap: the image release bundle must include the
  repository's PostgreSQL retrieval image because strict preflight requires an
  immutable `POSTGRES_IMAGE` in addition to Backend, MCP Runner, Frontend, and
  RAG.
- Add hermetic focused coverage for release bundle completeness and failure
  behavior; no test may require registry access.
- Reuse existing release, image, Compose, backup, and rollback scripts instead
  of creating a parallel deployment mechanism.
- Preserve all protected runtime paths and avoid recreating live services while
  requirements are still being established.
- Fail closed when images are mutable, required paths are unsafe, checksums do
  not match, or rollback inputs are absent.

## Acceptance Criteria (evolving)

- [x] A sanitized gap report names every missing/invalid production field.
- [x] The release script selects all five required images and emits a complete
      immutable production image fragment in push mode.
- [x] Focused tests prove a failed/partial build cannot be mistaken for a
      complete promotion bundle.
- [x] A synthetic protected candidate passes strict preflight and production
      Compose rendering without using live secrets.
- [x] Every application and PostgreSQL image must resolve to an immutable
      registry digest before a live recreation is permitted.
- [x] Backup, restore, health, and rollback gates remain intact.
- [x] No secret value, registry credential, private chat data, or object key is
      committed or printed.

## Definition of Done

- Focused configuration/script tests pass.
- Compose production render and strict preflight pass against a hermetic
  candidate; live promotion remains blocked until the operator supplies the
  missing production values and explicitly authorizes registry publication.
- Operational documentation and Trellis specs match the executable contract.
- Changes are committed, the task is archived, and nothing is pushed unless
  explicitly authorized and a registry is configured.

## Out of Scope

- Rotating credentials or changing user/provider data.
- Deleting old images, backup sets, or runtime state.
- Pushing to an unknown registry.
- Editing `.env.single-server`, generating live Team keys, choosing the public
  invite URL, or recreating any live service without a separately confirmed
  production operation.

## Technical Approach

Extend the existing release authority instead of adding a second deployment
path. `release-images.sh` will treat the PostgreSQL retrieval runtime as the
fifth required release image and emit its immutable digest beside the four
application images. A hermetic fake-Docker test will exercise local, push, and
failure paths without network or registry credentials. The standalone gate and
deployment documentation will consume the same five-image contract.

## Decision (ADR-lite)

**Context**: Strict production preflight requires five immutable registry image
references, while the release script currently emits only four and the live
environment still uses local retained tags.

**Decision**: Complete and test the registry-ready release chain now, but do
not publish images, synthesize production secrets, alter the live env, or
recreate services in this task.

**Consequences**: The repository will have an executable, hermetically tested
promotion path and an exact sanitized gap inventory. Actual digest values and
live promotion remain blocked—correctly—on explicit operator authorization and
the missing production configuration.

## Research References

- [`research/promotion-gap.md`](research/promotion-gap.md) — live/preflight gap,
  missing PostgreSQL release artifact, documentation drift, and authority
  boundary.

## Technical Notes

- Relevant contracts start in `.trellis/spec/operations/` and
  `mm-chat/docs/deployment/`.
- Likely executable authorities include `preflight-single-server.sh`, image
  release scripts, Compose production overrides, and the env example.
- Research evidence is recorded in `research/promotion-gap.md`.
