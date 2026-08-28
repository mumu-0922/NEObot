# Bug Analysis: LobeHub Skill install could discover but not materialize packages

## 1. Root Cause Category

- **Category**: B/E — Cross-layer contract plus implicit assumption.
- **Specific cause**: Marketplace discovery and package download were assumed
  to share one M2M authorization contract. LobeHub accepts the configured token
  for list/category but rejects it on `/download`. The first fallback then
  assumed an exact nested GitHub Skill could safely reuse whole-repository
  codeload, although repository size and Skill package size are different
  trust/bounds domains.

## 2. Why earlier fixes failed

1. Treating zero `resources` as the cause addressed a visible detail symptom,
   but the same token failed `/download` for populated Skills too.
2. Reusing direct codeload correctly pinned source identity but had incomplete
   scope: `openclaw/openclaw` exceeded 96 MB before timeout while Neo Chat's
   source archive limit is 32 MiB and the requested Skill is about 3 KiB.

## 3. Prevention mechanisms

| Priority | Mechanism | Specific action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Separate Marketplace product identity from package transport and rebind only after validation | Done |
| P0 | Runtime bounds | Enumerate only the exact pinned Skill directory; reject symlink/submodule/limit violations | Done |
| P0 | Integrity | Verify every raw file against the Git blob SHA declared by GitHub | Done |
| P0 | Tests | Assert zero legacy `/download`, zero codeload, pre-I/O source mismatch rejection, and zero mutation on drift | Done |
| P1 | Documentation | Record outbound hosts and the exact subtree contract | Done |

## 4. Systematic expansion

- **Similar issues**: Any marketplace adapter that conflates browse credentials
  with package credentials can fail after discovery succeeds.
- **Design improvement**: Keep source identity, immutable transport coordinate,
  and validated package fingerprint as separate values until ingestion.
- **Process improvement**: Live-probe both the advertised transport endpoint
  and its payload size before declaring an existing source adapter reusable.

## 5. Knowledge capture

- [x] Updated `.trellis/spec/backend/skill-supply-chain.md`.
- [x] Updated Agent Runtime architecture and deployment documentation.
- [x] Added focused supply-chain regression tests.
- [x] Confirmed no generated spec-template mirror exists in this repository.
