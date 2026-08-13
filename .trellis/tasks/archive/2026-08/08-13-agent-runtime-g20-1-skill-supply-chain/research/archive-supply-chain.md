# Archive and Supply-Chain Validation

## Threat model

Every candidate archive, path, mode bit, `SKILL.md`, `neo.runtime.json`,
Markdown instruction, script, asset, and source label is untrusted data. The
Backend must never extract a candidate into its working tree and must never
invoke a package command, installer, hook, interpreter, or test during G20.1.

## Recommended bounded ZIP pipeline

1. Fetch or accept at most 32 MiB of ZIP bytes under a request deadline.
2. Hash the exact source artifact before parsing it.
3. Open with Go `archive/zip`; never use a shell archiver.
4. Reject:
   - absolute, traversal, backslash, NUL, invalid UTF-8, overlong, empty, or
     non-canonical paths;
   - duplicate paths and NFC/case-fold canonical collisions;
   - symlinks, hardlink-like entries, devices, sockets, FIFOs, and any file
     whose mode is not regular;
   - more than 2,048 files, more than 16 MiB per file, more than 128 MiB total
     expanded bytes, or an excessive expansion ratio;
   - missing/multiple package-root `SKILL.md` files;
   - malformed Agent Skills frontmatter or strict Neo Manifest JSON.
5. Read regular files through independent expanded-byte limits; do not create
   filesystem files.
6. Generate a canonical ZIP in sorted path order with fixed metadata.
7. Compute the package fingerprint from a domain-separated ordered inventory
   of canonical path, size, and byte hash, not from ZIP timestamps/compression.
8. Persist source ZIP, canonical package ZIP, SBOM, validation report, and
   PostgreSQL provenance. No ingestion step executes package content.

## Agent Skills compatibility

The live Agent Skills specification checked on 2026-08-13 requires:

- a package directory with `SKILL.md`;
- YAML frontmatter containing `name` and `description`;
- `name` of 1-64 lowercase alphanumeric/hyphen characters, without leading,
  trailing, or consecutive hyphens, matching the parent directory;
- description of 1-1024 characters;
- optional `license`, `compatibility`, string-to-string `metadata`, and
  experimental space-separated `allowed-tools`;
- arbitrary Markdown body and optional resources such as `scripts/`,
  `references/`, and `assets/`.

Unknown non-authoritative frontmatter may be retained only as untrusted source
metadata or ignored. `allowed-tools` participates in package bytes/inventory
but cannot become a Tool Registry entry, grant, credential, model setting, or
execution permission.

## Wrapped repositories

LobeHub/ZIP packages normally have their package files at ZIP root. GitHub
codeload ZIPs add a repository wrapper directory, so the exact-Git adapter must
provide the expected wrapper plus an explicitly validated optional package
subdirectory. The validator strips only that server-derived prefix. It must not
guess among multiple `SKILL.md` files.

## Malicious corpus

Focused tests should include traversal, absolute and Windows paths, duplicate
entries, Unicode/case collisions, symlink/special-file modes, invalid UTF-8,
too many files, oversized files, compression bombs, multiple roots, malformed
YAML, invalid names/descriptions, duplicate JSON keys, unknown manifest fields,
floating images/dependencies, inline-secret-like manifest fields, and benign
archives with different order/timestamps/compression that reproduce the same
fingerprint.

