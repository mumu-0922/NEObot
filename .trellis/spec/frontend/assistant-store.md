# Assistant Store Frontend Contract

## Scenario: My Assistants and Assistant Store

### 1. Scope / Trigger

Use this contract for `AssistantHub`, Assistant DTOs/client methods, legacy
Assistant persistence retirement, or starting a Conversation from an Assistant.

### 2. Signatures

- Navigation: `My Assistants | Assistant Store`.
- Store search sends `pageSize=20`, query, category, and normalized locale.
- The category rail follows the Backend-supplied LobeHub UI allowlist; Chinese
  `marketing` is presented as `商业`, and zero-result aggregate-only categories
  are not synthesized client-side.
- Mutations send the Backend-issued `revision` or confirmed `fingerprint`.
- `onSelect(LobeAgent)` is called only from an installed library entry carrying
  the complete `meta.systemRole` snapshot.

### 3. Contracts

- Backend storage is the only authority for installed and custom Assistants.
  Legacy `customAgents`, `usedAgents`, overrides, and market cache are cleared
  by an incremented persistence migration and are not persisted again.
- Store cards open detail; they never start a chat. Installation and starting a
  chat are separate explicit actions.
- Installation/review always shows that plugins, Knowledge Bases, external
  Tools, Models, secrets, and permissions are not included.
- Structured Tool declarations survive installation as display metadata in My
  Assistants, are visible before start, and link to Tools. The link never
  installs, selects, enables, or authorizes a Tool.
- Store updates are manual: admit a new upstream fingerprint, open My
  Assistants, confirm replacement, then submit revision/CAS update.
- A revision conflict preserves the draft and offers either loading the latest
  server version or saving the current draft as a new custom Assistant.
- Initial and next-page work is generation-fenced; initial searches abort stale
  requests, next-page failure preserves loaded results, and identifiers are
  deduplicated.
- Typed clients validate nested market agents/categories and pagination at
  runtime; malformed server data fails closed with `INVALID_SERVER_RESPONSE`.
- Assistant modal surfaces use the shared `Dialog` focus trap/restoration and
  Escape behavior; nested confirmations stop keyboard propagation.

### 4. Validation & Error Matrix

| Condition | UI result |
| --- | --- |
| Empty library | Open Store and show create/browse actions |
| Detail has no complete prompt/fingerprint | Disable install and show scoped error |
| Next page fails | Preserve cards and expose retry |
| Revision conflict | Preserve draft; show reload/copy recovery |
| Installed upstream item has a new unadmitted fingerprint | Show `Admit new version`, not reinstall |
| Local API mode | Explicit unsupported feature error; no browser-owned fallback |

### 5. Good / Base / Bad Cases

- Good: install in Store, start from My Assistants, and keep historical chat
  prompt snapshots unchanged after uninstall or update.
- Base: create several custom Assistants and sync them under one account.
- Bad: synthesize a prompt from title/description, trust a raw nested DTO, or
  silently enable a Tool named in the prompt.

### 6. Tests Required

- Focused Vitest for typed routes, malformed nested DTO rejection, tab/store
  composition, infinite scrolling, manual update confirmation, and CAS recovery.
- Run Prettier, ESLint, TypeScript, focused Vitest, and production build for a
  deployed UI change; verify compiled UI markers or rendered behavior.

### 7. Wrong vs Correct

#### Wrong

```typescript
return raw as AssistantMarketSearchResult;
instruction ||= `You are ${agent.meta.title}`;
```

#### Correct

```typescript
const agent = requireMarketAgent(rawAgent);
const instruction = installedEntry.systemPrompt.trim();
if (!instruction) return showScopedError();
```
