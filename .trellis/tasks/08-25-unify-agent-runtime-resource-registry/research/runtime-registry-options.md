# Runtime Resource Registry options

## Compared systems

### Pi / pi-web

- `DefaultPackageManager` resolves configured global/project packages into one
  `ResolvedPaths` snapshot containing extensions, skills, prompts, and themes.
- `DefaultResourceLoader` loads that snapshot for `AgentSession`; package-level
  filters make enable/disable atomic across contributed resource kinds.
- Project extensions and project Skills are gated by an explicit trust store
  before any extension factory is imported or executed.
- Installed resources extend the session's Tool set; the session does not build
  a second browser-owned execution registry.

### Neo Chat today

- `chatToolRegistry` already combines retrieval, Goal, MCP, and local Tool
  definitions and fails closed on Tool-name collisions.
- MCP has a strong server-authoritative `PrepareRun` frozen snapshot, grants,
  per-workspace/conversation selection, per-tool disable, credentials, and
  remote/Runner execution.
- Skills have an owner installation library and immutable materialized
  `RuntimeSkill` catalog, but every installed Skill is globally available and
  Skill lifecycle is separate from MCP selection.
- The Handler prepares MCP and Skills independently, appends Skill prompt text,
  then constructs the Tool registry. There is no shared resource descriptor,
  revision, source/scope model, or diagnostic projection.
- The retired browser Plugin runtime must not be revived; current security
  contracts intentionally keep execution server-owned.

## Constraints

- Keep MCP and Skill persistence as the authorities; do not create a shadow
  package/selection database.
- Do not let browser state authorize Tools, Skills, credentials, or Host paths.
- Preserve frozen MCP Run authority, local Skill fingerprint revalidation,
  hidden legacy aliases, approval policy, and ordered Tool scheduling.
- A future package abstraction may group multiple resources, but Neo currently
  has no safe package format for executable extensions or prompts.

## Feasible approaches

### A. Runtime snapshot foundation (recommended MVP)

Introduce one immutable `agentRuntimeResourceSnapshot` built after MCP and
Skill preparation. It owns source/scope/kind/status descriptors, collision and
unavailable diagnostics, revision hashing, the exact Tool registry for a Step,
and the Skill catalog prompt. Existing MCP/Skill APIs remain authorities and
existing frontend management surfaces remain separate.

Pros:

- removes Handler-level assembly drift without schema migration;
- makes the current runtime testable and diagnosable as one unit;
- preserves all mature security boundaries;
- creates a clean seam for future package grouping and UI.

Cons:

- users still manage Skills and MCP from separate screens;
- no package-level enable toggle across heterogeneous resource kinds yet.

### B. Snapshot plus unified Resources UI

Build A, add a sanitized runtime-resource API, and add one UI page that shows
builtins, installed Skills, selected MCP servers/tools, status, conflicts, and
links to the existing management flows.

Pros:

- visible diagnostics and one mental model for users;
- does not yet replace mature mutation APIs.

Cons:

- substantially larger frontend/API scope;
- risks presenting a unified toggle that cannot yet be atomic across the two
  persistence authorities.

### C. Full Pi-style package manager now

Create package manifests that may contribute executable extensions, Skills,
prompts, themes, and MCP definitions; add global/project enable state and trust.

Pros:

- closest surface parity with Pi.

Cons:

- requires a new signed/admitted package contract, migrations, project trust,
  extension sandbox/authority, prompt precedence, update/rollback, and a UI;
- would create exactly the second control plane the prior phase forbids if done
  before the runtime snapshot foundation.

## Recommendation

Ship A first. Define the sanitized resource DTO while keeping it internal so B
can be added without changing the snapshot contract. Defer C until executable
extension and prompt trust models are designed separately.
