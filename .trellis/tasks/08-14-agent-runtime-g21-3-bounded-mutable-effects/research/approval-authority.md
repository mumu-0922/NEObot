# G21.3 approval authority research

## Question

How can a headless production canary satisfy explicit mutable approval without
letting the canary process approve its own effects?

## Existing repo facts

- Migration `086` supports `automatic`, `once`, `per_commit` and `denied`
  approval classes. `ClaimCommit` rechecks the exact approval row, immutable
  intent fingerprint, live lease, Grant, Registry and Kill Switch.
- The G21.2 controller owns the Runner authority private key. Reusing that key
  as human approval authority would collapse two trust domains.
- Activation evidence is secure-file and release/plan fingerprint bound, but a
  mutable action needs a separate positive authorization fact.

## Compared approaches

### A. Separate offline-signed approval document — recommended

An operator signs one short-lived strict approval document with a key that is
not mounted into the canary. The process mounts only the public key and the
signed document. The activation record binds both fingerprints. The document
binds release, migration head, target, plan, caller, exact request/idempotency
identity, action/resource, actor/reason and validity window.

After Prepare returns the durable intent, the controller verifies the document
and records exactly one `per_commit` approval against that intent. Exact replay
reuses the same immutable approval and cannot dispatch a second effect.

### B. Automatic canary approval

This is operationally simpler but does not prove the approval boundary required
by G21.3 and would make the mutable action no stronger than G21.2 Artifact
publication. Rejected.

### C. Store an approval private key in the canary

This would allow the executor to manufacture its own authority and turns file
possession into both request and approval. Rejected.

## Required validation

- Strict schema and Ed25519 verification; unknown fields, signature tampering,
  placeholder keys, symlinks, insecure ownership/mode, stale/future windows and
  binding drift fail before database or Runner access.
- Approval key, document and Runner authority key must be pairwise distinct.
- Approval is `per_commit`, exact-action only and cannot be reused with another
  request, plan, resource, release or idempotency key.
- Only sanitized fingerprints, actor ID and reason code may be durable; no raw
  file content, key bytes or absolute path enters logs/evidence.
