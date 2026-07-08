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

When a new plan or scope change appears:

1. Write it into an architecture, contract, deployment, or tracking document before implementation starts.
2. Mirror new phases or checklist items in [`tracking/progress.md`](./tracking/progress.md).
3. Add dated evidence to [`tracking/process.md`](./tracking/process.md) when work completes.
4. Put new docs in the matching category instead of the workspace root.

Current post-Phase-10 planning lives in [`architecture/phase-11-plus-roadmap.md`](./architecture/phase-11-plus-roadmap.md).
