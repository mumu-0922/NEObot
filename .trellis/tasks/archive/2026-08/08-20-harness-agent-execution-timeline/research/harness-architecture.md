# DeepSeek Harness architecture reference

- Source: <https://github.com/deepseek-ai/deepseek-harness>
- Inspected commit: `141eb6fef83422698aef7a981029e843e8161534`
- Detailed prior research:
  `.trellis/tasks/archive/2026-08/08-20-show-terminal-agent-execution-details/research/deepseek-harness-terminal-presentation.md`

Harness persists a durable `tool/call -> tool/result` sequence. Tool-owned pure
`presentCall` and `presentResult` functions produce provider-neutral tagged
render intents. The frontend joins call and result by call identity and renders
typed cards; the same session events drive live updates and replay.

neo-chat should adopt those contracts and interaction semantics, not the whole
Harness framework. The backend must remain authoritative and must persist only
bounded, sanitized presentation data.

