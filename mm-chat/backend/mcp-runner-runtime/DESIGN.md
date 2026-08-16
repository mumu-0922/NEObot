# MCP Runner Runtime Design

## Goal

Provide reproducible local MCP artifacts without allowing Marketplace metadata
or user input to execute arbitrary package-manager commands in the Go backend
or Runner.

## Architecture

```text
reviewed package.json + package-lock.json
                  |
                  v
       immutable Runner image
                  |
                  v
manifest artifact ID -> on-demand stdio child process
```

The Marketplace adapter matches an exact provider, item version, deployment
signature, and manifest artifact. The persisted user Server stores only the
approved artifact ID and provenance hash. Runner control calls name that
artifact ID; they never carry an argv array.

## Decisions

| Decision | Reason |
| --- | --- |
| One shared Runner container | Isolates third-party code without one resident container per MCP. |
| Exact lockfile and `npm ci` at image build | Prevents runtime dependency resolution and mutable `latest` installs. |
| `--ignore-scripts` | Blocks dependency lifecycle code during image assembly. |
| Separate manifest admission | A package being present is not sufficient execution authority. |
| Exact Browser Tool allowlist | Upstream Playwright additions, especially unsafe code/evaluation, never become execution authority automatically. |
| Browser process scoped to Chat Run | Page, Cookie, and in-memory profile state cannot cross Run boundaries. |
| Idle reap and process cap | Bounds memory/PID use while preserving warm reuse. |

## Non-goals

- Installing arbitrary LobeHub npm, Python, Docker, Git, or binary entries.
- Running `npx`, `uvx`, shells, or package downloads during chat traffic.
- Giving Marketplace metadata direct process or filesystem authority.

## Security

Package code is untrusted even after review. It runs non-root in the dedicated
read-only Runner container with bounded CPU, memory, PIDs, tmpfs workspaces,
private authenticated control traffic, no Docker socket, no host source mount,
and no database/object-store network. Version and lock changes require a fresh
audit and image rebuild.
