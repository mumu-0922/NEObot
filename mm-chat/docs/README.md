# mm-chat Docs

This directory is the documentation control plane for the `mm-chat` refactor. Keep planning, inventories, contracts, deployment notes, and progress tracking here so future implementation work is easy to audit.

## Categories

| Category     | Path                               | Purpose                                                                                        |
| ------------ | ---------------------------------- | ---------------------------------------------------------------------------------------------- |
| Architecture | [`architecture/`](./architecture/) | Target architecture, migration phases, data/storage boundaries, rollout and rollback strategy. |
| Inventory    | [`inventory/`](./inventory/)       | Static analysis of the existing Neo Chat app before migration.                                 |
| Tracking     | [`tracking/`](./tracking/)         | Progress checklist and chronological process log.                                              |
| Contracts    | [`contracts/`](./contracts/)       | Future frontend API, backend API, event, and data contracts.                                   |
| Persistence  | [`persistence/`](./persistence/)   | Postgres schema, migration, projection, and runtime source-of-truth contracts.                 |
| Deployment   | [`deployment/`](./deployment/)     | Docker Compose, backup, restore, release, rollback, and operations guides.                     |

The active Tool integration is MCP. See
[`architecture/mcp-tools.md`](./architecture/mcp-tools.md),
[`contracts/mcp-tools-api.md`](./contracts/mcp-tools-api.md), and
[`deployment/mcp-runner.md`](./deployment/mcp-runner.md).

The current Agent is the ordinary Chat runtime in Agent mode. It combines the
installed Skill Store with bounded File, Terminal, Job, Browser/MCP, Goal, and
artifact Tools; `local_direct` needs no sudo, OCI Runner, or second control
application. See
[`architecture/agent-runtime.md`](./architecture/agent-runtime.md),
[`contracts/agent-runtime.md`](./contracts/agent-runtime.md),
[`contracts/local-skill-runtime.md`](./contracts/local-skill-runtime.md), and
[`deployment/agent-runtime.md`](./deployment/agent-runtime.md). Migration `098`
retires the disconnected Runner/Canary control-plane schema while preserving
Chat, Skill, File, MCP, Knowledge, and Memory authority.

## Update Rule

When a new plan or scope change appears:

1. Write it into an architecture, contract, deployment, or tracking document before implementation starts.
2. Mirror new phases or checklist items in [`tracking/progress.md`](./tracking/progress.md).
3. Add dated evidence to [`tracking/process.md`](./tracking/process.md) when work completes.
4. Put new docs in the matching category instead of the workspace root.

Current remaining-work authority lives in [`architecture/standalone-parity-sliced-cutover-plan.md`](./architecture/standalone-parity-sliced-cutover-plan.md), with the dedicated process log at [`tracking/standalone-parity-sliced-process.md`](./tracking/standalone-parity-sliced-process.md). The active G11.9 Knowledge product correction is specified in [`tracking/g11-knowledge-auto-rag-plan.md`](./tracking/g11-knowledge-auto-rag-plan.md) and recorded in [`tracking/g11-knowledge-auto-rag-process.md`](./tracking/g11-knowledge-auto-rag-process.md). Older phase plans remain supporting references for domain details.
