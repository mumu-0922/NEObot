# Pi resource management and conversation picker correction

## Scope

Research only. This note records the product correction requested after the
live rollout of the Slash-heavy Resource Orchestration UI. No product code was
changed while producing this note.

## User correction

The previous implementation mixed three different jobs into the composer Slash
palette:

1. installing and uninstalling resources;
2. choosing which installed resources a conversation may use;
3. invoking or auto-selecting resources while an Agent Run is executing.

The intended UX separates those jobs:

- **Skill Store / Tools pages** own inventory management: search, install,
  configure, health, update and uninstall;
- **composer Skill and MCP buttons** own the current conversation's durable
  selection of already-installed resources;
- **Agent runtime** may automatically choose a relevant installed resource for
  a task, within the conversation's authority and a frozen Run snapshot;
- pasting a supported Skill/MCP link into chat can start the same secure
  install/configuration workflow used by the management pages.

The lifecycle-oriented `/skill`, `/mcp`, `/resources` and `/reload` command wall
is therefore rejected as the primary Resource UX.

## What Pi actually separates

### Package inventory

The local Pi Web checkout keeps installation management outside the composer:

- `components/SkillsConfig.tsx` lists loaded Skills, searches skills.sh,
  installs by global/project scope, checks updates and changes Skill dormancy;
- `components/PluginsConfig.tsx` manages installed packages and delegates
  install/remove/update/enable/disable to `/api/plugins`;
- `app/api/skills/install/route.ts` performs project-trust checks before
  installing a project Skill;
- `app/api/plugins/route.ts` uses Pi's `SettingsManager` and
  `DefaultPackageManager` as the package authority.

The official Pi Web repository describes `/api/skills` as a projection from
`DefaultResourceLoader`, so the management surface and Agent runtime see the
same loaded resources:

- https://github.com/agegr/pi-web/blob/main/AGENTS.md
- https://github.com/agegr/pi-web/blob/main/README.md

### Session/runtime controls

`components/ChatInput.tsx` renders compact composer controls, including a
session Tool preset dropdown (`off`, `read-only`, `default`, `full`). The Tool
preset is not a package installer. It changes the active session's Tool policy.

Pi Skills can be explicitly invoked with `/skill:<name>` or selected by the
model through progressive disclosure. This is runtime use, not package
installation:

- https://github.com/earendil-works/pi/blob/main/packages/coding-agent/README.md

Pi's `PackageManager` changes installed packages; `DefaultResourceLoader`
discovers the resulting Skills/Extensions/Prompts/Themes and reloads its cached
runtime projection:

- https://github.com/earendil-works/pi/issues/645

### Slash is not the correct management hub

Pi Web exposes Slash command discovery, but an upstream discussion documents
the exact failure visible in Neo: a large Skill/Prompt set turns `/` into a wall
and proposes a dedicated Skill browser instead:

- https://github.com/earendil-works/pi/discussions/1623

Pi Web has also historically preferred GUI equivalents for Web interactions
whose TUI Slash commands require host-side dispatch:

- https://github.com/agegr/pi-web/issues/68

Conclusion: Pi supports the user's direction. Package management belongs in a
dedicated surface; compact session controls belong near the composer; automatic
Skill loading belongs in the runtime. Neo should adopt this separation instead
of copying every command into `/`.

## Neo source findings

### Reusable pieces

- `frontend/src/components/skills/SkillStore.tsx` already lists installed
  Package Skills and supports install/uninstall.
- `frontend/src/components/mcp/McpToolsPage.tsx` already separates Installed and
  Marketplace tabs.
- `frontend/src/components/mcp/McpToolsControl.tsx` already edits a durable,
  revision-bound per-conversation MCP selection.
- `backend/migrations/074_mcp_tools_foundation.up.sql` already provides
  `mcp_conversation_selections` and `mcp_conversation_servers`.
- `backend/internal/skillsupply` already owns admitted, immutable, per-user
  Skill installation.
- `backend/internal/chat/resource_tool.go` already supports bounded Agent
  discovery and secure install proposals without exposing direct package
  mutation to the model.
- Runtime Resource Snapshots already freeze the effective resources for a Run.

### Missing pieces

- Skill installations are user inventory only. There is no authoritative
  per-conversation Skill selection equivalent to MCP selection.
- `PrepareRuntimeSkills` currently materializes every installed Skill for the
  user; it cannot yet filter by the current conversation's policy.
- `MessageInput.tsx` has no Skill/MCP picker controls. It only exposes Knowledge,
  Agent/Chat mode, permission, reasoning and search.
- `ChatApp.tsx` currently loads every installed Skill to build a large Slash
  catalog and performs install/remove/enable/disable mutations from Slash.
- `lib/chat/slashCommands.ts` defines the lifecycle command wall the user has
  rejected.
- Conversational orchestration understands natural-language install intent,
  but there is no first-class pasted-link detector and install-preview flow.

## Corrected product model

```text
Inventory authority
  Skill Store                         Tools / MCP Marketplace
  install · update · uninstall       install · configure · health · uninstall
            |                                      |
            +---------- installed inventory -------+
                               |
Conversation policy            v
  composer [Skill] picker + [MCP] picker
  durable per-conversation enabled set, independent between conversations
                               |
Run authority                  v
  frozen Runtime Resource Snapshot
  = conversation-enabled resources
    + bounded Agent auto-activation permitted by policy
                               |
Execution                     v
  progressive Skill loading / MCP Tool calls / audited install proposal
```

### Invariants

1. Installing a resource does not automatically enable it in every existing
   conversation.
2. Changing selection in conversation A does not change conversation B.
3. Composer selectors never install, update or uninstall packages.
4. Store/Tools pages never silently rewrite every conversation's selection.
5. A Run uses a frozen snapshot. Mid-run installation creates a fresh,
   causally-linked continuation segment rather than mutating the active step.
6. Secret/OAuth configuration remains in the Tools UI and Backend vault.
7. Project-local resources remain subject to Workspace trust; Pi Web documents
   the same trust requirement for project resource loading:
   https://github.com/agegr/pi-web/issues/236

## Recommended MVP

### Composer

- Add one compact Skill icon and one compact MCP icon beside the existing
  Knowledge/Agent controls.
- Each opens a searchable multi-select popover of installed, authorized
  resources only.
- The button visually distinguishes zero, one and multiple selections; selected
  names are shown in the popover, not as a row of permanent chips.
- Selection is saved immediately through a revision-bound Backend endpoint and
  reloaded when switching conversations or refreshing the page.
- During an active Run the current snapshot is locked; a user selection change
  applies to the next Run/continuation boundary and is labelled accordingly.

### Management pages

- Skill Store remains the full installed/store/detail surface and owns
  install/uninstall.
- Tools remains the full installed/marketplace/configuration/health surface and
  owns install/uninstall/configuration.
- Remove conversation-selection responsibilities from the large management
  pages once the composer picker is authoritative, while retaining diagnostics.

### Slash and conversational links

- Remove Resource lifecycle commands from the root Slash palette.
- Keep any genuinely session-oriented commands only if they have independent
  product value; do not use Slash as a second management UI.
- A pasted supported Skill/MCP URL is treated as untrusted install intent:
  parse host and immutable identifier server-side, show a sanitized preview,
  and delegate to the existing Skill/MCP authority.
- Unknown/arbitrary URLs remain ordinary chat text or bounded discovery input;
  the Agent never runs `git`, `npm`, Docker or Bash to install them.

## Main unresolved preference

When the Agent auto-selects an already-installed Skill/MCP to complete a task,
there are three valid persistence policies:

1. **Run-only (recommended):** use it only in the current Run/continuation;
   persistent conversation selection changes only when the user clicks the
   composer picker.
2. **Persist automatically:** write it into the current conversation selection,
   making later turns reuse it without explicit user action.
3. **Ask before use:** require a lightweight confirmation before activating an
   installed but unselected resource.

Run-only best preserves user-visible state, avoids surprising future authority
expansion, and matches the frozen snapshot model. This preference must be fixed
before the persistence and runtime contracts are implemented.

