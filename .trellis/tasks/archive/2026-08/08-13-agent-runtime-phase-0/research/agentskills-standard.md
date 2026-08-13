# Agent Skills Standard Research

## Source

```text
specification: https://agentskills.io/specification.md
captured: 2026-08-13
```

## Standard Package Contract

- A Skill is a directory with a required `SKILL.md`.
- `SKILL.md` begins with YAML frontmatter.
- Required frontmatter fields are `name` and `description`.
- Optional standard fields include `license`, `compatibility`, `metadata`, and
  experimental `allowed-tools`.
- Common optional directories are `scripts/`, `references/`, and `assets/`.
- The standard encourages progressive disclosure: discover name/description,
  then load full instructions/supporting files only when relevant.

## Neo Compatibility Decision

Neo accepts the standard package layout without redefining `SKILL.md`. Runtime
and security declarations live in a separate `neo.runtime.json` governed by the
Neo Runtime Manifest schema. This avoids polluting standard frontmatter while
allowing immutable runtime, resource, egress, secret and capability requests.

`allowed-tools` has no authorization power. It is untrusted package metadata
that may help admission compare declared intent, but actual execution is the
intersection of:

```text
admitted package/runtime fingerprint
AND server-issued Capability Grant
AND current user/project policy
AND Kill Switch/revocation state
AND Runner/Sandbox enforcement
```

## Admission Implications

- Validate required frontmatter and package naming before deeper inspection.
- Normalize paths without following symlinks outside the package root.
- Bound file count, individual size, expanded size and compression ratio.
- Treat Markdown, YAML, scripts, references, assets and metadata as untrusted.
- Hash canonical file paths plus bytes into the package fingerprint.
- Generate an SBOM for executable/runtime dependencies separately from the
  instruction package inventory.
- Never resolve floating dependencies or Git refs during execution.

## Conclusion

Use Agent Skills as the interoperability layer, not the runtime security model.
Neo's separate manifest and server grants supply the missing execution and
authorization contracts.
