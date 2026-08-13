# LobeHub Agent Marketplace Fit

## Finding

Neo Chat's current Assistant catalog is already a compatibility adapter over
the legacy LobeHub public Agent registry. The Backend reads localized static
JSON from `@lobehub/agents-index`, exposes `/v1/agents` list/detail, and bounds
the payload before the Frontend consumes it. The current UI records a remote
Assistant in browser-owned `usedAgents` only after selection; this resembles a
recently-used list, not an explicit install lifecycle.

The current supported Assistant snapshot is deliberately narrow:

- identifier;
- avatar, title, description, tags, category;
- author/homepage/created time;
- bounded `systemRole` from detail.

Current selection applies the `systemRole` as a Conversation system
instruction and creates or retitles a Conversation. It does not install an
executable runtime.

## Live LobeHub Difference

The current LobeHub Agent pages may describe richer configuration, including
`systemRole`, chat settings, plugins/tools, and content that assumes a specific
external integration. A direct import of only the prompt can therefore be
functionally incomplete. For example, an Agent can claim use of a research
plugin even when Neo Chat has not installed or selected the corresponding MCP
Server.

Live verification on 2026-08-13 found that the current marketplace exposes a
public React Router `.data` route. It returns paged serialized payloads with
fields including `systemRole`, `updatedAt`, `versionNumber`, install counts,
categories, and a reported `totalMarketCount` around 286.7k. The ordinary
category result window is separately bounded (the sampled `all` query returned
840 results across 40 pages of 21), so the global count must not be presented
as an exact browseable result count.

The `.data` route is a framework-internal public route, not a documented stable
marketplace API. It must therefore be consumed only through a bounded Backend
adapter with strict schema normalization, cache, timeouts, response-size
limits, circuit-breaking/fail-open behavior, and the legacy Registry as a
read-only discovery fallback. The browser must never fetch or parse LobeHub
HTML or React Router serialization directly.

The live catalog visibly includes third-party jailbreak/DAN prompts, fictional
browsing instructions, automated trading, and other high-risk presets. Catalog
availability is not a trust decision. Any production adapter must preserve
upstream official/validated signals only as display metadata and enforce a
separate Neo Chat admission policy.

## Recommended Product Contract

Use a true `My Assistants | Assistant Store` flow, but install a supported,
immutable Assistant preset snapshot rather than treating an Agent as an MCP
package.

An MVP installation should import only:

- source provenance and stable identifier/fingerprint;
- display metadata and bounded system instruction;
- optional display-only declaration of required capabilities/tools.

It must not automatically import or activate:

- provider credentials or secrets;
- model/provider selection;
- MCP Servers, plugins, or Tools;
- network/file/code permissions;
- unsupported chat configuration.

When an upstream Agent requires a Tool, show `Requires Tools` and let the user
open the existing MCP Marketplace. Assistant installation never installs or
enables the Tool implicitly.

## Architecture Options

### A. Upgrade the current adapter (recommended)

- Keep the Backend proxy/cache/normalization boundary.
- Add explicit per-user installed Assistant snapshots and provenance.
- Redesign the Frontend into `My Assistants | Assistant Store`.
- Migrate custom Assistants; treat legacy `usedAgents` as recent history or
  offer a one-time bounded migration rather than silently expanding authority.

This preserves the existing security boundary and minimizes source churn.

For the data source, extend this option with a live-market Backend adapter and
legacy Registry fallback rather than replacing the boundary with browser
scraping.

### B. Scrape the live LobeHub website

Not recommended. HTML/React payloads are not a stable contract, may drift, and
would make availability and parsing brittle. The browser must not scrape the
upstream directly.

### C. Full-fidelity Agent package import

Not recommended for MVP. It couples Assistants to provider/model settings and
Tool installation, weakens the clean Assistant/Skill/Tool separation, and
requires dependency resolution plus permission UX.

## Key Difference from MCP

```text
Assistant install -> copy a bounded role/workflow preset for Conversations
MCP install       -> deploy or register an executable external Tool endpoint
```

The stores may share browsing patterns, categories, installed state, and
provenance UX. They must not share execution or authorization semantics.

## Relevant Code

- `mm-chat/backend/internal/agents/service.go`
- `mm-chat/backend/internal/agents/handler.go`
- `mm-chat/frontend/src/components/assistant/AssistantHub.tsx`
- `mm-chat/frontend/src/services/api/agentService.ts`
- `mm-chat/frontend/src/lib/market/agents.ts`
- `mm-chat/frontend/src/store/core/settingsStore.ts`
- `mm-chat/frontend/src/components/app/ChatApp.tsx`
