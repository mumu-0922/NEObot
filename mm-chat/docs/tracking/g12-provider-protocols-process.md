# G12 Provider Protocols Process

## 2026-07-20 — Decision lock

The owner requested two protocol changes only: repair the already-visible
Gemini type, retain manual OpenAI-compatible URL/Key/model discovery without
provider presets, and add Anthropic Claude as one new native protocol.

Source inspection found that the administrator configuration and Gemini Models
connection test already accepted `Gemini`, but the Go runtime Provider resolver
handled only `OpenAI` and `OpenAI Compatible`. The visible Gemini choice was
therefore a server-mode migration gap. The local nanobot reference contains 41
named registry entries, but 34 share its OpenAI-compatible backend; that
registry breadth will not be copied into this UI.

Execution is split into Gemini and Anthropic commits. Evidence, runtime state,
rollback notes, and any live-provider use will be appended after each slice.
