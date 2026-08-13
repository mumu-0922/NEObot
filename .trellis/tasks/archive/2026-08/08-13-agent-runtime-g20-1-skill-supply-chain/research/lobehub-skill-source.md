# LobeHub Skill Source Evidence

## Live evidence captured 2026-08-13

- `https://lobehub.com/zh/skills` redirects to
  `https://market.lobehub.com/s/skills`.
- The served Skill describes LobeHub Market as a ZIP Skill marketplace with
  `SKILL.md` plus resources and recommends the official
  `@lobehub/market-cli`.
- npm `@lobehub/market-cli@0.0.41` is MIT and depends on
  `@lobehub/market-sdk@^0.40.1`.
- npm `@lobehub/market-sdk@0.40.1` has integrity
  `sha512-KUBfHu7flm/M6mtnlJfHYe6ypUjYgnWloxQ+3XSdbq9FspxMbHXzIhc7AS3+56r2LkonZXlZX8oe1eJ7W6EaZQ==`
  and points at `lobehub-biz/lobehub-market`.

## API shape from the pinned SDK

The pinned SDK uses:

```text
GET /api/v1/skills?...                        list
GET /api/v1/skills/<identifier>?version=...  detail
GET /api/v1/skills/<identifier>/download?version=...  ZIP bytes
GET /api/v1/skills/<identifier>/versions
```

List/detail models expose identifier, exact version, validation/official
display flags, manifest metadata, resources with file SHA-256/size, and GitHub
metadata. These fields remain untrusted discovery/provenance hints. Neo must
download the exact requested version, independently inspect every ZIP entry,
and derive its own fingerprints/SBOM.

The current CLI passes Market credentials to the SDK, downloads the ZIP, then
extracts entries to a local Skill directory. Neo must not copy that extraction
behavior: it should reuse only the authenticated transport and keep all
candidate processing in the no-execute in-memory/quarantine pipeline.

## Adapter decision

Extend `mcpclient.LobeHubMarketplace` with a bounded, no-cache exact-version
Skill ZIP method so the existing M2M token owner is reused. The Skill adapter
requires a non-empty exact version; `latest` and omitted versions are rejected.
The immutable source ref is `lobehub:<identifier>@<version>`. A later response
whose raw source hash differs for that ref is source drift.

