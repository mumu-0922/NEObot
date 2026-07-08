# mm-chat Docs

This directory is the documentation control plane for the `mm-chat` refactor. Keep planning, inventories, contracts, deployment notes, and progress tracking here so future implementation work is easy to audit.

## Categories

| Category | Path | Purpose |
|---|---|---|
| Architecture | [`architecture/`](./architecture/) | Target architecture, migration phases, data/storage boundaries, rollout and rollback strategy. |
| Inventory | [`inventory/`](./inventory/) | Static analysis of the existing Neo Chat app before migration. |
| Tracking | [`tracking/`](./tracking/) | Progress checklist and chronological process log. |
| Contracts | [`contracts/`](./contracts/) | Future frontend API, backend API, event, and data contracts. |
| Deployment | [`deployment/`](./deployment/) | Docker Compose, backup, restore, release, rollback, and operations guides. |

## Update Rule

When a task completes:

1. Update [`tracking/progress.md`](./tracking/progress.md).
2. Add a dated entry to [`tracking/process.md`](./tracking/process.md).
3. Put new docs in the matching category instead of the workspace root.
