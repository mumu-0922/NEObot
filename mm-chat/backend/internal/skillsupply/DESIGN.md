# Skill Supply Chain Design

## Goals

- Establish a Codex-style curated catalog and immutable package authority.
- Make source drift, package identity, runtime identity, and SBOM identity
  independently replayable.
- Preserve a hard no-execute boundary until later Agent Runtime groups pass
  their isolation and launch gates.

## Ingestion pipeline

```text
server-derived exact source
  -> bounded ZIP bytes
  -> in-memory archive/path/mode/size validation
  -> root SKILL.md + optional strict neo.runtime.json
  -> sorted path/size/file-hash inventory
  -> deterministic canonical ZIP + CycloneDX 1.6 SBOM
  -> content-addressed object writes
  -> immutable package + exact-source candidate transaction
```

Discovery has two explicit source adapters. OpenAI Curated is fixed to
`openai/skills/skills/.curated` at `main`; detail resolves `main` to a full
commit and install is fenced by that commit plus the displayed fingerprint.
LobeHub search/category/detail reuses the Backend-owned Marketplace M2M client.
Its install re-resolves an exact SemVer before calling the exact-version ZIP
download; mutable `latest` authority never reaches the ingestion pipeline.
Locale and sort values cross the public API only through fixed allowlists, so
the browser cannot turn Marketplace query parameters into a generic proxy.
Both adapters normalize to Neo Chat DTOs and converge only at archive
validation and owner-private persistence.

For an explicit LobeHub page, GitHub tree, or `SKILL.md` blob link, the source adapter derives
one exact Skill subdirectory and resolves a mutable ref to a full commit before
entering the same bounded ZIP pipeline. The legacy AIHero adapter parses its
restricted coordinate as data and follows the same path. No command from a
page or repository is executed.

LobeHub source identity is `lobehub:<identifier>@<version>`, while the root
manifest name remains package identity. They are deliberately separate because
Marketplace identifiers are not required to equal `SKILL.md` names.

No phase extracts files to a working directory or invokes a shell, package
manager, interpreter, build, hook, test, or candidate entrypoint.

## Identity model

| Identity | Input | Purpose |
| --- | --- | --- |
| source artifact | exact received ZIP bytes | detect immutable-source drift |
| package | domain-separated sorted path/size/file-hash inventory | ignore ZIP timestamps/order/compression |
| runtime bundle | package fingerprint plus canonical validated runtime declaration | bind the future runtime surface |
| SBOM | exact deterministic CycloneDX JSON bytes | verify stored component inventory |

`neo.runtime.json` contains declarations only. Source coordinate,
package/runtime/SBOM fingerprints, candidate/admission ID, reviewer, and
decision revision live in the server-generated envelope. This removes the
self-referential hash problem while keeping every manifest byte in package
identity.

## Durable authority

- `skill_package_versions` is immutable and content-addressed.
- `skill_package_candidates` owns one exact source coordinate and either a
  fingerprint-bound administrator decision or one owner-private `validated`
  direct candidate.
- `skill_installations` owns a user's exact public-admitted or same-owner
  private-validated package reference.
- `skill_conversation_selections` owns one revision-CAS selection per
  `(conversation, owner)`; child rows bind the exact owner installation and
  package fingerprint. Conversation deletion or uninstall cascades stale pins.
- A composite foreign key plus authority trigger prevents an installation from
  binding a different package, a non-admitted public candidate, or another
  owner's private candidate even outside the repository.
- PostgreSQL `jsonb` normalizes stored metadata, so immutable replay compares
  decoded `allowed_tools` and `capability_requests` structures rather than raw
  JSON bytes; equivalent candidates remain idempotent without weakening the
  fingerprint and scalar-field comparisons.
- Uninstall deletes only the current owner's reference; immutable candidates,
  versions, SBOMs, and review history survive.

## Runtime projection

Inventory, durable Conversation selection, and Run activation are separate
authorities. Agent Runs materialize only user-selected installations plus at
most two bounded lexical `agent_auto` matches from the same owner's installed
library. Automatic matches are recorded in the frozen Run resource report and
never written back to the Conversation selection.

## Failure ordering and rollback

Objects are written before the database transaction. Failure can therefore
leave unreachable content-addressed bytes, but can never create admitted
state. Repeated object writes are safe. Migration down refuses while any Skill
row exists. Application rollback keeps migration `083` and the object prefixes
in place; paired PostgreSQL/MinIO backup and restore is the data rollback unit.

## Admission policy

Instruction-only packages are reviewable but have no runtime fingerprint or
launch path. Executable packages are reviewable only when they are the
server-owned official synthetic fixture, request read-only workspace actions,
and request no Egress or Secret slots. Every other executable candidate stays
quarantined/ineligible for installation.

An explicit owner-private direct package is not a Store admission. Only an
instruction-only package that passes the existing archive/frontmatter/name
validation may be installed this way. The candidate remains `validated`, has
no reviewer, is excluded from Store queries, and is installable only when its
`owner_user_id` equals the installation owner.
