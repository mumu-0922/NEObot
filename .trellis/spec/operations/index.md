# Operations Development Guidelines

> Executable repository, CI, release, and runtime-state contracts.

## Guidelines index

| Guide                                                     | Scope                                                                                                     |
| --------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| [Repository root boundary](./repository-root-boundary.md) | The thin Git root, `mm-chat/` product root, automation paths, runtime protection, and verification gates. |

## Pre-development checklist

For repository layout, GitHub automation, build entrypoint, or cleanup changes:

1. Read
   [`repository-root-boundary.md`](./repository-root-boundary.md).
2. Confirm `mm-chat/` remains the only product root.
3. Identify protected runtime paths before changing files.
4. Define backup, rollback, and clean-copy verification before deletion.

## Quality check

- Run `bash mm-chat/scripts/verify-standalone.sh --full`.
- Run the frontend, backend, and RAG component checks.
- Render Compose with both the example and active environment files.
- Prove `mm-chat/` tracked source and protected runtime paths were not changed
  by a root-only cleanup.
- Validate GitHub Actions syntax and live health when deployment entrypoints
  change.

**Language**: All documentation should be written in English.
