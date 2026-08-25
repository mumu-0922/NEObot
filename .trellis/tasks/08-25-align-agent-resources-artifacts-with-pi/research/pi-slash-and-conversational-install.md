# Pi slash commands and conversational resource acquisition

## Scope

Research only. No product code was changed. Sources are the local Pi Web checkout,
its installed `@earendil-works/pi-coding-agent` SDK, Pi official documentation,
and Neo Chat source.

## Pi findings

### Full SDK/TUI built-ins

`/home/mumu/projects/pi-web/node_modules/@earendil-works/pi-coding-agent/dist/core/slash-commands.js`
defines:

- settings, model, scoped-models
- export, import, share, copy
- name, session, changelog, hotkeys
- fork, clone, tree
- trust, login, logout
- new, compact, resume, reload, quit

This list is a TUI command surface, not a requirement that every command belongs
in a Web product.

### Pi Web command surface

`/home/mumu/projects/pi-web/components/ChatInput.tsx` exposes only five Web
built-ins: compact, reload, name, session, copy. It then merges commands returned
by `get_commands` and groups them as extension, prompt, and skill.

`/home/mumu/projects/pi-web/lib/rpc-manager.ts` builds `get_commands` from the
same live AgentSession resource graph:

- registered Extension commands;
- prompt templates;
- every loaded Skill as `skill:<name>`.

`/home/mumu/projects/pi-web/node_modules/@earendil-works/pi-coding-agent/dist/core/agent-session.js`
expands `/skill:<name> args` into the selected Skill's `SKILL.md` body plus args
before starting the model prompt. `/reload` reloads resources.

### Installation is separate from slash invocation

Pi's official Skills documentation defines discovery paths and explicit
`/skill:<name>` invocation. Pi Packages may bundle Extensions, Skills, Prompts,
and Themes, while CLI/UI management handles package installation.

Pi Web confirms the separation:

- `/api/skills/search` searches skills.sh;
- `/api/skills/install` runs the Skill installer after project trust checks;
- `/api/plugins` delegates install/remove/update/enable/disable to
  `DefaultPackageManager`;
- neither API is exposed to the Agent model as an autonomous Tool by default.

Therefore “Agent detects a gap and installs a resource” is a Neo product feature
beyond Pi parity. The reusable Pi lesson is unified resource loading and explicit
reload, not silent package mutation.

Official references:

- https://badlogic-pi-mono.mintlify.app/coding-agent/skills
- https://badlogic-pi-mono.mintlify.app/coding-agent/extensions
- https://badlogic-pi-mono.mintlify.app/coding-agent/pi-packages
- https://badlogic-pi-mono.mintlify.app/concepts/extensibility

## Neo Chat findings

### Skill supply

- `mm-chat/backend/internal/skillsupply/handler.go` exposes Candidate ingest/review,
  admitted Store list/detail/install, and owner library list/uninstall.
- Installation requires an exact candidate ID plus package fingerprint.
- Candidate admission is administrator-only; normal user install cannot bypass it.
- `mm-chat/backend/internal/chat/local_skill_selection.go` supports deterministic
  legacy `/skill-name`, not Pi's `/skill:<name>`.
- Runtime Skill discovery is progressive disclosure through the `skill` Tool.

Gap: no general discovery provider/search query, no Agent-accessible search or
install proposal Tool, no slash picker, and no installation continuation flow.

### MCP supply

- `mm-chat/backend/internal/mcpclient/handler.go` exposes marketplace
  search/detail/install and server/selection/auth management.
- `mm-chat/backend/internal/mcpclient/marketplace.go` re-fetches exact versions,
  validates deployment hashes, restricts compatible transports, stores credentials
  server-side, performs online validation, and can enable a Server for a conversation.
- Marketplace install is administrator-only today.
- Secret/OAuth/custom endpoint details require a structured configuration path.

Gap: the capability exists in the management UI but is not callable through a
conversation command or safe Agent proposal Tool.

### Runtime

- `mm-chat/backend/internal/chat/agent_runtime_resources.go` freezes authorized
  Skills, MCP Tools, built-ins, and diagnostics into a per-Run snapshot.
- Mid-run installation cannot simply mutate provider Tool definitions without
  breaking trace/replay consistency.

Required seam: install is a state transition outside the frozen step, followed
by a new snapshot and a causally linked continuation segment.

## Recommended architecture

### Command plane

Add a typed command registry shared by Backend and composer. Commands carry:

- name, aliases, description, argument schema;
- category and source;
- availability/permission state;
- execution kind: client action, read query, mutation proposal, or Skill invoke.

The composer uses the registry for autocomplete; Backend remains authoritative
for parsing and execution. Do not implement slash commands as fragile prompt text.

### Resource Orchestrator

Introduce a thin orchestration layer with adapters:

- Skill catalog/discovery -> existing skillsupply service;
- MCP catalog/discovery -> existing mcpclient service;
- mutation -> exact existing domain methods;
- runtime refresh -> existing Agent Runtime Resource Registry.

The orchestrator owns only cross-domain proposal state, approval/config handoff,
audit correlation, idempotency, and run continuation. It must not own package
validation, credentials, MCP selection, or Skill admission.

### Agent tools

Only expose safe intent Tools:

- `resource_search`: read-only bounded query returning redacted candidates;
- `resource_request_install`: creates an exact revision-fenced proposal;
- optionally `resource_status`: read-only installed/current-run status.

Do not expose `resource_install` as a direct model mutation Tool. A proposal is
resolved by policy/UI, and the actual mutation remains server-side.

### Continuation

The safe lifecycle is:

1. current Run identifies a capability gap;
2. it searches and requests one exact candidate;
3. Run enters waiting-for-resource state or closes a segment;
4. approval/configuration executes through existing authority;
5. Backend creates a fresh resource snapshot;
6. a linked continuation resumes the original task with the mutation result.

This gives users “continue automatically” without hot-swapping Tool definitions
inside an immutable Run.

## Main risks

- supply-chain attack through model-chosen packages;
- prompt injection in catalog metadata/descriptions;
- Secret leakage through chat or trace;
- infinite search/install loops;
- concurrent duplicate install and stale candidate revision;
- installing a resource when the real problem is bad arguments, transient network,
  or a broken existing Tool;
- divergence between slash, natural-language, management UI, and runtime state.

All catalog metadata must remain untrusted data, all writes revision-fenced and
audited, and all entry paths must call the same domain services.
