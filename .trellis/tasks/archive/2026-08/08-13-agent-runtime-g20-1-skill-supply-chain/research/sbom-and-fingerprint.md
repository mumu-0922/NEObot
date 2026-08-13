# SBOM and Fingerprint Authority

## Identity split

G20.1 needs three distinct immutable identities:

```text
sourceArtifactSHA256  exact bytes fetched/uploaded from one immutable source ref
packageFingerprint   canonical package path+byte inventory
runtimeFingerprint   package fingerprint + validated immutable runtime surface
sbomFingerprint      exact canonical CycloneDX JSON bytes
```

Source metadata and filenames never override these values. Reusing the same
claimed immutable source reference with different source bytes is source drift
and must return a conflict without updating the earlier candidate.

## Circularity found in the Phase 0 schema

The current Phase 0 `neo.runtime.json` schema requires the package itself to
contain `packageFingerprint`, `runtimeBundleFingerprint`, `sbomFingerprint`,
and `admissionId`. If `neo.runtime.json` participates in the package inventory,
the package would have to contain the hash of bytes that contain that same
hash. This has no usable deterministic construction and would tempt an unsafe
exception that excludes or redacts manifest bytes from package identity.

Recommended correction:

- candidate `neo.runtime.json` contains declarations only: package name and
  version, pinned runtime image/platform/user, entrypoints, immutable declared
  dependencies, capability/egress/secret requests, resources, and limits;
- the server-generated immutable package/admission record binds source ref,
  source artifact hash, package/runtime/SBOM fingerprints, candidate/admission
  ID, reviewer, and decision revision;
- the full manifest bytes remain inside the package fingerprint, so any
  declaration change creates a new package identity.

This preserves the Phase 0 trust model while removing the circular field
dependency.

## Canonical package fingerprint

Use SHA-256 over a versioned domain and sorted inventory records, for example:

```text
neo.skill-package/v1\0
<path-length>:<path>\0<size>\0<file-sha256>\n
...
```

The API renders it as `sha256:<64 lowercase hex>`. ZIP timestamps, owner bits,
entry order, compression level, source URL, display metadata, and upload
filename do not participate. Path and file bytes do.

## Runtime fingerprint

Instruction-only Agent Skills may omit `neo.runtime.json`; they get no runtime
fingerprint and cannot launch code. When the manifest exists, hash a canonical
JSON projection of the complete validated runtime declaration together with
the package fingerprint under `neo.skill-runtime-bundle/v1`. Images and every
declared dependency require immutable digests. G20.1 does not build or execute
the later OCI Runtime Bundle; G20.3 must verify the same fingerprint while
building/launching it.

## CycloneDX

Generate deterministic CycloneDX 1.6 JSON containing:

- the Skill application component with name/version/package fingerprint;
- one file component per canonical path and SHA-256;
- the pinned runtime image and explicitly declared dependencies when present;
- a dependency edge from the Skill component to those components;
- no host path, credential, prompt, user content, live timestamp, or random
  serial number.

Canonical JSON bytes are stored under the SBOM hash. Replaying validation from
the same package files must reproduce package/runtime/SBOM fingerprints byte
for byte.

