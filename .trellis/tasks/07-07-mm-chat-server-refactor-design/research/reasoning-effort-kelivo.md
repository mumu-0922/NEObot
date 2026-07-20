# Reasoning Effort Research

## Sources

- Kelivo repository commit `92db9a4`:
  `lib/features/chat/widgets/reasoning_budget_sheet.dart`,
  `lib/desktop/reasoning_budget_popover.dart`, and
  `lib/core/services/api/providers/openai_common.dart`.
- OpenAI reasoning guide, read 2026-07-20:
  <https://developers.openai.com/api/docs/guides/reasoning>.
- OpenAI latest-model guide, read 2026-07-20:
  <https://developers.openai.com/api/docs/guides/latest-model>.

## Kelivo Behavior

Kelivo stores one semantic `thinkingBudget` setting and presents Off, Auto,
Light, Medium, Heavy, conditionally XHigh/Max, and a custom token budget. It
then adapts that value per provider instead of forwarding the raw token count
everywhere:

- OpenAI maps budgets to `reasoning_effort` and normalizes unsupported levels
  against a model-family support matrix.
- Gemini maps the same presets to either `thinkingBudget` or `thinkingLevel`.
- Anthropic maps them to thinking budget or newer-model effort contracts.
- XHigh/Max are hidden unless the selected model supports them.

## OpenAI Contract

OpenAI documents model-dependent effort values including `none`, `minimal`,
`low`, `medium`, `high`, `xhigh`, and `max`. GPT-5.6 supports `none`, `low`,
`medium`, `high`, `xhigh`, and `max`; omitted effort uses a model-dependent
default. The UI must therefore not assume every model accepts every value.

## mm-chat Adaptation

Use a typed semantic level rather than exposing a provider token budget:

- UI: Off, Auto, Low, Medium, High, conditionally XHigh and Max.
- Persist `reasoningEffort` alongside the existing `useReasoning` boolean.
- Go validates the untrusted config and preserves legacy `useReasoning=true`
  as High when no level exists.
- OpenAI-compatible and Responses requests use model-normalized effort.
- Anthropic maps levels to bounded budgets while keeping `budget_tokens` below
  `max_tokens`.
- Unknown OpenAI-compatible models clamp XHigh/Max to High instead of sending a
  likely unsupported value.

This preserves the existing on/off contract while adding model-aware strength
without copying Kelivo's provider-specific token-budget state into the UI.
