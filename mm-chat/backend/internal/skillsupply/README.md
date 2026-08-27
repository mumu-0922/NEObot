# skillsupply

`skillsupply` owns the server-authoritative Skill package supply chain. Its
user-facing catalog is the fixed `openai/skills` `skills/.curated` directory.
It also accepts an exact GitHub Skill directory, plus the legacy AIHero
adapter, and resolves mutable refs to exact commits before validation. Every
source is validated in memory without execution, written as immutable
quarantine/package/SBOM objects, and persisted as an owner-private installation.
Administrator admissions remain available as an internal compatibility path.

## Boundaries

- Assistant presets, legacy browser text Skills, MCP Tools, credentials,
  models, grants, Runs, and Sandboxes are outside this package.
- `SKILL.md` and `neo.runtime.json` are untrusted declarations.
- `allowed-tools` is display-only and never creates Tool authority.
- Candidate admission itself never executes package bytes. Installed packages
  become eligible only through the ordinary Chat Agent `local_direct` runtime.

## API

The handler serves authenticated routes below `/v1/skills`:

- administrator candidate creation/detail/review under `/candidates`;
- fixed curated catalog list/detail/install under `/catalog`;
- legacy admitted Store list/detail/install under `/store`;
- current-owner list/uninstall under `/library`.
- owner-authorized, revision-CAS conversation selection under
  `/conversations/{conversationId}/selection`.

All JSON bodies are bounded and strict, responses are `no-store`, admission is
fingerprint/revision fenced, and raw instructions, object keys, credentials,
host paths, package bytes, and SBOM bytes are never projected.

The catalog lists only the bounded fixed GitHub directory. Detail resolves
`main` to a 40-character commit, selects one exact Skill directory, and returns
validated display metadata. Install must repeat that exact commit and package
fingerprint; source drift is rejected before ingestion.

An explicit GitHub tree or `SKILL.md` blob link uses the internal
`InstallDirectSkillLink` boundary. It requires an unambiguous Skill directory,
pins a mutable ref to a 40-character commit, and installs a `validated`
candidate scoped to the current owner. The legacy AIHero adapter enters the
same boundary. Neither path executes npm/Git/Shell commands, requires
administrator review, or publishes the candidate in the legacy Store.

Installed inventory and conversation selection are separate. Agent Runs
materialize the conversation's selected Skills plus at most two relevant,
already-installed run-only activations. Automatic activation is recorded in the
frozen Runtime Resource Snapshot as `agent_auto` and is never written back to
the conversation selection.

## Verification

```bash
bash mm-chat/scripts/verify-skill-supply-chain.sh
bash mm-chat/scripts/verify-skill-supply-chain-postgres17.sh
```

The first gate is offline. The second starts a disposable PostgreSQL 17
container and proves migration replay, grants, source drift, review CAS,
ownership, install/uninstall, guarded down, and clean re-up.

See [DESIGN.md](./DESIGN.md) for the trust and identity model.
