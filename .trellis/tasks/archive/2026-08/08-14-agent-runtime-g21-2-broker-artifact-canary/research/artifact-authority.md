# G21.2 Artifact authority research

## Existing publication primitive

`agentbroker.ArtifactPublisher` already implements the decisive byte-ordering
contract:

1. reject stale or unauthorized metadata before opening quarantine;
2. read one bounded snapshot of quarantine bytes;
3. recheck exact size and SHA-256;
4. scan those same bytes;
5. put the immutable object;
6. attach the PostgreSQL row;
7. delete the object if row attachment fails;
8. delete quarantine after authoritative publication.

It caps a publication at 32 MiB. The missing production pieces are a live
authority resolver and PostgreSQL `AttachArtifact` implementation.

## Persistence gap

Migration `090` created `agent_artifacts`, but the table is owned by
`agent_product_owner`. No worker role has direct DML and the only exposed
function is the user-facing read projection. Granting `agent_effect_control`
INSERT would collapse effect and Artifact authority and bypass live
Attempt/Grant/Kill-Switch checks.

## Recommended migration 091

Add `091_agent_artifact_publication` with a dedicated NOLOGIN
`agent_artifact_control` role and exact `SECURITY DEFINER` functions owned by
the existing product owner:

- authorize one candidate before object upload;
- attach one exact object after upload with the same checks repeated under
  row locks;
- replay the same Artifact row exactly and reject ID/name/object collisions.

The functions must recheck:

- the matching committing Broker intent and exact Tool/action;
- user, Run, Attempt, current generation and live lease;
- Run snapshot plus Grant/Registry fingerprints and Kill-Switch epoch;
- non-revoked Grant;
- frozen snapshot Artifact max bytes and exact allowed media type;
- deterministic object key, bounded name/media/size and SHA-256.

The control role receives function execution only: no direct table DML, no
owner membership, no schema CREATE and no access to other product mutations.
The G21.2 LOGIN may inherit this control role alongside the three existing
runtime roles; no other production principal receives it.

## Quarantine and object store

The G21.2 canary uses a dedicated private quarantine mount and the normal
server-side object store. The Sandbox has only its per-Attempt write-only Unix
Artifact socket and no object-store credential. Exact-host evidence combines
the already verified Runner intake boundary with a canary fixture copied into
the matching Attempt quarantine; it never mounts the object store or database
into the Sandbox.

The canary scanner is intentionally narrow: only the plan's exact text/JSON
media allowlist and bounded bytes are accepted. General malware/content policy
and user Artifact publication remain later work.

## Rollback and backup

Migration 091 adds no new Artifact table and does not delete Artifact rows on
down. Rollback revokes and drops only the attach/authorize functions and narrow
role after G21.2 workers are stopped and role membership is removed. Existing
`090` guarded rollback continues to protect all Artifact rows.

PostgreSQL and object storage remain one backup/restore unit. Restore starts
with Runtime and Broker canaries off, verifies every `agent_artifacts.object_key`
against the object mirror, removes unreferenced canary objects, then re-enables
only after a fresh exact-host activation.
