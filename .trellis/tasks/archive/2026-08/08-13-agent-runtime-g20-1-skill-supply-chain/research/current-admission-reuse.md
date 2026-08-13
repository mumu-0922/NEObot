# Current Admission and Authority Reuse

## Scope

This note records the existing Neo Chat seams that G20.1 can reuse without
merging Assistant, Skill, and Tool authority.

## Existing patterns

### Assistant Store

`mm-chat/backend/internal/agents/` already proves the following patterns:

- the authenticated Go API is the only mutation surface;
- `AUTH_BOOTSTRAP_USER_ID` is the deployment administrator;
- upstream detail is normalized and canonically fingerprinted before review;
- administrator review persists a bounded immutable snapshot;
- ordinary-user install requires the exact admitted fingerprint;
- library rows are owner-bound and mutation/deletion uses revision CAS;
- Store declarations such as `required_tools` remain display-only and never
  install or enable a Tool.

The Skill Store should reuse these invariants, not the Assistant tables or
types. A Skill is a package/version with provenance and object bytes, while an
Assistant is a prompt preset.

### MCP Marketplace and LobeHub transport

`mm-chat/backend/internal/mcpclient/marketplace_lobehub.go` already owns:

- same-origin HTTPS validation and bounded redirects;
- M2M token acquisition with a short-lived signed client assertion;
- token singleflight/cache, one retry after `401`, response limits, and a
  bounded metadata cache;
- backend-only Marketplace credentials;
- the transport shared by the Assistant adapter through
  `FetchAgentMarketJSON`.

G20.1 should extend this owner with narrowly scoped Skill metadata/download
methods. It must not introduce a second Marketplace token cache or expose the
machine credential to the Skill package, browser, or object store.

### Object storage

`mm-chat/backend/internal/storage.ObjectStore` provides safe generated-key
`Put/Get/Delete` operations for local and MinIO/S3 backends. Local and S3
implementations reject absolute paths, traversal, backslashes, drive-like
keys, and malformed path components.

The Skill supply chain should own content-addressed keys under distinct
prefixes:

```text
skill-quarantine/sha256/<source-artifact-hash>.zip
skill-packages/sha256/<package-fingerprint>.zip
skill-sboms/sha256/<sbom-fingerprint>.cdx.json
```

The Sandbox never receives object-store credentials; that remains a later
Broker/Runner concern.

### Strict external JSON

`mm-chat/backend/internal/strictjson` rejects bounded-body violations,
duplicate keys at any depth, unknown struct fields, and trailing JSON values.
It is suitable for `neo.runtime.json`. Domain validation and cross-document
authority remain the Skill supply-chain service's responsibility.

### Database and release gates

Migration `082_assistant_library` and
`scripts/verify-assistant-store-postgres17.sh` provide the nearest examples for:

- additive tables with explicit checks and least-privilege runtime grants;
- owner-bound repository integration tests;
- partial uniqueness and CAS assertions;
- disposable PostgreSQL 17 fresh/replay/down/up verification.

The current paired PostgreSQL/MinIO production backup already captures all
database tables and bucket objects. G20.1 must extend restore sampling and the
acceptance query so Skill package, SBOM, and quarantine coordinates are proven
after restore.

## Reuse decision

Create a separate `internal/skillsupply` package and `083` migration. Reuse the
M2M transport, object-store interface, strict JSON decoder, authenticated route
wiring, administrator identity, CAS conventions, and PostgreSQL drill style.
Do not reuse Assistant/MCP persistence tables, installation rows, grants, or
runtime execution paths.

