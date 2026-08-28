# Fix remaining LobeHub Skill install rejection

## Goal

Allow ordinary LobeHub/OpenClaw Skills with nested community metadata to pass
Neo Chat's structural `SKILL.md` validation without turning that untrusted
metadata into runtime authority.

## What I already know

- The browser sends the install request; three observed attempts returned HTTP
  `400` with an 80-byte response.
- That response size exactly matches Neo Chat's `INVALID_SKILL_PACKAGE` JSON.
- `openclaw-openclaw-summarize@1.0.3` resolves to the valid exact source
  `openclaw/openclaw/skills/summarize` at a pinned commit.
- Its root `SKILL.md` is 2,125 bytes and has valid name/description, but
  `metadata.openclaw` is a nested mapping containing lists and objects.
- `decodeFrontmatter` currently rejects any metadata value that is not a
  scalar string through `scalarMap`.

## Requirements

- Keep `metadata` itself restricted to a YAML mapping with bounded top-level
  entry count and unique scalar-string keys.
- Preserve bounded top-level string metadata entries in the existing
  `map[string]string`; this retains version and other current authority.
- Accept non-scalar metadata values as opaque package content and do not
  project them into `SkillMetadata`, runtime policy, permissions, tools,
  secrets, dependencies, or installation behavior.
- Keep name, description, license, compatibility, allowed-tools, archive,
  owner, fingerprint, and runtime-manifest validation unchanged.
- Malformed metadata roots, duplicate keys, invalid keys, excessive entries,
  invalid retained strings, aliases, or unsafe YAML structures remain rejected.
- Add regression coverage using OpenClaw-shaped nested metadata and prove the
  Marketplace install completes with its LobeHub source identity.

## Acceptance Criteria

- [x] Exact OpenClaw-shaped `summarize` metadata validates with an empty
  projected string metadata map.
- [x] Top-level string `metadata.version` remains preserved and authoritative.
- [x] Duplicate, non-mapping, aliased, or over-limit metadata is rejected.
- [x] Marketplace install succeeds through the pinned GitHub subtree and the
  next exact detail reports `installed: true`.
- [x] Backend focused/full tests, vet, security scan, deployment build, and
  health check pass.

## Definition of Done

- Parser and Marketplace regression tests cover the observed failure.
- Skill supply-chain spec records the opaque nested metadata rule.
- Backend is rebuilt and healthy.
- Work is committed, task archived, and journal recorded; no push.

## Technical Approach

Replace the all-or-nothing `scalarMap` metadata decoder with a bounded
metadata projector. It validates the top-level mapping and keys, retains only
safe scalar strings, recursively validates that ignored values contain no YAML
aliases and stay within explicit depth/node bounds, and ignores those values
after validation. No DTO or storage schema changes are required.

## Decision (ADR-lite)

**Context**: Agent Skills ecosystems use namespaced nested metadata for
platform-specific hints. Neo Chat needs structural compatibility but must not
grant those hints authority.

**Decision**: Accept and bound nested metadata as opaque bytes; project only
explicit safe scalar strings into Neo Chat's existing metadata map.

**Consequences**: Community/OpenClaw Skills install normally while nested
installer commands, requirements, secrets, and platform hints remain inert.

## Out of Scope

- Executing or interpreting OpenClaw install metadata.
- Installing declared binaries or Homebrew/npm packages.
- Changing the Skill Store UI or global announcement presentation.
- Weakening the root manifest, archive, runtime, or Tool-policy checks.
- Frontend/RAG/standalone full suites for this focused parser repair.

## Research References

- [`research/observed-parser-rejection.md`](research/observed-parser-rejection.md)
  — live request, source, and manifest evidence.
