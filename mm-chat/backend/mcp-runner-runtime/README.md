# MCP Runner Runtime

This directory is the reviewed dependency lock for MCP server artifacts shipped
inside the optional Neo Chat MCP Runner image.

## Contract

- Every package is pinned to an exact version and integrity in
  `package-lock.json`.
- The Docker build uses `npm ci --omit=dev --ignore-scripts`; chat traffic never
  runs `npm`, `npx`, or another package downloader.
- Adding a package here does not make it executable. A matching reviewed entry
  must also exist in `mm-chat/mcp/manifest.json`.
- `@playwright/mcp` is pinned for the reviewed Browser artifact; its manifest
  uses an exact Tool allowlist because the upstream package also ships an
  RCE-equivalent unsafe-code Tool.
- The Runner starts an approved binary only on first use and reaps it according
  to the manifest lifecycle limits.

## Verification

```bash
npm ci --omit=dev --ignore-scripts
npm audit --omit=dev --audit-level=moderate
docker build --target mcp-runner -t mm-chat/mcp-runner:test ..
```

## Usage

This directory is not a standalone service. Build the parent backend
Dockerfile's `mcp-runner` target, then start that image with the reviewed
`mm-chat/mcp/manifest.json` and Runner token as documented in
`mm-chat/docs/deployment/mcp-runner.md`.

Do not run `npm update` as part of an unrelated change. Review the package
release, dependency diff, integrity, executable path, MCP behavior, and audit
result before changing the lock.

## Files

- `package.json`: exact approved direct artifacts.
- `package-lock.json`: complete transitive dependency and integrity lock.
- `DESIGN.md`: trust boundary and update rationale.
