# Bug Analysis: LobeHub Skill rejected by scalar-only metadata parser

## 1. Root Cause Category

- **Category**: E — Implicit assumption, with D — Test coverage gap.
- **Specific cause**: Neo Chat assumed every `SKILL.md` metadata value was a
  scalar string. The public Skill ecosystem also uses namespaced nested maps
  for platform hints, but the Marketplace install fixture only contained
  `metadata.version`, so the incompatible assumption survived integration
  tests.

## 2. Why Earlier Fixes Did Not Finish the Flow

1. Replacing LobeHub's unavailable package endpoint fixed transport, but the
   pinned GitHub bytes still entered the unchanged scalar-only parser.
2. Existing Marketplace tests proved exact source identity and persistence
   with a Neo-native fixture, not the OpenClaw manifest shape returned by the
   live catalog.

## 3. Prevention Mechanisms

| Priority | Mechanism | Specific action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Separate structurally accepted opaque metadata from projected Neo authority | Done |
| P0 | Regression | Use OpenClaw-shaped metadata in the Marketplace install fixture | Done |
| P0 | Security | Reject aliases, duplicate keys, typed top-level scalars, and bounded-resource violations | Done |
| P1 | Contract | Record ecosystem metadata compatibility in the Skill supply-chain spec | Done |

## 4. Systematic Expansion

- **Similar issues**: third-party Skills can add new namespaced metadata without
  changing their executable instructions.
- **Design improvement**: parser compatibility and runtime authority are now
  separate decisions; accepting syntax cannot silently grant capabilities.
- **Process improvement**: external-source integration fixtures must preserve
  at least one real upstream manifest shape instead of only synthetic native
  manifests.

## 5. Knowledge Capture

- [x] Updated `.trellis/spec/backend/skill-supply-chain.md`.
- [x] Added parser and full Marketplace-flow regression coverage.
- [x] Kept all third-party installer declarations inert.
