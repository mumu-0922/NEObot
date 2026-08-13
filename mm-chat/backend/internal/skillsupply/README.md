# skillsupply

`skillsupply` owns the G20.1 server-authoritative Skill package supply chain.
It accepts only server-derived official, exact LobeHub version, exact GitHub
commit, or authenticated ZIP candidates; validates them in memory without
execution; writes immutable quarantine/package/SBOM objects; and persists
administrator admissions plus owner-bound install references.

## Boundaries

- Assistant presets, legacy browser text Skills, MCP Tools, credentials,
  models, grants, Runs, and Sandboxes are outside this package.
- `SKILL.md` and `neo.runtime.json` are untrusted declarations.
- `allowed-tools` is display-only and never creates Tool authority.
- G20.1 cannot launch a candidate. `/v1/code/executions` remains unavailable.

## API

The handler serves authenticated routes below `/v1/skills`:

- administrator candidate creation/detail/review under `/candidates`;
- admitted Store list/detail/install under `/store`;
- current-owner list/uninstall under `/library`.

All JSON bodies are bounded and strict, responses are `no-store`, admission is
fingerprint/revision fenced, and raw instructions, object keys, credentials,
host paths, package bytes, and SBOM bytes are never projected.

## Verification

```bash
bash mm-chat/scripts/verify-skill-supply-chain.sh
bash mm-chat/scripts/verify-skill-supply-chain-postgres17.sh
```

The first gate is offline. The second starts a disposable PostgreSQL 17
container and proves migration replay, grants, source drift, review CAS,
ownership, install/uninstall, guarded down, and clean re-up.

See [DESIGN.md](./DESIGN.md) for the trust and identity model.
