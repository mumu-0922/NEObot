# LobeHub Marketplace Adapter Research

## Verified upstream contract

- `https://lobehub.com/mcp` redirects to
  `https://market.lobehub.com/s/plugins`.
- Official packages checked on 2026-08-11:
  `@lobehub/market-cli@0.0.40`, `@lobehub/market-sdk@0.40.0`, and
  `@lobehub/market-types@1.17.1`.
- The SDK uses `GET /api/v1/plugins`,
  `GET /api/v1/plugins/{identifier}`, and
  `GET /api/v1/plugins/{identifier}/manifest`.
- Live verification on 2026-08-11 showed the current detail endpoint succeeds
  with `locale=zh-CN` but fails when Neo Chat forwards `version=2.2.0`. The
  public item version must therefore be checked against the returned current
  detail locally; an upstream `version` query is not part of the usable
  contract.
- Category counts use `GET /api/v1/plugins/categories?locale=&q=` and list
  filtering uses the bounded `category` query parameter on
  `GET /api/v1/plugins`.
- Marketplace item icons are either short emoji/text or external image URLs;
  current high-ranked items commonly use HTTPS GitHub avatar URLs.
- Anonymous plugin listing currently returns `401 Missing bearer token`.
- M2M authorization posts `client_credentials` to `/oauth/token` with an
  HS256 JWT client assertion whose issuer/subject are the client ID and whose
  audience is the exact token endpoint.
- Detail metadata exposes `deploymentOptions`. Each option contains
  `connection.type` (`http|stdio|sse`), optional URL/command/args/config schema,
  an installation method, and an optional recommended flag.

## Design consequence

Neo Chat uses a bounded Go adapter and Docker-secret-backed M2M credentials.
LobeHub remains an untrusted discovery source. Install authority is derived
only after the backend re-fetches the current authoritative detail without an
upstream version parameter, compares its returned version to the client-selected
exact version, and validates the deployment hash. Public HTTPS HTTP options use
existing private MCP creation/SSRF checks; exact reviewed stdio fingerprints
resolve to immutable Runner artifacts. No Marketplace command is ever executed
and no extra resident container is introduced.
