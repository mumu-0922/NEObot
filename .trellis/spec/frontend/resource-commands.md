# Composer Resource Command Contract

## 1. Scope / Trigger

Apply when changing composer slash autocomplete, deterministic Skill
invocation, Skill/MCP discovery/install commands, or Resource process cards.

## 2. Signatures

```ts
type SlashCommandGroup = "builtin" | "skill" | "mcp" | "session";

buildSlashCommands(installedSkillNames: readonly string[]): SlashCommandDefinition[]
filterSlashCommands(commands, value): SlashCommandDefinition[]
parseSlashCommand(value): ParsedSlashCommand | null
```

The typed client exposes `resources.getCatalog`, `resources.search`, and
`resources.install`. Server responses pass strict Zod schemas before use.

## 3. Contracts

- Typing `/` opens a scrollable grouped palette containing every built-in and
  every valid installed `/skill:<name>` command. Do not truncate away `/mcp`,
  `/resources`, `/reload`, or later dynamic Skills.
- Support Arrow Up/Down, Tab/Enter selection, Escape dismissal, mouse hover and
  click, argument hints, loading, and empty-result states. The input retains
  keyboard focus after insertion.
- Slash parsing is deterministic and runs before normal message submission.
  Unknown commands show an error and are not sent to the model. An installed
  `/skill:<name> [args]` is sent so Backend deterministic loading remains
  authority; an unknown Skill is rejected locally.
- Search/install commands call the shared Resource API. They never copy
  candidate revisions from browser storage. Credential-requiring MCP entries
  open the existing Marketplace configuration UI instead of collecting a
  Secret in chat.
- `/reload` is denied while a Run is active. Otherwise refresh installed Skill
  commands and read the current Resource catalog. Browser state is not Resource
  authority.
- Resource process cards render only the sanitized `card=resource`
  presentation and the existing approval projection. Raw Tool arguments,
  results, candidate bodies, credentials, URLs, and paths never render.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| input is not one single-line slash command | normal composer path |
| unknown command | visible bounded error; no model/API mutation |
| `/skill:<name>` not in current installed library | reject locally |
| install lacks exact search match | open management surface; no write |
| MCP requires auth/configuration | open Marketplace; no chat Secret field |
| server response violates DTO | `INVALID_SERVER_RESPONSE`; do not render/use |
| Run active and `/reload` entered | reject; preserve current snapshot |
| palette query has no match | visible empty state; Enter follows normal unknown-command handling |

## 5. Good / Base / Bad Cases

- **Good:** type `/`, select `/skill install`, search exact candidate, install
  through the shared API, then see the dynamic `/skill:<name>` after refresh.
- **Base:** no installed Skills; built-in Skill/MCP/Resource/Reload commands
  remain discoverable.
- **Bad:** slice the palette to eight entries, infer mutation intent with an
  LLM, save a candidate fingerprint in localStorage, or add credential inputs
  to the composer.

## 6. Tests Required

- Build/filter/parse/insertion tests, unsafe Skill-name rejection, group order,
  and root `/` inclusion of required commands and dynamic Skills.
- MessageInput composition for grouped loading/empty palette and keyboard
  controls.
- Resource client route/body/strict-response tests and process-card sanitizer,
  approval, and Secret/path rejection tests.
- Skill Store exact detail fallback for candidates outside the first list page.
- Frontend format, lint, strict typecheck, full Vitest, and production build.

## 7. Wrong vs Correct

```text
Wrong: submit "/mcp install X" as prose and hope the model mutates correctly
Correct: deterministic parser -> Resource search -> exact server install API

Wrong: show only the first eight slash entries
Correct: bounded-height scrolling list containing every authorized command
```
